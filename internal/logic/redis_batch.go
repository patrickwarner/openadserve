package logic

import (
	"fmt"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/redis/go-redis/v9"
)

// BatchFrequencyResults holds frequency cap check results for multiple creatives
func BatchFrequencyCheck(store *db.RedisStore, userID string, creatives []models.Creative, dataStore models.AdDataStore) (map[string]bool, error) {
	if store == nil || store.Client == nil {
		return nil, ErrNilRedisStore
	}

	if len(creatives) == 0 {
		return make(map[string]bool), nil
	}

	// Use Redis pipeline to batch all GET commands
	pipe := store.Client.Pipeline()

	// Map to store pipeline commands and creative info
	commands := make(map[string]*redis.StringCmd)
	creativesByKey := make(map[string]models.Creative)

	// Add all frequency cap GETs to pipeline
	for _, c := range creatives {
		key := fmt.Sprintf("freqcap:%s:%d", userID, c.LineItemID)
		creativeKey := fmt.Sprintf("%d_%d", c.PublisherID, c.LineItemID)

		commands[creativeKey] = pipe.Get(store.Ctx, key)
		creativesByKey[creativeKey] = c
	}

	// Execute pipeline
	_, err := pipe.Exec(store.Ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("pipeline exec failed: %w", err)
	}

	// Process results
	result := make(map[string]bool)
	for creativeKey, cmd := range commands {
		creative := creativesByKey[creativeKey]

		// Get frequency cap for this line item
		cap := DefaultFrequencyCap
		li := dataStore.GetLineItem(creative.PublisherID, creative.LineItemID)
		if li != nil && li.FrequencyCap > 0 {
			cap = li.FrequencyCap
		}

		// Get current count (0 if key doesn't exist)
		count, err := cmd.Int64()
		if err == redis.Nil {
			count = 0
		} else if err != nil {
			count = 0 // Fail open
		}

		// Check if frequency cap is exceeded
		result[creativeKey] = count >= int64(cap)
	}

	return result, nil
}

// BatchPacingCheck performs pacing checks for multiple creatives using pipeline, excluding PID keys
func BatchPacingCheck(store *db.RedisStore, creatives []models.Creative, dataStore models.AdDataStore, cfg config.Config) (map[string]bool, error) {
	if store == nil || store.Client == nil {
		return nil, ErrNilRedisStore
	}

	if len(creatives) == 0 {
		return make(map[string]bool), nil
	}

	now := nowFn()
	today := now.Format("2006-01-02")

	// Separate PID creatives (can't be batched) from batchable ones
	var batchableCreatives []models.Creative
	var pidCreatives []models.Creative

	for _, c := range creatives {
		li := dataStore.GetLineItem(c.PublisherID, c.LineItemID)
		if li != nil && li.PaceType == models.PacingPID {
			pidCreatives = append(pidCreatives, c)
		} else {
			batchableCreatives = append(batchableCreatives, c)
		}

	}

	result := make(map[string]bool)

	// Handle batchable creatives with pipeline
	if len(batchableCreatives) > 0 {
		pipe := store.Client.Pipeline()

		// Maps to store pipeline commands
		pacingCommands := make(map[string]*redis.StringCmd)
		clickCommands := make(map[string]*redis.StringCmd)

		// Add pacing and click count GETs to pipeline
		for _, c := range batchableCreatives {
			creativeKey := fmt.Sprintf("%d_%d", c.PublisherID, c.LineItemID)

			// Add serve count GET (for pacing decisions)
			pacingKey := fmt.Sprintf("pacing:serves:%d:%s", c.LineItemID, today)
			pacingCommands[creativeKey] = pipe.Get(store.Ctx, pacingKey)

			// Add click count GET if needed
			li := dataStore.GetLineItem(c.PublisherID, c.LineItemID)
			if li != nil && li.DailyClickCap > 0 {
				clickKey := fmt.Sprintf("clicks:lineitem:%d:%s", c.LineItemID, today)
				clickCommands[creativeKey] = pipe.Get(store.Ctx, clickKey)
			}
		}

		// Execute pipeline
		_, err := pipe.Exec(store.Ctx)
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("pacing pipeline exec failed: %w", err)
		}

		// Process batchable results
		for _, c := range batchableCreatives {
			creativeKey := fmt.Sprintf("%d_%d", c.PublisherID, c.LineItemID)
			li := dataStore.GetLineItem(c.PublisherID, c.LineItemID)

			if li == nil {
				result[creativeKey] = true // No config, allow by default
				continue
			}

			// Check basic eligibility
			if !li.Active ||
				(!li.StartDate.IsZero() && now.Before(li.StartDate)) ||
				(!li.EndDate.IsZero() && now.After(li.EndDate)) {
				result[creativeKey] = false
				continue
			}

			// Check click cap if applicable
			if clickCmd, exists := clickCommands[creativeKey]; exists {
				clickCount, err := clickCmd.Int64()
				if err == redis.Nil {
					clickCount = 0
				} else if err != nil {
					clickCount = 0 // Fail open
				}

				if clickCount >= int64(li.DailyClickCap) {
					result[creativeKey] = false
					continue
				}
			}

			// Check pacing
			pacingCmd := pacingCommands[creativeKey]
			count, err := pacingCmd.Int64()
			if err == redis.Nil {
				count = 0
			} else if err != nil {
				count = 0 // Fail open
			}

			capDaily := int64(li.DailyImpressionCap)

			switch li.PaceType {
			case models.PacingASAP:
				result[creativeKey] = capDaily <= 0 || count < capDaily

			case models.PacingEven:
				if capDaily > 0 {
					start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
					elapsed := now.Sub(start)
					allowed := int64(float64(capDaily) * (float64(elapsed) / float64(24*time.Hour)))
					result[creativeKey] = count < allowed
				} else {
					result[creativeKey] = true
				}

			default:
				result[creativeKey] = true
			}
		}
	}

	// Handle PID creatives individually (can't be batched due to immediate state updates)
	for _, c := range pidCreatives {
		creativeKey := fmt.Sprintf("%d_%d", c.PublisherID, c.LineItemID)
		eligible, err := IsLineItemPacingEligible(store, c.PublisherID, c.LineItemID, dataStore, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to check PID pacing for creative %s: %w", creativeKey, err)
		}
		result[creativeKey] = eligible
	}

	return result, nil
}
