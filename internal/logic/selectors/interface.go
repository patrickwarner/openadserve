package selectors

import (
	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/models"
)

// Selector defines a pluggable interface for ad selection.
type Selector interface {
	SelectAd(store *db.RedisStore, database *db.DB, dataStore models.AdDataStore,
		placementID, userID string,
		width, height int,
		ctx models.TargetingContext, cfg config.Config) (*models.AdResponse, error)
}
