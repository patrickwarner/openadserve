package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/middleware"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/patrickwarner/openadserve/internal/token"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// OpenRTBImpression kept for backwards compatibility of tests.
type OpenRTBImpression struct{}

// ImpressionHandler handles GET /impression pixel requests.
func (s *Server) ImpressionHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "ImpressionHandler",
		trace.WithAttributes(
			attribute.String("http.method", "GET"),
			attribute.String("http.route", "/impression"),
		))
	defer span.End()

	// Get trace-aware logger from middleware
	logger := middleware.LoggerFromRequest(r, s.Logger)

	start := time.Now()
	const endpoint = "impression"
	const method = "GET"

	if s.Analytics == nil {
		span.RecordError(fmt.Errorf("analytics unavailable"))
		span.SetStatus(codes.Error, "analytics unavailable")
		logger.Error("analytics unavailable")
		s.Metrics.IncrementImpressions("500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics unavailable", http.StatusInternalServerError)
		return
	}

	tok := r.URL.Query().Get("t")
	if tok == "" {
		logger.Warn("missing token")
		s.Metrics.IncrementImpressions("401")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	payload, err := token.Verify(tok, s.TokenSecret, s.TokenTTL)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid token")
		logger.Warn("token verify", zap.Error(err))
		s.Metrics.IncrementImpressions("401")
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

	if observability.ShouldSample(observability.GetSamplingRate()) {
		logger.Info("impression", zap.String("request_id", payload.RequestID), zap.String("user_id", ""), zap.String("event_type", "impression"))
	}

	var pubID int
	var lineItemID int
	if id, err := strconv.Atoi(payload.CrID); err == nil {
		if cr := s.DB.FindCreativeByID(id); cr != nil {
			pubID = cr.PublisherID
			lineItemID = cr.LineItemID
			_ = s.Store.IncrementCTRImpression(cr.LineItemID)
		}
	}

	// Get LineItemID from token if available (newer tokens)
	if payload.LIID != "" {
		if id, err := strconv.Atoi(payload.LIID); err == nil {
			lineItemID = id
		}
	}

	if pubID == 0 || models.GetPublisherByID(s.AdDataStore, pubID) == nil {
		logger.Error("unknown publisher", zap.Int("publisher_id", pubID))
		s.Metrics.IncrementImpressions("400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "unknown publisher", http.StatusBadRequest)
		return
	}

	// Increment impression counter for billing
	if lineItemID > 0 {
		if err := logic.IncrementLineItemImpressions(s.Store, lineItemID); err != nil {
			logger.Error("failed to increment impression counter", zap.Error(err), zap.Int("line_item_id", lineItemID))
			// Don't fail the request - impression has already been recorded
		}
	}

	// Increment frequency cap counter for impression
	if lineItemID > 0 && payload.UserID != "" {
		if err := logic.IncrementFrequencyCap(s.Store, payload.UserID, pubID, lineItemID, s.AdDataStore); err != nil {
			logger.Error("failed to increment frequency cap counter", zap.Error(err), zap.Int("line_item_id", lineItemID))
			// Don't fail the request - impression has already been recorded
		}
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

	// Record the impression in ClickHouse for analytics.
	if err := s.Analytics.RecordImpression(ctx, s.AdDataStore, payload.RequestID, payload.ImpID, payload.CrID, lineItemID, deviceType, country, publisherID, payload.PlacementID); err != nil {
		logger.Error("analytics record", zap.Error(err))
		s.Metrics.IncrementImpressions("500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics error", http.StatusInternalServerError)
		return
	}
	s.Metrics.IncrementEvent("impression")

	s.Metrics.IncrementImpressions("200")
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
	w.Header().Set("Content-Type", "image/gif")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pixelGIF)
}
