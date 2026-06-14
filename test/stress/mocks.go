//go:build stress

package stress

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"ai-proxy/internal/client"
	"ai-proxy/internal/client/encryption"
	"ai-proxy/internal/provider"
)

// ═══════════════════════════════════════════════════════════════
// Test constants & fixtures
// ═══════════════════════════════════════════════════════════════

const (
	testClientID     = "test-client-stress"
	testClientSecret = "sk-stress-test-secret-1234567890"
	testMasterKey    = "stress-test-master-key"
)

var (
	testClientHash       = encryption.HashClientSecret(testClientSecret)
	testEncryptionKey    string
	testEncryptionSecret string
)

func init() {
	var err error
	testEncryptionKey, err = encryption.GenerateSecret()
	if err != nil {
		panic("failed to generate test encryption key: " + err.Error())
	}
	testEncryptionSecret, err = encryption.GenerateSecret()
	if err != nil {
		panic("failed to generate test encryption secret: " + err.Error())
	}
}

// testClient returns a Client fixture with encrypted-at-rest keys.
func testClient() *client.Client {
	now := time.Now()
	encKeyEnc, _ := encryption.EncryptClientKey(testMasterKey, testEncryptionKey)
	encSecretEnc, _ := encryption.EncryptClientKey(testMasterKey, testEncryptionSecret)

	return &client.Client{
		ID:               "mock-client-uuid",
		ClientID:         testClientID,
		ClientSecretHash: testClientHash,
		Name:             "Stress Test Client",
		Status:           client.ClientStatusActive,
		EncryptionKey:    encKeyEnc,
		EncryptionSecret: encSecretEnc,
		PreferredProviders: []client.ClientPreferredRoute{
			{Provider: "openai", Model: "gpt-4o"},
		},
		CreatedAt:     now,
		UpdatedAt:     now,
		LastRotatedAt: &now,
	}
}

// ═══════════════════════════════════════════════════════════════
// Mock: Client Repository (returns a single test client)
// ═══════════════════════════════════════════════════════════════

type mockClientRepo struct{}

func (m *mockClientRepo) Create(ctx context.Context, input client.CreateClientInput) (*client.Client, error) {
	return testClient(), nil
}
func (m *mockClientRepo) GetByID(ctx context.Context, id string) (*client.Client, error) {
	return testClient(), nil
}
func (m *mockClientRepo) GetByClientID(ctx context.Context, clientID string) (*client.Client, error) {
	return testClient(), nil
}
func (m *mockClientRepo) List(ctx context.Context, filter client.ClientFilter) (*client.ClientList, error) {
	return &client.ClientList{Clients: []client.Client{*testClient()}, Total: 1}, nil
}
func (m *mockClientRepo) Update(ctx context.Context, id string, input client.UpdateClientInput) (*client.Client, error) {
	return testClient(), nil
}
func (m *mockClientRepo) UpdateStatus(ctx context.Context, id string, status client.ClientStatus) (*client.Client, error) {
	return testClient(), nil
}
func (m *mockClientRepo) RotateKeys(ctx context.Context, id string, secretHash, encKey, encSecret string) (*client.Client, error) {
	return testClient(), nil
}
func (m *mockClientRepo) Delete(ctx context.Context, id string) error {
	return nil
}

// ═══════════════════════════════════════════════════════════════
// Mock: Provider Repository (returns a single test provider)
// ═══════════════════════════════════════════════════════════════

type mockProviderRepo struct {
	upstreamURL string
}

