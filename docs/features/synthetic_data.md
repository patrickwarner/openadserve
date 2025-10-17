# Synthetic Data Generation for CTR Optimization

This document explains how to bootstrap the CTR optimization system with realistic synthetic data when historical ad serving data is not available.

## Overview

The CTR prediction model requires 7 days of historical data. In development environments without real traffic, synthetic data generation provides realistic ad serving events with embedded CTR patterns.

## Embedded Patterns

| Pattern Type | Values | Impact |
|--------------|--------|--------|
| **Device CTR** | Mobile: 2.5%, Desktop: 1.2%, Tablet: 1.8% | Baseline performance |
| **Time of Day** | Evening (6-10 PM): +60-70%, Business (9-5 PM): +20-30% | Engagement multiplier |
| **Day of Week** | Wednesday: Peak, Weekdays > Weekends, Sunday: Lowest | Traffic distribution |
| **Line Items** | CPC campaigns: +20%, CPC + Mobile: +30% | Performance boost |

## Usage

### Automated Bootstrap (Recommended)

```bash
# Start the CTR predictor service
docker compose up ctr-predictor -d

# Run the bootstrap script
./scripts/bootstrap-ctr-optimization.sh
```

This script generates 7 days of data, trains the model, and verifies predictions.

### Manual Steps

```bash
# 1. Generate synthetic data
curl -X POST http://localhost:8000/generate-synthetic-data?days=7&impressions_per_day=10000

# 2. Train the model
curl -X POST http://localhost:8000/train \
  -H "Content-Type: application/json" \
  -d '{"days_back": 7, "min_impressions": 100}'

# 3. Test prediction
curl -X POST http://localhost:8000/predict \
  -H "Content-Type: application/json" \
  -d '{
    "line_item_id": 2,
    "device_type": "mobile",
    "country": "US", 
    "hour_of_day": 20,
    "day_of_week": 2
  }'
```

### Verify Data Generation

```sql
-- Check total events generated
SELECT 
    event_type,
    count(*) as events,
    toDate(min(timestamp)) as first_date,
    toDate(max(timestamp)) as last_date
FROM events 
GROUP BY event_type;

-- CTR by line item
SELECT 
    line_item_id,
    countIf(event_type = 'impression') as impressions,
    countIf(event_type = 'click') as clicks,
    clicks / impressions as ctr
FROM events 
GROUP BY line_item_id
ORDER BY line_item_id;
```

## Data Characteristics

| Metric | Value | Description |
|--------|-------|-------------|
| **Duration** | 7 days | Historical data period |
| **Volume** | ~10,000/day | Impressions (configurable) |
| **Overall CTR** | ~2.5% | Varies by context |
| **Device Split** | 60% mobile, 30% desktop, 10% tablet | Traffic distribution |
| **Peak CTR** | ~4.5% | Mobile CPC evening |
| **Baseline CTR** | ~1.0% | Desktop CPM morning |
| **Noise** | ±20% | Random variation |

## Expected Model Performance

| Context | Boost | Description |
|---------|-------|-------------|
| Mobile + CPC + Evening | 1.5x | High CTR context |
| Desktop + CPM + Morning | 1.0x | Baseline performance |
| Tablet + CPC + Business | 1.3x | Good performance |

## Testing CTR Optimization

Once bootstrapped, test the full optimization pipeline:

```bash
# 1. Enable CTR optimization
export CTR_OPTIMIZATION_ENABLED=true

# 2. Send ad requests with different contexts
curl -X POST http://localhost:8787/ad \
  -H "Content-Type: application/json" \
  -d '{
    "device": {"devicetype": 1},  // Mobile
    "user": {"id": "test-user"},
    "imp": [{"id": "1", "tagid": "test-placement"}]
  }'
```

## Monitoring

Check logs for CTR optimization:
```bash
docker compose logs openadserve | grep "CTR optimization applied"
```

Example: `INFO CTR optimization applied line_item_id=2 base_ecpm=3.0 boost_multiplier=1.5 optimized_ecpm=4.5`

## Updating Synthetic Data

To regenerate with different patterns:

1. **Modify** `synthetic_data.py` parameters
2. **Rebuild** CTR predictor: `docker compose build ctr-predictor`
3. **Re-run** bootstrap: `./scripts/bootstrap-ctr-optimization.sh`

## Production Transition

As real traffic accumulates:
1. Monitor real vs synthetic data quality
2. Retrain model daily: `curl -X POST http://localhost:8000/train`
3. Gradually increase `min_impressions` threshold
4. Disable synthetic generation once sufficient real data exists

## Troubleshooting

| Issue | Solutions |
|-------|----------|
| **No events generated** | Check ClickHouse connectivity, verify events table exists, check logs |
| **Model training fails** | Ensure data generated first, verify >1000 events, check permissions |
| **Predictions not working** | Confirm model loaded (`/model-info`), verify CTR optimization enabled |
