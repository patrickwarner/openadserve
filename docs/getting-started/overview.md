# Project Overview

A publisher-first ad server with OpenRTB compatibility and extensive customization capabilities.

## Architecture Overview

| Component | Technology | Purpose |
|-----------|------------|---------|
| **API Layer** | `internal/api` | REST handlers for ad requests and management |
| **Selection Logic** | `internal/logic` | Pluggable ad selection and filtering algorithms |
| **Data Layer** | `internal/db` | Database models and access patterns |
| **Configuration** | PostgreSQL | Campaign and line item storage |
| **Operational Data** | Redis | Real-time counters and rate limiting |
| **Analytics** | ClickHouse | Event tracking and reporting |

## Key Features

| Feature | Description |
|---------|-------------|
| **Publisher Control** | Pluggable ad selection logic, custom targeting, dual-counter pacing |
| **Native Ad Support** | Return raw assets for seamless publisher integration |
| **Quality Control** | Built-in ad reporting system for content moderation |
| **Analytics** | ClickHouse storage, Prometheus metrics, custom event tracking |
| **Integration** | JavaScript SDK, server-to-server API, traffic simulator |

## Educational Focus

This ad server demonstrates core advertising technology concepts through working code. The implementation covers:

**Ad Selection Logic**: Rule-based filtering with pluggable selector patterns  
**Campaign Pacing**: Dual-counter systems for budget and impression distribution  
**Real-Time Decisions**: Sub-100ms ad serving with caching and optimization  
**Data Architecture**: Multi-store patterns for configuration, counters, and analytics  
**Integration Patterns**: SDK design and server-to-server API structures  
**Quality Control**: User reporting and content moderation workflows

## Target Audience

| Audience | Use Case |
|----------|----------|
| **Students & Educators** | Learn advertising technology through working code and documentation |
| **Publishers** | Understand ad server internals and evaluate third-party alternatives |
| **Engineers** | Study complete ad serving implementation with modern Go practices |
| **Product Managers** | Gain technical depth on ad serving capabilities and trade-offs |

## Project Scope

**What this provides:**
- Educational platform for ad serving concepts
- Foundation for small to mid-sized publishers to build upon
- Starting point for custom ad selection algorithms  
- Research framework for ad serving behavior and integration experiments

**What this is NOT:**
- Enterprise ad server optimized for massive scale
- Complete solution for large publishers
- Replacement for existing monetization platforms

See [limitations.md](limitations.md) for detailed constraints and production considerations.

## Customizable Ad Selection

Implement the `selectors.Selector` interface to create custom ad selection logic. The default `RuleBasedSelector` handles standard targeting and pacing.

**Available Filters** (`internal/logic/filters`):
- `FilterByTargeting` - Device, geo, custom key-values
- `FilterBySize` - Creative dimensions and format compatibility  
- `FilterByActive` - Line item status
- `FilterByFrequency` - User exposure limits
- `FilterByPacing` - Daily delivery caps and dual-counter distribution

Compose filters to build sophisticated selection logic tailored to specific needs.
