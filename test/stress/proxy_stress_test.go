//go:build stress

package stress

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
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
// Mock: Client Repository (in-memory, returns a single test client)
// ═══════════════════════════════════════════════════════════════

const (
	testClientID     = "test-client-stress"
	testClientSecret = "sk-stress-test-secret-1234567890"
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

const testMasterKey = "stress-test-master-key"

func testClient() *client.Client {
	now := time.Now()

	// Encrypt keys at rest so the Service layer's decryptClientKeys can
	// properly decrypt them (mirrors the real repo which stores encrypted keys).
	// The cached client will have the raw keys for middleware access.
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
// Mock: Provider Repository (in-memory, returns a single test provider)
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
// Mock: ClientProviderKeyRepository (always returns "not found" → global key fallback)
// ═══════════════════════════════════════════════════════════════

type mockKeyRepo struct{}

func (m *mockKeyRepo) Set(ctx context.Context, input client.SetClientProviderKeyInput, encryptedKey string) (*client.ClientProviderKey, error) {
	return nil, nil
}
func (m *mockKeyRepo) Get(ctx context.Context, clientID, provider string) (*client.ClientProviderKey, error) {
	return nil, nil // not found → fall back to global key
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
// Mock upstream AI provider — returns controlled JSON responses (non-streaming)
// ═══════════════════════════════════════════════════════════════

type mockUpstreamHandler struct {
	mu         sync.Mutex
	requestLog []time.Time
	minLatency time.Duration
	maxLatency time.Duration
	failRate   float64
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

	latency := h.minLatency
	if h.maxLatency > h.minLatency {
		latency += time.Duration(rand.Int63n(int64(h.maxLatency - h.minLatency)))
	}
	if latency > 0 {
		time.Sleep(latency)
	}

	h.mu.Lock()
	fail := h.failRate > 0 && rand.Float64() < h.failRate
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
// Mock upstream SSE handler — returns chunked SSE responses (streaming)
// ═══════════════════════════════════════════════════════════════

type mockSSEUpstreamHandler struct {
	mu              sync.Mutex
	requestLog      []time.Time
	initialLatency  time.Duration // delay before first chunk
	chunkCount      int           // number of content chunks before [DONE]
	chunkInterval   time.Duration // delay between chunks
	failRate        float64
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

	// Simulate initial latency (like the mock JSON handler)
	if h.initialLatency > 0 {
		time.Sleep(h.initialLatency)
	}

	h.mu.Lock()
	fail := h.failRate > 0 && rand.Float64() < h.failRate
	h.mu.Unlock()

	if fail {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"Internal server error","type":"server_error"}}`))
		return
	}

	// SSE response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	// Write content chunks
	chunks := []string{
		"Hello",
		" this",
		" is",
		" a",
		" mock",
		" streaming",
		" response",
		" from",
		" the",
		" AI",
		" proxy",
	}

	for i := 0; i < h.chunkCount && i < len(chunks); i++ {
		chunk := chunks[i]
		sseLine := fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q},\"index\":0}]}\n\n", chunk)
		fmt.Fprint(w, sseLine)
		flusher.Flush()
		if h.chunkInterval > 0 {
			time.Sleep(h.chunkInterval)
		}
	}

	// End-of-stream marker
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// ═══════════════════════════════════════════════════════════════
// Metrics collector (shared)
// ═══════════════════════════════════════════════════════════════

type stressMetrics struct {
	mu          sync.Mutex
	latencies   []time.Duration
	success     int64
	failures    int64
	statusCount map[int]int64
	startTime   time.Time
	endTime     time.Time
}

func newStressMetrics() *stressMetrics {
	return &stressMetrics{
		statusCount: make(map[int]int64),
	}
}

func (m *stressMetrics) record(latency time.Duration, statusCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencies = append(m.latencies, latency)
	m.statusCount[statusCode]++
	if statusCode >= 200 && statusCode < 300 {
		m.success++
	} else {
		m.failures++
	}
}

func (m *stressMetrics) percentile(p float64) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(m.latencies))
	copy(sorted, m.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil(p/100.0*float64(len(sorted))) - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (m *stressMetrics) total() int64                     { return m.success + m.failures }
func (m *stressMetrics) rps() float64                     { return float64(m.total()) / m.duration().Seconds() }
func (m *stressMetrics) duration() time.Duration          { return m.endTime.Sub(m.startTime) }
func (m *stressMetrics) p50() time.Duration               { return m.percentile(50) }
func (m *stressMetrics) p90() time.Duration               { return m.percentile(90) }
func (m *stressMetrics) p99() time.Duration               { return m.percentile(99) }
func (m *stressMetrics) p100() time.Duration              { return m.percentile(100) }

func (m *stressMetrics) report(concurrency int) string {
	return fmt.Sprintf(`
┌──────────────────────────────────────────────────────────────┐
│  Concurrency: %3d                                            │
├──────────────────────────────────────────────────────────────┤
│  Total requests:    %6d                                     │
│  Successful:        %6d  (%.1f%%)                           │
│  Failed:            %6d  (%.1f%%)                           │
│  Duration:          %s                                       │
│  Throughput:        %8.1f  req/s                             │
├──────────────────────────────────────────────────────────────┤
│  Latency                                                     │
│    p50 (median):    %12s                                     │
│    p90:             %12s                                     │
│    p99:             %12s                                     │
│    p100 (max):      %12s                                     │
└──────────────────────────────────────────────────────────────┘`,
		concurrency,
		m.total(), m.success, pct(m.success, m.total()),
		m.failures, pct(m.failures, m.total()),
		formatDur(m.duration()), m.rps(),
		formatDur(m.p50()), formatDur(m.p90()),
		formatDur(m.p99()), formatDur(m.p100()))
}

// ═══════════════════════════════════════════════════════════════
// Streaming-specific metrics (extends stressMetrics)
// ═══════════════════════════════════════════════════════════════

type streamingMetrics struct {
	*stressMetrics
	mu               sync.Mutex
	ttfcLatencies    []time.Duration // time-to-first-chunk
	totalBytes       int64
	contentVerified  int64 // count of responses that contained valid SSE content
}

func newStreamingMetrics() *streamingMetrics {
	return &streamingMetrics{
		stressMetrics: newStressMetrics(),
		ttfcLatencies: []time.Duration{},
	}
}

func (m *streamingMetrics) recordStream(ttfc, total time.Duration, statusCode int, bytes int64, verified bool) {
	m.stressMetrics.record(total, statusCode)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttfcLatencies = append(m.ttfcLatencies, ttfc)
	m.totalBytes += bytes
	if verified {
		m.contentVerified++
	}
}

func (m *streamingMetrics) ttfcPercentile(p float64) time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ttfcLatencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(m.ttfcLatencies))
	copy(sorted, m.ttfcLatencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil(p/100.0*float64(len(sorted))) - 1)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (m *streamingMetrics) avgBytesPerResponse() float64 {
	m.mu.Lock()
	total := m.total()
	m.mu.Unlock()
	if total == 0 {
		return 0
	}
	return float64(m.totalBytes) / float64(total)
}

func (m *streamingMetrics) reportStream(concurrency int) string {
	avgBytes := m.avgBytesPerResponse()
	return fmt.Sprintf(`
┌──────────────────────────────────────────────────────────────┐
│  Concurrency: %3d — SSE Streaming                           │
├──────────────────────────────────────────────────────────────┤
│  Total requests:    %6d                                     │
│  Successful:        %6d  (%.1f%%)                           │
│  Failed:            %6d  (%.1f%%)                           │
│  Duration:          %s                                       │
│  Throughput:        %8.1f  req/s                             │
│  Total data:        %s                                       │
│  Avg/response:      %.0f  bytes                              │
│  Content verified:  %d / %d                                  │
├──────────────────────────────────────────────────────────────┤
│  Total latency                                               │
│    p50 (median):    %12s                                     │
│    p90:             %12s                                     │
│    p99:             %12s                                     │
│    p100 (max):      %12s                                     │
├──────────────────────────────────────────────────────────────┤
│  Time-to-first-chunk (TTFC)                                  │
│    p50 (median):    %12s                                     │
│    p90:             %12s                                     │
│    p99:             %12s                                     │
│    p100 (max):      %12s                                     │
└──────────────────────────────────────────────────────────────┘`,
		concurrency,
		m.total(), m.success, pct(m.success, m.total()),
		m.failures, pct(m.failures, m.total()),
		formatDur(m.duration()), m.rps(),
		formatBytes(m.totalBytes), avgBytes,
		m.contentVerified, m.total(),
		formatDur(m.p50()), formatDur(m.p90()),
		formatDur(m.p99()), formatDur(m.p100()),
		formatDur(m.ttfcPercentile(50)), formatDur(m.ttfcPercentile(90)),
		formatDur(m.ttfcPercentile(99)), formatDur(m.ttfcPercentile(100)))
}

func pct(n, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func formatDur(d time.Duration) string {
	if d < time.Microsecond {
		return fmt.Sprintf("%dns", d.Nanoseconds())
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%.1fµs", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Milliseconds()))
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}

// ═══════════════════════════════════════════════════════════════
// Non-streaming stress test runner
// ═══════════════════════════════════════════════════════════════

func runStressTest(serverURL string, concurrency, requestsPerWorker int) *stressMetrics {
	metrics := newStressMetrics()
	metrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 30 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("stress-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				body := `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}],"stream":false}`

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("Authorization", "Bearer "+testClientSecret)
				req.Header.Set("X-Nonce", nonce)
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", now))

				start := time.Now()
				resp, err := client.Do(req)
				latency := time.Since(start)

				if err != nil {
					metrics.record(latency, 500)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				metrics.record(latency, resp.StatusCode)
			}
		}(w)
	}

	wg.Wait()
	metrics.endTime = time.Now()

	return metrics
}

// ═══════════════════════════════════════════════════════════════
// Streaming stress test runner
// ═══════════════════════════════════════════════════════════════

func runStreamingStressTest(serverURL string, concurrency, requestsPerWorker int) *streamingMetrics {
	metrics := newStreamingMetrics()
	metrics.stressMetrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 60 * time.Second, // longer timeout for streaming
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("sse-stress-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				body := `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}],"stream":true}`

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.stressMetrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("Authorization", "Bearer "+testClientSecret)
				req.Header.Set("X-Nonce", nonce)
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", now))

				start := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					totalLatency := time.Since(start)
					metrics.recordStream(totalLatency, totalLatency, 500, 0, false)
					continue
				}

				// Read response body as SSE
				var firstChunkLatency time.Duration
				var totalBytes int64
				firstChunk := true
				verified := false
				buf := make([]byte, 4096)

				for {
					n, readErr := resp.Body.Read(buf)
					if n > 0 {
						totalBytes += int64(n)
						if firstChunk {
							firstChunkLatency = time.Since(start)
							firstChunk = false
						}
						// Check if this chunk contains [DONE]
						if strings.Contains(string(buf[:n]), "[DONE]") {
							verified = true
						}
					}
					if readErr != nil {
						break
					}
				}
				resp.Body.Close()

				totalLatency := time.Since(start)
				metrics.recordStream(firstChunkLatency, totalLatency, resp.StatusCode, totalBytes, verified)
			}
		}(w)
	}

	wg.Wait()
	metrics.stressMetrics.endTime = time.Now()

	return metrics
}

// ═══════════════════════════════════════════════════════════════
// X-Auth header builder for encrypted auth path
// ═══════════════════════════════════════════════════════════════

// buildXAuthHeader encrypts "client_id:timestamp:nonce" with AES-GCM using
// the client's encryption_key (base64-encoded 32-byte key).
func buildXAuthHeader(clientID, encryptionKey, nonce string, timestamp int64) string {
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
// Encrypted auth: non-streaming stress test runner
// ═══════════════════════════════════════════════════════════════

func runStressTestEncrypted(serverURL string, concurrency, requestsPerWorker int) *stressMetrics {
	metrics := newStressMetrics()
	metrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 30 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("xauth-stress-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				xAuth := buildXAuthHeader(testClientID, testEncryptionKey, nonce, now)
				body := `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}],"stream":false}`

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("X-Auth", xAuth)

				start := time.Now()
				resp, err := client.Do(req)
				latency := time.Since(start)

				if err != nil {
					metrics.record(latency, 500)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				metrics.record(latency, resp.StatusCode)
			}
		}(w)
	}

	wg.Wait()
	metrics.endTime = time.Now()

	return metrics
}

// ═══════════════════════════════════════════════════════════════
// Encrypted auth: streaming stress test runner
// ═══════════════════════════════════════════════════════════════

func runStreamingStressTestEncrypted(serverURL string, concurrency, requestsPerWorker int) *streamingMetrics {
	metrics := newStreamingMetrics()
	metrics.stressMetrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 60 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("xauth-sse-stress-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				xAuth := buildXAuthHeader(testClientID, testEncryptionKey, nonce, now)
				body := `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}],"stream":true}`

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.stressMetrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("X-Auth", xAuth)

				start := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					totalLatency := time.Since(start)
					metrics.recordStream(totalLatency, totalLatency, 500, 0, false)
					continue
				}

				// Read response body as SSE
				var firstChunkLatency time.Duration
				var totalBytes int64
				firstChunk := true
				verified := false
				buf := make([]byte, 4096)

				for {
					n, readErr := resp.Body.Read(buf)
					if n > 0 {
						totalBytes += int64(n)
						if firstChunk {
							firstChunkLatency = time.Since(start)
							firstChunk = false
						}
						if strings.Contains(string(buf[:n]), "[DONE]") {
							verified = true
						}
					}
					if readErr != nil {
						break
					}
				}
				resp.Body.Close()

				totalLatency := time.Since(start)
				metrics.recordStream(firstChunkLatency, totalLatency, resp.StatusCode, totalBytes, verified)
			}
		}(w)
	}

	wg.Wait()
	metrics.stressMetrics.endTime = time.Now()

	return metrics
}

// ═══════════════════════════════════════════════════════════════
// Load-test levels
// ═══════════════════════════════════════════════════════════════

type stressLevel struct {
	concurrency int
	perWorker   int
	label       string
}

var stressLevels = []stressLevel{
	{concurrency: 1, perWorker: 50, label: "Sequential (baseline)"},
	{concurrency: 5, perWorker: 50, label: "Light load"},
	{concurrency: 10, perWorker: 50, label: "Moderate load"},
	{concurrency: 25, perWorker: 40, label: "High load"},
	{concurrency: 50, perWorker: 20, label: "Heavy load"},
	{concurrency: 100, perWorker: 10, label: "Burst load"},
}

var streamingLevels = []stressLevel{
	{concurrency: 1, perWorker: 20, label: "Sequential (baseline)"},
	{concurrency: 5, perWorker: 20, label: "Light load"},
	{concurrency: 10, perWorker: 15, label: "Moderate load"},
	{concurrency: 25, perWorker: 10, label: "High load"},
	{concurrency: 50, perWorker: 6, label: "Heavy load"},
}

// ══════════════════════════════════════════════════════════════════════════
// Test 1: Non-streaming (JSON) stress test
//   go test -tags=stress -v ./test/stress -run TestProxyStress -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStress(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock upstream ────────────────────────────────
	upstreamHandler := &mockUpstreamHandler{
		minLatency: 20 * time.Millisecond,
		maxLatency: 80 * time.Millisecond,
		failRate:   0.02,
	}
	mockUpstream := httptest.NewServer(upstreamHandler)
	defer mockUpstream.Close()
	t.Logf("Mock upstream provider running at: %s", mockUpstream.URL)

	// ── 2. Set up dependencies ────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 30*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Create Gin router ─────────────────────────────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.AuthMiddleware(clientSvc),
		security.NonceMiddleware(nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ Warm-up (10 requests) ━━━")
	warmupMetrics := runStressTest(proxyServer.URL, 2, 5)
	t.Log(warmupMetrics.report(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all stress levels ─────────────────────────────
	var allMetrics []*stressMetrics

	for _, l := range stressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStressTest(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.report(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║           NON-STREAMING STRESS TEST — SUMMARY                  ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Logf("Mock upstream latency:  %s – %s (uniform random)", formatDur(20*time.Millisecond), formatDur(80*time.Millisecond))
	t.Logf("Mock upstream failure:  2%%")
	t.Logf("Middleware chain:       Auth → Nonce → RateLimit → Route → Proxy → Forward")
	t.Log()
	for i, l := range stressLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].report(l.concurrency))
	}
	t.Log()
	t.Log("Dashboard summary:")
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log("Level    Concurrency    Throughput     p50        p99        Errors")
	t.Log("────────────────────────────────────────────────────────────────")
	for i, l := range stressLevels {
		m := allMetrics[i]
		bar := ""
		maxRps := 1000.0
		barLen := int(m.rps() / maxRps * 20)
		if barLen > 20 {
			barLen = 20
		} else if barLen < 1 {
			barLen = 1
		}
		for j := 0; j < barLen; j++ {
			bar += "█"
		}
		t.Logf("  %d  │  %3d concurrent  │  %8.1f/s   │ %7s  │ %7s  │ %3d (%4.1f%%)   %s",
			i+1, l.concurrency, m.rps(),
			formatDur(m.p50()), formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()),
			bar)
	}
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  NON-STREAMING STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}

// ══════════════════════════════════════════════════════════════════════════
// Test 2: Streaming (SSE) stress test
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStress -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStress(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock SSE upstream ───────────────────────────
	sseHandler := &mockSSEUpstreamHandler{
		initialLatency: 20 * time.Millisecond,  // ~20ms before first token
		chunkCount:     11,                      // 11 content chunks
		chunkInterval:  5 * time.Millisecond,    // 5ms between chunks → ~55ms total streaming
		failRate:       0.02,
	}
	mockUpstream := httptest.NewServer(sseHandler)
	defer mockUpstream.Close()
	t.Logf("Mock SSE upstream running at: %s", mockUpstream.URL)
	t.Logf("  Initial latency: %s", formatDur(sseHandler.initialLatency))
	t.Logf("  Chunks: %d × %s interval", sseHandler.chunkCount, formatDur(sseHandler.chunkInterval))
	t.Logf("  Est. stream duration: %s", formatDur(sseHandler.initialLatency+time.Duration(sseHandler.chunkCount)*sseHandler.chunkInterval))

	// ── 2. Set up deps ───────────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 60*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Gin router (same middleware chain) ────────────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.AuthMiddleware(clientSvc),
		security.NonceMiddleware(nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ SSE Warm-up (6 requests) ━━━")
	warmupMetrics := runStreamingStressTest(proxyServer.URL, 2, 3)
	t.Log(warmupMetrics.reportStream(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all streaming stress levels ──────────────────
	var allMetrics []*streamingMetrics

	for _, l := range streamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStreamingStressTest(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.reportStream(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║            STREAMING (SSE) STRESS TEST — SUMMARY               ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Log("Mock SSE upstream:")
	t.Logf("  Initial latency:  %s", formatDur(sseHandler.initialLatency))
	t.Logf("  Chunks:           %d × %s interval", sseHandler.chunkCount, formatDur(sseHandler.chunkInterval))
	t.Logf("  Total stream:     ~%s", formatDur(sseHandler.initialLatency+time.Duration(sseHandler.chunkCount)*sseHandler.chunkInterval))
	t.Log("Middleware chain:    Auth → Nonce → RateLimit → Route → Proxy → ForwardStreaming")
	t.Log()

	t.Log("Per-level reports:")
	for i, l := range streamingLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].reportStream(l.concurrency))
	}

	t.Log()
	t.Log("Streaming dashboard:")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log("Lvl  Concurrency   Throughput    TTFC p50   TTFC p99   Total p99   Errors")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	for i, l := range streamingLevels {
		m := allMetrics[i]
		t.Logf(" %2d     %3d          %7.1f/s     %8s    %8s    %8s    %3d (%4.1f%%)",
			i+1, l.concurrency, m.rps(),
			formatDur(m.ttfcPercentile(50)), formatDur(m.ttfcPercentile(99)),
			formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()))
	}
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("Key observations:")
	t.Log("  - TTFC (time-to-first-chunk) measures how fast the proxy starts streaming")
	t.Log("  - Total latency includes all chunks + inter-chunk delays from upstream")
	t.Log("  - Content verification checks that each response contains [DONE] marker")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  STREAMING STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}

// ══════════════════════════════════════════════════════════════════════════
// Test 3: Non-streaming stress test — Encrypted X-Auth path
//   go test -tags=stress -v ./test/stress -run TestProxyStressEncryptedAuth -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStressEncryptedAuth(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock upstream ────────────────────────────────
	upstreamHandler := &mockUpstreamHandler{
		minLatency: 20 * time.Millisecond,
		maxLatency: 80 * time.Millisecond,
		failRate:   0.02,
	}
	mockUpstream := httptest.NewServer(upstreamHandler)
	defer mockUpstream.Close()
	t.Logf("Mock upstream provider running at: %s", mockUpstream.URL)

	// ── 2. Set up dependencies ────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 30*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Create Gin router with DualAuthMiddleware ──────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.DualAuthMiddleware(clientSvc, nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ Warm-up (10 requests, encrypted X-Auth) ━━━")
	warmupMetrics := runStressTestEncrypted(proxyServer.URL, 2, 5)
	t.Log(warmupMetrics.report(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all stress levels ─────────────────────────────
	var allMetrics []*stressMetrics

	for _, l := range stressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each, X-Auth auth) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStressTestEncrypted(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.report(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║   NON-STREAMING STRESS TEST — ENCRYPTED X-AUTH AUTH             ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Logf("Mock upstream latency:  %s – %s (uniform random)", formatDur(20*time.Millisecond), formatDur(80*time.Millisecond))
	t.Logf("Mock upstream failure:  2%%")
	t.Logf("Auth method:            X-Auth (AES-GCM encrypted payload)")
	t.Logf("Middleware chain:       DualAuth (encrypted) → RateLimit → Route → Proxy → Forward")
	t.Log()
	for i, l := range stressLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].report(l.concurrency))
	}
	t.Log()
	t.Log("Dashboard summary:")
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log("Level    Concurrency    Throughput     p50        p99        Errors")
	t.Log("────────────────────────────────────────────────────────────────")
	for i, l := range stressLevels {
		m := allMetrics[i]
		bar := ""
		rps := m.rps()
		maxRps := 1000.0
		barLen := int(rps / maxRps * 20)
		if barLen > 20 {
			barLen = 20
		} else if barLen < 1 {
			barLen = 1
		}
		for j := 0; j < barLen; j++ {
			bar += "█"
		}
		t.Logf("  %d  │  %3d concurrent  │  %8.1f/s   │ %7s  │ %7s  │ %3d (%4.1f%%)   %s",
			i+1, l.concurrency, rps,
			formatDur(m.p50()), formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()),
			bar)
	}
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  X-AUTH STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}

// ══════════════════════════════════════════════════════════════════════════
// Test 5: Non-streaming stress test — File body with Bearer auth
//   go test -tags=stress -v ./test/stress -run TestProxyStressFileBody -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStressFileBody(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock upstream ────────────────────────────────
	upstreamHandler := &mockUpstreamHandler{
		minLatency: 20 * time.Millisecond,
		maxLatency: 80 * time.Millisecond,
		failRate:   0.02,
	}
	mockUpstream := httptest.NewServer(upstreamHandler)
	defer mockUpstream.Close()
	t.Logf("Mock upstream provider running at: %s", mockUpstream.URL)

	// ── 2. Set up dependencies ────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 30*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Create Gin router ─────────────────────────────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.DualAuthMiddleware(clientSvc, nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ Warm-up (10 requests, file body) ━━━")
	warmupMetrics := runStressTestFileBody(proxyServer.URL, 2, 5)
	t.Log(warmupMetrics.report(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all stress levels ─────────────────────────────
	var allMetrics []*stressMetrics

	for _, l := range stressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each, file body, Bearer auth) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStressTestFileBody(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.report(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║   NON-STREAMING STRESS TEST — FILE BODY (BEARER AUTH)          ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Logf("Request body:         ~%d KB (base64 image + file_ids + JSON)", len(buildFileBodyJSON(false, ""))/1024)
	t.Logf("Mock upstream latency: %s – %s (uniform random)", formatDur(20*time.Millisecond), formatDur(80*time.Millisecond))
	t.Logf("Mock upstream failure:  2%%")
	t.Logf("Auth method:            Bearer (client_secret)")
	t.Logf("Middleware chain:       DualAuth (Bearer) → RateLimit → Route → Proxy → Forward")
	t.Log()
	for i, l := range stressLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].report(l.concurrency))
	}
	t.Log()
	t.Log("Dashboard summary:")
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log("Level    Concurrency    Throughput     p50        p99        Errors")
	t.Log("────────────────────────────────────────────────────────────────")
	for i, l := range stressLevels {
		m := allMetrics[i]
		bar := ""
		maxRps := 1000.0
		barLen := int(m.rps() / maxRps * 20)
		if barLen > 20 {
			barLen = 20
		} else if barLen < 1 {
			barLen = 1
		}
		for j := 0; j < barLen; j++ {
			bar += "█"
		}
		t.Logf("  %d  │  %3d concurrent  │  %8.1f/s   │ %7s  │ %7s  │ %3d (%4.1f%%)   %s",
			i+1, l.concurrency, m.rps(),
			formatDur(m.p50()), formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()),
			bar)
	}
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  FILE BODY (BEARER) STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}

// ══════════════════════════════════════════════════════════════════════════
// Test 6: Streaming stress test — File body with Bearer auth
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStressFileBody -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStressFileBody(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock SSE upstream ───────────────────────────
	sseHandler := &mockSSEUpstreamHandler{
		initialLatency: 20 * time.Millisecond,
		chunkCount:     11,
		chunkInterval:  5 * time.Millisecond,
		failRate:       0.02,
	}
	mockUpstream := httptest.NewServer(sseHandler)
	defer mockUpstream.Close()
	t.Logf("Mock SSE upstream running at: %s", mockUpstream.URL)

	// ── 2. Set up deps ───────────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 60*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Gin router ────────────────────────────────────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.DualAuthMiddleware(clientSvc, nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ SSE Warm-up (6 requests, file body) ━━━")
	warmupMetrics := runStreamingStressTestFileBody(proxyServer.URL, 2, 3)
	t.Log(warmupMetrics.reportStream(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all streaming stress levels ──────────────────
	var allMetrics []*streamingMetrics

	for _, l := range streamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each, file body, Bearer auth) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStreamingStressTestFileBody(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.reportStream(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║      STREAMING STRESS TEST — FILE BODY (BEARER AUTH)            ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Log("Mock SSE upstream:")
	t.Logf("  Initial latency:  %s", formatDur(sseHandler.initialLatency))
	t.Logf("  Chunks:           %d × %s interval", sseHandler.chunkCount, formatDur(sseHandler.chunkInterval))
	t.Logf("  Total stream:     ~%s", formatDur(sseHandler.initialLatency+time.Duration(sseHandler.chunkCount)*sseHandler.chunkInterval))
	t.Logf("  Request body:      ~%d KB (base64 image + file_ids + JSON)", len(buildFileBodyJSON(true, ""))/1024)
	t.Log("Auth method:        Bearer (client_secret)")
	t.Log("Middleware chain:   DualAuth (Bearer) → RateLimit → Route → Proxy → ForwardStreaming")
	t.Log()

	t.Log("Per-level reports:")
	for i, l := range streamingLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].reportStream(l.concurrency))
	}

	t.Log()
	t.Log("Streaming dashboard:")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log("Lvl  Concurrency   Throughput    TTFC p50   TTFC p99   Total p99   Errors")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	for i, l := range streamingLevels {
		m := allMetrics[i]
		t.Logf(" %2d     %3d          %7.1f/s     %8s    %8s    %8s    %3d (%4.1f%%)",
			i+1, l.concurrency, m.rps(),
			formatDur(m.ttfcPercentile(50)), formatDur(m.ttfcPercentile(99)),
			formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()))
	}
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  FILE BODY (BEARER) STREAMING STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}

// ══════════════════════════════════════════════════════════════════════════
// Test 7: Non-streaming stress test — File body with X-Auth encrypted auth
//   go test -tags=stress -v ./test/stress -run TestProxyStressFileBodyEncrypted -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStressFileBodyEncrypted(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock upstream ────────────────────────────────
	upstreamHandler := &mockUpstreamHandler{
		minLatency: 20 * time.Millisecond,
		maxLatency: 80 * time.Millisecond,
		failRate:   0.02,
	}
	mockUpstream := httptest.NewServer(upstreamHandler)
	defer mockUpstream.Close()
	t.Logf("Mock upstream provider running at: %s", mockUpstream.URL)

	// ── 2. Set up dependencies ────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 30*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Create Gin router with DualAuthMiddleware ──────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.DualAuthMiddleware(clientSvc, nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ Warm-up (10 requests, file body, X-Auth) ━━━")
	warmupMetrics := runStressTestFileBodyEncrypted(proxyServer.URL, 2, 5)
	t.Log(warmupMetrics.report(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all stress levels ─────────────────────────────
	var allMetrics []*stressMetrics

	for _, l := range stressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each, file body, X-Auth auth) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStressTestFileBodyEncrypted(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.report(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║ NON-STREAMING STRESS TEST — FILE BODY (X-AUTH ENCRYPTED AUTH)  ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Logf("Request body:         ~%d KB (base64 image + file_ids + JSON)", len(buildFileBodyJSON(false, ""))/1024)
	t.Logf("Mock upstream latency: %s – %s (uniform random)", formatDur(20*time.Millisecond), formatDur(80*time.Millisecond))
	t.Logf("Mock upstream failure:  2%%")
	t.Logf("Auth method:            X-Auth (AES-GCM encrypted payload)")
	t.Logf("Middleware chain:       DualAuth (encrypted) → RateLimit → Route → Proxy → Forward")
	t.Log()
	for i, l := range stressLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].report(l.concurrency))
	}
	t.Log()
	t.Log("Dashboard summary:")
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log("Level    Concurrency    Throughput     p50        p99        Errors")
	t.Log("────────────────────────────────────────────────────────────────")
	for i, l := range stressLevels {
		m := allMetrics[i]
		bar := ""
		rps := m.rps()
		maxRps := 1000.0
		barLen := int(rps / maxRps * 20)
		if barLen > 20 {
			barLen = 20
		} else if barLen < 1 {
			barLen = 1
		}
		for j := 0; j < barLen; j++ {
			bar += "█"
		}
		t.Logf("  %d  │  %3d concurrent  │  %8.1f/s   │ %7s  │ %7s  │ %3d (%4.1f%%)   %s",
			i+1, l.concurrency, rps,
			formatDur(m.p50()), formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()),
			bar)
	}
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  FILE BODY (X-AUTH) STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}

// ══════════════════════════════════════════════════════════════════════════
// Test 8: Streaming stress test — File body with X-Auth encrypted auth
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStressFileBodyEncrypted -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStressFileBodyEncrypted(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock SSE upstream ───────────────────────────
	sseHandler := &mockSSEUpstreamHandler{
		initialLatency: 20 * time.Millisecond,
		chunkCount:     11,
		chunkInterval:  5 * time.Millisecond,
		failRate:       0.02,
	}
	mockUpstream := httptest.NewServer(sseHandler)
	defer mockUpstream.Close()
	t.Logf("Mock SSE upstream running at: %s", mockUpstream.URL)

	// ── 2. Set up deps ───────────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 60*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Gin router with DualAuthMiddleware ────────────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.DualAuthMiddleware(clientSvc, nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ SSE Warm-up (6 requests, file body, X-Auth) ━━━")
	warmupMetrics := runStreamingStressTestFileBodyEncrypted(proxyServer.URL, 2, 3)
	t.Log(warmupMetrics.reportStream(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all streaming stress levels ──────────────────
	var allMetrics []*streamingMetrics

	for _, l := range streamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each, file body, X-Auth auth) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStreamingStressTestFileBodyEncrypted(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.reportStream(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║  STREAMING STRESS TEST — FILE BODY (X-AUTH ENCRYPTED AUTH)     ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Log("Mock SSE upstream:")
	t.Logf("  Initial latency:  %s", formatDur(sseHandler.initialLatency))
	t.Logf("  Chunks:           %d × %s interval", sseHandler.chunkCount, formatDur(sseHandler.chunkInterval))
	t.Logf("  Total stream:     ~%s", formatDur(sseHandler.initialLatency+time.Duration(sseHandler.chunkCount)*sseHandler.chunkInterval))
	t.Logf("  Request body:      ~%d KB (base64 image + file_ids + JSON)", len(buildFileBodyJSON(true, ""))/1024)
	t.Log("Auth method:        X-Auth (AES-GCM encrypted payload)")
	t.Log("Middleware chain:   DualAuth (encrypted) → RateLimit → Route → Proxy → ForwardStreaming")
	t.Log()

	t.Log("Per-level reports:")
	for i, l := range streamingLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].reportStream(l.concurrency))
	}

	t.Log()
	t.Log("Streaming dashboard:")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log("Lvl  Concurrency   Throughput    TTFC p50   TTFC p99   Total p99   Errors")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	for i, l := range streamingLevels {
		m := allMetrics[i]
		t.Logf(" %2d     %3d          %7.1f/s     %8s    %8s    %8s    %3d (%4.1f%%)",
			i+1, l.concurrency, m.rps(),
			formatDur(m.ttfcPercentile(50)), formatDur(m.ttfcPercentile(99)),
			formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()))
	}
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  FILE BODY (X-AUTH) STREAMING STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}

// ══════════════════════════════════════════════════════════════════════════
// File-based request body payload (simulates a vision-style file reference)
// ══════════════════════════════════════════════════════════════════════════

// fileBodyJSON is a realistic chat completion body with base64-encoded file
// content and file_ids. The base64 string is ~1500 chars of synthetic data
// to simulate a real file attachment, resulting in a ~2KB request body.
var fileBodyBase64 string

func init() {
	// Generate synthetic base64 content to simulate a file attachment
	var buf bytes.Buffer
	for i := 0; i < 200; i++ {
		buf.WriteString("This is sample file content for stress testing the AI Proxy. ")
	}
	fileBodyBase64 = base64.RawStdEncoding.EncodeToString(buf.Bytes())
}

func buildFileBodyJSON(stream bool, fileID string) string {
	streamStr := "false"
	if stream {
		streamStr = "true"
	}
	// Escape any characters that could break JSON
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
}`, fileID, fileBodyBase64, streamStr, fileID)
}

