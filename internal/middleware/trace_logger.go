package middleware

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// loggerKey is the context key for the logger
type loggerKey struct{}

// WithTraceLogger returns middleware that adds trace IDs to the logger in context
func WithTraceLogger(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get trace context from the request
			span := trace.SpanFromContext(r.Context())
			if span.SpanContext().IsValid() {
				// Create a logger with trace ID
				tracedLogger := logger.With(
					zap.String("trace_id", span.SpanContext().TraceID().String()),
					zap.String("span_id", span.SpanContext().SpanID().String()),
				)
				// Add the logger to context
				ctx := context.WithValue(r.Context(), loggerKey{}, tracedLogger)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LoggerFromContext retrieves the logger from context
// If no logger is found, returns the provided fallback logger
func LoggerFromContext(ctx context.Context, fallback *zap.Logger) *zap.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*zap.Logger); ok {
		return logger
	}
	// If no logger in context, try to add trace ID from span
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		return fallback.With(
			zap.String("trace_id", span.SpanContext().TraceID().String()),
			zap.String("span_id", span.SpanContext().SpanID().String()),
		)
	}
	return fallback
}

// LoggerFromRequest is a convenience function to get logger from HTTP request
func LoggerFromRequest(r *http.Request, fallback *zap.Logger) *zap.Logger {
	return LoggerFromContext(r.Context(), fallback)
}
