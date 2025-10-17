# Forecasting Engine

The ad server includes a forecasting engine that predicts available inventory and identifies conflicts between line items competing for the same traffic.

## Key Features

- **Opportunity-based analysis**: Uses all ad requests (not just impressions) to show total inventory
- **Conflict detection**: Identifies competing line items with overlapping targeting
- **Traffic patterns**: Analyzes 30 days of historical data with day-of-week adjustments
- **Targeting support**: Geography, device types, and custom key-values

## API Endpoint

### `POST /forecast`

Request a forecast for a potential line item configuration.

#### Request Parameters

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `start_date` | string | Yes | Campaign flight start (ISO 8601) |
| `end_date` | string | Yes | Campaign flight end (ISO 8601) |
| `budget_type` | string | Yes | One of: `cpm`, `cpc`, `flat` |
| `budget` | float | Yes | Total budget amount |
| `publisher_id` | int | Yes | Target publisher ID |
| `cpm` | float | If CPM | Cost per thousand impressions |
| `cpc` | float | If CPC | Cost per click |
| `priority` | int | No | Priority level (1=high, 2=medium, 3=low) |
| `countries` | array | No | ISO country codes (e.g., ["US", "CA"]) |
| `device_types` | array | No | Device types: mobile, desktop, tablet |
| `key_values` | object | No | Custom targeting key-value pairs |
| `daily_cap` | int | No | Maximum impressions per day |
| `pacing` | string | No | Delivery pacing: `ASAP` or `Even` |

#### Example Request

```json
{
  "start_date": "2024-01-15T00:00:00Z",
  "end_date": "2024-01-22T00:00:00Z",
  "budget_type": "cpm",
  "budget": 1000.0,
  "cpm": 5.0,
  "publisher_id": 1
}
```

#### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| **Summary Metrics** | | |
| `total_opportunities` | int | Total ad requests matching targeting |
| `available_impressions` | int | Unfilled inventory available |
| `estimated_impressions` | int | Projected impressions for this line item |
| `estimated_clicks` | int | Projected clicks (CPM/CPC campaigns) |
| `estimated_spend` | float | Projected spend |
| `fill_rate` | float | Current fill rate for matching traffic |
| `estimated_ctr` | float | Expected click-through rate |
| **Arrays** | | |
| `daily_forecast` | array | Daily projections with date and metrics |
| `conflicts` | array | Competing line items with overlap info |
| `warnings` | array | Advisory messages about data quality |

#### Conflict Object Fields

| Field | Type | Description |
|-------|------|-------------|
| `line_item_id` | int | Competing line item ID |
| `line_item_name` | string | Line item name |
| `priority` | int | Priority level |
| `overlap_percentage` | float | Targeting overlap (0.0 to 1.0) |
| `estimated_impact_impressions` | int | Delivery impact |
| `conflict_type` | string | `higher_priority`, `same_priority`, or `lower_priority` |

## Data Requirements

### ClickHouse Schema

The forecasting engine requires the enhanced events table with key-values:

```sql
ALTER TABLE events ADD COLUMN key_values Map(String, String);
```

### Historical Data

- Minimum 7 days of ad request data recommended
- Key-values are captured only from ad_request events
- Impressions and clicks are joined via request_id

## Implementation Details

### Query Strategy

1. Query 30 days of ad_request events with targeting filters
2. Join with impression events to calculate fill rates
3. Analyze existing line items for targeting overlap
4. Apply pacing and budget constraints for projections

Queries use 15-minute time buckets and are limited to 10,000 patterns for performance.

## Limitations

- Single publisher forecasts only
- Rule-based predictions (no ML)
- Basic day-of-week traffic adjustments
- No seasonality modeling
- No frequency cap simulation

## Error Handling

| Status Code | Description | Common Causes |
|-------------|-------------|---------------|
| 200 | Successful forecast | - |
| 400 | Invalid request | Missing fields, invalid dates, unknown publisher |
| 500 | Server error | Database issues, insufficient data |

## Testing

```bash
# Generate test data
docker compose exec openadserve go run ./tools/traffic_simulator

# Run unit tests
go test ./internal/forecasting/ -v

# Test API endpoint
curl -X POST http://localhost:8787/forecast \
  -H "Content-Type: application/json" \
  -d '{"start_date": "2024-01-15T00:00:00Z", "end_date": "2024-01-22T00:00:00Z", "budget_type": "cpm", "budget": 1000, "cpm": 5, "publisher_id": 1}'
```