package security

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"log/slog"

	"ai-proxy/internal/logger"
)

// NonceMaxAge is the default time window for accepting timestamped requests.
const NonceMaxAge = 5 * time.Minute

/* ─── Error helpers (inline to avoid import cycle with internal/shared) ── */

type errorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func abortWithError(c *gin.Context, code int, message, detail string) {
	c.JSON(code, gin.H{
		"success": false,
		"error":   errorBody{Code: code, Message: message, Detail: detail},
	})
	c.Abort()
}

/* ─── NonceMiddleware ───────────────────────────────────── */

// NonceMiddleware returns a Gin handler that validates X-Timestamp and X-Nonce
// headers for replay protection. X-Client-ID must be present (set by auth).
//
// Header requirements:
//   - X-Client-ID:  Client identifier
//   - X-Nonce:      Unique per-request nonce string (checked first)
//   - X-Timestamp:  Unix epoch seconds (int64)
func NonceMiddleware(store NonceStore, maxAge time.Duration) gin.HandlerFunc {
	if maxAge <= 0 {
		maxAge = NonceMaxAge
	}

	return func(c *gin.Context) {
		clientID := c.GetHeader("X-Client-ID")
		nonce := c.GetHeader("X-Nonce")
		timestamp := c.GetHeader("X-Timestamp")

		if clientID == "" {
			abortWithError(c, http.StatusUnauthorized, "Unauthorized", "Missing X-Client-ID header")
			return
		}

		// Validate nonce presence before timestamp (cheaper check first)
		if nonce == "" {
			logger.FromContext(c.Request.Context()).Warn("missing nonce",
				slog.String("client_id", clientID),
			)
			abortWithError(c, http.StatusUnauthorized, "Invalid or missing nonce", "X-Nonce header is required")
			return
		}

		// Validate timestamp
		if err := ValidateTimestamp(timestamp, maxAge); err != nil {
			logger.FromContext(c.Request.Context()).Warn("timestamp validation failed",
				slog.String("client_id", clientID),
				slog.String("error", err.Error()),
			)
			abortWithError(c, http.StatusBadRequest, "Invalid or expired timestamp", err.Error())
			return
		}

		// Check nonce uniqueness
		if !store.IsUnique(clientID, nonce) {
			logger.FromContext(c.Request.Context()).Warn("nonce replay detected",
				slog.String("client_id", clientID),
				slog.String("nonce", nonce),
			)
			abortWithError(c, http.StatusUnauthorized, "Nonce already used", "")
			return
		}

		c.Next()
	}
}

/* ─── RateLimitMiddleware ────────────────────────────────── */

// RateLimitMiddleware returns a Gin handler that enforces per-client rate limits.
// It reads the client identifier from the X-Client-ID header and checks against
// the provided RateLimiter. Sets X-RateLimit-Remaining and Retry-After headers.
func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := c.GetHeader("X-Client-ID")

		if clientID == "" {
			c.Next()
			return
		}

		if !rl.Allow(clientID) {
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", strconv.Itoa(int(rl.refillInterval.Seconds())))

			logger.FromContext(c.Request.Context()).Warn("rate limit exceeded",
				slog.String("client_id", clientID),
			)
			abortWithError(c, http.StatusTooManyRequests, "Rate limit exceeded",
				fmt.Sprintf("retry after %d seconds", int(rl.refillInterval.Seconds())))
			return
		}

		c.Header("X-RateLimit-Remaining", strconv.Itoa(rl.Remaining(clientID)))
		c.Next()
	}
}
