package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ── helper constructors ──────────────────────────────────────────────────────

func newTestGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c
}

func setupVisionProxyServer(t *testing.T, providerUUID, providerURL, model string, enabled bool) *Server {
	t.Helper()
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	if enabled {
		require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{
			Scenario: typ.ScenarioOpenAI,
			VisionProxy: typ.VisionProxyConfig{
				Enabled:    true,
				ProviderID: providerUUID,
				Model:      model,
				TimeoutMs:  3000,
			},
		}))
	}

	if providerUUID != "" {
		_ = cfg.AddProvider(&typ.Provider{
			UUID:    providerUUID,
			Name:    "vision-mock",
			APIBase: providerURL,
			Token:   "test-token",
			Enabled: true,
			Timeout: 10,
		})
	}

	return NewServer(cfg)
}

// ── helper function tests ────────────────────────────────────────────────────

func TestCollectOpenAIUserText(t *testing.T) {
	parts := []openai.ChatCompletionContentPartUnionParam{
		openai.TextContentPart("Hello"),
		openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: "https://example.com/img.png"}),
		openai.TextContentPart("world"),
	}
	text := collectOpenAIUserText(parts)
	assert.Equal(t, "Hello\nworld", text)
}

func TestCollectOpenAIUserText_TextOnly(t *testing.T) {
	parts := []openai.ChatCompletionContentPartUnionParam{
		openai.TextContentPart("Only text"),
	}
	assert.Equal(t, "Only text", collectOpenAIUserText(parts))
}

func TestCollectOpenAIUserText_Empty(t *testing.T) {
	assert.Equal(t, "", collectOpenAIUserText(nil))
}

func TestCollectAnthropicUserText(t *testing.T) {
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("first"),
		anthropic.NewTextBlock("second"),
	}
	assert.Equal(t, "first\nsecond", collectAnthropicUserText(content))
}

func TestCollectBetaAnthropicUserText(t *testing.T) {
	content := []anthropic.BetaContentBlockParamUnion{
		anthropic.NewBetaTextBlock("beta text"),
	}
	assert.Equal(t, "beta text", collectBetaAnthropicUserText(content))
}

func TestAnthropicImageToURL_URL(t *testing.T) {
	img := &anthropic.ImageBlockParam{
		Source: anthropic.ImageBlockParamSourceUnion{
			OfURL: &anthropic.URLImageSourceParam{URL: "https://example.com/img.png"},
		},
	}
	assert.Equal(t, "https://example.com/img.png", anthropicImageToURL(img))
}

func TestAnthropicImageToURL_Base64(t *testing.T) {
	img := &anthropic.ImageBlockParam{
		Source: anthropic.ImageBlockParamSourceUnion{
			OfBase64: &anthropic.Base64ImageSourceParam{
				MediaType: "image/jpeg",
				Data:      "abc123",
			},
		},
	}
	assert.Equal(t, "data:image/jpeg;base64,abc123", anthropicImageToURL(img))
}

func TestAnthropicImageToURL_Nil(t *testing.T) {
	assert.Equal(t, "", anthropicImageToURL(nil))
}

func TestBetaAnthropicImageToURL_URL(t *testing.T) {
	img := &anthropic.BetaImageBlockParam{
		Source: anthropic.BetaImageBlockParamSourceUnion{
			OfURL: &anthropic.BetaURLImageSourceParam{URL: "https://example.com/beta.png"},
		},
	}
	assert.Equal(t, "https://example.com/beta.png", betaAnthropicImageToURL(img))
}

// ── applyVisionProxyOpenAI disabled → no-op ──────────────────────────────────

