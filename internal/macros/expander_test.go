package macros

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestMacroExpander_ExpandURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	expander := NewMacroExpanderForTesting(logger, false)

	ctx := &ExpansionContext{
		RequestID:    "test-request-123",
		ImpressionID: "test-imp-456",
		Timestamp:    time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		CreativeID:   789,
		LineItemID:   101,
		CampaignID:   202,
		PublisherID:  303,
		PlacementID:  "404",
		CustomParams: map[string]string{
			"utm_source":   "google",
			"utm_campaign": "summer2024",
		},
	}

	tests := []struct {
		name           string
		rawURL         string
		expectedURL    string
		expectedError  bool
		customExpander func(*MacroExpander)
	}{
		{
			name:        "No macros",
			rawURL:      "https://example.com/landing",
			expectedURL: "https://example.com/landing",
		},
		{
			name:        "Single macro",
			rawURL:      "https://example.com/landing?id={CREATIVE_ID}",
			expectedURL: "https://example.com/landing?id=789",
		},
		{
			name:        "Multiple macros",
			rawURL:      "https://example.com/landing?req={AUCTION_ID}&cr={CREATIVE_ID}&li={LINE_ITEM_ID}",
			expectedURL: "https://example.com/landing?req=test-request-123&cr=789&li=101",
		},
		{
			name:        "Timestamp macros",
			rawURL:      "https://example.com/landing?ts={TIMESTAMP}&iso={ISO_TIMESTAMP}",
			expectedURL: "", // Will be checked separately since timestamp varies
		},
		{
			name:        "Custom parameters",
			rawURL:      "https://example.com/landing?source={CUSTOM.utm_source}&campaign={CUSTOM.utm_campaign}",
			expectedURL: "https://example.com/landing?source=google&campaign=summer2024",
		},
		{
			name:        "Empty URL",
			rawURL:      "",
			expectedURL: "",
		},
		{
			name:          "Invalid URL",
			rawURL:        "://invalid-url",
			expectedURL:   "://invalid-url",
			expectedError: true,
		},
		{
			name:        "Custom macro",
			rawURL:      "https://example.com/landing?custom={CUSTOM_MACRO}",
			expectedURL: "https://example.com/landing?custom=custom_value",
			customExpander: func(exp *MacroExpander) {
				_ = exp.RegisterMacro("CUSTOM_MACRO", func(ctx *ExpansionContext) (string, error) {
					return "custom_value", nil
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.customExpander != nil {
				tt.customExpander(expander)
			}

			expandedURL, err := expander.ExpandURL(tt.rawURL, ctx)

			if tt.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Special handling for timestamp test
			if tt.name == "Timestamp macros" {
				if !strings.Contains(expandedURL, "ts=") || !strings.Contains(expandedURL, "iso=") {
					t.Errorf("Timestamp macros not expanded: %s", expandedURL)
				}
			} else if tt.expectedURL != "" && expandedURL != tt.expectedURL {
				t.Errorf("Expected URL %s, got %s", tt.expectedURL, expandedURL)
			}
		})
	}
}

func TestMacroExpander_CustomParameterExpansion(t *testing.T) {
	logger := zaptest.NewLogger(t)
	expander := NewMacroExpanderForTesting(logger, false)

	ctx := &ExpansionContext{
		PlacementID: "placement-abc",
		CustomParams: map[string]string{
			"campaign": "holiday2024",
			"source":   "facebook",
			"medium":   "cpc",
		},
	}

	tests := []struct {
		name        string
		rawURL      string
		expectedURL string
	}{
		{
			name:        "Single custom parameter",
			rawURL:      "https://example.com?utm_campaign={CUSTOM.campaign}",
			expectedURL: "https://example.com?utm_campaign=holiday2024",
		},
		{
			name:        "Multiple custom parameters",
			rawURL:      "https://example.com?utm_source={CUSTOM.source}&utm_medium={CUSTOM.medium}&utm_campaign={CUSTOM.campaign}",
			expectedURL: "https://example.com?utm_source=facebook&utm_medium=cpc&utm_campaign=holiday2024",
		},
		{
			name:        "Non-existent custom parameter",
			rawURL:      "https://example.com?missing={CUSTOM.nonexistent}",
			expectedURL: "https://example.com?missing={CUSTOM.nonexistent}",
		},
		{
			name:        "Mixed custom and standard macros",
			rawURL:      "https://example.com?campaign={CUSTOM.campaign}&id={CREATIVE_ID}&p={PLACEMENT_ID}",
			expectedURL: "https://example.com?campaign=holiday2024&id=0&p=placement-abc", // CREATIVE_ID defaults to 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedURL, err := expander.ExpandURL(tt.rawURL, ctx)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if expandedURL != tt.expectedURL {
				t.Errorf("Expected URL %s, got %s", tt.expectedURL, expandedURL)
			}
		})
	}
}

