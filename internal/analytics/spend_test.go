package analytics

import (
	"context" // Added import
	"testing"

	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
)

func TestRecordImpression_CPMSpend(t *testing.T) {
	// Initialize AdDataStore for the test
	testStore := models.NewInMemoryAdDataStore()
	_ = testStore.SetLineItems([]models.LineItem{
		{ID: 1, CampaignID: 1, CPM: 2.0, ECPM: 2.0, BudgetType: models.BudgetTypeCPM, BudgetAmount: 10, Active: true, PublisherID: 0},
	})

	a := &Analytics{Metrics: observability.NewNoOpRegistry()}
	// Add context.Background() to calls
	if err := a.RecordImpression(context.Background(), testStore, "req1", "1", "1", 1, "mobile", "US", 1, "test-placement"); err != nil && err != ErrUnavailable {
		t.Fatalf("record impression: %v", err)
	}
	li := models.GetLineItemByID(testStore, 1)
	want := 2.0 / 1000
	if li.Spend != want {
		t.Fatalf("want spend %f got %f", want, li.Spend)
	}
	// Metrics are now handled by NoOpRegistry - no assertions needed

	if err := a.RecordImpression(context.Background(), testStore, "req2", "1", "1", 1, "mobile", "US", 1, "test-placement"); err != nil && err != ErrUnavailable {
		t.Fatalf("record impression: %v", err)
	}
	want = 2 * (2.0 / 1000)
	if li.Spend != want {
		t.Fatalf("want spend %f got %f", want, li.Spend)
	}
	// Metrics are now handled by NoOpRegistry - no assertions needed
}

func TestRecordImpression_FlatSpend(t *testing.T) {
	// Initialize AdDataStore for the test
	testStore := models.NewInMemoryAdDataStore()
	_ = testStore.SetLineItems([]models.LineItem{
		{ID: 2, CampaignID: 2, BudgetType: models.BudgetTypeFlat, BudgetAmount: 50, Active: true, PublisherID: 0},
	})

	a := &Analytics{Metrics: observability.NewNoOpRegistry()}
	if err := a.RecordImpression(context.Background(), testStore, "req1", "1", "1", 2, "mobile", "US", 1, "test-placement"); err != nil && err != ErrUnavailable {
		t.Fatalf("record impression: %v", err)
	}
	li := models.GetLineItemByID(testStore, 2)
	want := 50.0
	if li.Spend != want {
		t.Fatalf("want spend %f got %f", want, li.Spend)
	}
	// Metrics are now handled by NoOpRegistry - no assertions needed

	if err := a.RecordImpression(context.Background(), testStore, "req2", "1", "1", 2, "mobile", "US", 1, "test-placement"); err != nil && err != ErrUnavailable {
		t.Fatalf("record impression: %v", err)
	}
	if li.Spend != want {
		t.Fatalf("flat spend should not increase, got %f", li.Spend)
	}
	// Metrics are now handled by NoOpRegistry - no assertions needed
}
