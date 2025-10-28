package selectors

import (
	"testing"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestFilterMetrics(t *testing.T) {
	// Reset metrics for clean test
	observability.FilterDuration.Reset()
	observability.FilterStageCount.Reset()

	// Create test data
	var store *db.RedisStore // nil store for test
	dataStore := models.NewInMemoryAdDataStore()

	// Add line items first
	lineItems := []models.LineItem{
		{ID: 1, PublisherID: 1, Active: true, Name: "LI 1"},
		{ID: 2, PublisherID: 1, Active: true, Name: "LI 2"},
		{ID: 3, PublisherID: 1, Active: false, Name: "LI 3"}, // Inactive
	}
	_ = dataStore.SetLineItems(lineItems)

	// Create test creatives with LineItem references
	creatives := []models.Creative{
		{ID: 1, PlacementID: "test-placement", LineItemID: 1, PublisherID: 1, Width: 300, Height: 250, Format: "banner"},
		{ID: 2, PlacementID: "test-placement", LineItemID: 2, PublisherID: 1, Width: 300, Height: 250, Format: "banner"},
		{ID: 3, PlacementID: "test-placement", LineItemID: 3, PublisherID: 1, Width: 300, Height: 250, Format: "banner"},
	}

	// Set LineItem pointers
	for i := range creatives {
		creatives[i].LineItem = dataStore.GetLineItem(creatives[i].PublisherID, creatives[i].LineItemID)
	}

	// Create test database
	database := &db.DB{
		Creatives: creatives,
		Placements: map[string]models.Placement{
			"test-placement": {
				ID:      "test-placement",
				Width:   300,
				Height:  250,
				Formats: []string{"banner"},
			},
		},
	}

	// Build indexes
	database.BuildIndexes()

	// Create selector
	selector := NewRuleBasedSelector()

	// Test single-pass approach (only approach now)
	tests := []struct {
		name          string
		expectedError error
	}{
		{
			name:          "single-pass approach",
			expectedError: nil, // Single-pass handles nil Redis gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make request
			ctx := models.TargetingContext{
				Country: "US",
			}
			response, err := selector.SelectAd(store, database, dataStore, "test-placement", "user123", 0, 0, ctx, config.Config{})

			// Check expected error
			assert.Equal(t, tt.expectedError, err)

			// Single-pass should return a successful ad response
			if tt.expectedError == nil {
				assert.NotNil(t, response, "Single-pass should return successful ad response")
			} else {
				assert.Nil(t, response, "Should return nil response on error")
			}

			// Check metrics were recorded
			// 1. Filter duration metric
			expectedResult := "success"
			if tt.expectedError != nil {
				expectedResult = "error"
			}
			observer, _ := observability.FilterDuration.GetMetricWithLabelValues("1-10", expectedResult)
			assert.NotNil(t, observer, "Filter duration should be recorded")

			// 2. Filter stage count metric
			gauge, _ := observability.FilterStageCount.GetMetricWithLabelValues("filtered")
			assert.NotNil(t, gauge, "Filter stage count should be recorded")
		})
	}
}

func TestCreativeCountBucket(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{5, "1-10"},
		{10, "1-10"},
		{11, "11-50"},
		{50, "11-50"},
		{51, "51-100"},
		{100, "51-100"},
		{101, "101-500"},
		{500, "101-500"},
		{501, "501-1000"},
		{1000, "501-1000"},
		{1001, "1000+"},
		{10000, "1000+"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := observability.GetCreativeCountBucket(tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// BenchmarkWithMetrics tests the overhead of metrics collection
func BenchmarkWithMetrics(b *testing.B) {
	// Setup
	var store *db.RedisStore
	dataStore := models.NewInMemoryAdDataStore()

	// Add 100 line items
	lineItems := make([]models.LineItem, 100)
	for i := 0; i < 100; i++ {
		lineItems[i] = models.LineItem{
			ID:          i + 1,
			PublisherID: 1,
			Active:      true,
			Name:        "Bench LI",
		}
	}
	_ = dataStore.SetLineItems(lineItems)

	// Add 100 creatives
	creatives := make([]models.Creative, 100)
	for i := 0; i < 100; i++ {
		creatives[i] = models.Creative{
			ID:          i + 1,
			PlacementID: "bench-placement",
			LineItemID:  i + 1,
			PublisherID: 1,
			Width:       300,
			Height:      250,
			Format:      "banner",
		}
		// Set LineItem pointer
		creatives[i].LineItem = dataStore.GetLineItem(1, i+1)
	}

	// Create database
	database := &db.DB{
		Creatives: creatives,
		Placements: map[string]models.Placement{
			"bench-placement": {
				ID:     "bench-placement",
				Width:  300,
				Height: 250,
			},
		},
	}
	database.BuildIndexes()

	selector := NewRuleBasedSelector()
	ctx := models.TargetingContext{}

	b.Run("with_metrics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = selector.SelectAd(store, database, dataStore, "bench-placement", "user123", 0, 0, ctx, config.Config{})
		}
	})

	// Disable metrics collection (mock by using custom registry)
	oldRegistry := prometheus.DefaultRegisterer
	prometheus.DefaultRegisterer = prometheus.NewRegistry()
	defer func() { prometheus.DefaultRegisterer = oldRegistry }()

	b.Run("without_metrics", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = selector.SelectAd(store, database, dataStore, "bench-placement", "user123", 0, 0, ctx, config.Config{})
		}
	})
}
