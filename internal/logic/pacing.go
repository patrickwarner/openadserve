// Package logic contains the runtime decision making used by the ad server.
//
// The pacing routines in this file determine when an individual line item is
// allowed to serve based on its chosen pacing model. Each line item specifies
// one of three models:
//   - PacingASAP delivers impressions as quickly as possible until a daily cap
//     is reached. It is best suited for short, bursty campaigns.
//   - PacingEven spreads delivery uniformly throughout the day so budget isn't
//     exhausted early.
//   - PacingPID uses a simple proportional–integral–derivative controller to
//     adapt the serving rate to real-time traffic. This reacts smoothly to
//     spikes and lulls while still respecting the daily cap.
//
// Redis is used as the backing store for serve and impression counters as well
// as PID controller state. All keys are scoped to the current day so each day's
// delivery is tracked independently.
package logic

import (
	"fmt"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// nowFn is used to get the current time. In production it's time.Now,
// but in tests we can replace it to simulate different times of day.
var nowFn = time.Now

// checkPIDPacing applies the PID algorithm for a single line item.
//
// The function compares the number of serves recorded today against the
// expected count for the current time and updates the stored error and integral
// values in Redis. It returns true when delivery should continue and false when
// the line item ought to hold back. The daily impression cap is enforced
// regardless of the controller output.
func checkPIDPacing(store *db.RedisStore, lineItemID int, count, capDaily int64, today string, cfg config.Config) bool {
	// Hard safety check - never exceed daily cap regardless of PID output
	if count >= capDaily {
		return false
	}

	now := nowFn()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	elapsed := now.Sub(start)
	target := float64(capDaily) * (float64(elapsed) / float64(24*time.Hour))

	errKey := fmt.Sprintf("pid:last:%d:%s", lineItemID, today)
	intKey := fmt.Sprintf("pid:int:%d:%s", lineItemID, today)

	lastErr, _ := store.Client.Get(store.Ctx, errKey).Float64()
	integral, _ := store.Client.Get(store.Ctx, intKey).Float64()

	errorVal := target - float64(count)
	derivative := errorVal - lastErr
	integral += errorVal

	// Implement integral windup protection to prevent excessive accumulation
	maxIntegral := float64(capDaily) * 0.5
	if integral > maxIntegral {
		integral = maxIntegral
	} else if integral < -maxIntegral {
		integral = -maxIntegral
	}

	// Use configurable PID coefficients with conservative defaults
	kp := cfg.PIDKp
	ki := cfg.PIDKi
	kd := cfg.PIDKd
	control := kp*errorVal + ki*integral + kd*derivative

	store.Client.Set(store.Ctx, errKey, fmt.Sprintf("%f", errorVal), 24*time.Hour)
	store.Client.Set(store.Ctx, intKey, fmt.Sprintf("%f", integral), 24*time.Hour)

	return control > 0
}

// IsLineItemPacingEligible evaluates whether a line item is allowed to serve at
// the current moment. It performs all read-only checks against Redis and the
// configured line item but does **not** modify any counters. Incrementing the
// serve or impression count is the caller's responsibility after an ad is
// actually delivered.
//
// The function enforces start and end dates, click caps and the pacing strategy
// selected for the line item. Eligibility is determined using the counters
// stored in Redis, which are keyed by line item and day.
func IsLineItemPacingEligible(store *db.RedisStore, publisherID, lineItemID int, dataStore models.AdDataStore, cfg config.Config) (bool, error) {
	if store == nil || store.Client == nil {
		return false, ErrNilRedisStore
	}
	// Look up the line item configuration
	li := dataStore.GetLineItem(publisherID, lineItemID)

	if li == nil {
		return true, nil // no config, allow by default
	}
	if !li.Active {
		return false, nil
	}

	now := nowFn()
	if !li.StartDate.IsZero() && now.Before(li.StartDate) {
		return false, nil
	}
	if !li.EndDate.IsZero() && now.After(li.EndDate) {
		return false, nil
	}

	capDaily := int64(li.DailyImpressionCap)

	// Check click cap before doing any pacing logic
	if li.DailyClickCap > 0 {
		today := now.Format("2006-01-02")
		clickKey := fmt.Sprintf("clicks:lineitem:%d:%s", lineItemID, today)
		clicks, err := store.Client.Get(store.Ctx, clickKey).Int64()
		if err != nil && err != redis.Nil {
			zap.L().Error("redis get clicks", zap.Error(err))
		}
		if clicks >= int64(li.DailyClickCap) {
			return false, nil
		}
	}

	// Build Redis key for today's serve count (used for pacing decisions)
	today := now.Format("2006-01-02")
	key := fmt.Sprintf("pacing:serves:%d:%s", lineItemID, today)

	// Fetch current serve count (zero if missing or parse error)
	count, err := store.Client.Get(store.Ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			// Key doesn't exist, start with 0
			count = 0
		} else {
			zap.L().Error("redis get serves", zap.Error(err))
			count = 0
		}
	}

	switch li.PaceType {
	case models.PacingASAP:
		if capDaily > 0 && count >= capDaily {
			return false, nil
		}

	case models.PacingEven:
		if capDaily > 0 {
			// fraction of day elapsed
			now := nowFn()
			start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			elapsed := now.Sub(start)
			allowed := int64(float64(capDaily) * (float64(elapsed) / float64(24*time.Hour)))
			if count >= allowed {
				return false, nil
			}
		}
	case models.PacingPID:
		if capDaily > 0 {
			if !checkPIDPacing(store, lineItemID, count, capDaily, today, cfg) {
				return false, nil
			}
		}
	}

	// No longer increment counter here - that happens after successful serving
	return true, nil
}

