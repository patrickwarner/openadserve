package forecasting

import (
	"context"
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// For testing, we'll use the real InMemoryAdDataStore
func newMockAdDataStore() models.AdDataStore {
	return models.NewInMemoryAdDataStore()
}

func TestBidBasedConflictDetection(t *testing.T) {
	logger := zap.NewNop()
	store := newMockAdDataStore()

	engine := &Engine{
		AdStore: store,
		Logger:  logger,
	}

	// Add test campaigns
	err := store.InsertCampaign(&models.Campaign{
		ID:          1,
		Name:        "High CPM Campaign",
		PublisherID: 1,
	})
	assert.NoError(t, err)

	err = store.InsertCampaign(&models.Campaign{
		ID:          2,
		Name:        "Low CPM Campaign",
		PublisherID: 1,
	})
	assert.NoError(t, err)

	// Add competing line items with same priority but different bids
	tomorrow := time.Now().AddDate(0, 0, 1)
	nextWeek := time.Now().AddDate(0, 0, 7)

	highCPMLineItem := &models.LineItem{
		ID:           1,
		Name:         "High CPM Line Item",
		CampaignID:   1,
		PublisherID:  1,
		Priority:     models.PriorityMedium,
		BudgetType:   models.BudgetTypeCPM,
		CPM:          15.0, // $15 CPM
		BudgetAmount: 1000.0,
		Spend:        0.0,
		StartDate:    tomorrow,
		EndDate:      nextWeek,
		Country:      "US",
		DeviceType:   "mobile",
		Active:       true,
	}

	lowCPMLineItem := &models.LineItem{
		ID:           2,
		Name:         "Low CPM Line Item",
		CampaignID:   2,
		PublisherID:  1,
		Priority:     models.PriorityMedium,
		BudgetType:   models.BudgetTypeCPM,
		CPM:          5.0, // $5 CPM
		BudgetAmount: 1000.0,
		Spend:        0.0,
		StartDate:    tomorrow,
		EndDate:      nextWeek,
		Country:      "US",
		DeviceType:   "mobile",
		Active:       true,
	}

	err = store.InsertLineItem(highCPMLineItem)
	assert.NoError(t, err)

	err = store.InsertLineItem(lowCPMLineItem)
	assert.NoError(t, err)

	// Test forecast request with medium CPM
	req := &models.ForecastRequest{
		PublisherID: 1,
		Priority:    1, // Medium priority (index 1)
		BudgetType:  models.BudgetTypeCPM,
		CPM:         10.0, // $10 CPM - between the two existing line items
		Budget:      500.0,
		StartDate:   tomorrow,
		EndDate:     nextWeek,
		Countries:   []string{"US"},
		DeviceTypes: []string{"mobile"}, // Match line item device type
	}

	conflicts, err := engine.detectConflicts(context.Background(), req)
	assert.NoError(t, err)

	// Should detect one conflict (high CPM line item)
	// Low CPM line item should be filtered out as we clearly outbid it (10.0 > 5.0 * 1.1)
	assert.Len(t, conflicts, 1, "Should detect exactly one conflict")

	conflict := conflicts[0]
	assert.Equal(t, 1, conflict.LineItemID, "Should conflict with high CPM line item")
	assert.Equal(t, "higher_priority", conflict.ConflictType, "High CPM should be treated as higher priority")
}

func TestECPMCalculation(t *testing.T) {
	logger := zap.NewNop()
	store := newMockAdDataStore()

	engine := &Engine{
		AdStore: store,
		Logger:  logger,
	}

	tests := []struct {
		name     string
		req      *models.ForecastRequest
		expected float64
	}{
		{
			name: "CPM Campaign",
			req: &models.ForecastRequest{
				BudgetType: models.BudgetTypeCPM,
				CPM:        12.5,
			},
			expected: 12.5,
		},
		{
			name: "CPC Campaign",
			req: &models.ForecastRequest{
				BudgetType: models.BudgetTypeCPC,
				CPC:        2.0,
			},
			expected: 20.0, // 2.0 * 0.01 * 1000 = 20.0 eCPM
		},
		{
			name: "Unknown Budget Type",
			req: &models.ForecastRequest{
				BudgetType: "unknown",
			},
			expected: 0.0, // Should return 0 for unsupported types
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecpm := engine.calculateForecastECPM(tt.req)
			assert.Equal(t, tt.expected, ecpm, "eCPM calculation should match expected value")
		})
	}
}

func TestLineItemECPMCalculation(t *testing.T) {
	logger := zap.NewNop()
	store := newMockAdDataStore()

	engine := &Engine{
		AdStore: store,
		Logger:  logger,
	}

	tests := []struct {
		name     string
		li       *models.LineItem
		expected float64
	}{
		{
			name: "CPM Line Item",
			li: &models.LineItem{
				BudgetType: models.BudgetTypeCPM,
				CPM:        8.0,
			},
			expected: 8.0,
		},
		{
			name: "CPC Line Item",
			li: &models.LineItem{
				BudgetType: models.BudgetTypeCPC,
				CPC:        1.5,
			},
			expected: 15.0, // 1.5 * 0.01 * 1000 = 15.0 eCPM
		},
		{
			name: "Unknown Budget Type",
			li: &models.LineItem{
				BudgetType: "unknown",
			},
			expected: 0.0, // Should return 0 for unsupported types
		},
		{
			name:     "Nil Line Item",
			li:       nil,
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecpm := engine.calculateLineItemECPM(tt.li)
			assert.Equal(t, tt.expected, ecpm, "Line item eCPM calculation should match expected value")
		})
	}
}
