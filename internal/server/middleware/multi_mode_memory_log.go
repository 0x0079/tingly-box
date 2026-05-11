package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/pkg/obs"
)

// Debug-capture memory budget. MaxRequestBodyBytes is the hard limit;
// MaxRequestBodies only bounds map overhead in many-tiny-requests cases.
// The 32MiB request budget is paired with a 32MiB StreamEventStore
// budget (configured in server init) for a 64MiB total ceiling.
const (
	MaxRequestBodies          = 200
	MaxRequestBodyBytes int64 = 32 * 1024 * 1024
)

// Gin context keys used by scenario_recording.go to attach stream events
// to the same body_ref this middleware emits.
const (
	CtxKeyBodyRef          = "body_ref"
	CtxKeyStreamEventStore = "stream_event_store"
)

// MultiModeMemoryLogMiddleware provides Gin middleware with both persistent and memory log storage
// Logs are written to:
// 1. Multi-mode logger (text + JSON files for persistence)
// 2. Memory (circular buffer for quick API access)
// 3. Request body store (pure memory, referenced by body_ref ID)
// 4. Stream event store (pure memory, keyed by the same body_ref)
type MultiModeMemoryLogMiddleware struct {
	logger           *logrus.Logger
	multiLogger      *obs.MultiLogger
	requestBodyStore *obs.RequestBodyStore
	streamEventStore *obs.StreamEventStore
	sanitizeImages   bool
}

// NewMultiModeMemoryLogMiddleware creates a new middleware with both persistent and memory logging
func NewMultiModeMemoryLogMiddleware(multiLogger *obs.MultiLogger) *MultiModeMemoryLogMiddleware {
	if multiLogger == nil {
		// Fallback for test environments where no multi-logger is configured.
		l := logrus.New()
		if gin.Mode() == gin.TestMode {
			l.SetOutput(io.Discard)
		}
		return &MultiModeMemoryLogMiddleware{
			logger:           l,
			multiLogger:      nil,
			requestBodyStore: nil,
			sanitizeImages:   true,
		}
	}
	// Get a logger scoped to HTTP source
	httpLogger := multiLogger.GetLogrusLogger(obs.LogSourceHTTP)

	return &MultiModeMemoryLogMiddleware{
		logger:           httpLogger,
		multiLogger:      multiLogger,
		requestBodyStore: obs.NewRequestBodyStore(MaxRequestBodies, MaxRequestBodyBytes),
		sanitizeImages:   true,
	}
}

func (m *MultiModeMemoryLogMiddleware) SetStreamEventStore(s *obs.StreamEventStore) {
	if m == nil {
		return
	}
	m.streamEventStore = s
}

func (m *MultiModeMemoryLogMiddleware) GetStreamEventStore() *obs.StreamEventStore {
	if m == nil {
		return nil
	}
	return m.streamEventStore
}

// Middleware returns a Gin middleware compatible with gin.Logger()
// It logs all HTTP requests to both the multi-mode logger and memory
func (m *MultiModeMemoryLogMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Tee the request body so the handler still sees it; the body
		// is committed to the store after c.Next() once it has been
		// fully read.
		var bodyBuffer *bytes.Buffer
		var bodyRef string
		if m.requestBodyStore != nil && c.Request.Body != nil && c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			bodyBuffer = &bytes.Buffer{}
			c.Request.Body = io.NopCloser(io.TeeReader(c.Request.Body, bodyBuffer))
		}

		// Bound non-streaming response capture; SSE goes through
		// StreamEventStore instead and bypasses this buffer.
		w := &responseBodyWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
			limit:          maxBufferedResponseBytes,
		}
		c.Writer = w

		if m.streamEventStore != nil {
			c.Set(CtxKeyStreamEventStore, m.streamEventStore)
		}

		// Pre-assign body_ref so streaming recorders can attach events
		// to it before c.Next() returns.
		if m.requestBodyStore != nil && bodyBuffer != nil {
			bodyRef = m.requestBodyStore.ReserveID()
			c.Set(CtxKeyBodyRef, bodyRef)
		}

		// Process request
		c.Next()

		// Build log entry manually for more control
		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()

		if raw != "" {
			path = path + "?" + raw
		}

		// Extract error details if any (including panics caught by gin.Recovery)
		var errorMsg string
		var errorType string
		if len(c.Errors) > 0 {
			// Get the last error (most recent)
			lastErr := c.Errors.Last()
			errorMsg = lastErr.Error()

			// For panic errors, include additional context
			if lastErr.Type == gin.ErrorTypeBind {
				errorType = "bind_error"
			} else if lastErr.Type == gin.ErrorTypePublic {
				errorType = "public_error"
			} else if lastErr.Type == gin.ErrorTypePrivate {
				errorType = "private_error"
			} else {
				// Convert ErrorType to string safely
				errorType = fmt.Sprintf("error_type_%d", lastErr.Type)
			}
		}

		// Build fields with error message if available
		fields := logrus.Fields{
			"type":       "http_request",
			"status":     statusCode,
			"latency":    latency,
			"client_ip":  clientIP,
			"method":     method,
			"path":       path,
			"body_size":  bodySize,
			"user_agent": c.Request.UserAgent(),
		}

		// Always store the request body. Gating on 4xx/5xx misses
		// streaming responses that fail mid-flight after a 200 header.
		if bodyBuffer != nil && bodyRef != "" && bodyBuffer.Len() > 0 {
			body := bodyBuffer.String()
			if m.sanitizeImages {
				if sanitized, saved := obs.SanitizeBase64Images(body); saved > 0 {
					body = sanitized
					fields["body_image_bytes_omitted"] = saved
				}
			}
			m.requestBodyStore.Store(bodyRef, method, path, body)
			fields["body_ref"] = bodyRef
		}

		// Add error message field if error occurred
		if errorMsg != "" {
			fields["error"] = errorMsg
			if errorType != "" {
				fields["error_type"] = errorType
			}
		}

		// Add response body for error responses (4xx/5xx)
		if statusCode >= 400 && w.body.Len() > 0 {
			respBytes := w.body.Bytes()
			fields["response_body"] = string(respBytes)
		}

		// Log with structured fields including error details
		m.logger.WithFields(fields).Log(getLogLevel(statusCode), fmt.Sprintf("%s %s %d %v %s %d",
			method,
			path,
			statusCode,
			latency,
			clientIP,
			bodySize,
		))
	}
}

