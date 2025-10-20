package ratelimit

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements the Cache interface using Redis
type RedisCache struct {
	client    *redis.Client
	keyPrefix string
	ttl       time.Duration
	ctx       context.Context
}

// RedisConfig holds configuration for the Redis cache
type RedisConfig struct {
	Addr      string        // Redis server address (e.g., "localhost:6379")
	Password  string        // Redis password (empty for no auth)
	DB        int           // Redis database number
	KeyPrefix string        // Prefix for all keys (e.g., "ratelimit:")
	TTL       time.Duration // Default TTL for keys
}

// NewRedisCache creates a new Redis cache with the specified configuration
func NewRedisCache(config RedisConfig) (*RedisCache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	// Test the connection with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{
		client:    rdb,
		keyPrefix: config.KeyPrefix,
		ttl:       config.TTL,
		ctx:       context.Background(),
	}, nil
}

// Get retrieves a value from Redis
func (r *RedisCache) Get(key uint64) *ClientLimiter {
	redisKey := r.getRedisKey(key)
	val, err := r.client.Get(r.ctx, redisKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		// Log error but don't fail - return nil to indicate cache miss
		return nil
	}

	if len(val) < 16 { // 8 bytes for float64 + 8 bytes for int64 timestamp
		return nil
	}

	limiter := &ClientLimiter{
		allowedRequests: math.Float64frombits(binary.LittleEndian.Uint64(val[0:8])),
		lastRequest:     time.Unix(0, int64(binary.LittleEndian.Uint64(val[8:16]))),
	}

	return limiter
}

// Set stores a value in Redis
func (r *RedisCache) Set(key uint64, value *ClientLimiter) {
	redisKey := r.getRedisKey(key)

	// Pack into 16 bytes: 8 bytes float64 + 8 bytes int64 timestamp
	data := make([]byte, 16)
	binary.LittleEndian.PutUint64(data[0:8], math.Float64bits(value.allowedRequests))
	binary.LittleEndian.PutUint64(data[8:16], uint64(value.lastRequest.UnixNano()))

	r.client.Set(r.ctx, redisKey, data, r.ttl)
}

// Delete removes a value from Redis
func (r *RedisCache) Delete(key uint64) {
	redisKey := r.getRedisKey(key)
	r.client.Del(r.ctx, redisKey)
}

// Close closes the Redis connection
func (r *RedisCache) Close() error {
	return r.client.Close()
}

// getRedisKey creates a Redis key with the configured prefix
func (r *RedisCache) getRedisKey(key uint64) string {
	return fmt.Sprintf("%s%x", r.keyPrefix, key)
}
