package models

import "time"

// AdReport represents a user-submitted report about an ad.
type AdReport struct {
	ID           int       `json:"id"`
	CreativeID   int       `json:"creative_id"`
	LineItemID   int       `json:"line_item_id"`
	CampaignID   int       `json:"campaign_id"`
	PublisherID  int       `json:"publisher_id"`
	UserID       string    `json:"user_id"`
	PlacementID  string    `json:"placement_id"`
	ReportReason string    `json:"report_reason"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	CreatedAt    time.Time `json:"created_at"`
	Status       string    `json:"status"`
}

// ReportReason describes a predefined reason for reporting an ad.
type ReportReason struct {
	Code               string `json:"code"`
	DisplayName        string `json:"display_name"`
	Description        string `json:"description"`
	Severity           string `json:"severity"`
	AutoBlockThreshold *int   `json:"auto_block_threshold,omitempty"`
}
