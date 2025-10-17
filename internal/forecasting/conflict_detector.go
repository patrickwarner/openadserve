package forecasting

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

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

		// Convert numeric priority from request to string priority, then to rank
		var reqRank int
		if req.Priority > 0 {
			// Convert numeric priority to string priority then get rank
			switch req.Priority {
			case 1:
				reqRank = models.PriorityRank(models.PriorityHigh) // rank 0
			case 2:
				reqRank = models.PriorityRank(models.PriorityMedium) // rank 1
			case 3:
				reqRank = models.PriorityRank(models.PriorityLow) // rank 2
			default:
				reqRank = models.PriorityRank(models.PriorityLow) // default to lowest priority
			}
		} else {
			// Default to high priority if not specified
			reqRank = models.PriorityRank(models.PriorityHigh)
		}

		if liRank < reqRank {
			conflictType = "higher_priority"
		} else if liRank > reqRank {
			conflictType = "lower_priority"
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
			Priority:          liRank, // Use numeric rank instead of string
			OverlapPercentage: overlapPct,
			ConflictType:      conflictType,
		}

		// Estimate impact based on priority and remaining budget
		switch conflictType {
		case "higher_priority":
			// Higher priority items will take inventory first
			remainingBudget := li.BudgetAmount - li.Spend
			if remainingBudget > 0 && req.CPM > 0 {
				// Rough estimate: higher priority could take significant inventory
				conflict.EstimatedImpact = int64(overlapPct * 0.7 * float64(req.Budget/req.CPM*1000))
			}
		case "lower_priority":
			// We might steal from lower priority items
			if li.CPM > 0 {
				conflict.EstimatedImpact = int64(overlapPct * 0.3 * float64(li.BudgetAmount/li.CPM*1000))
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
