# CTR Optimization for CPC Line Items

This document describes the CTR (Click-Through Rate) optimization feature that enhances CPC line item delivery by predicting click probability and adjusting eCPM calculations accordingly.

## Overview

The CTR optimization system uses machine learning to predict the likelihood of a click for each CPC line item based on contextual factors like device type, geography, and time. This enables the ad server to prioritize line items that are more likely to generate clicks in specific contexts, improving overall campaign performance and publisher revenue.

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐
│   Ad Request    │───▶│   Ad Server      │───▶│  CTR Predictor      │
│                 │    │  (Go Service)    │    │  (Python Service)   │
└─────────────────┘    └──────────────────┘    └─────────────────────┘
                                │                           │
                                ▼                           ▼
                       ┌─────────────────┐    ┌─────────────────────┐
                       │  Redis Cache    │    │    ClickHouse       │
                       │ (Predictions)   │    │ (Training Data)     │
                       └─────────────────┘    └─────────────────────┘
```

### Components

1. **Ad Server (Go)**: Main service that handles ad requests and applies CTR optimization
2. **CTR Predictor (Python)**: ML service that provides click probability predictions
3. **Redis**: Caches predictions to minimize latency
4. **ClickHouse**: Stores historical click data for model training

## How It Works

### 1. Model Training

The CTR predictor trains a logistic regression model using historical impression and click data:

```python
# Features used for prediction
- line_item_id (encoded)
- device_type (encoded: mobile, desktop, tablet)
- country (encoded: US, UK, CA, etc.)
- publisher_id (encoded)
- hour_of_day (0-23)
- day_of_week (0-6)
- is_weekend (boolean)
- is_business_hours (9-17)
- is_evening_hours (18-22)
```

### 2. Prediction Request

When selecting ads, the ad server sends a prediction request:

```json
{
  "line_item_id": 123,
  "device_type": "mobile",
  "country": "US",
  "publisher_id": 1,
  "hour_of_day": 14,
  "day_of_week": 2
}
```

### 3. CTR Score and Boost

The predictor returns a CTR score and boost multiplier:

```json
{
  "line_item_id": 123,
  "ctr_score": 0.025,
  "confidence": 0.8,
  "boost_multiplier": 1.5
}
```

### 4. eCPM Optimization

The ad server applies the boost to CPC line items:

```
Optimized eCPM = Base eCPM × Boost Multiplier
```

Example:
- Base eCPM: $3.00
- Boost Multiplier: 1.5 (50% boost for mobile)
- **Optimized eCPM: $4.50**

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CTR_OPTIMIZATION_ENABLED` | Enable/disable CTR optimization | `false` |
| `CTR_PREDICTOR_URL` | URL of the CTR prediction service | `http://localhost:8000` |
| `CLICKHOUSE_HOST` | ClickHouse server for training data | `localhost` |
| `CLICKHOUSE_PORT` | ClickHouse port | `9000` |
| `CLICKHOUSE_DB` | ClickHouse database name | `default` |

### Enabling CTR Optimization

```bash
# Enable CTR optimization
export CTR_OPTIMIZATION_ENABLED=true
export CTR_PREDICTOR_URL=http://ctr-predictor:8000

# Start services
docker-compose up -d
```

## Model Training

### Initial Training

Train the model using the last 7 days of data:

```bash
curl -X POST http://localhost:8000/train \
  -H "Content-Type: application/json" \
  -d '{"days_back": 7, "min_impressions": 100}'
```

### Response

```json
{
  "status": "success",
  "samples_trained": 15420,
  "model_accuracy": 0.732,
  "auc_score": 0.785
}
```

### Automated Retraining

Set up a cron job to retrain the model daily:

```bash
# Retrain model every day at 2 AM
0 2 * * * curl -X POST http://ctr-predictor:8000/train -H "Content-Type: application/json" -d '{"days_back": 7}'
```

## Performance Characteristics

### Latency

- **Prediction Timeout**: 100ms
- **Cache Hit Rate**: ~90% (5-minute TTL)
- **Average Response Time**: <10ms (cached), <50ms (uncached)

### Fallback Behavior

This system's fallback behavior:
- Service unavailable → Use base eCPM (no optimization)
- Timeout → Use base eCPM (graceful degradation)
- Unknown line item → Use default boost (1.0x)

