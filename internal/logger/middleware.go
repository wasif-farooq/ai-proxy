package logger

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"log/slog"
)

// Middleware returns a Gin handler that:
//  1. Generates a request ID
//  2. Attaches a request-scoped logger to the context
//  3. Logs the request on completion with status, latency, and method
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = newRequestID()
		}

		// Attach logger with request context to the Gin context
		l := Default().With(
			slog.String(KeyRequestID, requestID),
			slog.String(KeyMethod, c.Request.Method),
			slog.String(KeyEndpoint, c.Request.URL.Path),
			slog.String(KeyIPAddress, c.ClientIP()),
			slog.String(KeyUserAgent, c.Request.UserAgent()),
		)
		c.Set(KeyRequestID, requestID)
		c.Request = c.Request.WithContext(WithContext(c.Request.Context(), KeyRequestID, requestID))

		// Process request
		c.Next()

		// Log after response
		latency := time.Since(start)
		status := c.Writer.Status()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		l.LogAttrs(c.Request.Context(), level, "request completed",
			slog.Int(KeyStatusCode, status),
			slog.String(KeyDuration, latency.String()),
		)
	}
}

// newRequestID generates a 16-byte hex request ID.
func newRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp if crypto/rand fails (extremely rare)
		v := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(v >> (uint(i) * 8))
		}
	}
	return hex.EncodeToString(b)
}
