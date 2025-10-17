# Rate Limiting

The ad server includes built-in rate limiting to protect against request floods and ensure fair resource allocation across line items. Rate limiting uses a token bucket algorithm and applies only to direct line items.

## Key Features

- **Per-line-item isolation**: Each direct line item has its own rate limit bucket
- **Token bucket algorithm**: Provides burst capacity with sustained rate control
- **Real-time monitoring**: Prometheus metrics for observability
- **Configurable**: All parameters adjustable via environment variables
- **Selective application**: Only applies to direct line items, not programmatic

## How It Works

### Token Bucket Algorithm

Each direct line item gets a token bucket with two key parameters:

- **Capacity**: Maximum number of tokens (burst allowance)
- **Refill Rate**: Tokens added per second (sustained rate)

When an ad request comes in:
1. If the bucket has tokens, one token is consumed and the request proceeds
2. If the bucket is empty, the request is blocked (rate limited)
3. Tokens refill continuously at the configured rate, up to capacity

### Integration Point

Rate limiting is applied early in the ad selection pipeline to ensure fair competition:
1. Targeting
2. Size matching
3. Active status
4. **Rate limiting** ← Applied here
5. Frequency capping
6. Pacing
7. Programmatic bidding
8. Priority ranking

Rate limiting filters out ineligible creatives without stopping the entire selection process.

## Configuration

Rate limiting is configured via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_ENABLED` | `true` | Enable/disable rate limiting |
| `RATE_LIMIT_CAPACITY` | `100` | Token bucket capacity (burst size) |
| `RATE_LIMIT_REFILL_RATE` | `10` | Tokens added per second |


## Monitoring

### Prometheus Metrics

| Metric | Description |
|--------|-------------|
| `adserver_ratelimit_requests_total{line_item_id}` | Total requests processed by rate limiter per line item |
| `adserver_ratelimit_hits_total{line_item_id}` | Total requests blocked by rate limiter per line item |

### Key Queries

| Query Purpose | PromQL |
|---------------|--------|
| **Hit rate percentage** | `rate(adserver_ratelimit_hits_total[5m]) / rate(adserver_ratelimit_requests_total[5m]) * 100` |
| **Top rate-limited line items** | `topk(10, rate(adserver_ratelimit_hits_total[5m]))` |
| **Total blocks per second** | `sum(rate(adserver_ratelimit_hits_total[5m]))` |
| **Hit rate for specific line item** | `adserver_ratelimit_hits_total{line_item_id="123"} / adserver_ratelimit_requests_total{line_item_id="123"}` |

## Implementation Details

### Code Structure

```
internal/logic/ratelimit/
├── token_bucket.go      # Core token bucket implementation
├── limiter.go          # Line item rate limiter manager
└── token_bucket_test.go # Unit tests
```

### Key Components

**TokenBucket** (`token_bucket.go`):
- Thread-safe token bucket implementation
- Tracks capacity, current tokens, refill rate
- Records hit/total statistics

**LineItemLimiter** (`limiter.go`):
- Manages token buckets per line item ID
- Integrates with Prometheus metrics
- Provides statistics aggregation

**Integration** (`rule_based.go`):
- Applied in ad selection pipeline via `SetRateLimiter()` method
- Only affects direct line items
- Graceful degradation when disabled
- Rate limiter is optional - selector works without it

### Thread Safety

All rate limiting components are thread-safe:
- `TokenBucket` uses mutex for state protection
- `LineItemLimiter` uses read-write mutex for bucket map
- Lazy bucket creation with double-checked locking

## Tuning Guidelines

| Traffic Level | Capacity (Burst) | Refill Rate | Monitoring Threshold |
|---------------|------------------|-------------|----------------------|
| **High** | 100-500 tokens | 20-100 tokens/sec | Alert if hit rate >10% |
| **Medium** | 50-100 tokens | 5-20 tokens/sec | Alert if hit rate >15% |
| **Low** | 10-50 tokens | 1-5 tokens/sec | Alert if hit rate >20% |

## Troubleshooting

| Issue | Solution |
|-------|----------|
| **High rate limit hits** | Check traffic patterns, review targeting, increase limits temporarily |
| **Performance concerns** | Rate limiting adds ~1μs per request, ~100 bytes per line item |
| **Debug rate limiting** | Set `DEBUG_TRACE=true` to trace decisions in `/test/bid` endpoint |

## Best Practices

- **Start conservative**: Begin with lower limits and increase as needed
- **Monitor actively**: Set up alerts for unusual hit rates
- **Test thoroughly**: Verify rate limiting in staging environments
- **Regular review**: Adjust limits based on traffic patterns

## Limitations

- In-memory only (not shared across server instances)
- Line item buckets persist for server lifetime
- No automatic adjustment based on system load
- Does not account for request complexity differences
