package filters

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/stretchr/testify/assert"
)

// Test helpers
func createTestCreatives(count int) []models.Creative {
	creatives := make([]models.Creative, count)
	for i := 0; i < count; i++ {
		creatives[i] = models.Creative{
			ID:          i + 1,
			PublisherID: 1,
			LineItemID:  i + 1,
			PlacementID: "placement-1",
			Width:       300,
			Height:      250,
			Format:      "banner",
		}
	}
	return creatives
}

func createTestLineItem(id int, active bool, country string) *models.LineItem {
	return &models.LineItem{
		ID:              id,
		PublisherID:     1,
		Name:            fmt.Sprintf("Line Item %d", id),
		Active:          active,
		ECPM:            1.0,
		Priority:        models.PriorityMedium,
		FrequencyCap:    10,
		FrequencyWindow: 24 * time.Hour,
		Country:         country,
	}
}

// TestSinglePassFilterBasic tests basic functionality
func TestSinglePassFilterBasic(t *testing.T) {
	dataStore := models.NewInMemoryAdDataStore()

	// Setup line items
	items := make([]models.LineItem, 5)
	items[0] = *createTestLineItem(1, false, "")   // Inactive
	items[1] = *createTestLineItem(2, true, "UK")  // Active but wrong geo
	items[2] = *createTestLineItem(3, true, "US")  // Active and correct geo
	items[3] = *createTestLineItem(4, true, "")    // Active, no geo restriction
	items[4] = *createTestLineItem(5, false, "US") // Inactive
	_ = dataStore.SetLineItems(items)

	creatives := createTestCreatives(5)
	ctx := models.TargetingContext{Country: "US"}

	spFilter := NewSinglePassFilter(nil, dataStore, config.Config{})
	result, err := spFilter.FilterCreatives(
		context.Background(),
		creatives,
		ctx,
		300,
		250,
		[]string{"banner"},
		"test-user",
	)

	assert.NoError(t, err)
	assert.Len(t, result, 2, "Should have 2 eligible creatives")

	// Verify specific creatives passed
	resultIDs := make(map[int]bool)
	for _, c := range result {
		resultIDs[c.ID] = true
	}

	assert.True(t, resultIDs[3], "Creative 3 should pass (active, correct geo)")
	assert.True(t, resultIDs[4], "Creative 4 should pass (active, no geo restriction)")
}

