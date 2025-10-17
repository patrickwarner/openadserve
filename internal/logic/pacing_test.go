package logic

import (
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/models"
)

func testConfig() config.Config {
	return config.Config{
		PIDKp: 0.3,
		PIDKi: 0.05,
		PIDKd: 0.1,
	}
}

func TestIsLineItemPacingEligible_EvenPacing(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	// Override line items for testing
	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 1, CampaignID: 1, PublisherID: 0, DailyImpressionCap: 100, PaceType: models.PacingEven, CPM: 1.0, ECPM: 1.0, Active: true},
	})

	// Fix current time to 06:00 UTC (25% through day)
	fixed := time.Date(2025, 5, 24, 6, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }

	key := "pacing:serves:1:2025-05-24"

	// Case A: count < allowed (24 < 25) → serve
	if err := ms.Set(key, "24"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err := IsLineItemPacingEligible(store, 0, 1, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected to serve when count < allowed")
	}

	// Case B: count == allowed (25) → block
	if err := ms.Set(key, "25"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err = IsLineItemPacingEligible(store, 0, 1, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected to block when count == allowed")
	}

	// Case C: count > allowed (26) → block
	if err := ms.Set(key, "26"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err = IsLineItemPacingEligible(store, 0, 1, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected to block when count > allowed")
	}
}

func TestIsLineItemPacingEligible_ASAPPacing(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 2, CampaignID: 2, PublisherID: 0, DailyImpressionCap: 3, PaceType: models.PacingASAP, CPM: 1.0, ECPM: 1.0, Active: true},
	})

	// nowFn can be real time here
	nowFn = time.Now

	key := "pacing:serves:2:" + nowFn().Format("2006-01-02")

	// Case A: count < cap (0) → serve
	if err := ms.Set(key, "0"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err := IsLineItemPacingEligible(store, 0, 2, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected to serve when under cap")
	}

	// Case B: count == cap (3) → block
	if err := ms.Set(key, "3"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err = IsLineItemPacingEligible(store, 0, 2, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected to block when count == cap")
	}
}

func TestIsLineItemPacingEligible_FlightDates(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	start := time.Date(2025, 5, 20, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 3, CampaignID: 3, PublisherID: 0, StartDate: start, EndDate: end, DailyImpressionCap: 5, PaceType: models.PacingASAP, CPM: 1.0, ECPM: 1.0, Active: true},
	})

	// Before start date should block
	nowFn = func() time.Time { return start.Add(-time.Hour) }
	ok, err := IsLineItemPacingEligible(store, 0, 3, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected to block before start date")
	}

	// Within flight window should allow when under cap
	nowFn = func() time.Time { return start.Add(12 * time.Hour) }
	key := "pacing:serves:3:" + nowFn().Format("2006-01-02")
	if err := ms.Set(key, "0"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err = IsLineItemPacingEligible(store, 0, 3, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected to serve during flight window")
	}

	// After end date should block
	nowFn = func() time.Time { return end.Add(time.Hour) }
	ok, err = IsLineItemPacingEligible(store, 0, 3, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected to block after end date")
	}
}

func TestIsLineItemPacingEligible_ClickCap(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 4, CampaignID: 4, PublisherID: 0, DailyImpressionCap: 10, DailyClickCap: 2, PaceType: models.PacingASAP, CPM: 1.0, ECPM: 1.0, Active: true},
	})

	nowFn = time.Now
	clickKey := "clicks:lineitem:4:" + nowFn().Format("2006-01-02")
	if err := ms.Set(clickKey, "2"); err != nil {
		t.Fatalf("failed to set click key: %v", err)
	}
	ok, err := IsLineItemPacingEligible(store, 0, 4, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected to block when clicks exceed cap")
	}

	if err := ms.Set(clickKey, "1"); err != nil {
		t.Fatalf("failed to set click key: %v", err)
	}
	paceKey := "pacing:serves:4:" + nowFn().Format("2006-01-02")
	if err := ms.Set(paceKey, "0"); err != nil {
		t.Fatalf("failed to set pace key: %v", err)
	}
	ok, err = IsLineItemPacingEligible(store, 0, 4, models.NewTestAdDataStore(), testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected to serve when clicks below cap")
	}
}

