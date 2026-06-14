package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"log/slog"

	"ai-proxy/internal/client"
	"ai-proxy/internal/client/encryption"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/security"
	"ai-proxy/internal/shared"
)

/* ─── Auth Middleware ────────────────────────────────────── */

// AuthMiddleware returns a Gin handler that authenticates a client using
// the X-Client-ID and Authorization headers. The validated client is stored
// in the Gin context for downstream handlers.
//
// Header requirements:
//   - X-Client-ID:  The client identifier
//   - Authorization: Bearer <client_secret>
func AuthMiddleware(clientService *client.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := c.GetHeader("X-Client-ID")
		authHeader := c.GetHeader("Authorization")

		if clientID == "" {
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Missing X-Client-ID header")
			return
		}

		// Extract bearer token
		secret := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			secret = strings.TrimPrefix(authHeader, "Bearer ")
		}
		if secret == "" {
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Missing or invalid Authorization header")
			return
		}

		// Look up client
		cl, err := clientService.GetByClientID(c.Request.Context(), clientID)
		if err != nil {
			// ErrNotFound means the client doesn't exist — treat as auth failure
			if errors.Is(err, shared.ErrNotFound) {
				logger.FromContext(c.Request.Context()).Warn("client not found",
					slog.String("client_id", clientID),
				)
				abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client credentials")
				return
			}
			logger.FromContext(c.Request.Context()).Error("client lookup failed",
				slog.String("client_id", clientID),
				slog.String("error", err.Error()),
			)
			abortWithProxyError(c, http.StatusInternalServerError, "Internal server error", "")
			return
		}
		if cl == nil {
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client credentials")
			return
		}

		// Check client status
		if cl.Status != client.ClientStatusActive {
			logger.FromContext(c.Request.Context()).Warn("client not active",
				slog.String("client_id", clientID),
				slog.String("status", string(cl.Status)),
			)
			if cl.Status == client.ClientStatusSuspended {
				abortWithProxyError(c, http.StatusForbidden, "Client is suspended", "This client has been suspended")
			} else {
				abortWithProxyError(c, http.StatusForbidden, "Client is revoked", "This client has been revoked")
			}
			return
		}

		// Validate secret
		if !clientService.ValidateClientSecret(cl, secret) {
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client secret")
			return
		}

		// Store client in context
		c.Set("client", cl)
		c.Set("client_id", cl.ClientID)
		c.Set("preferred_providers", cl.PreferredProviders)

		// Store client and client_id in request context so the proxy can access them
		ctx := context.WithValue(c.Request.Context(), clientIDKey{}, cl.ClientID)
		ctx = context.WithValue(ctx, clientKey{}, cl)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

/* ─── DualAuthMiddleware ──────────────────────────────────── */

// DualAuthMiddleware supports two authentication methods:
//
// 1. Bearer token (legacy):
//    - X-Client-ID + Authorization: Bearer <client_secret>
//    - X-Nonce + X-Timestamp headers for replay protection
//
// 2. Encrypted X-Auth (recommended for public API):
//    - X-Client-ID + X-Auth: <AES-GCM encrypted payload>
//    - Encrypted payload contains "client_id:timestamp:nonce"
//    - Decrypted server-side using client's encryption_key
//
// After successful auth, the validated client is stored in the Gin context
// for downstream handlers. Nonce/timestamp validation is handled internally
// for both paths.
func DualAuthMiddleware(clientService *client.Service, nonceStore security.NonceStore, maxAge time.Duration) gin.HandlerFunc {
	if maxAge <= 0 {
		maxAge = 5 * time.Minute
	}

	return func(c *gin.Context) {
		clientID := c.GetHeader("X-Client-ID")
		authHeader := c.GetHeader("Authorization")
		xAuth := c.GetHeader("X-Auth")

		if clientID == "" {
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Missing X-Client-ID header")
			return
		}

		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			// ── Bearer token path ──
			handleBearerAuth(c, clientService, nonceStore, maxAge, clientID, authHeader)
		} else if xAuth != "" {
			// ── Encrypted X-Auth path ──
			handleEncryptedAuth(c, clientService, nonceStore, maxAge, clientID, xAuth)
		} else {
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized",
				"Missing authentication. Provide Authorization: Bearer or X-Auth header.")
		}
	}
}

