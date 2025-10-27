# Data Store Architecture

This ad server uses a single-instance, in-memory data store architecture optimized for performance.

## Architecture Overview

**Campaign Data Storage:**
- PostgreSQL: Source of truth for campaigns, line items, placements
- In-memory cache: All campaign data loaded at startup for ultra-fast lookups
- Automatic reload: Configurable interval to sync from PostgreSQL

**Operational Data Storage:**
- Redis: Real-time counters (frequency caps, pacing, rate limiting)
- ClickHouse: Analytics events and historical data

## Performance Characteristics

| Aspect | Implementation |
|--------|----------------|
| **Ad Selection** | In-memory lookups (sub-millisecond) |
| **Frequency Caps** | Redis counters (1-2ms) |
| **Pacing Control** | Redis state tracking |
| **Analytics** | Async ClickHouse writes |

## Configuration

```bash
# Required connections
POSTGRES_DSN=postgres://user:pass@host:5432/db
REDIS_ADDR=redis:6379
CLICKHOUSE_DSN=clickhouse://host:9000/db

# Data reload settings
RELOAD_INTERVAL=30s  # Auto-reload campaign data from PostgreSQL
```

## Scaling Considerations

**Single-Instance Design:**
- All campaign data fits in memory (optimized for most use cases)
- Redis provides operational state persistence across restarts
- Horizontal scaling not supported in this open source version

**Need to Scale?**
Planning a larger deployment? Reach out to discuss distributed data stores, multi-instance setups, and real-time config sync. Contact Patrick Warner via [email](mailto:patrick@openadserve.com) or [LinkedIn](https://www.linkedin.com/in/warnerpatrick).