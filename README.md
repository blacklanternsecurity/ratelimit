# Rate Limiter

[![Go Version](https://img.shields.io/badge/go-1.19+-FF8400.svg)](https://golang.org/) [![License](https://img.shields.io/badge/license-GPLv3-FF8400.svg)](LICENSE) [![Tests](https://github.com/blacklanternsecurity/ratelimit/workflows/Tests/badge.svg)](https://github.com/blacklanternsecurity/ratelimit/actions)

A high-performance, non-blocking token bucket rate limiter designed for HTTP projects. Features accurate retry-after headers, multiple cache backends (in-memory and Redis), and flexible IP normalization to prevent circumvention via [IPv6 source address spoofing](https://github.com/blacklanternsecurity/trevorproxy).

## Features

- **Token Bucket Algorithm**: Smooth rate limiting with burst capacity
- **Non-blocking**: Fast, lock-free operations
- **Accurate Retry-After**: Precise timing for client backoff
- **Multiple Cache Backends**: In-memory LRU and Redis support
- **IP Normalization**: Configurable subnet masking for IPv4/IPv6
- **Flexible Keying**: Support for source IP, destination, and custom identifiers

## Installation

```bash
go get github.com/blacklanternsecurity/ratelimit
```

## Quick Start

### Basic Usage

```go
package main

import (
    "fmt"
    "net/http"
    "time"
    
    "github.com/blacklanternsecurity/ratelimit"
)

func main() {
    // Create a rate limiter with default settings
    limiter := ratelimit.NewDefault()
    
    http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
        // Check rate limit for client IP
        retryAfter := limiter.IsAllowed(r.RemoteAddr, r.Host, "api")
        
        if retryAfter > 0 {
            w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
            w.WriteHeader(http.StatusTooManyRequests)
            w.Write([]byte("Rate limit exceeded"))
            return
        }
        
        // Process request
        w.Write([]byte("OK"))
    })
    
    http.ListenAndServe(":8080", nil)
}
```

### Custom Configuration

```go
config := ratelimit.Config{
    Capacity:           1000,              // Cache capacity
    WindowSize:         60 * time.Second,  // Rate limit window
    MaxReqs:            100,               // Requests per window (this doubles as the max burst capacity)
    Expiration:         5 * time.Minute,   // Cache entry TTL
    IPv4SubnetMask:     24,                // /24 subnet for IPv4
    IPv6SubnetMask:     56,                // /56 subnet for IPv6
    IncludeSource:      true,              // Include source IP in key
    IncludeDestination: true,              // Include destination in key
}

limiter := ratelimit.New(config)
```

### Redis Backend

```go
redisConfig := ratelimit.RedisConfig{
    Addr:      "localhost:6379",
    Password:  "",
    DB:        0,
    KeyPrefix: "ratelimit:",
    TTL:       5 * time.Minute,
}

limiter, err := ratelimit.NewWithRedis(config, redisConfig)
if err != nil {
    log.Fatal(err)
}
defer limiter.Close()
```

## Configuration Options

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `Capacity` | `int` | Maximum number of rate limiters to cache | `100000` |
| `WindowSize` | `time.Duration` | Time window for rate limiting | `1 * time.Second` |
| `MaxReqs` | `int` | Maximum requests allowed per window (doubles as max burst) | `10` |
| `Expiration` | `time.Duration` | How long to keep rate limiters in cache | `1 * time.Hour` |
| `IPv4SubnetMask` | `int` | IPv4 subnet mask for IP normalization | `32` (no squashing) |
| `IPv6SubnetMask` | `int` | IPv6 subnet mask for IP normalization | `56` |
| `IncludeSource` | `bool` | Include source IP in rate limit key | `true` |
| `IncludeDestination` | `bool` | Include destination in rate limit key | `true` |

## API Reference

### `IsAllowed(ip, destination, identifier string, maxReqs ...int) int`

Checks if a request should be allowed and returns the retry-after time in seconds.

**Parameters:**
- `ip`: Client IP address
- `destination`: Destination hostname/URL
- `identifier`: Custom identifier for the rate limit
- `maxReqs`: Optional override for max requests (uses config default if not provided)

**Returns:**
- `0`: Request is allowed
- `>0`: Request blocked, retry after N seconds

### Examples

```go
// Basic rate limiting
retryAfter := limiter.IsAllowed("192.168.1.1", "api.example.com", "user123")

// Custom rate limit for specific endpoint
retryAfter := limiter.IsAllowed("192.168.1.1", "api.example.com", "login", 5) // 5 req/min

// Global rate limiting (no destination)
retryAfter := limiter.IsAllowed("192.168.1.1", "", "global")
```

## IP Normalization

The rate limiter supports configurable IP normalization to group similar IPs:

```go
// Group all IPs in /24 subnet (192.168.1.0/24)
config.IPv4SubnetMask = 24

// Group all IPs in /56 subnet for IPv6
config.IPv6SubnetMask = 56
```

## Cache Backends

### In-Memory Cache
- Uses LRU eviction with TTL
- Fast, no external dependencies
- Suitable for single-instance deployments

### Redis Cache
- Distributed rate limiting across multiple instances
- Persistent across restarts
- Requires Redis server

```go
// Custom cache implementation
type CustomCache struct {
    // Your implementation
}

func (c *CustomCache) Get(key uint64) *ratelimit.ClientLimiter { /* ... */ }
func (c *CustomCache) Set(key uint64, value *ratelimit.ClientLimiter) { /* ... */ }
func (c *CustomCache) Delete(key uint64) { /* ... */ }
func (c *CustomCache) Close() error { /* ... */ }

limiter := ratelimit.NewWithCache(config, &CustomCache{})
```

## HTTP Middleware Example

```go
func RateLimitMiddleware(limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            retryAfter := limiter.IsAllowed(r.RemoteAddr, r.Host, "api")
            
            if retryAfter > 0 {
                w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
                w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limiter.maxReqs))
                w.Header().Set("X-RateLimit-Remaining", "0")
                http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
                return
            }
            
            next.ServeHTTP(w, r)
        })
    }
}
```

## Development

### Running Tests

```bash
# Run all tests
go test -v ./...

# Run with race detection
go test -race -v ./...

# Run specific test
go test -v -run TestBasicRateLimit
```

### Linting

The project uses standard Go tools for code quality:

```bash
# Check for common issues
go vet ./...

# Check formatting
gofmt -s -l .

# Auto-format code
gofmt -s -w .
```

### CI/CD

The GitHub Actions workflow automatically runs:
- `go vet` for static analysis
- `gofmt` for code formatting checks
- Full test suite across Go versions 1.19-1.22
- Redis integration tests

## Performance

- **Non-blocking**: No locks in the hot path
- **Memory efficient**: Configurable cache size and TTL
- **Fast**: Optimized hash functions and minimal allocations
- **Scalable**: Redis backend supports distributed deployments
