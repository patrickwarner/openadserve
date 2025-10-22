package analytics

import (
	"context"

	"github.com/patrickwarner/openadserve/internal/models"
)

var _ AnalyticsService = (*MockAnalytics)(nil)

// MockAnalytics is a mock implementation of Analytics for testing
type MockAnalytics struct {
	DB          *MockClickHouseDB
	PG          interface{} // Not used in tests
	AdDataStore interface{} // Not used in tests
	Metrics     interface{} // Not used in tests
}

// NewMockAnalytics creates a new mock analytics instance
func NewMockAnalytics() *MockAnalytics {
	return &MockAnalytics{
		DB: &MockClickHouseDB{},
	}
}

// RecordEvent records a custom event (mock implementation)
func (m *MockAnalytics) RecordEvent(ctx context.Context, dataStore models.AdDataStore, eventType, requestID, impID, creativeID string, lineItemID int, cost float64, targetingCtx models.TargetingContext, publisherID int, placementID string) error {
	return nil
}

// RecordImpression records an impression event (mock implementation)
func (m *MockAnalytics) RecordImpression(ctx context.Context, dataStore models.AdDataStore, requestID, impID, creativeID string, lineItemID int, deviceType, country string, publisherID int, placementID string) error {
	return nil
}

// RecordClick records a click event (mock implementation)
func (m *MockAnalytics) RecordClick(ctx context.Context, dataStore models.AdDataStore, requestID, impID, creativeID string, lineItemID int, deviceType, country string, publisherID int, placementID string) error {
	return nil
}

// MockClickHouseDB is a mock implementation of ClickHouse database for testing
type MockClickHouseDB struct{}

// Close is a mock implementation
func (m *MockClickHouseDB) Close() error {
	return nil
}
