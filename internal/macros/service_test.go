package macros

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/patrickwarner/openadserve/internal/models"
)

func TestService_GetDestinationURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := NewServiceForTesting(logger)

	tests := []struct {
		name        string
		creative    *models.Creative
		lineItem    *models.LineItem
		expectedURL string
		expectEmpty bool
		expectError bool
	}{
		{
			name: "Creative-level click URL with macros",
			creative: &models.Creative{
				ID:       123,
				ClickURL: "https://example.com/landing?cr={CREATIVE_ID}&req={AUCTION_ID}",
				LineItem: &models.LineItem{
					ID:       456,
					ClickURL: "https://fallback.com/landing",
				},
			},
			expectedURL: "https://example.com/landing?cr=123&req=test-request",
		},
		{
			name: "Line item-level click URL fallback",
			creative: &models.Creative{
				ID:       123,
				ClickURL: "", // Empty, should fallback to line item
				LineItem: &models.LineItem{
					ID:       456,
					ClickURL: "https://lineitem.com/landing?li={LINE_ITEM_ID}",
				},
			},
			expectedURL: "https://lineitem.com/landing?li=456",
		},
		{
			name: "No click URL configured",
			creative: &models.Creative{
				ID:       123,
				ClickURL: "",
				LineItem: &models.LineItem{
					ID:       456,
					ClickURL: "",
				},
			},
			expectEmpty: true,
		},
		{
			name: "Creative without line item",
			creative: &models.Creative{
				ID:       123,
				ClickURL: "https://example.com/landing",
				LineItem: nil,
			},
			expectedURL: "https://example.com/landing",
		},
		{
			name: "Creative with custom parameters",
			creative: &models.Creative{
				ID:       123,
				ClickURL: "https://example.com/landing?utm_source={CUSTOM.source}&utm_campaign={CUSTOM.campaign}",
				LineItem: &models.LineItem{ID: 456},
			},
			expectedURL: "https://example.com/landing?utm_source=google&utm_campaign=summer2024",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clickCtx := &ClickContext{
				RequestID:    "test-request",
				ImpressionID: "test-impression",
				Timestamp:    time.Now(),
				CreativeID:   int32(tt.creative.ID),
				LineItemID:   456,
				CampaignID:   789,
				PublisherID:  101,
				CustomParams: map[string]string{
					"source":   "google",
					"campaign": "summer2024",
				},
			}

			expandedURL, err := service.GetDestinationURL(context.Background(), tt.creative, clickCtx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.expectEmpty && expandedURL != "" {
				t.Errorf("Expected empty URL but got: %s", expandedURL)
			}
			if !tt.expectEmpty && !tt.expectError && expandedURL != tt.expectedURL {
				t.Errorf("Expected URL %s, got %s", tt.expectedURL, expandedURL)
			}
		})
	}
}

func TestService_ExpandClickURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := NewServiceForTesting(logger)

	clickCtx := &ClickContext{
		RequestID:    "req-123",
		ImpressionID: "imp-456",
		Timestamp:    time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
		CreativeID:   789,
		LineItemID:   101,
		CampaignID:   202,
		PublisherID:  303,
		CustomParams: map[string]string{
			"utm_source": "facebook",
			"utm_medium": "cpc",
		},
	}

	tests := []struct {
		name        string
		rawURL      string
		expectedURL string
		expectError bool
	}{
		{
			name:        "Standard macros",
			rawURL:      "https://example.com?req={AUCTION_ID}&cr={CREATIVE_ID}&li={LINE_ITEM_ID}",
			expectedURL: "https://example.com?req=req-123&cr=789&li=101",
		},
		{
			name:        "Custom parameters",
			rawURL:      "https://example.com?source={CUSTOM.utm_source}&medium={CUSTOM.utm_medium}",
			expectedURL: "https://example.com?source=facebook&medium=cpc",
		},
		{
			name:        "Mixed macros and custom parameters",
			rawURL:      "https://example.com?id={CREATIVE_ID}&source={CUSTOM.utm_source}&ts={TIMESTAMP}",
			expectedURL: "", // Will check components separately due to timestamp variation
		},
		{
			name:        "Empty URL",
			rawURL:      "",
			expectedURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandedURL, err := service.ExpandClickURL(tt.rawURL, clickCtx)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Special handling for mixed macros test with timestamp
			if tt.name == "Mixed macros and custom parameters" {
				if !strings.Contains(expandedURL, "id=789") ||
					!strings.Contains(expandedURL, "source=facebook") ||
					!strings.Contains(expandedURL, "ts=") {
					t.Errorf("Mixed macros not properly expanded: %s", expandedURL)
				}
			} else if tt.expectedURL != "" && expandedURL != tt.expectedURL {
				t.Errorf("Expected URL %s, got %s", tt.expectedURL, expandedURL)
			}
		})
	}
}

