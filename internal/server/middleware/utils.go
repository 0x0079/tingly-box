package middleware

import (
	"bytes"

	"github.com/gin-gonic/gin"
)

// maxBufferedResponseBytes bounds the in-memory capture of non-streaming
// response bodies. Streaming responses (SSE) are captured separately via
// StreamEventStore at the event-recording layer, so this limit only
// protects against pathological non-streaming payloads.
const maxBufferedResponseBytes = 256 * 1024

// responseBodyWriter wraps gin.ResponseWriter and captures bytes written
// up to a fixed limit so the middleware can attach the body to log
// entries for error responses without risking unbounded memory growth.
type responseBodyWriter struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	limit   int
	dropped int // bytes that were not captured because the limit was hit
}

func (r *responseBodyWriter) Write(b []byte) (int, error) {
	if r.limit <= 0 || r.body.Len() < r.limit {
		remaining := r.limit - r.body.Len()
		if remaining > 0 && remaining < len(b) {
			r.body.Write(b[:remaining])
			r.dropped += len(b) - remaining
		} else {
			r.body.Write(b)
		}
	} else {
		r.dropped += len(b)
	}
	return r.ResponseWriter.Write(b)
}