func (m *mockProviderRepo) Create(ctx context.Context, input provider.CreateProviderInput) (*provider.Provider, error) {
	return m.testProvider(), nil
}
func (m *mockProviderRepo) GetByID(ctx context.Context, id string) (*provider.Provider, error) {
	return m.testProvider(), nil
}
func (m *mockProviderRepo) GetByProviderID(ctx context.Context, providerID provider.ProviderID) (*provider.Provider, error) {
	return m.testProvider(), nil
}
func (m *mockProviderRepo) List(ctx context.Context, enabledOnly bool) ([]provider.Provider, error) {
	return []provider.Provider{*m.testProvider()}, nil
}
func (m *mockProviderRepo) Update(ctx context.Context, id string, input provider.UpdateProviderInput) (*provider.Provider, error) {
	return m.testProvider(), nil
}
func (m *mockProviderRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockProviderRepo) testProvider() *provider.Provider {
	return &provider.Provider{
		ID:         "mock-provider-uuid",
		ProviderID: provider.ProviderOpenAI,
		Name:       "OpenAI (Mock)",
		APIKey:     "sk-mock-key-for-stress-testing",
		BaseURL:    m.upstreamURL,
		Enabled:    true,
		Models:     []string{"gpt-4", "gpt-4o", "gpt-3.5-turbo"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// ═══════════════════════════════════════════════════════════════
// Mock: ClientProviderKeyRepository (always returns "not found")
// ═══════════════════════════════════════════════════════════════

type mockKeyRepo struct{}

func (m *mockKeyRepo) Set(ctx context.Context, input client.SetClientProviderKeyInput, encryptedKey string) (*client.ClientProviderKey, error) {
	return nil, nil
}
func (m *mockKeyRepo) Get(ctx context.Context, clientID, provider string) (*client.ClientProviderKey, error) {
	return nil, nil // not found → global key fallback
}
func (m *mockKeyRepo) Delete(ctx context.Context, clientID, provider string) error {
	return nil
}
func (m *mockKeyRepo) List(ctx context.Context, clientID string) ([]client.ClientProviderKeyListItem, error) {
	return []client.ClientProviderKeyListItem{}, nil
}
func (m *mockKeyRepo) DeleteAllForClient(ctx context.Context, clientID string) error {
	return nil
}

// ═══════════════════════════════════════════════════════════════
// Mock upstream — non-streaming (returns JSON)
// ═══════════════════════════════════════════════════════════════

// UpstreamConfig controls mock upstream behavior.
type UpstreamConfig struct {
	MinLatency time.Duration
	MaxLatency time.Duration
	FailRate   float64
}

// DefaultUpstreamConfig returns a sensible default for stress tests.
func DefaultUpstreamConfig() UpstreamConfig {
	return UpstreamConfig{
		MinLatency: 20 * time.Millisecond,
		MaxLatency: 80 * time.Millisecond,
		FailRate:   0.02,
	}
}

type mockUpstreamHandler struct {
	mu         sync.Mutex
	requestLog []time.Time
	UpstreamConfig
}

func (h *mockUpstreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.requestLog = append(h.requestLog, time.Now())
	h.mu.Unlock()

	auth := r.Header.Get("Authorization")
	if auth == "" || !bytes.HasPrefix([]byte(auth), []byte("Bearer ")) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing auth"})
		return
	}

	latency := h.MinLatency
	if h.MaxLatency > h.MinLatency {
		latency += time.Duration(rand.Int63n(int64(h.MaxLatency - h.MinLatency)))
	}
	if latency > 0 {
		time.Sleep(latency)
	}

	h.mu.Lock()
	fail := h.FailRate > 0 && rand.Float64() < h.FailRate
	h.mu.Unlock()

	if fail {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"Internal server error","type":"server_error"}}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{
		"id": "chatcmpl-mock-stress",
		"object": "chat.completion",
		"created": 1700000000,
		"model": "gpt-4o",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "This is a mock stress test response."
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 20,
			"total_tokens": 30
		}
	}`))
}

// ═══════════════════════════════════════════════════════════════
// Mock upstream — SSE streaming
// ═══════════════════════════════════════════════════════════════

// SSEUpstreamConfig controls mock SSE upstream behavior.
type SSEUpstreamConfig struct {
	InitialLatency time.Duration // delay before first chunk
	ChunkCount     int           // content chunks before [DONE]
	ChunkInterval  time.Duration // delay between chunks
	FailRate       float64
}

// DefaultSSEUpstreamConfig returns sensible defaults for streaming stress tests.
func DefaultSSEUpstreamConfig() SSEUpstreamConfig {
	return SSEUpstreamConfig{
		InitialLatency: 20 * time.Millisecond,
		ChunkCount:     11,
		ChunkInterval:  5 * time.Millisecond,
		FailRate:       0.02,
	}
}

func (cfg SSEUpstreamConfig) EstimatedDuration() time.Duration {
	return cfg.InitialLatency + time.Duration(cfg.ChunkCount)*cfg.ChunkInterval
}

type mockSSEUpstreamHandler struct {
	mu         sync.Mutex
	requestLog []time.Time
	SSEUpstreamConfig
}

func (h *mockSSEUpstreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.requestLog = append(h.requestLog, time.Now())
	h.mu.Unlock()

	auth := r.Header.Get("Authorization")
	if auth == "" || !bytes.HasPrefix([]byte(auth), []byte("Bearer ")) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing auth"})
		return
	}

	if h.InitialLatency > 0 {
		time.Sleep(h.InitialLatency)
	}

	h.mu.Lock()
	fail := h.FailRate > 0 && rand.Float64() < h.FailRate
	h.mu.Unlock()

	if fail {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"Internal server error","type":"server_error"}}`))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	chunks := []string{
		"Hello", " this", " is", " a", " mock",
		" streaming", " response", " from", " the", " AI", " proxy",
	}

	for i := 0; i < h.ChunkCount && i < len(chunks); i++ {
		sseLine := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q},\"index\":0}]}\n\n", chunks[i])
		fmt.Fprint(w, sseLine)
		flusher.Flush()
		if h.ChunkInterval > 0 {
			time.Sleep(h.ChunkInterval)
		}
	}

	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}
