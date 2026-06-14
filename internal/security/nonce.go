package security

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// NonceEntry tracks a nonce and its expiration.
type NonceEntry struct {
	Nonce     string
	ExpiresAt time.Time
}

// NonceStore defines the contract for nonce deduplication.
type NonceStore interface {
	// IsUnique returns true if the nonce has not been seen before within the TTL window.
	IsUnique(clientID, nonce string) bool
	// Clean removes expired entries.
	Clean()
	// Stop terminates the background cleanup goroutine.
	Stop()
}

const numShards = 16

// nonceShard is a single shard of the nonce store, each with its own lock.
type nonceShard struct {
	mu      sync.Mutex
	entries map[string]*NonceEntry
}

// ShardedNonceStore provides a sharded in-memory implementation of NonceStore.
// It uses 16 independent shards, each with its own mutex, to reduce lock contention
// under high throughput (1,500+ req/s). Keys are distributed across shards via
// a hash of "clientID:nonce".
type ShardedNonceStore struct {
	shards [numShards]*nonceShard
	ttl    time.Duration
	stopCh chan struct{}
}

// NewInMemoryNonceStore creates a nonce store with the given TTL and starts
// a background cleanup goroutine that runs every 5 minutes.
func NewInMemoryNonceStore(ttl time.Duration) *ShardedNonceStore {
	s := &ShardedNonceStore{
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	for i := 0; i < numShards; i++ {
		s.shards[i] = &nonceShard{
			entries: make(map[string]*NonceEntry),
		}
	}
	go s.cleanupLoop()
	return s
}

// shardIndex returns the shard index for a given key.
func (s *ShardedNonceStore) shardIndex(key string) int {
	h := sha256.Sum256([]byte(key))
	return int(h[0]) % numShards
}

// Stop terminates the background cleanup goroutine.
func (s *ShardedNonceStore) Stop() {
	close(s.stopCh)
}

// IsUnique returns true if the nonce is new within the TTL window.
// If seen before (or within the TTL), returns false.
func (s *ShardedNonceStore) IsUnique(clientID, nonce string) bool {
	key := fmt.Sprintf("%s:%s", clientID, nonce)
	idx := s.shardIndex(key)
	shard := s.shards[idx]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	if _, exists := shard.entries[key]; exists {
		return false
	}

	shard.entries[key] = &NonceEntry{
		Nonce:     nonce,
		ExpiresAt: time.Now().Add(s.ttl),
	}
	return true
}

// Clean removes all expired entries from every shard.
func (s *ShardedNonceStore) Clean() {
	now := time.Now()
	for i := 0; i < numShards; i++ {
		shard := s.shards[i]
		shard.mu.Lock()
		for k, entry := range shard.entries {
			if now.After(entry.ExpiresAt) {
				delete(shard.entries, k)
			}
		}
		shard.mu.Unlock()
	}
}

// Len returns the total number of tracked nonces across all shards.
func (s *ShardedNonceStore) Len() int {
	total := 0
	for i := 0; i < numShards; i++ {
		shard := s.shards[i]
		shard.mu.Lock()
		total += len(shard.entries)
		shard.mu.Unlock()
	}
	return total
}

// cleanupLoop runs every 5 minutes and purges expired entries.
func (s *ShardedNonceStore) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Clean()
		case <-s.stopCh:
			return
		}
	}
}
