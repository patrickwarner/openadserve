package logic

import (
	"strings"
	"testing"

	"github.com/patrickwarner/openadserve/internal/models"
)

func TestResolveTargetingFromUA(t *testing.T) {
	tests := []struct {
		name            string
		ua              string
		expectedDevice  string
		expectedOS      string // Can use strings.Contains for version
		expectedBrowser string // Can use strings.Contains for version
		expectedIsBot   bool
	}{
		{
			name:            "Windows Chrome",
			ua:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.75 Safari/537.36",
			expectedDevice:  "desktop",
			expectedOS:      "Windows 10", // uasurfer might give "Windows 10" or just "Windows"
			expectedBrowser: "Chrome",     // uasurfer might give "Chrome 100"
			expectedIsBot:   false,
		},
		{
			name:            "Mac Safari",
			ua:              "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Safari/605.1.15",
			expectedDevice:  "desktop",
			expectedOS:      "OSX", // uasurfer outputs OSMacOSX
			expectedBrowser: "Safari",
			expectedIsBot:   false,
		},
		{
			name:            "iPhone Safari",
			ua:              "Mozilla/5.0 (iPhone; CPU iPhone OS 15_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Mobile/15E148 Safari/605.1.15",
			expectedDevice:  "mobile",
			expectedOS:      "iOS", // uasurfer might give "iOS 15.0"
			expectedBrowser: "Safari",
			expectedIsBot:   false,
		},
		{
			name:            "Android Chrome",
			ua:              "Mozilla/5.0 (Linux; Android 11; SM-G975F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/100.0.4896.58 Mobile Safari/537.36",
			expectedDevice:  "mobile",
			expectedOS:      "Android", // uasurfer might give "Android 11"
			expectedBrowser: "Chrome",
			expectedIsBot:   false,
		},
		{
			name:            "iPad Safari",
			ua:              "Mozilla/5.0 (iPad; CPU OS 15_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Mobile/15E148 Safari/605.1.15",
			expectedDevice:  "tablet",
			expectedOS:      "iOS", // uasurfer might give "iPadOS 15.0" or "iOS 15.0"
			expectedBrowser: "Safari",
			expectedIsBot:   false,
		},
		{
			name:            "Googlebot",
			ua:              "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			expectedDevice:  "desktop",   // uasurfer identifies Googlebot as DeviceDesktop
			expectedOS:      "Bot",       // uasurfer OSBot
			expectedBrowser: "GoogleBot", // uasurfer BrowserGoogleBot
			expectedIsBot:   true,
		},
		{
			name:            "Empty UA",
			ua:              "",
			expectedDevice:  "other", // Default from uasurfer
			expectedOS:      "Unknown",
			expectedBrowser: "Unknown",
			expectedIsBot:   false,
		},
		{
			name:            "Bogus UA",
			ua:              "completely-bogus-ua-string-12345",
			expectedDevice:  "other", // Default from uasurfer
			expectedOS:      "Unknown",
			expectedBrowser: "Unknown",
			expectedIsBot:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Note: ResolveTargetingFromUA doesn't use geoip, so nil is fine for g.
			// It also doesn't use ip, so empty string is fine.
			ctx := ResolveTargetingFromUA(tc.ua)

			if ctx.DeviceType != tc.expectedDevice {
				t.Errorf("DeviceType: expected '%s', got '%s'", tc.expectedDevice, ctx.DeviceType)
			}
			// For OS and Browser, uasurfer can be very specific with versions.
			// We'll check if the expected name is contained in the result.
			if !strings.Contains(ctx.OS, tc.expectedOS) {
				t.Errorf("OS: expected to contain '%s', got '%s'", tc.expectedOS, ctx.OS)
			}
			if !strings.Contains(ctx.Browser, tc.expectedBrowser) {
				t.Errorf("Browser: expected to contain '%s', got '%s'", tc.expectedBrowser, ctx.Browser)
			}
			if ctx.IsBot != tc.expectedIsBot {
				t.Errorf("IsBot: expected %t, got %t", tc.expectedIsBot, ctx.IsBot)
			}
		})
	}
}

