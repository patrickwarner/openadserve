package observability

import (
	"math/rand"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitLogger constructs a production zap.Logger configured for the service.
// The returned logger should be passed to other components for structured logging.
func InitLogger() (*zap.Logger, error) {
	level := getLogLevel()
	return InitLoggerWithLevel(level, "openadserve")
}

// InitLoggerWithService constructs a production zap.Logger configured for the service.
// The returned logger should be passed to other components for structured logging.
func InitLoggerWithService(serviceName string) (*zap.Logger, error) {
	level := getLogLevel()
	return InitLoggerWithLevel(level, serviceName)
}

// InitLoggerWithLevel constructs a zap.Logger at the provided level.
// The returned logger is named with the service name and installed as the global logger.
func InitLoggerWithLevel(level zapcore.Level, serviceName string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)

	// Configure encoder to use consistent field names that match Promtail expectations
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.NameKey = "logger"
	cfg.EncoderConfig.CallerKey = "caller"
	cfg.EncoderConfig.MessageKey = "msg"
	cfg.EncoderConfig.StacktraceKey = "stacktrace"

	logger, err := cfg.Build()
	if err != nil {
		return nil, err
	}

	// Add service name as a permanent field and use it as the logger name
	logger = logger.Named(serviceName).With(zap.String("service", serviceName))
	zap.ReplaceGlobals(logger)
	return logger, nil
}

var (
	samplingMutex sync.Mutex
	samplingStats = make(map[float64]SamplingStats)
)

type SamplingStats struct {
	Total   int64
	Sampled int64
	Rate    float64
}

// getLogLevel determines the appropriate log level based on environment
func getLogLevel() zapcore.Level {
	env := strings.ToLower(os.Getenv("ENV"))
	logLevel := strings.ToUpper(os.Getenv("LOG_LEVEL"))

	// Environment-based defaults
	switch env {
	case "development", "dev":
		if logLevel == "" {
			return zap.DebugLevel
		}
	case "staging", "test":
		if logLevel == "" {
			return zap.InfoLevel
		}
	default: // production
		if logLevel == "" {
			return zap.InfoLevel
		}
	}

	// Override with explicit LOG_LEVEL
	switch logLevel {
	case "DEBUG":
		return zap.DebugLevel
	case "INFO":
		return zap.InfoLevel
	case "WARN":
		return zap.WarnLevel
	case "ERROR":
		return zap.ErrorLevel
	default:
		return zap.InfoLevel
	}
}

// ShouldSample returns true if the log should be sampled based on the given rate
// rate should be between 0.0 and 1.0 (e.g., 0.1 for 10% sampling)
// This function is thread-safe and tracks sampling statistics
func ShouldSample(rate float64) bool {
	if rate >= 1.0 {
		return true
	}
	if rate <= 0.0 {
		return false
	}

	// Use thread-safe math/rand (Go 1.20+)
	shouldSample := rand.Float64() < rate

	// Update sampling statistics
	samplingMutex.Lock()
	stats := samplingStats[rate]
	stats.Total++
	stats.Rate = rate
	if shouldSample {
		stats.Sampled++
	}
	samplingStats[rate] = stats
	samplingMutex.Unlock()

	return shouldSample
}

// GetSamplingRate returns the appropriate sampling rate based on environment
func GetSamplingRate() float64 {
	env := strings.ToLower(os.Getenv("ENV"))
	switch env {
	case "development", "dev":
		return 1.0 // No sampling in development
	case "staging", "test":
		return 0.5 // 50% sampling in staging
	default: // production
		return 0.1 // 10% sampling in production
	}
}

// GetSamplingStats returns current sampling statistics
func GetSamplingStats() map[float64]SamplingStats {
	samplingMutex.Lock()
	defer samplingMutex.Unlock()

	// Create a copy to avoid race conditions
	result := make(map[float64]SamplingStats)
	for rate, stats := range samplingStats {
		result[rate] = stats
	}
	return result
}

// LogSamplingStats logs current sampling statistics for monitoring
func LogSamplingStats(logger *zap.Logger) {
	stats := GetSamplingStats()
	if len(stats) == 0 {
		return
	}

	for rate, stat := range stats {
		actualRate := float64(stat.Sampled) / float64(stat.Total)
		logger.Info("sampling stats",
			zap.Float64("target_rate", rate),
			zap.Float64("actual_rate", actualRate),
			zap.Int64("total_logs", stat.Total),
			zap.Int64("sampled_logs", stat.Sampled),
		)
	}
}

// ResetSamplingStats clears the sampling statistics (useful for periodic resets)
func ResetSamplingStats() {
	samplingMutex.Lock()
	defer samplingMutex.Unlock()
	samplingStats = make(map[float64]SamplingStats)
}
