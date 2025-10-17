package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/token"

	"go.uber.org/zap"
)

// CustomEvent describes a publisher-defined event for analytics.
// CustomEvent kept for backwards compatibility of tests.
type CustomEvent struct{}

// AllowedEventTypes lists the custom event names accepted by the server.
// Publishers should document the behaviour of each event so its
// meaning is clear when referenced in pacing or reporting logic.
var AllowedEventTypes = map[string]struct{}{
	"like":    {}, // user expressed a like/heart reaction
	"share":   {}, // user shared the content
	"comment": {}, // user commented on the content
}

// EventHandler handles GET /event pixel requests.
func (s *Server) EventHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	const endpoint = "event"
	const method = "GET"

	if s.Analytics == nil {
		s.Logger.Error("analytics unavailable")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics unavailable", http.StatusInternalServerError)
		return
	}

	tok := r.URL.Query().Get("t")
	if tok == "" {
		s.Logger.Warn("missing token")
		s.Metrics.IncrementEvent("bad_event")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "token required", http.StatusUnauthorized)
		return
	}
	payload, err := token.Verify(tok, s.TokenSecret, s.TokenTTL)
	if err != nil {
		s.Logger.Warn("token verify", zap.Error(err))
		s.Metrics.IncrementEvent("bad_event")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	evType := r.URL.Query().Get("type")
	if evType == "" {
		s.Logger.Error("missing event type")
		s.Metrics.IncrementEvent("bad_event")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "type required", http.StatusBadRequest)
		return
	}

	if _, ok := AllowedEventTypes[evType]; !ok {
		s.Logger.Error("unknown event type", zap.String("type", evType))
		s.Metrics.IncrementEvent("bad_event")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "unknown event type", http.StatusBadRequest)
		return
	}

	var pubID int
	var lineItemID int
	if id, err := strconv.Atoi(payload.CrID); err == nil {
		if cr := s.DB.FindCreativeByID(id); cr != nil {
			pubID = cr.PublisherID
			lineItemID = cr.LineItemID
			_ = s.Store.IncrementCustomEvent(cr.LineItemID, evType)
		}
	}

	// Get LineItemID from token if available (newer tokens)
	if payload.LIID != "" {
		if id, err := strconv.Atoi(payload.LIID); err == nil {
			lineItemID = id
		}
	}

	if pubID == 0 || models.GetPublisherByID(s.AdDataStore, pubID) == nil {
		s.Logger.Error("unknown publisher", zap.Int("publisher_id", pubID))
		s.Metrics.IncrementEvent("bad_event")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "unknown publisher", http.StatusBadRequest)
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

	// Create targeting context for event
	targetingCtx := models.TargetingContext{
		DeviceType: deviceType,
		Country:    country,
		KeyValues:  make(map[string]string), // Events don't have key-values
	}

	if err := s.Analytics.RecordEvent(r.Context(), s.AdDataStore, evType, payload.RequestID, payload.ImpID, payload.CrID, lineItemID, 0, targetingCtx, publisherID); err != nil {
		s.Logger.Error("analytics record", zap.Error(err))
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics error", http.StatusInternalServerError)
		return
	}
	s.Logger.Info("custom event", zap.String("request_id", payload.RequestID), zap.String("event_type", evType))
	s.Metrics.IncrementEvent(evType)
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
	w.Header().Set("Content-Type", "image/gif")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pixelGIF)
}