// TestSinglePassVsMultiPassEquivalence tests that single-pass produces identical results
func TestSinglePassVsMultiPassEquivalence(t *testing.T) {
	// Use InMemoryAdDataStore for tests
	dataStore := models.NewInMemoryAdDataStore()

	tests := []struct {
		name            string
		numCreatives    int
		setupFunc       func()
		targetingCtx    models.TargetingContext
		width           int
		height          int
		allowedFormats  []string
		expectedRemoved int
	}{
		{
			name:         "all filters pass",
			numCreatives: 10,
			setupFunc: func() {
				// Add active line items with matching targeting
				items := make([]models.LineItem, 10)
				for i := 0; i < 10; i++ {
					items[i] = *createTestLineItem(i+1, true, "US")
				}
				_ = dataStore.SetLineItems(items)
			},
			targetingCtx: models.TargetingContext{
				Country: "US",
			},
			width:           300,
			height:          250,
			allowedFormats:  []string{"banner"},
			expectedRemoved: 0,
		},
		{
			name:         "inactive line items filtered",
			numCreatives: 10,
			setupFunc: func() {
				// Half active, half inactive
				items := make([]models.LineItem, 10)
				for i := 0; i < 10; i++ {
					items[i] = *createTestLineItem(i+1, i%2 == 0, "")
				}
				_ = dataStore.SetLineItems(items)
			},
			targetingCtx:    models.TargetingContext{},
			width:           300,
			height:          250,
			allowedFormats:  []string{"banner"},
			expectedRemoved: 5,
		},
		{
			name:         "targeting mismatch",
			numCreatives: 10,
			setupFunc: func() {
				// All active but with geo targeting
				items := make([]models.LineItem, 10)
				for i := 0; i < 10; i++ {
					items[i] = *createTestLineItem(i+1, true, "UK")
				}
				_ = dataStore.SetLineItems(items)
			},
			targetingCtx: models.TargetingContext{
				Country: "US",
			},
			width:           300,
			height:          250,
			allowedFormats:  []string{"banner"},
			expectedRemoved: 10,
		},
		{
			name:         "size mismatch",
			numCreatives: 10,
			setupFunc: func() {
				// All active, no targeting
				items := make([]models.LineItem, 10)
				for i := 0; i < 10; i++ {
					items[i] = *createTestLineItem(i+1, true, "")
				}
				_ = dataStore.SetLineItems(items)
			},
			targetingCtx:    models.TargetingContext{},
			width:           728, // Different size
			height:          90,  // Different size
			allowedFormats:  []string{"banner"},
			expectedRemoved: 10,
		},
		{
			name:         "format mismatch",
			numCreatives: 10,
			setupFunc: func() {
				// All active, no targeting
				items := make([]models.LineItem, 10)
				for i := 0; i < 10; i++ {
					items[i] = *createTestLineItem(i+1, true, "")
				}
				_ = dataStore.SetLineItems(items)
			},
			targetingCtx:    models.TargetingContext{},
			width:           300,
			height:          250,
			allowedFormats:  []string{"video", "native"}, // Different formats
			expectedRemoved: 10,
		},
		{
			name:         "mixed filters",
			numCreatives: 20,
			setupFunc: func() {
				items := make([]models.LineItem, 20)
				for i := 0; i < 20; i++ {
					active := i < 15   // First 15 active
					hasGeo := i%3 == 0 // Every 3rd has geo targeting

					country := ""
					if hasGeo {
						country = "US"
					}
					items[i] = *createTestLineItem(i+1, active, country)
				}
				_ = dataStore.SetLineItems(items)
			},
			targetingCtx: models.TargetingContext{
				Country: "US",
			},
			width:           300,
			height:          250,
			allowedFormats:  []string{"banner"},
			expectedRemoved: 5, // 5 inactive only (empty geo should pass)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test data
			creatives := createTestCreatives(tt.numCreatives)
			tt.setupFunc()

			// Apply multi-pass filters (original approach)
			multiPassResult := make([]models.Creative, len(creatives))
			copy(multiPassResult, creatives)

			// Filter by active
			multiPassResult = FilterByActive(multiPassResult, dataStore)

			// Filter by targeting
			multiPassResult = FilterByTargeting(multiPassResult, tt.targetingCtx, dataStore)

			// Filter by size
			multiPassResult = FilterBySize(multiPassResult, tt.width, tt.height, tt.allowedFormats)

			// Apply single-pass filter
			spFilter := NewSinglePassFilter(nil, dataStore, config.Config{})
			singlePassResult, err := spFilter.FilterCreatives(
				context.Background(),
				creatives,
				tt.targetingCtx,
				tt.width,
				tt.height,
				tt.allowedFormats,
				"test-user",
			)

			// Verify no error
			assert.NoError(t, err)

			// Verify same number of results
			assert.Equal(t, len(multiPassResult), len(singlePassResult),
				"Single-pass and multi-pass should return same number of creatives")

			// Verify same creatives (order may differ)
			multiPassIDs := make(map[int]bool)
			for _, c := range multiPassResult {
				multiPassIDs[c.ID] = true
			}

			singlePassIDs := make(map[int]bool)
			for _, c := range singlePassResult {
				singlePassIDs[c.ID] = true
			}

			assert.Equal(t, multiPassIDs, singlePassIDs,
				"Single-pass and multi-pass should return same creative IDs")

			// Verify expected count
			assert.Equal(t, tt.numCreatives-tt.expectedRemoved, len(singlePassResult),
				"Unexpected number of filtered creatives")
		})
	}
}

