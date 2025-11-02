package forecasting

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// DefaultECPMConflictThreshold is the default eCPM difference threshold for conflict determination
// A 10% difference is used to account for CTR prediction variance and market dynamics
const DefaultECPMConflictThreshold = 1.1

// getECPMConflictThreshold returns the configurable eCPM threshold from environment
// or the default value. Threshold determines when same-priority bids are considered conflicts.
func getECPMConflictThreshold() float64 {
	if thresholdStr := os.Getenv("ECPM_CONFLICT_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.ParseFloat(thresholdStr, 64); err == nil && threshold > 1.0 {
			return threshold
		}
	}
	return DefaultECPMConflictThreshold
}

// detectConflicts identifies line items that compete for the same inventory
func (e *Engine) detectConflicts(ctx context.Context, req *models.ForecastRequest) ([]models.ConflictingLineItem, error) {
	conflicts := make([]models.ConflictingLineItem, 0)

	// Get all active line items for the publisher
	lineItems := e.AdStore.GetLineItemsByPublisher(req.PublisherID)

	for _, li := range lineItems {
		// Skip if line item is not active during forecast period
		if !isActiveInPeriod(&li, req.StartDate, req.EndDate) {
			continue
		}

		// Calculate targeting overlap
		overlapPct := calculateTargetingOverlap(&li, req)
		if overlapPct == 0 {
			continue // No overlap
		}

		// Determine conflict type based on priority (lower rank = higher priority)
		conflictType := "same_priority"
		liRank := models.PriorityRank(li.Priority)

		// Convert numeric priority index from request to priority string, then to rank
		// This approach works with any configured priority system (default: ["high", "medium", "low"])
		// Priority index 0 = highest priority, 1 = second highest, etc.
		// Example: req.Priority=0 -> "high" -> rank 0, req.Priority=1 -> "medium" -> rank 1

		// Validate priority index - return error for invalid input to fail fast
		if !models.ValidatePriorityIndex(req.Priority) {
			return nil, fmt.Errorf("invalid priority index %d, must be 0-%d",
				req.Priority, len(models.PriorityOrder)-1)
		}

		reqPriorityString := models.PriorityFromIndex(req.Priority)
		reqPriorityRank := models.PriorityRank(reqPriorityString)

		if liRank < reqPriorityRank {
			conflictType = "higher_priority"
		} else if liRank > reqPriorityRank {
			conflictType = "lower_priority"
		} else {
			// Same priority - check if bid difference resolves the conflict
			reqECPM := e.calculateForecastECPM(req)
			liECPM := e.calculateLineItemECPM(&li)
			threshold := getECPMConflictThreshold()

			// If our bid is significantly higher, this isn't really a conflict
			if reqECPM > liECPM*threshold {
				continue // Skip this "conflict" - we would clearly win
			}
			// If conflicting line item's bid is significantly higher, we would clearly lose
			if liECPM > reqECPM*threshold {
				conflictType = "higher_priority" // Treat as effective higher priority
			}
		}

		// Get campaign for name
		campaign := e.AdStore.GetCampaign(li.CampaignID)
		campaignName := ""
		if campaign != nil {
			campaignName = campaign.Name
		}

		conflict := models.ConflictingLineItem{
			LineItemID:        li.ID,
			LineItemName:      li.Name,
			CampaignID:        li.CampaignID,
			CampaignName:      campaignName,
			Priority:          models.PriorityToIndex(li.Priority), // Convert to index for consistent UI display
			OverlapPercentage: overlapPct,
			ConflictType:      conflictType,
		}

		// Estimate impact based on priority and bid competition
		reqECPM := e.calculateForecastECPM(req)
		liECPM := e.calculateLineItemECPM(&li)

		switch conflictType {
		case "higher_priority":
			// Higher priority items (or same priority with higher bids) will take inventory first
			remainingBudget := li.BudgetAmount - li.Spend
			if remainingBudget > 0 {
				// Calculate potential impressions from remaining budget
				var potentialImpressions int64
				if li.CPM > 0 {
					potentialImpressions = int64(remainingBudget / li.CPM * 1000)
				} else if reqECPM > 0 {
					// Use our eCPM as proxy for impression estimation
					potentialImpressions = int64(remainingBudget / reqECPM * 1000)
				}

				// Impact is based on overlap and competitive strength
				impactRate := overlapPct * 0.7 // Base impact rate
				// Increase impact if they have significantly higher eCPM
				if liECPM > reqECPM*1.2 {
					impactRate = overlapPct * 0.9 // Very high impact
				}

				conflict.EstimatedImpact = int64(impactRate * float64(potentialImpressions))
			}
		case "lower_priority":
			// We might preempt lower priority items, but impact depends on bid difference
			if li.CPM > 0 {
				remainingImpressions := int64(li.BudgetAmount / li.CPM * 1000)
				preemptionRate := overlapPct * 0.3 // Base preemption rate

				// Increase preemption if we have significantly higher eCPM
				if reqECPM > liECPM*1.2 {
					preemptionRate = overlapPct * 0.8 // High preemption
				} else if reqECPM > liECPM*1.05 {
					preemptionRate = overlapPct * 0.5 // Moderate preemption
				}

				conflict.EstimatedImpact = int64(preemptionRate * float64(remainingImpressions))
			}
		case "same_priority":
			// For truly competitive same-priority conflicts, impact is based on bid ratio
			remainingBudget := li.BudgetAmount - li.Spend
			if remainingBudget > 0 && li.CPM > 0 {
				remainingImpressions := int64(remainingBudget / li.CPM * 1000)

				// Market share based on eCPM ratio
				totalECPM := reqECPM + liECPM
				if totalECPM > 0 {
					// Our share of the contested inventory
					ourShare := reqECPM / totalECPM
					// Impact is the portion they would take from us
					conflict.EstimatedImpact = int64(overlapPct * (1 - ourShare) * float64(remainingImpressions))
				}
			}
		}

		conflicts = append(conflicts, conflict)
	}

	e.Logger.Info("detected conflicts",
		zap.Int("conflict_count", len(conflicts)),
		zap.Int("publisher_id", req.PublisherID),
	)

	return conflicts, nil
}

