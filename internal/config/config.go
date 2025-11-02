package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds application configuration derived from environment variables.
type Config struct {
	Port                   string
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	RedisAddr              string
	ClickHouseDSN          string
	PostgresDSN            string
	GeoIPDB                string
	DebugTrace             bool
	ReloadInterval         time.Duration
	TokenSecret            string
	TokenTTL               time.Duration
	RateLimitEnabled       bool
	RateLimitCapacity      int
	RateLimitRefillRate    int
	CTROptimizationEnabled bool
	CTRPredictorURL        string
	CTRPredictorTimeout    time.Duration
	CTRPredictorCacheTTL   time.Duration
	ProgrammaticBidTimeout time.Duration
	ServiceName            string
	// PID pacing configuration
	PIDKp float64
	PIDKi float64
	PIDKd float64
	// Database connection pooling configuration
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBConnMaxIdleTime time.Duration
	// ClickHouse connection pooling configuration
	CHMaxOpenConns    int
	CHMaxIdleConns    int
	CHConnMaxLifetime time.Duration
	CHConnMaxIdleTime time.Duration
	// Tracing configuration
	TracingEnabled    bool
	TempoEndpoint     string
	TracingSampleRate float64
}

// Load parses environment variables and returns a Config populated with
// defaults when variables are absent.
func Load() Config {
	cfg := Config{}

	cfg.Port = getenv("PORT", "8787")
	cfg.ReadTimeout = envDuration("READ_TIMEOUT", 5*time.Second)
	cfg.WriteTimeout = envDuration("WRITE_TIMEOUT", 10*time.Second)
	cfg.RedisAddr = getenv("REDIS_ADDR", "localhost:6379")
	cfg.ClickHouseDSN = getenv("CLICKHOUSE_DSN", "clickhouse://default:@localhost:9000/default?async_insert=1&wait_for_async_insert=1")
	cfg.PostgresDSN = getenv("POSTGRES_DSN", "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable")
	cfg.GeoIPDB = getenv("GEOIP_DB", "internal/geoip/testdata/GeoLite2-Country.mmdb")
	cfg.DebugTrace = envBool("DEBUG_TRACE", false)
	// default to 30 seconds between automatic reloads
	cfg.ReloadInterval = envDuration("RELOAD_INTERVAL", 30*time.Second)
	cfg.TokenSecret = getenv("TOKEN_SECRET", "")
	cfg.TokenTTL = envDuration("TOKEN_TTL", 30*time.Minute)
	cfg.RateLimitEnabled = envBool("RATE_LIMIT_ENABLED", true)
	cfg.RateLimitCapacity = envInt("RATE_LIMIT_CAPACITY", 100)
	cfg.RateLimitRefillRate = envInt("RATE_LIMIT_REFILL_RATE", 10)
	cfg.CTROptimizationEnabled = envBool("CTR_OPTIMIZATION_ENABLED", false)
	cfg.CTRPredictorURL = getenv("CTR_PREDICTOR_URL", "http://localhost:8000")
	cfg.CTRPredictorTimeout = envDuration("CTR_PREDICTOR_TIMEOUT", 100*time.Millisecond)
	cfg.CTRPredictorCacheTTL = envDuration("CTR_PREDICTOR_CACHE_TTL", 5*time.Minute)
	cfg.ProgrammaticBidTimeout = envDuration("PROGRAMMATIC_BID_TIMEOUT", 800*time.Millisecond)
	cfg.ServiceName = getenv("SERVICE_NAME", "openadserve")

	// PID pacing configuration with conservative defaults
	cfg.PIDKp = envFloat("PID_KP", 0.3)
	cfg.PIDKi = envFloat("PID_KI", 0.05)
	cfg.PIDKd = envFloat("PID_KD", 0.1)

	// Database connection pooling configuration
	cfg.DBMaxOpenConns = envInt("DB_MAX_OPEN_CONNS", 25)
	cfg.DBMaxIdleConns = envInt("DB_MAX_IDLE_CONNS", 5)
	cfg.DBConnMaxLifetime = envDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute)
	cfg.DBConnMaxIdleTime = envDuration("DB_CONN_MAX_IDLE_TIME", 1*time.Minute)

	// ClickHouse connection pooling configuration
	// Default to higher values than PostgreSQL due to async insert patterns and high event volume
	cfg.CHMaxOpenConns = envInt("CH_MAX_OPEN_CONNS", 100)
	cfg.CHMaxIdleConns = envInt("CH_MAX_IDLE_CONNS", 25)
	cfg.CHConnMaxLifetime = envDuration("CH_CONN_MAX_LIFETIME", 5*time.Minute)
	cfg.CHConnMaxIdleTime = envDuration("CH_CONN_MAX_IDLE_TIME", 1*time.Minute)

	// Tracing configuration
	cfg.TracingEnabled = envBool("TRACING_ENABLED", false)
	cfg.TempoEndpoint = getenv("TEMPO_ENDPOINT", "tempo:4317")
	cfg.TracingSampleRate = envFloat("TRACING_SAMPLE_RATE", 1.0) // Default to 100% sampling for dev

	return cfg
}

// getenv returns the value of the environment variable if set, otherwise def.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envDuration parses an environment variable into a time.Duration.
// The value can be a duration string (e.g. "5s") or a number of seconds.
// If the variable is unset or invalid, def is returned.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	return def
}

// envBool parses a boolean environment variable. Accepted values are those
// supported by strconv.ParseBool. When unset or invalid, def is returned.
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return def
}

// envInt parses an integer environment variable. When unset or invalid, def is returned.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	return def
}

// envFloat parses a float64 environment variable. When unset or invalid, def is returned.
func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return def
}