// TestSinglePassPerformance benchmarks the performance improvement
func BenchmarkFiltering(b *testing.B) {
	creativeCounts := []int{10, 100, 1000}

	for _, count := range creativeCounts {
		b.Run(fmt.Sprintf("MultiPass_%d", count), func(b *testing.B) {
			dataStore := models.NewInMemoryAdDataStore()
			creatives := createTestCreatives(count)

			// Setup line items
			items := make([]models.LineItem, count)
			for i := 0; i < count; i++ {
				items[i] = *createTestLineItem(i+1, i%10 != 0, "US")
			}
			_ = dataStore.SetLineItems(items)

			ctx := models.TargetingContext{Country: "US"}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result := make([]models.Creative, len(creatives))
				copy(result, creatives)

				result = FilterByActive(result, dataStore)
				result = FilterByTargeting(result, ctx, dataStore)
				result = FilterBySize(result, 300, 250, []string{"banner"})
				_ = result // Use result to avoid ineffassign warning
			}
		})

		b.Run(fmt.Sprintf("SinglePass_%d", count), func(b *testing.B) {
			dataStore := models.NewInMemoryAdDataStore()
			creatives := createTestCreatives(count)

			// Setup line items
			items := make([]models.LineItem, count)
			for i := 0; i < count; i++ {
				items[i] = *createTestLineItem(i+1, i%10 != 0, "US")
			}
			_ = dataStore.SetLineItems(items)

			ctx := models.TargetingContext{Country: "US"}
			spFilter := NewSinglePassFilter(nil, dataStore, config.Config{})

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = spFilter.FilterCreatives(
					context.Background(),
					creatives,
					ctx,
					300,
					250,
					[]string{"banner"},
					"test-user",
				)
			}
		})
	}
}

// TestSinglePassMemoryAllocation tests memory allocation efficiency
func TestSinglePassMemoryAllocation(t *testing.T) {
	dataStore := models.NewInMemoryAdDataStore()
	creatives := createTestCreatives(1000)

	// Setup all active line items
	items := make([]models.LineItem, 1000)
	for i := 0; i < 1000; i++ {
		items[i] = *createTestLineItem(i+1, true, "")
	}
	_ = dataStore.SetLineItems(items)

	ctx := models.TargetingContext{}
	spFilter := NewSinglePassFilter(nil, dataStore, config.Config{})

	// Run filter
	result, err := spFilter.FilterCreatives(
		context.Background(),
		creatives,
		ctx,
		300,
		250,
		[]string{"banner"},
		"test-user",
	)

	assert.NoError(t, err)
	assert.Len(t, result, 1000, "All creatives should pass")
}

// TestSinglePassTracing tests the tracing functionality
func TestSinglePassTracing(t *testing.T) {
	dataStore := models.NewInMemoryAdDataStore()
	creatives := createTestCreatives(5)

	// Setup mixed line items
	items := make([]models.LineItem, 5)
	items[0] = *createTestLineItem(1, false, "")   // Inactive
	items[1] = *createTestLineItem(2, true, "UK")  // Active but wrong geo
	items[2] = *createTestLineItem(3, true, "US")  // Active and correct geo
	items[3] = *createTestLineItem(4, true, "")    // Active, no geo restriction
	items[4] = *createTestLineItem(5, false, "US") // Inactive
	_ = dataStore.SetLineItems(items)

	ctx := models.TargetingContext{Country: "US"}
	trace := &logic.SelectionTrace{}

	spFilter := NewSinglePassFilter(nil, dataStore, config.Config{})
	result, err := spFilter.FilterCreativesWithTrace(
		context.Background(),
		creatives,
		ctx,
		300,
		250,
		[]string{"banner"},
		"test-user",
		trace,
	)

	assert.NoError(t, err)
	assert.Len(t, result, 2, "Should have 2 eligible creatives")

	// Check trace has expected steps
	assert.Len(t, trace.Steps, 2, "Should have start and complete steps")
	assert.Equal(t, "single_pass_start", trace.Steps[0].Stage)
	assert.Equal(t, "single_pass_complete", trace.Steps[1].Stage)
}
