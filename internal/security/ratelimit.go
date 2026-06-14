package security

import (
	"sync"
	"time"
)

// RateLimiter implements a per-client token bucket rate limiter.
// Each client gets a bucket of tokens that refills at a fixed interval.
// Burst sizes allow short-term spikes.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     int           // tokens added per interval
	interval time.Duration // refill interval
	burst    int           // maximum bucket capacity
	stopCh   chan struct{}
}

type bucket struct {
	tokens    int
	lastRefill time.Time
	lastUsed  time.Time
}

// NewRateLimiter creates a rate limiter.
//   - requestsPerMin: maximum sustained requests per minute per client
//   - burst:          maximum burst size (peak tokens)
func NewRateLimiter(requestsPerMin, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     requestsPerMin,
		interval: 1 * time.Minute,
		burst:    burst,
		stopCh:   make(chan struct{}),
	}
	// If burst is not set, default to requestsPerMin
	if rl.burst <= 0 {
		rl.burst = requestsPerMin
	}
	go rl.refillLoop()
	return rl
}

// Stop terminates the background refill goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// Allow checks whether a request from the given client should be permitted.
// Returns true if the request is within the rate limit, false if rate-limited.
func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[clientID]
	if !ok {
		// First request — create bucket with full burst capacity
		rl.buckets[clientID] = &bucket{
			tokens:    rl.burst - 1,
			lastRefill: time.Now(),
			lastUsed:  time.Now(),
		}
		return true
	}

	b.lastUsed = time.Now()
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// Remaining returns the number of tokens left in the client's bucket.
func (rl *RateLimiter) Remaining(clientID string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[clientID]
	if !ok {
		return rl.burst
	}
	return b.tokens
}

// Reset clears all rate limit state (e.g., when a client rotates keys or is suspended).
func (rl *RateLimiter) Reset(clientID string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.buckets, clientID)
}

// refillLoop periodically refills all buckets and removes stale entries.
// Runs every 1 minute and adds `rate` tokens to each bucket (capped at burst).
func (rl *RateLimiter) refillLoop() {
	ticker := time.NewTicker(rl.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.refill()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) refill() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for id, b := range rl.buckets {
		// Refill tokens: add the rate per interval
		b.tokens += rl.rate
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.lastRefill = now

		// Remove stale buckets that haven't been used in a while
		// (more than 2x the refill interval since last actual use)
		if b.tokens >= rl.burst && now.Sub(b.lastUsed) > 2*rl.interval {
			delete(rl.buckets, id)
		}
	}
}
