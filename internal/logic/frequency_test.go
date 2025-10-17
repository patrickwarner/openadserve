package logic

import (
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/models"
	// setupTestRedis is in ad_selector_test.go, which is in the same 'logic' package.
	// "fmt" and "github.com/patrickwarner/openadserve/internal/db" removed as they are unused.
)

func TestHasUserExceededFrequencyCap(t *testing.T) {

	defaultWin := DefaultFrequencyWindow // To ensure consistency if DefaultFrequencyWindow changes

	testCases := []struct {
		name                string
		lineItem            models.LineItem
		userID              string
		lineItemIDToUse     int // For cases where LI might not be found
		impressionsToPreLog int
		expectedResult      bool
	}{
		{
			name:                "No prior impressions, cap 3",
			lineItem:            models.LineItem{ID: 801, CampaignID: 801, PublisherID: 0, FrequencyCap: 3, FrequencyWindow: defaultWin, Active: true},
			userID:              "user1",
			lineItemIDToUse:     801,
			impressionsToPreLog: 0,
			expectedResult:      false, // Increments to 1; 1 > 3 is false
		},
		{
			name:                "1 prior impression, cap 3",
			lineItem:            models.LineItem{ID: 802, CampaignID: 802, PublisherID: 0, FrequencyCap: 3, FrequencyWindow: defaultWin, Active: true},
			userID:              "user2",
			lineItemIDToUse:     802,
			impressionsToPreLog: 1,
			expectedResult:      false, // Increments to 2; 2 > 3 is false
		},
		{
			name:                "2 prior impressions, cap 3 (meets cap)",
			lineItem:            models.LineItem{ID: 803, CampaignID: 803, PublisherID: 0, FrequencyCap: 3, FrequencyWindow: defaultWin, Active: true},
			userID:              "user3",
			lineItemIDToUse:     803,
			impressionsToPreLog: 2,
			expectedResult:      false, // Increments to 3; 3 > 3 is false
		},
		{
			name:                "3 prior impressions, cap 3 (exceeds cap)",
			lineItem:            models.LineItem{ID: 804, CampaignID: 804, PublisherID: 0, FrequencyCap: 3, FrequencyWindow: defaultWin, Active: true},
			userID:              "user4",
			lineItemIDToUse:     804,
			impressionsToPreLog: 3,
			expectedResult:      true, // Increments to 4; 4 > 3 is true
		},
		{
			name:                "No prior impressions, cap 1",
			lineItem:            models.LineItem{ID: 805, CampaignID: 805, PublisherID: 0, FrequencyCap: 1, FrequencyWindow: defaultWin, Active: true},
			userID:              "user5",
			lineItemIDToUse:     805,
			impressionsToPreLog: 0,
			expectedResult:      false, // Increments to 1; 1 > 1 is false
		},
		{
			name:                "1 prior impression, cap 1 (exceeds cap)",
			lineItem:            models.LineItem{ID: 806, CampaignID: 806, PublisherID: 0, FrequencyCap: 1, FrequencyWindow: defaultWin, Active: true},
			userID:              "user6",
			lineItemIDToUse:     806,
			impressionsToPreLog: 1,
			expectedResult:      true, // Increments to 2; 2 > 1 is true
		},
		{
			name:                "Line item not found, uses DefaultFrequencyCap (3)",
			lineItem:            models.LineItem{}, // Not added to InMemoryLineItems
			userID:              "user7",
			lineItemIDToUse:     807,   // This ID won't be in InMemoryLineItems
			impressionsToPreLog: 2,     // Default cap is 3
			expectedResult:      false, // Increments to 3; 3 > DefaultCap (3) is false
		},
		{
			name:                "Line item not found, uses DefaultFrequencyCap (3), exceeds",
			lineItem:            models.LineItem{}, // Not added to InMemoryLineItems
			userID:              "user8",
			lineItemIDToUse:     808,
			impressionsToPreLog: 3,    // Default cap is 3
			expectedResult:      true, // Increments to 4; 4 > DefaultCap (3) is true
		},
		{
			name:                "Line item with FrequencyCap 0, uses DefaultFrequencyCap (3)",
			lineItem:            models.LineItem{ID: 809, CampaignID: 809, PublisherID: 0, FrequencyCap: 0, FrequencyWindow: defaultWin, Active: true},
			userID:              "user9",
			lineItemIDToUse:     809,
			impressionsToPreLog: 3,
			expectedResult:      true, // Increments to 4; 4 > DefaultCap (3) is true
		},
		{
			name:                "Line item with custom FrequencyCap 5, exceeds",
			lineItem:            models.LineItem{ID: 810, CampaignID: 810, PublisherID: 0, FrequencyCap: 5, FrequencyWindow: defaultWin, Active: true},
			userID:              "user10",
			lineItemIDToUse:     810,
			impressionsToPreLog: 5,
			expectedResult:      true, // Increments to 6; 6 > 5 is true
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ms, store := setupTestRedis(t)
			defer ms.Close()

			testDataStore := models.NewTestAdDataStore()
			currentTestLineItems := []models.LineItem{}
			if tc.lineItem.ID != 0 {
				currentTestLineItems = append(currentTestLineItems, tc.lineItem)
			}
			_ = testDataStore.SetLineItems(currentTestLineItems)

			var windowForPreLog time.Duration
			effectiveLI := testDataStore.GetLineItem(0, tc.lineItemIDToUse) // Get LI after index is built with current test item

			if effectiveLI != nil && effectiveLI.FrequencyWindow > 0 {
				windowForPreLog = effectiveLI.FrequencyWindow
			} else {
				windowForPreLog = DefaultFrequencyWindow // Fallback to default if LI not found or its window is 0
			}

			for i := 0; i < tc.impressionsToPreLog; i++ {
				_, err := store.IncrementImpression(tc.userID, tc.lineItemIDToUse, windowForPreLog)
				if err != nil {
					t.Fatalf("Failed to pre-log impression: %v", err)
				}
			}

			result, err := HasUserExceededFrequencyCap(store, tc.userID, 0, tc.lineItemIDToUse, testDataStore)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tc.expectedResult {
				// Determine effective cap for logging
				var effectiveCap int
				capLI := testDataStore.GetLineItem(0, tc.lineItemIDToUse) // Re-fetch for cap information
				if capLI != nil && capLI.FrequencyCap > 0 {
					effectiveCap = capLI.FrequencyCap
				} else {
					effectiveCap = DefaultFrequencyCap
				}

				t.Errorf("User: %s, LI: %d, PreLogged: %d, Cap: %d - Expected HasUserExceededFrequencyCap to be %v, but got %v.",
					tc.userID, tc.lineItemIDToUse, tc.impressionsToPreLog, effectiveCap, tc.expectedResult, result)
			}
		})
	}
}

func TestHasUserExceededFrequencyCap_NilStore(t *testing.T) {
	exceeded, err := HasUserExceededFrequencyCap(nil, "u1", 0, 1, models.NewTestAdDataStore())
	if err != ErrNilRedisStore {
		t.Fatalf("expected ErrNilRedisStore, got %v", err)
	}
	if exceeded {
		t.Error("expected exceeded to be false with nil store")
	}
}
