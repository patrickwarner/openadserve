package logic

import (
	"context"
	"testing"

	"github.com/patrickwarner/openadserve/internal/db"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// setupTestRedis spins up an in-memory Redis and points db.Redis at it.
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *db.RedisStore) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	store := &db.RedisStore{
		Client: redis.NewClient(&redis.Options{Addr: s.Addr()}),
		Ctx:    context.Background(),
	}
	return s, store
}
