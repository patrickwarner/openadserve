# Ad Reporting: Publisher-First Quality Control

The built-in ad reporting mechanism exemplifies this ad server's **publisher-first philosophy** by putting content quality control directly in the hands of publishers and their users. This feature represents a fundamental shift away from the traditional ad serving model where publishers have little recourse against problematic advertisements.

## Publisher-First Philosophy in Practice

### Quality Over Quantity
Many traditional ad servers prioritize filling inventory at any cost. Our reporting system enables publishers to maintain high-quality user experiences by:

- **Immediate User Feedback**: Users can flag problematic ads instantly, creating a direct feedback loop
- **Publisher Control**: Publishers gain visibility into ad quality issues affecting their audience
- **Advertiser Accountability**: Creates incentives for advertisers to maintain higher creative standards

### Transparency and Trust
The reporting system builds trust between publishers and their audience by:

- **Demonstrating Care**: Shows users that their experience matters to the publisher
- **Responsive Moderation**: Provides a clear path for addressing user concerns
- **Community Protection**: Helps identify and prevent harmful or misleading advertisements

## Technical Implementation

### Core Components

#### 1. Report Model (`internal/models/ad_report.go`)
```go
type AdReport struct {
    ID           int64     `json:"id"`
    AdID         string    `json:"ad_id"`
    UserID       *string   `json:"user_id,omitempty"`
    ReportReason string    `json:"report_reason"`
    IPAddress    string    `json:"ip_address"`
    UserAgent    string    `json:"user_agent"`
    Status       string    `json:"status"`
    CreatedAt    time.Time `json:"created_at"`
}
```

#### 2. Report Reasons with Severity Levels
The system includes predefined report categories with severity-based prioritization:

- **Critical**: `malware` - Security threats requiring immediate action
- **High**: `offensive`, `misleading` - Content that violates platform standards
- **Medium**: `other` - General quality issues
- **Low**: `irrelevant` - Mismatched content that doesn't harm users

### API Endpoint

#### `POST /report`
Accepts token-based ad reports with automatic validation and analytics tracking.

**Request Body:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiJ9...",
  "reason": "offensive"
}
```

**Response:**
```json
{
  "success": true,
  "message": "Report submitted successfully"
}
```

### Security Features

- **Token-Based Authentication**: Prevents unauthorized or spam reports
- **IP and User Agent Tracking**: Enables abuse detection and pattern analysis
- **Rate Limiting**: Built-in protection against report flooding
- **Audit Trail**: Complete tracking of report lifecycle for compliance

## Integration Guide

### JavaScript SDK Integration

The JavaScript SDK provides seamless reporting functionality:

```javascript
// Automatic report URL extraction from ad response
AdSDK.loadAd('placement-id').then(ad => {
    // Report button automatically configured
    document.getElementById('report-btn').onclick = () => {
        AdSDK.reportAd(ad.repturl, 'offensive');
    };
});

// Manual report submission
AdSDK.reportAd('report-token', 'misleading').then(response => {
    if (response.success) {
        showToast('Thank you for your report');
    }
});
```

### Custom Integration

For publishers with custom implementations:

```javascript
function reportAd(reportToken, reason) {
    fetch('/report', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({
            token: reportToken,
            reason: reason
        })
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            // Handle successful report
            updateUI('Report submitted successfully');
        }
    });
}
```

## Analytics and Monitoring

### Report Tracking
All reports generate analytics events that can be monitored via:

- **ClickHouse**: Detailed report event storage for analysis
- **Prometheus**: Real-time metrics for report volumes and patterns
- **Grafana**: Dashboards for visualizing report trends

### Key Metrics
- Report volume by reason category
- Report-to-impression ratios
- Geographic distribution of reports
- Advertiser performance by report frequency

