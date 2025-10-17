package filters

import (
	"context"
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	logic "github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *db.RedisStore) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	store := &db.RedisStore{
		Client: redis.NewClient(&redis.Options{Addr: s.Addr()}),
		Ctx:    context.Background(),
	}
	return s, store
}

func testConfig() config.Config {
	return config.Config{
		PIDKp: 0.3,
		PIDKi: 0.05,
		PIDKd: 0.1,
	}
}

func TestFilterByTargeting(t *testing.T) {
	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{
		{ID: 1, CampaignID: 1, DeviceType: "mobile", PublisherID: 0},
		{ID: 2, CampaignID: 2, DeviceType: "desktop", PublisherID: 0},
	})

	creatives := []models.Creative{
		{ID: 1, LineItemID: 1},
		{ID: 2, LineItemID: 2},
	}

	ctx := models.TargetingContext{DeviceType: "mobile"}
	filtered := FilterByTargeting(creatives, ctx, testDataStore)
	if len(filtered) != 1 || filtered[0].ID != 1 {
		t.Fatalf("expected only creative 1, got %+v", filtered)
	}
}

func TestFilterBySize(t *testing.T) {
	creatives := []models.Creative{
		{ID: 1, Width: 300, Height: 250, Format: "html"},
		{ID: 2, Width: 728, Height: 90, Format: "video"},
	}

	filtered := FilterBySize(creatives, 300, 250, []string{"html"})
	if len(filtered) != 1 || filtered[0].ID != 1 {
		t.Fatalf("expected only creative 1, got %+v", filtered)
	}
}

func TestFilterByFrequency(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{{ID: 1, CampaignID: 1, PublisherID: 0}})

	creatives := []models.Creative{{ID: 1, LineItemID: 1}}
	userID := "u1"

	// exceed frequency cap
	for i := 0; i < logic.DefaultFrequencyCap; i++ {
		_, err := store.IncrementImpression(userID, 1, logic.DefaultFrequencyWindow)
		if err != nil {
			t.Fatalf("increment: %v", err)
		}
	}

	filtered, err := FilterByFrequency(store, creatives, userID, testDataStore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("expected creative to be filtered, got %+v", filtered)
	}
}

func TestFilterByPacing(t *testing.T) {
	ms, store := setupTestRedis(t)
	defer ms.Close()

	testDataStore := models.NewTestAdDataStore()
	_ = testDataStore.SetLineItems([]models.LineItem{{ID: 1, CampaignID: 1, DailyImpressionCap: 1, PaceType: models.PacingASAP, PublisherID: 0}})

	creatives := []models.Creative{{ID: 1, LineItemID: 1}}

	key := "pacing:serves:1:" + time.Now().Format("2006-01-02")
	if err := ms.Set(key, "1"); err != nil {
		t.Fatalf("set: %v", err)
	}

	_, err := FilterByPacing(store, creatives, testDataStore, testConfig())
	if err != ErrPacingLimitReached {
		t.Fatalf("expected ErrPacingLimitReached, got %v", err)
	}
}

func TestFilterByFrequency_NilStore(t *testing.T) {
	creatives := []models.Creative{{ID: 1, LineItemID: 1}}
	_, err := FilterByFrequency(nil, creatives, "user", models.NewTestAdDataStore())
	if err != logic.ErrNilRedisStore {
		t.Fatalf("expected ErrNilRedisStore, got %v", err)
	}
}

func TestFilterByPacing_NilStore(t *testing.T) {
	creatives := []models.Creative{{ID: 1, LineItemID: 1}}
	_, err := FilterByPacing(nil, creatives, models.NewTestAdDataStore(), testConfig())
	if err != logic.ErrNilRedisStore {
		t.Fatalf("expected ErrNilRedisStore, got %v", err)
	}
}