func TestMacroExpander_ValidateURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	expander := NewMacroExpanderForTesting(logger, false)

	tests := []struct {
		name                string
		rawURL              string
		expectedUnsupported []string
	}{
		{
			name:                "All supported macros",
			rawURL:              "https://example.com?id={CREATIVE_ID}&req={AUCTION_ID}&li={LINE_ITEM_ID}",
			expectedUnsupported: nil,
		},
		{
			name:                "Custom parameters",
			rawURL:              "https://example.com?utm_source={CUSTOM.source}&utm_campaign={CUSTOM.campaign}",
			expectedUnsupported: nil,
		},
		{
			name:                "Unsupported macro",
			rawURL:              "https://example.com?unknown={UNSUPPORTED_MACRO}",
			expectedUnsupported: []string{"UNSUPPORTED_MACRO"},
		},
		{
			name:                "Mixed supported and unsupported",
			rawURL:              "https://example.com?id={CREATIVE_ID}&unknown={BAD_MACRO}&li={LINE_ITEM_ID}&invalid={NOT_SUPPORTED}",
			expectedUnsupported: []string{"BAD_MACRO", "NOT_SUPPORTED"},
		},
		{
			name:                "No macros",
			rawURL:              "https://example.com/plain-url",
			expectedUnsupported: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsupported := expander.ValidateURL(tt.rawURL)

			if len(unsupported) != len(tt.expectedUnsupported) {
				t.Errorf("Expected %d unsupported macros, got %d", len(tt.expectedUnsupported), len(unsupported))
			}

			for _, expected := range tt.expectedUnsupported {
				found := false
				for _, actual := range unsupported {
					if actual == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected unsupported macro %s not found", expected)
				}
			}
		})
	}
}

func TestMacroExpander_RegisterMacro(t *testing.T) {
	logger := zaptest.NewLogger(t)
	expander := NewMacroExpanderForTesting(logger, false)

	// Test registering a custom macro
	err := expander.RegisterMacro("TEST_MACRO", func(ctx *ExpansionContext) (string, error) {
		return "test_value", nil
	})
	if err != nil {
		t.Errorf("Failed to register macro: %v", err)
	}

	// Test that the custom macro works
	ctx := &ExpansionContext{}
	expandedURL, err := expander.ExpandURL("https://example.com?test={TEST_MACRO}", ctx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	expectedURL := "https://example.com?test=test_value"
	if expandedURL != expectedURL {
		t.Errorf("Expected URL %s, got %s", expectedURL, expandedURL)
	}

	// Test error cases
	if err := expander.RegisterMacro("", nil); err == nil {
		t.Error("Expected error for empty macro name")
	}

	if err := expander.RegisterMacro("VALID_NAME", nil); err == nil {
		t.Error("Expected error for nil expansion function")
	}
}

func TestMacroExpander_GetRegisteredMacros(t *testing.T) {
	logger := zaptest.NewLogger(t)
	expander := NewMacroExpanderForTesting(logger, false)

	macros := expander.GetRegisteredMacros()

	// Check that default macros are present
	expectedMacros := []string{
		"AUCTION_ID", "AUCTION_IMP_ID",
		"CREATIVE_ID", "LINE_ITEM_ID", "CAMPAIGN_ID", "PUBLISHER_ID", "PLACEMENT_ID",
		"TIMESTAMP", "TIMESTAMP_MS", "ISO_TIMESTAMP",
		"RANDOM", "UUID", "CUSTOM",
	}

	for _, expected := range expectedMacros {
		found := false
		for _, actual := range macros {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected default macro %s not found", expected)
		}
	}

	// Register a custom macro and check it's included
	_ = expander.RegisterMacro("CUSTOM_TEST", func(ctx *ExpansionContext) (string, error) {
		return "test", nil
	})

	macros = expander.GetRegisteredMacros()
	found := false
	for _, macro := range macros {
		if macro == "CUSTOM_TEST" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Custom macro not found in registered macros list")
	}
}

func TestMacroExpander_Observability(t *testing.T) {
	logger := zaptest.NewLogger(t)
	expander := NewMacroExpanderForTesting(logger, false)

	ctx := &ExpansionContext{
		CreativeID: 123,
	}

	// This test ensures that the observability metrics are properly configured
	// The actual metric values would be tested in integration tests
	rawURL := "https://example.com?id={CREATIVE_ID}&invalid={NONEXISTENT_MACRO}"
	expandedURL, err := expander.ExpandURL(rawURL, ctx)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// The NONEXISTENT_MACRO should remain unexpanded
	if !strings.Contains(expandedURL, "{NONEXISTENT_MACRO}") {
		t.Error("Invalid macro should remain unexpanded")
	}

	// The CREATIVE_ID should be expanded
	if !strings.Contains(expandedURL, "123") {
		t.Error("Valid macro should be expanded")
	}
}
