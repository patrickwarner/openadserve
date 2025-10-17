package api

import (
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/patrickwarner/openadserve/internal/forecasting"
	"github.com/patrickwarner/openadserve/internal/models"
	"github.com/patrickwarner/openadserve/internal/observability"
)

// ForecastHandler handles forecast requests
func (s *Server) ForecastHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	endpoint := "/forecast"
	method := r.Method

	// Only allow POST
	if r.Method != http.MethodPost {
		s.Metrics.IncrementRequests(endpoint, method, "405")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req models.ForecastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.Logger.Error("invalid forecast request", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.PublisherID <= 0 {
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "publisher_id is required", http.StatusBadRequest)
		return
	}

	// Validate date fields
	if req.StartDate.IsZero() || req.EndDate.IsZero() {
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "start_date and end_date are required", http.StatusBadRequest)
		return
	}

	if req.EndDate.Before(req.StartDate) {
		s.Metrics.IncrementRequests(endpoint, method, "400")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "end_date must be after start_date", http.StatusBadRequest)
		return
	}

	// Log forecast request (sampled)
	if observability.ShouldSample(observability.GetSamplingRate()) {
		s.Logger.Info("forecast request",
			zap.Int("publisher_id", req.PublisherID),
			zap.String("budget_type", req.BudgetType),
			zap.Float64("budget", req.Budget),
			zap.Time("start_date", req.StartDate),
			zap.Time("end_date", req.EndDate),
		)
	}

	// Create forecasting engine if not already initialized
	if s.ForecastEngine == nil {
		// Initialize forecasting engine with dependencies
		s.ForecastEngine = forecasting.NewEngine(
			s.ClickHouseDB, // ClickHouse connection
			nil,            // Redis client (can be nil for POC)
			s.AdDataStore,  // Ad data store
			s.Logger,       // Logger
		)
	}

	// Execute forecast
	response, err := s.ForecastEngine.Forecast(r.Context(), &req)
	if err != nil {
		s.Logger.Error("forecast failed", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "Forecast failed", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.Logger.Error("encode forecast response", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		return
	}

	// Success metrics
	s.Metrics.IncrementRequests(endpoint, method, "200")
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))

	if observability.ShouldSample(observability.GetSamplingRate()) {
		s.Logger.Info("forecast completed",
			zap.Int("publisher_id", req.PublisherID),
			zap.Int64("estimated_impressions", response.EstimatedImpressions),
			zap.Int("conflicts", len(response.Conflicts)),
			zap.Duration("duration", time.Since(start)),
		)
	}
}
