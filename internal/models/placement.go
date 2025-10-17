package models

// Placement represents an ad slot on a publisher's website or application.
// It defines the default dimensions (width, height) and allowed creative formats (e.g., "html", "native")
// for that specific slot. Publishers configure placements to map areas of their inventory to
// ad server configurations. This gives them control over what types of ads can appear in designated locations.
type Placement struct {
	// ID is a publisher-defined unique identifier for the placement (e.g., "homepage-banner-300x250", "article-sidebar-native").
	// This ID is used in ad requests (as `tagid`) to specify which ad slot is being filled.
	ID          string `json:"id"`
	PublisherID int    `json:"publisher_id"`
	// Width is the default width of the ad slot in pixels.
	// While creatives must generally match this, ad requests can optionally override dimensions.
	Width int `json:"width"`
	// Height is the default height of the ad slot in pixels.
	// Similar to Width, this can be overridden by specific ad requests.
	Height int `json:"height"`
	// Formats is a list of creative formats allowed to serve in this placement (e.g., ["html", "native"]).
	// Creatives selected for this placement must have a format that is in this list.
	// This allows publishers to enforce, for example, that only native ads appear in a native-only slot.
	Formats []string `json:"formats"`
}