// handleBearerAuth authenticates using a Bearer token (client secret).
func handleBearerAuth(c *gin.Context, clientService *client.Service, nonceStore security.NonceStore, maxAge time.Duration, clientID, authHeader string) {
	secret := strings.TrimPrefix(authHeader, "Bearer ")
	if secret == "" {
		abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Missing or invalid Authorization header")
		return
	}

	cl, err := clientService.GetByClientID(c.Request.Context(), clientID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			logger.FromContext(c.Request.Context()).Warn("client not found", slog.String("client_id", clientID))
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client credentials")
			return
		}
		logger.FromContext(c.Request.Context()).Error("client lookup failed",
			slog.String("client_id", clientID),
			slog.String("error", err.Error()),
		)
		abortWithProxyError(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}
	if cl == nil {
		abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client credentials")
		return
	}

	// Check client status
	if cl.Status != client.ClientStatusActive {
		rejectNonActive(c, cl.Status)
		return
	}

	// Validate secret
	if !clientService.ValidateClientSecret(cl, secret) {
		abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client secret")
		return
	}

	// Validate nonce and timestamp headers
	nonce := c.GetHeader("X-Nonce")
	timestamp := c.GetHeader("X-Timestamp")
	if nonce == "" {
		abortWithProxyError(c, http.StatusUnauthorized, "Invalid or missing nonce", "X-Nonce header is required")
		return
	}
	if err := security.ValidateTimestamp(timestamp, maxAge); err != nil {
		logger.FromContext(c.Request.Context()).Warn("timestamp validation failed",
			slog.String("client_id", clientID),
			slog.String("error", err.Error()),
		)
		abortWithProxyError(c, http.StatusBadRequest, "Invalid or expired timestamp", err.Error())
		return
	}
	if !nonceStore.IsUnique(clientID, nonce) {
		logger.FromContext(c.Request.Context()).Warn("nonce replay detected",
			slog.String("client_id", clientID),
			slog.String("nonce", nonce),
		)
		abortWithProxyError(c, http.StatusUnauthorized, "Nonce already used", "")
		return
	}

	setClientInContext(c, cl)
}

