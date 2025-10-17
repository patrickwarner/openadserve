package models

import "encoding/json"

// Creative represents an ad unit, which is the actual piece of content to be displayed.
// It is associated with a specific LineItem (for delivery rules) and a Campaign (for reporting).
// Creatives are configured by publishers and define the ad's appearance and assets.
type Creative struct {
	ID int `json:"id"` // Unique identifier for the creative.
	// PlacementID is the ID of the Placement this creative is primarily intended for,
	// though it might serve in others if sizes/formats match. Corresponds to Placement.ID.
	PlacementID string `json:"placement_id"`
	LineItemID  int    `json:"line_item_id"` // Identifier of the LineItem this creative belongs to. Corresponds to LineItem.ID.
	CampaignID  int    `json:"campaign_id"`  // Identifier of the Campaign this creative is part of. Corresponds to Campaign.ID.
	PublisherID int    `json:"publisher_id"`
	// HTML contains the ad markup if the creative format is "html".
	// Publishers provide this markup.
	HTML string `json:"html"`
	// Native holds the raw JSON data for native ad assets if the creative format is "native".
	// This allows publishers to define flexible, custom structures for their native ads,
	// which they can then render with their own client-side templating.
	Native json.RawMessage `json:"native,omitempty"`
	Width  int             `json:"width"`  // Width of the creative in pixels.
	Height int             `json:"height"` // Height of the creative in pixels.
	// Format specifies the type of the creative, e.g., "html" or "native".
	// This dictates how the ad content (HTML or Native) is interpreted and rendered.
	Format string `json:"format"`
	// ClickURL is the destination URL where users should be redirected when they click on the ad.
	// Supports macro expansion for dynamic values like {AUCTION_ID}, {CREATIVE_ID}, etc.
	ClickURL string `json:"click_url,omitempty"`

	// LineItem is a cached pointer to the associated LineItem to avoid repeated lookups.
	// This field is populated when creatives are loaded from the database and should not be serialized.
	LineItem *LineItem `json:"-"`
}

// AdResponse is a simplified structure used for responding to ad requests (`/ad` endpoint).
// It contains the essential details required by the client (e.g., SDK) to render the ad and track it.
type AdResponse struct {
	CreativeID int `json:"creative_id"` // The ID of the selected Creative (Creative.ID).
	// HTML contains the ad markup if the creative format is "html".
	HTML string `json:"html"`
	// Native holds the raw JSON data for native ad assets if the creative format is "native".
	// This allows publishers to receive structured data and render it using their own native templating.
	Native     json.RawMessage `json:"native,omitempty"`
	CampaignID int             `json:"campaign_id"`  // The ID of the campaign this ad belongs to (Campaign.ID).
	LineItemID int             `json:"line_item_id"` // The ID of the line item this ad belongs to (LineItem.ID).
	Price      float64         `json:"price"`        // The eCPM price of the ad.
}
