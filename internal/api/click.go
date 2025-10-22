package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/macros"
	"github.com/patrickwarner/openadserve/internal/middleware"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/patrickwarner/openadserve/internal/token"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// OpenRTBClick mirrors OpenRTBImpression for click events.
// OpenRTBClick kept for backwards compatibility of tests.
type OpenRTBClick struct{}

// ClickHandler handles GET /click pixel requests and redirects to destination URLs.
func (s *Server) ClickHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "ClickHandler",
		trace.WithAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.route", "/click"),
		))
	defer span.End()

	// Get trace-aware logger from middleware
	logger := middleware.LoggerFromRequest(r, s.Logger)

	start := time.Now()
	const endpoint = "/click"
	const method = "GET"

	if s.Analytics == nil {
		span.RecordError(fmt.Errorf("analytics unavailable"))
		span.SetStatus(codes.Error, "analytics unavailable")
		logger.Error("analytics unavailable")
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics unavailable", http.StatusInternalServerError)
		return
	}

	tok := r.URL.Query().Get("t")
	if tok == "" {
		logger.Warn("missing token")
		s.Metrics.IncrementRequests(endpoint, method, "401")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	payload, err := token.Verify(tok, s.TokenSecret, s.TokenTTL)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid token")
		logger.Warn("token verify", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "401")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Add token payload attributes to span
	span.SetAttributes(
		attribute.String("request_id", payload.RequestID),
		attribute.String("impression_id", payload.ImpID),
		attribute.String("creative_id", payload.CrID),
		attribute.String("campaign_id", payload.CID),
		attribute.String("line_item_id", payload.LIID),
	)

	var pubID int
	var lineItemID int
	var creative *models.Creative

	// Single lookup with validation - fixes N+1 query pattern
	if id, err := strconv.Atoi(payload.CrID); err == nil {
		if cr := s.DB.FindCreativeByID(id); cr != nil {
			// Validate publisher exists during creative lookup to avoid N+1 pattern
			if models.GetPublisherByID(s.AdDataStore, cr.PublisherID) == nil {
				logger.Error("creative references unknown publisher",
					zap.Int("creative_id", id),
					zap.Int("publisher_id", cr.PublisherID))
				s.Metrics.IncrementRequests(endpoint, method, "400")
				s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
				http.Error(w, "unknown publisher", http.StatusBadRequest)
				return
			}

			creative = cr
			pubID = cr.PublisherID
			lineItemID = cr.LineItemID
			_ = s.Store.IncrementClick(cr.LineItemID)
			_ = s.Store.IncrementCTRClick(cr.LineItemID)
		}
	}

	// Get LineItemID from token if available (newer tokens)
	if payload.LIID != "" {
		if id, err := strconv.Atoi(payload.LIID); err == nil {
			lineItemID = id
		}
	}

	// Early exit if no creative found
	if creative == nil {
		logger.Error("creative not found", zap.String("creative_id", payload.CrID))
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "creative not found", http.StatusBadRequest)
		return
	}

	// Get publisher ID from token or fallback to creative lookup
	publisherID := pubID
	if payload.PubID != "" {
		if id, err := strconv.Atoi(payload.PubID); err == nil {
			publisherID = id
		}
	}

	// Resolve device type and country from request headers/IP
	deviceType, country := logic.ResolveTargetingFromRequest(r, s.GeoIP)

	// Record click analytics
	if err := s.Analytics.RecordClick(ctx, s.AdDataStore, payload.RequestID, payload.ImpID, payload.CrID, lineItemID, deviceType, country, publisherID, payload.PlacementID); err != nil {
		logger.Error("analytics record", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics error", http.StatusInternalServerError)
		return
	}

	if observability.ShouldSample(observability.GetSamplingRate()) {
		logger.Info("click", zap.String("request_id", payload.RequestID), zap.String("user_id", ""), zap.String("event_type", "click"))
	}
	s.Metrics.IncrementEvent("click")

	// Determine destination URL and handle redirect or pixel response
	destinationURL := ""
	if creative != nil && s.MacroService != nil {
		// Use custom parameters from token (passed in ad request ext field)
		customParams := payload.CustomParams
		if customParams == nil {
			customParams = make(map[string]string)
		}

		// Create click context for macro expansion
		clickCtx := macros.NewClickContextFromRequest(
			payload.RequestID,
			payload.ImpID,
			creative,
			customParams,
		)

		// Get expanded destination URL
		if expandedURL, err := s.MacroService.GetDestinationURL(ctx, creative, clickCtx); err != nil {
			logger.Error("Failed to expand destination URL",
				zap.Int("creative_id", creative.ID),
				zap.Error(err))
		} else {
			destinationURL = expandedURL
		}
	}

	// If we have a destination URL, redirect to it
	if destinationURL != "" {
		// Validate URL before redirecting
		if parsedURL, err := url.Parse(destinationURL); err != nil {
			logger.Error("Invalid destination URL",
				zap.String("url", destinationURL),
				zap.Error(err))
			s.Metrics.IncrementRequests(endpoint, method, "200")
			s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
			s.sendPixelResponse(w)
		} else if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			logger.Warn("Unsafe destination URL scheme",
				zap.String("url", destinationURL),
				zap.String("scheme", parsedURL.Scheme))
			s.Metrics.IncrementRequests(endpoint, method, "200")
			s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
			s.sendPixelResponse(w)
		} else {
			// Safe to redirect
			logger.Debug("Redirecting to destination URL",
				zap.String("url", destinationURL),
				zap.String("request_id", payload.RequestID))
			s.Metrics.IncrementEvent("click_redirect")
			s.Metrics.IncrementRequests(endpoint, method, "302")
			s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
			http.Redirect(w, r, destinationURL, http.StatusFound)
		}
	} else {
		// No destination URL configured, return tracking pixel
		s.Metrics.IncrementRequests(endpoint, method, "200")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		s.sendPixelResponse(w)
	}
}

// sendPixelResponse sends a 1x1 tracking pixel response
func (s *Server) sendPixelResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/gif")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pixelGIF)
}
