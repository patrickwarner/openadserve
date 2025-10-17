package optimization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/observability"
	"go.uber.org/zap"
)

func TestCTRPredictionClient_GetPrediction(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/predict" {
			t.Errorf("Expected path /predict, got %s", r.URL.Path)
		}

		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// Parse request
		var req PredictionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
			return
		}

		// Verify request fields
		if req.LineItemID != 123 {
			t.Errorf("Expected LineItemID 123, got %d", req.LineItemID)
		}

		// Return test response
		resp := PredictionResponse{
			LineItemID:      123,
			CTRScore:        0.025,
			Confidence:      0.8,
			BoostMultiplier: 2.5,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client
	logger := zap.NewNop()
	client := NewCTRPredictionClient(server.URL, 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

	// Test request
	req := &PredictionRequest{
		LineItemID: 123,
		DeviceType: "mobile",
		Country:    "US",
		HourOfDay:  14,
		DayOfWeek:  2,
	}

	// Make prediction
	ctx := context.Background()
	resp, err := client.GetPrediction(ctx, req)
	if err != nil {
		t.Fatalf("GetPrediction failed: %v", err)
	}

	// Verify response
	if resp.LineItemID != 123 {
		t.Errorf("Expected LineItemID 123, got %d", resp.LineItemID)
	}

	if resp.CTRScore != 0.025 {
		t.Errorf("Expected CTRScore 0.025, got %f", resp.CTRScore)
	}

	if resp.Confidence != 0.8 {
		t.Errorf("Expected Confidence 0.8, got %f", resp.Confidence)
	}

	if resp.BoostMultiplier != 2.5 {
		t.Errorf("Expected BoostMultiplier 2.5, got %f", resp.BoostMultiplier)
	}
}

func TestCTRPredictionClient_GetPrediction_ServiceUnavailable(t *testing.T) {
	// Create client with invalid URL
	logger := zap.NewNop()
	client := NewCTRPredictionClient("http://invalid-url:9999", 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

	// Test request
	req := &PredictionRequest{
		LineItemID: 123,
		DeviceType: "mobile",
		Country:    "US",
		HourOfDay:  14,
		DayOfWeek:  2,
	}

	// Make prediction - should return default values
	ctx := context.Background()
	resp, err := client.GetPrediction(ctx, req)
	if err != nil {
		t.Fatalf("GetPrediction should not fail when service unavailable: %v", err)
	}

	// Verify default response
	if resp.LineItemID != 123 {
		t.Errorf("Expected LineItemID 123, got %d", resp.LineItemID)
	}

	if resp.CTRScore != 0.01 {
		t.Errorf("Expected default CTRScore 0.01, got %f", resp.CTRScore)
	}

	if resp.Confidence != 0.5 {
		t.Errorf("Expected default Confidence 0.5, got %f", resp.Confidence)
	}

	if resp.BoostMultiplier != 1.0 {
		t.Errorf("Expected default BoostMultiplier 1.0, got %f", resp.BoostMultiplier)
	}
}

func TestCTRPredictionClient_Cache(t *testing.T) {
	callCount := 0

	// Create test server that counts calls
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		resp := PredictionResponse{
			LineItemID:      123,
			CTRScore:        0.025,
			Confidence:      0.8,
			BoostMultiplier: 2.5,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client
	logger := zap.NewNop()
	client := NewCTRPredictionClient(server.URL, 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

	// Test request
	req := &PredictionRequest{
		LineItemID: 123,
		DeviceType: "mobile",
		Country:    "US",
		HourOfDay:  14,
		DayOfWeek:  2,
	}

	ctx := context.Background()

	// First call - should hit the service
	resp1, err := client.GetPrediction(ctx, req)
	if err != nil {
		t.Fatalf("First GetPrediction failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 service call, got %d", callCount)
	}

	// Second call - should use cache
	resp2, err := client.GetPrediction(ctx, req)
	if err != nil {
		t.Fatalf("Second GetPrediction failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 service call (cached), got %d", callCount)
	}

	// Verify both responses are identical
	if resp1.BoostMultiplier != resp2.BoostMultiplier {
		t.Errorf("Cached response differs from original")
	}
}

func TestCTRPredictionClient_CacheExpiration(t *testing.T) {
	callCount := 0

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		resp := PredictionResponse{
			LineItemID:      123,
			CTRScore:        0.025,
			Confidence:      0.8,
			BoostMultiplier: 2.5,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("Failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create client
	logger := zap.NewNop()
	client := NewCTRPredictionClient(server.URL, 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

	// Test request
	req := &PredictionRequest{
		LineItemID: 123,
		DeviceType: "mobile",
		Country:    "US",
		HourOfDay:  14,
		DayOfWeek:  2,
	}

	ctx := context.Background()

	// First call
	_, err := client.GetPrediction(ctx, req)
	if err != nil {
		t.Fatalf("First GetPrediction failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 service call, got %d", callCount)
	}

	// Manually expire cache entry
	cacheKey := client.cacheKey(req)
	client.cacheMu.Lock()
	if cached, exists := client.cache[cacheKey]; exists {
		cached.TTL = 1 * time.Nanosecond                    // Very short TTL
		cached.Timestamp = time.Now().Add(-1 * time.Second) // Make it expired
	}
	client.cacheMu.Unlock()

	// Second call - should hit service again due to expiration
	_, err = client.GetPrediction(ctx, req)
	if err != nil {
		t.Fatalf("Second GetPrediction failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("Expected 2 service calls (cache expired), got %d", callCount)
	}
}

func TestCTRPredictionClient_HealthCheck(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("Expected path /health, got %s", r.URL.Path)
		}

		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "healthy"}); err != nil {
			t.Errorf("Failed to encode health response: %v", err)
		}
	}))
	defer server.Close()

	// Create client
	logger := zap.NewNop()
	client := NewCTRPredictionClient(server.URL, 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

	// Test health check
	ctx := context.Background()
	err := client.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestCTRPredictionClient_CacheStats(t *testing.T) {
	logger := zap.NewNop()
	client := NewCTRPredictionClient("http://localhost:8000", 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

	// Initially empty cache
	stats := client.GetCacheStats()
	if stats["total_entries"] != 0 {
		t.Errorf("Expected 0 total entries, got %v", stats["total_entries"])
	}

	// Add some cache entries manually
	client.cacheMu.Lock()
	client.cache["key1"] = &CachedPrediction{
		Response:  &PredictionResponse{},
		Timestamp: time.Now(),
		TTL:       5 * time.Minute,
	}
	client.cache["key2"] = &CachedPrediction{
		Response:  &PredictionResponse{},
		Timestamp: time.Now().Add(-10 * time.Minute), // Expired
		TTL:       5 * time.Minute,
	}
	client.cacheMu.Unlock()

	// Check stats
	stats = client.GetCacheStats()
	if stats["total_entries"] != 2 {
		t.Errorf("Expected 2 total entries, got %v", stats["total_entries"])
	}

	if stats["expired_entries"] != 1 {
		t.Errorf("Expected 1 expired entry, got %v", stats["expired_entries"])
	}

	if stats["active_entries"] != 1 {
		t.Errorf("Expected 1 active entry, got %v", stats["active_entries"])
	}
}

func TestCTRPredictionClient_CleanupExpiredCache(t *testing.T) {
	logger := zap.NewNop()
	client := NewCTRPredictionClient("http://localhost:8000", 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

	// Add cache entries
	client.cacheMu.Lock()
	client.cache["active"] = &CachedPrediction{
		Response:  &PredictionResponse{},
		Timestamp: time.Now(),
		TTL:       5 * time.Minute,
	}
	client.cache["expired"] = &CachedPrediction{
		Response:  &PredictionResponse{},
		Timestamp: time.Now().Add(-10 * time.Minute), // Expired
		TTL:       5 * time.Minute,
	}
	client.cacheMu.Unlock()

	// Cleanup
	client.CleanupExpiredCache()

	// Check that only active entry remains
	client.cacheMu.RLock()
	if len(client.cache) != 1 {
		t.Errorf("Expected 1 cache entry after cleanup, got %d", len(client.cache))
	}

	if _, exists := client.cache["active"]; !exists {
		t.Errorf("Expected active entry to remain")
	}

	if _, exists := client.cache["expired"]; exists {
		t.Errorf("Expected expired entry to be removed")
	}
	client.cacheMu.RUnlock()
}
