# Tools and Utilities

## Traffic Simulator

A helper in `tools/traffic_simulator` can generate random OpenRTB traffic for
load testing. Each ad request is assigned a unique request ID so that
impression events can be tied back to the originating request, mirroring the
behaviour of the JavaScript SDK. The simulator can run for a fixed number of
requests or for a specified duration. When both a request count and duration are
provided, the requests are evenly spaced across that time. Parameters such as
server URL, number of users, placements and concurrency can be overridden via
flags. A `-rate` option sends requests at a steady number per second. The
`-stats` flag prints aggregated results every few seconds while the script runs.
When the run completes,
totals for sent requests, impressions, clicks, no-bids and errors are printed.
Use `-click-rate` to control the probability that an impression generates a
click (default `0.05`). The `-debug` flag logs each request in detail. The
`-label` flag tags the output so multiple runs can be compared. The `-flush`
flag clears operational data from Redis before starting and `-redis` can point to a custom Redis
address when flushing.

### Traffic Surges and Chaos

To stress test pacing behaviour you can introduce short bursts of traffic or
randomized jitter in the request spacing. The following flags control this
behaviour:

- `-surge-interval` sets how often surge periods occur (e.g. `30s`).
- `-surge-duration` controls how long each surge lasts.
- `-surge-multiplier` multiplies the base request rate during a surge.
- `-jitter` applies a random ± percentage to each interval (e.g. `0.2` for 20%).

Surge and jitter settings are optional; leaving them at their defaults results
in steady traffic just like previous versions.

Example:

```bash
go run ./tools/traffic_simulator -server=http://localhost:8787 \
  -users=100 -placements=header,sidebar -requests=1000 -concurrency=20 \
  -api-key=demo123 \
  -stats -label=run1 -flush -click-rate=0.05

# run for 30 seconds instead of a fixed request count
go run ./tools/traffic_simulator -server=http://localhost:8787 \
  -users=100 -placements=header,content_rect -duration=30s -rate=5 \
  -api-key=demo123 -stats -label=experimentA -click-rate=0.05
```

## Analytics and Reporting

### Campaign Reports

Generate campaign performance reports:

```bash
go run ./tools/campaign_report -campaign-id=123 -days=30
```

Provides campaign metrics, daily breakdown, top creatives, and optimization insights.

### Querying Events

Analytics events can be inspected with the helper at `tools/query_events`.
Provide a request ID and it prints the matching records as JSON:

```bash
go run ./tools/query_events -id=req123
```

### Demo Data Generator

The `tools/fake_data` utility generates demo data for testing:

```bash
go run ./tools/fake_data
```

When the database is empty, this tool inserts a demo publisher with ID `1`
and API key `demo123` before creating the requested number of additional
publishers.
