package selectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	logic "github.com/patrickwarner/openadserve/internal/logic"
	filters "github.com/patrickwarner/openadserve/internal/logic/filters"
	"github.com/patrickwarner/openadserve/internal/logic/ratelimit"
	"github.com/patrickwarner/openadserve/internal/logic/render"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/patrickwarner/openadserve/internal/optimization"

	"go.uber.org/zap"
)

var (
	ErrNoEligibleAd       = errors.New("no eligible ad found for user")
	ErrPacingLimitReached = errors.New("line item pacing limit reached")
	ErrRateLimitReached   = errors.New("line item rate limit reached")
	ErrUnknownPlacement   = errors.New("unknown placement")
)

const defaultProgrammaticBidTimeout = 800 * time.Millisecond

var defaultCTROptimizationEnabled = func() bool {
	v := os.Getenv("CTR_OPTIMIZATION_ENABLED")
	if v == "" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return false
}()

// bid represents a programmatic bid response.
type bid struct {
	Price float64
	Adm   string
}

// defaultShuffleFn shuffles creatives using rand.Shuffle. It relies on the
// package-level random source which is not goroutine safe; the selector invokes
// it only in single-threaded code paths.
var defaultShuffleFn = func(creatives []models.Creative) {
	rand.Shuffle(len(creatives), func(i, j int) {
		creatives[i], creatives[j] = creatives[j], creatives[i]
	})
}

// ShuffleFn randomizes the creative slice. Tests may replace it for
// deterministic behavior.
var ShuffleFn = defaultShuffleFn