func TestMatchesTargeting(t *testing.T) {

	tests := []struct {
		name              string
		lineItemTargeting models.LineItem
		userContext       models.TargetingContext
		expectedMatch     bool
	}{
		{
			name:              "No targeting rules on LineItem",
			lineItemTargeting: models.LineItem{ID: 1001, CampaignID: 1001, Active: true}, // No specific targeting rules
			userContext:       models.TargetingContext{DeviceType: "mobile", Country: "US", OS: "iOS", Browser: "Safari"},
			expectedMatch:     true,
		},
		{
			name:              "Exact match on Country",
			lineItemTargeting: models.LineItem{ID: 1002, CampaignID: 1002, Country: "US", Active: true},
			userContext:       models.TargetingContext{Country: "US"},
			expectedMatch:     true,
		},
		{
			name:              "Mismatch on Country",
			lineItemTargeting: models.LineItem{ID: 1003, CampaignID: 1003, Country: "US", Active: true},
			userContext:       models.TargetingContext{Country: "CA"},
			expectedMatch:     false,
		},
		{
			name:              "Exact match on Region",
			lineItemTargeting: models.LineItem{ID: 10031, CampaignID: 10031, Country: "US", Region: "CA", Active: true},
			userContext:       models.TargetingContext{Country: "US", Region: "CA"},
			expectedMatch:     true,
		},
		{
			name:              "Mismatch on Region",
			lineItemTargeting: models.LineItem{ID: 10032, CampaignID: 10032, Country: "US", Region: "CA", Active: true},
			userContext:       models.TargetingContext{Country: "US", Region: "NY"},
			expectedMatch:     false,
		},
		{
			name:              "Exact match on DeviceType",
			lineItemTargeting: models.LineItem{ID: 1004, CampaignID: 1004, DeviceType: "mobile", Active: true},
			userContext:       models.TargetingContext{DeviceType: "mobile"},
			expectedMatch:     true,
		},
		{
			name:              "Mismatch on DeviceType",
			lineItemTargeting: models.LineItem{ID: 1005, CampaignID: 1005, DeviceType: "mobile", Active: true},
			userContext:       models.TargetingContext{DeviceType: "desktop"},
			expectedMatch:     false,
		},
		{
			name:              "Exact match on OS",
			lineItemTargeting: models.LineItem{ID: 1006, CampaignID: 1006, OS: "iOS", Active: true},
			userContext:       models.TargetingContext{OS: "iOS 15.0"}, // Contains "iOS"
			expectedMatch:     true,
		},
		{
			name:              "Mismatch on OS",
			lineItemTargeting: models.LineItem{ID: 1007, CampaignID: 1007, OS: "iOS", Active: true},
			userContext:       models.TargetingContext{OS: "Android 11"},
			expectedMatch:     false,
		},
		{
			name:              "Exact match on Browser",
			lineItemTargeting: models.LineItem{ID: 1008, CampaignID: 1008, Browser: "Safari", Active: true},
			userContext:       models.TargetingContext{Browser: "Safari 15.0"}, // Contains "Safari"
			expectedMatch:     true,
		},
		{
			name:              "Mismatch on Browser",
			lineItemTargeting: models.LineItem{ID: 1009, CampaignID: 1009, Browser: "Safari", Active: true},
			userContext:       models.TargetingContext{Browser: "Chrome 100"},
			expectedMatch:     false,
		},
		{
			name:              "All rules match",
			lineItemTargeting: models.LineItem{ID: 1010, CampaignID: 1010, Country: "US", DeviceType: "mobile", OS: "iOS", Browser: "Safari", Active: true},
			userContext:       models.TargetingContext{Country: "US", DeviceType: "mobile", OS: "iOS 15.0", Browser: "Safari 15.0"},
			expectedMatch:     true,
		},
		{
			name:              "One rule mismatches out of several (Country)",
			lineItemTargeting: models.LineItem{ID: 1011, CampaignID: 1011, Country: "CA", DeviceType: "mobile", OS: "iOS", Browser: "Safari", Active: true},
			userContext:       models.TargetingContext{Country: "US", DeviceType: "mobile", OS: "iOS 15.0", Browser: "Safari 15.0"},
			expectedMatch:     false,
		},
		{
			name:              "LineItem has Country rule, context for Country is empty",
			lineItemTargeting: models.LineItem{ID: 1012, CampaignID: 1012, Country: "US", Active: true},
			userContext:       models.TargetingContext{DeviceType: "mobile"}, // Country is empty
			expectedMatch:     false,                                         // Expect non-match if rule is specific and context is not.
		},
		{
			name:              "LineItem Country rule is empty, context for Country is not",
			lineItemTargeting: models.LineItem{ID: 1013, CampaignID: 1013, DeviceType: "mobile", Active: true}, // Country rule is empty
			userContext:       models.TargetingContext{Country: "US", DeviceType: "mobile"},
			expectedMatch:     true, // Empty rule on LI means it matches any context for that field.
		},
		{
			name:              "LineItem OS rule is partial match (iOS vs iOS 15.0)",
			lineItemTargeting: models.LineItem{ID: 1014, CampaignID: 1014, OS: "iOS", Active: true},
			userContext:       models.TargetingContext{OS: "iOS 15.0"},
			expectedMatch:     true,
		},
		{
			name: "LineItem OS rule is more specific than context (iOS 15.0 vs iOS)",
			// This case highlights that current logic is strings.Contains(context, lineItemRule)
			// So, if LI says "iOS 15.0" and context says "iOS", it won't match.
			lineItemTargeting: models.LineItem{ID: 1015, CampaignID: 1015, OS: "iOS 15.0", Active: true},
			userContext:       models.TargetingContext{OS: "iOS"},
			expectedMatch:     false,
		},
		{
			name:              "LineItem Browser rule is partial match (Chrome vs Chrome Mobile)",
			lineItemTargeting: models.LineItem{ID: 1016, CampaignID: 1016, Browser: "Chrome", Active: true},
			userContext:       models.TargetingContext{Browser: "Chrome Mobile 100.0"},
			expectedMatch:     true,
		},
		{
			name:              "Case-insensitive Country match",
			lineItemTargeting: models.LineItem{ID: 1017, CampaignID: 1017, Country: "US", Active: true},
			userContext:       models.TargetingContext{Country: "us"},
			expectedMatch:     true,
		},
		{
			name:              "Case-insensitive DeviceType match",
			lineItemTargeting: models.LineItem{ID: 1018, CampaignID: 1018, DeviceType: "mobile", Active: true},
			userContext:       models.TargetingContext{DeviceType: "Mobile"},
			expectedMatch:     true,
		},
		{
			name:              "Case-insensitive OS match",
			lineItemTargeting: models.LineItem{ID: 1019, CampaignID: 1019, OS: "iOS", Active: true},
			userContext:       models.TargetingContext{OS: "ios 16"},
			expectedMatch:     true,
		},
		{
			name:              "Case-insensitive Browser match",
			lineItemTargeting: models.LineItem{ID: 1020, CampaignID: 1020, Browser: "Safari", Active: true},
			userContext:       models.TargetingContext{Browser: "safari 15"},
			expectedMatch:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create test data store for this specific test
			testDataStore := models.NewTestAdDataStore()
			_ = testDataStore.SetLineItems([]models.LineItem{tc.lineItemTargeting})

			// The creative's LineItemID must match the ID of the line item we just put in the store.
			creative := models.Creative{LineItemID: tc.lineItemTargeting.ID}

			// Log the retrieved line item for debugging all sub-tests
			retrievedLi := testDataStore.GetLineItemByID(creative.LineItemID)
			t.Logf("Test: %s, Retrieved LI: %+v, Expected LI in table: %+v", tc.name, retrievedLi, tc.lineItemTargeting)

			match := MatchesTargeting(creative, tc.userContext, testDataStore)
			if match != tc.expectedMatch {
				t.Errorf("expected match %t, got %t (LineItem: %+v, Context: %+v)", tc.expectedMatch, match, tc.lineItemTargeting, tc.userContext)
			}
		})
	}
}

func TestMatchesKeyValues(t *testing.T) {
	li := &models.LineItem{ID: 200, CampaignID: 200, KeyValues: map[string]string{"a": "1", "b": "2"}, Active: true}
	ctx := models.TargetingContext{KeyValues: map[string]string{"a": "1", "b": "2"}}
	if !MatchesKeyValues(li, ctx) {
		t.Error("expected key/value match")
	}

	ctx = models.TargetingContext{KeyValues: map[string]string{"a": "1"}}
	if MatchesKeyValues(li, ctx) {
		t.Error("expected mismatch when key missing")
	}

	ctx = models.TargetingContext{KeyValues: map[string]string{"a": "1", "b": "9"}}
	if MatchesKeyValues(li, ctx) {
		t.Error("expected mismatch when value differs")
	}

	if !MatchesKeyValues(&models.LineItem{ID: 201, Active: true}, models.TargetingContext{}) {
		t.Error("empty rules should match")
	}
}
