package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/patrickwarner/openadserve/internal/reporting"
	"go.uber.org/zap"
)

// CampaignReportHandler handles GET /api/campaigns/{id}/report requests.
// Generates a comprehensive performance report for a specific campaign including
// metrics, daily breakdowns, creative performance, and line item analysis.
//
// Query Parameters:
//   - days: Number of days to include in the report (default: 7, max: 365)
//
// Response: JSON containing CampaignSummary with all metrics and breakdowns
func (s *Server) CampaignReportHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	endpoint := "/api/campaigns/{id}/report"
	method := r.Method

	// Only allow GET
	if r.Method != http.MethodGet {
		s.Metrics.IncrementRequests(endpoint, method, "405")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if ClickHouse is available
	if s.ClickHouseDB == nil {
		s.Logger.Error("clickhouse unavailable")
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "analytics database unavailable", http.StatusInternalServerError)
		return
	}

	// Extract campaign ID from URL path
	vars := mux.Vars(r)
	campaignIDStr, ok := vars["id"]
	if !ok {
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "campaign_id is required", http.StatusBadRequest)
		return
	}

	campaignID, err := strconv.Atoi(campaignIDStr)
	if err != nil || campaignID <= 0 {
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "invalid campaign_id", http.StatusBadRequest)
		return
	}

	// Parse optional days query parameter (default: 7, max: 365)
	days := 7
	if daysParam := r.URL.Query().Get("days"); daysParam != "" {
		parsedDays, err := strconv.Atoi(daysParam)
		if err != nil || parsedDays <= 0 {
			s.Metrics.IncrementRequests(endpoint, method, "400")
			s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
			http.Error(w, "invalid days parameter", http.StatusBadRequest)
			return
		}
		if parsedDays > 365 {
			parsedDays = 365 // Cap at 365 days
		}
		days = parsedDays
	}

	// Verify campaign exists
	campaign := s.AdDataStore.GetCampaign(campaignID)
	if campaign == nil {
		s.Metrics.IncrementRequests(endpoint, method, "404")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "campaign not found", http.StatusNotFound)
		return
	}

	// Generate campaign report
	summary, err := reporting.GenerateCampaignReport(r.Context(), s.ClickHouseDB, campaignID, days)
	if err != nil {
		s.Logger.Error("failed to generate campaign report",
			zap.Int("campaign_id", campaignID),
			zap.Int("days", days),
			zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "failed to generate report", http.StatusInternalServerError)
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		s.Logger.Error("failed to encode campaign report response",
			zap.Int("campaign_id", campaignID),
			zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		return
	}

	s.Logger.Info("campaign report generated",
		zap.Int("campaign_id", campaignID),
		zap.Int("days", days),
		zap.Int64("impressions", summary.TotalMetrics.Impressions),
		zap.Int64("clicks", summary.TotalMetrics.Clicks),
		zap.Float64("spend", summary.TotalMetrics.Spend))

	s.Metrics.IncrementRequests(endpoint, method, "200")
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
}
