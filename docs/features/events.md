# Custom Events

This server allows publishers to track engagement beyond standard impressions and clicks. Custom events must be listed in the allowlist defined in `internal/api/event.go`.

## Default Event Types

The default distribution includes the following whitelisted event types:

### `like`
- **Purpose**: User expressed a like/heart reaction to the ad content
- **Common Use Cases**: Social media style engagement, product appreciation
- **Example**: User clicks a heart icon or like button within a native ad

### `share` 
- **Purpose**: User shared the ad content with others
- **Common Use Cases**: Social sharing, viral content tracking
- **Example**: User clicks a share button to post the ad to social media

### `comment`
- **Purpose**: User commented on or engaged with the ad content  
- **Common Use Cases**: Interactive content, user-generated content campaigns
- **Example**: User opens a comment form or leaves feedback on the ad

## Usage Examples

### Native Ad Implementation

For native ads using the `data-ad-event` attribute:

```html
<div class="native-ad">
  <h3>Amazing Product</h3>
  <p>Check out this incredible offer!</p>
  <div class="ad-actions">
    <button data-ad-event="like">❤️ Like</button>
    <button data-ad-event="share">🔗 Share</button> 
    <button data-ad-event="comment">💬 Comment</button>
  </div>
</div>
```

### HTML Ad Implementation

For HTML ads rendered in iframes using the `AdSDK_Creative_Event` function:

```html
<div class="ad-content">
  <button onclick="AdSDK_Creative_Event('like')">Like This Ad</button>
  <button onclick="AdSDK_Creative_Event('share')">Share</button>
</div>
```

### Manual Event Tracking

For direct API calls, append the event type to the `evturl`:

```
GET /event?t=GENERATED_TOKEN&type=like
GET /event?t=GENERATED_TOKEN&type=share  
GET /event?t=GENERATED_TOKEN&type=comment
```

## Event Processing

Events are processed as follows:
1. **Validation**: Event type must be in the allowlist (`internal/api/event.go`)
2. **Authentication**: Token must be valid and not expired
3. **Recording**: Events are stored in ClickHouse for analytics
4. **Counting**: Daily counters per line item are maintained in Redis
5. **Integration**: Counts can be used for pacing, billing, and optimization logic

## Adding New Event Types

To add a new custom event type:

1. Update the `AllowedEventTypes` map in `internal/api/event.go`:
```go
var AllowedEventTypes = map[string]struct{}{
    "like":         {}, // user expressed a like/heart reaction
    "share":        {}, // user shared the content  
    "comment":      {}, // user commented on the content
    "your_event":   {}, // your custom event description
}
```

2. Document the event purpose and trigger conditions
3. Restart the ad server to apply changes
4. Test the new event type with your creative implementations

## Demo Implementation

See `/static/demo/social-feed.html` for a complete working example of custom event tracking in native ads, including real-time counter updates and visual feedback.
