# Migration Notes: data_source_openstack_blockstorage_availability_zones_v3

## What was migrated

Migrated `data_source_openstack_blockstorage_availability_zones_v3.go` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Key changes

### Main data source file

- Replaced `*schema.Resource` with a struct `blockStorageAvailabilityZonesV3DataSource`
  implementing `datasource.DataSource` and `datasource.DataSourceWithConfigure`.
- Replaced `map[string]*schema.Schema` with `datasource.SchemaResponse` / `schema.Schema`
  using typed attribute structs (`schema.StringAttribute`, `schema.ListAttribute`).
- Replaced `validation.StringInSlice(...)` with
  `stringvalidator.OneOfCaseInsensitive(...)` from `terraform-plugin-framework-validators`.
- The SDK `Default: "available"` on the `state` attribute has no direct framework
  equivalent — handled at read time: if `state` is null/unknown/empty the default
  `"available"` is applied in the `Read` method.
- Added an explicit `id` attribute (computed) in the schema — the framework does not
  inject `id` automatically like the SDK does.
- Replaced `*schema.ResourceData` with a model struct
  `blockStorageAvailabilityZonesV3DataSourceModel` using `tfsdk` struct tags.
- Region resolution uses `config.DetermineRegion(region)` instead of the SDK helper
  `GetRegion(d, config)` (which depends on `*schema.ResourceData`).
- `types.ListValueFrom(ctx, types.StringType, zones)` replaces `d.Set("names", zones)`.
- Provider data is injected via the `Configure` method receiving `*Config`.

### Test file

- No functional changes needed — the test uses `terraform-plugin-testing` which is
  framework-agnostic. The `ProviderFactories` and helper functions (`testAccPreCheck`,
  `testAccPreCheckNonAdminOnly`) are unchanged.

## New dependencies required

- `github.com/hashicorp/terraform-plugin-framework` (v1.16.1 used for compile check)
- `github.com/hashicorp/terraform-plugin-framework-validators` (v0.16.0 used)

## Provider wiring note

The migrated data source uses the framework interface and can no longer be registered
in the SDK provider's `DataSourcesMap`. It must be registered through a framework
provider or a mux server. The existing `provider.go` line:

```go
"openstack_blockstorage_availability_zones_v3": dataSourceBlockStorageAvailabilityZonesV3(),
```

would need to be removed and the data source registered via
`NewBlockStorageAvailabilityZonesV3DataSource` in a framework-aware provider.
This wiring is **not** included in the migrated outputs (per task constraints).

## Compile check result

Built successfully in isolation (`go build ./openstack/...`) using a minimal stub module
at `/tmp/framework-datasource-check/` with only the migrated file and a `Config` stub.
No compile errors.
