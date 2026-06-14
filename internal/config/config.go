package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the application.
type Config struct {
	// Environment
	Env         string // development, staging, production
	ServiceName string

	// Server
	ServerHost string
	ServerPort int
	ServerReadTimeout  time.Duration
	ServerWriteTimeout time.Duration

	// Database
	DatabaseURL          string
	DatabaseMaxConns     int
	DatabaseMinConns     int
	DatabaseMaxIdleTime  time.Duration

	// Logging
	LogLevel  string // debug, info, warn, error
	LogFormat string // json, text

	// Auth
	JWTSecret           string
	JWTExpiration       time.Duration
	RefreshTokenExpiry  time.Duration

	// CORS
	AllowedOrigins []string

	// Rate Limiting
	RateLimitRequestsPerMin int
	RateLimitBurst          int

	// Redis (for nonce store & rate limiting)
	RedisURL string

	// Encryption
	EncryptionKey string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	cfg := &Config{
		Env:         getEnv("ENV", "development"),
		ServiceName: getEnv("SERVICE_NAME", "ai-proxy"),

		ServerHost:        getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort:        getEnvInt("SERVER_PORT", 8080),
		ServerReadTimeout:  getEnvDur("SERVER_READ_TIMEOUT", 30*time.Second),
		ServerWriteTimeout: getEnvDur("SERVER_WRITE_TIMEOUT", 30*time.Second),

		DatabaseURL:          getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/ai_proxy?sslmode=disable"),
		DatabaseMaxConns:     getEnvInt("DATABASE_MAX_CONNS", 25),
		DatabaseMinConns:     getEnvInt("DATABASE_MIN_CONNS", 5),
		DatabaseMaxIdleTime:  getEnvDur("DATABASE_MAX_IDLE_TIME", 5*time.Minute),

		LogLevel:  getEnv("LOG_LEVEL", "info"),
		LogFormat: getEnv("LOG_FORMAT", "text"),

		JWTSecret:          getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpiration:      getEnvDur("JWT_EXPIRATION", 1*time.Hour),
		RefreshTokenExpiry: getEnvDur("REFRESH_TOKEN_EXPIRY", 30*24*time.Hour),

		AllowedOrigins: getEnvSlice("ALLOWED_ORIGINS", []string{"http://localhost:5173"}),

		RateLimitRequestsPerMin: getEnvInt("RATE_LIMIT_REQUESTS_PER_MIN", 60),
		RateLimitBurst:          getEnvInt("RATE_LIMIT_BURST", 10),

		RedisURL: getEnv("REDIS_URL", ""),

		EncryptionKey: getEnv("ENCRYPTION_KEY", ""),
	}
	return cfg
}

// Addr returns the address string for the server to listen on.
func (c *Config) Addr() string {
	return c.ServerHost + ":" + strconv.Itoa(c.ServerPort)
}

// IsProduction returns true if the environment is production.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// IsDevelopment returns true if the environment is development.
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

/* ─── Helpers ───────────────────────────────────────────── */

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvDur(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func getEnvSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		return splitAndTrim(v, ",")
	}
	return fallback
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
