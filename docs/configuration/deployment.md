# Deployment Guide

The project can run locally using Docker Compose or it can be launched manually with dependencies installed separately.

## Docker Compose

The repository includes a `docker-compose.yml` file that starts the server along with Redis, Postgres, ClickHouse, Prometheus, Loki, Promtail, Grafana, Tempo, and optionally the CTR predictor service.

```bash
docker compose up
```

- Grafana is available at `http://localhost:3000` (username `admin`, password `admin`).
- The server listens on `http://localhost:8787` by default.
- A `prebid-server` container is exposed at `http://localhost:8060` when using the `prebid` profile. See [Programmatic Demand](docs/programmatic.md) for configuration guidance.
- Logs are collected by Promtail and stored in Loki. You can explore them in Grafana's **Explore** section.
- Distributed tracing is enabled by default with traces sent to Tempo. Use Grafana's **Explore** section to query traces and correlate them with logs.

The `data/GeoLite2-Country.mmdb` file provides GeoIP lookup out of the box. The server creates the tables on startup. Run the `fake_data` tool from inside the `openadserve` container to load demo data:

```bash
docker compose exec openadserve go run ./tools/fake_data
```

When the database is empty this command inserts a demo publisher (ID `1`, API key `demo123`) before generating additional publishers.

## Using Distributed Tracing

The ad server includes comprehensive distributed tracing with OpenTelemetry and Grafana Tempo integration:

### Finding and Using Trace IDs

**Trace IDs are automatically included in all application logs**:
```json
{
  "level": "error",
  "trace_id": "172ed24edd575ad8999472a68e6cfb72",
  "span_id": "a83dd8775de972b9", 
  "msg": "invalid api key"
}
```

**To trace a request**:
1. Find any log entry related to your request (error logs, info logs, etc.)
2. Copy the `trace_id` value
3. In Grafana, go to **Explore** → Select **Tempo** data source
4. Paste the trace ID and click **Run Query**

### TraceQL Queries

Use TraceQL to find traces by attributes:
```
// Find all traces for the ad server
{service.name="openadserve"}

// Find traces with errors
{service.name="openadserve" && status=error}

// Find ad request traces by publisher
{service.name="openadserve" && publisher_id="1"}
```

### Trace-to-Logs Correlation

The Grafana setup includes automatic correlation between traces and logs:
- Click on any span in a trace view
- Select "Logs for this span" to see all related log entries
- All logs within a request automatically include the same trace ID

This enables seamless debugging: start with a trace to see the request flow, then drill down to specific log entries for detailed error information.

## Manual Setup

1. **Start Redis** (set `REDIS_ADDR` if using an existing instance):

   ```bash
   docker run -p 6379:6379 redis
   ```

2. **Start ClickHouse** (set `CLICKHOUSE_DSN` for your instance):

   ```bash
   docker run -p 9000:9000 clickhouse/clickhouse-server:latest
   ```

3. **Start Postgres** (set `POSTGRES_DSN` for your instance):

   ```bash
   docker run -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:15
   # default DSN assumes 127.0.0.1
   export POSTGRES_DSN=postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable
   ```

4. **(Optional) GeoIP** – set `GEOIP_DB` to a MaxMind GeoLite2 database. The repo ships with a country database in `data/GeoLite2-Country.mmdb`, but you can supply a City database for region targeting.
5. **Generate demo data**:

   ```bash
   go run ./tools/fake_data
   ```

   The first run on an empty database creates a demo publisher with ID `1` and API key `demo123` before adding more publishers.

6. **Run the server**:

   ```bash
   go run cmd/server/main.go
   ```

Set the `PORT` environment variable to change the listen port. Enable `DEBUG_TRACE=1` to always include selection traces in ad responses for troubleshooting. 

For logging configuration, set `ENV=development` for full debug logging, or `ENV=production` for reduced log volume via sampling. Use `LOG_LEVEL` to override the default log level.

Use `RELOAD_INTERVAL` to automatically reload campaigns, line items and creatives from Postgres. The default interval is `30s`; set it to `0` to disable automatic reloading.
