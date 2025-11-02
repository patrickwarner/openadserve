package forecasting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// InventoryAvailability represents the available inventory for a forecast
type InventoryAvailability struct {
	TotalOpportunities   int64
	AvailableImpressions int64
	EstimatedImpressions int64
	FillRate             float64
	AverageCTR           float64
	DataDays             int
	DailyBreakdown       map[string]int64 // date -> opportunities
}

// calculateAvailableInventory estimates the available inventory based on historical patterns
func (e *Engine) calculateAvailableInventory(ctx context.Context, req *models.ForecastRequest, patterns []*TrafficPattern) (*InventoryAvailability, error) {
	inventory := &InventoryAvailability{
		DailyBreakdown: make(map[string]int64),
	}

	if len(patterns) == 0 {
		// No historical data - return zero availability
		return inventory, nil
	}

	// Aggregate patterns by day
	dailyPatterns := aggregatePatternsByDay(patterns)
	inventory.DataDays = len(dailyPatterns)

	// Calculate averages from historical data
	var totalOpps, totalImps, totalClicks int64
	for _, pattern := range dailyPatterns {
		totalOpps += pattern.Opportunities
		totalImps += pattern.Impressions
		totalClicks += pattern.Clicks
	}

	// Calculate average daily traffic
	avgDailyOpps := totalOpps / int64(len(dailyPatterns))

	// Calculate overall rates
	if totalOpps > 0 {
		inventory.FillRate = float64(totalImps) / float64(totalOpps)
	}
	if totalImps > 0 {
		inventory.AverageCTR = float64(totalClicks) / float64(totalImps)
	}

	// Project inventory for the forecast period
	current := req.StartDate
	for !current.After(req.EndDate) {
		dayStr := current.Format("2006-01-02")

		// Check if we have historical data for this specific day and placement combination
		var dailyOpportunities int64
		foundHistoricalData := false

		// When forecasting for specific placements, look for placement-specific patterns
		if len(req.PlacementIDs) > 0 {
			for _, placementID := range req.PlacementIDs {
				key := fmt.Sprintf("%s_%s", dayStr, placementID)
				if historicalPattern, exists := dailyPatterns[key]; exists {
					dailyOpportunities += historicalPattern.Opportunities
					foundHistoricalData = true
				}
			}
		} else {
			// For publisher-wide forecasts, look for any pattern matching the date
			for key, historicalPattern := range dailyPatterns {
				if strings.HasPrefix(key, dayStr+"_") || key == dayStr {
					dailyOpportunities += historicalPattern.Opportunities
					foundHistoricalData = true
				}
			}
		}

		if foundHistoricalData {
			// Use historical data if available
			inventory.DailyBreakdown[dayStr] = dailyOpportunities
			inventory.TotalOpportunities += dailyOpportunities
		} else {
			// Otherwise use average with day-of-week adjustment
			dayOfWeek := current.Weekday()
			adjustedOpps := applyDayOfWeekAdjustment(avgDailyOpps, dayOfWeek)
			inventory.DailyBreakdown[dayStr] = adjustedOpps
			inventory.TotalOpportunities += adjustedOpps
		}

		current = current.AddDate(0, 0, 1)
	}

	// Calculate available impressions considering both unfilled inventory and potential preemption
	currentFillRate := inventory.FillRate
	baseAvailable := int64(float64(inventory.TotalOpportunities) * (1 - currentFillRate))

	if currentFillRate > 0.95 {
		// If fill rate is very high, base availability is limited
		baseAvailable = int64(float64(inventory.TotalOpportunities) * 0.05)
	}

	inventory.AvailableImpressions = baseAvailable

	// Initial estimate before conflict resolution
	inventory.EstimatedImpressions = inventory.AvailableImpressions

	// Apply daily cap if specified
	if req.DailyCap > 0 {
		days := int(req.EndDate.Sub(req.StartDate).Hours()/24) + 1
		maxImpressions := int64(req.DailyCap * days)
		if inventory.EstimatedImpressions > maxImpressions {
			inventory.EstimatedImpressions = maxImpressions
		}
	}

	e.Logger.Info("calculated inventory",
		zap.Int64("total_opportunities", inventory.TotalOpportunities),
		zap.Int64("available_impressions", inventory.AvailableImpressions),
		zap.Float64("fill_rate", inventory.FillRate),
		zap.Int("data_days", inventory.DataDays),
	)

	return inventory, nil
}

// applyDayOfWeekAdjustment applies typical traffic patterns by day of week
func applyDayOfWeekAdjustment(baseValue int64, dayOfWeek time.Weekday) int64 {
	// Typical web traffic patterns - adjust as needed
	adjustments := map[time.Weekday]float64{
		time.Sunday:    0.85, // Lower weekend traffic
		time.Monday:    1.05, // Monday spike
		time.Tuesday:   1.10, // Peak weekday
		time.Wednesday: 1.10, // Peak weekday
		time.Thursday:  1.05, // Slightly lower
		time.Friday:    0.95, // Friday dropoff
		time.Saturday:  0.80, // Lower weekend traffic
	}

	if adj, ok := adjustments[dayOfWeek]; ok {
		return int64(float64(baseValue) * adj)
	}
	return baseValue
}
