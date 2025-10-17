package db

import (
	"fmt"

	"github.com/patrickwarner/openadserve/internal/models"
)

// DB holds in-memory creatives and placement definitions loaded from Postgres.
type DB struct {
	Creatives  []models.Creative
	Placements map[string]models.Placement
	Publishers map[int]models.Publisher

	creativeIndexByPlacement map[string][]models.Creative
	creativeIndexByID        map[int]*models.Creative
}

// Init loads creatives and placements from Postgres and validates their
// relationships. A DB instance containing the loaded data is returned.
func Init(pg *Postgres, dataStore models.AdDataStore) (*DB, error) {
	pls, err := pg.LoadPlacements()
	if err != nil {
		return nil, fmt.Errorf("load placements: %w", err)
	}
	placements := make(map[string]models.Placement, len(pls))
	for _, p := range pls {
		placements[p.ID] = p
	}

	pubs, err := pg.LoadPublishers()
	if err != nil {
		return nil, fmt.Errorf("load publishers: %w", err)
	}
	publishers := make(map[int]models.Publisher, len(pubs))
	for _, p := range pubs {
		publishers[p.ID] = p
	}

	creatives, err := pg.LoadCreatives()
	if err != nil {
		return nil, fmt.Errorf("load creatives: %w", err)
	}

	indexByPlacement := make(map[string][]models.Creative)
	indexByID := make(map[int]*models.Creative)

	for i := range creatives {
		cr := &creatives[i]
		var lineItem *models.LineItem
		var campaign *models.Campaign

		lineItem = dataStore.GetLineItem(cr.PublisherID, cr.LineItemID)
		campaign = dataStore.GetCampaign(cr.CampaignID)

		if lineItem == nil {
			return nil, fmt.Errorf("creative %d references undefined line item %d", cr.ID, cr.LineItemID)
		}
		// Cache the LineItem pointer to avoid repeated lookups during ad serving
		cr.LineItem = lineItem

		if campaign == nil {
			return nil, fmt.Errorf("creative %d references undefined campaign %d", cr.ID, cr.CampaignID)
		}
		if _, ok := placements[cr.PlacementID]; !ok {
			return nil, fmt.Errorf("creative %d references undefined placement %s", cr.ID, cr.PlacementID)
		}

		indexByPlacement[cr.PlacementID] = append(indexByPlacement[cr.PlacementID], *cr)
		indexByID[cr.ID] = cr
	}

	return &DB{Creatives: creatives, Placements: placements, Publishers: publishers, creativeIndexByPlacement: indexByPlacement, creativeIndexByID: indexByID}, nil
}

// FindCreativesForPlacement returns all creatives that match a placement ID.
func (d *DB) FindCreativesForPlacement(placementID string) []models.Creative {
	if cs, ok := d.creativeIndexByPlacement[placementID]; ok {
		return cs
	}
	return nil
}

// FindCreativeByID retrieves a creative by its ID.
func (d *DB) FindCreativeByID(id int) *models.Creative {
	if cr, ok := d.creativeIndexByID[id]; ok {
		return cr
	}
	return nil
}

// FindCampaignByID retrieves a campaign by its ID using the models layer.
func FindCampaignByID(store models.AdDataStore, id int) *models.Campaign {
	return models.GetCampaignByID(store, id)
}

// FindLineItemByID retrieves a line item by its ID using the models layer.
func FindLineItemByID(store models.AdDataStore, pubID, id int) *models.LineItem {
	return models.GetLineItem(store, pubID, id)
}

// GetPlacement returns the placement definition for the given ID.
func (d *DB) GetPlacement(id string) (models.Placement, bool) {
	p, ok := d.Placements[id]
	return p, ok
}

// GetPublisher returns the publisher definition for the given ID.
func (d *DB) GetPublisher(id int) (models.Publisher, bool) {
	p, ok := d.Publishers[id]
	return p, ok
}

// BuildIndexes builds the internal indexes for the DB. Used primarily for testing.
func (d *DB) BuildIndexes() {
	indexByPlacement := make(map[string][]models.Creative)
	indexByID := make(map[int]*models.Creative)

	for i := range d.Creatives {
		cr := &d.Creatives[i]
		// LineItem should already be populated during Init()
		indexByPlacement[cr.PlacementID] = append(indexByPlacement[cr.PlacementID], *cr)
		indexByID[cr.ID] = cr
	}

	d.creativeIndexByPlacement = indexByPlacement
	d.creativeIndexByID = indexByID
}
