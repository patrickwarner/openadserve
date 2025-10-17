package logic

import (
	"fmt"
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/models"
)

func testBatchConfig() config.Config {
	return config.Config{
		PIDKp: 0.3,
		PIDKi: 0.05,
		PIDKd: 0.1,
	}
}

func TestBatchFrequencyCheck(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 101, CampaignID: 101, PublisherID: 1, FrequencyCap: 3, FrequencyWindow: DefaultFrequencyWindow, Active: true},
		{ID: 102, CampaignID: 102, PublisherID: 1, FrequencyCap: 5, FrequencyWindow: DefaultFrequencyWindow, Active: true},
		{ID: 103, CampaignID: 103, PublisherID: 1, FrequencyCap: 1, FrequencyWindow: DefaultFrequencyWindow, Active: true},
		{ID: 104, CampaignID: 104, PublisherID: 1, FrequencyCap: 0, FrequencyWindow: DefaultFrequencyWindow, Active: true}, // Uses default cap
	})

	userID := "test_user_batch"
	creatives := []models.Creative{
		{ID: 201, LineItemID: 101, PublisherID: 1, Width: 300, Height: 250},
		{ID: 202, LineItemID: 102, PublisherID: 1, Width: 300, Height: 250},
		{ID: 203, LineItemID: 103, PublisherID: 1, Width: 300, Height: 250},
		{ID: 204, LineItemID: 104, PublisherID: 1, Width: 300, Height: 250}, // Uses default cap
		{ID: 205, LineItemID: 999, PublisherID: 1, Width: 300, Height: 250}, // Line item doesn't exist
	}

	// Pre-populate frequency counts
	testCases := map[int]int64{
		101: 2, // Under cap (3)
		102: 5, // At cap (5)
		103: 1, // At cap (1)
		104: 2, // Under default cap (3)
		999: 4, // Over default cap (3) for non-existent line item
	}

	for lineItemID, count := range testCases {
		key := fmt.Sprintf("freqcap:%s:%d", userID, lineItemID)
		if err := store.Client.Set(store.Ctx, key, count, 0).Err(); err != nil {
			t.Fatalf("failed to set frequency count for line item %d: %v", lineItemID, err)
		}
	}

	// Test batched frequency checking
	result, err := BatchFrequencyCheck(store, userID, creatives, testDataStore)
	if err != nil {
		t.Fatalf("BatchFrequencyCheck failed: %v", err)
	}

	// Verify results
	expectedResults := map[string]bool{
		"1_101": false, // 2 < 3 (not exceeded)
		"1_102": true,  // 5 >= 5 (exceeded)
		"1_103": true,  // 1 >= 1 (exceeded)
		"1_104": false, // 2 < 3 (default cap, not exceeded)
		"1_999": true,  // 4 >= 3 (default cap, exceeded)
	}

	if len(result) != len(expectedResults) {
		t.Fatalf("expected %d results, got %d", len(expectedResults), len(result))
	}

	for key, expected := range expectedResults {
		if actual, exists := result[key]; !exists {
			t.Errorf("missing result for key %s", key)
		} else if actual != expected {
			t.Errorf("key %s: expected %v, got %v", key, expected, actual)
		}
	}
}