func TestIsLineItemPacingEligible_UnlimitedImpressions(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 5, CampaignID: 5, PublisherID: 0, DailyImpressionCap: 0, PaceType: models.PacingASAP, CPM: 1.0, ECPM: 1.0, Active: true},
	})

	nowFn = time.Now
	key := "pacing:serves:5:" + nowFn().Format("2006-01-02")
	if err := ms.Set(key, "1000"); err != nil {
		t.Fatalf("failed to set pacing key: %v", err)
	}
	ok, err := IsLineItemPacingEligible(store, 0, 5, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected to serve with unlimited impressions")
	}
}

func TestIsLineItemPacingEligible_NilStore(t *testing.T) {
	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{{ID: 99, CampaignID: 99, PublisherID: 0, PaceType: models.PacingASAP, Active: true}})
	ok, err := IsLineItemPacingEligible(nil, 0, 99, testDataStore, testConfig())
	if err != ErrNilRedisStore {
		t.Fatalf("expected ErrNilRedisStore, got %v", err)
	}
	if ok {
		t.Error("expected eligibility to be false with nil store")
	}
}

func TestIsLineItemPacingEligible_PIDPacing_BlockWhenOver(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 6, CampaignID: 6, PublisherID: 0, DailyImpressionCap: 100, PaceType: models.PacingPID, CPM: 1.0, ECPM: 1.0, Active: true},
	})

	fixed := time.Date(2025, 5, 24, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }

	key := "pacing:serves:6:2025-05-24"
	if err := ms.Set(key, "60"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err := IsLineItemPacingEligible(store, 0, 6, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected to block when over target")
	}
}

func TestIsLineItemPacingEligible_PIDPacing_AllowWhenUnder(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 7, CampaignID: 7, PublisherID: 0, DailyImpressionCap: 100, PaceType: models.PacingPID, CPM: 1.0, ECPM: 1.0, Active: true},
	})

	fixed := time.Date(2025, 5, 24, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }

	key := "pacing:serves:7:2025-05-24"
	if err := ms.Set(key, "40"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}
	ok, err := IsLineItemPacingEligible(store, 0, 7, testDataStore, testConfig())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected to allow when under target")
	}
}

func TestIncrementLineItemServes(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	nowFn = func() time.Time { return time.Date(2025, 5, 24, 12, 0, 0, 0, time.UTC) }

	err := IncrementLineItemServes(store, 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key := "pacing:serves:123:2025-05-24"
	val, err := ms.Get(key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if val != "1" {
		t.Errorf("expected 1, got %s", val)
	}

	err = IncrementLineItemServes(store, 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err = ms.Get(key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if val != "2" {
		t.Errorf("expected 2, got %s", val)
	}
}

func TestIncrementLineItemImpressions(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	nowFn = func() time.Time { return time.Date(2025, 5, 24, 12, 0, 0, 0, time.UTC) }

	err := IncrementLineItemImpressions(store, 456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key := "pacing:impressions:456:2025-05-24"
	val, err := ms.Get(key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if val != "1" {
		t.Errorf("expected 1, got %s", val)
	}

	err = IncrementLineItemImpressions(store, 456)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, err = ms.Get(key)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if val != "2" {
		t.Errorf("expected 2, got %s", val)
	}
}

func TestCheckPIDPacing(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	// Fix time to noon (50% through day)
	fixed := time.Date(2025, 5, 24, 12, 0, 0, 0, time.UTC)
	nowFn = func() time.Time { return fixed }
	today := fixed.Format("2006-01-02")

	lineItemID := 999
	capDaily := int64(100)

	// Test case 1: Count at daily cap (hard safety check)
	result := checkPIDPacing(store, lineItemID, 100, capDaily, today, testConfig())
	if result {
		t.Error("expected false when count >= capDaily")
	}

	// Test case 2: Count over daily cap
	result = checkPIDPacing(store, lineItemID, 150, capDaily, today, testConfig())
	if result {
		t.Error("expected false when count > capDaily")
	}

	// Test case 3: Count under target (should allow)
	result = checkPIDPacing(store, lineItemID, 40, capDaily, today, testConfig())
	if !result {
		t.Error("expected true when count under target (40 < 50 at noon)")
	}

	// Test case 4: Count over target (should block)
	result = checkPIDPacing(store, lineItemID, 60, capDaily, today, testConfig())
	if result {
		t.Error("expected false when count over target (60 > 50 at noon)")
	}
}