## Monitoring

### Health Checks

```bash
# Check CTR predictor health
curl http://localhost:8000/health

# Get model information
curl http://localhost:8000/model-info
```

### Metrics

The system exposes several Prometheus metrics:

- CTR prediction request count and success/failure rate
- CTR prediction latency histogram
- Cache hit/miss ratio
- Model accuracy and AUC scores
- Boost multiplier distribution
- Line item performance changes

### Example Logs

```
INFO CTR optimization applied line_item_id=123 base_ecpm=3.0 boost_multiplier=1.5 optimized_ecpm=4.5
WARN CTR prediction failed, using base eCPM line_item_id=456 error="context deadline exceeded"
```

## Business Impact

### Expected Improvements

1. **Higher CTR**: CPC campaigns receive better targeting
2. **Increased Revenue**: Publishers earn more from well-performing line items
3. **Better User Experience**: More relevant ads for each context
4. **Campaign Performance**: Advertisers see improved click rates

### Example Scenarios

**Mobile Users (Evening)**
- Device: Mobile
- Time: 8 PM
- Expected: 50% boost for CPC line items
- Result: Mobile-optimized CPC campaigns win more auctions

**Desktop Users (Business Hours)**
- Device: Desktop  
- Time: 2 PM
- Expected: 10% boost for CPC line items
- Result: Moderate optimization, maintains revenue balance

## Troubleshooting

### Common Issues

1. **No Predictions**: Check if CTR service is running and accessible
2. **Low Cache Hit Rate**: Increase cache TTL or check request patterns
3. **Poor Model Performance**: Retrain with more data or adjust features
4. **High Latency**: Check network connectivity and increase timeout

### Debug Commands

```bash
# Check service connectivity
curl -v http://ctr-predictor:8000/health

# Test prediction manually
curl -X POST http://ctr-predictor:8000/predict \
  -H "Content-Type: application/json" \
  -d '{"line_item_id": 123, "device_type": "mobile", "country": "US", "publisher_id": 1, "hour_of_day": 14, "day_of_week": 2}'

# Check CTR prediction service metrics via Prometheus
curl http://openadserve:8787/metrics | grep ctr_prediction
```

## Testing

### Integration Tests

Run the integration test suite:

```bash
go test -v ./tests/integration/ctr_optimization/
```

### A/B Testing

Compare performance with and without optimization:

1. Deploy with `CTR_OPTIMIZATION_ENABLED=false` (baseline)
2. Collect metrics for 1 week
3. Enable optimization with `CTR_OPTIMIZATION_ENABLED=true`
4. Compare CTR and revenue metrics

### Load Testing

Test CTR predictor under load:

```bash
# Install hey load testing tool
go install github.com/rakyll/hey@latest

# Run load test
hey -n 1000 -c 10 -m POST -H "Content-Type: application/json" \
  -d '{"line_item_id": 123, "device_type": "mobile", "country": "US", "publisher_id": 1, "hour_of_day": 14, "day_of_week": 2}' \
  http://localhost:8000/predict
```

## Future Enhancements

### Planned Features

1. **Real-time Model Updates**: Stream learning from live click data
2. **Multi-armed Bandit**: Explore/exploit optimization for new line items
3. **Deep Learning Models**: Neural networks for more complex patterns
4. **Cross-device Tracking**: User journey optimization
5. **Contextual Bandits**: Personalized predictions per user segment

### Advanced Features

1. **Feature Engineering**: Add publisher-specific features
2. **Model Ensembles**: Combine multiple models for better accuracy
3. **Seasonal Adjustments**: Account for daily/weekly/monthly patterns
4. **Geographic Modeling**: Country/region-specific predictions
5. **Creative-level Optimization**: Optimize at creative rather than line item level

## Security Considerations

1. **Data Privacy**: No PII is used in predictions
2. **Model Security**: Models are stored in secured volume mounts
3. **API Security**: Internal service communication only
4. **Audit Logging**: All predictions and model changes are logged

## Conclusion

The CTR optimization feature provides a powerful enhancement to CPC line item delivery, leveraging machine learning to improve both advertiser performance and publisher revenue. The system is designed for high availability, low latency, and graceful degradation, ensuring reliable ad serving even when optimization components are unavailable.
