package ratelimit

import (
	"fmt"
	"sync"

	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
)

// LineItemLimiter manages rate limiting for multiple line items.
//
// Each line item gets its own token bucket, created lazily on first access.
// The limiter integrates with an injected metrics registry to track rate limiting activity.
//
// Example usage:
//
//	config := Config{Capacity: 100, RefillRate: 10, Enabled: true}
//	metrics := observability.NewPrometheusRegistry()
//	limiter := NewLineItemLimiter(config, metrics)
//
//	if limiter.Allow("line-item-123") {
//	    // Process ad request for line item 123
//	} else {
//	    // Line item 123 is rate limited
//	}
type LineItemLimiter struct {
	buckets map[string]*TokenBucket       // Map of line item ID to token bucket
	mu      sync.RWMutex                  // Protects the buckets map
	config  Config                        // Rate limiting configuration
	metrics observability.MetricsRegistry // Metrics registry for tracking rate limiting activity
}

// Config holds the configuration for rate limiting.
type Config struct {
	Capacity   int  // Token bucket capacity (burst allowance)
	RefillRate int  // Tokens added per second (sustained rate)
	Enabled    bool // Whether rate limiting is active
}

// NewLineItemLimiter creates a new line item rate limiter with the given configuration.
func NewLineItemLimiter(config Config, metrics observability.MetricsRegistry) *LineItemLimiter {
	return &LineItemLimiter{
		buckets: make(map[string]*TokenBucket),
		config:  config,
		metrics: metrics,
	}
}

// Allow checks if a request for the given line item should be allowed.
//
// Parameters:
//   - lineItemID: String identifier of the line item
//
// Returns:
//   - true if the request should be allowed (token available)
//   - false if the request should be rate limited (no tokens available)
//
// If rate limiting is disabled via config, this method always returns true.
// The method automatically creates token buckets for new line items and
// updates metrics via the injected registry for monitoring.
func (lil *LineItemLimiter) Allow(lineItemID string) bool {
	if !lil.config.Enabled {
		return true
	}

	// Update metrics for monitoring
	lil.metrics.IncrementRateLimitRequests(lineItemID)

	// Get or create token bucket for this line item
	lil.mu.RLock()
	bucket, exists := lil.buckets[lineItemID]
	lil.mu.RUnlock()

	if !exists {
		// Double-checked locking pattern to avoid race conditions
		lil.mu.Lock()
		bucket, exists = lil.buckets[lineItemID]
		if !exists {
			bucket = NewTokenBucket(lil.config.Capacity, lil.config.RefillRate)
			lil.buckets[lineItemID] = bucket
		}
		lil.mu.Unlock()
	}

	// Check if request is allowed
	allowed := bucket.Allow()
	if !allowed {
		lil.metrics.IncrementRateLimitHits(lineItemID)
	}

	return allowed
}

// GetStats returns rate limiting statistics for all line items.
//
// Returns a map where keys are line item IDs and values contain
// statistics about rate limiting activity for that line item.
//
// This method is thread-safe and provides a snapshot of current statistics.
func (lil *LineItemLimiter) GetStats() map[string]RateLimitStats {
	lil.mu.RLock()
	defer lil.mu.RUnlock()

	stats := make(map[string]RateLimitStats)
	for lineItemID, bucket := range lil.buckets {
		hits, total := bucket.Stats()
		hitRate := 0.0
		if total > 0 {
			hitRate = float64(hits) / float64(total)
		}
		stats[lineItemID] = RateLimitStats{
			LineItemID: lineItemID,
			Hits:       hits,
			Total:      total,
			HitRate:    hitRate,
		}
	}

	return stats
}

// RateLimitStats contains statistics about rate limiting for a single line item.
type RateLimitStats struct {
	LineItemID string  `json:"LineItemID"` // Line item identifier
	Hits       int64   `json:"Hits"`       // Number of rate limited requests
	Total      int64   `json:"Total"`      // Total number of requests processed
	HitRate    float64 `json:"HitRate"`    // Percentage of requests rate limited (0.0-1.0)
}

// String returns a human-readable representation of the rate limit statistics.
func (rls RateLimitStats) String() string {
	return fmt.Sprintf("LineItem %s: %d/%d hits (%.2f%%)",
		rls.LineItemID, rls.Hits, rls.Total, rls.HitRate*100)
}

// ShouldRateLimit determines if a line item should be subject to rate limiting.
//
// Currently, only direct line items are rate limited. Programmatic line items
// are excluded since they already have external rate limiting mechanisms.
func (lil *LineItemLimiter) ShouldRateLimit(lineItem *models.LineItem) bool {
	return lineItem.Type == models.LineItemTypeDirect
}
