package filters

import (
	"errors"
	"fmt"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	logic "github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
)

// ErrPacingLimitReached is returned when all creatives are filtered out due to pacing limits.
var ErrPacingLimitReached = errors.New("line item pacing limit reached")

// FilterByTargeting returns creatives whose line items match the given targeting context.
func FilterByTargeting(creatives []models.Creative, ctx models.TargetingContext, dataStore models.AdDataStore) []models.Creative {
	var out []models.Creative
	for _, c := range creatives {
		if logic.MatchesTargeting(c, ctx, dataStore) {
			out = append(out, c)
		}
	}
	return out
}

// FilterBySize filters creatives that don't match the requested size or allowed formats.
func FilterBySize(creatives []models.Creative, width, height int, allowedFormats []string) []models.Creative {
	var out []models.Creative
	for _, c := range creatives {
		if creativeFitsPlacement(c, width, height, allowedFormats) {
			out = append(out, c)
		}
	}
	return out
}

// FilterByActive removes creatives whose line items are disabled.
func FilterByActive(creatives []models.Creative, dataStore models.AdDataStore) []models.Creative {
	var out []models.Creative
	for _, c := range creatives {
		li := dataStore.GetLineItem(c.PublisherID, c.LineItemID)
		// A creative must have a valid, active line item to pass.
		if li != nil && li.Active {
			out = append(out, c)
		}
	}
	return out
}

// FilterByFrequency removes creatives whose frequency cap has been exceeded for the user.
func FilterByFrequency(store *db.RedisStore, creatives []models.Creative, userID string, dataStore models.AdDataStore) ([]models.Creative, error) {
	if store == nil || store.Client == nil {
		return nil, logic.ErrNilRedisStore
	}

	// Use batched frequency checking
	exceeded, err := logic.BatchFrequencyCheck(store, userID, creatives, dataStore)
	if err != nil {
		return nil, err
	}

	var out []models.Creative
	for _, c := range creatives {
		creativeKey := fmt.Sprintf("%d_%d", c.PublisherID, c.LineItemID)
		if !exceeded[creativeKey] {
			out = append(out, c)
		}
	}
	return out, nil
}

// FilterByPacing filters creatives blocked by line item pacing rules. Returns ErrPacingLimitReached
// if all creatives are filtered out due to pacing caps.
func FilterByPacing(store *db.RedisStore, creatives []models.Creative, dataStore models.AdDataStore, cfg config.Config) ([]models.Creative, error) {
	if store == nil || store.Client == nil {
		return nil, logic.ErrNilRedisStore
	}

	// Use batched pacing checking (automatically handles PID creatives individually)
	eligible, err := logic.BatchPacingCheck(store, creatives, dataStore, cfg)
	if err != nil {
		return nil, err
	}

	var out []models.Creative
	for _, c := range creatives {
		creativeKey := fmt.Sprintf("%d_%d", c.PublisherID, c.LineItemID)
		if eligible[creativeKey] {
			out = append(out, c)
		}
	}

	if len(out) == 0 && len(creatives) > 0 {
		return nil, ErrPacingLimitReached
	}
	return out, nil
}

// FilterByPacingWithTrace filters creatives blocked by line item pacing rules and provides detailed trace info.
func FilterByPacingWithTrace(store *db.RedisStore, creatives []models.Creative, dataStore models.AdDataStore, cfg config.Config) ([]models.Creative, map[string]string, error) {
	if store == nil || store.Client == nil {
		return nil, map[string]string{"error": "redis_unavailable"}, logic.ErrNilRedisStore
	}

	var out []models.Creative
	rejectionCounts := make(map[string]int)

	for _, c := range creatives {
		eligible, reason, err := logic.IsLineItemPacingEligibleWithReason(store, c.PublisherID, c.LineItemID, dataStore, cfg)
		if err != nil {
			return nil, map[string]string{"error": err.Error()}, err
		}
		if eligible {
			out = append(out, c)
		} else {
			rejectionCounts[reason]++
		}
	}

	details := make(map[string]string)
	details["input_count"] = fmt.Sprintf("%d", len(creatives))
	details["output_count"] = fmt.Sprintf("%d", len(out))

	for reason, count := range rejectionCounts {
		details[fmt.Sprintf("rejected_%s", reason)] = fmt.Sprintf("%d", count)
	}

	if len(out) == 0 && len(creatives) > 0 {
		return nil, details, ErrPacingLimitReached
	}
	return out, details, nil
}

// creativeFitsPlacement checks that the creative matches the requested size and format constraints.
func creativeFitsPlacement(c models.Creative, width, height int, allowedFormats []string) bool {
	if width > 0 && c.Width != width {
		return false
	}
	if height > 0 && c.Height != height {
		return false
	}
	if len(allowedFormats) > 0 && c.Format != "" {
		for _, f := range allowedFormats {
			if f == c.Format {
				return true
			}
		}
		return false
	}
	return true
}
