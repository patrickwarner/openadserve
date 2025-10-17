package token

import (
	"strings"
	"testing"
)

func TestValidateCustomParams(t *testing.T) {
	secret := []byte("test-secret-key")

	t.Run("Valid parameters pass validation", func(t *testing.T) {
		validParams := map[string]string{
			"utm_source":   "google",
			"user_segment": "premium",
			"category":     "tech",
		}

		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", validParams, secret)
		if err != nil {
			t.Errorf("Expected valid parameters to pass, got error: %v", err)
		}
	})

	t.Run("Too many parameters rejected", func(t *testing.T) {
		tooManyParams := make(map[string]string)
		for i := 0; i < MaxCustomParamsCount+1; i++ {
			tooManyParams[strings.Repeat("k", i+1)] = "value"
		}

		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", tooManyParams, secret)
		if err == nil {
			t.Error("Expected error for too many parameters, got nil")
		}
		if !strings.Contains(err.Error(), "too many custom parameters") {
			t.Errorf("Expected 'too many custom parameters' error, got: %v", err)
		}
	})

	t.Run("Key too long rejected", func(t *testing.T) {
		longKeyParams := map[string]string{
			strings.Repeat("k", MaxCustomParamKeyLength+1): "value",
		}

		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", longKeyParams, secret)
		if err == nil {
			t.Error("Expected error for key too long, got nil")
		}
		if !strings.Contains(err.Error(), "key too long") {
			t.Errorf("Expected 'key too long' error, got: %v", err)
		}
	})

	t.Run("Value too long rejected", func(t *testing.T) {
		longValueParams := map[string]string{
			"key": strings.Repeat("v", MaxCustomParamValueLength+1),
		}

		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", longValueParams, secret)
		if err == nil {
			t.Error("Expected error for value too long, got nil")
		}
		if !strings.Contains(err.Error(), "value too long") {
			t.Errorf("Expected 'value too long' error, got: %v", err)
		}
	})

	t.Run("Empty key rejected", func(t *testing.T) {
		emptyKeyParams := map[string]string{
			"": "value",
		}

		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", emptyKeyParams, secret)
		if err == nil {
			t.Error("Expected error for empty key, got nil")
		}
		if !strings.Contains(err.Error(), "key cannot be empty") {
			t.Errorf("Expected 'key cannot be empty' error, got: %v", err)
		}
	})

	t.Run("Maximum allowed parameters work", func(t *testing.T) {
		maxParams := make(map[string]string)
		for i := 0; i < MaxCustomParamsCount; i++ {
			// Create keys of max length that are still unique
			key := strings.Repeat("k", MaxCustomParamKeyLength-1) + string(rune('0'+i))
			value := strings.Repeat("v", MaxCustomParamValueLength)
			maxParams[key] = value
		}

		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", maxParams, secret)
		if err != nil {
			t.Errorf("Expected maximum allowed parameters to work, got error: %v", err)
		}
	})

	t.Run("Nil parameters allowed", func(t *testing.T) {
		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", nil, secret)
		if err != nil {
			t.Errorf("Expected nil parameters to be allowed, got error: %v", err)
		}
	})

	t.Run("Empty parameters allowed", func(t *testing.T) {
		emptyParams := make(map[string]string)
		_, err := GenerateWithAuctionData("req1", "imp1", "cr1", "c1", "li1", "u1", "p1", "pl1", 2.5, "USD", emptyParams, secret)
		if err != nil {
			t.Errorf("Expected empty parameters to be allowed, got error: %v", err)
		}
	})
}

func TestValidateCustomParamsDirectly(t *testing.T) {
	t.Run("validateCustomParams function tests", func(t *testing.T) {
		// Test the validation function directly
		validParams := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}

		err := validateCustomParams(validParams)
		if err != nil {
			t.Errorf("Expected valid params to pass, got: %v", err)
		}

		// Test boundary conditions
		boundaryParams := map[string]string{
			strings.Repeat("k", MaxCustomParamKeyLength): strings.Repeat("v", MaxCustomParamValueLength),
		}

		err = validateCustomParams(boundaryParams)
		if err != nil {
			t.Errorf("Expected boundary params to pass, got: %v", err)
		}
	})
}

func TestTokenSizeWithMaxParams(t *testing.T) {
	secret := []byte("test-secret-key-for-size-testing")

	// Create token with max allowed parameters to test actual size
	maxParams := make(map[string]string)
	for i := 0; i < MaxCustomParamsCount; i++ {
		// Use realistic parameter names and values
		key := []string{
			"utm_source", "utm_campaign", "user_segment", "content_cat",
			"placement_ctx", "audience_type", "page_type", "engagement",
			"loyalty_tier", "visit_freq",
		}[i]
		value := []string{
			"google_ads", "summer_sale_2024", "premium_subscriber", "technology",
			"above_fold_premium", "high_intent", "article_detail", "high",
			"platinum_member", "frequent_visitor",
		}[i]
		maxParams[key] = value
	}

	token, err := GenerateWithAuctionData(
		"test-request-123456",
		"test-impression-789",
		"creative-101",
		"campaign-202",
		"lineitem-303",
		"user-abc123def456",
		"publisher-404",
		"homepage-hero-banner",
		15.750000,
		"USD",
		maxParams,
		secret,
	)

	if err != nil {
		t.Fatalf("Failed to generate token with max params: %v", err)
	}

	t.Logf("Token with max parameters length: %d characters", len(token))
	t.Logf("Sample token: %s", token[:100]+"...") // Show first 100 chars

	// Verify token size is reasonable (under 1KB for safe URL usage)
	if len(token) > 1024 {
		t.Errorf("Token size too large: %d chars (over 1KB limit)", len(token))
	}

	// Verify the token can be verified successfully
	verified, err := Verify(token, secret, 0)
	if err != nil {
		t.Errorf("Failed to verify generated token: %v", err)
	}

	if len(verified.CustomParams) != MaxCustomParamsCount {
		t.Errorf("Expected %d custom params, got %d", MaxCustomParamsCount, len(verified.CustomParams))
	}
}
