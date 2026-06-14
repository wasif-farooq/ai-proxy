//go:build stress

package stress

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"

	"ai-proxy/internal/client"
	"ai-proxy/internal/client/encryption"
	"ai-proxy/internal/config"
	"ai-proxy/internal/logger"
	"ai-proxy/internal/provider"
	"ai-proxy/internal/security"
	"ai-proxy/internal/shared"
)

// ═══════════════════════════════════════════════════════════════
// AuthMethod — which authentication path to test
// ═══════════════════════════════════════════════════════════════

type AuthMethod int

const (
	AuthBearer AuthMethod = iota
	AuthXAuth
)

func (a AuthMethod) String() string {
	switch a {
	case AuthBearer:
		return "Bearer"
	case AuthXAuth:
		return "X-Auth (encrypted)"
	default:
		return "unknown"
	}
}

// ═══════════════════════════════════════════════════════════════
// BodyBuilder — function that produces a request body
// ═══════════════════════════════════════════════════════════════

// BodyBuilder generates a request body. The stream parameter controls
// whether the body requests streaming (true) or not (false). The id
// parameter is used for unique identifiers like file_ids or nonces
// embedded in the body.
type BodyBuilder func(stream bool, id string) string

// SimpleBody returns a minimal chat completion body.
func SimpleBody(stream bool, id string) string {
	streamStr := "false"
	if stream {
		streamStr = "true"
	}
	return fmt.Sprintf(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}],"stream":%s}`, streamStr)
}

// FileBodyBase64 is a synthetic base64 string simulating file content.
var FileBodyBase64 string

func init() {
	var buf bytes.Buffer
	for i := 0; i < 200; i++ {
		buf.WriteString("This is sample file content for stress testing the AI Proxy. ")
	}
	FileBodyBase64 = base64.RawStdEncoding.EncodeToString(buf.Bytes())
}

// FileBody returns a chat completion body with vision-style file references.
func FileBody(stream bool, fileID string) string {
	streamStr := "false"
	if stream {
		streamStr = "true"
	}
	return fmt.Sprintf(`{
  "model": "gpt-4o",
  "messages": [
    {
      "role": "user",
      "content": [
        {"type": "text", "text": "Analyze this file: %s"},
        {"type": "image_url", "image_url": {"url": "data:image/png;base64,%s"}}
      ]
    }
  ],
  "stream": %s,
  "file_ids": ["%s"]
}`, fileID, FileBodyBase64, streamStr, fileID)
}

// ═══════════════════════════════════════════════════════════════
// X-Auth header builder
// ═══════════════════════════════════════════════════════════════

// BuildXAuthHeader encrypts "client_id:timestamp:nonce" with AES-GCM.
func BuildXAuthHeader(clientID, encryptionKey, nonce string, timestamp int64) string {
	key, err := base64.RawURLEncoding.DecodeString(encryptionKey)
	if err != nil {
		return ""
	}
	payload := fmt.Sprintf("%s:%d:%s", clientID, timestamp, nonce)
	encrypted, err := encryption.Encrypt(key, []byte(payload))
	if err != nil {
		return ""
	}
	return encrypted
}

// ═══════════════════════════════════════════════════════════════
// Load-test levels
// ═══════════════════════════════════════════════════════════════

type stressLevel struct {
	Concurrency int
	PerWorker   int
	Label       string
}

// StressLevels defines concurrency levels for non-streaming tests.
var StressLevels = []stressLevel{
	{Concurrency: 1, PerWorker: 50, Label: "Sequential (baseline)"},
	{Concurrency: 5, PerWorker: 50, Label: "Light load"},
	{Concurrency: 10, PerWorker: 50, Label: "Moderate load"},
	{Concurrency: 25, PerWorker: 40, Label: "High load"},
	{Concurrency: 50, PerWorker: 20, Label: "Heavy load"},
	{Concurrency: 100, PerWorker: 10, Label: "Burst load"},
}

// StreamingLevels defines concurrency levels for streaming tests.
var StreamingLevels = []stressLevel{
	{Concurrency: 1, PerWorker: 20, Label: "Sequential (baseline)"},
	{Concurrency: 5, PerWorker: 20, Label: "Light load"},
	{Concurrency: 10, PerWorker: 15, Label: "Moderate load"},
	{Concurrency: 25, PerWorker: 10, Label: "High load"},
	{Concurrency: 50, PerWorker: 6, Label: "Heavy load"},
}

// ═══════════════════════════════════════════════════════════════
// TestHarness — shared test setup
// ═══════════════════════════════════════════════════════════════

// MiddlewareChain describes which Gin middleware to use for authentication.
type MiddlewareChain int

const (
	// ChainLegacy uses AuthMiddleware + NonceMiddleware (original Bearer-only path).
	ChainLegacy MiddlewareChain = iota
	// ChainDual uses DualAuthMiddleware (handles both Bearer and X-Auth).
	ChainDual
)

// TestHarness encapsulates all dependencies for a stress test.
type TestHarness struct {
	T              testingT
	ProxyServer    *httptest.Server
	MockUpstream   *httptest.Server
	ClientSvc      *client.Service
	ProviderReg    *provider.Registry
	ProviderKeySvc *client.ProviderKeyService
	Proxy          *provider.Proxy
	NonceStore     security.NonceStore
	RateLimiter    *security.RateLimiter
	Chain          MiddlewareChain

	upstreamCfg    UpstreamConfig
	sseUpstreamCfg SSEUpstreamConfig
}

type testingT interface {
	Log(args ...interface{})
	Logf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	Helper()
}

// NewHarness creates a TestHarness with a mock upstream and proxy server.
// Use Upstream/SSEUpstream to configure the mock behavior before calling Setup.
func NewHarness() *TestHarness {
	return &TestHarness{
		Chain: ChainDual,
	}
}

// Setup initializes all dependencies and starts the servers.
func (h *TestHarness) Setup() {
	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// Start mock upstream
	if h.sseUpstreamCfg.ChunkCount > 0 {
		handler := &mockSSEUpstreamHandler{SSEUpstreamConfig: h.sseUpstreamCfg}
		h.MockUpstream = httptest.NewServer(handler)
	} else {
		cfg := h.upstreamCfg
		if cfg.MinLatency == 0 {
			cfg = DefaultUpstreamConfig()
		}
		handler := &mockUpstreamHandler{UpstreamConfig: cfg}
		h.MockUpstream = httptest.NewServer(handler)
	}

	// Dependencies
	clientRepo := &mockClientRepo{}
	h.ClientSvc = client.NewService(clientRepo, testMasterKey)

	providerRepo := &mockProviderRepo{upstreamURL: h.MockUpstream.URL}
	h.ProviderReg = provider.NewRegistry(providerRepo)
	if err := h.ProviderReg.Refresh(context.Background()); err != nil {
		panic("refresh registry: " + err.Error())
	}

	h.ProviderKeySvc = client.NewProviderKeyService(&mockKeyRepo{}, h.ClientSvc, testMasterKey)
	h.Proxy = provider.NewProxy(h.ProviderReg, h.ProviderKeySvc, 30*time.Second)

	h.NonceStore = security.NewInMemoryNonceStore(5 * time.Minute)
	h.RateLimiter = security.NewRateLimiter(100000, 10000)

	// Gin router
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")

	switch h.Chain {
	case ChainLegacy:
		api.POST("/chat/completions",
			provider.AuthMiddleware(h.ClientSvc),
			security.NonceMiddleware(h.NonceStore, 5*time.Minute),
			security.RateLimitMiddleware(h.RateLimiter),
			provider.RouteMiddleware(h.Proxy),
		)
	case ChainDual:
		api.POST("/chat/completions",
			provider.DualAuthMiddleware(h.ClientSvc, h.NonceStore, 5*time.Minute),
			security.RateLimitMiddleware(h.RateLimiter),
			provider.RouteMiddleware(h.Proxy),
		)
	}

	h.ProxyServer = httptest.NewServer(router)
}

// Close shuts down all servers.
func (h *TestHarness) Close() {
	if h.ProxyServer != nil {
		h.ProxyServer.Close()
	}
	if h.MockUpstream != nil {
		h.MockUpstream.Close()
	}
	if h.RateLimiter != nil {
		h.RateLimiter.Stop()
	}
}

// WithUpstream sets non-streaming upstream config and returns h for chaining.
func (h *TestHarness) WithUpstream(cfg UpstreamConfig) *TestHarness {
	h.upstreamCfg = cfg
	return h
}

// WithSSEUpstream sets streaming upstream config and returns h for chaining.
func (h *TestHarness) WithSSEUpstream(cfg SSEUpstreamConfig) *TestHarness {
	h.sseUpstreamCfg = cfg
	return h
}

// WithLegacyChain uses the old Auth+Nonce middleware (Bearer only).
func (h *TestHarness) WithLegacyChain() *TestHarness {
	h.Chain = ChainLegacy
	return h
}

// WithDualChain uses DualAuthMiddleware (Bearer + X-Auth).
func (h *TestHarness) WithDualChain() *TestHarness {
	h.Chain = ChainDual
	return h
}