// ══════════════════════════════════════════════════════════════════════════
// File body: non-streaming stress test runner (Bearer auth)
// ══════════════════════════════════════════════════════════════════════════

func runStressTestFileBody(serverURL string, concurrency, requestsPerWorker int) *stressMetrics {
	metrics := newStressMetrics()
	metrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 30 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("file-body-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				fileID := fmt.Sprintf("file-stress-%d-%d-%d", workerID, i, atomic.LoadUint64(&nonceCounter))
				body := buildFileBodyJSON(false, fileID)

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("Authorization", "Bearer "+testClientSecret)
				req.Header.Set("X-Nonce", nonce)
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", now))

				start := time.Now()
				resp, err := client.Do(req)
				latency := time.Since(start)

				if err != nil {
					metrics.record(latency, 500)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				metrics.record(latency, resp.StatusCode)
			}
		}(w)
	}

	wg.Wait()
	metrics.endTime = time.Now()

	return metrics
}

// ══════════════════════════════════════════════════════════════════════════
// File body: streaming stress test runner (Bearer auth)
// ══════════════════════════════════════════════════════════════════════════

func runStreamingStressTestFileBody(serverURL string, concurrency, requestsPerWorker int) *streamingMetrics {
	metrics := newStreamingMetrics()
	metrics.stressMetrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 60 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("file-body-sse-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				fileID := fmt.Sprintf("file-stress-%d-%d-%d", workerID, i, atomic.LoadUint64(&nonceCounter))
				body := buildFileBodyJSON(true, fileID)

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.stressMetrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("Authorization", "Bearer "+testClientSecret)
				req.Header.Set("X-Nonce", nonce)
				req.Header.Set("X-Timestamp", fmt.Sprintf("%d", now))

				start := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					totalLatency := time.Since(start)
					metrics.recordStream(totalLatency, totalLatency, 500, 0, false)
					continue
				}

				var firstChunkLatency time.Duration
				var totalBytes int64
				firstChunk := true
				verified := false
				buf := make([]byte, 4096)

				for {
					n, readErr := resp.Body.Read(buf)
					if n > 0 {
						totalBytes += int64(n)
						if firstChunk {
							firstChunkLatency = time.Since(start)
							firstChunk = false
						}
						if strings.Contains(string(buf[:n]), "[DONE]") {
							verified = true
						}
					}
					if readErr != nil {
						break
					}
				}
				resp.Body.Close()

				totalLatency := time.Since(start)
				metrics.recordStream(firstChunkLatency, totalLatency, resp.StatusCode, totalBytes, verified)
			}
		}(w)
	}

	wg.Wait()
	metrics.stressMetrics.endTime = time.Now()

	return metrics
}

