// Package reporting provides campaign performance reporting functionality.
// It queries ClickHouse analytics data to generate comprehensive reports
// including metrics, daily breakdowns, creative performance, and line item analysis.
package reporting

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CampaignMetrics represents performance metrics for a campaign over a specific time period.
// All financial metrics are in USD. CTR is expressed as a percentage (0-100).
type CampaignMetrics struct {
	CampaignID  int       `json:"campaign_id"` // Campaign identifier
	Date        time.Time `json:"date"`        // Date for daily metrics, current time for totals
	Impressions int64     `json:"impressions"` // Total ad impressions served
	Clicks      int64     `json:"clicks"`      // Total clicks received
	Spend       float64   `json:"spend"`       // Total amount spent in USD
	CTR         float64   `json:"ctr"`         // Click-through rate as percentage (clicks/impressions * 100)
	CPM         float64   `json:"cpm"`         // Cost per mille (cost per 1000 impressions) in USD
	CPC         float64   `json:"cpc"`         // Cost per click in USD
}

// CampaignSummary contains comprehensive campaign performance data including
// overall metrics, daily breakdowns, creative performance analysis, and line item breakdowns.
type CampaignSummary struct {
	CampaignID      int               `json:"campaign_id"`       // Campaign identifier
	TotalMetrics    CampaignMetrics   `json:"total_metrics"`     // Aggregated metrics for the entire reporting period
	DailyMetrics    []CampaignMetrics `json:"daily_metrics"`     // Day-by-day performance breakdown
	TopCreatives    []CreativeMetrics `json:"top_creatives"`     // Top performing creatives ranked by CTR
	LineItemMetrics []LineItemMetrics `json:"line_item_metrics"` // Performance breakdown by line item
}

// CreativeMetrics represents performance metrics for individual creatives within a campaign.
// Used to identify top and bottom performing creative assets.
type CreativeMetrics struct {
	CreativeID  int     `json:"creative_id"` // Creative asset identifier
	Impressions int64   `json:"impressions"` // Total impressions for this creative
	Clicks      int64   `json:"clicks"`      // Total clicks for this creative
	CTR         float64 `json:"ctr"`         // Click-through rate as percentage
	Spend       float64 `json:"spend"`       // Total spend for this creative in USD
}

// LineItemMetrics represents performance metrics for individual line items within a campaign.
// Used to analyze delivery performance and budget utilization across different line items.
type LineItemMetrics struct {
	LineItemID  int     `json:"line_item_id"` // Line item identifier
	Impressions int64   `json:"impressions"`  // Total impressions for this line item
	Clicks      int64   `json:"clicks"`       // Total clicks for this line item
	Spend       float64 `json:"spend"`        // Total spend for this line item in USD
	CTR         float64 `json:"ctr"`          // Click-through rate as percentage
	CPM         float64 `json:"cpm"`          // Cost per mille (cost per 1000 impressions) in USD
	CPC         float64 `json:"cpc"`          // Cost per click in USD
	BudgetType  string  `json:"budget_type"`  // Budget type: "cpm", "cpc", or "flat"
}

// GenerateCampaignReport queries ClickHouse for campaign performance data and
// assembles a comprehensive report including daily metrics, totals, and creative performance.
// Returns a CampaignSummary with all calculated metrics and insights.
func GenerateCampaignReport(ctx context.Context, db *sql.DB, campaignID int, days int) (*CampaignSummary, error) {
	summary := &CampaignSummary{
		CampaignID: campaignID,
	}

	// Get daily metrics from ClickHouse
	dailyMetrics, err := getDailyMetrics(ctx, db, campaignID, days)
	if err != nil {
		return nil, fmt.Errorf("get daily metrics: %w", err)
	}
	summary.DailyMetrics = dailyMetrics

	// Calculate aggregated total metrics from daily data
	totalMetrics := CampaignMetrics{
		CampaignID: campaignID,
		Date:       time.Now(),
	}

	for _, dm := range dailyMetrics {
		totalMetrics.Impressions += dm.Impressions
		totalMetrics.Clicks += dm.Clicks
		totalMetrics.Spend += dm.Spend
	}

	// Calculate derived metrics (CTR, CPM, CPC)
	if totalMetrics.Impressions > 0 {
		totalMetrics.CTR = float64(totalMetrics.Clicks) / float64(totalMetrics.Impressions) * 100
		totalMetrics.CPM = totalMetrics.Spend / float64(totalMetrics.Impressions) * 1000
	}
	if totalMetrics.Clicks > 0 {
		totalMetrics.CPC = totalMetrics.Spend / float64(totalMetrics.Clicks)
	}
	summary.TotalMetrics = totalMetrics

	// Get top performing creatives ranked by CTR
	topCreatives, err := getTopCreatives(ctx, db, campaignID, days, 5)
	if err != nil {
		return nil, fmt.Errorf("get top creatives: %w", err)
	}
	summary.TopCreatives = topCreatives

	// Get line item performance metrics
	lineItemMetrics, err := getLineItemMetrics(ctx, db, campaignID, days)
	if err != nil {
		return nil, fmt.Errorf("get line item metrics: %w", err)
	}
	summary.LineItemMetrics = lineItemMetrics

	return summary, nil
}

