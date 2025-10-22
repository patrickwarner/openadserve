package analytics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
)

// AnalyticsService defines the interface for analytics operations.
// Implementations should handle cases where underlying storage is unavailable
// by returning ErrUnavailable.
type AnalyticsService interface {
	// RecordEvent records a custom analytics event with targeting context.
	RecordEvent(ctx context.Context, store models.AdDataStore, eventType, requestID, impID, creativeID string, lineItemID int, cost float64, targetingCtx models.TargetingContext, publisherID int, placementID string) error
	// RecordImpression is a convenience wrapper for impression events.
	RecordImpression(ctx context.Context, store models.AdDataStore, requestID, impID, creativeID string, lineItemID int, deviceType, country string, publisherID int, placementID string) error
	// RecordClick is a convenience wrapper for click events and CPC spend.
	RecordClick(ctx context.Context, store models.AdDataStore, requestID, impID, creativeID string, lineItemID int, deviceType, country string, publisherID int, placementID string) error
}

// Analytics wraps a ClickHouse DB connection.
type Analytics struct {
	DB          *sql.DB
	PG          *db.Postgres
	AdDataStore models.AdDataStore
	Metrics     observability.MetricsRegistry
}

// EventRecord mirrors a row in the events table.
type EventRecord struct {
	Timestamp   time.Time         `json:"timestamp"`
	EventType   string            `json:"event_type"`
	RequestID   string            `json:"request_id"`
	ImpID       string            `json:"imp_id"`
	CreativeID  *int32            `json:"creative_id"`
	CampaignID  *int32            `json:"campaign_id"`
	LineItemID  *int32            `json:"line_item_id"`
	Cost        float64           `json:"cost"`
	DeviceType  *string           `json:"device_type"`
	Country     *string           `json:"country"`
	PublisherID *int32            `json:"publisher_id"`
	PlacementID *string           `json:"placement_id"`
	KeyValues   map[string]string `json:"key_values,omitempty"`
}

// InitClickHouse connects to ClickHouse and ensures the events table exists.
// pg may be nil if spend persistence is not needed.
func InitClickHouse(dsn string, pg *db.Postgres, metrics observability.MetricsRegistry) (*Analytics, error) {
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}
	db.SetMaxOpenConns(25)
	if err := db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}
	create := `CREATE TABLE IF NOT EXISTS events (
       timestamp    DateTime,
       event_type   String,
       request_id   String,
       imp_id       String,
       creative_id  Nullable(Int32),
       campaign_id  Nullable(Int32),
       line_item_id Nullable(Int32),
       cost         Float64,
       device_type  Nullable(String),
       country      Nullable(String),
       publisher_id Nullable(Int32),
       placement_id Nullable(String),
       key_values   Map(String, String)
   ) ENGINE=MergeTree() ORDER BY (event_type, timestamp)`
	if _, err := db.ExecContext(context.Background(), create); err != nil {
		return nil, fmt.Errorf("clickhouse create table: %w", err)
	}

	zap.L().Info("Connected to ClickHouse")
	return &Analytics{DB: db, PG: pg, Metrics: metrics}, nil
}

// SetAdDataStore sets the AdDataStore reference after initialization.
func (a *Analytics) SetAdDataStore(store models.AdDataStore) {
	if a != nil {
		a.AdDataStore = store
	}
}

// RecordEvent inserts a single event row into the events table.
// ErrUnavailable is returned when the analytics DB is not configured.
var ErrUnavailable = fmt.Errorf("analytics unavailable")

