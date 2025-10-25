# Integration Guide

This section explains how to integrate the ad server into a web page using the provided JavaScript SDK. The SDK is designed to simplify the process for publishers, providing flexible options for rendering ads and tracking engagement, while also enabling advanced customization.

## JavaScript SDK

The SDK is located at `static/sdk/adsdk.js` and offers several functions to fetch and display ads, giving publishers control over the integration process.

### SDK Methods

| Method | Parameters | Description |
|--------|------------|-------------|
| `setApiKey(apiKey)` | `apiKey` (string) | Set publisher API key globally |
| `setPublisherId(publisherId)` | `publisherId` (number) | Set publisher ID globally |
| `renderAd()` | `placementId`, `containerId`, `baseUrl?`, `keyValues?` | Render HTML or banner ad in iframe (server-composed) |
| `renderNativeAd()` | `placementId`, `containerId`, `templateFn`, `baseUrl?`, `keyValues?` | Render native ad with custom template |
| `fetchAd()` | `placementId`, `baseUrl?`, `keyValues?` | Get ad response without rendering |
| `reportAd()` | `reportUrl`, `reason`, `baseUrl?` | Submit ad quality report |
| `showReportModal()` | `reportUrl`, `baseUrl?` | Display report modal UI |
| `getReportLinkHTML()` | `text`, `className?`, `style?` | Generate report button HTML |

### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `placementId` | string | Yes | Ad placement ID configured in ad server |
| `containerId` | string | Yes* | DOM element ID for ad rendering |
| `templateFn` | function | Yes* | Custom template function for native ads |
| `baseUrl` | string | No | Override default ad server URL |
| `keyValues` | object | No | Custom targeting key-value pairs |
| `reportUrl` | string | Yes* | Report URL from ad response |
| `reason` | string | Yes* | Report reason code |
| `apiKey` | string | Yes | Publisher API key |
| `publisherId` | number | Yes | Publisher account ID |

*Required for specific methods only

Set your API key and publisher ID once via `AdSDK.setApiKey()` and `AdSDK.setPublisherId()` (or global variables `window.AD_SERVER_API_KEY` and `window.AD_SERVER_PUBLISHER_ID`).

### Example: Fetch Ad for Custom Rendering
```javascript
AdSDK.fetchAd("sidebar-banner", undefined, { "article_id": "12345" })
  .then(response => {
    const bid = response.seatbid[0].bid[0];
    // Custom rendering logic here
  });
```

## Server-to-Server (S2S) Request

You can also call the ad server directly from your backend or mobile code without using the SDK. This is useful for server-side rendering or integrating the ad decision into other systems. Each S2S request must include the publisher's ID in the `ext.publisher_id` field (e.g., `"ext":{"publisher_id":1}`). The publisher's API key must still be sent in the `X-API-Key` header.

```bash
curl -X POST http://localhost:8787/ad \
  -H "Content-Type: application/json" \
  -H "X-API-Key: YOUR_PUBLISHER_API_KEY" \
  -d '{"id":"req1","imp":[{"id":"1","tagid":"header"}],"user":{"id":"user1"},"device":{"ua":"server","ip":"203.0.113.10"},"ext":{"publisher_id":1}}'
```

The response contains the same OpenRTB bid object returned to the SDK, allowing full server-to-server workflows.


Events and Callbacks:
- **`AdSDK:noAd` Event**: If no ad is available for a given request when using `renderAd` or `renderNativeAd`, the SDK dispatches a custom DOM event named `AdSDK:noAd` on the specified container element. Publishers can listen for this event to implement fallback behavior, such as collapsing the ad slot or displaying alternative content. This gives publishers control over the user experience when ads aren't filled.
- **`AdSDK_Creative_Event(eventType)`** (callable from within sandboxed HTML ad iframes): For creatives rendered by `AdSDK.renderAd` (which are placed in an iframe), this function is exposed globally as `window.AdSDK_Creative_Event` *inside the iframe*. The creative can call `AdSDK_Creative_Event('yourEventType')` to signal that a custom event has occurred (e.g., `AdSDK_Creative_Event('like')`, `AdSDK_Creative_Event('video_milestone_75')`). The SDK on the parent page listens for this signal and then uses the `evturl` from the original ad response to record this event with the server, appending `&type=yourEventType`. This allows publishers to track meaningful engagement beyond simple clicks.

Core SDK Responsibilities:
The SDK automates several key tasks to simplify integration while providing hooks for publisher customization:
1.  **User Identification**: Manages a unique user ID, typically stored in `localStorage`, to support frequency capping and user-level targeting. Publishers generally don't need to manage this directly but benefit from its effects.
2.  **API Communication**: Constructs and sends the `POST /ad` request to the ad server, including any specified `keyValues` and device information. It also handles parsing the JSON response.
    -   For debugging convenience, ad requests can include the `?debug=1` query parameter (or be enabled via the `DEBUG_TRACE` environment variable on the server) to return a detailed selection trace in the ad response. The SDK transparently passes this along if configured.
