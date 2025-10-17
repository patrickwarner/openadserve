package selectors

import (
	"testing"

	"github.com/patrickwarner/openadserve/internal/geoip"
	logic "github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
)

// TestSelectAd_CountryTargeting verifies that campaigns with a country
// restriction only serve to requests from that country.
func TestSelectAd_CountryTargeting(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	// --- GeoIP Initialization (no CWD change needed) ---
	g, err := geoip.Init("../../geoip/testdata/GeoLite2-Country.mmdb")
	if err != nil {
		t.Fatalf("geoip init: %v", err)
	}
	defer func() { _ = g.Close() }()

	// --- Setup test-specific models and DB instance ---

	// Setup test-specific LineItem (simplified for country targeting)
	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 106, CampaignID: 106, Country: "US", PaceType: models.PacingASAP, CPM: 2.0, ECPM: 2.0, Active: true, PublisherID: 0},
	})

	// Setup test-specific Campaign
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 106, Name: "Test Campaign US"},
	})
	// No BuildCampaignIndex needed for campaigns

	// Setup test-specific Creative and Placement for DB
	testCreativeForSidebarUS := models.Creative{
		ID: 600, PlacementID: "sidebar", LineItemID: 106, CampaignID: 106,
		HTML: "Test Ad US", Width: 160, Height: 600, Format: "html",
	}

	testPlacementSidebar := models.Placement{
		ID: "sidebar", Width: 160, Height: 600, Formats: []string{"html"},
	}

	database := createTestDB([]models.Creative{testCreativeForSidebarUS},
		map[string]models.Placement{"sidebar": testPlacementSidebar})

	// --- Test Logic ---
	ctx := logic.ResolveTargeting(g, "", "192.0.2.5") // Should resolve to US
	ad, err := SelectAd(store, database, testDataStore, "sidebar", "user1", 0, 0, ctx, testConfig())
	if err != nil {
		t.Fatalf("expected ad, got error: %v", err)
	}
	if ad.CampaignID != 106 {
		t.Errorf("expected campaign 106, got %d", ad.CampaignID)
	}

	// different country should fail
	ctx = logic.ResolveTargeting(g, "", "198.51.100.5")
	if _, err := SelectAd(store, database, testDataStore, "sidebar", "user1", 0, 0, ctx, testConfig()); err == nil {
		t.Error("expected no ad for non-matching country")
	}
}

// TestSelectAd_RegionTargeting verifies that campaigns with a region
// restriction only serve to requests from that region.
func TestSelectAd_RegionTargeting(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	g, err := geoip.Init("../../geoip/testdata/geo_fallback.json")
	if err != nil {
		t.Fatalf("geoip init: %v", err)
	}
	defer func() { _ = g.Close() }()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 200, CampaignID: 200, Country: "US", Region: "CA", PaceType: models.PacingASAP, CPM: 2.0, ECPM: 2.0, Active: true, PublisherID: 0},
	})

	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 200, Name: "Test Campaign CA"},
	})

	testCreative := models.Creative{
		ID: 601, PlacementID: "sidebar", LineItemID: 200, CampaignID: 200,
		HTML: "Test Ad CA", Width: 160, Height: 600, Format: "html",
	}

	testPlacement := models.Placement{
		ID: "sidebar", Width: 160, Height: 600, Formats: []string{"html"},
	}

	database := createTestDB([]models.Creative{testCreative},
		map[string]models.Placement{"sidebar": testPlacement})

	ctx := logic.ResolveTargeting(g, "", "192.0.2.5") // CA
	ad, err := SelectAd(store, database, testDataStore, "sidebar", "user1", 0, 0, ctx, testConfig())
	if err != nil {
		t.Fatalf("expected ad, got error: %v", err)
	}
	if ad.CampaignID != 200 {
		t.Errorf("expected campaign 200, got %d", ad.CampaignID)
	}

	ctx = logic.ResolveTargeting(g, "", "198.51.100.5") // NY
	if _, err := SelectAd(store, database, testDataStore, "sidebar", "user1", 0, 0, ctx, testConfig()); err == nil {
		t.Error("expected no ad for non-matching region")
	}
}
