package api

import (
	"net/http"
	"time"
)

// HealthHandler responds with a simple status check.
func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	const endpoint = "health"
	const method = "GET"

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))

	s.Metrics.IncrementRequests(endpoint, method, "200")
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
}