3.  **Creative Rendering (for `renderAd` and `renderNativeAd`)**:
    -   For HTML and banner ads (`renderAd`), it securely renders the creative markup within a sandboxed iframe using the `srcdoc` attribute. Banner creatives are server-side composed into responsive HTML before being delivered to the SDK. This isolates the creative from the parent page, enhancing security and publisher control over their page integrity.
    -   For native ads (`renderNativeAd`), it calls the publisher-provided `templateFn` with the ad assets and injects the returned HTML into the specified container, giving full display control to the publisher.
4.  **Tracking Pixel Firing (Automated for `renderAd`, `renderNativeAd`)**: Automatically requests the `impurl` (impression URL) when an ad is successfully rendered (and typically when it becomes visible, though initial versions might fire on render). It also facilitates the handling of `clkurl` (click URL) and `evturl` (custom event URL) by ensuring these are correctly associated with the rendered ad and can be triggered by user interactions or SDK calls. For `fetchAd`, the publisher is responsible for firing these URLs.
5.  **Token Management**: Handles the secure tokens embedded in `impurl`, `clkurl`, and `evturl`. These tokens are essential for validating tracking requests and typically expire after a configurable duration (`TOKEN_TTL`, defaulting to 30 minutes). The SDK ensures these tokens are used correctly for SDK-managed interactions.

This combination of automated functions and customizable parameters ensures that publishers can integrate the ad server quickly while retaining significant control over the ad experience and data flow.

## Example: Standard HTML Ad

Include the SDK and call `renderAd` for the placements you want to display. This is the simplest way to get an ad on the page:

```html
<script src="/static/sdk/adsdk.js"></script>
<div id="ad-header"></div>
<script>
  const slot = document.getElementById("ad-header");
  slot.addEventListener("AdSDK:noAd", () => {
    slot.style.display = "none"; // collapse when no ad
  });
  AdSDK.renderAd("header", "ad-header", undefined, { category: "sports" });
</script>
```

See `static/demo/index.html` for a full working example that displays multiple placements, and `static/demo/social-feed.html` for advanced native ad integration examples.

## Creative Formats

The ad server supports three creative formats, each optimized for different use cases:

### HTML Format
Custom ad markup provided directly by advertisers. Rendered in a sandboxed iframe for security. Best for interactive ads, rich media, or advertiser-provided creative code.

### Banner Format
Image-based ads with responsive image support. Publishers provide banner creative data as JSON with image URLs, dimensions, and alt text. The server automatically composes banner data into optimized HTML with srcset attributes for retina/high-DPI displays. This format is ideal for standard display advertising with minimal publisher integration complexity.

**Banner Creative JSON Structure:**
```json
{
  "image": "https://example.com/ad-728x90.jpg",
  "alt": "Premium Headphones - 40% Off Spring Sale",
  "images": [
    {"url": "https://example.com/ad-728x90.jpg", "width": 728, "height": 90},
    {"url": "https://example.com/ad-1456x180.jpg", "width": 1456, "height": 180}
  ]
}
```

The server composes this into an `<img>` tag with proper srcset for responsive rendering. Publishers use the same `renderAd()` method as HTML ads - no special handling required.

### Native Format
Maximum flexibility for publishers to control ad presentation. The server returns raw JSON assets, and publishers provide a custom template function to render ads seamlessly within their site design. Best for in-feed ads, sponsored content, and custom ad layouts.

## Example: Native Ad with Custom Rendering and Event Tracking

Native ads provide the ultimate flexibility for publishers to seamlessly integrate advertising content into their site's design. The `renderNativeAd` function, combined with a custom template function, empowers this.

```html
<div id="native-ad-container"></div>
<script>
  // Publisher-defined template function
  function createMyNativeAdHtml(assets) {
    // 'assets' is a JSON object like:
    // { "title": "My Awesome Product", "image": {"url": "http://.../img.jpg", "w": 300, "h": 200}, "sponsoredBy": "BrandX", "clickUrl": "...", "impressionTrackers": ["...", "..."], "custom_data_field": "any value" }
    // The actual asset structure depends on what's configured in the creative on the server.

    // Note: For click tracking, the SDK typically wraps the rendered content or expects specific handlers.
    // For simplicity, this example assumes the click is handled on the main container or specific elements.
    // The evturl for custom events is handled via data-ad-event.

    let html = `<div class="my-native-ad">`;
    if (assets.image && assets.image.url) {
      html += `<img src="${assets.image.url}" alt="${assets.title || 'Ad image'}" style="max-width:100%; height:auto;">`;
    }
    if (assets.title) {
      html += `<h3>${assets.title}</h3>`;
    }
    if (assets.sponsoredBy) {
      html += `<p>Sponsored by: ${assets.sponsoredBy}</p>`;
    }
    // Example of an element that can trigger a custom 'like' event
    html += `<button data-ad-event="like">Like this Ad!</button>`;
    // Example of an element that can trigger a custom 'learn_more' event
    html += `<a href="#" data-ad-event="learn_more" onclick="window.open(assets.clickUrl); return false;">Learn More</a>`;
    html += `</div>`;
    return html;
  }

  const nativeSlot = document.getElementById("native-ad-container");
  nativeSlot.addEventListener("AdSDK:noAd", () => {
    nativeSlot.style.display = "none"; // Collapse if no ad is returned
    console.log("No native ad to display in native-ad-container.");
  });

  AdSDK.renderNativeAd(
    "native_feed_placement_id", // The ID of your native placement
    "native-ad-container",        // The ID of the container to render into
    createMyNativeAdHtml,         // Your custom HTML templating function
    undefined,                    // Optional baseUrl override
    { "content_type": "article_feed" } // Optional keyValues for targeting
  );
</script>
```

