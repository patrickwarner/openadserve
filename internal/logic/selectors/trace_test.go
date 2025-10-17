package selectors

import (
	"testing"

	logic "github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
)

// TestSelectAdWithTrace verifies that line item IDs are captured in the trace steps.
func TestSelectAdWithTrace(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 201, CampaignID: 201, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, Active: true, PublisherID: 0},
		{ID: 202, CampaignID: 202, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, Active: true, PublisherID: 0},
	})
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 201, Name: "C201"},
		{ID: 202, Name: "C202"},
	})

	creativeA := models.Creative{ID: 1, PlacementID: "header", LineItemID: 201, CampaignID: 201, HTML: "A", Width: 320, Height: 50, Format: "html"}
	creativeB := models.Creative{ID: 2, PlacementID: "header", LineItemID: 202, CampaignID: 202, HTML: "B", Width: 320, Height: 50, Format: "html"}

	placement := models.Placement{ID: "header", Width: 320, Height: 50, Formats: []string{"html"}}
	database := createTestDB([]models.Creative{creativeA, creativeB}, map[string]models.Placement{"header": placement})

	ctx := models.TargetingContext{}
	var trace logic.SelectionTrace
	selector := NewRuleBasedSelector()
	_, err := selector.SelectAdWithTrace(store, database, testDataStore, "header", "user", 0, 0, ctx, &trace, testConfig())
	if err != nil {
		t.Fatalf("select ad: %v", err)
	}
	if len(trace.Steps) == 0 {
		t.Fatal("expected trace steps")
	}
	// First step should contain both line items
	found201, found202 := false, false
	for _, id := range trace.Steps[0].LineItemIDs {
		if id == 201 {
			found201 = true
		}
		if id == 202 {
			found202 = true
		}
	}
	if !found201 || !found202 {
		t.Errorf("expected line item IDs 201 and 202 in trace, got %+v", trace.Steps[0].LineItemIDs)
	}
}