func TestBatchPacingCheck(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	// Fix time to noon (50% through day)
	fixed := time.Date(2025, 5, 24, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	defer func() { nowFn = time.Now }()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 201, CampaignID: 201, PublisherID: 2, DailyImpressionCap: 100, PaceType: models.PacingASAP, Active: true},
		{ID: 202, CampaignID: 202, PublisherID: 2, DailyImpressionCap: 100, PaceType: models.PacingEven, Active: true},
		{ID: 203, CampaignID: 203, PublisherID: 2, DailyImpressionCap: 100, PaceType: models.PacingPID, Active: true}, // Should be handled individually
		{ID: 204, CampaignID: 204, PublisherID: 2, DailyImpressionCap: 50, DailyClickCap: 5, PaceType: models.PacingASAP, Active: true},
		{ID: 205, CampaignID: 205, PublisherID: 2, DailyImpressionCap: 0, PaceType: models.PacingASAP, Active: true}, // Unlimited
	})

	creatives := []models.Creative{
		{ID: 301, LineItemID: 201, PublisherID: 2, Width: 300, Height: 250}, // ASAP pacing
		{ID: 302, LineItemID: 202, PublisherID: 2, Width: 300, Height: 250}, // Even pacing
		{ID: 303, LineItemID: 203, PublisherID: 2, Width: 300, Height: 250}, // PID pacing
		{ID: 304, LineItemID: 204, PublisherID: 2, Width: 300, Height: 250}, // With click cap
		{ID: 305, LineItemID: 205, PublisherID: 2, Width: 300, Height: 250}, // Unlimited
	}

	today := fixed.Format("2006-01-02")

	// Set up pacing counts
	pacingData := map[int]int64{
		201: 50,  // Under ASAP cap (100)
		202: 40,  // Under even target (50 at noon)
		203: 60,  // Over PID target (50 at noon)
		204: 30,  // Under ASAP cap (50)
		205: 999, // Unlimited, should always pass
	}

	for lineItemID, count := range pacingData {
		key := fmt.Sprintf("pacing:serves:%d:%s", lineItemID, today)
		if err := store.Client.Set(store.Ctx, key, count, 0).Err(); err != nil {
			t.Fatalf("failed to set pacing count for line item %d: %v", lineItemID, err)
		}
	}

	// Set up click count for line item 204 (at cap)
	clickKey := fmt.Sprintf("clicks:lineitem:204:%s", today)
	if err := store.Client.Set(store.Ctx, clickKey, 5, 0).Err(); err != nil {
		t.Fatalf("failed to set click count: %v", err)
	}

	// Test batched pacing checking
	result, err := BatchPacingCheck(store, creatives, testDataStore, testBatchConfig())
	if err != nil {
		t.Fatalf("BatchPacingCheck failed: %v", err)
	}

	// Verify results
	expectedResults := map[string]bool{
		"2_201": true,  // ASAP: 50 < 100 (eligible)
		"2_202": true,  // Even: 40 < 50 (allowed at noon, eligible)
		"2_203": false, // PID: should be blocked (handled individually)
		"2_204": false, // ASAP with click cap: clicks at limit (5 >= 5)
		"2_205": true,  // Unlimited impressions (eligible)
	}

	if len(result) != len(expectedResults) {
		t.Fatalf("expected %d results, got %d", len(expectedResults), len(result))
	}

	for key, expected := range expectedResults {
		if actual, exists := result[key]; !exists {
			t.Errorf("missing result for key %s", key)
		} else if actual != expected {
			t.Errorf("key %s: expected %v, got %v", key, expected, actual)
		}
	}
}

func TestBatchPacingCheck_PIDSeparation(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	fixed := time.Date(2025, 5, 24, 6, 0, 0, 0, time.UTC) // 25% through day
	nowFn = func() time.Time { return fixed }
	defer func() { nowFn = time.Now }()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 301, CampaignID: 301, PublisherID: 3, DailyImpressionCap: 100, PaceType: models.PacingASAP, Active: true},
		{ID: 302, CampaignID: 302, PublisherID: 3, DailyImpressionCap: 100, PaceType: models.PacingPID, Active: true},
		{ID: 303, CampaignID: 303, PublisherID: 3, DailyImpressionCap: 100, PaceType: models.PacingPID, Active: true},
	})

	creatives := []models.Creative{
		{ID: 401, LineItemID: 301, PublisherID: 3, Width: 300, Height: 250}, // ASAP (batchable)
		{ID: 402, LineItemID: 302, PublisherID: 3, Width: 300, Height: 250}, // PID (individual)
		{ID: 403, LineItemID: 303, PublisherID: 3, Width: 300, Height: 250}, // PID (individual)
	}

	today := fixed.Format("2006-01-02")

	// Set up counts - PID creatives should be under target at 6AM (25 expected)
	pacingData := map[int]int64{
		301: 20, // ASAP: under cap
		302: 20, // PID: under target (should allow)
		303: 30, // PID: over target (should block)
	}

	for lineItemID, count := range pacingData {
		key := fmt.Sprintf("pacing:serves:%d:%s", lineItemID, today)
		if err := store.Client.Set(store.Ctx, key, count, 0).Err(); err != nil {
			t.Fatalf("failed to set pacing count for line item %d: %v", lineItemID, err)
		}
	}

	result, err := BatchPacingCheck(store, creatives, testDataStore, testBatchConfig())
	if err != nil {
		t.Fatalf("BatchPacingCheck failed: %v", err)
	}

	// Verify that PID creatives are handled correctly
	expectedResults := map[string]bool{
		"3_301": true,  // ASAP: handled in batch, under cap
		"3_302": true,  // PID: handled individually, under target
		"3_303": false, // PID: handled individually, over target
	}

	for key, expected := range expectedResults {
		if actual, exists := result[key]; !exists {
			t.Errorf("missing result for key %s", key)
		} else if actual != expected {
			t.Errorf("key %s: expected %v, got %v", key, expected, actual)
		}
	}
}

