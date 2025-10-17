package models

import "go.uber.org/zap"

// Publisher represents a site or app that uses the ad server.
type Publisher struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
	APIKey string `json:"api_key"`
}

// SetPublishers replaces the in-memory publisher slice.
// This function delegates to the AdDataStore for thread-safe access.
func SetPublishers(store AdDataStore, p []Publisher) {
	if store == nil {
		return
	}
	if err := store.SetPublishers(p); err != nil {
		zap.L().Warn("failed to set publishers", zap.Error(err))
	}
}

// GetPublisherByID returns the publisher with the given ID or nil if not found.
// This function delegates to the AdDataStore for thread-safe access.
func GetPublisherByID(store AdDataStore, id int) *Publisher {
	if store == nil {
		return nil
	}
	return store.GetPublisher(id)
}
