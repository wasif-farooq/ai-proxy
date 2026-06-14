//go:build stress

package stress

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// RunnerConfig — configures a stress test run
// ═══════════════════════════════════════════════════════════════

// RunnerConfig controls how each request is built and sent.
type RunnerConfig struct {
	ServerURL     string
	Concurrency   int
	RequestsPerWorker int
	Body          BodyBuilder
	Auth          AuthMethod
	Stream        bool
	Timeout       time.Duration
	NoncePrefix   string
}

// DefaultRunnerConfig returns a sensible RunnerConfig.
func DefaultRunnerConfig(serverURL string, auth AuthMethod, stream bool) RunnerConfig {
	prefix := "stress"
	if stream {
		prefix = "sse-stress"
	}
	if auth == AuthXAuth {
		prefix = "xauth-" + prefix
	}
	timeout := 30 * time.Second
	if stream {
		timeout = 60 * time.Second
	}
	return RunnerConfig{
		ServerURL:     serverURL,
		Concurrency:   1,
		RequestsPerWorker: 50,
		Body:          SimpleBody,
		Auth:          auth,
		Stream:        stream,
		Timeout:       timeout,
		NoncePrefix:   prefix,
	}
}

// ═══════════════════════════════════════════════════════════════
// Non-streaming runner
// ═══════════════════════════════════════════════════════════════

// RunStress executes a non-streaming stress test with the given config.
func RunStress(cfg RunnerConfig) *stressMetrics {
	metrics := newStressMetrics()
	metrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: cfg.Timeout,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < cfg.RequestsPerWorker; i++ {
				nonce := fmt.Sprintf("%s-%d-%d-%d", cfg.NoncePrefix, workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				bodyID := fmt.Sprintf("req-%d-%d-%d", workerID, i, atomic.LoadUint64(&nonceCounter))
				body := cfg.Body(false, bodyID)

				req, err := http.NewRequest("POST", cfg.ServerURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				applyAuth(cfg.Auth, req, nonce, now)

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
// Streaming runner
// ═══════════════════════════════════════════════════════════════

// RunStreamingStress executes a streaming (SSE) stress test.
func RunStreamingStress(cfg RunnerConfig) *streamingMetrics {
	metrics := newStreamingMetrics()
	metrics.stressMetrics.startTime = time.Now()

	var nonceCounter uint64
	var wg sync.WaitGroup

	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			client := &http.Client{
				Timeout: cfg.Timeout,
				Transport: &http.Transport{
					MaxIdleConnsPerHost: 100,
					MaxIdleConns:        200,
				},
			}

			for i := 0; i < cfg.RequestsPerWorker; i++ {
				nonce := fmt.Sprintf("%s-%d-%d-%d", cfg.NoncePrefix, workerID, i, atomic.AddUint64(&nonceCounter, 1))
				now := time.Now().Unix()
				bodyID := fmt.Sprintf("req-%d-%d-%d", workerID, i, atomic.LoadUint64(&nonceCounter))
				body := cfg.Body(true, bodyID)

				req, err := http.NewRequest("POST", cfg.ServerURL+"/api/v1/chat/completions",
					bytes.NewReader([]byte(body)))
				if err != nil {
					metrics.stressMetrics.record(0, 500)
					continue
				}

				req.Header.Set("Content-Type", "application/json")
				applyAuth(cfg.Auth, req, nonce, now)

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

// ═══════════════════════════════════════════════════════════════
// Auth header helpers
// ═══════════════════════════════════════════════════════════════

func applyAuth(auth AuthMethod, req *http.Request, nonce string, timestamp int64) {
	req.Header.Set("X-Client-ID", testClientID)

	switch auth {
	case AuthBearer:
		req.Header.Set("Authorization", "Bearer "+testClientSecret)
		req.Header.Set("X-Nonce", nonce)
		req.Header.Set("X-Timestamp", fmt.Sprintf("%d", timestamp))
	case AuthXAuth:
		xAuth := BuildXAuthHeader(testClientID, testEncryptionKey, nonce, timestamp)
		req.Header.Set("X-Auth", xAuth)
	}
}

// ═══════════════════════════════════════════════════════════════
// Warmup helpers
// ═══════════════════════════════════════════════════════════════

// Warmup runs a small stress test to warm up the proxy.
func Warmup(serverURL string, auth AuthMethod, stream bool) *stressMetrics {
	cfg := DefaultRunnerConfig(serverURL, auth, stream)
	cfg.Concurrency = 2
	cfg.RequestsPerWorker = 5
	if stream {
		cfg.RequestsPerWorker = 3
	}
	cfg.NoncePrefix = "warmup"
	return RunStress(cfg)
}

// WarmupStream runs a small streaming stress test to warm up.
func WarmupStream(serverURL string, auth AuthMethod) *streamingMetrics {
	cfg := DefaultRunnerConfig(serverURL, auth, true)
	cfg.Concurrency = 2
	cfg.RequestsPerWorker = 3
	cfg.NoncePrefix = "warmup-sse"
	return RunStreamingStress(cfg)
}

// ═══════════════════════════════════════════════════════════════
// Dashboard printing
// ═══════════════════════════════════════════════════════════════

// PrintDashboard outputs the non-streaming stress test summary table.
func PrintDashboard(t testingT, header string, levels []stressLevel, results []*stressMetrics) {
	t.Log()
	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Logf("║  %-58s ║", header)
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()

	for i, l := range levels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.Label, l.Concurrency)
		t.Log(results[i].report(l.Concurrency))
	}

	t.Log()
	t.Log("Dashboard summary:")
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log("Level    Concurrency    Throughput     p50        p99        Errors")
	t.Log("────────────────────────────────────────────────────────────────")
	for i, l := range levels {
		m := results[i]
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
			i+1, l.Concurrency, m.rps(),
			formatDur(m.p50()), formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()),
			bar)
	}
	t.Log("────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Logf("  %s", header)
	t.Log("══════════════════════════════════════════════════════════════════")
}

// PrintStreamingDashboard outputs the streaming stress test summary table.
func PrintStreamingDashboard(t testingT, header string, levels []stressLevel, results []*streamingMetrics) {
	t.Log()
	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Logf("║  %-58s ║", header)
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log()

	for i, l := range levels {
		t.Logf("── Level %d: %s (%d concurrent) ──", i+1, l.Label, l.Concurrency)
		t.Log(results[i].reportStream(l.Concurrency))
	}

	t.Log()
	t.Log("Streaming dashboard:")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log("Lvl  Concurrency   Throughput    TTFC p50   TTFC p99   Total p99   Errors")
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	for i, l := range levels {
		m := results[i]
		t.Logf(" %2d     %3d          %7.1f/s     %8s    %8s    %8s    %3d (%4.1f%%)",
			i+1, l.Concurrency, m.rps(),
			formatDur(m.ttfcPercentile(50)), formatDur(m.ttfcPercentile(99)),
			formatDur(m.p99()),
			m.failures, pct(m.failures, m.total()))
	}
	t.Log("─────────────────────────────────────────────────────────────────────────────")
	t.Log()
	t.Log("══════════════════════════════════════════════════════════════════")
	t.Logf("  %s", header)
	t.Log("══════════════════════════════════════════════════════════════════")
}
