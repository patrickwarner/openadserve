package macros

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/models"
)

// Service provides macro expansion capabilities for click URLs
type Service struct {
	expander *MacroExpander
	logger   *zap.Logger
}

// NewService creates a new macro expansion service
func NewService(logger *zap.Logger) *Service {
	return &Service{
		expander: NewMacroExpander(logger),
		logger:   logger.Named("macro_service"),
	}
}

// NewServiceForTesting creates a new macro expansion service for testing with isolated metrics
func NewServiceForTesting(logger *zap.Logger) *Service {
	return &Service{
		expander: NewMacroExpanderForTesting(logger, false),
		logger:   logger.Named("macro_service"),
	}
}

// RegisterCustomMacro allows registration of additional macro expansion functions
func (s *Service) RegisterCustomMacro(name string, expansionFunc ExpansionFunc) error {
	return s.expander.RegisterMacro(name, expansionFunc)
}

// GetRegisteredMacros returns a list of all registered macro names
func (s *Service) GetRegisteredMacros() []string {
	return s.expander.GetRegisteredMacros()
}

// ValidateURL validates that a URL contains only supported macros
func (s *Service) ValidateURL(rawURL string) []string {
	return s.expander.ValidateURL(rawURL)
}

// ExpandClickURL expands macros in a click URL using ad request context
func (s *Service) ExpandClickURL(rawURL string, req *ClickContext) (string, error) {
	if rawURL == "" {
		return "", nil
	}

	// Create expansion context
	ctx := &ExpansionContext{
		RequestID:    req.RequestID,
		ImpressionID: req.ImpressionID,
		Timestamp:    req.Timestamp,
		CreativeID:   req.CreativeID,
		LineItemID:   req.LineItemID,
		CampaignID:   req.CampaignID,
		PublisherID:  req.PublisherID,
		PlacementID:  req.PlacementID,
		CustomParams: req.CustomParams,
	}

	// The expander handles all macro types, including custom parameters
	return s.expander.ExpandURL(rawURL, ctx)
}

// GetDestinationURL determines the final destination URL for a click
// Priority: Creative.ClickURL > LineItem.ClickURL > empty string
func (s *Service) GetDestinationURL(ctx context.Context, creative *models.Creative, clickCtx *ClickContext) (string, error) {
	var rawURL string

	// Priority 1: Creative-level click URL
	if creative.ClickURL != "" {
		rawURL = creative.ClickURL
		s.logger.Debug("Using creative-level click URL",
			zap.Int("creative_id", creative.ID),
			zap.String("url", rawURL))
	} else if creative.LineItem != nil && creative.LineItem.ClickURL != "" {
		// Priority 2: Line item-level click URL
		rawURL = creative.LineItem.ClickURL
		s.logger.Debug("Using line item-level click URL",
			zap.Int("line_item_id", creative.LineItem.ID),
			zap.String("url", rawURL))
	} else {
		// No click URL configured
		s.logger.Debug("No click URL configured",
			zap.Int("creative_id", creative.ID),
			zap.Int("line_item_id", creative.LineItemID))
		return "", nil
	}

	// Expand macros in the URL
	expandedURL, err := s.ExpandClickURL(rawURL, clickCtx)
	if err != nil {
		s.logger.Error("Failed to expand click URL macros, using original URL",
			zap.String("raw_url", rawURL),
			zap.Error(err))
		// Return the original URL instead of failing completely
		// This ensures clicks still work even if macro expansion fails
		return rawURL, nil
	}

	return expandedURL, nil
}

// ClickContext contains all the context needed for click URL macro expansion
type ClickContext struct {
	// Request identifiers
	RequestID    string
	ImpressionID string
	Timestamp    time.Time

	// Ad identifiers
	CreativeID  int32
	LineItemID  int32
	CampaignID  int32
	PublisherID int32
	PlacementID string

	// Custom parameters from ad request
	CustomParams map[string]string
}

// NewClickContextFromRequest creates a ClickContext from an ad request and selected creative
func NewClickContextFromRequest(
	requestID, impressionID string,
	creative *models.Creative,
	customParams map[string]string,
) *ClickContext {
	ctx := &ClickContext{
		RequestID:    requestID,
		ImpressionID: impressionID,
		Timestamp:    time.Now(),
		CreativeID:   int32(creative.ID),
		LineItemID:   int32(creative.LineItemID),
		CampaignID:   int32(creative.CampaignID),
		PublisherID:  int32(creative.PublisherID),
		PlacementID:  creative.PlacementID,
		CustomParams: customParams,
	}

	return ctx
}
