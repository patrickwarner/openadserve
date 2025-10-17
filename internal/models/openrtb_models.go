package models

// OpenRTBRequest is a simplified version of the IAB OpenRTB 2.5 Bid Request object.
// It includes the essential fields required by this ad server to process an ad request
// and make a targeting decision. Publishers or their SDKs construct this object.
type OpenRTBRequest struct {
	ID     string       `json:"id"`     // Unique ID of the ad request, provided by the client. Used for tracking and debugging.
	Imp    []Impression `json:"imp"`    // Array of impression objects, representing one or more ad opportunities. Usually one for this server.
	User   User         `json:"user"`   // User object containing information about the user.
	Device Device       `json:"device"` // Device object containing information about the user's device.
	// Ext holds extension fields. This is where publishers can include custom data.
	Ext RequestExt `json:"ext,omitempty"`
}

// Impression object represents an ad slot or impression opportunity.
type Impression struct {
	ID    string `json:"id"`    // Unique ID for this impression object within the request.
	TagID string `json:"tagid"` // Identifier for the specific ad placement on the page. Corresponds to models.Placement.ID.
	// W and H are optional width and height in pixels.
	// If provided, these can override the default dimensions of the Placement specified by TagID for this specific request.
	// This gives publishers flexibility to request different sizes for the same placement on a per-request basis.
	W int `json:"w,omitempty"`
	H int `json:"h,omitempty"`
}

// User object contains information about the user for whom the ad is being requested.
type User struct {
	ID string `json:"id"` // Unique identifier for the user, managed by the publisher or SDK (e.g., a cookie ID).
}

// Device object provides information about the user's device.
type Device struct {
	UA string `json:"ua"` // User-Agent string of the device's browser. Used for device, OS, and browser targeting.
	IP string `json:"ip"` // IPv4 address of the device. Used for geo-targeting.
}

// RequestExt provides a means to extend the OpenRTBRequest object with custom data.
// This is a key area for publisher customization.
type RequestExt struct {
	// KV (Key-Values) is a map where publishers can send arbitrary key-value pairs.
	// These pairs can represent contextual information (e.g., "category": "sports", "keywords": "running,shoes")
	// or user segment data (e.g., "interest": "gardening", "premium_subscriber": "true").
	// Line items can then be targeted against these custom key-values, enabling highly flexible
	// and publisher-specific targeting logic.
	KV          map[string]string `json:"kv,omitempty"`
	PublisherID int               `json:"publisher_id"`
	// CustomParams are key-value pairs that will be available for macro expansion in click URLs.
	// These parameters are passed through the click tracking token and can be used in destination URLs
	// using the {CUSTOM.key} macro syntax. For example, if CustomParams contains {"utm_source": "homepage"},
	// then {CUSTOM.utm_source} in a creative's click_url will be replaced with "homepage".
	CustomParams map[string]string `json:"custom_params,omitempty"`
}

// OpenRTBResponse is a simplified version of the IAB OpenRTB 2.5 Bid Response object.
// It is returned by the ad server when an ad is selected (or not).
type OpenRTBResponse struct {
	ID      string    `json:"id"`      // ID of the ad request to which this is a response. Should mirror OpenRTBRequest.ID.
	SeatBid []SeatBid `json:"seatbid"` // Array of seatbid objects. For this server, typically one seatbid with one bid.
	// Nbr (No-Bid Reason) code. Included if no ad is served.
	// See IAB OpenRTB specification for common codes (e.g., 0: Unknown Error, 1: Technical Error, 2: Invalid Request, etc.).
	// This helps publishers diagnose why no ad was returned.
	Nbr int `json:"nbr,omitempty"`
}

// SeatBid object typically represents a buyer or a seat in an auction.
// In this simplified server, it acts as a container for the bid(s).
type SeatBid struct {
	Bid []Bid `json:"bid"` // Array of bid objects. Typically contains a single bid from this server.
}

// Bid object contains the details of the ad that won the internal auction for the impression.
type Bid struct {
	ID    string `json:"id"`    // Unique ID for this bid. Can be the impression ID or a generated bid ID.
	ImpID string `json:"impid"` // ID of the impression object in the request to which this bid pertains. Mirrors Impression.ID.
	// CrID is the Creative ID from the ad server's database (models.Creative.ID).
	// This identifies the specific creative asset that was selected.
	CrID string `json:"crid"`
	// CID is the Campaign ID (models.Campaign.ID) of the campaign that won and whose creative is being served.
	// This aligns with the OpenRTB specification where `cid` identifies the winning campaign.
	// It is useful for tracking and reporting purposes.
	CID string `json:"cid"`
	// Adm contains the ad markup (for HTML ads) or a JSON string of assets (for native ads).
	// Publishers use this content to render the ad. The format (HTML/JSON) is determined by the creative's configuration.
	// This field directly provides the ad content, demonstrating publisher control over creative assets.
	Adm string `json:"adm"`
	// Price is the eCPM (effective cost per mille) of the bid, representing its value.
	Price float64 `json:"price"`
	// ImpURL is a pre-signed URL that, when called (typically by the client/SDK after rendering), records an impression for this ad.
	// It includes a token for validation and tracking.
	ImpURL string `json:"impurl,omitempty"`
	// ClickURL is a pre-signed URL that, when called (typically on user click), records a click for this ad.
	// It also includes a token.
	ClickURL string `json:"clkurl,omitempty"`
	// EventURL is a pre-signed base URL for tracking custom events related to this ad (e.g., "like", "video_complete").
	// The client/SDK appends an event type parameter (e.g., `&type=like`) to this URL. Includes a token.
	// This enables publishers to track a wide range of interactions beyond simple impressions and clicks.
	EventURL string `json:"evturl,omitempty"`
	// ReportURL is a pre-signed URL for submitting an ad report.
	ReportURL string `json:"repturl,omitempty"`
}
