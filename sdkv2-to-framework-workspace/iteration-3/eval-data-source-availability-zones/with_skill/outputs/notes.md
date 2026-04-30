# Migration Notes: openstack_blockstorage_availability_zones_v3 Data Source

## Source file
`/Users/simon.lynch/git/terraform-provider-openstack/openstack/data_source_openstack_blockstorage_availability_zones_v3.go`

## Pre-flight audit (scope: single data source)

### Schema inventory
- `region`: Optional+Computed TypeString — maps directly to `schema.StringAttribute{Optional: true, Computed: true}`.
- `state`: Optional TypeString with `Default: "available"` and `ValidateFunc: validation.StringInSlice(["available","unavailable"], true)` (case-insensitive). No `ForceNew`. No state upgraders. No importer (data source).
- `names`: Computed TypeList of TypeString — maps to `schema.ListAttribute{Computed: true, ElementType: types.StringType}`.

### Block decision
No `MaxItems: 1 + nested Elem` patterns — all attributes are primitives or a flat list. No blocks needed.

### State upgrade
No `SchemaVersion` or `StateUpgraders`. Not applicable.

### Import shape
Data source — no importer.

## Key conversion decisions

### `Default: "available"` on `state`
The framework has no `Default` on a `Computed: false` optional attribute that works exactly like SDKv2. The cleanest equivalent is to handle the default in `Read` logic: if `state` is null/unknown/empty, treat it as `"available"` and write `"available"` back to state. This preserves the user-facing behaviour.

Alternative considered: `stringdefault.StaticString("available")` from `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault` — but this package applies to resource schemas. For data sources, applying the default in Read is idiomatic and avoids a dependency on the resource defaults package.

### `ValidateFunc: validation.StringInSlice(["available","unavailable"], true)` (case-insensitive)
Replaced with `stringvalidator.OneOfCaseInsensitive("available", "unavailable")` from `github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator`.

### `GetRegion(d, config)` helper
The SDKv2 `GetRegion` uses `*schema.ResourceData`. In the framework Read, we resolve region by calling `config.DetermineRegion(state.Region.ValueString())` which is the underlying method that `GetRegion` delegates to (defined in `utils/v2/auth/config.go`).

### `hashcode.Strings` for ID
The SDKv2 data source used `hashcode.Strings(zones)` to generate a stable ID. The framework migration keeps this import unchanged — the `hashcode` package is provider-level, not SDK-specific.

### `d.Set` / `d.SetId`
Replaced with typed model struct fields written back via `resp.State.Set(ctx, &state)`.

### ID attribute
Added explicit `id` attribute to the schema (`schema.StringAttribute{Computed: true}`) and model struct (`types.String`). In SDKv2, `d.SetId(...)` is special-cased; in the framework, `id` must be an explicit computed attribute.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- Added `testAccProtoV6ProviderFactories` var using `providerserver.NewProtocol6WithError(NewFrameworkProvider())`.
- Updated imports: removed `terraform-plugin-sdk/v2` indirect deps; added `github.com/hashicorp/terraform-plugin-framework/providerserver` and `github.com/hashicorp/terraform-plugin-go/tfprotov6`.
- The `NewFrameworkProvider()` function must be implemented in the real provider package (a full framework provider wrapping all migrated data sources and resources). In production this would replace the SDKv2 `Provider()` function as part of the full provider migration.

## Compile check

The openstack repo does NOT import `terraform-plugin-framework` (framework is absent from `go.mod`). Running both SDKv2 and framework in the same binary requires `terraform-plugin-mux`, which is explicitly out of scope for a single-release migration.

To verify compilation in isolation, the repo was copied to `/tmp/terraform-provider-openstack-compile-check/` and then a clean isolated module was created at `/tmp/framework-compile-check/` containing:
- The migrated data source file.
- Minimal stubs for `Config`, `NewFrameworkProvider`, and test helpers.
- Dependencies: `terraform-plugin-framework@v1.19.0`, `terraform-plugin-framework-validators@v0.19.0`, `terraform-plugin-testing@v1.16.0`, `terraform-plugin-go@v0.31.0`, `gophercloud/v2@v2.10.0`, `terraform-provider-openstack/utils/v2`.

Results:
- `go build ./openstack/...` — **PASS**
- `go vet ./openstack/...` — **PASS**

## What a full migration would require

These files are the migrated data source in isolation. A full provider migration would additionally need:

1. A complete `FrameworkProvider` implementing `provider.Provider` (with all schema, configure, resources, data sources).
2. All other data sources and resources migrated to framework types.
3. `main.go` updated to use `providerserver.NewProtocol6WithError(openstack.NewFrameworkProvider())`.
4. `go.mod` updated to add `terraform-plugin-framework` and `terraform-plugin-framework-validators`; `terraform-plugin-sdk/v2` removed after all resources are migrated.
5. Shared helpers (e.g. `GetRegion` in `util.go`) either adapted or bypassed in favour of framework equivalents.