func (a *Analytics) RecordEvent(ctx context.Context, store models.AdDataStore, eventType, requestID, impID, creativeID string, lineItemID int, cost float64, targetingCtx models.TargetingContext, publisherID int, placementID string) error {
	if a == nil || a.DB == nil {
		return ErrUnavailable
	}
	var cr sql.NullInt32
	if creativeID != "" {
		if id, err := strconv.Atoi(creativeID); err == nil {
			cr.Int32 = int32(id)
			cr.Valid = true
		}
	}

	// Look up campaign ID from line item ID
	var cmp sql.NullInt32
	var li sql.NullInt32
	if lineItemID > 0 && store != nil {
		if lineItem := models.GetLineItemByID(store, lineItemID); lineItem != nil {
			// Set campaign ID from line item
			cmp.Int32 = int32(lineItem.CampaignID)
			cmp.Valid = true
			// Set line item ID
			li.Int32 = int32(lineItemID)
			li.Valid = true
		}
	}

	// Handle contextual fields from targeting context
	var dt sql.NullString
	if targetingCtx.DeviceType != "" {
		dt.String = targetingCtx.DeviceType
		dt.Valid = true
	}

	var co sql.NullString
	if targetingCtx.Country != "" {
		co.String = targetingCtx.Country
		co.Valid = true
	}

	var pub sql.NullInt32
	if publisherID > 0 {
		pub.Int32 = int32(publisherID)
		pub.Valid = true
	}

	keyValues := targetingCtx.KeyValues

	// Handle placement_id
	var pid sql.NullString
	if placementID != "" {
		pid.String = placementID
		pid.Valid = true
	}

	stmt := `INSERT INTO events (timestamp, event_type, request_id, imp_id, creative_id, campaign_id, line_item_id, cost, device_type, country, publisher_id, placement_id, key_values) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := a.DB.ExecContext(ctx, stmt, time.Now(), eventType, requestID, impID, cr, cmp, li, cost, dt, co, pub, pid, keyValues); err != nil {
		zap.L().Error("clickhouse insert failed", zap.Error(err), zap.String("event_type", eventType))
		return fmt.Errorf("insert %s event: %w", eventType, err)
	}
	return nil
}

// RecordImpression is a convenience wrapper for RecordEvent.
func (a *Analytics) RecordImpression(ctx context.Context, store models.AdDataStore, requestID, impID, creativeID string, lineItemID int, deviceType, country string, publisherID int, placementID string) error {
	// Look up the line item once so we can calculate cost and update spend.
	var li *models.LineItem
	if lineItemID > 0 {
		li = models.GetLineItemByID(store, lineItemID)
	}

	// Calculate how much this impression costs based on budget type.
	var cost float64
	if li != nil {
		switch li.BudgetType {
		case models.BudgetTypeFlat:
			// A flat-budget line item spends the entire budget on the first impression.
			if li.Spend == 0 {
				cost = li.BudgetAmount
			}
		default:
			// CPM line items charge per thousand impressions.
			cost = li.CPM / 1000
		}
	}

	// Create targeting context for impression
	targetingCtx := models.TargetingContext{
		DeviceType: deviceType,
		Country:    country,
		KeyValues:  make(map[string]string), // Impressions don't have key-values by default
	}

	// Persist the impression event along with its cost.
	if err := a.RecordEvent(ctx, store, "impression", requestID, impID, creativeID, lineItemID, cost, targetingCtx, publisherID, placementID); err != nil {
		// still update spend tracking but surface the error
		if li != nil {
			switch li.BudgetType {
			case models.BudgetTypeFlat:
				if li.Spend == 0 {
					li.Spend = li.BudgetAmount
				}
			default:
				li.Spend += li.CPM / 1000
			}
			a.Metrics.SetSpendTotal(strconv.Itoa(li.CampaignID), li.Spend)
			a.saveSpend(li)
		}
		if errors.Is(err, ErrUnavailable) {
			return ErrUnavailable
		}
		return fmt.Errorf("record impression: %w", err)
	}

	// Update in-memory spend tracking and the Prometheus metric.
	if li != nil {
		switch li.BudgetType {
		case models.BudgetTypeFlat:
			if li.Spend == 0 {
				li.Spend = li.BudgetAmount
			}
		default:
			li.Spend += li.CPM / 1000
		}
		a.Metrics.SetSpendTotal(strconv.Itoa(li.CampaignID), li.Spend)
		a.saveSpend(li)
	}
	return nil
}

// RecordClick is a convenience wrapper for click events and CPC spend.
func (a *Analytics) RecordClick(ctx context.Context, store models.AdDataStore, requestID, impID, creativeID string, lineItemID int, deviceType, country string, publisherID int, placementID string) error {
	var li *models.LineItem
	if lineItemID > 0 {
		li = models.GetLineItemByID(store, lineItemID)
	}

	var cost float64
	if li != nil && li.BudgetType == models.BudgetTypeCPC {
		cost = li.CPC
	}

	// Create targeting context for click
	targetingCtx := models.TargetingContext{
		DeviceType: deviceType,
		Country:    country,
		KeyValues:  make(map[string]string), // Clicks don't have key-values by default
	}

	if err := a.RecordEvent(ctx, store, "click", requestID, impID, creativeID, lineItemID, cost, targetingCtx, publisherID, placementID); err != nil {
		if li != nil && li.BudgetType == models.BudgetTypeCPC {
			li.Spend += li.CPC
			a.Metrics.SetSpendTotal(strconv.Itoa(li.CampaignID), li.Spend)
			a.saveSpend(li)
		}
		if errors.Is(err, ErrUnavailable) {
			return ErrUnavailable
		}
		return fmt.Errorf("record click: %w", err)
	}

	if li != nil && li.BudgetType == models.BudgetTypeCPC {
		li.Spend += li.CPC
		a.Metrics.SetSpendTotal(strconv.Itoa(li.CampaignID), li.Spend)
		a.saveSpend(li)
	}
	return nil
}

// saveSpend persists line item spend to the data store if configured.
func (a *Analytics) saveSpend(li *models.LineItem) {
	if a == nil || li == nil {
		return
	}

	if a.AdDataStore == nil {
		zap.L().Error("AdDataStore not configured for spend updates",
			zap.Int("line_item_id", li.ID))
		a.Metrics.IncrementSpendPersistErrors()
		return
	}

	if err := a.AdDataStore.UpdateLineItemSpend(li.PublisherID, li.ID, li.Spend); err != nil {
		zap.L().Error("update spend via data store", zap.Error(err), zap.Int("line_item_id", li.ID))
		a.Metrics.IncrementSpendPersistErrors()
	}
}

// Close terminates the ClickHouse connection.
func (a *Analytics) Close() {
	if a != nil && a.DB != nil {
		if err := a.DB.Close(); err != nil {
			zap.L().Error("clickhouse close", zap.Error(err))
		}
	}
}

// GetEventsByRequestID returns all events for a given request ID ordered by timestamp.
func (a *Analytics) GetEventsByRequestID(id string) ([]EventRecord, error) {
	if a == nil || a.DB == nil {
		return nil, ErrUnavailable
	}
	query := `SELECT timestamp, event_type, request_id, imp_id, creative_id, campaign_id, line_item_id, cost, device_type, country, publisher_id, placement_id FROM events WHERE request_id=? ORDER BY timestamp`
	rows, err := a.DB.QueryContext(context.Background(), query, id)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			zap.L().Warn("rows close", zap.Error(err))
		}
	}()

	var events []EventRecord
	for rows.Next() {
		var ev EventRecord
		if err := rows.Scan(&ev.Timestamp, &ev.EventType, &ev.RequestID, &ev.ImpID, &ev.CreativeID, &ev.CampaignID, &ev.LineItemID, &ev.Cost, &ev.DeviceType, &ev.Country, &ev.PublisherID, &ev.PlacementID); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return events, nil
}
