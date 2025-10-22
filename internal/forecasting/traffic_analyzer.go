package forecasting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// TrafficPattern represents aggregated traffic data for a specific segment
type TrafficPattern struct {
	TimeWindow    time.Time
	PublisherID   int
	PlacementID   string
	Country       string
	DeviceType    string
	KeyValues     map[string]string
	Opportunities int64
	Impressions   int64
	Clicks        int64
	FillRate      float64
	CTR           float64
}

// analyzeTrafficPatterns queries historical traffic data matching the forecast criteria
func (e *Engine) analyzeTrafficPatterns(ctx context.Context, req *models.ForecastRequest) ([]*TrafficPattern, error) {
	// Build the parameterized query for opportunities (ad_requests)
	query, args := buildTrafficQuery(req)

	rows, err := e.ClickHouse.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ClickHouse for traffic patterns (publisher %d): %w", req.PublisherID, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			e.Logger.Warn("failed to close rows", zap.Error(closeErr))
		}
	}()

	patterns := make([]*TrafficPattern, 0)
	for rows.Next() {
		var p TrafficPattern
		var kvJSON string
		var publisherID *int32
		var placementID *string
		var country *string
		var deviceType *string

		err := rows.Scan(
			&p.TimeWindow,
			&publisherID,
			&placementID,
			&country,
			&deviceType,
			&kvJSON,
			&p.Opportunities,
			&p.Impressions,
			&p.Clicks,
		)
		if err != nil {
			return nil, fmt.Errorf("scan traffic pattern: %w", err)
		}

		// Convert nullable fields
		if publisherID != nil {
			p.PublisherID = int(*publisherID)
		}
		if placementID != nil {
			p.PlacementID = *placementID
		}
		if country != nil {
			p.Country = *country
		}
		if deviceType != nil {
			p.DeviceType = *deviceType
		}

		// Calculate rates
		if p.Opportunities > 0 {
			p.FillRate = float64(p.Impressions) / float64(p.Opportunities)
		}
		if p.Impressions > 0 {
			p.CTR = float64(p.Clicks) / float64(p.Impressions)
		}

		// Parse key-values if needed
		if kvJSON != "" && kvJSON != "{}" {
			// ClickHouse returns Map as JSON string
			// For POC, we'll keep it simple
			p.KeyValues = make(map[string]string)
		}

		patterns = append(patterns, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate traffic patterns: %w", err)
	}

	e.Logger.Info("analyzed traffic patterns",
		zap.Int("pattern_count", len(patterns)),
		zap.Int("publisher_id", req.PublisherID),
	)

	return patterns, nil
}

// buildTrafficQuery constructs the ClickHouse query for traffic analysis with parameterized inputs
func buildTrafficQuery(req *models.ForecastRequest) (string, []interface{}) {
	// Base query joins ad_requests with impressions and clicks
	query := `
	WITH opportunities AS (
		SELECT 
			toStartOfFifteenMinutes(timestamp) as time_window,
			publisher_id,
			placement_id,
			request_id,
			imp_id,
			device_type,
			country,
			key_values
		FROM events
		WHERE 
			event_type = 'ad_request'
			AND publisher_id = ?
			AND timestamp >= now() - INTERVAL 30 DAY
	),
	impressions AS (
		SELECT 
			request_id,
			COUNT(*) as impression_count
		FROM events
		WHERE 
			event_type = 'impression'
			AND timestamp >= now() - INTERVAL 30 DAY
		GROUP BY request_id
	),
	clicks AS (
		SELECT 
			request_id,
			COUNT(*) as click_count
		FROM events
		WHERE 
			event_type = 'click'
			AND timestamp >= now() - INTERVAL 30 DAY
		GROUP BY request_id
	)
	SELECT 
		o.time_window,
		o.publisher_id,
		o.placement_id,
		o.country,
		o.device_type,
		toString(o.key_values) as key_values_json,
		COUNT(DISTINCT o.request_id) as opportunities,
		SUM(COALESCE(i.impression_count, 0)) as impressions,
		SUM(COALESCE(c.click_count, 0)) as clicks
	FROM opportunities o
	LEFT JOIN impressions i ON o.request_id = i.request_id
	LEFT JOIN clicks c ON o.request_id = c.request_id
	WHERE 1=1
	`

	// Add targeting filters with parameters
	conditions := []string{}
	args := []interface{}{req.PublisherID}

	if len(req.Countries) > 0 {
		placeholders := make([]string, len(req.Countries))
		for i, c := range req.Countries {
			placeholders[i] = "?"
			args = append(args, c)
		}
		conditions = append(conditions, fmt.Sprintf("AND o.country IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(req.DeviceTypes) > 0 {
		placeholders := make([]string, len(req.DeviceTypes))
		for i, d := range req.DeviceTypes {
			placeholders[i] = "?"
			args = append(args, d)
		}
		conditions = append(conditions, fmt.Sprintf("AND o.device_type IN (%s)", strings.Join(placeholders, ",")))
	}

	// Add placement_id filters with parameters
	if len(req.PlacementIDs) > 0 {
		placeholders := make([]string, len(req.PlacementIDs))
		for i, p := range req.PlacementIDs {
			placeholders[i] = "?"
			args = append(args, p)
		}
		conditions = append(conditions, fmt.Sprintf("AND o.placement_id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Add key-value filters with parameters
	for k, v := range req.KeyValues {
		conditions = append(conditions, "AND o.key_values[?] = ?")
		args = append(args, k, v)
	}

	// Append conditions and group by
	if len(conditions) > 0 {
		query += "\n" + strings.Join(conditions, "\n")
	}

	query += `
	GROUP BY 
		o.time_window,
		o.publisher_id,
		o.placement_id,
		o.country,
		o.device_type,
		o.key_values
	ORDER BY o.time_window DESC
	LIMIT 10000`

	return query, args
}

// aggregatePatternsByDay aggregates 15-minute patterns into daily totals
func aggregatePatternsByDay(patterns []*TrafficPattern) map[string]*TrafficPattern {
	daily := make(map[string]*TrafficPattern)

	for _, p := range patterns {
		day := p.TimeWindow.Format("2006-01-02")

		if existing, ok := daily[day]; ok {
			existing.Opportunities += p.Opportunities
			existing.Impressions += p.Impressions
			existing.Clicks += p.Clicks
		} else {
			daily[day] = &TrafficPattern{
				TimeWindow:    p.TimeWindow.Truncate(24 * time.Hour),
				PublisherID:   p.PublisherID,
				PlacementID:   p.PlacementID,
				Country:       p.Country,
				DeviceType:    p.DeviceType,
				KeyValues:     p.KeyValues,
				Opportunities: p.Opportunities,
				Impressions:   p.Impressions,
				Clicks:        p.Clicks,
			}
		}
	}

	// Recalculate rates for daily aggregates
	for _, p := range daily {
		if p.Opportunities > 0 {
			p.FillRate = float64(p.Impressions) / float64(p.Opportunities)
		}
		if p.Impressions > 0 {
			p.CTR = float64(p.Clicks) / float64(p.Impressions)
		}
	}

	return daily
}
