# Analytics and Reporting

Campaign performance tracking using ClickHouse for real-time analytics.

## Data Model

Events are stored in ClickHouse with the following schema:

| Field | Type | Nullable | Description |
|-------|------|----------|-------------|
| `timestamp` | DateTime | No | Event timestamp |
| `event_type` | String | No | Event type: `impression`, `click`, `ad_request`, `ad_served`, or custom |
| `request_id` | String | No | Unique ad request identifier |
| `imp_id` | String | No | Impression identifier from request |
| `creative_id` | Int32 | Yes | ID of the creative served |
| `campaign_id` | Int32 | Yes | Associated campaign ID |
| `line_item_id` | Int32 | Yes | Associated line item ID |
| `cost` | Float64 | No | Cost/price of the event |
| `device_type` | String | Yes | Device type: `mobile`, `desktop`, `tablet` |
| `country` | String | Yes | ISO 3166-1 alpha-2 country code |
| `publisher_id` | Int32 | Yes | Publisher identifier |

**Table Engine**: `MergeTree()` ordered by `(event_type, timestamp)` for optimal query performance.

## Campaign Reports

Generate performance reports with the CLI tool:

```bash
go run ./tools/campaign_report -campaign-id=123 -days=30
```

**Output includes:**
- Overall metrics (impressions, clicks, CTR, spend, CPM, CPC)
- Daily breakdown table
- Top performing creatives
- Automated performance insights

**Example:**
```
📊 OVERALL PERFORMANCE
Total Impressions:  3,206
Total Clicks:       293
Overall CTR:        9.14%

🎨 TOP PERFORMING CREATIVES
Creative ID | Impressions | Clicks |   CTR   
------------|-------------|--------|----------
    1005021 |         840 |    167 |  19.88%
    1005032 |       1,013 |     56 |   5.53%

💡 INSIGHTS
✅ Excellent CTR (9.14%) - campaign performing well!
📈 Creative 1005021 is performing 3.9x better than Creative 1005032
```

## Custom Events

Track custom events beyond impressions and clicks:

```bash
# Configure allowed events
ALLOWED_EVENTS=like,share,conversion,signup

# Events recorded via pre-signed URLs
GET /event?t=TOKEN&type=like
```

## Grafana Dashboards

The included dashboard provides real-time metrics:

- Live impression/click counts
- CTR and spend calculations  
- Campaign performance comparison
- Creative analysis

Key queries:
```sql
-- Total impressions (24h)
SELECT count() FROM events 
WHERE event_type = 'impression' 
AND timestamp >= now() - INTERVAL 24 HOUR

-- Campaign CTR
SELECT 
    campaign_id,
    countIf(event_type = 'click') / countIf(event_type = 'impression') * 100 as ctr
FROM events 
WHERE timestamp >= now() - INTERVAL 24 HOUR 
GROUP BY campaign_id
```

## CTR Optimization Metrics

When CTR optimization is enabled, additional analytics track machine learning model performance:

- Model prediction accuracy vs. actual CTR outcomes
- Context-specific CTR patterns (device type, country, time-based)  
- Optimization boost multiplier effectiveness
- Training data volume and model retrain frequency

Key queries for CTR optimization monitoring:
```sql
-- CTR by device and country (for model validation)
SELECT 
    device_type,
    country,
    countIf(event_type = 'click') / countIf(event_type = 'impression') * 100 as ctr
FROM events 
WHERE timestamp >= now() - INTERVAL 7 DAY
AND device_type IS NOT NULL AND country IS NOT NULL
GROUP BY device_type, country

-- Model training data availability  
SELECT count() FROM events 
WHERE timestamp >= now() - INTERVAL 7 DAY
AND line_item_id IS NOT NULL
AND event_type IN ('impression', 'click')
```

