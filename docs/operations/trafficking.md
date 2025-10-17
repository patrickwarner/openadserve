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
| **Placements** | `id`, `width`, `height`, `formats` | ID is manually set, formats are comma-separated |
| **Line Items** | `start_date`, `end_date`, `cpm/cpc`, `active` | Set targeting and budget constraints |
| **Creatives** | `placement_id`, `line_item_id`, `html` | Must reference existing entities |

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