func TestApplyVisionProxyOpenAI_Disabled(t *testing.T) {
	s := setupVisionProxyServer(t, "", "", "", false)
	c := newTestGinContext()

	req := protocol.OpenAIChatCompletionRequest{}
	req.Model = "gpt-4o-mini"
	req.Messages = []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
			openai.TextContentPart("What is this?"),
			openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: "https://example.com/img.png"}),
		}),
	}

	s.applyVisionProxyOpenAI(c, &req, typ.ScenarioOpenAI)

	// With the proxy disabled, the message should be unchanged.
	assert.Equal(t, 2, len(req.Messages[0].OfUser.Content.OfArrayOfContentParts))
}

// ── applyVisionProxyOpenAI enabled → replace image with description ──────────

func TestApplyVisionProxyOpenAI_Enabled_ReplacesImagePart(t *testing.T) {
	// Stand up a local mock that returns a fixed description.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4o-vision",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "a red apple on a table",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer mockServer.Close()

	s := setupVisionProxyServer(t, "vision-provider", mockServer.URL, "gpt-4o-vision", true)
	c := newTestGinContext()

	req := protocol.OpenAIChatCompletionRequest{}
	req.Model = "gpt-4o-mini"
	req.Messages = []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
			openai.TextContentPart("What's in this image?"),
			openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: "https://example.com/apple.png"}),
		}),
	}

	s.applyVisionProxyOpenAI(c, &req, typ.ScenarioOpenAI)

	parts := req.Messages[0].OfUser.Content.OfArrayOfContentParts
	require.Len(t, parts, 2, "text part + description part")

	// Second part should now be a text description.
	require.NotNil(t, parts[1].OfText)
	assert.True(t, strings.Contains(parts[1].OfText.Text, "a red apple on a table"))
}

// ── applyVisionProxyOpenAI: fail-open when provider missing ──────────────────

func TestApplyVisionProxyOpenAI_FailOpen_BadProvider(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	require.NoError(t, err)

	// Enable vision proxy but point to a non-existent provider UUID.
	require.NoError(t, cfg.SetScenarioConfig(typ.ScenarioConfig{
		Scenario: typ.ScenarioOpenAI,
		VisionProxy: typ.VisionProxyConfig{
			Enabled:    true,
			ProviderID: "does-not-exist",
			Model:      "gpt-4o",
			TimeoutMs:  500,
		},
	}))
	s := NewServer(cfg)
	c := newTestGinContext()

	req := protocol.OpenAIChatCompletionRequest{}
	req.Model = "gpt-4o-mini"
	req.Messages = []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
			openai.TextContentPart("Describe it"),
			openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: "https://example.com/img.png"}),
		}),
	}

	// Must not panic and must not block.
	s.applyVisionProxyOpenAI(c, &req, typ.ScenarioOpenAI)

	parts := req.Messages[0].OfUser.Content.OfArrayOfContentParts
	// The image part was dropped (fail-open); only the text part remains.
	require.Len(t, parts, 1)
	assert.NotNil(t, parts[0].OfText)
}

// ── cache: same image in one request calls vision model only once ─────────────

func TestApplyVisionProxyOpenAI_CacheDeduplicatesImage(t *testing.T) {
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := map[string]interface{}{
			"id": "chatcmpl-cache-test", "object": "chat.completion",
			"created": time.Now().Unix(), "model": "gpt-4o-vision",
			"choices": []map[string]interface{}{
				{"index": 0, "message": map[string]interface{}{"role": "assistant", "content": "a cat"}, "finish_reason": "stop"},
			},
			"usage": map[string]interface{}{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer mockServer.Close()

	s := setupVisionProxyServer(t, "vision-provider", mockServer.URL, "gpt-4o-vision", true)
	c := newTestGinContext()

	sameURL := "https://example.com/cat.png"
	req := protocol.OpenAIChatCompletionRequest{}
	req.Model = "gpt-4o-mini"
	req.Messages = []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage([]openai.ChatCompletionContentPartUnionParam{
			openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: sameURL}),
			openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{URL: sameURL}),
		}),
	}

	s.applyVisionProxyOpenAI(c, &req, typ.ScenarioOpenAI)

	assert.Equal(t, 1, callCount, "vision model should be called only once for the same image URL")
}
