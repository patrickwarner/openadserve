# Forecasting Integration Tests

This directory contains integration tests for the forecasting engine.

## Test Scripts

### test_forecast.py

Tests the `/forecast` API endpoint with various scenarios and validates responses.

**Prerequisites:**
- Ad server running on `localhost:8787`
- Python 3.x with `requests` library
- Historical traffic data (generate with traffic simulator)

**Setup:**

1. Start the ad server:
   ```bash
   docker compose up
   ```

2. Generate test data:
   ```bash
   docker compose exec openadserve go run ./tools/fake_data
   docker compose exec openadserve go run ./tools/traffic_simulator
   ```

3. Install Python dependencies:
   ```bash
   pip install requests
   ```

**Usage:**

```bash
# Run the forecast integration test
python tests/integration/forecasting/test_forecast.py
```

**Sample Request:**
```json
{
  "start_date": "2024-01-15T00:00:00Z",
  "end_date": "2024-01-22T00:00:00Z",
  "budget_type": "cpm",
  "budget": 1000.0,
  "cpm": 5.0,
  "publisher_id": 1,
  "countries": ["US"],
  "device_types": ["mobile", "desktop"],
  "key_values": {
    "category": "sports"
  }
}
```

**Expected Response Fields:**
- `total_opportunities`: Total ad requests matching targeting
- `available_impressions`: Unfilled inventory available  
- `estimated_impressions`: Projected impressions for this line item
- `fill_rate`: Current fill rate for matching traffic
- `conflicts`: Array of competing line items
- `daily_forecast`: Daily breakdown of projections
- `warnings`: Advisory messages about data quality

The test validates the API response structure and provides meaningful output for debugging forecasting accuracy.