// ══════════════════════════════════════════════════════════════════════════
// File body: non-streaming stress test runner (X-Auth encrypted auth)
// ══════════════════════════════════════════════════════════════════════════

func runStressTestFileBodyEncrypted(serverURL string, concurrency, requestsPerWorker int) *stressMetrics {
	metrics := newStressMetrics()
	metrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 30 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("xauth-file-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				xAuth := buildXAuthHeader(testClientID, testEncryptionKey, nonce, now)
				fileID := fmt.Sprintf("file-stress-%d-%d-%d", workerID, i, atomic.LoadUint64(&nonceCounter))
				body := buildFileBodyJSON(false, fileID)

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("X-Auth", xAuth)

				start := time.Now()
				resp, err := client.Do(req)
				latency := time.Since(start)

				if err != nil {
					metrics.record(latency, 500)
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				metrics.record(latency, resp.StatusCode)
			}
		}(w)
	}

	wg.Wait()
	metrics.endTime = time.Now()

	return metrics
}

// ══════════════════════════════════════════════════════════════════════════
// File body: streaming stress test runner (X-Auth encrypted auth)
// ══════════════════════════════════════════════════════════════════════════

func runStreamingStressTestFileBodyEncrypted(serverURL string, concurrency, requestsPerWorker int) *streamingMetrics {
	metrics := newStreamingMetrics()
	metrics.stressMetrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: 60 * time.Second,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < requestsPerWorker; i++ {
				nonce := fmt.Sprintf("xauth-file-sse-%d-%d-%d", workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				xAuth := buildXAuthHeader(testClientID, testEncryptionKey, nonce, now)
				fileID := fmt.Sprintf("file-stress-%d-%d-%d", workerID, i, atomic.LoadUint64(&nonceCounter))
				body := buildFileBodyJSON(true, fileID)

				req, err := http.NewRequest("POST", serverURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.stressMetrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Client-ID", testClientID)
				req.Header.Set("X-Auth", xAuth)

				start := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					totalLatency := time.Since(start)
					metrics.recordStream(totalLatency, totalLatency, 500, 0, false)
					continue
				}

				var firstChunkLatency time.Duration
				var totalBytes int64
				firstChunk := true
				verified := false
				buf := make([]byte, 4096)

				for {
					n, readErr := resp.Body.Read(buf)
					if n > 0 {
						totalBytes += int64(n)
						if firstChunk {
							firstChunkLatency = time.Since(start)
							firstChunk = false
						}
						if strings.Contains(string(buf[:n]), "[DONE]") {
							verified = true
						}
					}
					if readErr != nil {
						break
					}
				}
				resp.Body.Close()

				totalLatency := time.Since(start)
				metrics.recordStream(firstChunkLatency, totalLatency, resp.StatusCode, totalBytes, verified)
			}
		}(w)
	}

	wg.Wait()
	metrics.stressMetrics.endTime = time.Now()

	return metrics
}

