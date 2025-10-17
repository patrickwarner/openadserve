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
	)
}
