package server

import (
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// audioUploadMaxMemory caps in-memory multipart parsing for audio uploads.
// OpenAI tops out at 25 MiB per file; 32 MiB leaves headroom for form fields
// before gin spills the body to disk.
const audioUploadMaxMemory int64 = 32 << 20

// audioFormFields holds the parsed multipart fields shared between the
// transcription and translation handlers. Translations ignore language and
// timestamp_granularities; the parser still extracts them so callers can
// validate or pass them through uniformly.
type audioFormFields struct {
	file                   multipart.File
	header                 *multipart.FileHeader
	model                  string
	language               string
	prompt                 string
	responseFormat         string
	temperature            *float64
	timestampGranularities []string
}

// parseAudioForm parses a multipart/form-data request for the audio endpoints.
// On success the caller MUST close `fields.file`.
func parseAudioForm(c *gin.Context) (*audioFormFields, error) {
	if err := c.Request.ParseMultipartForm(audioUploadMaxMemory); err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file is required: %w", err)
	}

	model := c.PostForm("model")
	if model == "" {
		file.Close()
		return nil, fmt.Errorf("model is required")
	}

	if header.Size <= 0 {
		file.Close()
		return nil, fmt.Errorf("file is empty")
	}

	fields := &audioFormFields{
		file:           file,
		header:         header,
		model:          model,
		language:       c.PostForm("language"),
		prompt:         c.PostForm("prompt"),
		responseFormat: c.PostForm("response_format"),
	}

	if tempStr := c.PostForm("temperature"); tempStr != "" {
		v, err := strconv.ParseFloat(tempStr, 64)
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("invalid temperature: %w", err)
		}
		fields.temperature = &v
	}

	// OpenAI sends timestamp granularities as `timestamp_granularities[]` form values.
	if c.Request.PostForm != nil {
		fields.timestampGranularities = c.Request.PostForm["timestamp_granularities[]"]
	}

	return fields, nil
}

// HandleOpenAIAudioTranscriptions serves OpenAI-compatible /audio/transcriptions
// (Whisper speech-to-text) requests via the mixin route group.
//
// Reachable from any scenario whose descriptor declares TransportVoice or
// TransportOpenAI: the canonical home is the dedicated `voice` scenario, and
// `openai` (the general-purpose entry point) is extended to support TransportVoice.
func (s *Server) HandleOpenAIAudioTranscriptions(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) &&
		!typ.ScenarioSupportsTransport(scenarioType, typ.TransportVoice) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("scenario %s does not support audio transcriptions", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	fields, err := parseAudioForm(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	defer fields.file.Close()

	requestModel := fields.model
	responseModel := requestModel

	rule, err := s.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	provider, selectedService, err := s.routingSelector.SelectServiceForVoice(c, scenarioType, rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if provider.APIStyle != protocol.APIStyleOpenAI {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("unsupported provider api style for audio transcriptions: %s", provider.APIStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	actualModel := selectedService.Model

	params := openai.AudioTranscriptionNewParams{
		File:  openai.File(fields.file, fields.header.Filename, fields.header.Header.Get("Content-Type")),
		Model: openai.AudioModel(actualModel),
	}
	if fields.language != "" {
		params.Language = openai.String(fields.language)
	}
	if fields.prompt != "" {
		params.Prompt = openai.String(fields.prompt)
	}
	if fields.responseFormat != "" {
		params.ResponseFormat = openai.AudioResponseFormat(fields.responseFormat)
	}
	if fields.temperature != nil {
		params.Temperature = openai.Float(*fields.temperature)
	}
	if len(fields.timestampGranularities) > 0 {
		params.TimestampGranularities = fields.timestampGranularities
	}

	sessionID := resolveSessionID(c, nil)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	SetTrackingContext(c, rule, provider, actualModel, responseModel, false)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	resp, cancel, err := forwarding.ForwardOpenAIAudioTranscriptions(fc, wrapper, &params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		s.trackUsageWithTokenUsage(c, usage, err)
		logrus.Errorf("Failed to forward audio transcription request: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	// Audio transcription billing is duration-based; no token counts to record.
	usage := protocol.NewTokenUsageWithCache(0, 0, 0)
	s.trackUsageWithTokenUsage(c, usage, nil)

	// The response is a union (Transcription / TranscriptionVerbose / etc.).
	// Pass through the raw JSON to preserve the requested response_format shape.
	if raw := resp.RawJSON(); raw != "" {
		c.Data(http.StatusOK, "application/json", []byte(raw))
		return
	}

	// Fallback: re-marshal if RawJSON is unavailable.
	body, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

// HandleOpenAIAudioTranslations serves OpenAI-compatible /audio/translations
// (Whisper translates non-English audio into English text) requests.
func (s *Server) HandleOpenAIAudioTranslations(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	if !isValidRuleScenario(scenarioType) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("invalid scenario: %s", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) &&
		!typ.ScenarioSupportsTransport(scenarioType, typ.TransportVoice) {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("scenario %s does not support audio translations", scenario),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	fields, err := parseAudioForm(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	defer fields.file.Close()

	requestModel := fields.model
	responseModel := requestModel

	rule, err := s.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	provider, selectedService, err := s.routingSelector.SelectServiceForVoice(c, scenarioType, rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	if provider.APIStyle != protocol.APIStyleOpenAI {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("unsupported provider api style for audio translations: %s", provider.APIStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	actualModel := selectedService.Model

	// Translations does not accept language or timestamp_granularities; ignore those
	// fields if clients send them, rather than erroring.
	params := openai.AudioTranslationNewParams{
		File:  openai.File(fields.file, fields.header.Filename, fields.header.Header.Get("Content-Type")),
		Model: openai.AudioModel(actualModel),
	}
	if fields.prompt != "" {
		params.Prompt = openai.String(fields.prompt)
	}
	if fields.responseFormat != "" {
		params.ResponseFormat = openai.AudioTranslationNewParamsResponseFormat(fields.responseFormat)
	}
	if fields.temperature != nil {
		params.Temperature = openai.Float(*fields.temperature)
	}

	sessionID := resolveSessionID(c, nil)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	SetTrackingContext(c, rule, provider, actualModel, responseModel, false)

	wrapper := s.clientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	resp, cancel, err := forwarding.ForwardOpenAIAudioTranslations(fc, wrapper, &params)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		usage := protocol.NewTokenUsageWithCache(0, 0, 0)
		s.trackUsageWithTokenUsage(c, usage, err)
		logrus.Errorf("Failed to forward audio translation request: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to forward request: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}

	usage := protocol.NewTokenUsageWithCache(0, 0, 0)
	s.trackUsageWithTokenUsage(c, usage, nil)

	if raw := resp.RawJSON(); raw != "" {
		c.Data(http.StatusOK, "application/json", []byte(raw))
		return
	}

	body, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Failed to marshal response: " + err.Error(),
				Type:    "api_error",
			},
		})
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}
