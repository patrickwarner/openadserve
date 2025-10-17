package selectors

import (
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"
)

// populateCreativeLineItems populates the LineItem cache for test creatives
func populateCreativeLineItems(creatives []models.Creative, dataStore models.AdDataStore) []models.Creative {
	for i := range creatives {
		creatives[i].LineItem = dataStore.GetLineItem(creatives[i].PublisherID, creatives[i].LineItemID)
	}
	return creatives
}

// createTestDB creates a test database with LineItem cache populated and indexes built
func createTestDB(creatives []models.Creative, placements map[string]models.Placement) *db.DB {
	database := &db.DB{
		Creatives:  creatives,
		Placements: placements,
	}

	// Build indexes and populate LineItem cache
	database.BuildIndexes()
	return database
}
