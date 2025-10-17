package models

import (
	"time"
)

// ForecastRequest represents a request to forecast availability for a potential line item
type ForecastRequest struct {
	// Line item configuration
	StartDate  time.Time `json:"start_date"`
	EndDate    time.Time `json:"end_date"`
	BudgetType string    `json:"budget_type"` // CPM, CPC, Flat
	Budget     float64   `json:"budget"`
	CPM        float64   `json:"cpm,omitempty"`
	CPC        float64   `json:"cpc,omitempty"`
	Priority   int       `json:"priority"`
	DailyCap   int       `json:"daily_cap,omitempty"`
	Pacing     string    `json:"pacing,omitempty"` // ASAP, Even

	// Targeting criteria
	PublisherID  int               `json:"publisher_id"`
	PlacementIDs []string          `json:"placement_ids,omitempty"`
	Countries    []string          `json:"countries,omitempty"`
	Regions      []string          `json:"regions,omitempty"`
	DeviceTypes  []string          `json:"device_types,omitempty"`
	OS           []string          `json:"os,omitempty"`
	Browsers     []string          `json:"browsers,omitempty"`
	KeyValues    map[string]string `json:"key_values,omitempty"`
}

// ForecastResponse contains the forecasted availability and conflicts
type ForecastResponse struct {
	// Summary metrics
	TotalOpportunities   int64   `json:"total_opportunities"`
	AvailableImpressions int64   `json:"available_impressions"`
	EstimatedImpressions int64   `json:"estimated_impressions"`
	EstimatedClicks      int64   `json:"estimated_clicks,omitempty"`
	EstimatedSpend       float64 `json:"estimated_spend"`
	FillRate             float64 `json:"fill_rate"`
	EstimatedCTR         float64 `json:"estimated_ctr,omitempty"`

	// Daily breakdown
	DailyForecast []DailyForecast `json:"daily_forecast"`

	// Competing line items
	Conflicts []ConflictingLineItem `json:"conflicts"`

	// Warnings or limitations
	Warnings []string `json:"warnings,omitempty"`
}

// DailyForecast represents the forecast for a single day
type DailyForecast struct {
	Date                 time.Time `json:"date"`
	Opportunities        int64     `json:"opportunities"`
	AvailableImpressions int64     `json:"available_impressions"`
	EstimatedImpressions int64     `json:"estimated_impressions"`
	EstimatedClicks      int64     `json:"estimated_clicks,omitempty"`
	EstimatedSpend       float64   `json:"estimated_spend"`
}

// ConflictingLineItem represents a line item competing for the same inventory
type ConflictingLineItem struct {
	LineItemID        int     `json:"line_item_id"`
	LineItemName      string  `json:"line_item_name"`
	CampaignID        int     `json:"campaign_id"`
	CampaignName      string  `json:"campaign_name"`
	Priority          int     `json:"priority"`
	OverlapPercentage float64 `json:"overlap_percentage"`
	EstimatedImpact   int64   `json:"estimated_impact_impressions"`
	ConflictType      string  `json:"conflict_type"` // "higher_priority", "same_priority", "lower_priority"
}

// TrafficPattern represents historical traffic patterns for a segment
type TrafficPattern struct {
	TimeWindow    string            `json:"time_window"` // "hour", "day", "week"
	PublisherID   int               `json:"publisher_id"`
	PlacementID   string            `json:"placement_id,omitempty"`
	Country       string            `json:"country,omitempty"`
	DeviceType    string            `json:"device_type,omitempty"`
	KeyValues     map[string]string `json:"key_values,omitempty"`
	Opportunities int64             `json:"opportunities"`
	Impressions   int64             `json:"impressions"`
	Clicks        int64             `json:"clicks"`
	FillRate      float64           `json:"fill_rate"`
	CTR           float64           `json:"ctr"`
}
