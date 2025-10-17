package ctr_optimization_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/patrickwarner/openadserve/internal/logic/selectors"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/patrickwarner/openadserve/internal/optimization"

	"go.uber.org/zap"
)

// TestCTROptimizationLogic tests the CTR optimization logic directly
func TestCTROptimizationLogic(t *testing.T) {
	// Create mock CTR prediction service
	mockCTRServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(map[string]string{"status": "healthy"}); err != nil {
				t.Errorf("Failed to encode health response: %v", err)
			}
			return
		}

		if r.URL.Path == "/predict" {
			var req struct {
				LineItemID int    `json:"line_item_id"`
				DeviceType string `json:"device_type"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Mock response: mobile gets higher boost for CPC line items
			boostMultiplier := 1.0
			if req.LineItemID == 2 && req.DeviceType == "mobile" {
				boostMultiplier = 1.5 // 50% boost for mobile CPC
			} else if req.LineItemID == 2 && req.DeviceType == "desktop" {
				boostMultiplier = 1.1 // 10% boost for desktop CPC
			}

			response := map[string]interface{}{
				"line_item_id":     req.LineItemID,
				"ctr_score":        0.025,
				"confidence":       0.8,
				"boost_multiplier": boostMultiplier,
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("Failed to encode prediction response: %v", err)
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockCTRServer.Close()

	// Set up test data
	setupCTRTestData(t)

	t.Run("CTR_Client_Basic_Functionality", func(t *testing.T) {
		// Test CTR client directly
		logger := zap.NewNop()
		ctrClient := optimization.NewCTRPredictionClient(mockCTRServer.URL, 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

		// Enable CTR optimization for this test
		if err := os.Setenv("CTR_OPTIMIZATION_ENABLED", "true"); err != nil {
			t.Fatalf("Failed to set environment variable: %v", err)
		}
		defer func() {
			if err := os.Unsetenv("CTR_OPTIMIZATION_ENABLED"); err != nil {
				t.Errorf("Failed to unset environment variable: %v", err)
			}
		}()

		// Test mobile prediction
		mobileReq := &optimization.PredictionRequest{
			LineItemID: 2,
			DeviceType: "mobile",
			Country:    "US",
			HourOfDay:  14,
			DayOfWeek:  2,
		}

		ctx := context.Background()
		mobileResp, err := ctrClient.GetPrediction(ctx, mobileReq)
		if err != nil {
			t.Fatalf("Mobile prediction failed: %v", err)
		}

		if mobileResp.BoostMultiplier != 1.5 {
			t.Errorf("Expected mobile boost 1.5, got %f", mobileResp.BoostMultiplier)
		}

		// Test desktop prediction
		desktopReq := &optimization.PredictionRequest{
			LineItemID: 2,
			DeviceType: "desktop",
			Country:    "US",
			HourOfDay:  14,
			DayOfWeek:  2,
		}

		desktopResp, err := ctrClient.GetPrediction(ctx, desktopReq)
		if err != nil {
			t.Fatalf("Desktop prediction failed: %v", err)
		}

		if desktopResp.BoostMultiplier != 1.1 {
			t.Errorf("Expected desktop boost 1.1, got %f", desktopResp.BoostMultiplier)
		}

		t.Logf("Mobile boost: %f, Desktop boost: %f", mobileResp.BoostMultiplier, desktopResp.BoostMultiplier)
	})

	t.Run("eCPM_Calculation_With_CTR_Optimization", func(t *testing.T) {
		// Test the eCPM calculation logic directly
		logger := zap.NewNop()
		ctrClient := optimization.NewCTRPredictionClient(mockCTRServer.URL, 200*time.Millisecond, 5*time.Minute, logger, observability.NewNoOpRegistry())

		if err := os.Setenv("CTR_PREDICTOR_URL", mockCTRServer.URL); err != nil {
			t.Fatalf("Failed to set CTR_PREDICTOR_URL: %v", err)
		}
		defer func() {
			if err := os.Unsetenv("CTR_PREDICTOR_URL"); err != nil {
				t.Errorf("Failed to unset CTR_PREDICTOR_URL: %v", err)
			}
		}()

		// Create selector with CTR optimization
		selector := selectors.NewRuleBasedSelector()
		selector.SetCTROptimizationEnabled(true)
		selector.SetLogger(logger)
		selector.SetCTRClient(ctrClient)

		// Test line items
		cpcLineItem := &models.LineItem{
			ID:         2,
			BudgetType: models.BudgetTypeCPC,
			ECPM:       3.0, // Base eCPM: $3.00
		}

		cpmLineItem := &models.LineItem{
			ID:         1,
			BudgetType: models.BudgetTypeCPM,
			ECPM:       5.0, // Base eCPM: $5.00
		}

		// Note: We can't easily test calculateOptimizedECPM directly as it's not exported
		// In a real scenario, you'd add a testing interface or make the method public for testing
		// For now, this test demonstrates the CTR client works correctly

		// Mobile context would boost CPC line item in real usage
		_ = models.TargetingContext{
			DeviceType: "mobile",
			Country:    "US",
		}

		t.Logf("CPC Line Item - Base eCPM: $%.2f", cpcLineItem.ECPM)
		t.Logf("CPM Line Item - Base eCPM: $%.2f", cpmLineItem.ECPM)
		t.Logf("Mobile context should boost CPC line item by 50%% to $%.2f", cpcLineItem.ECPM*1.5)
	})
}

func setupCTRTestData(t *testing.T) {
	// Set up test line items
	lineItems := []models.LineItem{
		{
			ID:          1,
			PublisherID: 1,
			CampaignID:  1,
			Name:        "CPM Campaign",
			Active:      true,
			Priority:    models.PriorityMedium,
			ECPM:        5.0,
			BudgetType:  models.BudgetTypeCPM,
			CPM:         5.0,
			StartDate:   time.Now().Add(-24 * time.Hour),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
		{
			ID:          2,
			PublisherID: 1,
			CampaignID:  2,
			Name:        "CPC Campaign",
			Active:      true,
			Priority:    models.PriorityMedium,
			ECPM:        3.0, // Lower base eCPM, should get boosted
			BudgetType:  models.BudgetTypeCPC,
			CPC:         1.5,
			StartDate:   time.Now().Add(-24 * time.Hour),
			EndDate:     time.Now().Add(24 * time.Hour),
		},
	}

	// Initialize AdDataStore for the integration test
	testStore := models.NewInMemoryAdDataStore()
	_ = testStore.SetLineItems(lineItems)

	// Set up test publisher
	publishers := []models.Publisher{
		{
			ID:   1,
			Name: "Test Publisher",
		},
	}
	_ = testStore.SetPublishers(publishers)
}
