//go:build stress

package stress

import (
	"math/rand"
	"testing"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════
// Test 1: Non-streaming — Bearer auth, simple body
//   go test -tags=stress -v ./test/stress -run TestProxyStress -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStress(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithUpstream(DefaultUpstreamConfig()).WithLegacyChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock upstream provider running at: %s", h.MockUpstream.URL)
	t.Logf("Proxy server running at: %s", h.ProxyServer.URL)

	t.Log("\n━━━ Warm-up (10 requests) ━━━")
	warmup := Warmup(h.ProxyServer.URL, AuthBearer, false)
	t.Log(warmup.report(2))

	var all []*stressMetrics
	for _, l := range StressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthBearer, false)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		m := RunStress(cfg)
		t.Log(m.report(l.Concurrency))
		all = append(all, m)
	}

	PrintDashboard(t, "NON-STREAMING STRESS TEST — SUMMARY", StressLevels, all)
}

// ══════════════════════════════════════════════════════════════════════════
// Test 2: Streaming — Bearer auth, simple body
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStress -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStress(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithSSEUpstream(DefaultSSEUpstreamConfig()).WithLegacyChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock SSE upstream running at: %s", h.MockUpstream.URL)

	t.Log("\n━━━ SSE Warm-up (6 requests) ━━━")
	warmup := WarmupStream(h.ProxyServer.URL, AuthBearer)
	t.Log(warmup.reportStream(2))

	var all []*streamingMetrics
	for _, l := range StreamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthBearer, true)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		m := RunStreamingStress(cfg)
		t.Log(m.reportStream(l.Concurrency))
		all = append(all, m)
	}

	PrintStreamingDashboard(t, "STREAMING (SSE) STRESS TEST — SUMMARY", StreamingLevels, all)
}

// ══════════════════════════════════════════════════════════════════════════
// Test 3: Non-streaming — X-Auth encrypted auth, simple body
//   go test -tags=stress -v ./test/stress -run TestProxyStressEncryptedAuth -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStressEncryptedAuth(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithUpstream(DefaultUpstreamConfig()).WithDualChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock upstream provider running at: %s", h.MockUpstream.URL)
	t.Logf("Proxy server running at: %s", h.ProxyServer.URL)

	t.Log("\n━━━ Warm-up (10 requests, encrypted X-Auth) ━━━")
	warmup := Warmup(h.ProxyServer.URL, AuthXAuth, false)
	t.Log(warmup.report(2))

	var all []*stressMetrics
	for _, l := range StressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each, X-Auth auth) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthXAuth, false)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		m := RunStress(cfg)
		t.Log(m.report(l.Concurrency))
		all = append(all, m)
	}

	PrintDashboard(t, "NON-STREAMING STRESS TEST — ENCRYPTED X-AUTH AUTH", StressLevels, all)
}

// ══════════════════════════════════════════════════════════════════════════
// Test 4: Streaming — X-Auth encrypted auth, simple body
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStressEncryptedAuth -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStressEncryptedAuth(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithSSEUpstream(DefaultSSEUpstreamConfig()).WithDualChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock SSE upstream running at: %s", h.MockUpstream.URL)

	t.Log("\n━━━ SSE Warm-up (6 requests, encrypted X-Auth) ━━━")
	warmup := WarmupStream(h.ProxyServer.URL, AuthXAuth)
	t.Log(warmup.reportStream(2))

	var all []*streamingMetrics
	for _, l := range StreamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each, X-Auth auth) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthXAuth, true)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		m := RunStreamingStress(cfg)
		t.Log(m.reportStream(l.Concurrency))
		all = append(all, m)
	}

	PrintStreamingDashboard(t, "STREAMING STRESS TEST — ENCRYPTED X-AUTH AUTH", StreamingLevels, all)
}

// ══════════════════════════════════════════════════════════════════════════
// Test 5: Non-streaming — Bearer auth, file body
//   go test -tags=stress -v ./test/stress -run TestProxyStressFileBody -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStressFileBody(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithUpstream(DefaultUpstreamConfig()).WithDualChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock upstream provider running at: %s", h.MockUpstream.URL)
	t.Logf("Proxy server running at: %s", h.ProxyServer.URL)
	t.Logf("Request body: ~%d KB (base64 image + file_ids + JSON)", len(FileBody(false, ""))/1024)

	t.Log("\n━━━ Warm-up (10 requests, file body) ━━━")
	warmup := Warmup(h.ProxyServer.URL, AuthBearer, false)
	t.Log(warmup.report(2))

	var all []*stressMetrics
	for _, l := range StressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each, file body, Bearer auth) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthBearer, false)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		cfg.Body = FileBody
		m := RunStress(cfg)
		t.Log(m.report(l.Concurrency))
		all = append(all, m)
	}

	PrintDashboard(t, "NON-STREAMING STRESS TEST — FILE BODY (BEARER AUTH)", StressLevels, all)
}

