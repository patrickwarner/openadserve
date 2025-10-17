package models

// TargetingContext holds parsed and derived information from an ad request,
// used for evaluating line item targeting rules. It is populated based on the
// incoming ad request's User-Agent string, IP address (for geo-location),
// and any custom key-values provided by the publisher in `OpenRTBRequest.Ext.KV`.
// This structure centralizes all data points needed for targeting decisions,
// facilitating cleaner and more efficient filtering logic.
type TargetingContext struct {
	DeviceType string // Device type (e.g., "mobile", "desktop", "tablet", "tv"). Derived from User-Agent.
	OS         string // Operating system name and version (e.g., "iOS 15.1", "Android 12"). Derived from User-Agent.
	Browser    string // Browser name and version (e.g., "Chrome 98.0", "Safari 15.1"). Derived from User-Agent.
	IsBot      bool   // True if the User-Agent is identified as a known bot or crawler.
	Country    string // ISO 3166-1 alpha-2 country code derived from the request's IP address (e.g., "US", "CA").
	Region     string // Region or subdivision code derived from the IP address (e.g., "CA" for California in US, "ON" for Ontario in CA).
	// KeyValues contains the custom key-value pairs sent by the publisher in the ad request (`OpenRTBRequest.Ext.KV`).
	// This is a critical feature for publisher customization, allowing them to pass their own contextual signals
	// (e.g., content categories like "sports", user attributes like "premium_subscriber") for targeting.
	// Line items can then be configured to target these specific key-values.
	KeyValues map[string]string
}
