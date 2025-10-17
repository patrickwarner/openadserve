package api

import (
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ReloadHandler reloads campaigns, line items and creatives from Postgres.
func (s *Server) ReloadHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	const endpoint = "reload"
	const method = "POST"

	if err := s.Reload(); err != nil {
		s.Logger.Error("reload failed", zap.Error(err))
		s.Metrics.IncrementRequests(endpoint, method, "500")
		s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
		http.Error(w, "reload failed", http.StatusInternalServerError)
		return
	}

	s.Metrics.IncrementRequests(endpoint, method, "204")
	s.Metrics.RecordRequestLatency(endpoint, method, time.Since(start))
	w.WriteHeader(http.StatusNoContent)
}