// handleEncryptedAuth authenticates by decrypting the X-Auth header which
// contains "client_id:timestamp:nonce" encrypted with AES-GCM using the
// client's encryption_key. This is the recommended auth method for public APIs.
func handleEncryptedAuth(c *gin.Context, clientService *client.Service, nonceStore security.NonceStore, maxAge time.Duration, clientID, xAuth string) {
	cl, err := clientService.GetByClientID(c.Request.Context(), clientID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			logger.FromContext(c.Request.Context()).Warn("client not found", slog.String("client_id", clientID))
			abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client credentials")
			return
		}
		logger.FromContext(c.Request.Context()).Error("client lookup failed",
			slog.String("client_id", clientID),
			slog.String("error", err.Error()),
		)
		abortWithProxyError(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}
	if cl == nil {
		abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid client credentials")
		return
	}

	// Check client status
	if cl.Status != client.ClientStatusActive {
		rejectNonActive(c, cl.Status)
		return
	}

	// Decode the client's encryption key (base64 → 32-byte AES key)
	key, err := base64.RawURLEncoding.DecodeString(cl.EncryptionKey)
	if err != nil {
		logger.FromContext(c.Request.Context()).Error("failed to decode client encryption key",
			slog.String("client_id", clientID),
			slog.String("error", err.Error()),
		)
		abortWithProxyError(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	// Decrypt X-Auth payload
	plaintext, err := encryption.Decrypt(key, xAuth)
	if err != nil {
		logger.FromContext(c.Request.Context()).Warn("failed to decrypt X-Auth",
			slog.String("client_id", clientID),
			slog.String("error", err.Error()),
		)
		abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid X-Auth payload")
		return
	}

	// Parse "client_id:timestamp:nonce"
	parts := strings.SplitN(string(plaintext), ":", 3)
	if len(parts) != 3 {
		logger.FromContext(c.Request.Context()).Warn("malformed X-Auth payload",
			slog.String("client_id", clientID),
		)
		abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Invalid X-Auth payload format")
		return
	}

	authClientID, timestamp, nonce := parts[0], parts[1], parts[2]

	// Verify client_id in payload matches header
	if authClientID != clientID {
		logger.FromContext(c.Request.Context()).Warn("X-Auth client_id mismatch",
			slog.String("header_client_id", clientID),
			slog.String("payload_client_id", authClientID),
		)
		abortWithProxyError(c, http.StatusUnauthorized, "Unauthorized", "Client ID mismatch in encrypted auth")
		return
	}

	// Validate timestamp
	if err := security.ValidateTimestamp(timestamp, maxAge); err != nil {
		logger.FromContext(c.Request.Context()).Warn("timestamp validation failed",
			slog.String("client_id", clientID),
			slog.String("error", err.Error()),
		)
		abortWithProxyError(c, http.StatusBadRequest, "Invalid or expired timestamp", err.Error())
		return
	}

	// Validate nonce uniqueness
	if !nonceStore.IsUnique(clientID, nonce) {
		logger.FromContext(c.Request.Context()).Warn("nonce replay detected",
			slog.String("client_id", clientID),
			slog.String("nonce", nonce),
		)
		abortWithProxyError(c, http.StatusUnauthorized, "Nonce already used", "")
		return
	}

	setClientInContext(c, cl)
}

// setClientInContext stores the validated client in both Gin context and
// request context for downstream handlers.
func setClientInContext(c *gin.Context, cl *client.Client) {
	c.Set("client", cl)
	c.Set("client_id", cl.ClientID)
	c.Set("preferred_providers", cl.PreferredProviders)

	ctx := context.WithValue(c.Request.Context(), clientIDKey{}, cl.ClientID)
	ctx = context.WithValue(ctx, clientKey{}, cl)
	c.Request = c.Request.WithContext(ctx)

	c.Next()
}

// rejectNonActive aborts the request with an appropriate error based on the
// client's non-active status.
func rejectNonActive(c *gin.Context, status client.ClientStatus) {
	if status == client.ClientStatusSuspended {
		abortWithProxyError(c, http.StatusForbidden, "Client is suspended", "This client has been suspended")
	} else {
		abortWithProxyError(c, http.StatusForbidden, "Client is revoked", "This client has been revoked")
	}
}

/* ─── Provider Routing Middleware ────────────────────────── */

// RouteMiddleware returns a Gin handler that routes the request to the
// appropriate upstream AI provider. It supports both regular and streaming
// responses based on the request body's `stream` field.
func RouteMiddleware(proxy *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Read request body (Gin caches it internally)
		body, err := c.GetRawData()
		if err != nil {
			abortWithProxyError(c, http.StatusBadRequest, "Bad request", "Failed to read request body")
			return
		}

	// Get preferred providers from context (set by AuthMiddleware)
	preferredProviders, _ := c.Get("preferred_providers")
	prefs, _ := preferredProviders.([]client.ClientPreferredRoute)

	// Determine target model, provider, and stream mode
	model, providerID, isStream, err := resolveModelAndProvider(body, prefs)
		if err != nil {
			abortWithProxyError(c, http.StatusBadRequest, "Bad request", err.Error())
			return
		}

		start := time.Now()

		if isStream {
			// Streaming response
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")

			if err := proxy.ForwardStreaming(c.Request.Context(), c.Writer, providerID, model, body); err != nil {
				logger.FromContext(c.Request.Context()).Error("streaming proxy error",
					slog.String("error", err.Error()),
				)
				// Can't write error headers after streaming started
				return
			}
		} else {
			// Regular JSON response
			result, err := proxy.Forward(c.Request.Context(), providerID, model, body)
			if err != nil {
				logger.FromContext(c.Request.Context()).Error("proxy error",
					slog.String("error", err.Error()),
				)
				abortWithProxyError(c, http.StatusBadGateway, "Bad gateway", err.Error())
				return
			}

			// Copy response headers
			for k, v := range result.Headers {
				c.Header(k, v)
			}

			c.Data(result.StatusCode, "application/json", result.Body)
		}

		latencyMs := time.Since(start).Milliseconds()
		logger.FromContext(c.Request.Context()).Debug("proxy request completed",
			slog.String("model", model),
			slog.Int64("latency_ms", latencyMs),
		)
	}
}

/* ─── Helpers ────────────────────────────────────────────── */

// resolveModelAndProvider extracts model and stream flags from the request body
// and determines which provider should handle it.
func resolveModelAndProvider(body []byte, preferredProviders []client.ClientPreferredRoute) (string, ProviderID, bool, error) {
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", "", false, fmt.Errorf("invalid request body: %w", err)
	}

	if req.Model == "" {
		return "", "", false, fmt.Errorf("model is required")
	}

	// Check preferred routes: first matching model wins
	providerID := ProviderID("")
	for _, route := range preferredProviders {
		if route.Model == req.Model && IsValidProviderID(route.Provider) {
			providerID = ProviderID(route.Provider)
			break
		}
	}

	if providerID == "" {
		// Fallback: map model prefix to provider
		providerID = modelToProvider(req.Model)
	}

	return req.Model, providerID, req.Stream, nil
}

// modelToProvider maps common model name patterns to provider IDs.
func modelToProvider(model string) ProviderID {
	model = strings.ToLower(model)
	switch {
	case strings.HasPrefix(model, "gpt-"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"), strings.HasPrefix(model, "dall-e"):
		return ProviderOpenAI
	case strings.HasPrefix(model, "claude-"):
		return ProviderAnthropic
	case strings.HasPrefix(model, "gemini-"):
		return ProviderGoogle
	case strings.HasPrefix(model, "llama"), strings.HasPrefix(model, "mistral"), strings.HasPrefix(model, "mixtral"), strings.HasPrefix(model, "codellama"), strings.HasPrefix(model, "phi-"), strings.HasPrefix(model, "qwen"), strings.HasPrefix(model, "deepseek-coder"), strings.HasPrefix(model, "nemotron"), strings.HasPrefix(model, "command"):
		return ProviderOllama
	default:
		return ProviderOpenAI // default fallback
	}
}

// abortWithProxyError sends a JSON error response and aborts.
func abortWithProxyError(c *gin.Context, code int, message, detail string) {
	c.JSON(code, gin.H{
		"success": false,
		"error": gin.H{
			"code":    code,
			"message": message,
			"detail":  detail,
		},
	})
	c.Abort()
}