func TestService_RegisterCustomMacro(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := NewServiceForTesting(logger)

	// Register a custom macro
	err := service.RegisterCustomMacro("TEST_MACRO", func(ctx *ExpansionContext) (string, error) {
		return "test_value", nil
	})
	if err != nil {
		t.Errorf("Failed to register custom macro: %v", err)
	}

	// Test that the custom macro works
	clickCtx := &ClickContext{
		RequestID: "test-request",
	}

	expandedURL, err := service.ExpandClickURL("https://example.com?test={TEST_MACRO}", clickCtx)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	expectedURL := "https://example.com?test=test_value"
	if expandedURL != expectedURL {
		t.Errorf("Expected URL %s, got %s", expectedURL, expandedURL)
	}
}

func TestService_ValidateURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := NewServiceForTesting(logger)

	tests := []struct {
		name                string
		rawURL              string
		expectedUnsupported []string
	}{
		{
			name:                "Valid URL with supported macros",
			rawURL:              "https://example.com?id={CREATIVE_ID}&req={AUCTION_ID}",
			expectedUnsupported: nil,
		},
		{
			name:                "URL with unsupported macro",
			rawURL:              "https://example.com?invalid={UNSUPPORTED_MACRO}",
			expectedUnsupported: []string{"UNSUPPORTED_MACRO"},
		},
		{
			name:                "Custom parameters are valid",
			rawURL:              "https://example.com?utm_source={CUSTOM.source}",
			expectedUnsupported: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsupported := service.ValidateURL(tt.rawURL)

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

func TestNewClickContextFromRequest(t *testing.T) {
	creative := &models.Creative{
		ID:          123,
		LineItemID:  456,
		CampaignID:  789,
		PublisherID: 101,
		PlacementID: "test-placement",
	}

	customParams := map[string]string{
		"utm_source":   "google",
		"utm_campaign": "test",
	}

	ctx := NewClickContextFromRequest(
		"req-123",
		"imp-456",
		creative,
		customParams,
	)

	if ctx.RequestID != "req-123" {
		t.Errorf("Expected RequestID %s, got %s", "req-123", ctx.RequestID)
	}
	if ctx.ImpressionID != "imp-456" {
		t.Errorf("Expected ImpressionID %s, got %s", "imp-456", ctx.ImpressionID)
	}
	if ctx.CreativeID != 123 {
		t.Errorf("Expected CreativeID %d, got %d", 123, ctx.CreativeID)
	}
	if ctx.LineItemID != 456 {
		t.Errorf("Expected LineItemID %d, got %d", 456, ctx.LineItemID)
	}
	if ctx.CampaignID != 789 {
		t.Errorf("Expected CampaignID %d, got %d", 789, ctx.CampaignID)
	}
	if ctx.PublisherID != 101 {
		t.Errorf("Expected PublisherID %d, got %d", 101, ctx.PublisherID)
	}
	if ctx.CustomParams["utm_source"] != "google" {
		t.Errorf("Expected utm_source %s, got %s", "google", ctx.CustomParams["utm_source"])
	}
	if ctx.CustomParams["utm_campaign"] != "test" {
		t.Errorf("Expected utm_campaign %s, got %s", "test", ctx.CustomParams["utm_campaign"])
	}

	// Check that timestamp is recent (within last minute)
	if time.Since(ctx.Timestamp) > time.Minute {
		t.Error("Timestamp should be recent")
	}
}
