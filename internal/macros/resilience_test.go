package macros

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/patrickwarner/openadserve/internal/models"
)

func TestMacroExpansion_ResilientToFailures(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := NewServiceForTesting(logger)

	// Register a macro that always fails
	err := service.RegisterCustomMacro("FAILING_MACRO", func(ctx *ExpansionContext) (string, error) {
		return "", fmt.Errorf("test macro failure")
	})
	if err != nil {
		t.Fatalf("Failed to register failing macro: %v", err)
	}

	// Test creative with a URL containing both good and bad macros
	creative := &models.Creative{
		ID:          123,
		LineItemID:  456,
		CampaignID:  789,
		PublisherID: 101,
		PlacementID: "test-placement",
		ClickURL:    "https://example.com/landing?good={CREATIVE_ID}&bad={FAILING_MACRO}&req={AUCTION_ID}",
		LineItem: &models.LineItem{
			ID:       456,
			ClickURL: "",
		},
	}

	// Create click context
	clickCtx := NewClickContextFromRequest(
		"test-request-123",
		"test-impression-456",
		creative,
		nil,
	)

	// Test macro expansion - should not fail even with failing macro
	expandedURL, err := service.GetDestinationURL(context.Background(), creative, clickCtx)
	if err != nil {
		t.Errorf("Expected macro expansion to succeed despite failing macro, got error: %v", err)
	}

	// Verify the URL is usable and good macros were expanded
	if expandedURL == "" {
		t.Error("Expected non-empty URL despite macro failure")
	}

	// Good macros should be expanded
	if !containsParam(expandedURL, "good=123") {
		t.Errorf("Expected good macro to be expanded in URL: %s", expandedURL)
	}

	if !containsParam(expandedURL, "req=test-request-123") {
		t.Errorf("Expected request ID macro to be expanded in URL: %s", expandedURL)
	}

	// Bad macro should remain unexpanded (this is acceptable behavior)
	// The key point is that the click still works
	t.Logf("Resilient URL expansion result: %s", expandedURL)
}

func TestService_GetDestinationURL_HandlesExpansionFailures(t *testing.T) {
	logger := zaptest.NewLogger(t)
	service := NewServiceForTesting(logger)

	// Test with completely invalid URL structure that might cause expansion errors
	creative := &models.Creative{
		ID:          123,
		LineItemID:  456,
		CampaignID:  789,
		PublisherID: 101,
		ClickURL:    "https://example.com/landing?invalid={NONEXISTENT_MACRO}",
		LineItem: &models.LineItem{
			ID:       456,
			ClickURL: "",
		},
	}

	clickCtx := NewClickContextFromRequest(
		"test-request",
		"test-impression",
		creative,
		nil,
	)

	// Should return the original URL even if macros can't be expanded
	expandedURL, err := service.GetDestinationURL(context.Background(), creative, clickCtx)
	if err != nil {
		t.Errorf("Expected service to handle expansion failure gracefully, got error: %v", err)
	}

	// Should get back a usable URL (either expanded or original)
	if expandedURL == "" {
		t.Error("Expected non-empty URL even with expansion failures")
	}

	t.Logf("URL with expansion failures handled: %s", expandedURL)
}

// Helper function from existing tests
func containsParam(url, param string) bool {
	for i := 0; i <= len(url)-len(param); i++ {
		if url[i:i+len(param)] == param {
			return true
		}
	}
	return false
}
