# Migration Summary: data_source_openstack_blockstorage_availability_zones_v3

## What was migrated

Single data source: `openstack_blockstorage_availability_zones_v3`

Source file: `openstack/data_source_openstack_blockstorage_availability_zones_v3.go`
Test file:   `openstack/data_source_openstack_blockstorage_availability_zones_v3_test.go`

## Key changes

### Implementation file

- Replaced `*schema.Resource` + `ReadContext` function with a Go type `blockStorageAvailabilityZonesV3DataSource` implementing `datasource.DataSource` and `datasource.DataSourceWithConfigure`.
- Schema moved from inline `map[string]*schema.Schema` to the `Schema` method using `datasource/schema` package (not `resource/schema`).
- Added explicit `id` attribute (Computed) to the framework schema — the framework requires all state attributes to be declared.
- `state` attribute: `Default: "available"` cannot be expressed in a data source schema attribute (no `Default` field on `datasource/schema.StringAttribute`); the default is instead applied in the `Read` method when the value is null/unknown.
- `ValidateFunc: validation.StringInSlice([]string{"available", "unavailable"}, true)` → `stringvalidator.OneOfCaseInsensitive("available", "unavailable")` (case-insensitive flag preserved).
- `hashcode.Strings(zones)` (SDKv2 helper) removed; a deterministic string `"<region>-<state>"` is used as the resource ID instead.
- `GetRegion(d, config)` (requires `*schema.ResourceData`) replaced with direct field access from the typed model struct.
- All state writes use typed `resp.State.Set(ctx, cfg)` instead of `d.Set(...)`.
- Removed SDKv2 imports: `terraform-plugin-sdk/v2/diag`, `terraform-plugin-sdk/v2/helper/schema`, `terraform-plugin-sdk/v2/helper/validation`, `terraform-provider-openstack/utils/v2/hashcode`.

### Test file

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- **Note**: `testAccProtoV6ProviderFactories` does not yet exist in `openstack/provider_test.go`. It must be added as part of completing the provider-level migration (step 4/5 of the workflow). A minimal addition:

```go
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider("test")()),
}
```

  where `NewFrameworkProvider` is the framework provider constructor to be created when the full provider is migrated. The test will compile-fail until then (per TDD step 7 — red-first).

## Imports used

- `github.com/hashicorp/terraform-plugin-framework/datasource`
- `github.com/hashicorp/terraform-plugin-framework/datasource/schema`
- `github.com/hashicorp/terraform-plugin-framework/schema/validator`
- `github.com/hashicorp/terraform-plugin-framework/types`
- `github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator`
- `github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/availabilityzones`
- stdlib: `context`, `fmt`, `sort`

## Preserved user-facing attributes

| Attribute | SDKv2 | Framework |
|---|---|---|
| `region` | Optional + Computed | Optional + Computed |
| `state` | Optional, default "available" | Optional + Computed, default applied in Read |
| `names` | Computed, TypeList of string | Computed, ListAttribute(StringType) |
