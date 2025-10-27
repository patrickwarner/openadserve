package filters

import (
	"context"
	"fmt"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
)

// SinglePassFilter performs all filtering operations in a single pass using simple slice operations
type SinglePassFilter struct {
	store     *db.RedisStore
	dataStore models.AdDataStore
	cfg       config.Config
}

// NewSinglePassFilter creates an optimized single-pass filter
func NewSinglePassFilter(store *db.RedisStore, dataStore models.AdDataStore, cfg config.Config) *SinglePassFilter {
	return &SinglePassFilter{
		store:     store,
		dataStore: dataStore,
		cfg:       cfg,
	}
}

// FilterCreatives applies all filters in a single pass using simple slice operations
func (spf *SinglePassFilter) FilterCreatives(
	ctx context.Context,
	creatives []models.Creative,
	targetingCtx models.TargetingContext,
	width, height int,
	allowedFormats []string,
	userID string,
) ([]models.Creative, error) {

	if len(creatives) == 0 {
		return nil, nil
	}

	// Pre-allocate result slice with reasonable capacity
	filtered := make([]models.Creative, 0, len(creatives))

	// Collect Redis batch data as we filter
	var creativesForRedis []models.Creative

	// Single pass through all creatives - optimized inline logic
	for _, c := range creatives {
		// 1. Active check
		li := spf.dataStore.GetLineItem(c.PublisherID, c.LineItemID)
		if li == nil || !li.Active {
			continue
		}

		// 2. Targeting check
		if !logic.MatchesTargeting(c, targetingCtx, spf.dataStore) {
			continue
		}

		// 3. Size/format check
		if !creativeFitsPlacement(c, width, height, allowedFormats) {
			continue
		}

		// Store line item in creative for later use
		if c.LineItem == nil {
			c.LineItem = li
		}

		// Add to intermediate result
		filtered = append(filtered, c)

		// Prepare for Redis operations if needed
		if spf.store != nil && spf.store.Client != nil && (li.FrequencyCap > 0 || true) { // always check pacing
			creativesForRedis = append(creativesForRedis, c)
		}
	}

	// Early exit if no creatives passed basic filters
	if len(filtered) == 0 {
		return nil, nil
	}

	// Apply Redis-based filters if available
	if spf.store != nil && spf.store.Client != nil && len(creativesForRedis) > 0 {
		finalFiltered, err := spf.applyRedisFilters(filtered, creativesForRedis, userID)
		if err != nil {
			return nil, err
		}
		return finalFiltered, nil
	}

	return filtered, nil
}

// applyRedisFilters applies frequency and pacing filters using optimized batch operations
func (spf *SinglePassFilter) applyRedisFilters(
	preFiltered []models.Creative,
	creativesForRedis []models.Creative,
	userID string,
) ([]models.Creative, error) {

	// Batch frequency check
	exceeded, err := logic.BatchFrequencyCheck(spf.store, userID, creativesForRedis, spf.dataStore)
	if err != nil {
		return nil, err
	}

	// Batch pacing check
	eligible, err := logic.BatchPacingCheck(spf.store, creativesForRedis, spf.dataStore, spf.cfg)
	if err != nil {
		return nil, err
	}

	// Filter results based on Redis checks
	finalResult := make([]models.Creative, 0, len(preFiltered))

	for _, c := range preFiltered {
		creativeKey := fmt.Sprintf("%d_%d", c.PublisherID, c.LineItemID)

		// Check frequency cap
		if exceeded[creativeKey] {
			continue
		}

		// Check pacing
		if !eligible[creativeKey] {
			continue
		}

		finalResult = append(finalResult, c)
	}

	// Check for pacing limit error
	if len(finalResult) == 0 && len(preFiltered) > 0 {
		return nil, ErrPacingLimitReached
	}

	return finalResult, nil
}

// FilterCreativesWithTrace performs single-pass filtering with detailed tracing
func (spf *SinglePassFilter) FilterCreativesWithTrace(
	ctx context.Context,
	creatives []models.Creative,
	targetingCtx models.TargetingContext,
	width, height int,
	allowedFormats []string,
	userID string,
	trace *logic.SelectionTrace,
) ([]models.Creative, error) {

	// Record initial state
	if trace != nil {
		trace.AddStep("single_pass_start", creatives)
	}

	// Perform filtering
	filtered, err := spf.FilterCreatives(ctx, creatives, targetingCtx, width, height,
		allowedFormats, userID)

	// Record final state with details
	if trace != nil {
		details := make(map[string]string)
		details["input_count"] = fmt.Sprintf("%d", len(creatives))
		details["output_count"] = fmt.Sprintf("%d", len(filtered))
		details["filter_type"] = "simple_single_pass"

		trace.AddStepWithDetails("single_pass_complete", filtered, details)
	}

	return filtered, err
}
