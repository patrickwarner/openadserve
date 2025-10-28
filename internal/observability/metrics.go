package observability

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// total requests per endpoint, method and status code
	RequestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "adserver_requests_total",
			Help: "Total API requests received",
		},
		[]string{"endpoint", "method", "status"},
	)

	// request latency in seconds per endpoint/method
	RequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "adserver_request_duration_seconds",
			Help:    "Histogram of request latencies",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"endpoint", "method"},
	)

	// number of no-bid responses
	NoBidCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "adserver_nobid_total",
			Help: "Total no-bid (empty) responses",
		},
	)

	// number of impression events received (status code label)
	ImpressionCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "adserver_impressions_total",
			Help: "Total impression events",
		},
		[]string{"status"},
	)

	// number of events recorded, labelled by type
	EventCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "adserver_events_total",
			Help: "Total events recorded",
		},
		[]string{"type"},
	)

	// spend tracked per campaign/line item
	SpendTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "adserver_spend_total",
			Help: "Total spend recorded",
		},
		[]string{"campaign"},
	)

	// number of errors persisting spend updates
	SpendPersistErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "adserver_spend_persist_errors_total",
			Help: "Total spend persistence errors",
		},
	)

	// rate limit hits per line item
	RateLimitHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "adserver_ratelimit_hits_total",
			Help: "Total rate limit hits per line item",
		},
		[]string{"line_item_id"},
	)

	// rate limit requests per line item
	RateLimitRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "adserver_ratelimit_requests_total",
			Help: "Total rate limit requests per line item",
		},
		[]string{"line_item_id"},
	)

	// number of ad reports submitted
	ReportCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "adserver_reports_total",
			Help: "Total ad reports submitted",
		},
	)

	// CTR prediction requests labelled by outcome
	CTRPredictionRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "adserver_ctr_prediction_total",
			Help: "Total CTR prediction requests",
		},
		[]string{"outcome"},
	)

	// Latency of CTR prediction service calls
	CTRPredictionLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "adserver_ctr_prediction_duration_seconds",
			Help:    "Duration of CTR prediction requests",
			Buckets: prometheus.DefBuckets,
		},
	)

	// Distribution of boost multipliers returned by the predictor
	CTRBoostMultiplier = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "adserver_ctr_boost_multiplier",
			Help:    "Histogram of CTR boost multipliers",
			Buckets: prometheus.LinearBuckets(0, 0.1, 21),
		},
	)

	// Filter duration for ad selection
	FilterDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "adserver_filter_duration_seconds",
			Help: "Duration of filter operations in ad selection",
			Buckets: []float64{
				0.0001, // 100μs
				0.0005, // 500μs
				0.001,  // 1ms
				0.002,  // 2ms
				0.005,  // 5ms
				0.01,   // 10ms
				0.02,   // 20ms
				0.05,   // 50ms
				0.1,    // 100ms
			},
		},
		[]string{"creative_count_bucket", "result"},
	)

	// Number of creatives filtered at each stage
	FilterStageCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "adserver_filter_stage_creatives",
			Help: "Number of creatives remaining after each filter stage",
		},
		[]string{"stage"},
	)
)

func init() {
	// register all metrics
	prometheus.MustRegister(
		RequestCount,
		RequestLatency,
		NoBidCount,
		ImpressionCount,
		EventCount,
		SpendTotal,
		SpendPersistErrors,
		RateLimitHits,
		RateLimitRequests,
		ReportCount,
		CTRPredictionRequests,
		CTRPredictionLatency,
		CTRBoostMultiplier,
		FilterDuration,
		FilterStageCount,
	)
}

// GetCreativeCountBucket returns a bucket label for the number of creatives
func GetCreativeCountBucket(count int) string {
	switch {
	case count <= 10:
		return "1-10"
	case count <= 50:
		return "11-50"
	case count <= 100:
		return "51-100"
	case count <= 500:
		return "101-500"
	case count <= 1000:
		return "501-1000"
	default:
		return "1000+"
	}
}