func TestBatchingEmptyArrays(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()

	t.Run("empty creatives array - frequency", func(t *testing.T) {
		result, err := BatchFrequencyCheck(store, "user123", []models.Creative{}, testDataStore)
		if err != nil {
			t.Fatalf("BatchFrequencyCheck with empty array failed: %v", err)
		}

		if result == nil {
			t.Error("expected non-nil result map")
		}

		if len(result) != 0 {
			t.Errorf("expected empty result map, got %d entries", len(result))
		}
	})

	t.Run("empty creatives array - pacing", func(t *testing.T) {
		result, err := BatchPacingCheck(store, []models.Creative{}, testDataStore, testBatchConfig())
		if err != nil {
			t.Fatalf("BatchPacingCheck with empty array failed: %v", err)
		}

		if result == nil {
			t.Error("expected non-nil result map")
		}

		if len(result) != 0 {
			t.Errorf("expected empty result map, got %d entries", len(result))
		}
	})

	t.Run("nil store - frequency", func(t *testing.T) {
		creatives := []models.Creative{
			{ID: 1, LineItemID: 1, PublisherID: 1},
		}

		result, err := BatchFrequencyCheck(nil, "user123", creatives, testDataStore)
		if err != ErrNilRedisStore {
			t.Fatalf("expected ErrNilRedisStore, got %v", err)
		}

		if result != nil {
			t.Error("expected nil result with nil store")
		}
	})

	t.Run("nil store - pacing", func(t *testing.T) {
		creatives := []models.Creative{
			{ID: 1, LineItemID: 1, PublisherID: 1},
		}

		result, err := BatchPacingCheck(nil, creatives, testDataStore, testBatchConfig())
		if err != ErrNilRedisStore {
			t.Fatalf("expected ErrNilRedisStore, got %v", err)
		}

		if result != nil {
			t.Error("expected nil result with nil store")
		}
	})

	t.Run("missing line items", func(t *testing.T) {
		// Test with creatives that reference non-existent line items
		creatives := []models.Creative{
			{ID: 501, LineItemID: 999, PublisherID: 5}, // Non-existent line item
			{ID: 502, LineItemID: 998, PublisherID: 5}, // Non-existent line item
		}

		// Frequency check with missing line items should use default cap
		freqResult, err := BatchFrequencyCheck(store, "user123", creatives, testDataStore)
		if err != nil {
			t.Fatalf("BatchFrequencyCheck with missing line items failed: %v", err)
		}

		// Should return results (using default frequency cap)
		if len(freqResult) != 2 {
			t.Errorf("expected 2 frequency results, got %d", len(freqResult))
		}

		// Pacing check with missing line items should allow by default
		pacingResult, err := BatchPacingCheck(store, creatives, testDataStore, testBatchConfig())
		if err != nil {
			t.Fatalf("BatchPacingCheck with missing line items failed: %v", err)
		}

		if len(pacingResult) != 2 {
			t.Errorf("expected 2 pacing results, got %d", len(pacingResult))
		}

		// Missing line items should be allowed (no pacing restrictions)
		for key, allowed := range pacingResult {
			if !allowed {
				t.Errorf("expected missing line item %s to be allowed, got blocked", key)
			}
		}
	})

	t.Run("redis pipeline errors", func(t *testing.T) {
		// Close the Redis store to force pipeline errors
		ms.Close()

		creatives := []models.Creative{
			{ID: 601, LineItemID: 1, PublisherID: 6},
		}

		// Both batch functions should handle Redis errors gracefully
		_, err := BatchFrequencyCheck(store, "user123", creatives, testDataStore)
		if err == nil {
			t.Error("expected error from BatchFrequencyCheck with closed Redis")
		}

		_, err = BatchPacingCheck(store, creatives, testDataStore, testBatchConfig())
		if err == nil {
			t.Error("expected error from BatchPacingCheck with closed Redis")
		}
	})
}
