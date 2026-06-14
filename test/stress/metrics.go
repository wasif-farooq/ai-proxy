//go:build stress

package stress

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// Non-streaming metrics collector
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

func (m *stressMetrics) total() int64            { return m.success + m.failures }
func (m *stressMetrics) rps() float64            { return float64(m.total()) / m.duration().Seconds() }
func (m *stressMetrics) duration() time.Duration { return m.endTime.Sub(m.startTime) }
func (m *stressMetrics) p50() time.Duration      { return m.percentile(50) }
func (m *stressMetrics) p90() time.Duration      { return m.percentile(90) }
func (m *stressMetrics) p99() time.Duration      { return m.percentile(99) }
func (m *stressMetrics) p100() time.Duration     { return m.percentile(100) }

func (m *stressMetrics) report(concurrency int) string {
	total := m.total()
	return fmt.Sprintf(`%s
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
%s`,
		"┌──────────────────────────────────────────────────────────────┐",
		concurrency,
		total, m.success, pct(m.success, total),
		m.failures, pct(m.failures, total),
		formatDur(m.duration()), m.rps(),
		formatDur(m.p50()), formatDur(m.p90()),
		formatDur(m.p99()), formatDur(m.p100()),
		"└──────────────────────────────────────────────────────────────┘")
}

// ═══════════════════════════════════════════════════════════════
// Streaming metrics collector (extends stressMetrics)
// ═══════════════════════════════════════════════════════════════

type streamingMetrics struct {
	*stressMetrics
	mu              sync.Mutex
	ttfcLatencies   []time.Duration // time-to-first-chunk
	totalBytes      int64
	contentVerified int64 // count of responses containing [DONE]
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
	total := m.total()
	return fmt.Sprintf(`%s
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
%s`,
		"┌──────────────────────────────────────────────────────────────┐",
		concurrency,
		total, m.success, pct(m.success, total),
		m.failures, pct(m.failures, total),
		formatDur(m.duration()), m.rps(),
		formatBytes(m.totalBytes), avgBytes,
		m.contentVerified, total,
		formatDur(m.p50()), formatDur(m.p90()),
		formatDur(m.p99()), formatDur(m.p100()),
		formatDur(m.ttfcPercentile(50)), formatDur(m.ttfcPercentile(90)),
		formatDur(m.ttfcPercentile(99)), formatDur(m.ttfcPercentile(100)),
		"└──────────────────────────────────────────────────────────────┘")
}

// ═══════════════════════════════════════════════════════════════
// Format helpers
// ═══════════════════════════════════════════════════════════════

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
