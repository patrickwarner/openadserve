# OpenAdServe

A lightweight, single-instance ad server with **pluggable ad selection logic** and **publisher-first design**. Built to educate and demonstrate core ad serving concepts including targeting, pacing, and programmatic integration. Supports custom targeting, frequency capping, CTR optimization, and optional header bidding via Prebid Server.

---

## Key Features

- **Pluggable Ad Selection**: Customize or replace ad selection algorithms
- **Inventory Forecasting**: Predict available inventory and detect line item conflicts
- **Custom Targeting**: Device/OS/browser detection, GeoIP, custom key-values
- **Campaign Management**: Line items, placements, frequency capping, dual-counter pacing system
- **CTR Optimization**: Optional ML predictor for context-aware CPC optimization
- **Header Bidding**: Optional Prebid Server integration for programmatic demand
- **Ad Quality Control**: User reporting system for publisher content control
- **AI-Powered Automation**: AdCP (Ad Context Protocol) support for natural language advertising workflows
- **Analytics**: ClickHouse storage, Prometheus metrics, Grafana dashboards
- **Integration**: JavaScript SDK, server-to-server API, traffic simulator

---

## Quick Start

Get started quickly using Docker Compose:

```bash
# Basic ad server setup
docker compose up
docker compose exec openadserve go run ./tools/fake_data

# Or with optional Prebid Server for header bidding demo
docker compose --profile prebid up
docker compose exec openadserve go run ./tools/fake_data
```

This launches the ad server with all dependencies and populates demo data. Services will be available at:
- Ad Server: `http://localhost:8787`
- Grafana: `http://localhost:3000`
- Prebid Server (optional): `http://localhost:8060`


For detailed setup instructions, see the [Quick Start Guide](docs/getting-started/quickstart.md).


---

## Documentation

Complete documentation is available in the [docs](docs/) folder:

**Getting Started:**
- [Quick Start Guide](docs/getting-started/quickstart.md) - Detailed setup instructions
- [Project Overview](docs/getting-started/overview.md) - High-level project goals and feature overview
- [Systems Architecture](docs/getting-started/architecture.md) - Technical architecture and component design
- [Limitations](docs/getting-started/limitations.md) - Known constraints and production considerations

**API & Integration:**
- [API Reference](docs/api/api.md) - Complete REST API endpoint documentation
- [Integration Guide](docs/api/integration.md) - JavaScript SDK and server-to-server integration
- [Ad Reporting](docs/api/ad_reporting.md) - User reporting system for ad quality control

**Architecture:**
- [Ad Decisioning Algorithm](docs/architecture/ad_decisioning.md) - How the system selects winning creatives
- [Data Store Configuration](docs/architecture/data_stores.md) - In-memory vs Redis data store setup
- [Pacing System](docs/architecture/pacing-system.md) - Budget and impression pacing algorithms
- [Rate Limiting](docs/architecture/rate_limiting.md) - Request throttling and protection
- [Multi-Tenancy](docs/architecture/multi_tenancy.md) - Multi-publisher configuration

**Features:**
- [AdCP Integration](docs/adcp-integration.md) - AI-powered advertising automation via Ad Context Protocol
- [Analytics and Reporting](docs/features/analytics.md) - ClickHouse integration and metrics
- [CTR Optimization](docs/features/ctr_optimization.md) - Machine learning CTR prediction
- [Click URLs](docs/features/click_urls.md) - Click URL management and macro expansion
- [Custom Events](docs/features/events.md) - Custom event tracking
- [Inventory Forecasting](docs/features/forecasting.md) - Predict available inventory
- [Programmatic Demand](docs/features/programmatic.md) - Header bidding and Prebid Server
- [Synthetic Data](docs/features/synthetic_data.md) - Test data generation for CTR optimization

**Configuration & Operations:**
- [Configuration Guide](docs/configuration/configuration.md) - Environment variables and setup
- [Deployment Guide](docs/configuration/deployment.md) - Production deployment
- [Tools and Utilities](docs/configuration/tools.md) - Traffic simulator and analytics tools
- [Development Guide](docs/operations/development.md) - Testing and customization
- [Trafficking](docs/operations/trafficking.md) - Campaign and line item management

---

## Known Limitations

This is a single-instance ad server designed for learning and small to medium-scale deployments. Horizontal scaling requires enterprise solutions. For production scaling considerations, see [limitations.md](docs/getting-started/limitations.md) for detailed constraints.

---

## Author

**Patrick Warner** - Technical and product leader passionate about building ad tech that actually works for publishers. 

- LinkedIn: [linkedin.com/in/warnerpatrick](https://www.linkedin.com/in/warnerpatrick)

---

## License

This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

### Third-Party Components

This project includes third-party components with their own licenses:

- **GeoLite2 Database**: Created by MaxMind, licensed under Creative Commons Attribution-ShareAlike 4.0 International License
- **Prebid Server** (optional): Apache License 2.0
- **Go Dependencies**: Various licenses (MIT, Apache 2.0, BSD, etc.)

See [NOTICE](NOTICE) file for complete attribution and license information.
