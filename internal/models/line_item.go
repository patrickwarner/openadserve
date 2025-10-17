package models

import (
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Line item priority levels. These are used to determine the order of consideration during ad selection.
const (
	PriorityHigh   = "high"   // Highest priority, considered first.
	PriorityMedium = "medium" // Medium priority.
	PriorityLow    = "low"    // Lowest priority, considered last.
)

// Line item budget types. These define how a line item's budget is spent and how it bids.
const (
	BudgetTypeCPM  = "cpm"  // Cost Per Mille (Thousand Impressions): Line item bids and spends based on impressions.
	BudgetTypeCPC  = "cpc"  // Cost Per Click: Line item bids based on an eCPM derived from CPC and CTR, spends based on clicks.
	BudgetTypeFlat = "flat" // Flat Rate: Line item has a fixed total budget, often for sponsorships or fixed placements.
)

// Line item types indicate the source or nature of the line item.
const (
	// LineItemTypeDirect represents a directly sold or managed deal by the publisher.
	// Creatives are typically hosted and managed within this ad server.
	LineItemTypeDirect = "direct"
	// LineItemTypeProgrammatic represents a line item that sources its bid and creative from an external endpoint.
	// This is used for integrating with third-party bidders or programmatic demand sources.
	LineItemTypeProgrammatic = "programmatic"
)

// PriorityOrder defines the default evaluation order for priority levels, from highest to lowest.
// This order can be customized by publishers via the `PRIORITY_ORDER` environment variable,
// allowing them to define their own priority tiers or change the ranking without code modification.
// The first element in this slice has the highest rank.
var PriorityOrder = []string{PriorityHigh, PriorityMedium, PriorityLow}

// priorityRank maps priority level strings to their numerical rank for efficient sorting and comparison.
var priorityRank map[string]int

// LineItem is the core entity for ad delivery control. It defines the specific targeting rules,
// delivery parameters (pacing, frequency caps), budget, and bidding strategy for a set of creatives
// under a Campaign. Publishers use LineItems to implement their ad sales strategies and manage
// how different ad products are delivered to various audiences and contexts.
// The granularity of LineItem settings provides significant publisher control over monetization.
type LineItem struct {
	ID                 int       `json:"id"`          // Unique identifier for the line item.
	CampaignID         int       `json:"campaign_id"` // ID of the Campaign this line item belongs to (Campaign.ID).
	PublisherID        int       `json:"publisher_id"`
	Name               string    `json:"name"`                 // Descriptive name for the line item (e.g., "US Mobile Banner - Q3").
	StartDate          time.Time `json:"start_date"`           // The date and time when the line item becomes eligible to serve (flight start).
	EndDate            time.Time `json:"end_date"`             // The date and time when the line item stops serving (flight end).
	DailyImpressionCap int       `json:"daily_impression_cap"` // Maximum impressions allowed per day. 0 means unlimited.
	DailyClickCap      int       `json:"daily_click_cap"`      // Maximum clicks allowed per day. 0 means unlimited.
	// PaceType controls how impressions are delivered over time. Valid values: PacingASAP, PacingEven.
	// This allows publishers to choose between rapid delivery or spreading it out.
	PaceType string `json:"pace_type"`
	// Priority determines the preference in ad selection, governed by PriorityOrder.
	// Publishers can use this to ensure high-value line items are considered first.
	Priority string `json:"priority"`
	// FrequencyCap limits how many times a user sees ads from this line item.
	FrequencyCap int `json:"frequency_cap"`
	// FrequencyWindow defines the time duration for the FrequencyCap (e.g., 3 impressions per 24 hours).
	FrequencyWindow time.Duration `json:"frequency_window"`
	// Standard targeting parameters.
	Country    string `json:"country"`     // Target specific countries (ISO 3166-1 alpha-2 code).
	Region     string `json:"region"`      // Target specific regions within a country (e.g., state, province).
	DeviceType string `json:"device_type"` // Target device types (e.g., "mobile", "desktop", "tablet").
	OS         string `json:"os"`          // Target operating systems (e.g., "iOS", "Android").
	Browser    string `json:"browser"`     // Target specific browsers (e.g., "Chrome", "Safari").
	Active     bool   `json:"active"`      // Toggles whether the line item is currently active and eligible for serving.
	// KeyValues enables publisher-defined custom targeting. Ad requests can include arbitrary key-value
	// pairs in `OpenRTBRequest.Ext.KV`. If they match the rules defined here (e.g. {"category": "sports"}),
	// the line item becomes eligible. This provides a powerful mechanism for publishers to leverage
	// their own first-party data (like content categories or user segments) for precise targeting.
	KeyValues map[string]string `json:"key_values,omitempty"`
	CPM       float64           `json:"cpm"` // Cost Per Mille (Thousand Impressions) bid, if budget type is CPM.
	CPC       float64           `json:"cpc"` // Cost Per Click bid, if budget type is CPC.
	// ECPM (Effective Cost Per Mille) is the normalized price used for ranking line items from different
	// buying models (CPM, CPC) in an auction. For CPM line items, ECPM is typically `CPM`.
	// For CPC line items, ECPM is calculated as `CPC * EstimatedCTR * 1000`.
	// This ensures fair competition and yield optimization for the publisher.
	ECPM float64 `json:"ecpm"`
	// BudgetType defines how the line item bids and how its spend is measured.
	// Valid values: BudgetTypeCPM, BudgetTypeCPC, BudgetTypeFlat.
	// This gives publishers flexibility in how they structure deals.
	BudgetType string `json:"budget_type"`
	// BudgetAmount is the total monetary budget for this line item. Delivery stops if exhausted.
	BudgetAmount float64 `json:"budget_amount"`
	// Spend is the accumulated spend for this line item. Tracked in memory and periodically persisted.
	Spend float64 `json:"spend"`
	// Type differentiates direct deals from programmatic auctions (LineItemTypeDirect, LineItemTypeProgrammatic).
	// Defaults to LineItemTypeDirect when empty. This allows publishers to manage both their own inventory
	// and integrate external demand.
	Type string `json:"type,omitempty"`
	// Endpoint is an optional URL to fetch a bid for programmatic line items (if Type is LineItemTypeProgrammatic).
	// Publishers can integrate third-party bidders here.
	Endpoint string `json:"endpoint,omitempty"`
	// ClickURL is the default destination URL for ads in this line item.
	// Can be overridden at the creative level. Supports macro expansion for dynamic values.
	ClickURL string `json:"click_url,omitempty"`
}

// SetLineItems replaces all in-memory line items using the provided store.
func SetLineItems(store AdDataStore, items []LineItem) {
	if store == nil {
		return
	}
	if err := store.SetLineItems(items); err != nil {
		zap.L().Warn("failed to set line items", zap.Error(err))
	}
}

// SetLineItemsForPublisher replaces the line items for a specific publisher using the provided store.
func SetLineItemsForPublisher(store AdDataStore, pubID int, items []LineItem) {
	if store == nil {
		return
	}
	if err := store.SetLineItemsForPublisher(pubID, items); err != nil {
		zap.L().Warn("failed to set line items for publisher", zap.Error(err), zap.Int("publisher_id", pubID))
	}
}

// buildPriorityRank initializes the internal mapping from priority level strings (e.g., "high")
// to their numerical rank (e.g., 0 for the highest). This is used for efficient sorting
// and comparison during ad selection.
func buildPriorityRank() {
	newPriorityRank := make(map[string]int, len(PriorityOrder))
	for i, p := range PriorityOrder {
		newPriorityRank[p] = i
	}
	priorityRank = newPriorityRank // Atomic swap could be considered if read concurrently, but init context is usually safe.
}

// init function is automatically called when the package is loaded.
// It checks for a `PRIORITY_ORDER` environment variable. If set, it overrides the default
// PriorityOrder slice with comma-separated values from the environment variable.
// This allows publishers to customize line item priority levels and their evaluation order
// without modifying the source code, enhancing the system's configurability.
// After setting the PriorityOrder, it builds the priorityRank map.
func init() {
	if env := os.Getenv("PRIORITY_ORDER"); env != "" {
		customPriorityOrder := strings.Split(env, ",")
		trimmedPriorityOrder := make([]string, 0, len(customPriorityOrder))
		for _, p := range customPriorityOrder {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				trimmedPriorityOrder = append(trimmedPriorityOrder, trimmed)
			}
		}
		if len(trimmedPriorityOrder) > 0 {
			PriorityOrder = trimmedPriorityOrder
		}
	}
	buildPriorityRank()
}

// PriorityRank returns the numerical ranking for a given priority level string.
// Lower numbers indicate higher priority. Unrecognized priority strings are ranked
// after all known levels (i.e., given the lowest priority).
// This function allows the ad selection logic to easily compare line items by priority.
func PriorityRank(p string) int {
	// Access to priorityRank should be thread-safe if it can be modified post-init.
	// However, it's typically built once during init.
	if r, ok := priorityRank[p]; ok {
		return r
	}
	return len(PriorityOrder) // Rank unrecognized priorities last.
}

// GetLineItem returns a LineItem for a given publisher and ID.
// This function delegates to the AdDataStore for thread-safe access.
func GetLineItem(store AdDataStore, pubID, id int) *LineItem {
	if store == nil {
		return nil
	}
	return store.GetLineItem(pubID, id)
}

// GetLineItemByID searches for a line item across all publishers.
// This is kept for backward compatibility with existing tests.
func GetLineItemByID(store AdDataStore, id int) *LineItem {
	if store == nil {
		return nil
	}
	return store.GetLineItemByID(id)
}