// getDailyMetrics queries ClickHouse for daily performance metrics for the specified
// campaign over the given number of days. Returns metrics grouped by date with
// calculated CTR, CPM, and CPC for each day.
func getDailyMetrics(ctx context.Context, db *sql.DB, campaignID int, days int) ([]CampaignMetrics, error) {
	query := `
		SELECT
			toDate(timestamp) as date,
			countIf(event_type = 'impression') as impressions,
			countIf(event_type = 'click') as clicks,
			sum(cost) as spend,
			round(if(impressions > 0, clicks / impressions * 100, 0), 2) as ctr,
			round(if(impressions > 0, spend / impressions * 1000, 0), 2) as cpm,
			round(if(clicks > 0, spend / clicks, 0), 2) as cpc
		FROM events
		WHERE campaign_id = ?
			AND timestamp >= now() - INTERVAL ? DAY
		GROUP BY date
		ORDER BY date DESC`

	rows, err := db.QueryContext(ctx, query, campaignID, days)
	if err != nil {
		return nil, fmt.Errorf("query daily metrics: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var metrics []CampaignMetrics
	for rows.Next() {
		var m CampaignMetrics
		m.CampaignID = campaignID // Set it directly since we're filtering by it
		err := rows.Scan(&m.Date, &m.Impressions, &m.Clicks,
			&m.Spend, &m.CTR, &m.CPM, &m.CPC)
		if err != nil {
			return nil, fmt.Errorf("scan daily metrics: %w", err)
		}
		metrics = append(metrics, m)
	}
	return metrics, rows.Err()
}

// getTopCreatives queries ClickHouse for the top performing creatives within a campaign
// ranked by CTR (click-through rate). Only includes creatives with non-null IDs and
// returns up to 'limit' results ordered by CTR descending.
func getTopCreatives(ctx context.Context, db *sql.DB, campaignID int, days int, limit int) ([]CreativeMetrics, error) {
	query := `
		SELECT
			assumeNotNull(creative_id) as creative_id,
			countIf(event_type = 'impression') as impressions,
			countIf(event_type = 'click') as clicks,
			round(if(impressions > 0, clicks / impressions * 100, 0), 2) as ctr,
			sum(cost) as spend
		FROM events
		WHERE campaign_id = ?
			AND creative_id IS NOT NULL
			AND timestamp >= now() - INTERVAL ? DAY
		GROUP BY creative_id
		ORDER BY ctr DESC
		LIMIT ?`

	rows, err := db.QueryContext(ctx, query, campaignID, days, limit)
	if err != nil {
		return nil, fmt.Errorf("query top creatives: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var creatives []CreativeMetrics
	for rows.Next() {
		var c CreativeMetrics
		err := rows.Scan(&c.CreativeID, &c.Impressions, &c.Clicks, &c.CTR, &c.Spend)
		if err != nil {
			return nil, fmt.Errorf("scan creative metrics: %w", err)
		}
		creatives = append(creatives, c)
	}
	return creatives, rows.Err()
}

// getLineItemMetrics queries ClickHouse for performance metrics of all line items within a campaign.
// Returns metrics grouped by line item ID with calculated CTR, CPM, and CPC for each line item.
func getLineItemMetrics(ctx context.Context, db *sql.DB, campaignID int, days int) ([]LineItemMetrics, error) {
	query := `
		SELECT
			assumeNotNull(line_item_id) as line_item_id,
			countIf(event_type = 'impression') as impressions,
			countIf(event_type = 'click') as clicks,
			sum(cost) as spend,
			round(if(impressions > 0, clicks / impressions * 100, 0), 2) as ctr,
			round(if(impressions > 0, spend / impressions * 1000, 0), 2) as cpm,
			round(if(clicks > 0, spend / clicks, 0), 2) as cpc
		FROM events
		WHERE campaign_id = ?
			AND line_item_id IS NOT NULL
			AND timestamp >= now() - INTERVAL ? DAY
		GROUP BY line_item_id
		ORDER BY impressions DESC`

	rows, err := db.QueryContext(ctx, query, campaignID, days)
	if err != nil {
		return nil, fmt.Errorf("query line item metrics: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var lineItems []LineItemMetrics
	for rows.Next() {
		var li LineItemMetrics
		err := rows.Scan(&li.LineItemID, &li.Impressions, &li.Clicks, &li.Spend, &li.CTR, &li.CPM, &li.CPC)
		if err != nil {
			return nil, fmt.Errorf("scan line item metrics: %w", err)
		}
		lineItems = append(lineItems, li)
	}
	return lineItems, rows.Err()
}
