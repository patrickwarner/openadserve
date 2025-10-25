# Ad Server Trafficking UI - Internal Dev Tool

## Overview

Simple web interface for managing ad server entities during development and testing. **Not intended for production or client use.**

## Access

Start the server and visit: `http://localhost:8787/static/admin.html`

## Entity Setup Order

1. **Publishers** - Create a publisher first
2. **Placements** - Define ad slots (width/height/formats)  
3. **Campaigns** - Group line items under a publisher
4. **Line Items** - Set targeting, budgets, dates
5. **Creatives** - Upload actual ad content

## Entity Fields

| Entity | Required Fields | Notes |
|--------|-----------------|-------|
| **Publishers** | `name`, `domain` | - |
| **Placements** | `id`, `width`, `height`, `formats` | ID is manually set, formats are comma-separated (html, banner, native) |
| **Line Items** | `start_date`, `end_date`, `cpm/cpc`, `active` | Set targeting and budget constraints |
| **Creatives** | `placement_id`, `line_item_id`, `format`, content | Format determines content field (html, banner JSON, or native JSON) |

## Creative Formats

The ad server supports three creative formats:

### HTML Format
Custom ad markup for interactive or rich media ads. Enter HTML directly in the content field. The SDK renders this in a sandboxed iframe for security.

**Example:**
```html
<div style="background:#333;color:#fff;padding:20px;">
  <h2>Special Offer!</h2>
  <p>Get 40% off premium headphones</p>
</div>
```

### Banner Format
Image-based display ads with responsive image support. Enter JSON with image URLs and dimensions. The server automatically composes this into optimized HTML with srcset for retina displays.

**Example:**
```json
{
  "image": "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?w=728&h=90&fit=crop",
  "alt": "Premium Headphones - 40% Off Spring Sale",
  "images": [
    {"url": "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?w=728&h=90&fit=crop", "width": 728, "height": 90},
    {"url": "https://images.unsplash.com/photo-1505740420928-5e560c06d30e?w=1456&h=180&fit=crop", "width": 1456, "height": 180}
  ]
}
```

### Native Format
Flexible JSON assets for publisher-controlled rendering. Define custom fields that publishers render using their own templates.

**Example:**
```json
{
  "title": "Premium Wireless Headphones",
  "description": "Experience crystal-clear sound with our latest model",
  "brand": "AudioTech",
  "image": "https://example.com/product.jpg",
  "sponsored": "Sponsored by AudioTech"
}
```

## Testing

After setup, test ad delivery:
```bash
curl -X POST http://localhost:8787/ad -H "Content-Type: application/json" \
  -d '{"publisher_id": 1, "placement_id": "banner", "user_id": "test"}'
```

## Notes

- Delete operations cascade (e.g., deleting line items removes creatives)
- No pagination - use small test datasets
- Check server logs for detailed error messages
