package ratelimit

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// MemoryCache implements the Cache interface using an in-memory LRU cache
type MemoryCache struct {
	cache *expirable.LRU[uint64, *ClientLimiter]
}

// NewMemoryCache creates a new memory cache with the specified capacity and expiration
func NewMemoryCache(capacity int, expiration time.Duration) *MemoryCache {
	cache := expirable.NewLRU[uint64, *ClientLimiter](capacity, nil, expiration)
	return &MemoryCache{
		cache: cache,
	}
}

// Get retrieves a value from the cache
func (m *MemoryCache) Get(key uint64) *ClientLimiter {
	value, found := m.cache.Get(key)
	if !found {
		return nil
	}
	return value
}

// Set stores a value in the cache
func (m *MemoryCache) Set(key uint64, value *ClientLimiter) {
	m.cache.Add(key, value)
}

// Delete removes a value from the cache
func (m *MemoryCache) Delete(key uint64) {
	m.cache.Remove(key)
}

// Close cleans up any resources used by the cache
func (m *MemoryCache) Close() error {
	// Memory cache doesn't need explicit cleanup
	return nil
}
