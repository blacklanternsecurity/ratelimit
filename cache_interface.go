package ratelimit

// Cache represents a key-value cache interface for rate limiting
type Cache interface {
	// Get retrieves a value from the cache
	// Returns the ClientLimiter if found, nil if not found
	Get(key uint64) *ClientLimiter
	
	// Set stores a value in the cache
	// The value will be stored with the specified key
	Set(key uint64, value *ClientLimiter)
	
	// Delete removes a value from the cache
	// If the key doesn't exist, this is a no-op
	Delete(key uint64)
	
	// Close cleans up any resources used by the cache
	// Returns an error if cleanup fails
	Close() error
}
