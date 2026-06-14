package shared

import (
	"github.com/gin-gonic/gin"
	"ai-proxy/internal/config"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/security"
)

// NewRouter creates a new Gin engine with the standard middleware chain
// applied: recovery, logger, CORS, and security headers.
func NewRouter(cfg *config.Config) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()

	// Global middleware chain
	r.Use(gin.Recovery())
	r.Use(logger.Middleware())
	r.Use(CORSMiddleware(cfg))
	r.Use(security.Middleware())

	// Health check (no auth required)
	r.GET("/health", HealthHandler)

	return r
}

// HealthHandler returns a simple health check response.
func HealthHandler(c *gin.Context) {
	SendOK(c, gin.H{
		"status":  "ok",
		"service": "ai-proxy",
	})
}

// CORSMiddleware handles Cross-Origin Resource Sharing.
func CORSMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		for _, allowed := range cfg.AllowedOrigins {
			if origin == allowed {
				c.Header("Access-Control-Allow-Origin", origin)
				break
			}
		}
		if origin == "" && !cfg.IsProduction() {
			c.Header("Access-Control-Allow-Origin", "*")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID, X-Timestamp, X-Nonce, X-Client-ID")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}


