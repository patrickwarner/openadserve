# Known Limitations

This document outlines the current limitations of this ad server implementation. 

## Critical Limitations

### Horizontal Scaling Limitations
- **Single-Instance Architecture**: 
  - All campaign/line item data loaded into memory of single server instance
  - No distributed data store for campaign configuration
  - Redis used only for operational counters (frequency caps, pacing)
  - Impact: Cannot scale beyond single server for campaign data
- **Memory constraints**: Campaign data limited by available server RAM
- **No distributed state**: Multiple instances would serve inconsistent campaigns

### Security & Production Readiness
- **Limited authentication framework**: HMAC token system provides basic security but lacks full OAuth/JWT framework
- **No audit logging**: Changes to campaigns/line items are not tracked
- **Unauthenticated admin endpoints**: CRUD operations have no access control
- **No user management**: No roles, permissions, or multi-user support

## Performance & Scalability Limitations

### Request Processing
- **Synchronous ad selection**: No async processing, blocks on database lookups
- **Creative scanning**: O(1) placement lookup but O(n) filtering within placements
- **Memory allocation**: Heavy use of slices and maps without pooling

### Database Performance
- **Campaign data reloads**:
  - Full table scans: Campaign reloads fetch all records (no incremental updates)
  - Blocking reloads: 30-second reload cycles can block request processing
  - Manual reload required: Configuration changes need `/reload` endpoint call

### External Dependencies
- **ClickHouse bottleneck**: Analytics writes not buffered, can block serving
- **CTR prediction timeout**: 100ms timeout may be too aggressive under load
- **Prebid timeout constraints**: 800ms programmatic bid timeout may cause timeouts

## Technical Debt & Architecture

- **Tight coupling**: Database, analytics, and serving logic are interdependent
- **Error handling**: Many error scenarios result in 500s rather than graceful degradation
- **Limited targeting**: Geo-targeting limited to country/region (no city/DMA)
- **No user segments**: Cannot target based on user behavioral data
- **No alerting**: No built-in alerting for system health or performance issues
- **Basic log sampling**: Fixed environment-based sampling may not suit all traffic patterns

## Management & Operations

- **Basic admin UI only**: Includes development admin interface but not suitable for production use
- **No campaign workflow**: No approval process, creative review, or campaign lifecycle management
- **Limited reporting**: No real-time dashboards or custom report generation
- **No billing system**: No invoicing, payment processing, or financial reporting
- **Manual operations**: No backup/restore, disaster recovery, or automated maintenance
- **No publisher onboarding**: No self-service registration or account management

## Feature Gaps vs. Commercial Ad Servers

### Advanced Targeting
- **No audience targeting**: Cannot target based on user demographics or interests
- **Limited frequency capping**: Simple impression-based only, no time-window sophistication
- **Missing brand safety**: No content categorization or blocking capabilities

### Ad Format Limitations
- **Basic format support**: Supports HTML, banner (responsive images), and native formats (no video, audio, interactive)
- **Simple native ads**: Basic JSON structure without standardized templates
- **No creative validation**: No automated scanning for malicious or policy-violating content

### Campaign Management
- **Limited budget pacing**: Line items have dual-counter pacing system but use simpler algorithms than commercial systems
- **Manual optimization**: No automatic bid adjustments or optimization

### Data & Privacy
- **No consent management**: Missing GDPR/CCPA compliance features
- **Limited data protection**: No PII anonymization or data retention policies
- **No identity resolution**: Cannot link users across devices or sessions

## Scale Limitations

| Component | Limitation | Impact |
|-----------|------------|---------|
| **Throughput** | ~2,000-3,000 QPS max | Single instance, memory-bound |
| **Scaling** | Vertical only | Cannot distribute campaign data horizontally |
| **Geographic** | Single-region deployment | No multi-region support |
| **Creative Assets** | No CDN integration | Limited to small files served locally |
| **PostgreSQL** | Single instance only | Database limited to vertical scaling |
| **ClickHouse** | Single node only | Analytics capacity tied to one machine |
| **Redis** | Memory-bounded | Operational data limited by available RAM |
| **Logs** | No rotation/archival | Manual log management required |

## Strengths Despite Limitations

While this document focuses on limitations, this ad server provides:
- **Clean, readable codebase** for learning and customization
- **Publisher-first design** with flexibility prioritized over scale
- **Comprehensive documentation** with working examples
- **Modern observability** using Prometheus, Grafana, and OpenTelemetry
- **Real ad tech concepts** correctly implemented
- **Extensible architecture** allowing gradual enhancement

This implementation serves as an educational resource and foundation for small to medium-scale ad serving needs.

## Need More?

Planning to run this at serious scale or need features beyond what's included? Feel free to reach out—I'm happy to discuss custom solutions, hosting options, or enhancements that fit your specific needs. Contact Patrick Warner via [email](mailto:patrick@openadserve.com) or [LinkedIn](https://www.linkedin.com/in/warnerpatrick).
