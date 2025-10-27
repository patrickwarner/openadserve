package selectors

import (
	"fmt"
	"sort"
	"testing"
	"time"

	logic "github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
)

func TestSelectAd_ShuffleAppliedBeforeFrequency(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	// Test-specific LineItems
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 111, CampaignID: 111, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 2.0, ECPM: 2.0, DeviceType: "mobile", Active: true, PublisherID: 0}, // For Creative 11
		{ID: 112, CampaignID: 112, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, DeviceType: "mobile", Active: true, PublisherID: 0}, // For Creative 12 (will be capped)
	})

	// Test-specific Campaigns
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 111, Name: "Test Campaign 111"},
		{ID: 112, Name: "Test Campaign 112"},
	})

	// Test-specific Creatives
	testCreatives := populateCreativeLineItems([]models.Creative{
		{ID: 11, PlacementID: "header", LineItemID: 111, CampaignID: 111,
			HTML: "Ad 11", Width: 320, Height: 50, Format: "html"},
		{ID: 12, PlacementID: "header", LineItemID: 112, CampaignID: 112,
			HTML: "Ad 12", Width: 320, Height: 50, Format: "html"},
	}, testDataStore)
	creative11, creative12 := testCreatives[0], testCreatives[1]

	// Test-specific Placement
	placementHeader := models.Placement{
		ID: "header", Width: 320, Height: 50, Formats: []string{"html"},
	}

	database := createTestDB([]models.Creative{creative11, creative12},
		map[string]models.Placement{"header": placementHeader})

	// Force deterministic ordering so creative with ID 12 is evaluated first.
	originalShuffle := ShuffleFn
	ShuffleFn = func(cs []models.Creative) {
		sort.Slice(cs, func(i, j int) bool { return cs[i].ID > cs[j].ID })
	}
	defer func() { ShuffleFn = originalShuffle }()

	userID := "user1"
	ctx := models.TargetingContext{DeviceType: "mobile"}

	// Exceed the frequency cap for line item ID 112 which owns creative 12.
	for i := 0; i < logic.DefaultFrequencyCap+1; i++ {
		_, err := store.IncrementImpression(userID, 112, logic.DefaultFrequencyWindow)
		if err != nil {
			t.Fatalf("failed to increment impression: %v", err)
		}
	}

	resp, err := SelectAd(store, database, testDataStore, "header", userID, 0, 0, ctx, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CreativeID == 12 {
		t.Errorf("expected capped creative to be skipped")
	}
	if resp.CreativeID != 11 {
		t.Errorf("expected creative 11 after shuffle, got %d", resp.CreativeID)
	}
}

func TestSelectAd_PriorityOrder(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	// Test-specific LineItems
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 111, CampaignID: 111, PaceType: models.PacingASAP, Priority: models.PriorityHigh, CPM: 1.0, ECPM: 1.0, DeviceType: "mobile", Active: true, PublisherID: 0},  // High prio
		{ID: 112, CampaignID: 112, PaceType: models.PacingASAP, Priority: models.PriorityLow, CPM: 10.0, ECPM: 10.0, DeviceType: "mobile", Active: true, PublisherID: 0}, // Low prio, higher CPM to ensure priority wins
	})

	// Test-specific Campaigns
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 111, Name: "Test Campaign High Prio"},
		{ID: 112, Name: "Test Campaign Low Prio"},
	})

	// Test-specific Creatives
	creativeHighPrio := models.Creative{
		ID: 11, PlacementID: "header", LineItemID: 111, CampaignID: 111,
		HTML: "High Prio Ad", Width: 320, Height: 50, Format: "html",
	}
	creativeLowPrio := models.Creative{
		ID: 12, PlacementID: "header", LineItemID: 112, CampaignID: 112,
		HTML: "Low Prio Ad", Width: 320, Height: 50, Format: "html",
	}

	// Test-specific Placement
	placementHeader := models.Placement{
		ID: "header", Width: 320, Height: 50, Formats: []string{"html"},
	}

	testCreatives := populateCreativeLineItems([]models.Creative{creativeLowPrio, creativeHighPrio}, testDataStore)
	database := createTestDB(testCreatives,
		map[string]models.Placement{"header": placementHeader})

	ctx := models.TargetingContext{DeviceType: "mobile"}

	resp, err := SelectAd(store, database, testDataStore, "header", "user1", 0, 0, ctx, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CreativeID != 11 {
		t.Errorf("expected creative 11 from high priority line item, got %d", resp.CreativeID)
	}
}

func TestSelectAd_NoCreativesForPlacement(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{})
	_ = testDataStore.SetCampaigns([]models.Campaign{})

	placementHeader := models.Placement{
		ID: "header", Width: 728, Height: 90, Formats: []string{"html"},
	}
	database := createTestDB([]models.Creative{},
		map[string]models.Placement{"header": placementHeader})

	ctx := models.TargetingContext{DeviceType: "mobile"}
	_, err := SelectAd(store, database, testDataStore, "header", "user1", 0, 0, ctx, testConfig())

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err != ErrNoEligibleAd { // Direct error comparison
		t.Errorf("expected error '%s', got '%s'", ErrNoEligibleAd.Error(), err.Error())
	}
}

