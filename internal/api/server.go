package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"database/sql"

	"github.com/patrickwarner/openadserve/internal/analytics"
	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/forecasting"
	"github.com/patrickwarner/openadserve/internal/geoip"
	"github.com/patrickwarner/openadserve/internal/logic/selectors"
	"github.com/patrickwarner/openadserve/internal/macros"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"

	"go.uber.org/zap"
)

const (
	defaultCTRDefault = 0.5
	ctrWeightDefault  = 2.0
)

var (
	defaultCTR = defaultCTRDefault
	ctrWeight  = ctrWeightDefault
)

func init() {
	if v := os.Getenv("DEFAULT_CTR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			defaultCTR = f
		}
	}
	if v := os.Getenv("CTR_WEIGHT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			ctrWeight = f
		}
	}
}

// Server groups dependencies for HTTP handlers.
type Server struct {
	Logger         *zap.Logger
	Store          *db.RedisStore
	DB             *db.DB
	PG             *db.Postgres
	ClickHouseDB   *sql.DB
	Analytics      analytics.AnalyticsService
	GeoIP          *geoip.GeoIP
	SelectorMap    map[int]selectors.Selector
	DebugTrace     bool
	TokenSecret    []byte
	TokenTTL       time.Duration
	reloadMu       sync.Mutex
	AdDataStore    models.AdDataStore
	Metrics        observability.MetricsRegistry
	Config         config.Config
	MacroService   *macros.Service
	ForecastEngine *forecasting.Engine
}

// NewServer constructs a Server.
func NewServer(logger *zap.Logger, store *db.RedisStore, database *db.DB, pg *db.Postgres, ch *sql.DB, analytics analytics.AnalyticsService, geo *geoip.GeoIP, selector selectors.Selector, debug bool, secret []byte, ttl time.Duration, adDataStore models.AdDataStore, metrics observability.MetricsRegistry, cfg config.Config) *Server {
	if selector == nil {
		rs := selectors.NewRuleBasedSelector()
		rs.SetCTROptimizationEnabled(cfg.CTROptimizationEnabled)
		selector = rs
	}

	return &Server{
		Logger:       logger,
		Store:        store,
		DB:           database,
		PG:           pg,
		ClickHouseDB: ch,
		Analytics:    analytics,
		GeoIP:        geo,
		SelectorMap:  map[int]selectors.Selector{0: selector},
		DebugTrace:   debug,
		TokenSecret:  secret,
		TokenTTL:     ttl,
		AdDataStore:  adDataStore,
		Metrics:      metrics,
		Config:       cfg,
		MacroService: macros.NewService(logger),
	}
}

const AdDataUpdateChannel = "ad-data-updates"

type UpdateMessage struct {
	Entity string `json:"entity"`
	Action string `json:"action"`
	ID     any    `json:"id"`
}

func (s *Server) notifyUpdate(entity string, action string, id any) {
	if s.Store == nil || s.Store.Client == nil {
		s.Logger.Warn("redis store not available, skipping update notification")
		return
	}
	msg := UpdateMessage{Entity: entity, Action: action, ID: id}
	payload, err := json.Marshal(msg)
	if err != nil {
		s.Logger.Error("failed to marshal update message", zap.Error(err))
		return
	}

	ctx := context.Background()
	if err := s.Store.Client.Publish(ctx, AdDataUpdateChannel, payload).Err(); err != nil {
		s.Logger.Error("failed to publish update message", zap.Error(err))
	}
}

// RegisterSelector associates a Selector with a publisher ID. A selector
// registered for ID 0 acts as the default when no specific publisher mapping
// exists.
func (s *Server) RegisterSelector(pubID int, selector selectors.Selector) {
	if s.SelectorMap == nil {
		s.SelectorMap = make(map[int]selectors.Selector)
	}
	if selector == nil {
		selector = selectors.NewRuleBasedSelector()
	}
	s.SelectorMap[pubID] = selector
}

// Reload refreshes campaigns, line items and creatives from Postgres.
func (s *Server) Reload() error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	if s.PG == nil {
		return fmt.Errorf("postgres unavailable")
	}

	items, err := s.PG.LoadLineItems()
	if err != nil {
		return fmt.Errorf("load line items: %w", err)
	}

	campaigns, err := s.PG.LoadCampaigns()
	if err != nil {
		return fmt.Errorf("load campaigns: %w", err)
	}

	publishers, err := s.PG.LoadPublishers()
	if err != nil {
		return fmt.Errorf("load publishers: %w", err)
	}

	placements, err := s.PG.LoadPlacements()
	if err != nil {
		return fmt.Errorf("load placements: %w", err)
	}

	// Use AdDataStore for atomic reload of all data
	if err := s.AdDataStore.ReloadAll(items, campaigns, publishers, placements); err != nil {
		return fmt.Errorf("reload ad data: %w", err)
	}

	database, err := db.Init(s.PG, s.AdDataStore)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	s.DB = database

	s.UpdateCTR()

	return nil
}

// UpdateCTR recalculates CTR and eCPM for CPC line items.
func (s *Server) UpdateCTR() {
	if s.Store == nil || s.Store.Client == nil {
		return
	}

	// Get all publisher IDs that have line items
	publisherIDs := s.AdDataStore.GetAllPublisherIDs()

	updates := make(map[int]float64)

	for _, pubID := range publisherIDs {
		lineItems := s.AdDataStore.GetLineItemsByPublisher(pubID)
		if lineItems == nil {
			continue
		}

		for _, li := range lineItems {
			imps, clicks := s.Store.GetCTRCounts(li.ID)
			ctr := (float64(clicks) + defaultCTR*ctrWeight) / (float64(imps) + ctrWeight)
			if li.BudgetType == models.BudgetTypeCPC {
				updates[li.ID] = li.CPC * ctr * 1000
			}
		}
	}

	if len(updates) > 0 {
		if err := s.AdDataStore.UpdateLineItemsECPM(updates); err != nil {
			zap.L().Error("failed to bulk update line item eCPM", zap.Error(err))
		}
	}
}