// IsLineItemPacingEligibleWithReason performs the same logic as
// IsLineItemPacingEligible but also returns a short string explaining why a line
// item was deemed ineligible. This is useful for debugging or surfacing pacing
// diagnostics to callers.
func IsLineItemPacingEligibleWithReason(store *db.RedisStore, publisherID, lineItemID int, dataStore models.AdDataStore, cfg config.Config) (bool, string, error) {
	if store == nil || store.Client == nil {
		return false, "redis_unavailable", ErrNilRedisStore
	}
	// Look up the line item configuration
	li := dataStore.GetLineItem(publisherID, lineItemID)

	if li == nil {
		return true, "", nil // no config, allow by default
	}
	if !li.Active {
		return false, "inactive", nil
	}

	now := nowFn()
	if !li.StartDate.IsZero() && now.Before(li.StartDate) {
		return false, "not_started", nil
	}
	if !li.EndDate.IsZero() && now.After(li.EndDate) {
		return false, "expired", nil
	}

	capDaily := int64(li.DailyImpressionCap)

	// Check click cap before doing any pacing logic
	if li.DailyClickCap > 0 {
		today := now.Format("2006-01-02")
		clickKey := fmt.Sprintf("clicks:lineitem:%d:%s", lineItemID, today)
		clicks, err := store.Client.Get(store.Ctx, clickKey).Int64()
		if err != nil && err != redis.Nil {
			zap.L().Error("redis get clicks", zap.Error(err))
		}
		if clicks >= int64(li.DailyClickCap) {
			return false, "click_cap_reached", nil
		}
	}

	// Build Redis key for today's serve count (used for pacing decisions)
	today := now.Format("2006-01-02")
	key := fmt.Sprintf("pacing:serves:%d:%s", lineItemID, today)

	// Fetch current serve count (zero if missing or parse error)
	count, err := store.Client.Get(store.Ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			// Key doesn't exist, start with 0
			count = 0
		} else {
			zap.L().Error("redis get serves", zap.Error(err))
			count = 0
		}
	}

	switch li.PaceType {
	case models.PacingASAP:
		if capDaily > 0 && count >= capDaily {
			return false, "asap_daily_cap_reached", nil
		}

	case models.PacingEven:
		if capDaily > 0 {
			// fraction of day elapsed
			now := nowFn()
			start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			elapsed := now.Sub(start)
			allowed := int64(float64(capDaily) * (float64(elapsed) / float64(24*time.Hour)))
			if count >= allowed {
				return false, "even_pacing_throttled", nil
			}
		}
	case models.PacingPID:
		if capDaily > 0 {
			if !checkPIDPacing(store, lineItemID, count, capDaily, today, cfg) {
				return false, "pid_pacing_throttled", nil
			}
		}
	}

	return true, "", nil
}

// IncrementLineItemServes records that a line item attempted to serve an ad.
// The serve counter is used by the pacing algorithms and is independent of the
// impression counter used for billing. This function should be invoked as soon
// as an ad is selected so that subsequent eligibility checks see the updated
// value.
func IncrementLineItemServes(store *db.RedisStore, lineItemID int) error {
	if store == nil || store.Client == nil {
		return ErrNilRedisStore
	}

	now := nowFn()
	today := now.Format("2006-01-02")
	key := fmt.Sprintf("pacing:serves:%d:%s", lineItemID, today)

	// Increment with 24h TTL on first set
	newVal, err := store.Client.Incr(store.Ctx, key).Result()
	if err != nil {
		zap.L().Error("redis incr serves", zap.Error(err))
		return err
	}
	if newVal == 1 {
		store.Client.Expire(store.Ctx, key, 24*time.Hour)
	}
	return nil
}

// IncrementLineItemImpressions bumps the impression counter for a line item.
// The impression count is used for billing and reporting rather than pacing and
// should only be incremented once the impression tracking pixel or equivalent
// confirmation has fired.
func IncrementLineItemImpressions(store *db.RedisStore, lineItemID int) error {
	if store == nil || store.Client == nil {
		return ErrNilRedisStore
	}

	now := nowFn()
	today := now.Format("2006-01-02")
	key := fmt.Sprintf("pacing:impressions:%d:%s", lineItemID, today)

	// Increment with 24h TTL on first set
	newVal, err := store.Client.Incr(store.Ctx, key).Result()
	if err != nil {
		zap.L().Error("redis incr impressions", zap.Error(err))
		return err
	}
	if newVal == 1 {
		store.Client.Expire(store.Ctx, key, 24*time.Hour)
	}
	return nil
}