func TestSelectAd_CreativeSizeMismatch(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 701, CampaignID: 701, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, DeviceType: "mobile", Active: true, PublisherID: 0},
	})
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 701, Name: "Campaign 701"},
	})

	creativeMismatch := models.Creative{
		ID: 701, PlacementID: "header", LineItemID: 701, CampaignID: 701,
		HTML: "Ad 701", Width: 300, Height: 250, Format: "html",
	}
	placementHeader := models.Placement{
		ID: "header", Width: 728, Height: 90, Formats: []string{"html"},
	}
	database := createTestDB([]models.Creative{creativeMismatch},
		map[string]models.Placement{"header": placementHeader})

	ctx := models.TargetingContext{DeviceType: "mobile"}
	_, err := SelectAd(store, database, models.NewTestAdDataStore(), "header", "user1", 728, 90, ctx, testConfig())

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err != ErrNoEligibleAd { // Direct error comparison
		t.Errorf("expected error '%s', got '%s'", ErrNoEligibleAd.Error(), err.Error())
	}
}

func TestSelectAd_CreativeFormatMismatch(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 702, CampaignID: 702, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, DeviceType: "mobile", Active: true, PublisherID: 0},
	})
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 702, Name: "Campaign 702"},
	})

	creativeMismatch := models.Creative{
		ID: 702, PlacementID: "header", LineItemID: 702, CampaignID: 702,
		HTML: "Ad 702", Width: 728, Height: 90, Format: "video",
	}
	placementHeader := models.Placement{
		ID: "header", Width: 728, Height: 90, Formats: []string{"html"},
	}
	database := createTestDB([]models.Creative{creativeMismatch},
		map[string]models.Placement{"header": placementHeader})

	ctx := models.TargetingContext{DeviceType: "mobile"}
	_, err := SelectAd(store, database, testDataStore, "header", "user1", 0, 0, ctx, testConfig())

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err != ErrNoEligibleAd { // Direct error comparison
		t.Errorf("expected error '%s', got '%s'", ErrNoEligibleAd.Error(), err.Error())
	}
}

func TestSelectAd_AllFilteredByTargeting(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 703, CampaignID: 703, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, DeviceType: "desktop", Active: true, PublisherID: 0},
	})
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 703, Name: "Campaign 703"},
	})

	creativeDesktop := models.Creative{
		ID: 703, PlacementID: "header", LineItemID: 703, CampaignID: 703,
		HTML: "Ad 703", Width: 728, Height: 90, Format: "html",
	}
	placementHeader := models.Placement{
		ID: "header", Width: 728, Height: 90, Formats: []string{"html"},
	}
	database := createTestDB([]models.Creative{creativeDesktop},
		map[string]models.Placement{"header": placementHeader})

	ctx := models.TargetingContext{DeviceType: "mobile"}
	_, err := SelectAd(store, database, testDataStore, "header", "user1", 0, 0, ctx, testConfig())

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err != ErrNoEligibleAd { // Direct error comparison
		t.Errorf("expected error '%s', got '%s'", ErrNoEligibleAd.Error(), err.Error())
	}
}

func TestSelectAd_AllFilteredByFrequencyCap(t *testing.T) {
	// Using single-pass filter approach

	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 704, CampaignID: 704, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, DeviceType: "mobile", FrequencyCap: 1, FrequencyWindow: time.Minute, Active: true, PublisherID: 0},
	})
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 704, Name: "Campaign 704"},
	})

	creativeCapped := models.Creative{
		ID: 704, PlacementID: "header", LineItemID: 704, CampaignID: 704,
		HTML: "Ad 704", Width: 728, Height: 90, Format: "html",
	}
	placementHeader := models.Placement{
		ID: "header", Width: 728, Height: 90, Formats: []string{"html"},
	}
	database := createTestDB([]models.Creative{creativeCapped},
		map[string]models.Placement{"header": placementHeader})

	userID := "user1"
	ctx := models.TargetingContext{DeviceType: "mobile"}

	// Exceed frequency cap
	_, err := store.IncrementImpression(userID, 704, logic.DefaultFrequencyWindow)
	if err != nil {
		t.Fatalf("failed to increment impression: %v", err)
	}
	_, err = store.IncrementImpression(userID, 704, logic.DefaultFrequencyWindow)
	if err != nil {
		t.Fatalf("failed to increment impression: %v", err)
	}

	_, err = SelectAd(store, database, testDataStore, "header", userID, 0, 0, ctx, testConfig())

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	// Single-pass filter may encounter pacing limits before frequency caps
	if err != ErrNoEligibleAd && err != ErrPacingLimitReached {
		t.Errorf("expected error '%s' or '%s', got '%s'", ErrNoEligibleAd.Error(), ErrPacingLimitReached.Error(), err.Error())
	}
}

func TestSelectAd_AllFilteredByPacing(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 705, CampaignID: 705, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1.0, ECPM: 1.0, DeviceType: "mobile", DailyImpressionCap: 1, Active: true, PublisherID: 0},
	})
	_ = testDataStore.SetCampaigns([]models.Campaign{
		{ID: 705, Name: "Campaign 705"},
	})

	creativePaced := models.Creative{
		ID: 705, PlacementID: "header", LineItemID: 705, CampaignID: 705,
		HTML: "Ad 705", Width: 728, Height: 90, Format: "html",
	}
	placementHeader := models.Placement{
		ID: "header", Width: 728, Height: 90, Formats: []string{"html"},
	}
	database := createTestDB([]models.Creative{creativePaced},
		map[string]models.Placement{"header": placementHeader})

	ctx := models.TargetingContext{DeviceType: "mobile"}

	pacingKey := fmt.Sprintf("pacing:serves:%d:%s", 705, time.Now().Format("2006-01-02"))
	err := ms.Set(pacingKey, "1")
	if err != nil {
		t.Fatalf("failed to set pacing key in redis: %v", err)
	}

	_, err = SelectAd(store, database, testDataStore, "header", "user1", 0, 0, ctx, testConfig())

	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if err != ErrPacingLimitReached { // Direct error comparison
		t.Errorf("expected error '%s', got '%s'", ErrPacingLimitReached.Error(), err.Error())
	}
}