// ══════════════════════════════════════════════════════════════════════════
// Test 4: Streaming stress test — Encrypted X-Auth path
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStressEncryptedAuth -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStressEncryptedAuth(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)
	logger.Init(logger.Config{Level: "error", Format: "text", AddSource: false})

	// ── 1. Start mock SSE upstream ───────────────────────────
	sseHandler := &mockSSEUpstreamHandler{
		initialLatency: 20 * time.Millisecond,
		chunkCount:     11,
		chunkInterval:  5 * time.Millisecond,
		failRate:       0.02,
	}
	mockUpstream := httptest.NewServer(sseHandler)
	defer mockUpstream.Close()
	t.Logf("Mock SSE upstream running at: %s", mockUpstream.URL)

	// ── 2. Set up deps ───────────────────────────────────────
	clientRepo := &mockClientRepo{}
	clientSvc := client.NewService(clientRepo, "stress-test-master-key")

	providerRepo := &mockProviderRepo{upstreamURL: mockUpstream.URL}
	providerReg := provider.NewRegistry(providerRepo)
	if err := providerReg.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh registry: %v", err)
	}

	providerKeySvc := client.NewProviderKeyService(&mockKeyRepo{}, clientSvc, "stress-test-master-key")
	proxy := provider.NewProxy(providerReg, providerKeySvc, 60*time.Second)

	nonceStore := security.NewInMemoryNonceStore(5 * time.Minute)
	rateLimiter := security.NewRateLimiter(100000, 10000)
	defer rateLimiter.Stop()

	// ── 3. Gin router with DualAuthMiddleware ────────────────
	cfg := config.Load()
	cfg.RateLimitRequestsPerMin = 100000
	cfg.RateLimitBurst = 10000

	router := shared.NewRouter(cfg)
	api := router.Group("/api/v1")
	api.POST("/chat/completions",
		provider.DualAuthMiddleware(clientSvc, nonceStore, 5*time.Minute),
		security.RateLimitMiddleware(rateLimiter),
		provider.RouteMiddleware(proxy),
	)

	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()
	t.Logf("Proxy server running at: %s", proxyServer.URL)

	// ── 4. Warm-up ───────────────────────────────────────────
	t.Log("\n━━━ SSE Warm-up (6 requests, encrypted X-Auth) ━━━")
	warmupMetrics := runStreamingStressTestEncrypted(proxyServer.URL, 2, 3)
	t.Log(warmupMetrics.reportStream(2))
	t.Log("Warm-up complete.\n")

	// ── 5. Run all streaming stress levels ──────────────────
	var allMetrics []*streamingMetrics

	for _, l := range streamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each, X-Auth auth) ━━━",
			l.label, l.concurrency, l.perWorker)

		metrics := runStreamingStressTestEncrypted(proxyServer.URL, l.concurrency, l.perWorker)
		t.Log(metrics.reportStream(l.concurrency))
		allMetrics = append(allMetrics, metrics)
	}

	// ── 6. Summary dashboard ─────────────────────────────────
	t.Log("\n\n╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║     STREAMING STRESS TEST — ENCRYPTED X-AUTH AUTH               ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()
	t.Log("Mock SSE upstream:")
	t.Logf("  Initial latency:  %s", formatDur(sseHandler.initialLatency))
	t.Logf("  Chunks:           %d × %s interval", sseHandler.chunkCount, formatDur(sseHandler.chunkInterval))
	t.Logf("  Total stream:     ~%s", formatDur(sseHandler.initialLatency+time.Duration(sseHandler.chunkCount)*sseHandler.chunkInterval))
	t.Log("Auth method:        X-Auth (AES-GCM encrypted payload)")
	t.Log("Middleware chain:   DualAuth (encrypted) → RateLimit → Route → Proxy → ForwardStreaming")
	t.Log()

	t.Log("Per-level reports:")
	for i, l := range streamingLevels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.label, l.concurrency)
		t.Log(allMetrics[i].reportStream(l.concurrency))
	}

	t.Log()
	t.Log("Streaming dashboard:")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log("Lvl  Concurrency   Throughput    TTFC p50   TTFC p99   Total p99   Errors")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	for i, l := range streamingLevels {
		m := allMetrics[i]
		t.Logf(" %2d     %3d          %7.1f/s     %8s    %8s    %8s    %3d (%4.1f%%)",
			i+1, l.concurrency, m.rps(),
			formatDur(m.ttfcPercentile(50)), formatDur(m.ttfcPercentile(99)),
			formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()))
	}
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Log("  X-AUTH STREAMING STRESS TEST COMPLETE")
	t.Log("══════════════════════════════════════════════════════════════════")
}
