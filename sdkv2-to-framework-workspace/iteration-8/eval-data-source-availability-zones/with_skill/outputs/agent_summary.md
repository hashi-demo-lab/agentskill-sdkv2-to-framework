# Migration summary — `openstack_blockstorage_availability_zones_v3` data source

## Scope

Migrated `data_source_openstack_blockstorage_availability_zones_v3.go` and its test file from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. No other provider files were modified.

## Pre-flight checks

- **Pre-flight 0 (mux check)**: Not a muxed migration — single data source migration, single release.
- **Pre-flight A (audit)**: Source file imports reviewed. The data source uses only `ReadContext`, no `StateUpgraders`, no custom `Importer`, no `Timeouts`, no `DiffSuppressFunc`, and no `MaxItems:1` nested blocks. Low-complexity migration.
- **Pre-flight B (plan)**: Scope confirmed to this single data source.
- **Pre-flight C (think pass)**: No blocks, no state upgrade, no import — straightforward read-only data source.

## Key decisions

### Schema attributes
- `region`: `Optional + Computed` — preserved exactly.
- `state`: `Optional + Computed` — the SDKv2 schema had `Default: "available"`. The framework `Default` package (`stringdefault.StaticString`) could be used, but since the attribute is `Computed`, the default is applied in the `Read` method (falling back to `"available"` when the attribute is null/unknown/empty), and `state` is written back to state after read. This avoids adding a `stringdefault` import while keeping identical runtime behaviour.
- `names`: `Computed` list of strings — mapped to `schema.ListAttribute{ElementType: types.StringType}`.
- `id`: Added explicit `Computed` `id` attribute (required by the framework; SDKv2 managed `id` implicitly via `d.SetId`).

### Resource ID
The SDKv2 implementation called `d.SetId(hashcode.Strings(zones))`. The framework version writes the same value to the `ID` field of the model struct using `types.StringValue(hashcode.Strings(zones))`.

### Region resolution
`GetRegion(d, config)` (which requires a `*schema.ResourceData`) is replaced with inline logic: check the config attribute, fall back to `d.config.Region`. This preserves the same precedence without touching the SDKv2 `GetRegion` helper.

### Client plumbing
`Configure` stores the `*Config` from `req.ProviderData`. `BlockStorageV3Client` is called on that stored config in `Read`.

## Test file

`ProviderFactories: testAccProviders` replaced with `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.

The `testAccProtoV6ProviderFactories` variable uses `providerserver.NewProtocol6WithError(NewOpenStackProvider()())`. `NewOpenStackProvider()` is the framework provider constructor that must be introduced as part of the broader provider migration (step 4 of the 12-step workflow). This test file intentionally does not compile until that step is complete — which is the correct TDD gate (step 7): the test should go red first, then green after the implementation is in place.

## Import constraints met

All imports are confined to: `terraform-plugin-framework*`, `gophercloud/v2`, `terraform-provider-openstack/utils/v2/hashcode`, and stdlib. No `terraform-plugin-sdk/v2` import in the migrated file.

## What remains for a full provider migration

This migration is one data source. To complete the full SDKv2 → framework migration, the remaining 12-step workflow items apply: serving via protocol v6 (`main.go`), migrating the provider definition and schema, migrating all other resources and data sources, and removing remaining SDKv2 references.
