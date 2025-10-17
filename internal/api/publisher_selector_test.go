package api

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/patrickwarner/openadserve/internal/analytics"
	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/logic/selectors"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// stub SQL driver that always succeeds
type stubDriver struct{}

func (stubDriver) Open(name string) (driver.Conn, error) { return stubConn{}, nil }

type stubConn struct{}

func (stubConn) Prepare(query string) (driver.Stmt, error) { return stubStmt{}, nil }
func (stubConn) Close() error                              { return nil }
func (stubConn) Begin() (driver.Tx, error)                 { return stubTx{}, nil }
func (stubConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return stubResult{}, nil
}

type stubStmt struct{}

func (stubStmt) Close() error                               { return nil }
func (stubStmt) NumInput() int                              { return 0 }
func (stubStmt) Exec([]driver.Value) (driver.Result, error) { return stubResult{}, nil }
func (stubStmt) Query([]driver.Value) (driver.Rows, error)  { return nil, nil }

type stubTx struct{}

func (stubTx) Commit() error   { return nil }
func (stubTx) Rollback() error { return nil }

type stubResult struct{}

func (stubResult) LastInsertId() (int64, error) { return 0, nil }
func (stubResult) RowsAffected() (int64, error) { return 0, nil }

func init() {
	sql.Register("stub", stubDriver{})
}

type recordSelector struct {
	called   *bool
	response *models.AdResponse
}

func (r recordSelector) SelectAd(*db.RedisStore, *db.DB, models.AdDataStore, string, string, int, int, models.TargetingContext, config.Config) (*models.AdResponse, error) {
	if r.called != nil {
		*r.called = true
	}
	return r.response, nil
}

func TestGetAdHandler_SelectorPerPublisher(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	redisStore := &db.RedisStore{
		Client: redisClient,
		Ctx:    context.Background(),
	}

	srv := &Server{
		Logger:      zap.NewNop(),
		Analytics:   analytics.NewMockAnalytics(),
		SelectorMap: map[int]selectors.Selector{},
		TokenSecret: []byte("secret"),
		TokenTTL:    time.Minute,
		Metrics:     observability.NewNoOpRegistry(),
		Store:       redisStore,
	}

	// Initialize AdDataStore for the test
	testStore := models.NewInMemoryAdDataStore()
	srv.AdDataStore = testStore
	models.SetPublishers(testStore, []models.Publisher{{ID: 1, Name: "p1", APIKey: "key1"}, {ID: 2, Name: "p2", APIKey: "key2"}})
	models.SetLineItems(testStore, []models.LineItem{
		{ID: 10, CampaignID: 100, PublisherID: 1, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1, ECPM: 1, Active: true},
		{ID: 20, CampaignID: 200, PublisherID: 2, PaceType: models.PacingASAP, Priority: models.PriorityMedium, CPM: 1, ECPM: 1, Active: true},
	})
	models.SetCampaigns(testStore, []models.Campaign{{ID: 100, PublisherID: 1}, {ID: 200, PublisherID: 2}})

	placement := models.Placement{ID: "slot", Width: 1, Height: 1, Formats: []string{"html"}}
	cr1 := models.Creative{ID: 1, PlacementID: "slot", LineItemID: 10, CampaignID: 100, PublisherID: 1, HTML: "a", Width: 1, Height: 1, Format: "html"}
	cr2 := models.Creative{ID: 2, PlacementID: "slot", LineItemID: 20, CampaignID: 200, PublisherID: 2, HTML: "b", Width: 1, Height: 1, Format: "html"}
	srv.DB = &db.DB{Creatives: []models.Creative{cr1, cr2}, Placements: map[string]models.Placement{"slot": placement}}

	var calledDefault, calledPub bool
	defaultAd := &models.AdResponse{CreativeID: 2, CampaignID: 200, LineItemID: 20, HTML: "b"}
	pubAd := &models.AdResponse{CreativeID: 1, CampaignID: 100, LineItemID: 10, HTML: "a"}

	defaultSel := recordSelector{called: &calledDefault, response: defaultAd}
	pubSel := recordSelector{called: &calledPub, response: pubAd}

	srv.SelectorMap[0] = defaultSel
	srv.RegisterSelector(1, pubSel)

	reqObj := models.OpenRTBRequest{
		ID:   "1",
		Imp:  []models.Impression{{ID: "1", TagID: "slot"}},
		User: models.User{ID: "u"},
		Ext:  models.RequestExt{PublisherID: 1},
	}
	body, _ := json.Marshal(reqObj)
	req := httptest.NewRequest(http.MethodPost, "/ad", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-API-Key", "key1")
	rec := httptest.NewRecorder()
	srv.GetAdHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !calledPub || calledDefault {
		t.Fatalf("expected publisher selector to run")
	}

	calledPub = false
	calledDefault = false
	reqObj.Ext.PublisherID = 2
	body, _ = json.Marshal(reqObj)
	req = httptest.NewRequest(http.MethodPost, "/ad", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-API-Key", "key2")
	rec = httptest.NewRecorder()
	srv.GetAdHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !calledDefault || calledPub {
		t.Fatalf("expected default selector to run for unknown publisher")
	}
}
