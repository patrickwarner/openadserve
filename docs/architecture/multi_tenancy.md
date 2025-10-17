# Multi-Tenancy and Publishers

This ad server supports multiple publishers in a single deployment. Publishers are stored in the `publishers` table and referenced by other entities such as placements, campaigns and creatives.

## Defining Publishers

Run `tools/fake_data` to populate the database with demo data. When the database is empty it creates a demo publisher with ID `1` and API key `demo123` before generating additional random publishers and related entities. The demo includes a second publisher with a basic placement, campaign, line item and creative so that requests specifying `publisher_id` `2` have data to serve.

## Including `publisher_id` in Ad Requests

Every ad request must specify the publisher so that the server can load the correct placements and selectors. Provide it in the OpenRTB `ext` object:

```json
{
  "id": "req1",
  "imp": [{"id": "1", "tagid": "header_2"}],
  "user": {"id": "u1"},
  "ext": {"publisher_id": 2}
}
```

If the server receives an unknown `publisher_id` it will reject the request.

## Custom Selectors per Publisher

Custom ad selection strategies can be registered on a per-publisher basis. After
creating a selector that implements `selectors.Selector`, register it with the
server. This typically happens in `cmd/server/main.go` once the `api.Server`
instance has been created:

```go
srvDeps := api.NewServer(logger, store, database, pg, analyticsSvc, geoSvc,
    selectors.NewRuleBasedSelector(), cfg.DebugTrace, []byte(cfg.TokenSecret), cfg.TokenTTL)

// Register our custom selector for publisher 2
srvDeps.RegisterSelector(2, MySelector{})
```

When an ad request specifies `publisher_id` `2`, the server will use
`MySelector` instead of the default rule-based selector defined in
`internal/api/server.go`.
