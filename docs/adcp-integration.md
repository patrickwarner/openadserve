# AdCP Integration Guide

OpenAdServe implements the [Ad Context Protocol (AdCP)](https://adcontextprotocol.org) through a dedicated MCP (Model Context Protocol) server, enabling AI-powered advertising automation.

## Overview

The AdCP integration allows AI assistants to:
- Discover available advertising inventory using natural language queries
- Create and manage media buys programmatically
- Access real-time forecasting data for informed decision-making

This implementation focuses on the **Media Buy Protocol**, providing AI agents with the ability to discover, evaluate, and purchase advertising inventory.

## Architecture

```
AI Assistant (Claude, etc.)
    ↓ (MCP Protocol)
OpenAdServe MCP Server
    ↓ (Internal APIs)
┌─────────────────────────────────┐
│ OpenAdServe Core Components     │
├─────────────────────────────────┤
│ • Forecasting Engine           │
│ • Ad Data Store                 │
│ • Campaign Management          │
│ • Analytics & Reporting        │
└─────────────────────────────────┘
    ↓
┌─────────────────────────────────┐
│ Data Layer                      │
├─────────────────────────────────┤
│ • PostgreSQL (Campaigns/Config)│
│ • Redis (Operational Counters) │
│ • ClickHouse (Analytics Data)  │
└─────────────────────────────────┘
```

## Features

### Media Buy Protocol Implementation

#### 1. Product Discovery (`get_products`)
- Publisher-based inventory discovery
- Real-time forecasting using historical data
- Support for CPM, CPC, and flat-rate campaigns
- Query specific placements or all available inventory

#### 2. Campaign Creation (`create_media_buy`)
- Automated campaign and line item setup
- Input validation and error handling
- Integration with existing publisher workflows
- **Note**: Creative content must be added separately. The AdCP Creative Protocol (future implementation) will handle creative asset management. For now, use the admin UI to add creatives after campaign creation.

## Getting Started

### Prerequisites

- OpenAdServe running with Docker Compose
- PostgreSQL with campaign data
- ClickHouse with analytics events (for forecasting)
- AI assistant supporting MCP (Claude Desktop, etc.)

### Setup

1. **Start the MCP Server**:
   ```bash
   docker compose up -d mcp-server
   ```

2. **Configure Your AI Assistant**:
   Add to your Claude Desktop configuration:
   ```json
   {
     "mcpServers": {
       "openadserve": {
         "command": "/absolute/path/to/openadserve/mcp-server-wrapper.sh"
       }
     }
   }
   ```

3. **Populate Demo Data** (if needed):
   ```bash
   docker compose exec openadserve go run ./tools/fake_data
   ```

### Basic Usage

#### Discovering Available Inventory

"Show me available ad inventory for publisher 1 for the next 30 days"

The AI assistant calls `get_products` with publisher_id, date range, and budget parameters.

#### Creating a Campaign

"Create a $10,000 CPM campaign called 'Holiday Sale 2024' for the header placement"

The AI assistant calls `create_media_buy` with campaign details.

## API Reference

### get_products

**Parameters:**
- `publisher_id` (required): Integer ID of the publisher
- `start_date` (required): Campaign start date (ISO 8601)
- `end_date` (required): Campaign end date (ISO 8601)
- `min_budget` (optional): Minimum budget for forecasting (defaults to $1000)
- `budget_type` (optional): "cpm", "cpc", or "flat" (default: "cpm")
- `priority` (optional): Campaign priority level 1-10 (defaults to 5)
- `cpm` (optional): Cost per mille for CPM campaigns (defaults to $2.00)
- `cpc` (optional): Cost per click for CPC campaigns (defaults to $1.00)
- `placement_ids` (optional): Specific placement IDs to forecast

**Response:**
```json
{
  "products": [
    {
      "id": "placement_header",
      "placement_id": "header",
      "placement_name": "header",
      "publisher": "publisher_1",
      "format": "html",
      "width": 728,
      "height": 90,
      "available_impressions": 1250000,
      "min_cpm": 1.0,
      "estimated_ctr": 0.024
    }
  ]
}
```

### create_media_buy

**Parameters:**
- `name` (required): Campaign name
- `publisher_id` (required): Integer ID of the publisher
- `budget` (required): Campaign budget (must be > 0)
- `budget_type` (required): "cpm", "cpc", or "flat"
- `placement_id` (required): Target placement ID
- `cpm` (required for CPM campaigns): Cost per mille rate (e.g., 2.50 for $2.50 CPM)
- `cpc` (required for CPC campaigns): Cost per click rate (e.g., 1.00 for $1.00 CPC)

**Response:**
```json
{
  "campaign_id": 123,
  "status": "created",
  "message": "Successfully created campaign 'Holiday Sale 2024' (ID: 123). Note: Creative content must be added separately via the Creative Protocol (future implementation) or admin UI before the campaign can serve ads."
}
```

**Important**: The Media Buy Protocol creates the campaign structure (campaign, line item, targeting, and budget). Creative content (HTML, images, videos) must be added separately:
- **Future**: Use the AdCP Creative Protocol tasks like `build_creative` and `preview_creative`
- **Current**: Use the OpenAdServe admin UI at `/admin` to create and associate creatives with the line item

## Configuration

### Environment Variables

The MCP server uses the same configuration as OpenAdServe:

- `POSTGRES_DSN`: PostgreSQL connection string
- `REDIS_ADDR`: Redis server address  
- `CLICKHOUSE_DSN`: ClickHouse connection string
- `LOG_LEVEL`: Logging level (DEBUG, INFO, WARN, ERROR)

### Docker Compose Configuration

```yaml
mcp-server:
  build: .
  command: ["tail", "-f", "/dev/null"]
  depends_on:
    postgres:
      condition: service_started
    redis:
      condition: service_started
    clickhouse:
      condition: service_healthy
    openadserve:
      condition: service_started
  env_file:
    - .env
  stdin_open: true
  tty: true
```

## Troubleshooting

### Common Issues

1. **"MCP server container is not running"**
   - Run: `docker compose up -d`
   - Check: `docker compose ps mcp-server`

2. **"No products found"**
   - Verify publisher ID exists
   - Ensure placements are configured
   - Load demo data: `docker compose exec openadserve go run ./tools/fake_data`

3. **"Forecasting failed, using defaults"**
   - Verify ClickHouse is accessible
   - Check analytics events exist for time period
   - Review logs: `docker compose logs mcp-server`

### Debugging

Enable debug logging:
```bash
LOG_LEVEL=DEBUG
```

View logs:
```bash
docker compose logs -f mcp-server
```

Test directly:
```bash
./mcp-server-wrapper.sh
```