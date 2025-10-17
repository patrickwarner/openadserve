package logic

import (
	"fmt"
	"time"

	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Default frequency cap settings if a campaign does not specify them
const (
	DefaultFrequencyCap    = 3
	DefaultFrequencyWindow = 1 * time.Minute
)

// HasUserExceededFrequencyCap returns true if the user has exceeded the allowed
// number of impressions for a line item within its configured window.
func HasUserExceededFrequencyCap(store *db.RedisStore, userID string, publisherID, lineItemID int, dataStore models.AdDataStore) (bool, error) {
	if store == nil || store.Client == nil {
		return false, ErrNilRedisStore
	}
	cap := DefaultFrequencyCap

	li := dataStore.GetLineItem(publisherID, lineItemID)
	if li != nil && li.FrequencyCap > 0 {
		cap = li.FrequencyCap
	}

	// Get current count without incrementing
	key := fmt.Sprintf("freqcap:%s:%d", userID, lineItemID)
	val, err := store.Client.Get(store.Ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			// No impressions yet, allow the ad
			return false, nil
		}
		zap.L().Error("redis freqcap", zap.Error(err))
		// Fail open — allow the ad if Redis is down or slow
		return false, nil
	}
	return val >= int64(cap), nil
}

// IncrementFrequencyCap increments the frequency cap count for a user and line item.
// This should be called AFTER successful ad serving, not during filtering.
func IncrementFrequencyCap(store *db.RedisStore, userID string, publisherID, lineItemID int, dataStore models.AdDataStore) error {
	if store == nil || store.Client == nil {
		return ErrNilRedisStore
	}

	window := DefaultFrequencyWindow
	li := dataStore.GetLineItem(publisherID, lineItemID)
	if li != nil && li.FrequencyWindow > 0 {
		window = li.FrequencyWindow
	}

	_, err := store.IncrementImpression(userID, lineItemID, window)
	if err != nil {
		zap.L().Error("failed to increment frequency cap", zap.Error(err))
		return err
	}
	return nil
}
