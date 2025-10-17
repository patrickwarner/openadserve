package models

import (
	"testing"
	"time"
)

func TestInMemoryAdDataStore_UpdateLineItemSpend(t *testing.T) {
	store := NewInMemoryAdDataStore()

	// Create test data
	lineItem := LineItem{
		ID:          1,
		PublisherID: 100,
		Name:        "Test Line Item",
		Spend:       0.0,
		StartDate:   time.Now(),
		EndDate:     time.Now().Add(24 * time.Hour),
	}

	// Set initial data
	err := store.SetLineItems([]LineItem{lineItem})
	if err != nil {
		t.Fatalf("Failed to set line items: %v", err)
	}

	// Update spend
	newSpend := 25.50
	err = store.UpdateLineItemSpend(100, 1, newSpend)
	if err != nil {
		t.Fatalf("Failed to update spend: %v", err)
	}

	// Verify spend was updated
	retrievedItem := store.GetLineItem(100, 1)
	if retrievedItem == nil {
		t.Fatal("Line item not found after spend update")
	}
	if retrievedItem.Spend != newSpend {
		t.Errorf("Expected spend %f, got %f", newSpend, retrievedItem.Spend)
	}
}

func TestInMemoryAdDataStore_UpdateLineItemsSpend(t *testing.T) {
	store := NewInMemoryAdDataStore()

	// Create test data
	lineItems := []LineItem{
		{
			ID:          1,
			PublisherID: 100,
			Name:        "Test Line Item 1",
			Spend:       0.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
		{
			ID:          2,
			PublisherID: 100,
			Name:        "Test Line Item 2",
			Spend:       0.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
		{
			ID:          3,
			PublisherID: 200,
			Name:        "Test Line Item 3",
			Spend:       0.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
	}

	// Set initial data
	err := store.SetLineItems(lineItems)
	if err != nil {
		t.Fatalf("Failed to set line items: %v", err)
	}

	// Update multiple spends
	updates := map[int]float64{
		1: 10.25,
		2: 20.50,
		3: 30.75,
	}

	err = store.UpdateLineItemsSpend(updates)
	if err != nil {
		t.Fatalf("Failed to update multiple spends: %v", err)
	}

	// Verify all spends were updated
	item1 := store.GetLineItem(100, 1)
	if item1 == nil || item1.Spend != 10.25 {
		t.Errorf("Line item 1 spend not updated correctly. Expected 10.25, got %f", item1.Spend)
	}

	item2 := store.GetLineItem(100, 2)
	if item2 == nil || item2.Spend != 20.50 {
		t.Errorf("Line item 2 spend not updated correctly. Expected 20.50, got %f", item2.Spend)
	}

	item3 := store.GetLineItem(200, 3)
	if item3 == nil || item3.Spend != 30.75 {
		t.Errorf("Line item 3 spend not updated correctly. Expected 30.75, got %f", item3.Spend)
	}
}

func TestInMemoryAdDataStore_UpdateLineItemsSpendEmpty(t *testing.T) {
	store := NewInMemoryAdDataStore()

	// Test with empty updates map
	err := store.UpdateLineItemsSpend(map[int]float64{})
	if err != nil {
		t.Errorf("Expected no error for empty updates map, got: %v", err)
	}
}

func TestInMemoryAdDataStore_UpdateLineItemSpendNonExistent(t *testing.T) {
	store := NewInMemoryAdDataStore()

	// Try to update spend for non-existent line item
	err := store.UpdateLineItemSpend(999, 999, 100.0)
	if err != nil {
		t.Errorf("Expected no error for non-existent line item, got: %v", err)
	}

	// Verify no line item was created
	item := store.GetLineItem(999, 999)
	if item != nil {
		t.Error("Expected no line item to be created for non-existent update")
	}
}

func TestInMemoryAdDataStore_UpdateLineItemSpendAtomicity(t *testing.T) {
	store := NewInMemoryAdDataStore()

	// Create test data
	lineItems := []LineItem{
		{
			ID:          1,
			PublisherID: 100,
			Name:        "Test Line Item 1",
			Spend:       5.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
		{
			ID:          2,
			PublisherID: 100,
			Name:        "Test Line Item 2",
			Spend:       10.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
	}

	err := store.SetLineItems(lineItems)
	if err != nil {
		t.Fatalf("Failed to set line items: %v", err)
	}

	// Update one item's spend
	err = store.UpdateLineItemSpend(100, 1, 15.0)
	if err != nil {
		t.Fatalf("Failed to update spend: %v", err)
	}

	// Verify the updated item has new spend
	item1 := store.GetLineItem(100, 1)
	if item1 == nil || item1.Spend != 15.0 {
		t.Errorf("Expected item 1 spend to be 15.0, got %f", item1.Spend)
	}

	// Verify the other item's spend is unchanged
	item2 := store.GetLineItem(100, 2)
	if item2 == nil || item2.Spend != 10.0 {
		t.Errorf("Expected item 2 spend to remain 10.0, got %f", item2.Spend)
	}
}

func TestInMemoryAdDataStore_UpdateLineItemsSpendPartialUpdates(t *testing.T) {
	store := NewInMemoryAdDataStore()

	// Create test data with multiple publishers
	lineItems := []LineItem{
		{
			ID:          1,
			PublisherID: 100,
			Name:        "Publisher 100 - Item 1",
			Spend:       0.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
		{
			ID:          2,
			PublisherID: 100,
			Name:        "Publisher 100 - Item 2",
			Spend:       0.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
		{
			ID:          3,
			PublisherID: 200,
			Name:        "Publisher 200 - Item 3",
			Spend:       0.0,
			StartDate:   time.Now(),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
	}

	err := store.SetLineItems(lineItems)
	if err != nil {
		t.Fatalf("Failed to set line items: %v", err)
	}

	// Update only some items
	updates := map[int]float64{
		1: 100.0, // Update item 1
		3: 300.0, // Update item 3, skip item 2
	}

	err = store.UpdateLineItemsSpend(updates)
	if err != nil {
		t.Fatalf("Failed to update spends: %v", err)
	}

	// Verify updated items
	item1 := store.GetLineItem(100, 1)
	if item1 == nil || item1.Spend != 100.0 {
		t.Errorf("Expected item 1 spend to be 100.0, got %f", item1.Spend)
	}

	item3 := store.GetLineItem(200, 3)
	if item3 == nil || item3.Spend != 300.0 {
		t.Errorf("Expected item 3 spend to be 300.0, got %f", item3.Spend)
	}

	// Verify non-updated item remains unchanged
	item2 := store.GetLineItem(100, 2)
	if item2 == nil || item2.Spend != 0.0 {
		t.Errorf("Expected item 2 spend to remain 0.0, got %f", item2.Spend)
	}
}
