package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/patrickwarner/openadserve/internal/macros"
	"github.com/patrickwarner/openadserve/internal/models"
)

func TestMacroExpansion_InClickContext(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := macros.NewServiceForTesting(logger)

	// Test creative with click URL containing macros
	creative := &models.Creative{
		ID:          123,
		LineItemID:  456,
		CampaignID:  789,
		PublisherID: 101,
		PlacementID: "test-placement",
		ClickURL:    "https://example.com/landing?id={CREATIVE_ID}&req={AUCTION_ID}&source={CUSTOM.utm_source}",
		LineItem: &models.LineItem{
			ID:       456,
			ClickURL: "",
		},
	}

	// Create mock HTTP request
	req := httptest.NewRequest("GET", "/click", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.RemoteAddr = "192.168.1.100:12345"

	// Custom parameters come from token (ad request ext field)
	customParams := map[string]string{
		"utm_source":   "google",
		"utm_campaign": "summer2024",
	}

	// Create click context
	clickCtx := macros.NewClickContextFromRequest(
		"test-request-123",
		"test-impression-456",
		creative,
		customParams,
	)

	// Test macro expansion
	expandedURL, err := service.GetDestinationURL(req.Context(), creative, clickCtx)
	if err != nil {
		t.Fatalf("Failed to expand URL: %v", err)
	}

	// Verify expanded URL contains expected values
	expected := map[string]string{
		"id=123":               "Creative ID",
		"req=test-request-123": "Request ID",
		"source=google":        "Custom parameter",
	}

	for expectedValue, description := range expected {
		if !containsParam(expandedURL, expectedValue) {
			t.Errorf("Expected %s (%s) in URL: %s", expectedValue, description, expandedURL)
		}
	}

	t.Logf("Successfully expanded URL: %s", expandedURL)
}

func TestMacroExpansion_FallbackToLineItem(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := macros.NewServiceForTesting(logger)

	// Test creative WITHOUT click URL (should fallback to line item)
	creative := &models.Creative{
		ID:          123,
		LineItemID:  456,
		CampaignID:  789,
		PublisherID: 101,
		PlacementID: "test-placement",
		ClickURL:    "", // Empty - should fallback
		LineItem: &models.LineItem{
			ID:       456,
			ClickURL: "https://lineitem.com/landing?li={LINE_ITEM_ID}&campaign={CAMPAIGN_ID}",
		},
	}

	// Create click context
	clickCtx := macros.NewClickContextFromRequest(
		"test-request-456",
		"test-impression-789",
		creative,
		nil,
	)

	// Test macro expansion
	expandedURL, err := service.GetDestinationURL(context.TODO(), creative, clickCtx)
	if err != nil {
		t.Fatalf("Failed to expand URL: %v", err)
	}

	// Verify it used line item URL and expanded macros
	expected := map[string]string{
		"lineitem.com": "Line item domain",
		"li=456":       "Line item ID",
		"campaign=789": "Campaign ID",
	}

	for expectedValue, description := range expected {
		if !containsParam(expandedURL, expectedValue) {
			t.Errorf("Expected %s (%s) in URL: %s", expectedValue, description, expandedURL)
		}
	}

	t.Logf("Successfully used line item fallback URL: %s", expandedURL)
}

func TestMacroExpansion_NoDestinationURL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := macros.NewServiceForTesting(logger)

	// Test creative with NO click URLs at all
	creative := &models.Creative{
		ID:          123,
		LineItemID:  456,
		CampaignID:  789,
		PublisherID: 101,
		PlacementID: "test-placement",
		ClickURL:    "", // Empty
		LineItem: &models.LineItem{
			ID:       456,
			ClickURL: "", // Also empty
		},
	}

	// Create click context
	clickCtx := macros.NewClickContextFromRequest(
		"test-request-789",
		"test-impression-101",
		creative,
		nil,
	)

	// Test macro expansion
	expandedURL, err := service.GetDestinationURL(context.TODO(), creative, clickCtx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should return empty string when no URLs are configured
	if expandedURL != "" {
		t.Errorf("Expected empty URL but got: %s", expandedURL)
	}

	t.Log("Successfully handled case with no destination URLs")
}

func TestCustomParameterFlow_FromAdRequest(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := macros.NewServiceForTesting(logger)

	// Test the complete flow: ad request ext -> token -> macro expansion
	creative := &models.Creative{
		ID:          123,
		LineItemID:  456,
		CampaignID:  789,
		PublisherID: 101,
		PlacementID: "test-placement",
		ClickURL:    "https://example.com/landing?source={CUSTOM.utm_source}&campaign={CUSTOM.utm_campaign}&segment={CUSTOM.user_segment}",
		LineItem: &models.LineItem{
			ID:       456,
			ClickURL: "",
		},
	}

	// Custom parameters as they would come from ad request ext field
	customParams := map[string]string{
		"utm_source":   "mobile_app",
		"utm_campaign": "holiday_sale",
		"user_segment": "premium",
	}

	// Create click context
	clickCtx := macros.NewClickContextFromRequest(
		"request-456",
		"impression-789",
		creative,
		customParams,
	)

	// Test macro expansion
	expandedURL, err := service.GetDestinationURL(context.TODO(), creative, clickCtx)
	if err != nil {
		t.Fatalf("Failed to expand URL: %v", err)
	}

	// Verify all custom parameters were expanded
	expected := map[string]string{
		"source=mobile_app":     "Custom utm_source parameter",
		"campaign=holiday_sale": "Custom utm_campaign parameter",
		"segment=premium":       "Custom user_segment parameter",
	}

	for expectedValue, description := range expected {
		if !containsParam(expandedURL, expectedValue) {
			t.Errorf("Expected %s (%s) in URL: %s", expectedValue, description, expandedURL)
		}
	}

	t.Logf("Successfully expanded custom parameters from ad request: %s", expandedURL)
}

// Helper function to check if URL contains a parameter or substring
func containsParam(url, param string) bool {
	// Simply check if the parameter appears anywhere in the URL
	for i := 0; i <= len(url)-len(param); i++ {
		if url[i:i+len(param)] == param {
			return true
		}
	}
	return false
}
