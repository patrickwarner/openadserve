package forecasting

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// MockAdDataStore implements models.AdDataStore for testing
type MockAdDataStore struct {
	lineItems []models.LineItem
	campaigns []models.Campaign
}

func (m *MockAdDataStore) GetLineItem(publisherID, lineItemID int) *models.LineItem {
	for _, li := range m.lineItems {
		if li.ID == lineItemID && li.PublisherID == publisherID {
			return &li
		}
	}
	return nil
}

func (m *MockAdDataStore) GetLineItemsByPublisher(publisherID int) []models.LineItem {
	var result []models.LineItem
	for _, li := range m.lineItems {
		if li.PublisherID == publisherID {
			result = append(result, li)
		}
	}
	return result
}

func (m *MockAdDataStore) GetCampaign(campaignID int) *models.Campaign {
	for _, c := range m.campaigns {
		if c.ID == campaignID {
			return &c
		}
	}
	return nil
}

func (m *MockAdDataStore) GetPublisher(publisherID int) *models.Publisher    { return nil }
func (m *MockAdDataStore) GetPlacement(placementID string) *models.Placement { return nil }
func (m *MockAdDataStore) GetLineItemByID(lineItemID int) *models.LineItem   { return nil }
func (m *MockAdDataStore) GetAllPublishers() []models.Publisher              { return nil }
func (m *MockAdDataStore) GetAllPublisherIDs() []int                         { return nil }
func (m *MockAdDataStore) GetAllCampaigns() []models.Campaign                { return m.campaigns }
func (m *MockAdDataStore) GetAllLineItems() []models.LineItem                { return m.lineItems }
func (m *MockAdDataStore) GetAllPlacements() []models.Placement              { return nil }
func (m *MockAdDataStore) SetLineItems(items []models.LineItem) error        { return nil }
func (m *MockAdDataStore) SetLineItemsForPublisher(publisherID int, items []models.LineItem) error {
	return nil
}
func (m *MockAdDataStore) SetCampaigns(campaigns []models.Campaign) error    { return nil }
func (m *MockAdDataStore) SetPublishers(publishers []models.Publisher) error { return nil }
func (m *MockAdDataStore) SetPlacements(placements []models.Placement) error { return nil }
func (m *MockAdDataStore) ReloadAll(lineItems []models.LineItem, campaigns []models.Campaign, publishers []models.Publisher, placements []models.Placement) error {
	return nil
}
func (m *MockAdDataStore) UpdateLineItemECPM(publisherID, lineItemID int, ecpm float64) error {
	return nil
}
func (m *MockAdDataStore) UpdateLineItemsECPM(updates map[int]float64) error { return nil }
func (m *MockAdDataStore) UpdateLineItemSpend(publisherID, lineItemID int, spend float64) error {
	return nil
}
func (m *MockAdDataStore) UpdateLineItemsSpend(updates map[int]float64) error { return nil }
func (m *MockAdDataStore) InsertPublisher(publisher *models.Publisher) error  { return nil }
func (m *MockAdDataStore) UpdatePublisher(publisher models.Publisher) error   { return nil }
func (m *MockAdDataStore) DeletePublisher(publisherID int) error              { return nil }
func (m *MockAdDataStore) InsertCampaign(campaign *models.Campaign) error     { return nil }
func (m *MockAdDataStore) UpdateCampaign(campaign models.Campaign) error      { return nil }
func (m *MockAdDataStore) DeleteCampaign(campaignID int) error                { return nil }
func (m *MockAdDataStore) InsertLineItem(lineItem *models.LineItem) error     { return nil }
func (m *MockAdDataStore) UpdateLineItem(lineItem models.LineItem) error      { return nil }
func (m *MockAdDataStore) DeleteLineItem(lineItemID int) error                { return nil }
func (m *MockAdDataStore) InsertPlacement(placement models.Placement) error   { return nil }
func (m *MockAdDataStore) UpdatePlacement(placement models.Placement) error   { return nil }
func (m *MockAdDataStore) DeletePlacement(placementID string) error           { return nil }

func TestValidateForecastRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *models.ForecastRequest
		wantErr bool
	}{
		{
			name: "valid CPM request",
			req: &models.ForecastRequest{
				StartDate:   time.Now(),
				EndDate:     time.Now().AddDate(0, 0, 7),
				BudgetType:  models.BudgetTypeCPM,
				Budget:      1000.0,
				CPM:         5.0,
				PublisherID: 1,
			},
			wantErr: false,
		},
		{
			name: "missing start date",
			req: &models.ForecastRequest{
				EndDate:     time.Now().AddDate(0, 0, 7),
				BudgetType:  models.BudgetTypeCPM,
				Budget:      1000.0,
				CPM:         5.0,
				PublisherID: 1,
			},
			wantErr: true,
		},
		{
			name: "invalid budget type",
			req: &models.ForecastRequest{
				StartDate:   time.Now(),
				EndDate:     time.Now().AddDate(0, 0, 7),
				BudgetType:  "invalid",
				Budget:      1000.0,
				PublisherID: 1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateForecastRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateForecastRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewEngine(t *testing.T) {
	logger := zap.NewNop()
	mockStore := &MockAdDataStore{}

	engine := NewEngine(nil, nil, mockStore, logger)

	if engine == nil {
		t.Error("NewEngine() returned nil")
		return
	}
	if engine.AdStore == nil {
		t.Error("NewEngine() did not set AdStore correctly")
	}
	if engine.Logger != logger {
		t.Error("NewEngine() did not set Logger correctly")
	}
}

func TestConflictDetectionBasic(t *testing.T) {
	logger := zap.NewNop()
	mockStore := &MockAdDataStore{
		lineItems: []models.LineItem{
			{
				ID:           1,
				CampaignID:   1,
				PublisherID:  1,
				Name:         "Test Line Item",
				Priority:     models.PriorityHigh,
				Country:      "US",
				DeviceType:   "mobile",
				Active:       true,
				StartDate:    time.Now().AddDate(0, 0, -1),
				EndDate:      time.Now().AddDate(0, 0, 30),
				BudgetAmount: 1000.0,
				Spend:        0.0,
			},
		},
		campaigns: []models.Campaign{
			{
				ID:   1,
				Name: "Test Campaign",
			},
		},
	}

	engine := NewEngine(nil, nil, mockStore, logger)

	req := &models.ForecastRequest{
		PublisherID: 1,
		StartDate:   time.Now(),
		EndDate:     time.Now().AddDate(0, 0, 7),
		Countries:   []string{"US"},
		DeviceTypes: []string{"mobile"},
		Priority:    2, // Medium priority (should conflict with high priority line item)
	}

	conflicts, err := engine.detectConflicts(context.Background(), req)
	if err != nil {
		t.Fatalf("detectConflicts() error = %v", err)
	}

	t.Logf("Found %d conflicts", len(conflicts))
	if len(conflicts) == 0 {
		// Debug: let's check the line items and targeting overlap
		lineItems := mockStore.GetLineItemsByPublisher(1)
		t.Logf("Found %d line items for publisher 1", len(lineItems))
		if len(lineItems) > 0 {
			li := &lineItems[0]
			active := isActiveInPeriod(li, req.StartDate, req.EndDate)
			overlap := calculateTargetingOverlap(li, req)
			t.Logf("Line item: Country=%s, DeviceType=%s, Active=%v", li.Country, li.DeviceType, li.Active)
			t.Logf("Line item dates: Start=%v, End=%v", li.StartDate, li.EndDate)
			t.Logf("Line item budget: Amount=%f, Spend=%f", li.BudgetAmount, li.Spend)
			t.Logf("Request: Countries=%v, DeviceTypes=%v", req.Countries, req.DeviceTypes)
			t.Logf("Request dates: Start=%v, End=%v", req.StartDate, req.EndDate)
			t.Logf("Is active in period: %v", active)
			t.Logf("Targeting overlap: %f", overlap)
		}
		t.Error("Expected at least one conflict, got none")
	}

	if len(conflicts) > 0 {
		conflict := conflicts[0]
		if conflict.ConflictType != "higher_priority" {
			t.Errorf("Expected conflict type 'higher_priority', got %s", conflict.ConflictType)
		}
		if conflict.OverlapPercentage <= 0 {
			t.Errorf("Expected positive overlap percentage, got %f", conflict.OverlapPercentage)
		}
	}
}
