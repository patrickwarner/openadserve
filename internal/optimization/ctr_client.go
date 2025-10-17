package optimization

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/patrickwarner/openadserve/internal/observability"
	"go.uber.org/zap"
)

// CTRPredictionClient provides access to the CTR prediction service.
type CTRPredictionClient struct {
	baseURL    string
	httpClient *http.Client
	cache      map[string]*CachedPrediction
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration
	logger     *zap.Logger
	metrics    observability.MetricsRegistry
}

// PredictionRequest represents the request to the CTR prediction service.
type PredictionRequest struct {
	LineItemID  int    `json:"line_item_id"`
	DeviceType  string `json:"device_type"`
	Country     string `json:"country"`
	HourOfDay   int    `json:"hour_of_day"`
	DayOfWeek   int    `json:"day_of_week"`
	PublisherID *int   `json:"publisher_id,omitempty"`
}

// PredictionResponse represents the response from the CTR prediction service.
type PredictionResponse struct {
	LineItemID      int     `json:"line_item_id"`
	CTRScore        float64 `json:"ctr_score"`
	Confidence      float64 `json:"confidence"`
	BoostMultiplier float64 `json:"boost_multiplier"`
}

// CachedPrediction wraps a prediction response with caching metadata.
type CachedPrediction struct {
	Response  *PredictionResponse
	Timestamp time.Time
	TTL       time.Duration
}

// IsExpired checks if the cached prediction has expired.
func (c *CachedPrediction) IsExpired() bool {
	return time.Since(c.Timestamp) > c.TTL
}

// NewCTRPredictionClient creates a new CTR prediction client.
func NewCTRPredictionClient(baseURL string, timeout, cacheTTL time.Duration, logger *zap.Logger, metrics observability.MetricsRegistry) *CTRPredictionClient {
	return &CTRPredictionClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		cache:    make(map[string]*CachedPrediction),
		cacheTTL: cacheTTL,
		logger:   logger,
		metrics:  metrics,
	}
}

// cacheKey generates a cache key for a prediction request.
func (c *CTRPredictionClient) cacheKey(req *PredictionRequest) string {
	return fmt.Sprintf("li:%d:dev:%s:country:%s:hour:%d:dow:%d",
		req.LineItemID, req.DeviceType, req.Country, req.HourOfDay, req.DayOfWeek)
}

// GetPrediction retrieves a CTR prediction for the given request.
// It returns a default boost multiplier of 1.0 if the service is unavailable.
func (c *CTRPredictionClient) GetPrediction(ctx context.Context, req *PredictionRequest) (*PredictionResponse, error) {
	// Check cache first
	cacheKey := c.cacheKey(req)
	c.cacheMu.RLock()
	cached, exists := c.cache[cacheKey]
	c.cacheMu.RUnlock()

	if exists && !cached.IsExpired() {
		return cached.Response, nil
	}

	// Make HTTP request to prediction service
	prediction, err := c.callPredictionService(ctx, req)
	if err != nil {
		c.logger.Warn("CTR prediction service unavailable, using default boost",
			zap.Error(err),
			zap.Int("line_item_id", req.LineItemID))

		// Return default response on error
		return &PredictionResponse{
			LineItemID:      req.LineItemID,
			CTRScore:        0.01, // 1% baseline CTR
			Confidence:      0.5,  // Medium confidence
			BoostMultiplier: 1.0,  // No boost
		}, nil
	}

	// Cache the response
	c.cacheMu.Lock()
	c.cache[cacheKey] = &CachedPrediction{
		Response:  prediction,
		Timestamp: time.Now(),
		TTL:       c.cacheTTL,
	}
	c.cacheMu.Unlock()

	return prediction, nil
}

// callPredictionService makes the actual HTTP call to the prediction service.
func (c *CTRPredictionClient) callPredictionService(ctx context.Context, req *PredictionRequest) (*PredictionResponse, error) {
	start := time.Now()
	outcome := "success"
	defer func() {
		c.metrics.RecordCTRPredictionLatency(time.Since(start))
		c.metrics.IncrementCTRPredictionRequests(outcome)
	}()

	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		outcome = "failure"
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/predict", bytes.NewReader(reqBody))
	if err != nil {
		outcome = "failure"
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Make request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		outcome = "failure"
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil && c.logger != nil {
			c.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		outcome = "failure"
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var prediction PredictionResponse
	if err := json.NewDecoder(resp.Body).Decode(&prediction); err != nil {
		outcome = "failure"
		return nil, fmt.Errorf("decode response: %w", err)
	}

	c.metrics.RecordCTRBoostMultiplier(prediction.BoostMultiplier)

	return &prediction, nil
}

// HealthCheck checks if the CTR prediction service is available.
func (c *CTRPredictionClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil && c.logger != nil {
			c.logger.Warn("failed to close response body", zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// ClearCache clears the prediction cache.
func (c *CTRPredictionClient) ClearCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cache = make(map[string]*CachedPrediction)
}

// GetCacheStats returns statistics about the cache.
func (c *CTRPredictionClient) GetCacheStats() map[string]interface{} {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()

	expired := 0
	for _, cached := range c.cache {
		if cached.IsExpired() {
			expired++
		}
	}

	return map[string]interface{}{
		"total_entries":   len(c.cache),
		"expired_entries": expired,
		"active_entries":  len(c.cache) - expired,
	}
}

// CleanupExpiredCache removes expired entries from the cache.
func (c *CTRPredictionClient) CleanupExpiredCache() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	for key, cached := range c.cache {
		if cached.IsExpired() {
			delete(c.cache, key)
		}
	}
}

// StartCacheCleanup starts a goroutine that periodically cleans up expired cache entries.
func (c *CTRPredictionClient) StartCacheCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			c.CleanupExpiredCache()
		}
	}()
}

// SetBaseURL sets the base URL for the CTR prediction service (for testing).
func (c *CTRPredictionClient) SetBaseURL(url string) {
	c.baseURL = url
}