// isActiveInPeriod checks if a line item is active during the forecast period
func isActiveInPeriod(li *models.LineItem, startDate, endDate time.Time) bool {
	// Check if line item has started
	if li.StartDate.After(endDate) {
		return false
	}

	// Check if line item has ended
	if !li.EndDate.IsZero() && li.EndDate.Before(startDate) {
		return false
	}

	// Check if budget is exhausted
	if li.Spend >= li.BudgetAmount {
		return false
	}

	return true
}

// calculateTargetingOverlap calculates the percentage of inventory overlap
func calculateTargetingOverlap(li *models.LineItem, req *models.ForecastRequest) float64 {
	overlap := 1.0 // Start with 100% overlap

	// Country targeting (string vs slice)
	if li.Country != "" && len(req.Countries) > 0 {
		found := false
		for _, country := range req.Countries {
			if li.Country == country {
				found = true
				break
			}
		}
		if !found {
			return 0 // No country overlap
		}
		// Reduce overlap based on country specificity
		overlap *= 0.8
	}

	// Device type targeting (string vs slice)
	if li.DeviceType != "" && len(req.DeviceTypes) > 0 {
		found := false
		for _, deviceType := range req.DeviceTypes {
			if li.DeviceType == deviceType {
				found = true
				break
			}
		}
		if !found {
			return 0 // No device overlap
		}
		overlap *= 0.9
	}

	// Browser targeting (string vs slice)
	if li.Browser != "" && len(req.Browsers) > 0 {
		found := false
		for _, browser := range req.Browsers {
			if li.Browser == browser {
				found = true
				break
			}
		}
		if !found {
			return 0 // No browser overlap
		}
		overlap *= 0.95
	}

	// OS targeting (string vs slice)
	if li.OS != "" && len(req.OS) > 0 {
		found := false
		for _, os := range req.OS {
			if li.OS == os {
				found = true
				break
			}
		}
		if !found {
			return 0 // No OS overlap
		}
		overlap *= 0.95
	}

	// Key-value targeting
	if len(li.KeyValues) > 0 && len(req.KeyValues) > 0 {
		kvOverlap := calculateKeyValueOverlap(li.KeyValues, req.KeyValues)
		if kvOverlap == 0 {
			return 0 // No KV overlap
		}
		overlap *= kvOverlap
	}

	// Note: No placement targeting in LineItem model - placement IDs come from ad request

	return overlap
}

// calculateKeyValueOverlap calculates overlap between key-value maps
func calculateKeyValueOverlap(liKV, reqKV map[string]string) float64 {
	if len(liKV) == 0 || len(reqKV) == 0 {
		return 1.0 // No KV targeting means full overlap
	}

	matches := 0
	total := 0

	// Check each KV pair in line item
	for k, v := range liKV {
		total++
		if reqVal, exists := reqKV[k]; exists && reqVal == v {
			matches++
		} else if exists && reqVal != v {
			// Different value for same key = no overlap
			return 0
		}
	}

	// Also check if request has additional KVs not in line item
	for k, v := range reqKV {
		if liVal, exists := liKV[k]; exists && liVal != v {
			return 0 // Conflicting values
		}
	}

	if total == 0 {
		return 1.0
	}

	// Return percentage of matching KVs
	return float64(matches) / float64(total)
}

// calculateForecastECPM calculates the eCPM for a forecast request
// Supports only CPM and CPC budget types for simplicity
func (e *Engine) calculateForecastECPM(req *models.ForecastRequest) float64 {
	switch req.BudgetType {
	case models.BudgetTypeCPM:
		// For CPM campaigns, eCPM is the CPM bid
		return req.CPM
	case models.BudgetTypeCPC:
		// For CPC campaigns, estimate eCPM using baseline CTR
		// Use 1% baseline CTR for consistent eCPM calculation
		avgCTR := 0.01                 // 1% baseline
		return req.CPC * avgCTR * 1000 // Convert to per-mille basis
	default:
		return 0.0
	}
}

// calculateLineItemECPM calculates the eCPM for an existing line item
// Supports only CPM and CPC budget types for simplicity
func (e *Engine) calculateLineItemECPM(li *models.LineItem) float64 {
	if li == nil {
		return 0.0
	}

	// Calculate eCPM based on budget type
	switch li.BudgetType {
	case models.BudgetTypeCPM:
		return li.CPM
	case models.BudgetTypeCPC:
		// Use baseline CTR for existing line items
		// In production, this could use historical performance data
		avgCTR := 0.01 // 1% baseline CTR
		return li.CPC * avgCTR * 1000
	default:
		return 0.0
	}
}
