package ratelimit

import (
	"testing"
	"time"
)

func TestNormalizeIP(t *testing.T) {
	// Test with default configuration (IPv4 no squashing, IPv6 /56)
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 56,
	}
	limiter := New(config)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "IPv4 address - no squashing",
			input:    "192.168.1.1",
			expected: "192.168.1.1",
		},
		{
			name:     "IPv6 address - should collapse to /56",
			input:    "2001:db8:85a3:8d3:1319:8a2e:370:7344",
			expected: "2001:db8:85a3:800::",
		},
		{
			name:     "IPv6 address with leading zeros",
			input:    "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			expected: "2001:db8:85a3::",
		},
		{
			name:     "IPv6 loopback",
			input:    "::1",
			expected: "::",
		},
		{
			name:     "Invalid IP",
			input:    "not-an-ip",
			expected: "not-an-ip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := limiter.normalizeIP(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeIP(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLimiterIPv4AndIPv6(t *testing.T) {
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 56,
	}
	limiter := New(config)
	destination := "api.example.com"

	// Test IPv4
	ipv4 := "192.168.1.1"
	if retryAfter := limiter.IsAllowed(ipv4, destination, ""); retryAfter != 0 {
		t.Error("First IPv4 request should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ipv4, destination, ""); retryAfter != 0 {
		t.Error("Second IPv4 request should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ipv4, destination, ""); retryAfter == 0 {
		t.Error("Third IPv4 request should be blocked")
	}

	// Test IPv6 - different addresses in same /56 subnet should be treated as same
	ipv6_1 := "2001:db8:85a3:8d3:1319:8a2e:370:7344"
	ipv6_2 := "2001:db8:85a3:8d3:ffff:ffff:ffff:ffff"

	if retryAfter := limiter.IsAllowed(ipv6_1, destination, ""); retryAfter != 0 {
		t.Error("First IPv6 request should be allowed")
	}
	// This should count as second request for the same /56 subnet
	if retryAfter := limiter.IsAllowed(ipv6_2, destination, ""); retryAfter != 0 {
		t.Error("Second IPv6 request in same /56 should be allowed")
	}
	// This should be blocked as it's the third request for the same /56 subnet
	if retryAfter := limiter.IsAllowed(ipv6_1, destination, ""); retryAfter == 0 {
		t.Error("Third IPv6 request in same /56 should be blocked")
	}
}

func TestLimiterOverrideMaxReqs(t *testing.T) {
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 56,
	}
	limiter := New(config)
	ip := "192.168.1.1"
	ip2 := "192.168.1.2"
	destination := "api.example.com"

	// Test with default limit (2)
	if retryAfter := limiter.IsAllowed(ip, destination, ""); retryAfter != 0 {
		t.Error("First request should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip, destination, ""); retryAfter != 0 {
		t.Error("Second request should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip, destination, ""); retryAfter == 0 {
		t.Error("Third request should be blocked with default limit")
	}

	// Test with overridden limit (1)
	if retryAfter := limiter.IsAllowed(ip2, destination, "", 1); retryAfter != 0 {
		t.Error("First request should be allowed with overridden limit")
	}
	if retryAfter := limiter.IsAllowed(ip2, destination, "", 1); retryAfter == 0 {
		t.Error("Second request should be blocked with overridden limit")
	}
}

func TestLimiterPerDestination(t *testing.T) {
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 56,
	}
	limiter := New(config)
	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"
	dest1 := "api1.example.com"
	dest2 := "api2.example.com"
	apiKey1 := "test-api-key-1"
	apiKey2 := "test-api-key-2"

	// Test 1: Same IP, same destination, no API key - should be rate limited together
	if retryAfter := limiter.IsAllowed(ip1, dest1, ""); retryAfter != 0 {
		t.Error("First request (ip1, dest1, no key) should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest1, ""); retryAfter != 0 {
		t.Error("Second request (ip1, dest1, no key) should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest1, ""); retryAfter == 0 {
		t.Error("Third request (ip1, dest1, no key) should be blocked")
	}

	// Test 2: Same IP, different destination, no API key - should have separate limits
	if retryAfter := limiter.IsAllowed(ip1, dest2, ""); retryAfter != 0 {
		t.Error("First request (ip1, dest2, no key) should be allowed despite dest1 being rate limited")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest2, ""); retryAfter != 0 {
		t.Error("Second request (ip1, dest2, no key) should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest2, ""); retryAfter == 0 {
		t.Error("Third request (ip1, dest2, no key) should be blocked")
	}

	// Test 3: Different IP, same destination, no API key - should have separate limits
	if retryAfter := limiter.IsAllowed(ip2, dest1, ""); retryAfter != 0 {
		t.Error("First request (ip2, dest1, no key) should be allowed despite ip1+dest1 being rate limited")
	}
	if retryAfter := limiter.IsAllowed(ip2, dest1, ""); retryAfter != 0 {
		t.Error("Second request (ip2, dest1, no key) should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip2, dest1, ""); retryAfter == 0 {
		t.Error("Third request (ip2, dest1, no key) should be blocked")
	}

	// Test 4: Same IP, same destination, different API keys - should have separate limits
	if retryAfter := limiter.IsAllowed(ip1, dest1, apiKey1); retryAfter != 0 {
		t.Error("First request (ip1, dest1, apiKey1) should be allowed despite ip1+dest1+no_key being rate limited")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest1, apiKey1); retryAfter != 0 {
		t.Error("Second request (ip1, dest1, apiKey1) should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest1, apiKey1); retryAfter == 0 {
		t.Error("Third request (ip1, dest1, apiKey1) should be blocked")
	}

	// Test 5: Same IP, same destination, different API key - should have separate limits from apiKey1
	if retryAfter := limiter.IsAllowed(ip1, dest1, apiKey2); retryAfter != 0 {
		t.Error("First request (ip1, dest1, apiKey2) should be allowed despite apiKey1 being rate limited")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest1, apiKey2); retryAfter != 0 {
		t.Error("Second request (ip1, dest1, apiKey2) should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ip1, dest1, apiKey2); retryAfter == 0 {
		t.Error("Third request (ip1, dest1, apiKey2) should be blocked")
	}

	// Test 6: Verify all combinations are still properly isolated
	// apiKey1 should still be blocked
	if retryAfter := limiter.IsAllowed(ip1, dest1, apiKey1); retryAfter == 0 {
		t.Error("Fourth request (ip1, dest1, apiKey1) should still be blocked")
	}
	// no key should still be blocked
	if retryAfter := limiter.IsAllowed(ip1, dest1, ""); retryAfter == 0 {
		t.Error("Fourth request (ip1, dest1, no key) should still be blocked")
	}
	// but different IP with same dest and apiKey1 should be allowed
	if retryAfter := limiter.IsAllowed(ip2, dest1, apiKey1); retryAfter != 0 {
		t.Error("First request (ip2, dest1, apiKey1) should be allowed - different IP")
	}
}

func TestLimiterCaseInsensitiveDestination(t *testing.T) {
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 56,
	}
	limiter := New(config)
	ip := "192.168.1.1"

	// Test that different cases of the same hostname are treated as the same destination
	destinations := []string{
		"API.EXAMPLE.COM",
		"api.example.com",
		"Api.Example.Com",
		"aPi.ExAmPlE.cOm",
	}

	// First request should be allowed
	if retryAfter := limiter.IsAllowed(ip, destinations[0], ""); retryAfter != 0 {
		t.Error("First request should be allowed")
	}

	// Second request with different case should be allowed (same destination)
	if retryAfter := limiter.IsAllowed(ip, destinations[1], ""); retryAfter != 0 {
		t.Error("Second request with different case should be allowed")
	}

	// Third request with different case should be blocked (same destination, limit reached)
	if retryAfter := limiter.IsAllowed(ip, destinations[2], ""); retryAfter == 0 {
		t.Error("Third request with different case should be blocked - same destination")
	}

	// Fourth request with different case should also be blocked
	if retryAfter := limiter.IsAllowed(ip, destinations[3], ""); retryAfter == 0 {
		t.Error("Fourth request with different case should be blocked - same destination")
	}
}

func TestNormalizeDestination(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "all lowercase",
			input:    "api.example.com",
			expected: "api.example.com",
		},
		{
			name:     "all uppercase",
			input:    "API.EXAMPLE.COM",
			expected: "api.example.com",
		},
		{
			name:     "mixed case",
			input:    "Api.Example.Com",
			expected: "api.example.com",
		},
		{
			name:     "random case",
			input:    "aPi.ExAmPlE.cOm",
			expected: "api.example.com",
		},
		{
			name:     "localhost",
			input:    "LOCALHOST",
			expected: "localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeDestination(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeDestination(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBasicRateLimit(t *testing.T) {
	// 2 requests per 5 seconds
	config := Config{
		Capacity:       100,
		WindowSize:     5 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 56,
	}
	limiter := New(config)
	ip := "192.168.1.1"
	destination := "api.example.com"

	// First two requests should pass

	// Allowed requests: before 2.0, after 1.0
	retryAfter1 := limiter.IsAllowed(ip, destination, "")
	if retryAfter1 != 0 {
		t.Errorf("Request 1 should be allowed, got retry-after: %d", retryAfter1)
	}

	// Allowed requests: before 1.0, after 0.0
	retryAfter2 := limiter.IsAllowed(ip, destination, "")
	if retryAfter2 != 0 {
		t.Errorf("Request 2 should be allowed, got retry-after: %d", retryAfter2)
	}

	// Third request should be blocked
	// Allowed requests, before: 0.0, after: -1.0
	// Retry-after: math.ceil(5/2 * -(-1.0 - 1.0))) = 5
	retryAfter3 := limiter.IsAllowed(ip, destination, "")
	if retryAfter3 == 0 {
		t.Error("Request 3 should be blocked")
	}
	if retryAfter3 != 5 {
		t.Errorf("Request 3 retry-after should be 5, got %d", retryAfter3)
	}

	// Fourth request should be blocked
	// Allowed requests, before: -1.0, after: -2.0
	// Retry-after: math.ceil(5/2 * -(-2.0 - 1.0))) = 8
	retryAfter4 := limiter.IsAllowed(ip, destination, "")
	if retryAfter4 == 0 {
		t.Error("Request 4 should be blocked")
	}
	if retryAfter4 != 8 {
		t.Errorf("Request 4 retry-after should be 8, got %d", retryAfter4)
	}

	// Fifth request should be blocked
	// Allowed requests, before: -2.0, after: -3.0
	// Retry-after: math.ceil(5/2 * -(-3.0 - 1.0))) = 10
	retryAfter5 := limiter.IsAllowed(ip, destination, "")
	if retryAfter5 == 0 {
		t.Error("Request 5 should be blocked")
	}
	if retryAfter5 != 10 {
		t.Errorf("Request 5 retry-after should be 10, got %d", retryAfter5)
	}

	// Wait for 5 seconds
	// This should replenish the quota from -3.0 to -1.0
	time.Sleep(5 * time.Second)

	// Sixth request should be blocked
	// Allowed requests, before: -1.0, after: -2.0
	// Retry-after: math.ceil(5/2 * -(-2.0 - 1.0))) = 8
	retryAfter6 := limiter.IsAllowed(ip, destination, "")
	if retryAfter6 == 0 {
		t.Error("Request 6 should be blocked")
	}
	if retryAfter6 != 8 {
		t.Errorf("Request 6 retry-after should be 8, got %d", retryAfter6)
	}

	// Sleep for the recommended retry-after
	time.Sleep(time.Duration(retryAfter6) * time.Second)

	// Seventh request should be allowed
	// Allowed requests should be 0.0
	retryAfter7 := limiter.IsAllowed(ip, destination, "")
	if retryAfter7 != 0 {
		t.Errorf("Request 7 should be allowed, got retry-after: %d", retryAfter7)
	}
}

func TestIPv4SubnetSquashing(t *testing.T) {
	// Test IPv4 /24 subnet squashing
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 24,
		IPv6SubnetMask: 56,
	}
	limiter := New(config)
	destination := "api.example.com"

	// Different IPs in same /24 subnet should be treated as same
	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"
	ip3 := "192.168.1.100"

	// First request from first IP should be allowed
	if retryAfter := limiter.IsAllowed(ip1, destination, ""); retryAfter != 0 {
		t.Error("First request from ip1 should be allowed")
	}

	// Second request from different IP in same /24 should be treated as second request
	if retryAfter := limiter.IsAllowed(ip2, destination, ""); retryAfter != 0 {
		t.Error("Second request from ip2 (same /24) should be allowed")
	}

	// Third request from another IP in same /24 should be blocked
	if retryAfter := limiter.IsAllowed(ip3, destination, ""); retryAfter == 0 {
		t.Error("Third request from ip3 (same /24) should be blocked")
	}
}

func TestIPv6SubnetSquashing(t *testing.T) {
	// Test IPv6 /64 subnet squashing (more aggressive than default /56)
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 64,
	}
	limiter := New(config)
	destination := "api.example.com"

	// Different IPs in same /64 subnet should be treated as same
	ip1 := "2001:db8:85a3:8d3:1319:8a2e:370:7344"
	ip2 := "2001:db8:85a3:8d3:ffff:ffff:ffff:ffff"
	ip3 := "2001:db8:85a3:8d4:0000:0000:0000:0001" // Different /64

	// First request from first IP should be allowed
	if retryAfter := limiter.IsAllowed(ip1, destination, ""); retryAfter != 0 {
		t.Error("First request from ip1 should be allowed")
	}

	// Second request from different IP in same /64 should be treated as second request
	if retryAfter := limiter.IsAllowed(ip2, destination, ""); retryAfter != 0 {
		t.Error("Second request from ip2 (same /64) should be allowed")
	}

	// Third request from another IP in same /64 should be blocked
	if retryAfter := limiter.IsAllowed(ip1, destination, ""); retryAfter == 0 {
		t.Error("Third request from ip1 (same /64) should be blocked")
	}

	// But different /64 should have separate limit
	if retryAfter := limiter.IsAllowed(ip3, destination, ""); retryAfter != 0 {
		t.Error("First request from ip3 (different /64) should be allowed")
	}
}

func TestNoSubnetSquashing(t *testing.T) {
	// Test with no subnet squashing (32 for IPv4, 128 for IPv6)
	config := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 128,
	}
	limiter := New(config)
	destination := "api.example.com"

	// Different IPs should have separate limits
	ipv4_1 := "192.168.1.1"
	ipv4_2 := "192.168.1.2"
	ipv6_1 := "2001:db8:85a3:8d3:1319:8a2e:370:7344"
	ipv6_2 := "2001:db8:85a3:8d3:ffff:ffff:ffff:ffff"

	// IPv4 addresses should have separate limits
	if retryAfter := limiter.IsAllowed(ipv4_1, destination, ""); retryAfter != 0 {
		t.Error("First request from ipv4_1 should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ipv4_1, destination, ""); retryAfter != 0 {
		t.Error("Second request from ipv4_1 should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ipv4_1, destination, ""); retryAfter == 0 {
		t.Error("Third request from ipv4_1 should be blocked")
	}

	// Different IPv4 should have separate limit
	if retryAfter := limiter.IsAllowed(ipv4_2, destination, ""); retryAfter != 0 {
		t.Error("First request from ipv4_2 should be allowed")
	}

	// IPv6 addresses should have separate limits
	if retryAfter := limiter.IsAllowed(ipv6_1, destination, ""); retryAfter != 0 {
		t.Error("First request from ipv6_1 should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ipv6_1, destination, ""); retryAfter != 0 {
		t.Error("Second request from ipv6_1 should be allowed")
	}
	if retryAfter := limiter.IsAllowed(ipv6_1, destination, ""); retryAfter == 0 {
		t.Error("Third request from ipv6_1 should be blocked")
	}

	// Different IPv6 should have separate limit
	if retryAfter := limiter.IsAllowed(ipv6_2, destination, ""); retryAfter != 0 {
		t.Error("First request from ipv6_2 should be allowed")
	}
}

func TestNormalizeIPWithDifferentMasks(t *testing.T) {
	// Test IPv4 /24 mask
	config24 := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 24,
		IPv6SubnetMask: 56,
	}
	limiter24 := New(config24)

	// Test IPv6 /64 mask
	config64 := Config{
		Capacity:       100,
		WindowSize:     1 * time.Second,
		MaxReqs:        2,
		Expiration:     60 * time.Second,
		IPv4SubnetMask: 32,
		IPv6SubnetMask: 64,
	}
	limiter64 := New(config64)

	// IPv4 /24 test
	result24 := limiter24.normalizeIP("192.168.1.100")
	expected24 := "192.168.1.0"
	if result24 != expected24 {
		t.Errorf("IPv4 /24 normalization: got %q, want %q", result24, expected24)
	}

	// IPv6 /64 test
	result64 := limiter64.normalizeIP("2001:db8:85a3:8d3:1319:8a2e:370:7344")
	expected64 := "2001:db8:85a3:8d3::"
	if result64 != expected64 {
		t.Errorf("IPv6 /64 normalization: got %q, want %q", result64, expected64)
	}
}
