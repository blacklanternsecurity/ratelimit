package ratelimit

import (
	"hash/fnv"
	"math"
	"net"
	"strings"
	"sync"
	"time"
)

/*

look, I don't care what it's called, bucket, leaky, token, windows, sliding whatever. do this. Each limiter is a float representing requests allowed to be made, and a timestamp representing when the last request was made. okay? Now, based on the window and the allowed requests per window, when a request is made, first thing we do is look at the timestamp and based on how long it's been since the last time we looked at it, we replenish (increment) its allowed requests up to a max (the burst). Since a request is being made, we decrement the float. If the result is negative, we deny with a retry-after calcuated from the negative quota and the window and the allowed requests per window, so that if the client sleeps for that time and tries next time, they will have been replenished up to 1 request. k?

*/

type Config struct {
	Capacity           int
	WindowSize         time.Duration
	MaxReqs            int
	Expiration         time.Duration
	IPv4SubnetMask     int
	IPv6SubnetMask     int
	IncludeSource      bool
	IncludeDestination bool
}

type Limiter struct {
	cache              Cache
	windowSize         time.Duration
	maxReqs            int
	ipv4SubnetMask     int
	ipv6SubnetMask     int
	includeSource      bool
	includeDestination bool
}

type ClientLimiter struct {
	mu              sync.Mutex
	allowedRequests float64
	lastRequest     time.Time
}

func New(config Config) *Limiter {
	cache := NewMemoryCache(config.Capacity, config.Expiration)
	
	return &Limiter{
		cache:              cache,
		windowSize:         config.WindowSize,
		maxReqs:            config.MaxReqs,
		ipv4SubnetMask:     config.IPv4SubnetMask,
		ipv6SubnetMask:     config.IPv6SubnetMask,
		includeSource:      config.IncludeSource,
		includeDestination: config.IncludeDestination,
	}
}

// NewWithCache creates a limiter with a custom cache implementation
func NewWithCache(config Config, cache Cache) *Limiter {
	return &Limiter{
		cache:              cache,
		windowSize:         config.WindowSize,
		maxReqs:            config.MaxReqs,
		ipv4SubnetMask:     config.IPv4SubnetMask,
		ipv6SubnetMask:     config.IPv6SubnetMask,
		includeSource:      config.IncludeSource,
		includeDestination: config.IncludeDestination,
	}
}

// NewWithRedis creates a limiter with Redis cache
func NewWithRedis(config Config, redisConfig RedisConfig) (*Limiter, error) {
	redisCache, err := NewRedisCache(redisConfig)
	if err != nil {
		return nil, err
	}
	
	return NewWithCache(config, redisCache), nil
}

// NewDefault creates a limiter with sensible defaults
func NewDefault() *Limiter {
	config := Config{
		Capacity:           100,
		WindowSize:         1 * time.Second,
		MaxReqs:            10,
		Expiration:         60 * time.Second,
		IPv4SubnetMask:     32, // No IPv4 squashing
		IPv6SubnetMask:     56, // /56 IPv6 squashing
		IncludeSource:      true,
		IncludeDestination: true,
	}
	return New(config)
}


// normalizeIP normalizes an IP address for rate limiting using configurable subnet masks
func (l *Limiter) normalizeIP(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		// If parsing fails, return the original string
		return ipStr
	}

	if ip.To4() != nil {
		// IPv4 address - apply configured subnet mask
		if l.ipv4SubnetMask == 32 {
			return ip.String() // No squashing
		}
		ipv4Mask := net.CIDRMask(l.ipv4SubnetMask, 32)
		subnet := ip.Mask(ipv4Mask)
		return subnet.String()
	}

	// IPv6 address - apply configured subnet mask
	if l.ipv6SubnetMask == 128 {
		return ip.String() // No squashing
	}
	ipv6Mask := net.CIDRMask(l.ipv6SubnetMask, 128)
	subnet := ip.Mask(ipv6Mask)
	return subnet.String()
}

// normalizeDestination normalizes destination (hostname) to lowercase for case-insensitive matching
func normalizeDestination(destination string) string {
	return strings.ToLower(destination)
}

// createCacheKey creates a composite cache key from normalized IP, destination, and identifier
func (l *Limiter) createCacheKey(normalizedIP, destination, identifier string) uint64 {
	// Use FNV-1a hash for fast, non-cryptographic hashing
	h := fnv.New64a()
	
	if l.includeSource {
		h.Write([]byte(normalizedIP))
		h.Write([]byte("|"))
	}
	
	if l.includeDestination {
		h.Write([]byte(destination))
		h.Write([]byte("|"))
	}
	
	h.Write([]byte(identifier))
	return h.Sum64()
}

func (l *Limiter) IsAllowed(ip, destination, identifier string, maxReqs ...int) int {
	now := time.Now()

	// Normalize the IP address and destination
	normalizedIP := l.normalizeIP(ip)
	normalizedDestination := normalizeDestination(destination)

	// Create composite cache key with normalized IP, destination, and identifier
	cacheKey := l.createCacheKey(normalizedIP, normalizedDestination, identifier)

	// Determine the rate limit
	rateLimit := l.maxReqs
	if len(maxReqs) > 0 && maxReqs[0] > 0 {
		rateLimit = maxReqs[0]
	}

	// Get or create limiter
	clientLimiter := l.cache.Get(cacheKey)
	if clientLimiter == nil {
		// Entry expired or doesn't exist - create new limiter
		clientLimiter = &ClientLimiter{
			allowedRequests: float64(rateLimit),
			lastRequest:     now, // Use the current time instead of time.Now() to avoid timing issues
		}
		l.cache.Set(cacheKey, clientLimiter)
	}

	// Rate limiting logic
	clientLimiter.mu.Lock()
	defer clientLimiter.mu.Unlock()

	// Calculate replenish rate (requests per second)
	replenishRate := float64(rateLimit) / l.windowSize.Seconds()

	// Calculate time since last request and replenish
	timeElapsed := now.Sub(clientLimiter.lastRequest)
	replenished := timeElapsed.Seconds() * replenishRate
	clientLimiter.allowedRequests += replenished
	clientLimiter.lastRequest = now

	// Cap at max burst (but allow negative values)
	if clientLimiter.allowedRequests > float64(rateLimit) {
		clientLimiter.allowedRequests = float64(rateLimit)
	}

	clientLimiter.allowedRequests--

	// Store the updated state back to cache
	l.cache.Set(cacheKey, clientLimiter)

	// Check if we have quota
	if clientLimiter.allowedRequests >= 0.0 {
		return 0 // Allowed
	}

	// Request is blocked - calculate retry-after based on negative quota
	// This is how many seconds we need to get back to 1.0 allowed requests
	timeToReplenish := -(clientLimiter.allowedRequests - 1.0) / replenishRate
	retryAfter := int(math.Ceil(timeToReplenish))

	return retryAfter
}

