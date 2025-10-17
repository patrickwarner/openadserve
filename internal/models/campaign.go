package models

import "go.uber.org/zap"

// Pacing types define how a line item's budget or impression goals are delivered over time.
// These are used by LineItems.
const (
	// PacingASAP delivers impressions as quickly as possible, without attempting to spread them out.
	// This is suitable for campaigns that need to achieve their goals rapidly.
	PacingASAP = "asap"
	// PacingEven attempts to distribute impressions evenly throughout the day, or over the line item's flight duration.
	// This helps in maintaining a consistent presence and preventing budget exhaustion too early.
	PacingEven = "even"
	// PacingPID uses a PID controller to dynamically adjust delivery rate
	// toward the target impressions-per-time goal. This reacts to traffic
	// fluctuations more smoothly than simple ASAP or even pacing.
	PacingPID = "pid"
)

// Campaign represents an advertising campaign. In this system, delivery rules,
// targeting, and budgets are primarily managed at the LineItem level.
// The Campaign serves as a lightweight container, mainly for grouping related
// LineItems and for high-level reporting and organization.
// Publishers use campaigns to structure their advertising efforts.
type Campaign struct {
	ID          int    `json:"id"`           // Unique identifier for the campaign.
	PublisherID int    `json:"publisher_id"` // Owning publisher for the campaign.
	Name        string `json:"name"`         // A human-readable name for the campaign (e.g., "Q4 Holiday Promotion").
}

// SetCampaigns replaces the in-memory campaign slice.
// This function delegates to the AdDataStore for thread-safe access.
func SetCampaigns(store AdDataStore, c []Campaign) {
	if store == nil {
		return
	}
	if err := store.SetCampaigns(c); err != nil {
		zap.L().Warn("failed to set campaigns", zap.Error(err))
	}
}

// GetCampaignByID returns the campaign matching the given ID, or nil if not found.
// This function delegates to the AdDataStore for thread-safe access.
func GetCampaignByID(store AdDataStore, id int) *Campaign {
	if store == nil {
		return nil
	}
	return store.GetCampaign(id)
}
