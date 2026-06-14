package client

import (
	"sync"
	"time"
)

// Cache provides a thread-safe in-memory store for client entities,
// used as a fast path for proxy authentication lookups.
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry // keyed by client_id
	ttl      time.Duration
	stopCh   chan struct{}
}

type cacheEntry struct {
	client    *Client
	expiresAt time.Time
}

// NewCache creates a cache with the given TTL and starts a background
// eviction goroutine that runs every minute.
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go c.evictLoop()
	return c
}

// Stop terminates the background eviction goroutine.
func (c *Cache) Stop() {
	close(c.stopCh)
}

// Get returns a cached client by client_id, or nil on miss.
func (c *Cache) Get(clientID string) *Client {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[clientID]
	if !ok {
		return nil
	}
	if time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.client
}

// Set inserts or refreshes a client in the cache.
func (c *Cache) Set(client *Client) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[client.ClientID] = &cacheEntry{
		client:    client,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a client from the cache by client_id.
func (c *Cache) Delete(clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, clientID)
}

// Clear purges all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// Len returns the number of cached entries.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// evictLoop runs every 60 seconds and removes expired entries.
func (c *Cache) evictLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.evictExpired()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cache) evictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, k)
		}
	}
}
