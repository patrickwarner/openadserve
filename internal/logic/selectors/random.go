package selectors

import (
	"math/rand"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	filters "github.com/patrickwarner/openadserve/internal/logic/filters"
	"github.com/patrickwarner/openadserve/internal/logic/render"
	"github.com/patrickwarner/openadserve/internal/models"
)

// RandomSelector is a very simple example implementation that ignores
// targeting and frequency rules and returns a random creative from the
// requested placement.
type RandomSelector struct{}

// SelectAd picks a random creative for the placement.
func (RandomSelector) SelectAd(store *db.RedisStore, database *db.DB, dataStore models.AdDataStore,
	placementID, userID string, width, height int,
	ctx models.TargetingContext, cfg config.Config) (*models.AdResponse, error) {
	placement, ok := database.GetPlacement(placementID)
	if !ok {
		return nil, ErrUnknownPlacement
	}
	creatives := database.FindCreativesForPlacement(placementID)

	creatives = filters.FilterByTargeting(creatives, ctx, dataStore)
	creatives = filters.FilterBySize(creatives, width, height, placement.Formats)
	var err error
	creatives, err = filters.FilterByFrequency(store, creatives, userID, dataStore)
	if err != nil {
		return nil, err
	}
	creatives, err = filters.FilterByPacing(store, creatives, dataStore, cfg)
	if err == filters.ErrPacingLimitReached {
		return nil, ErrPacingLimitReached
	}
	if len(creatives) == 0 {
		return nil, ErrNoEligibleAd
	}
	c := creatives[rand.Intn(len(creatives))]
	li := c.LineItem
	price := 0.0
	if li != nil {
		price = li.ECPM
	}

	// Compose banner HTML server-side if this is a banner creative
	html := c.HTML
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
	}, nil
}
