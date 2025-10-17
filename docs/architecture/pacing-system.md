# Dual-Counter Pacing System

The ad server implements a dual-counter pacing system that separates immediate serve counting (for pacing decisions) from delayed impression counting (for billing accuracy). This system solves the fundamental problem of delayed impression pixel feedback that can cause pacing algorithms to over-deliver.

## Overview

Impression pixels experience delays after ad serving due to network latency, browser behavior, user navigation, and ad blockers. When pacing algorithms rely solely on delayed impression feedback, it can lead to over-delivery against daily caps.

## Architecture

### Dual Counter System

The system maintains two separate Redis counters for each line item:

1. **Serve Counter** (`pacing:serves:{lineItemID}:{date}`)
   - Incremented immediately when an ad is selected/served
   - Used for pacing eligibility decisions
   - Provides real-time feedback for pacing algorithms
   - Incremented in `internal/api/ad.go` during ad selection

2. **Impression Counter** (`pacing:impressions:{lineItemID}:{date}`)
   - Incremented when impression pixels fire
   - Used for billing and reporting accuracy
   - Incremented in `internal/api/impression.go` on pixel requests
   - Represents actual user impressions

### Code Implementation

#### Key Functions

- `IncrementLineItemServes()` - Increments serve counter immediately
- `IncrementLineItemImpressions()` - Increments impression counter on pixel fire
- `checkPIDPacing()` - Shared PID controller logic with hard safety checks
- `IsLineItemPacingEligible()` - Uses serve counter for eligibility decisions

#### Redis Key Structure

```
pacing:serves:{lineItemID}:{YYYY-MM-DD}        # Serve counter
pacing:impressions:{lineItemID}:{YYYY-MM-DD}   # Impression counter
```

Both keys have 24-hour TTL for automatic cleanup.

## Pacing Strategies

| Strategy | Method | Formula/Logic |
|----------|--------|---------------|
| **ASAP** | Hard cap enforcement | `serveCount >= dailyImpressionCap` |
| **Even** | Smooth distribution | `allowedImpressions = dailyCap × (elapsedTime / 24hours)` |
| **PID** | Proportional-integral-derivative controller | `target = dailyCap × (elapsedTime / 24hours)` with kp=0.6, ki=0.2, kd=0.2 |

## Benefits

- **Prevents over-delivery**: All pacing strategies stay within daily caps
- **Real-time decisions**: Pacing based on immediate serve counts
- **Billing accuracy**: Impression counts reflect actual user views
- **Hard caps**: Never exceed daily limits regardless of algorithm output
- **Immediate feedback**: No waiting for delayed pixels
- **Memory efficiency**: Time-based TTL prevents unbounded growth

## Monitoring

| Metric | Healthy Range | Alert Threshold | Indicates |
|--------|---------------|-----------------|-----------|
| **Serve/Impression Ratio** | 1.05-1.15 | >1.2 or <0.95 | Pixel blocking or double-counting |
| **Daily Cap Compliance** | ≤100% | >100% | Safety check failure |
| **Pixel Delay** | <30s median | >60s median | Network or blocking issues |

## Integration Points

**Ad Selection Flow:**
1. Check pacing eligibility using serve counter
2. Select winning creative
3. **Immediately increment serve counter**
4. Generate impression pixel URL
5. Return ad response to client

**Impression Tracking Flow:**
1. Client fires impression pixel (delayed)
2. Verify token and extract line item ID
3. **Increment impression counter for billing**
4. Record analytics event
5. Return 1×1 GIF