// getLogLevel returns the appropriate log level based on status code
func getLogLevel(statusCode int) logrus.Level {
	if statusCode >= http.StatusInternalServerError {
		return logrus.ErrorLevel
	} else if statusCode >= http.StatusBadRequest {
		return logrus.WarnLevel
	}
	return logrus.InfoLevel
}

// GetEntries returns all log entries from memory in chronological order
func (m *MultiModeMemoryLogMiddleware) GetEntries() []*logrus.Entry {
	if m.multiLogger == nil {
		return []*logrus.Entry{}
	}
	httpLogger := m.multiLogger.WithSource(obs.LogSourceHTTP)
	return httpLogger.GetMemoryEntries()
}

// GetLatestEntries returns the newest N log entries from memory
func (m *MultiModeMemoryLogMiddleware) GetLatestEntries(n int) []*logrus.Entry {
	if m.multiLogger == nil {
		return []*logrus.Entry{}
	}
	httpLogger := m.multiLogger.WithSource(obs.LogSourceHTTP)
	return httpLogger.GetMemoryLatest(n)
}

// GetEntriesSince returns log entries from memory after the specified time
func (m *MultiModeMemoryLogMiddleware) GetEntriesSince(since time.Time) []*logrus.Entry {
	// Get the HTTP scoped memory sink from MultiLogger
	memorySink := m.multiLogger.GetMemorySink(obs.LogSourceHTTP)
	if memorySink == nil {
		return []*logrus.Entry{}
	}
	return memorySink.GetEntriesSince(since)
}

// GetEntriesByLevel returns log entries from memory matching the specified level
func (m *MultiModeMemoryLogMiddleware) GetEntriesByLevel(level logrus.Level) []*logrus.Entry {
	// Get the HTTP scoped memory sink from MultiLogger
	memorySink := m.multiLogger.GetMemorySink(obs.LogSourceHTTP)
	if memorySink == nil {
		return []*logrus.Entry{}
	}
	return memorySink.GetEntriesByLevel(level)
}

// Clear removes all log entries from memory
func (m *MultiModeMemoryLogMiddleware) Clear() {
	if m.multiLogger == nil {
		return
	}
	httpLogger := m.multiLogger.WithSource(obs.LogSourceHTTP)
	httpLogger.ClearMemory()
}

// Size returns the current number of stored log entries in memory
func (m *MultiModeMemoryLogMiddleware) Size() int {
	// Get the HTTP scoped memory sink from MultiLogger and return its size
	memorySink := m.multiLogger.GetMemorySink(obs.LogSourceHTTP)
	if memorySink == nil {
		return 0
	}
	return memorySink.Size()
}

// GetRequestBodyStore returns the request body store for retrieving stored request bodies
func (m *MultiModeMemoryLogMiddleware) GetRequestBodyStore() *obs.RequestBodyStore {
	return m.requestBodyStore
}
