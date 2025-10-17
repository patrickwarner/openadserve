package selectors

import (
	"testing"

	"github.com/patrickwarner/openadserve/internal/models"
)

func TestLineItemIsolationPerPublisher(t *testing.T) {
	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 101, CampaignID: 1, PublisherID: 1, PaceType: models.PacingASAP, CPM: 1, ECPM: 1, Active: true},
		{ID: 201, CampaignID: 2, PublisherID: 2, PaceType: models.PacingASAP, CPM: 1, ECPM: 1, Active: true},
	})

	li1 := testDataStore.GetLineItem(1, 101)
	if li1 == nil || li1.PublisherID != 1 {
		t.Fatalf("expected line item 101 for publisher 1")
	}
	li2 := testDataStore.GetLineItem(2, 201)
	if li2 == nil || li2.PublisherID != 2 {
		t.Fatalf("expected line item 201 for publisher 2")
	}
	if testDataStore.GetLineItem(1, 201) != nil {
		t.Fatalf("publisher 1 should not see publisher 2 line item")
	}
}
