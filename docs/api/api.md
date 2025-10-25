# API Reference

This document describes the HTTP endpoints provided by the ad server. Payloads follow a simplified OpenRTB format using JSON.

## Endpoints

| Method | Endpoint | Description | Authentication |
|--------|----------|-------------|----------------|
| `POST` | `/ad` | Request ad for placement | API Key required |
| `GET` | `/impression` | Record impression event | Token required |
| `GET` | `/click` | Record click event | Token required |
| `GET` | `/event` | Record custom event | Token required |
| `POST` | `/report` | Submit ad quality report | Token required |
| `POST` | `/test/bid` | Mock programmatic bidder | None |
| `POST` | `/reload` | Reload campaign data | None |
| `GET` | `/health` | Health check | None |
| `GET` | `/metrics` | Prometheus metrics | None |

## `POST /ad`

Request an ad for a placement. Include the publisher's API key in the `X-API-Key` header.

### Request Parameters

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique request identifier |
| `imp` | array | Yes | Array of impression objects |
| `imp[].id` | string | Yes | Unique identifier for impression |
| `imp[].tagid` | string | Yes | Placement ID from configuration |
| `imp[].w` | int | No | Override placement width |
| `imp[].h` | int | No | Override placement height |
| `user.id` | string | Yes | User identifier |
| `device.ua` | string | No | User-agent string |
| `device.ip` | string | No | Client IP address |
| `ext.publisher_id` | int | Yes | Publisher context ID |
| `ext.kv` | object | No | Custom targeting key-value pairs |

**Headers:**
- `X-API-Key`: Publisher API key (required)

If no IP is supplied (neither in `device.ip` nor via `X-Forwarded-For`), the server falls back to the remote address of the HTTP connection.
This fallback mechanism ensures geo-targeting can still function, though direct IP provision is recommended for accuracy.

**Debug Mode:** Add `?debug=1` to the request URL or set `DEBUG_TRACE=1` environment variable to include detailed selection trace in the response.

### Response Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Request ID from request |
| `seatbid` | array | Array of seat bid objects |
| `seatbid[].bid` | array | Array of bid objects |
| `seatbid[].bid[].id` | string | Bid identifier |
| `seatbid[].bid[].impid` | string | Impression ID from request |
| `seatbid[].bid[].crid` | string | Creative ID |
| `seatbid[].bid[].cid` | string | Campaign ID |
| `seatbid[].bid[].adm` | string | Ad markup (HTML for standard/banner ads, JSON for native) |
| `seatbid[].bid[].price` | float | Line item eCPM value |
| `seatbid[].bid[].impurl` | string | Impression tracking URL with token |
| `seatbid[].bid[].clkurl` | string | Click tracking URL with token |
| `seatbid[].bid[].evturl` | string | Event tracking URL with token |
| `nbr` | int | No-bid reason code (when no ads available) |

### Example Request/Response

```json
// Request
{
  "id": "req123",
  "imp": [{ "id": "1", "tagid": "header", "w": 320, "h": 50 }],
  "user": { "id": "user1" },
  "device": { "ua": "Mozilla/5.0", "ip": "203.0.113.10" },
  "ext": { "publisher_id": 1, "kv": { "category": "sports" } }
}

// Response
{
  "id": "req123",
  "seatbid": [{
    "bid": [{
      "id": "1", "impid": "1", "crid": "3", "cid": "101",
      "adm": "<div>...</div>", "price": 1.25,
      "impurl": "/impression?t=...", "clkurl": "/click?t=...", "evturl": "/event?t=..."
    }]
  }]
}
```

**Notes:**
- Empty `seatbid` array indicates no matching ads
- Tracking URLs contain pre-signed tokens (expire after 30 minutes)
- Creative formats:
  - **HTML**: Custom ad markup provided by advertiser (returned in `adm` field)
  - **Banner**: Image-based ads with JSON asset definition, server-side composed into HTML with responsive srcset support (returned as HTML in `adm` field)
  - **Native**: Flexible JSON assets for publisher-controlled rendering (returned as JSON in `adm` field)

## `GET /impression`

Record an impression by requesting the `impurl` from the bid response. This updates the impression counter in Redis for accurate billing (separate from the serve counter that was incremented during ad selection for pacing decisions). A transparent 1×1 GIF is returned.

## `GET /click`

Request the `clkurl` from the bid response to record a click. Returns a 1×1 GIF.

## `GET /event`

Track custom engagement events using the `evturl` from the bid response.

### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `t` | string | Yes | Signed token from bid response |
| `type` | string | Yes | Event type (must be configured) |

### Example

```
GET /event?t=TOKEN_FROM_BID_RESPONSE&type=like
```

Returns 1×1 GIF on success. See [configured custom events](events.md) for allowed event types.

## `POST /report`

Submit a user ad report.

### Request Parameters

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `token` | string | Yes | Tracking token from bid response |
| `reason` | string | Yes | Report reason code |

**Allowed reason codes:** `offensive`, `misleading`, `malware`, `irrelevant`, `other`

### Example

```json
{
  "token": "TOKEN_VALUE",
  "reason": "offensive"
}
```

Returns HTTP `201 Created` on success.

## `POST /test/bid`

Test endpoint that mimics a programmatic bidder. It ignores the request body and always
responds with a winning bid priced at `1.75` and simple HTML markup. Programmatic line
items can use `http://localhost:8787/test/bid` as their `Endpoint` during development.
See [Programmatic Demand](programmatic.md) for how these bids are incorporated during
ad selection.

## `POST /reload`

Reload campaigns, line items and creatives from Postgres at runtime. Invoke this after
inserting data (e.g., using the fake data script) to refresh the server without a restart. A successful reload returns
HTTP `204 No Content`.

## `GET /health`

Simple endpoint for uptime checks. Returns:

```json
{"status": "ok"}
```


## `GET /metrics`

Prometheus metrics endpoint. See included Grafana dashboards in `deploy/grafana/dashboards` for visualization.
