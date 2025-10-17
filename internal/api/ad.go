package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/patrickwarner/openadserve/internal/config"
	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/middleware"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
	"github.com/patrickwarner/openadserve/internal/token"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("openadserve")

// decodeOpenRTBRequest reads and unmarshals an OpenRTB request body.
func decodeOpenRTBRequest(r *http.Request) (*models.OpenRTBRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	defer func() {
		_ = r.Body.Close()
	}()

	var req models.OpenRTBRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return &req, nil
}

// writeOpenRTBResponse writes the given response as JSON.
func writeOpenRTBResponse(w http.ResponseWriter, resp models.OpenRTBResponse, trace logic.SelectionTrace, debug bool) error {
	out := struct {
		models.OpenRTBResponse
		Debug interface{} `json:"debug,omitempty"`
	}{resp, nil}
	if debug {
		out.Debug = map[string]interface{}{"trace": trace}
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(out)
}

// GetAdHandler handles POST /ad requests in OpenRTB-style format.
func (s *Server) GetAdHandler(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "GetAdHandler",
		trace.WithAttributes(
			attribute.String("http.method", "POST"),
			attribute.String("http.route", "/ad"),
		))
	defer span.End()

	// Get trace-aware logger from middleware
	logger := middleware.LoggerFromRequest(r, s.Logger)

	start := time.Now()
	const endpoint = "ad"
	const method = "POST"

	if s.Analytics == nil {
		logger.Error("analytics unavailable")
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics unavailable", http.StatusInternalServerError)
		return
	}

	req, err := decodeOpenRTBRequest(r)
	if err != nil {
		logger.Error("decode request", zap.Error(err), zap.String("event_type", "ad_request"))
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if len(req.Imp) == 0 || req.User.ID == "" {
		logger.Error("missing required fields", zap.String("event_type", "ad_request"))
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "imp[] and user.id required", http.StatusBadRequest)
		return
	}

	pub := models.GetPublisherByID(s.AdDataStore, req.Ext.PublisherID)
	if pub == nil {
		logger.Error("unknown publisher", zap.Int("publisher_id", req.Ext.PublisherID))
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "unknown publisher", http.StatusBadRequest)
		return
	}

	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" || apiKey != pub.APIKey {
		logger.Error("invalid api key",
			zap.Int("publisher_id", req.Ext.PublisherID),
			zap.String("request_id", req.ID))
		s.Metrics.IncrementRequests(endpoint, method, "401")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	placementID := req.Imp[0].TagID
	userID := req.User.ID
	width := req.Imp[0].W
	height := req.Imp[0].H
	deviceUA := req.Device.UA
	ipStr := r.Header.Get("X-Forwarded-For")
	if ipStr == "" {
		ipStr, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	targetingCtx := logic.ResolveTargeting(s.GeoIP, deviceUA, ipStr)
	if len(req.Ext.KV) > 0 {
		targetingCtx.KeyValues = req.Ext.KV
	}

	// Add request attributes to span
	span.SetAttributes(
		attribute.String("placement_id", placementID),
		attribute.String("user_id", userID),
		attribute.Int("publisher_id", req.Ext.PublisherID),
		attribute.Int("width", width),
		attribute.Int("height", height),
		attribute.String("ip_address", ipStr),
	)

	if err := s.Analytics.RecordEvent(ctx, s.AdDataStore, "ad_request", req.ID, req.Imp[0].ID, "", 0, 0, targetingCtx, req.Ext.PublisherID); err != nil {
		logger.Error("analytics record", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics error", http.StatusInternalServerError)
		return
	}
	if observability.ShouldSample(observability.GetSamplingRate()) {
		logger.Info("ad request", zap.String("request_id", req.ID), zap.String("user_id", userID), zap.String("event_type", "ad_request"))
	}
	s.Metrics.IncrementEvent("ad_request")

	debugEnabled := s.DebugTrace || r.URL.Query().Get("debug") == "1"
	var trace logic.SelectionTrace

	selector, ok := s.SelectorMap[req.Ext.PublisherID]
	if !ok {
		selector = s.SelectorMap[0]
	}

	var ad *models.AdResponse
	if debugEnabled {
		if ts, ok := selector.(interface {
			SelectAdWithTrace(*db.RedisStore, *db.DB, models.AdDataStore, string, string, int, int, models.TargetingContext, *logic.SelectionTrace, config.Config) (*models.AdResponse, error)
		}); ok {
			ad, err = ts.SelectAdWithTrace(s.Store, s.DB, s.AdDataStore, placementID, userID, width, height, targetingCtx, &trace, s.Config)
		} else {
			ad, err = selector.SelectAd(s.Store, s.DB, s.AdDataStore, placementID, userID, width, height, targetingCtx, s.Config)
		}
	} else {
		ad, err = selector.SelectAd(s.Store, s.DB, s.AdDataStore, placementID, userID, width, height, targetingCtx, s.Config)
	}
	if err != nil {
		// no-bid path
		span.SetAttributes(attribute.String("ad.result", "no_bid"))
		if err := s.Analytics.RecordEvent(ctx, s.AdDataStore, "no_ad", req.ID, req.Imp[0].ID, "", 0, 0, targetingCtx, req.Ext.PublisherID); err != nil {
			logger.Error("analytics record", zap.Error(err))
			s.Metrics.IncrementRequests(endpoint, method, "500")
			s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
			http.Error(w, "analytics error", http.StatusInternalServerError)
			return
		}
		if observability.ShouldSample(observability.GetSamplingRate()) {
			logger.Info("no ad", zap.String("request_id", req.ID), zap.String("user_id", userID), zap.String("event_type", "no_ad"))
		}
		s.Metrics.IncrementEvent("no_ad")
		s.Metrics.IncrementNoBids()
		s.Metrics.IncrementRequests(endpoint, method, "200")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))

		resp := models.OpenRTBResponse{
			ID:      req.ID,
			SeatBid: []models.SeatBid{},
			Nbr:     1,
		}
		if err := writeOpenRTBResponse(w, resp, trace, debugEnabled); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// successful bid - add span attributes
	span.SetAttributes(
		attribute.String("ad.result", "bid"),
		attribute.Int("ad.line_item_id", ad.LineItemID),
		attribute.Int("ad.campaign_id", ad.CampaignID),
		attribute.Int("ad.creative_id", ad.CreativeID),
		attribute.Float64("ad.price", ad.Price),
	)

	// increment serve counter immediately for pacing
	if err := logic.IncrementLineItemServes(s.Store, ad.LineItemID); err != nil {
		logger.Error("failed to increment serve counter", zap.Error(err), zap.Int("line_item_id", ad.LineItemID))
		// Continue serving the ad even if serve counter fails
	}

	adm := ad.HTML
	if len(ad.Native) > 0 {
		adm = string(ad.Native)
	}
	tok, err := token.GenerateWithCustomParams(req.ID, req.Imp[0].ID, fmt.Sprintf("%d", ad.CreativeID), fmt.Sprintf("%d", ad.CampaignID), fmt.Sprintf("%d", ad.LineItemID), userID, fmt.Sprintf("%d", req.Ext.PublisherID), req.Ext.CustomParams, s.TokenSecret)
	if err != nil {
		logger.Error("failed to generate token", zap.Error(err), zap.String("request_id", req.ID))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "internal server error (token generation)", http.StatusInternalServerError)
		return
	}
	impURL := "/impression?t=" + url.QueryEscape(tok)
	clkURL := "/click?t=" + url.QueryEscape(tok)
	evtURL := "/event?t=" + url.QueryEscape(tok)
	repURL := "/report?t=" + url.QueryEscape(tok)
	resp := models.OpenRTBResponse{
		ID: req.ID,
		SeatBid: []models.SeatBid{{
			Bid: []models.Bid{{
				ID:        "1",
				ImpID:     req.Imp[0].ID,
				CrID:      fmt.Sprintf("%d", ad.CreativeID),
				CID:       fmt.Sprintf("%d", ad.CampaignID),
				Adm:       adm,
				Price:     ad.Price,
				ImpURL:    impURL,
				ClickURL:  clkURL,
				EventURL:  evtURL,
				ReportURL: repURL,
			}},
		}},
	}
	if err := s.Analytics.RecordEvent(ctx, s.AdDataStore, "ad_served", req.ID, req.Imp[0].ID, fmt.Sprintf("%d", ad.CreativeID), ad.LineItemID, 0, targetingCtx, req.Ext.PublisherID); err != nil {
		logger.Error("analytics record", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics error", http.StatusInternalServerError)
		return
	}
	if observability.ShouldSample(observability.GetSamplingRate()) {
		logger.Info("ad served",
			zap.String("request_id", req.ID),
			zap.String("user_id", userID),
			zap.String("event_type", "ad_served"))
	}
	s.Metrics.IncrementEvent("ad_served")

	s.Metrics.IncrementRequests(endpoint, method, "200")
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))

	if err := writeOpenRTBResponse(w, resp, trace, debugEnabled); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