// fetchProgrammaticBid sends a minimal OpenRTB request to the given endpoint
// and returns the bid price and markup. Failures return zero values.
func fetchProgrammaticBid(ctx context.Context, endpoint string, width, height int) (float64, string, error) {
	reqBody := struct {
		Imp []struct {
			ID string `json:"id"`
			W  int    `json:"w"`
			H  int    `json:"h"`
		} `json:"imp"`
	}{Imp: []struct {
		ID string `json:"id"`
		W  int    `json:"w"`
		H  int    `json:"h"`
	}{{ID: "1", W: width, H: height}}}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	var out struct {
		SeatBid []struct {
			Bid []struct {
				Price float64 `json:"price"`
				Adm   string  `json:"adm"`
			} `json:"bid"`
		} `json:"seatbid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, "", err
	}
	if len(out.SeatBid) == 0 || len(out.SeatBid[0].Bid) == 0 {
		return 0, "", nil
	}
	return out.SeatBid[0].Bid[0].Price, out.SeatBid[0].Bid[0].Adm, nil
}

// RuleBasedSelector is the default Selector implementation that relies on the
// existing rule-based selection logic.
type RuleBasedSelector struct {
	rateLimiter            *ratelimit.LineItemLimiter
	ctrClient              *optimization.CTRPredictionClient
	logger                 *zap.Logger
	programmaticBidTimeout time.Duration
	ctrOptimizationEnabled bool
}

// NewRuleBasedSelector constructs a RuleBasedSelector and initializes the CTR
// optimization flag from the environment once.
func NewRuleBasedSelector() *RuleBasedSelector {
	return &RuleBasedSelector{
		programmaticBidTimeout: defaultProgrammaticBidTimeout,
		ctrOptimizationEnabled: defaultCTROptimizationEnabled,
	}
}

// SetRateLimiter configures rate limiting for this selector.
// Rate limiting is optional - if not set, no rate limiting is applied.
func (s *RuleBasedSelector) SetRateLimiter(rateLimiter *ratelimit.LineItemLimiter) {
	s.rateLimiter = rateLimiter
}

// SetCTRClient configures CTR prediction for CPC optimization.
// CTR optimization is optional - if not set, no optimization is applied.
func (s *RuleBasedSelector) SetCTRClient(ctrClient *optimization.CTRPredictionClient) {
	s.ctrClient = ctrClient
}

// SetProgrammaticBidTimeout configures the timeout used for programmatic bid requests.
// If zero, the default of 800ms is used.
func (s *RuleBasedSelector) SetProgrammaticBidTimeout(d time.Duration) {
	s.programmaticBidTimeout = d
}

// SetLogger configures the logger for this selector.
func (s *RuleBasedSelector) SetLogger(logger *zap.Logger) {
	s.logger = logger
}

// SetCTROptimizationEnabled overrides the CTR optimization flag.
func (s *RuleBasedSelector) SetCTROptimizationEnabled(enabled bool) {
	s.ctrOptimizationEnabled = enabled
}

// calculateOptimizedECPM calculates the eCPM for a line item, applying CTR optimization for CPC items.
func (s *RuleBasedSelector) calculateOptimizedECPM(li *models.LineItem, ctx models.TargetingContext, bids map[int]bid) float64 {
	if li == nil {
		return 0.0
	}

	// For programmatic line items, use the bid price
	if li.Type == models.LineItemTypeProgrammatic {
		if b, ok := bids[li.ID]; ok && b.Price > 0 {
			return b.Price
		}
		return 0.0
	}

	// Start with base eCPM
	baseECPM := li.ECPM

	// Apply CTR optimization only for CPC line items when enabled
	if s.ctrOptimizationEnabled && li.BudgetType == models.BudgetTypeCPC && s.ctrClient != nil {
		// Make CTR prediction request
		now := time.Now()
		predictionReq := &optimization.PredictionRequest{
			LineItemID: li.ID,
			DeviceType: ctx.DeviceType,
			Country:    ctx.Country,
			HourOfDay:  now.Hour(),
			DayOfWeek:  int(now.Weekday()),
		}

		// Get prediction with short timeout to avoid blocking ad serving
		predictionCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		prediction, err := s.ctrClient.GetPrediction(predictionCtx, predictionReq)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("CTR prediction failed, using base eCPM",
					zap.Error(err),
					zap.Int("line_item_id", li.ID))
			}
			return baseECPM
		}

		// Apply the boost multiplier to the base eCPM
		optimizedECPM := baseECPM * prediction.BoostMultiplier

		if s.logger != nil {
			s.logger.Debug("CTR optimization applied",
				zap.Int("line_item_id", li.ID),
				zap.Float64("base_ecpm", baseECPM),
				zap.Float64("ctr_score", prediction.CTRScore),
				zap.Float64("boost_multiplier", prediction.BoostMultiplier),
				zap.Float64("optimized_ecpm", optimizedECPM))
		}

		return optimizedECPM
	}

	return baseECPM
}

// SelectAd delegates to performSelection without tracing so existing behaviour
// is preserved.
func (s *RuleBasedSelector) SelectAd(store *db.RedisStore, database *db.DB, dataStore models.AdDataStore,
	placementID, userID string, width, height int,
	ctx models.TargetingContext, cfg config.Config) (*models.AdResponse, error) {
	return s.performSelection(store, database, dataStore, placementID, userID, width, height, ctx, nil, cfg)
}

// SelectAd chooses a creative for a given placement and user. Candidate
// creatives are shuffled before evaluating frequency cap and pacing so that the
// order of evaluation is random on each call.
func SelectAd(store *db.RedisStore, database *db.DB, dataStore models.AdDataStore, placementID, userID string, width, height int, ctx models.TargetingContext, cfg config.Config) (*models.AdResponse, error) {
	// Use default selector without rate limiting or CTR optimization for backward compatibility
	selector := NewRuleBasedSelector()
	return selector.performSelection(store, database, dataStore, placementID, userID, width, height, ctx, nil, cfg)
}

// SelectAdWithTrace behaves like SelectAd but records intermediate candidate lists
// in the provided SelectionTrace.
func (s *RuleBasedSelector) SelectAdWithTrace(store *db.RedisStore, database *db.DB, dataStore models.AdDataStore,
	placementID, userID string, width, height int, ctx models.TargetingContext,
	trace *logic.SelectionTrace, cfg config.Config) (*models.AdResponse, error) {
	return s.performSelection(store, database, dataStore, placementID, userID, width, height, ctx, trace, cfg)
}

// performSelection contains the core selection logic used by SelectAd and SelectAdWithTrace.
func (s *RuleBasedSelector) performSelection(store *db.RedisStore, database *db.DB, dataStore models.AdDataStore, placementID, userID string,
	width, height int, ctx models.TargetingContext, trace *logic.SelectionTrace, cfg config.Config) (*models.AdResponse, error) {
	// Resolve placement and dimensions
	placement, ok := database.GetPlacement(placementID)
	if !ok {
		return nil, ErrUnknownPlacement
	}

	if width == 0 {
		width = placement.Width
	}
	if height == 0 {
		height = placement.Height
	}

	// Initial creative pool for this placement
	creatives := database.FindCreativesForPlacement(placementID)
	initialCount := len(creatives)
	creativeCountBucket := observability.GetCreativeCountBucket(initialCount)

	if trace != nil {
		trace.AddStep("start", creatives)
	}

	// Apply optimized single-pass filtering
	filterStart := time.Now()
	spFilter := filters.NewSinglePassFilter(store, dataStore, cfg)
	var err error
	if trace != nil {
		creatives, err = spFilter.FilterCreativesWithTrace(
			context.Background(),
			creatives,
			ctx,
			width,
			height,
			placement.Formats,
			userID,
			trace,
		)
	} else {
		creatives, err = spFilter.FilterCreatives(
			context.Background(),
			creatives,
			ctx,
			width,
			height,
			placement.Formats,
			userID,
		)
	}

	// Record filter performance metrics
	filterDuration := time.Since(filterStart).Seconds()
	result := "success"
	if err != nil {
		if err == filters.ErrPacingLimitReached {
			result = "pacing_limit"
			observability.FilterDuration.WithLabelValues(creativeCountBucket, result).Observe(filterDuration)
			return nil, ErrPacingLimitReached
		}
		result = "error"
		observability.FilterDuration.WithLabelValues(creativeCountBucket, result).Observe(filterDuration)
		return nil, err
	} else if len(creatives) == 0 {
		result = "no_eligible"
	}

	observability.FilterDuration.WithLabelValues(creativeCountBucket, result).Observe(filterDuration)
	observability.FilterStageCount.WithLabelValues("filtered").Set(float64(len(creatives)))

	// Apply rate limiting
	creatives = s.applyRateLimit(creatives, dataStore, trace)

	// Gather programmatic bids
	bids := s.fetchProgrammaticBids(creatives, width, height)

	// Drop creatives that received no bid
	creatives = s.filterCreativesByBid(creatives, bids)

	if len(creatives) == 0 {
		return nil, ErrNoEligibleAd
	}

	// Rank creatives by priority and eCPM
	creatives = s.rankCreatives(creatives, ctx, bids, trace)

	// Return the highest ranked creative
	return s.buildAdResponse(creatives[0], ctx, bids), nil
}

// applyRateLimit removes creatives that exceed the line item rate limit. It returns the
// filtered slice and records the step if tracing is enabled.
func (s *RuleBasedSelector) applyRateLimit(creatives []models.Creative, dataStore models.AdDataStore, trace *logic.SelectionTrace) []models.Creative {
	if s.rateLimiter == nil {
		return creatives
	}

	var allowed []models.Creative
	for _, c := range creatives {
		li := c.LineItem
		if li != nil && s.rateLimiter.ShouldRateLimit(li) {
			lineItemIDStr := fmt.Sprintf("%d", li.ID)
			if !s.rateLimiter.Allow(lineItemIDStr) {
				continue // Skip due to rate limiting
			}
		}
		allowed = append(allowed, c)
	}
	if trace != nil {
		trace.AddStep("ratelimit", allowed)
	}
	return allowed
}

// fetchProgrammaticBids requests bids for all programmatic line items in the given
// creative set. The returned map is keyed by line item ID.
func (s *RuleBasedSelector) fetchProgrammaticBids(creatives []models.Creative, width, height int) map[int]bid {
	bids := make(map[int]bid)

	type liInfo struct {
		id  int
		url string
	}

	var items []liInfo
	for _, c := range creatives {
		li := c.LineItem
		if li != nil && li.Type == models.LineItemTypeProgrammatic && li.Endpoint != "" {
			if _, ok := bids[li.ID]; !ok {
				bids[li.ID] = bid{}
				items = append(items, liInfo{id: li.ID, url: li.Endpoint})
			}
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, it := range items {
		wg.Add(1)
		go func(id int, url string) {
			defer wg.Done()
			timeout := s.programmaticBidTimeout
			if timeout == 0 {
				timeout = defaultProgrammaticBidTimeout
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			bPrice, adm, err := fetchProgrammaticBid(ctx, url, width, height)
			mu.Lock()
			if err == nil {
				bids[id] = bid{Price: bPrice, Adm: adm}
			} else {
				bids[id] = bid{}
			}
			mu.Unlock()
		}(it.id, it.url)
	}
	wg.Wait()

	return bids
}

// filterCreativesByBid removes programmatic creatives that didn't receive a bid.
func (s *RuleBasedSelector) filterCreativesByBid(creatives []models.Creative, bids map[int]bid) []models.Creative {
	var filtered []models.Creative
	for _, c := range creatives {
		li := c.LineItem
		if li != nil && li.Type == models.LineItemTypeProgrammatic && li.Endpoint != "" {
			if bids[li.ID].Price == 0 {
				continue
			}
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// rankCreatives groups creatives by priority, shuffles each bucket and sorts them by
// optimized eCPM. Buckets are shuffled first so that the subsequent stable sort
// preserves a random order among creatives with identical eCPMs. The resulting
// slice is returned in ranked order.
func (s *RuleBasedSelector) rankCreatives(creatives []models.Creative, ctx models.TargetingContext,
	bids map[int]bid, trace *logic.SelectionTrace) []models.Creative {
	creativesByPriority := make(map[string][]models.Creative)
	// calculateOptimizedECPM may call the CTR prediction service. If invoked
	// inside the sort comparator it would be executed O(N log N) times, adding
	// latency and cost. Cache the per-line-item price so each line item is
	// evaluated at most once per request.
	priceCache := make(map[int]float64)
	for _, c := range creatives {
		li := c.LineItem
		priority := models.PriorityMedium
		if li != nil {
			if li.Priority != "" {
				priority = li.Priority
			}
			if _, ok := priceCache[li.ID]; !ok {
				priceCache[li.ID] = s.calculateOptimizedECPM(li, ctx, bids)
			}
		}
		creativesByPriority[priority] = append(creativesByPriority[priority], c)
	}

	var priorities []string
	for p := range creativesByPriority {
		priorities = append(priorities, p)
	}
	sort.SliceStable(priorities, func(i, j int) bool {
		return models.PriorityRank(priorities[i]) < models.PriorityRank(priorities[j])
	})

	creatives = creatives[:0]
	for _, p := range priorities {
		bucket := creativesByPriority[p]
		// Randomize the bucket so equal eCPM creatives get a random order.
		ShuffleFn(bucket)
		// Stable sort by eCPM descending; shuffle above makes price only a
		// tie-breaker when eCPMs match.
		sort.SliceStable(bucket, func(i, j int) bool {
			liA := bucket[i].LineItem
			liB := bucket[j].LineItem
			if liA == nil || liB == nil {
				return false
			}

			// Use the cached prices rather than recomputing.
			priceA := priceCache[liA.ID]
			priceB := priceCache[liB.ID]

			return priceA > priceB
		})
		creatives = append(creatives, bucket...)
	}

	if trace != nil {
		trace.AddStep("rank", creatives)
	}

	return creatives
}

// buildAdResponse constructs the final AdResponse using the ranked creative and any
// programmatic bid information.
func (s *RuleBasedSelector) buildAdResponse(c models.Creative, ctx models.TargetingContext,
	bids map[int]bid) *models.AdResponse {
	li := c.LineItem
	price := 0.0
	html := c.HTML
	if li != nil {
		price = s.calculateOptimizedECPM(li, ctx, bids)
		if li.Type == models.LineItemTypeProgrammatic {
			if b, ok := bids[li.ID]; ok && b.Price > 0 {
				price = b.Price
				if b.Adm != "" {
					html = b.Adm
				}
			}
		}
	}

	// Compose banner HTML server-side if this is a banner creative
	if len(c.Banner) > 0 {
		html = render.ComposeBannerHTML(c.Banner)
	}

	return &models.AdResponse{
		CreativeID: c.ID,
		HTML:       html,
		Native:     c.Native,
		Banner:     nil, // Don't send banner JSON to client - we composed HTML instead
		CampaignID: c.CampaignID,
		LineItemID: c.LineItemID,
		Price:      price,
	}
}
