package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	bucket := NewTokenBucket(5, 1) // 5 tokens, refill 1 per second

	// Should allow 5 requests initially
	for i := 0; i < 5; i++ {
		if !bucket.Allow() {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 6th request should be blocked
	if bucket.Allow() {
		t.Error("Expected 6th request to be blocked")
	}

	// Check stats
	hits, total := bucket.Stats()
	if hits != 1 {
		t.Errorf("Expected 1 hit, got %d", hits)
	}
	if total != 6 {
		t.Errorf("Expected 6 total requests, got %d", total)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	bucket := NewTokenBucket(2, 10) // 2 tokens, refill 10 per second

	// Exhaust tokens
	bucket.Allow()
	bucket.Allow()

	// Should be blocked
	if bucket.Allow() {
		t.Error("Expected request to be blocked")
	}

	// Wait and try again (tokens should refill)
	time.Sleep(200 * time.Millisecond) // 200ms = 0.2 seconds * 10 tokens/sec = 2 tokens

	if !bucket.Allow() {
		t.Error("Expected request to be allowed after refill")
	}
}
