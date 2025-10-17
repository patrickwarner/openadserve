# Configuration

This document explains how to configure the ad server, including environment variables, placements, creatives and campaigns. All entities are stored in PostgreSQL. The server creates the tables on startup and the `fake_data` tool loads demo data when run. When the database is empty it first adds a demo publisher (ID `1`, API key `demo123`).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| **Core Settings** | | |
| `PORT` | `8787` | HTTP server port |
| `READ_TIMEOUT` | `5s` | HTTP read timeout |
| `WRITE_TIMEOUT` | `10s` | HTTP write timeout |
| `REDIS_ADDR` | `localhost:6379` | Redis connection string for operational counters |
| `CLICKHOUSE_DSN` | `clickhouse://default:@localhost:9000/default?async_insert=1&wait_for_async_insert=1` | ClickHouse connection string with async inserts |
| `POSTGRES_DSN` | `postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable` | PostgreSQL connection string |
| `GEOIP_DB` | `data/GeoLite2-Country.mmdb` | GeoIP database path |
| `DEBUG_TRACE` | `false` | Enable debug tracing for ad selection |
| `ENV` | `production` | Environment type (development, staging, production) |
| `LOG_LEVEL` | *varies* | Override log level (DEBUG, INFO, WARN, ERROR) |
| `RELOAD_INTERVAL` | `30s` | Automatic reload interval for campaign data |
| **Distributed Tracing** | | |
| `TRACING_ENABLED` | `false` | Enable OpenTelemetry distributed tracing |
| `TEMPO_ENDPOINT` | `localhost:4317` | Tempo OTLP gRPC endpoint for trace export |
| `TRACING_SAMPLE_RATE` | `1.0` | Trace sampling rate (0.0 to 1.0) |
| **Authentication & Security** | | |
| `TOKEN_SECRET` | *required* | Random string for token signing |
| `TOKEN_TTL` | `30m` | Token expiration time |
| **CTR Estimation** | | |
| `DEFAULT_CTR` | `0.5` | Baseline CTR for new CPC line items |
| `CTR_WEIGHT` | `2.0` | Smoothing weight for CTR calculations |
| **Rate Limiting** | | |
| `RATE_LIMIT_ENABLED` | `true` | Enable/disable rate limiting |
| `RATE_LIMIT_CAPACITY` | `100` | Token bucket capacity (burst size) |
| `RATE_LIMIT_REFILL_RATE` | `10` | Tokens added per second |
| **CTR Optimization** | | |
| `CTR_OPTIMIZATION_ENABLED` | `false` | Enable ML CTR optimization for CPC line items |
| `CTR_PREDICTOR_URL` | `http://localhost:8000` | URL of the CTR prediction service |
| `CTR_PREDICTOR_TIMEOUT` | `100ms` | Timeout for CTR prediction requests |
| `CTR_PREDICTOR_CACHE_TTL` | `5m` | Cache TTL for CTR predictions |
| `PROGRAMMATIC_BID_TIMEOUT` | `800ms` | Timeout for external programmatic bid requests |

## Placements

Placements describe the ad slots available on your site.

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique identifier referenced by SDK and API |
| `publisher_id` | int | Publisher ID this placement belongs to |
| `width` | int | Default width in pixels (can be overridden) |
| `height` | int | Default height in pixels (can be overridden) |
| `formats` | array | Allowed creative formats: `html`, `native` |

Example:
```json
{"id": "header", "publisher_id": 1, "width": 320, "height": 50, "formats": ["html"]}
```

## Creatives

Creatives are the actual ads that can serve in a placement.

| Field | Type | Description |
|-------|------|-------------|
| `id` | int | Creative identifier |
| `placement_id` | string | Placement this creative belongs to |
| `line_item_id` | int | Line item controlling delivery rules |
| `campaign_id` | int | Campaign for reporting |
| `publisher_id` | int | Publisher ID |
| `html` | string | Markup for HTML creatives |
| `native` | object | Asset object for native creatives |
| `width` | int | Creative width (must match placement) |
| `height` | int | Creative height (must match placement) |
| `format` | string | Creative format: `html` or `native` |

Example:
```json
{"id": 1, "placement_id": "header", "line_item_id": 101, "campaign_id": 101, "html": "<div>Ad</div>", "width": 320, "height": 50, "format": "html"}
```

## Campaigns and Line Items

Campaigns serve as lightweight containers for reporting purposes. The core of delivery control, targeting, and budgeting lies within **line items**.

### Line Item Fields

| Field | Type | Description |
|-------|------|-------------|
| `ID` | int64 | Unique identifier for the line item |
| `CampaignID` | int64 | Identifier of the parent campaign |
| `PublisherID` | int64 | Identifier of the publisher |
| `Name` | string | Human-readable name for identification |
| `StartDate` / `EndDate` | time.Time | Flight period for line item activity |
| `DailyImpressionCap` | int | Max impressions per day (0 = no cap) |
| `DailyClickCap` | int | Max clicks per day (0 = no cap) |
| `CPM` | float64 | Bid price per thousand impressions |
| `CPC` | float64 | Bid price per click |
| `ECPM` | float64 | Effective CPM for auction ranking |
| `BudgetType` | enum | Spending model: `cpm`, `cpc`, or `flat` |
| `BudgetAmount` | float64 | Total monetary budget for line item |
| `Spend` | float64 | Currently accumulated spend |
| `PaceType` | enum | Delivery pacing: `asap`, `even`, or `pid` |
| `Priority` | enum | Publisher-defined priority level |
| `FrequencyCap` | int | Max impressions per user in window |
| `FrequencyWindow` | duration | Time window for frequency capping |
| `Country` | string | ISO 3166-1 alpha-2 country code |
| `Region` | string | State/province code for targeting |
| `DeviceType` | string | Device targeting: mobile, desktop, tablet |
| `OS` | string | Operating system targeting |
| `Browser` | string | Browser targeting |
| `KeyValues` | map | Custom key-value pairs for targeting |
| `Type` | enum | Line item type: `direct` or `programmatic` |
| `Endpoint` | string | URL for programmatic bid requests |
| `Active` | bool | Whether line item is enabled |

### Budget Types
- **`cpm`**: Cost Per Mille (thousand impressions). Spend accrued per impression.
- **`cpc`**: Cost Per Click. eCPM calculated from CPC bid and estimated CTR. Spend accrued per click.
- **`flat`**: Fixed budget for sponsorships or fixed-price deals.

### Pacing Types
- **`asap`**: Deliver impressions as quickly as possible with hard cap enforcement.
- **`even`**: Spread impressions evenly throughout the day.
- **`pid`**: Use PID controller for dynamic pacing with safety checks.

### CTR Calculation

For CPC line items, CTR is estimated using smoothed values to prevent zero eCPM:
```
CTR = (clicks + DEFAULT_CTR × CTR_WEIGHT) / (impressions + CTR_WEIGHT)
eCPM = CPC × CTR × 1000
```

See CTR Estimation environment variables above.

## Additional Configuration

### Token Security
Set `TOKEN_SECRET` to enable token signing for impression and click tracking. Tokens expire after `TOKEN_TTL` (default 30m).

### Programmatic Line Items
Programmatic line items call their `Endpoint` to retrieve OpenRTB bids. See [Programmatic Demand](programmatic.md). Use `/test/bid` for development testing.

### Rate Limiting
Built-in rate limiting protects against request floods. See [Rate Limiting Documentation](rate_limiting.md) for details.