**Custom Event Tracking with `data-ad-event`**:

As shown in the `createMyNativeAdHtml` function above, elements within your custom native ad HTML can trigger specific engagement events. By adding a `data-ad-event="yourEventType"` attribute to an element (e.g., a button or link):
- When a user clicks on such an element, the SDK will automatically capture this interaction.
- It then makes a request to the `evturl` (provided in the original ad response), appending `&type=yourEventType` to it.
- This allows publishers to track a wide variety of interactions beyond simple impressions and clicks, such as "video_play", "add_to_cart", "form_submit", or custom interactions like "like" or "share", directly from their native ad layouts.

This `data-ad-event` mechanism provides a declarative way for publishers to instrument their native ads for rich event tracking, further enhancing their ability to measure and optimize ad performance.

## Native Ad Demos and Examples

The ad server includes comprehensive native ad demos that showcase different integration patterns and use cases:

### Social Media Feed Demo (`/static/demo/social-feed.html`)

This demo demonstrates two distinct native ad formats:

#### 1. In-Feed Native Ads
- **Placement ID**: `social_native_feed`
- **Integration**: Seamlessly integrated sponsored posts that match social media styling
- **Custom Events**: Tracks like, share, and comment interactions
- **Features**:
  - Real-time engagement counters
  - Custom report button positioning
  - Responsive design that matches organic content
  - Event tracking with `data-ad-event` attributes

```javascript
AdSDK.renderNativeAd(
  "social_native_feed",
  "native-ad-container",
  (assets) => `
    <div class="post">
      <div class="post-header">
        <div class="avatar">${assets.brand?.[0] || 'AD'}</div>
        <div class="post-info">
          <h6>${assets.brand || assets.title}</h6>
          <p>Sponsored</p>
        </div>
      </div>
      <div class="post-content">
        <p>${assets.description}</p>
        ${assets.image ? `<img src="${assets.image}" alt="${assets.title}">` : ''}
        <a href="${assets.click}" class="cta-button">Learn More →</a>
      </div>
      <div class="post-actions">
        <button data-ad-event="like">❤️ <span class="count">0</span> Like</button>
        <button data-ad-event="share">🔗 <span class="count">0</span> Share</button>
        <button data-ad-event="comment">💬 <span class="count">0</span> Comment</button>
      </div>
    </div>
  `
);
```

#### 2. Background Skin Takeover
- **Placement ID**: `fullscreen_takeover`
- **Integration**: Non-blocking background branding with clickable sidebars
- **Configuration**: ASAP delivery, high priority, no frequency caps
- **Features**:
  - Full-screen background imagery
  - Branded sidebars with CTAs
  - Dismissible via close button, background click, or ESC key
  - Disabled report functionality for clean experience

```javascript
AdSDK.renderNativeAd(
  "fullscreen_takeover", 
  "takeover-container",
  (assets) => `
    <div class="takeover-background" style="background-image: url('${assets.background}')"></div>
    <button class="skin-close" onclick="hideSkin()">×</button>
    
    <div class="skin-sidebar-left">
      <div class="skin-offer">${assets.offer || '70%'}</div>
      <div class="skin-title">Black Friday Sale</div>
      <a href="${assets.click}" class="skin-cta">Shop Now</a>
    </div>
    
    <div class="skin-sidebar-right">
      <div class="skin-offer">OFF</div>
      <div class="skin-title">Everything Must Go!</div>
      <a href="${assets.click}" class="skin-cta">Save Big</a>
    </div>
  `
);
```

### Report Functionality Control

The SDK supports disabling automatic report button generation for certain ad formats:

```html
<!-- Disable automatic report button for background skin ads -->
<div id="takeover-container" data-disable-reporting="true">
  <!-- Native ad will be rendered here without auto-report button -->
</div>
```

When `data-disable-reporting="true"` is set on the container, the SDK will not automatically append its default report button, giving publishers full control over the user experience.

## Header Bidding with Prebid Server

The optional Docker Compose profile `prebid` starts a `prebid-server` container at
`http://localhost:8060`. Programmatic line items can use the endpoint
`http://prebid-server:8000/openrtb2/auction` to fetch bids. See
[Programmatic Demand](docs/programmatic.md) for a full explanation of how these bids
are fetched and compete with direct line items.
