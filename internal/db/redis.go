package db

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisStore wraps a redis client and context for operations.
type RedisStore struct {
	Client *redis.Client
	Ctx    context.Context
}

// InitRedis initializes a Redis client and returns a RedisStore.
func InitRedis(addr string) (*RedisStore, error) {
	rs := &RedisStore{
		Client: redis.NewClient(&redis.Options{Addr: addr}),
		Ctx:    context.Background(),
	}

	// Add OpenTelemetry instrumentation to Redis client
	if err := redisotel.InstrumentTracing(rs.Client); err != nil {
		return nil, fmt.Errorf("failed to instrument redis tracing: %w", err)
	}

	if err := rs.Client.Ping(rs.Ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	zap.L().Info("Connected to Redis", zap.String("addr", addr))
	return rs, nil
}

// IncrementImpression increments the Redis counter for (userID, creativeID).
// Sets a TTL of `window` if it's the first impression. Returns the current count.
func (r *RedisStore) IncrementImpression(userID string, creativeID int, window time.Duration) (int64, error) {
	key := fmt.Sprintf("freqcap:%s:%d", userID, creativeID)
	val, err := r.Client.Incr(r.Ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if val == 1 {
		r.Client.Expire(r.Ctx, key, window)
	}
	return val, nil
}

// IncrementClick increments the daily click counter for a line item.
// A 24h TTL is applied on first set.
func (r *RedisStore) IncrementClick(lineItemID int) error {
	key := fmt.Sprintf("clicks:lineitem:%d:%s", lineItemID, time.Now().Format("2006-01-02"))
	val, err := r.Client.Incr(r.Ctx, key).Result()
	if err != nil {
		return err
	}
	if val == 1 {
		r.Client.Expire(r.Ctx, key, 24*time.Hour)
	}
	return nil
}

// IncrementCustomEvent increments the daily counter for a custom event type on a line item.
// A 24h TTL is applied on first set.
func (r *RedisStore) IncrementCustomEvent(lineItemID int, eventType string) error {
	key := fmt.Sprintf("event:%s:lineitem:%d:%s", eventType, lineItemID, time.Now().Format("2006-01-02"))
	val, err := r.Client.Incr(r.Ctx, key).Result()
	if err != nil {
		return err
	}
	if val == 1 {
		r.Client.Expire(r.Ctx, key, 24*time.Hour)
	}
	return nil
}

// IncrementCTRImpression increments the total impression counter for CTR calculation.
func (r *RedisStore) IncrementCTRImpression(lineItemID int) error {
	key := fmt.Sprintf("ctr:lineitem:%d:imp", lineItemID)
	_, err := r.Client.Incr(r.Ctx, key).Result()
	return err
}

// IncrementCTRClick increments the total click counter for CTR calculation.
func (r *RedisStore) IncrementCTRClick(lineItemID int) error {
	key := fmt.Sprintf("ctr:lineitem:%d:click", lineItemID)
	_, err := r.Client.Incr(r.Ctx, key).Result()
	return err
}

// GetCTRCounts returns the total impressions and clicks for a line item.
func (r *RedisStore) GetCTRCounts(lineItemID int) (int64, int64) {
	impKey := fmt.Sprintf("ctr:lineitem:%d:imp", lineItemID)
	clickKey := fmt.Sprintf("ctr:lineitem:%d:click", lineItemID)
	imps, _ := r.Client.Get(r.Ctx, impKey).Int64()
	clicks, _ := r.Client.Get(r.Ctx, clickKey).Int64()
	return imps, clicks
}

// Close shuts down the Redis client.
func (r *RedisStore) Close() {
	if r != nil && r.Client != nil {
		if err := r.Client.Close(); err != nil {
			zap.L().Error("redis close", zap.Error(err))
		}
	}
}
