package logic

import "errors"

// ErrNilRedisStore is returned when a RedisStore pointer is nil or uninitialized.
var ErrNilRedisStore = errors.New("redis store is nil")
