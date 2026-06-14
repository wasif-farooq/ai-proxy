package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"log/slog"

	"ai-proxy/internal/logger"
)

/* ─── JWT ────────────────────────────────────────────────── */

// AdminClaims represents the claims in an admin JWT token.
type AdminClaims struct {
	AdminID string `json:"aid"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Exp     int64  `json:"exp"`
	Iat     int64  `json:"iat"`
}

// jwtHeader is the fixed JWT header for HMAC-SHA256 tokens.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// generateToken creates a signed JWT token for an admin user.
func generateToken(secret string, claims *AdminClaims) (string, error) {
	claims.Iat = time.Now().Unix()
	claims.Exp = time.Now().Add(1 * time.Hour).Unix()

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)

	sig := signJWT(secret, jwtHeader, encodedPayload)
	return jwtHeader + "." + encodedPayload + "." + sig, nil
}

// verifyToken validates a JWT token and returns the claims.
func verifyToken(secret, token string) (*AdminClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Verify signature
	expectedSig := signJWT(secret, parts[0], parts[1])
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	// Decode payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var claims AdminClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	// Check expiration
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func signJWT(secret, header, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(header + "." + payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

/* ─── Login Response ─────────────────────────────────────── */

// LoginResponse is returned on successful admin authentication.
type LoginResponse struct {
	Token     string `json:"token"`
	AdminID   string `json:"admin_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	ExpiresAt int64  `json:"expires_at"`
}

/* ─── Middleware ─────────────────────────────────────────── */

// AuthMiddleware returns a Gin handler that validates the admin JWT token
// from the Authorization header (Bearer scheme). On success, the admin
// claims are stored in the Gin context for downstream handlers.
func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			abortWithAdminError(c, http.StatusUnauthorized, "Unauthorized", "Missing or invalid Authorization header")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := verifyToken(jwtSecret, token)
		if err != nil {
			logger.FromContext(c.Request.Context()).Warn("admin auth failed",
				slog.String("error", err.Error()),
			)
			abortWithAdminError(c, http.StatusUnauthorized, "Unauthorized", "Invalid or expired token")
			return
		}

		// Store claims in context
		c.Set("admin_id", claims.AdminID)
		c.Set("admin_email", claims.Email)
		c.Set("admin_role", claims.Role)

		c.Next()
	}
}

// AdminOnly returns a middleware that restricts access to super_admin and admin roles.
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("admin_role")
		if role != "super_admin" && role != "admin" {
			abortWithAdminError(c, http.StatusForbidden, "Forbidden", "Admin access required")
			return
		}
		c.Next()
	}
}

// SuperAdminOnly restricts access to the super_admin role.
func SuperAdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("admin_role")
		if role != "super_admin" {
			abortWithAdminError(c, http.StatusForbidden, "Forbidden", "Super admin access required")
			return
		}
		c.Next()
	}
}

// abortWithAdminError sends a JSON error response and aborts.
func abortWithAdminError(c *gin.Context, code int, message, detail string) {
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
