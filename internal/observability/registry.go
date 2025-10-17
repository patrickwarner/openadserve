package observability

import "time"

// MetricsRegistry provides an interface for recording application metrics
// This replaces direct access to global Prometheus metrics with dependency injection
type MetricsRegistry interface {
	// HTTP Request metrics
	IncrementRequests(endpoint, method, status string)
	RecordRequestLatency(endpoint, method string, duration time.Duration)

	// Bid response metrics
	IncrementNoBids()

	// Event tracking metrics
	IncrementImpressions(status string)
	IncrementEvent(eventType string)

	// Spend tracking metrics
	SetSpendTotal(campaign string, amount float64)
	IncrementSpendPersistErrors()

	// Rate limiting metrics
	IncrementRateLimitRequests(lineItemID string)
	IncrementRateLimitHits(lineItemID string)

	// Report metrics
	IncrementReports()

	// CTR prediction metrics
	IncrementCTRPredictionRequests(outcome string)
	RecordCTRPredictionLatency(duration time.Duration)
	RecordCTRBoostMultiplier(multiplier float64)
}

// PrometheusRegistry implements MetricsRegistry using the existing global Prometheus metrics
type PrometheusRegistry struct{}

// NewPrometheusRegistry creates a new PrometheusRegistry
func NewPrometheusRegistry() *PrometheusRegistry {
	return &PrometheusRegistry{}
}

// HTTP Request metrics
func (r *PrometheusRegistry) IncrementRequests(endpoint, method, status string) {
	RequestCount.WithLabelValues(endpoint, method, status).Inc()
}

func (r *PrometheusRegistry) RecordRequestLatency(endpoint, method string, duration time.Duration) {
	RequestLatency.WithLabelValues(endpoint, method).Observe(duration.Seconds())
}

// Bid response metrics
func (r *PrometheusRegistry) IncrementNoBids() {
	NoBidCount.Inc()
}

// Event tracking metrics
func (r *PrometheusRegistry) IncrementImpressions(status string) {
	ImpressionCount.WithLabelValues(status).Inc()
}

func (r *PrometheusRegistry) IncrementEvent(eventType string) {
	EventCount.WithLabelValues(eventType).Inc()
}

// Spend tracking metrics
func (r *PrometheusRegistry) SetSpendTotal(campaign string, amount float64) {
	SpendTotal.WithLabelValues(campaign).Set(amount)
}

func (r *PrometheusRegistry) IncrementSpendPersistErrors() {
	SpendPersistErrors.Inc()
}

// Rate limiting metrics
func (r *PrometheusRegistry) IncrementRateLimitRequests(lineItemID string) {
	RateLimitRequests.WithLabelValues(lineItemID).Inc()
}

func (r *PrometheusRegistry) IncrementRateLimitHits(lineItemID string) {
	RateLimitHits.WithLabelValues(lineItemID).Inc()
}

// Report metrics
func (r *PrometheusRegistry) IncrementReports() {
	ReportCount.Inc()
}

// CTR prediction metrics
func (r *PrometheusRegistry) IncrementCTRPredictionRequests(outcome string) {
	CTRPredictionRequests.WithLabelValues(outcome).Inc()
}

func (r *PrometheusRegistry) RecordCTRPredictionLatency(duration time.Duration) {
	CTRPredictionLatency.Observe(duration.Seconds())
}

func (r *PrometheusRegistry) RecordCTRBoostMultiplier(multiplier float64) {
	CTRBoostMultiplier.Observe(multiplier)
}

// NoOpRegistry implements MetricsRegistry with no-op methods for testing
type NoOpRegistry struct{}

// NewNoOpRegistry creates a new NoOpRegistry
func NewNoOpRegistry() *NoOpRegistry {
	return &NoOpRegistry{}
}

// HTTP Request metrics
func (r *NoOpRegistry) IncrementRequests(endpoint, method, status string)                    {}
func (r *NoOpRegistry) RecordRequestLatency(endpoint, method string, duration time.Duration) {}

// Bid response metrics
func (r *NoOpRegistry) IncrementNoBids() {}

// Event tracking metrics
func (r *NoOpRegistry) IncrementImpressions(status string) {}
func (r *NoOpRegistry) IncrementEvent(eventType string)    {}

// Spend tracking metrics
func (r *NoOpRegistry) SetSpendTotal(campaign string, amount float64) {}
func (r *NoOpRegistry) IncrementSpendPersistErrors()                  {}

// Rate limiting metrics
func (r *NoOpRegistry) IncrementRateLimitRequests(lineItemID string) {}
func (r *NoOpRegistry) IncrementRateLimitHits(lineItemID string)     {}

// Report metrics
func (r *NoOpRegistry) IncrementReports() {}

// CTR prediction metrics
func (r *NoOpRegistry) IncrementCTRPredictionRequests(outcome string)     {}
func (r *NoOpRegistry) RecordCTRPredictionLatency(duration time.Duration) {}
func (r *NoOpRegistry) RecordCTRBoostMultiplier(multiplier float64)       {}
