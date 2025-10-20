package ratelimit

import (
	"testing"
	"time"
)

func TestRedisCache(t *testing.T) {
	config := RedisConfig{
		Addr:      "localhost:6379",
		Password:  "",
		DB:        15,
		KeyPrefix: "ratelimit:test:",
		TTL:       60 * time.Second,
	}

	cache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer cache.Close()

	// Test basic operations
	key := uint64(12345)
	limiter := &ClientLimiter{
		allowedRequests: 5.0,
		lastRequest:     time.Now(),
	}

	// Test Set and Get
	cache.Set(key, limiter)
	retrieved := cache.Get(key)
	if retrieved == nil {
		t.Error("Expected to find the limiter in cache")
	}
	if retrieved.allowedRequests != limiter.allowedRequests {
		t.Errorf("Expected allowedRequests %f, got %f", limiter.allowedRequests, retrieved.allowedRequests)
	}

	// Test Delete
	cache.Delete(key)
	deleted := cache.Get(key)
	if deleted != nil {
		t.Error("Expected limiter to be deleted from cache")
	}
}

func TestRedisCacheWithLimiter(t *testing.T) {
	config := RedisConfig{
		Addr:      "localhost:6379",
		Password:  "",
		DB:        15,
		KeyPrefix: "ratelimit:test:",
		TTL:       60 * time.Second,
	}

	redisCache, err := NewRedisCache(config)
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisCache.Close()

	// Create a limiter with Redis cache
	limiterConfig := Config{
		WindowSize:         1 * time.Second,
		MaxReqs:            2,
		IPv4SubnetMask:     32,
		IPv6SubnetMask:     56,
		IncludeSource:      true,
		IncludeDestination: true,
	}

	limiter := NewWithCache(limiterConfig, redisCache)

	// Test basic rate limiting with Redis
	ip := "192.168.1.1"
	destination := "api.example.com"

	// First request should be allowed
	if retryAfter := limiter.IsAllowed(ip, destination, ""); retryAfter != 0 {
		t.Error("First request should be allowed")
	}

	// Second request should be allowed
	if retryAfter := limiter.IsAllowed(ip, destination, ""); retryAfter != 0 {
		t.Error("Second request should be allowed")
	}

	// Third request should be blocked
	retryAfter := limiter.IsAllowed(ip, destination, "")
	t.Logf("Third request retryAfter: %d", retryAfter)
	if retryAfter == 0 {
		t.Error("Third request should be blocked")
	}
}
