package observability

import "time"

// MockMetricsRegistry is a mock implementation of MetricsRegistry for testing
type MockMetricsRegistry struct{}

// HTTP Request metrics
func (m *MockMetricsRegistry) IncrementRequests(endpoint, method, status string)                    {}
func (m *MockMetricsRegistry) RecordRequestLatency(endpoint, method string, duration time.Duration) {}

// Bid response metrics
func (m *MockMetricsRegistry) IncrementNoBids() {}

// Event tracking metrics
func (m *MockMetricsRegistry) IncrementImpressions(status string) {}
func (m *MockMetricsRegistry) IncrementEvent(eventType string)    {}

// Spend tracking metrics
func (m *MockMetricsRegistry) SetSpendTotal(campaign string, amount float64) {}
func (m *MockMetricsRegistry) IncrementSpendPersistErrors()                  {}

// Rate limiting metrics
func (m *MockMetricsRegistry) IncrementRateLimitRequests(lineItemID string) {}
func (m *MockMetricsRegistry) IncrementRateLimitHits(lineItemID string)     {}

// Report metrics
func (m *MockMetricsRegistry) IncrementReports() {}

// CTR prediction metrics
func (m *MockMetricsRegistry) IncrementCTRPredictionRequests(outcome string)     {}
func (m *MockMetricsRegistry) RecordCTRPredictionLatency(duration time.Duration) {}
func (m *MockMetricsRegistry) RecordCTRBoostMultiplier(multiplier float64)       {}
