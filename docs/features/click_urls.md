# Click URL Management and Macro Expansion

The ad server supports comprehensive click URL management with dynamic macro expansion, allowing advertisers to define destination URLs for their ads with rich contextual data.

## Overview

When users click on ads, they are redirected to advertiser-defined destination URLs. The system supports:

- **Creative-level click URLs**: URLs defined at the individual creative level
- **Line item-level click URLs**: Fallback URLs defined at the line item level
- **Macro expansion**: Dynamic substitution of contextual data into URLs
- **Custom parameters**: Support for advertiser-defined URL parameters
- **URL validation**: Security checks to prevent malicious redirects


## URL Priority System

The system uses the following priority order for determining destination URLs:

1. **Creative-level click URL** - Highest priority
2. **Line item-level click URL** - Fallback if creative URL is empty
3. **No redirect** - Returns tracking pixel if no URLs are configured

## Macro Expansion System

### Standard Macros

| Macro | Type | Description |
|-------|------|-------------|
| `{AUCTION_ID}` | Request | Ad request ID |
| `{AUCTION_IMP_ID}` | Request | Impression ID from request |
| `{CREATIVE_ID}` | Ad | ID of served creative |
| `{LINE_ITEM_ID}` | Ad | ID of line item |
| `{CAMPAIGN_ID}` | Ad | ID of campaign |
| `{PUBLISHER_ID}` | Ad | ID of publisher |
| `{PLACEMENT_ID}` | Ad | ID of placement |
| `{TIMESTAMP}` | Time | Unix timestamp in seconds |
| `{TIMESTAMP_MS}` | Time | Unix timestamp in milliseconds |
| `{ISO_TIMESTAMP}` | Time | ISO 8601 formatted timestamp |
| `{RANDOM}` | Utility | Random number for cache busting |
| `{UUID}` | Utility | Unique UUID for tracking |

### Custom Parameters

Publishers can pass custom contextual data using the `{CUSTOM.key}` pattern. Include custom parameters in the ad request's `ext.custom_params` object:

```json
{
  "ext": {
    "custom_params": {
      "utm_source": "homepage",
      "user_segment": "premium", 
      "content_category": "tech"
    }
  }
}
```

**Parameter Limits:**

| Limit | Value | Purpose |
|-------|-------|---------|
| Max parameters | 10 | Prevents token bloat |
| Max key length | 50 chars | Ensures URL compatibility |
| Max value length | 100 chars | Maintains reasonable URL length |

## Implementation Details

### Click Handler Flow

1. **Token Validation**: Verify the click tracking token
2. **Creative Resolution**: Load the creative and associated line item
3. **URL Resolution**: Determine destination URL using priority system
4. **Macro Expansion**: Replace macros with actual values
5. **URL Validation**: Check for safe redirect schemes (http/https only)
6. **Analytics Recording**: Record click event for reporting
7. **Redirect or Pixel**: Either redirect to destination or return tracking pixel

### Macro Expansion Process

1. Replace `{CUSTOM.key}` patterns with custom parameter values
2. Replace standard macros with contextual data
3. URL-encode all expanded values
4. Validate final URL

### Security

- Only `http://` and `https://` URL schemes allowed
- Invalid URLs fall back to tracking pixel
- All macro values are URL-encoded
- Click URLs require valid tracking tokens

## Configuration Examples

```json
// Creative with custom parameters
{
  "id": 123,
  "click_url": "https://advertiser.com/landing?source={CUSTOM.utm_source}&segment={CUSTOM.user_segment}&creative={CREATIVE_ID}"
}

// Line item fallback URL
{
  "id": 456,
  "click_url": "https://advertiser.com/default?campaign={CAMPAIGN_ID}&placement={PLACEMENT_ID}"
}
```

## SDK Integration

### Setting Custom Parameters

```javascript
// Global parameters for all ads
AdSDK.setCustomParams({
  user_segment: 'premium',
  content_category: 'technology'
});

// Placement-specific parameters
AdSDK.setAdSlotCustomParams('sidebar-300x250', {
  placement_context: 'sidebar',
  inventory_type: 'standard'
});

// Auto-capture UTM parameters
AdSDK.setUTMFromPage();
```

### Parameter Priority

Parameters are merged in priority order:
1. Per-call parameters (highest)
2. Per-slot parameters
3. Global parameters (lowest)

## Monitoring

Click URL metrics are available in Prometheus and visualized in the included Grafana dashboard at `deploy/grafana/dashboards/click_url_observability.json`.

Key metrics:
- `macro_expansions_total` - Macro expansion counts
- `macro_expansion_duration_seconds` - Expansion performance
- `adserver_requests_total{endpoint="/click"}` - Click request tracking