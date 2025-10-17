package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/patrickwarner/openadserve/internal/db"
	"github.com/patrickwarner/openadserve/internal/logic"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/token"

	"go.uber.org/zap"
)

// ReportRequest is the payload for submitting an ad report.
type ReportRequest struct {
	Token  string `json:"token"`
	Reason string `json:"reason"`
}

// ReportHandler handles POST /report requests.
func (s *Server) ReportHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	const endpoint = "report"
	const method = "POST"

	if s.PG == nil {
		s.Logger.Error("postgres unavailable")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "db unavailable", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			s.Logger.Warn("failed to close request body", zap.Error(closeErr))
		}
	}()

	var req ReportRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Token == "" || req.Reason == "" {
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "token and reason required", http.StatusBadRequest)
		return
	}

	pl, err := token.Verify(req.Token, s.TokenSecret, s.TokenTTL)
	if err != nil {
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Extract IP address without port number
	ipAddr := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ipAddr); err == nil {
		ipAddr = host
	}

	// Get placement ID from the creative
	creativeID := atoi(pl.CrID)
	placementID := ""
	if cr := s.DB.FindCreativeByID(creativeID); cr != nil {
		placementID = cr.PlacementID
	}

	report := models.AdReport{
		CreativeID:   creativeID,
		LineItemID:   atoi(pl.LIID),
		CampaignID:   atoi(pl.CID),
		PublisherID:  publisherFromCreative(s.DB, pl.CrID),
		UserID:       pl.UserID,
		PlacementID:  placementID,
		ReportReason: req.Reason,
		IPAddress:    ipAddr,
		UserAgent:    r.UserAgent(),
		Status:       "pending",
	}

	if err := s.PG.InsertAdReport(report); err != nil {
		s.Logger.Error("insert report", zap.Error(err))
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if s.Analytics != nil {
		publisherID := publisherFromCreative(s.DB, pl.CrID)
		if pl.PubID != "" {
			if id, err := strconv.Atoi(pl.PubID); err == nil {
				publisherID = id
			}
		}

		// Resolve device type and country from request headers/IP
		deviceType, country := logic.ResolveTargetingFromRequest(r, s.GeoIP)

		// Create targeting context for report
		targetingCtx := models.TargetingContext{
			DeviceType: deviceType,
			Country:    country,
			KeyValues:  make(map[string]string), // Reports don't have key-values
		}

		_ = s.Analytics.RecordEvent(r.Context(), s.AdDataStore, "ad_report", pl.RequestID, pl.ImpID, pl.CrID, atoi(pl.LIID), 0, targetingCtx, publisherID)
	}
	s.Metrics.IncrementEvent("ad_report")
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
	w.WriteHeader(http.StatusCreated)
}

func atoi(s string) int {
	if s == "" {
		return 0
	}
	var i int
	_, _ = fmt.Sscanf(s, "%d", &i)
	return i
}

func publisherFromCreative(db *db.DB, crID string) int {
	if id, err := strconv.Atoi(crID); err == nil {
		if cr := db.FindCreativeByID(id); cr != nil {
			return cr.PublisherID
		}
	}
	return 0
}
