package models

// NewTestAdDataStore creates a new in-memory ad data store for testing
func NewTestAdDataStore() AdDataStore {
	return NewInMemoryAdDataStore()
}
