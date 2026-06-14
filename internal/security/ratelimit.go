package security

import (
	"crypto/sha256"
	"sync"
	"time"
)

const rateLimiterNumShards = 16

// bucketShard is a single shard of the rate limiter, each with its own lock.
type bucketShard struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

// RateLimiter implements a per-client token bucket rate limiter with 16 shards
// to reduce lock contention under high throughput (1,500+ req/s).
type RateLimiter struct {
	shards         [rateLimiterNumShards]*bucketShard
	ratePerMin     int
	burst          int
	refillInterval time.Duration
	stopCh         chan struct{}
}

type bucket struct {
	tokens     int
	lastRefill time.Time
	lastUsed   time.Time
}

// NewRateLimiter creates a rate limiter.
//   - requestsPerMin: maximum sustained requests per minute per client
//   - burst:          maximum burst size (peak tokens)
func NewRateLimiter(requestsPerMin, burst int) *RateLimiter {
	if burst <= 0 {
		burst = requestsPerMin
	}
	rl := &RateLimiter{
		ratePerMin:     requestsPerMin,
		burst:          burst,
		refillInterval: 1 * time.Minute,
		stopCh:         make(chan struct{}),
	}
	for i := 0; i < rateLimiterNumShards; i++ {
		rl.shards[i] = &bucketShard{
			buckets: make(map[string]*bucket),
		}
	}
	go rl.refillLoop()
	return rl
}

// shardIndex returns the shard index for a given client ID.
func (rl *RateLimiter) shardIndex(clientID string) int {
	h := sha256.Sum256([]byte(clientID))
	return int(h[0]) % rateLimiterNumShards
}

// Stop terminates the background refill goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// Allow checks whether a request from the given client should be permitted.
// Returns true if the request is within the rate limit, false if rate-limited.
func (rl *RateLimiter) Allow(clientID string) bool {
	idx := rl.shardIndex(clientID)
	shard := rl.shards[idx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	b, ok := shard.buckets[clientID]
	if !ok {
		shard.buckets[clientID] = &bucket{
			tokens:     rl.burst - 1,
			lastRefill: time.Now(),
			lastUsed:   time.Now(),
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
	idx := rl.shardIndex(clientID)
	shard := rl.shards[idx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	b, ok := shard.buckets[clientID]
	if !ok {
		return rl.burst
	}
	return b.tokens
}

// Reset clears all rate limit state for a client.
func (rl *RateLimiter) Reset(clientID string) {
	idx := rl.shardIndex(clientID)
	shard := rl.shards[idx]

	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.buckets, clientID)
}

// refillLoop periodically refills all buckets and removes stale entries.
func (rl *RateLimiter) refillLoop() {
	ticker := time.NewTicker(rl.refillInterval)
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
	now := time.Now()
	for i := 0; i < rateLimiterNumShards; i++ {
		shard := rl.shards[i]
		shard.mu.Lock()
		for id, b := range shard.buckets {
			b.tokens += rl.ratePerMin
			if b.tokens > rl.burst {
				b.tokens = rl.burst
			}
			b.lastRefill = now

			// Remove stale buckets (full + unused for 2x the refill interval)
			if b.tokens >= rl.burst && now.Sub(b.lastUsed) > 2*rl.refillInterval {
				delete(shard.buckets, id)
			}
		}
		shard.mu.Unlock()
	}
}
