package forecasting

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// Engine provides forecasting capabilities for line items
type Engine struct {
	ClickHouse *sql.DB
	Redis      *redis.Client
	AdStore    models.AdDataStore
	Logger     *zap.Logger
}

// NewEngine creates a new forecasting engine
func NewEngine(clickhouse *sql.DB, redis *redis.Client, adStore models.AdDataStore, logger *zap.Logger) *Engine {
	return &Engine{
		ClickHouse: clickhouse,
		Redis:      redis,
		AdStore:    adStore,
		Logger:     logger,
	}
}

// Forecast generates a forecast for a potential line item configuration
func (e *Engine) Forecast(ctx context.Context, req *models.ForecastRequest) (*models.ForecastResponse, error) {
	// Validate request
	if err := validateForecastRequest(req); err != nil {
		return nil, fmt.Errorf("invalid forecast request: %w", err)
	}

	// Analyze historical traffic patterns
	patterns, err := e.analyzeTrafficPatterns(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze historical traffic patterns for publisher %d: %w", req.PublisherID, err)
	}

	// Calculate available inventory
	inventory, err := e.calculateAvailableInventory(ctx, req, patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate available inventory for period %s to %s: %w", req.StartDate.Format("2006-01-02"), req.EndDate.Format("2006-01-02"), err)
	}

	// Detect conflicts with existing line items
	conflicts, err := e.detectConflicts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to detect conflicts with existing line items for publisher %d: %w", req.PublisherID, err)
	}

	// Apply conflict resolution and calculate final forecast
	response := e.buildForecastResponse(req, patterns, inventory, conflicts)

	return response, nil
}

// validateForecastRequest ensures the request has valid parameters
func validateForecastRequest(req *models.ForecastRequest) error {
	if req.StartDate.IsZero() || req.EndDate.IsZero() {
		return fmt.Errorf("start_date and end_date are required")
	}
	if req.EndDate.Before(req.StartDate) {
		return fmt.Errorf("end_date must be after start_date")
	}
	if req.PublisherID <= 0 {
		return fmt.Errorf("publisher_id is required")
	}
	if req.BudgetType == "" {
		return fmt.Errorf("budget_type is required")
	}
	if req.Budget <= 0 {
		return fmt.Errorf("budget must be positive")
	}

	switch req.BudgetType {
	case models.BudgetTypeCPM:
		if req.CPM <= 0 {
			return fmt.Errorf("cpm must be positive for CPM campaigns")
		}
	case models.BudgetTypeCPC:
		if req.CPC <= 0 {
			return fmt.Errorf("cpc must be positive for CPC campaigns")
		}
	case models.BudgetTypeFlat:
		// Flat budget is valid
	default:
		return fmt.Errorf("invalid budget_type: %s", req.BudgetType)
	}

	return nil
}

// buildForecastResponse assembles the final forecast response
func (e *Engine) buildForecastResponse(req *models.ForecastRequest, patterns []*TrafficPattern, inventory *InventoryAvailability, conflicts []models.ConflictingLineItem) *models.ForecastResponse {
	response := &models.ForecastResponse{
		TotalOpportunities:   inventory.TotalOpportunities,
		AvailableImpressions: inventory.AvailableImpressions,
		EstimatedImpressions: inventory.EstimatedImpressions,
		Conflicts:            conflicts,
		DailyForecast:        make([]models.DailyForecast, 0),
		Warnings:             make([]string, 0),
	}

	// Calculate estimated spend and CTR
	switch req.BudgetType {
	case models.BudgetTypeCPM:
		response.EstimatedSpend = float64(response.EstimatedImpressions) * req.CPM / 1000.0
		response.EstimatedCTR = inventory.AverageCTR
		response.EstimatedClicks = int64(float64(response.EstimatedImpressions) * response.EstimatedCTR)
	case models.BudgetTypeCPC:
		response.EstimatedCTR = inventory.AverageCTR
		response.EstimatedClicks = int64(float64(response.EstimatedImpressions) * response.EstimatedCTR)
		response.EstimatedSpend = float64(response.EstimatedClicks) * req.CPC
	case models.BudgetTypeFlat:
		response.EstimatedSpend = req.Budget
	}

	// Cap spend at budget
	if response.EstimatedSpend > req.Budget {
		response.EstimatedSpend = req.Budget
		// Adjust impressions accordingly
		if req.BudgetType == models.BudgetTypeCPM {
			response.EstimatedImpressions = int64(req.Budget * 1000.0 / req.CPM)
		}
	}

	// Build daily forecast
	current := req.StartDate
	for !current.After(req.EndDate) {
		dailyOpps := inventory.DailyBreakdown[current.Format("2006-01-02")]
		dailyAvail := int64(float64(dailyOpps) * inventory.FillRate)
		dailyEst := calculateDailyAllocation(response.EstimatedImpressions, req.StartDate, req.EndDate, current, req.Pacing)

		daily := models.DailyForecast{
			Date:                 current,
			Opportunities:        dailyOpps,
			AvailableImpressions: dailyAvail,
			EstimatedImpressions: dailyEst,
		}

		switch req.BudgetType {
		case models.BudgetTypeCPM:
			daily.EstimatedSpend = float64(dailyEst) * req.CPM / 1000.0
			daily.EstimatedClicks = int64(float64(dailyEst) * response.EstimatedCTR)
		case models.BudgetTypeCPC:
			daily.EstimatedClicks = int64(float64(dailyEst) * response.EstimatedCTR)
			daily.EstimatedSpend = float64(daily.EstimatedClicks) * req.CPC
		}

		response.DailyForecast = append(response.DailyForecast, daily)
		current = current.AddDate(0, 0, 1)
	}

	// Calculate fill rate
	if response.TotalOpportunities > 0 {
		response.FillRate = float64(response.AvailableImpressions) / float64(response.TotalOpportunities)
	}

	// Add warnings
	if len(patterns) == 0 {
		response.Warnings = append(response.Warnings, "No historical data found for the specified targeting criteria")
	}
	if inventory.DataDays < 7 {
		response.Warnings = append(response.Warnings, fmt.Sprintf("Limited historical data available (%d days)", inventory.DataDays))
	}
	if len(conflicts) > 10 {
		response.Warnings = append(response.Warnings, fmt.Sprintf("High competition detected: %d conflicting line items", len(conflicts)))
	}

	return response
}

// calculateDailyAllocation determines how many impressions to allocate to a specific day
func calculateDailyAllocation(totalImpressions int64, startDate, endDate, currentDate time.Time, pacing string) int64 {
	totalDays := int(endDate.Sub(startDate).Hours()/24) + 1
	dayIndex := int(currentDate.Sub(startDate).Hours() / 24)

	switch pacing {
	case "ASAP":
		// Front-load delivery
		remainingDays := totalDays - dayIndex
		if remainingDays <= 0 {
			return 0
		}
		// Allocate more to earlier days
		weight := float64(remainingDays) / float64(totalDays)
		return int64(float64(totalImpressions) * weight * 2 / float64(totalDays))
	case "Even", "":
		// Even distribution
		return totalImpressions / int64(totalDays)
	default:
		// Default to even
		return totalImpressions / int64(totalDays)
	}
}
