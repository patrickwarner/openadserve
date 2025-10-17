// Package ratelimit implements token bucket rate limiting for ad server line items.
//
// The token bucket algorithm allows for burst traffic up to the bucket capacity
// while maintaining a sustained rate limit over time. This is ideal for ad serving
// where traffic can be bursty but needs to be controlled to prevent system overload.
package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a thread-safe token bucket rate limiter.
//
// The bucket has a fixed capacity and refills at a constant rate.
// Each request consumes one token. When the bucket is empty,
// requests are rejected until tokens refill.
//
// Example usage:
//
//	bucket := NewTokenBucket(100, 10) // 100 burst capacity, 10 tokens/second
//	if bucket.Allow() {
//	    // Process request
//	} else {
//	    // Rate limited - reject request
//	}
type TokenBucket struct {
	capacity   int        // Maximum number of tokens the bucket can hold
	tokens     int        // Current number of tokens in the bucket
	refillRate int        // Number of tokens added per second
	lastRefill time.Time  // Last time tokens were added to the bucket
	mu         sync.Mutex // Protects all bucket state
	hitCount   int64      // Number of requests that were rate limited
	totalCount int64      // Total number of requests processed
}

// NewTokenBucket creates a new token bucket with the specified capacity and refill rate.
//
// Parameters:
//   - capacity: Maximum number of tokens the bucket can hold (burst allowance)
//   - refillRate: Number of tokens added per second (sustained rate limit)
//
// The bucket starts full (with capacity tokens available).
func NewTokenBucket(capacity, refillRate int) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow attempts to consume one token from the bucket.
//
// Returns true if a token was available and consumed (request allowed).
// Returns false if no tokens are available (request should be rate limited).
//
// This method is thread-safe and includes automatic token refilling based
// on elapsed time since the last refill.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.totalCount++

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)

	tokensToAdd := int(elapsed.Seconds() * float64(tb.refillRate))
	if tokensToAdd > 0 {
		tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
		tb.lastRefill = now
	}

	// Try to consume a token
	if tb.tokens > 0 {
		tb.tokens--
		return true
	}

	// No tokens available - rate limit hit
	tb.hitCount++
	return false
}

// Stats returns the current rate limiting statistics.
//
// Returns:
//   - hits: Number of requests that were rate limited (blocked)
//   - total: Total number of requests processed by this bucket
//
// This method is thread-safe.
func (tb *TokenBucket) Stats() (hits, total int64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.hitCount, tb.totalCount
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