// ══════════════════════════════════════════════════════════════════════════
// Test 6: Streaming — Bearer auth, file body
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStressFileBody -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStressFileBody(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithSSEUpstream(DefaultSSEUpstreamConfig()).WithDualChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock SSE upstream running at: %s", h.MockUpstream.URL)
	t.Logf("Request body: ~%d KB (base64 image + file_ids + JSON)", len(FileBody(true, ""))/1024)

	t.Log("\n━━━ SSE Warm-up (6 requests, file body) ━━━")
	warmup := WarmupStream(h.ProxyServer.URL, AuthBearer)
	t.Log(warmup.reportStream(2))

	var all []*streamingMetrics
	for _, l := range StreamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each, file body, Bearer auth) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthBearer, true)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		cfg.Body = FileBody
		m := RunStreamingStress(cfg)
		t.Log(m.reportStream(l.Concurrency))
		all = append(all, m)
	}

	PrintStreamingDashboard(t, "STREAMING STRESS TEST — FILE BODY (BEARER AUTH)", StreamingLevels, all)
}

// ══════════════════════════════════════════════════════════════════════════
// Test 7: Non-streaming — X-Auth encrypted auth, file body
//   go test -tags=stress -v ./test/stress -run TestProxyStressFileBodyEncrypted -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStressFileBodyEncrypted(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithUpstream(DefaultUpstreamConfig()).WithDualChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock upstream provider running at: %s", h.MockUpstream.URL)
	t.Logf("Proxy server running at: %s", h.ProxyServer.URL)
	t.Logf("Request body: ~%d KB (base64 image + file_ids + JSON)", len(FileBody(false, ""))/1024)

	t.Log("\n━━━ Warm-up (10 requests, file body, X-Auth) ━━━")
	warmup := Warmup(h.ProxyServer.URL, AuthXAuth, false)
	t.Log(warmup.report(2))

	var all []*stressMetrics
	for _, l := range StressLevels {
		t.Logf("\n━━━ Running: %s (%d concurrent, %d req each, file body, X-Auth auth) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthXAuth, false)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		cfg.Body = FileBody
		m := RunStress(cfg)
		t.Log(m.report(l.Concurrency))
		all = append(all, m)
	}

	PrintDashboard(t, "NON-STREAMING STRESS TEST — FILE BODY (X-AUTH ENCRYPTED AUTH)", StressLevels, all)
}

// ══════════════════════════════════════════════════════════════════════════
// Test 8: Streaming — X-Auth encrypted auth, file body
//   go test -tags=stress -v ./test/stress -run TestProxyStreamingStressFileBodyEncrypted -timeout 5m
// ══════════════════════════════════════════════════════════════════════════

func TestProxyStreamingStressFileBodyEncrypted(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	h := NewHarness().WithSSEUpstream(DefaultSSEUpstreamConfig()).WithDualChain()
	h.Setup()
	defer h.Close()

	t.Logf("Mock SSE upstream running at: %s", h.MockUpstream.URL)
	t.Logf("Request body: ~%d KB (base64 image + file_ids + JSON)", len(FileBody(true, ""))/1024)

	t.Log("\n━━━ SSE Warm-up (6 requests, file body, X-Auth) ━━━")
	warmup := WarmupStream(h.ProxyServer.URL, AuthXAuth)
	t.Log(warmup.reportStream(2))

	var all []*streamingMetrics
	for _, l := range StreamingLevels {
		t.Logf("\n━━━ SSE: %s (%d concurrent, %d req each, file body, X-Auth auth) ━━━", l.Label, l.Concurrency, l.PerWorker)
		cfg := DefaultRunnerConfig(h.ProxyServer.URL, AuthXAuth, true)
		cfg.Concurrency = l.Concurrency
		cfg.RequestsPerWorker = l.PerWorker
		cfg.Body = FileBody
		m := RunStreamingStress(cfg)
		t.Log(m.reportStream(l.Concurrency))
		all = append(all, m)
	}

	PrintStreamingDashboard(t, "STREAMING STRESS TEST — FILE BODY (X-AUTH ENCRYPTED AUTH)", StreamingLevels, all)
}
