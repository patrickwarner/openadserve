# Quick Start

## Docker Compose

The easiest way to run the server and its dependencies is with Docker Compose:

1. First, copy the environment configuration:
```bash
cp .env.example .env
```

2. Start the services:
```bash
docker compose up
```

This launches the ad server, Redis, Postgres, ClickHouse, Prometheus, Grafana, Tempo, and optionally the CTR predictor service.
After the containers are running, populate the database with demo data from inside the `openadserve` container:

```bash
docker compose exec openadserve go run ./tools/fake_data
```


When the database is empty this command first inserts a demo publisher with ID `1` and API key `demo123`, then generates any additional publishers you request via the `--publishers` flag.

The stack uses the bundled `data/GeoLite2-Country.mmdb` for GeoIP lookup.
Grafana is available at `http://localhost:3000` (default credentials `admin/admin`). Logs from the containers are forwarded to Loki and can be viewed from Grafana's **Explore** tab. Distributed tracing is enabled by default with traces sent to Tempo - use Grafana's **Explore** section to query traces and correlate them with logs.

### Optional: Prebid Server
Start the stack with `docker compose --profile prebid up` to launch a
[`prebid-server`](https://prebid.org/) container on `http://localhost:8060`. You
can then configure a programmatic line item pointing at
`http://prebid-server:8000/openrtb2/auction`. See [Programmatic Demand](docs/programmatic.md)
for setup details.

### Container Time Sync
The services rely on accurate timestamps for metrics and logs. The `docker-compose.yml` mounts `/etc/timezone` and `/etc/localtime` into each container so they inherit the host clock. If logs show wrong times, ensure the host clock is correct and recreate the stack:

```bash
docker compose down
docker compose up -d
```

## Manual Setup

1. Start Redis (set `REDIS_ADDR` if using a different instance):

   ```bash
   docker run -p 6379:6379 redis
   ```

2. Start ClickHouse (set `CLICKHOUSE_DSN` for your instance):

   ```bash
   docker run -p 9000:9000 clickhouse/clickhouse-server:latest
   ```

3. Start Postgres (set `POSTGRES_DSN` for your instance):

   ```bash
   docker run -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15
   # default DSN assumes 127.0.0.1
   export POSTGRES_DSN=postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable
   ```

4. (Optional) set `GEOIP_DB` to a MaxMind GeoLite2-Country database. The repo
   includes one at `data/GeoLite2-Country.mmdb`.

5. Generate demo data:

   ```bash
   go run ./tools/fake_data
   ```

   When the database is empty, this tool inserts a demo publisher with ID `1`
   and API key `demo123` before creating the requested number of additional
   publishers.

6. Run the server:

   ```bash
   go run cmd/server/main.go
   ```

By default the server listens on `http://localhost:8787`. Set the `PORT` environment variable to change the port.

Copy the provided `.env.example` to `.env` and adjust values as needed. The `PRIORITY_ORDER` entry demonstrates how to customise the ranking of line item priorities. Set `DEBUG_TRACE=1` to return selection traces with each ad response. Use `RELOAD_INTERVAL` to periodically refresh campaigns, line items and creatives from the database. The default interval is `30s` and setting it to `0` disables automatic reloads. `DEFAULT_CTR` and `CTR_WEIGHT` control the baseline click-through rate used for CPC budgets.

### CTR Optimization Setup (Optional)

To enable machine learning CTR optimization for CPC line items:

1. Set `CTR_OPTIMIZATION_ENABLED=true` in your environment
2. The CTR predictor service will start automatically with Docker Compose
3. Bootstrap the model with synthetic data:
   ```bash
   docker compose exec ctr-predictor python bootstrap.py
   ```

See [CTR Optimization Documentation](ctr_optimization.md) for detailed configuration and usage.

## API Overview

`POST /ad` accepts a minimal OpenRTB payload and returns a bid with ad markup when one is available. `POST /impression` records an impression for analytics.

See the demo pages for working examples that use the JavaScript SDK:
- **Basic Demo**: `http://localhost:8787/static/demo/index.html` - Standard HTML ad placements
- **Native Ad Demo**: `http://localhost:8787/static/demo/social-feed.html` - Advanced native ad integration with in-feed ads and background skin takeover

Metrics are exposed at `http://localhost:8787/metrics` and visualised in the included Grafana dashboards (`deploy/grafana/dashboards`).
For basic uptime checks, `GET /health` returns a simple JSON status.

## Logging & Tracing

The server uses [zap](https://github.com/uber-go/zap) for structured JSON logs with automatic distributed tracing integration.
**All logs automatically include trace IDs and span IDs** when distributed tracing is enabled, providing seamless correlation between logs and traces.

### Log Configuration

- **ENV**: Controls log level and sampling rates
  - `development`: DEBUG level, 100% sampling (all logs)
  - `staging`: INFO level, 50% sampling
  - `production`: INFO level, 10% sampling (reduces volume by 90%)
- **LOG_LEVEL**: Override default log level (DEBUG, INFO, WARN, ERROR)

### Distributed Tracing

- **TRACING_ENABLED**: Enable OpenTelemetry tracing (default: `true` in Docker)
- **TEMPO_ENDPOINT**: Tempo endpoint for traces (default: `tempo:4317`)
- **TRACING_SAMPLE_RATE**: Sampling rate 0.0-1.0 (default: `1.0`)

Each request gets a unique trace ID that flows through all operations. Logs automatically include trace context:
```json
{
  "level": "info",
  "trace_id": "2fc1f011c62064ac5bfc941cf12e33cc",
  "span_id": "509903a6983d2241",
  "msg": "ad request",
  "request_id": "test-456"
}
```

High-volume operations (ad requests, impressions, clicks) use sampling to prevent log floods in production while maintaining full debugging in development.

When using Docker Compose, Promtail ships these logs to Loki and traces are sent to Tempo. Both can be queried in Grafana's **Explore** section with automatic trace-to-logs correlation.
