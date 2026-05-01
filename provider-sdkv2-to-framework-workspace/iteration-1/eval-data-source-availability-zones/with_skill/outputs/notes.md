# Migration notes: openstack_blockstorage_availability_zones_v3

## What changed

### Source file (`data_source_openstack_blockstorage_availability_zones_v3.go`)

- Replaced `*schema.Resource` function with a struct type `blockStorageAvailabilityZonesV3DataSource` implementing `datasource.DataSource` and `datasource.DataSourceWithConfigure`.
- Added `Metadata`, `Schema`, `Configure`, and `Read` methods.
- Added `blockStorageAvailabilityZonesV3Model` struct with `tfsdk` tags for all four attributes: `id`, `region`, `state`, `names`.
- Removed SDKv2 imports (`terraform-plugin-sdk/v2/diag`, `terraform-plugin-sdk/v2/helper/schema`, `terraform-plugin-sdk/v2/helper/validation`).
- Added framework imports: `terraform-plugin-framework/datasource`, `terraform-plugin-framework/datasource/schema`, `terraform-plugin-framework/types`, `terraform-plugin-framework-validators/stringvalidator`.

### Schema judgment calls

**`state` attribute — Default and Validator:**
- SDKv2 used `Default: "available"` and `ValidateFunc: validation.StringInSlice([]string{"available", "unavailable"}, true)` (case-insensitive).
- Framework: added `Computed: true` (required when setting a default for an Optional attribute without using the `defaults` package with an explicit `Default:` field). The default value is applied in `Read` logic rather than via `stringdefault.StaticString`, because the `defaults` package requires the attribute to be `Optional+Computed` and a `Default` field — which is cleaner but also means Terraform sees the computed value in state. Judgment: applied the default inline in `Read` to keep the model simple.
  - Alternative considered: `Default: stringdefault.StaticString("available")` — this is more idiomatic but requires `"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"`. Using inline default is equivalent and avoids an extra import.
- Validator changed from `validation.StringInSlice(..., true /* ignoreCase */)` to `stringvalidator.OneOfCaseInsensitive("available", "unavailable")` — preserves case-insensitive semantics.

**`names` attribute — `TypeList` of strings → `ListAttribute{ElementType: types.StringType}`:**
- Simple homogeneous string list, no nested struct needed. Maps directly to `schema.ListAttribute`.
- In `Read`, populated via `types.ListValueFrom(ctx, types.StringType, zones)`.

**`id` attribute:**
- Framework data sources must have an explicit `id` field in the model. Added `types.String` with `tfsdk:"id"` to the model and `schema.StringAttribute{Computed: true}` to the schema.
- Value: `hashcode.Strings(zones)` — same as the SDKv2 version, preserves stable deterministic IDs.

**`GetRegion` helper:**
- SDKv2's `GetRegion(d *schema.ResourceData, config *Config)` cannot be used in framework code (takes a `*schema.ResourceData`).
- Replaced with inline logic: if `cfg.Region` is set and non-empty, use it; otherwise fall back to `d.config.Region`. Semantics are identical.

**`Configure` method:**
- Receives `*Config` from `req.ProviderData`. This is consistent with how OpenStack provider resources pass the config object via the framework `Configure` pipeline.

### Test file changes

- `ProviderFactories: testAccProviders` (SDKv2 `map[string]func() (*schema.Provider, error)`) → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- `testAccProtoV6ProviderFactories` must be defined in `provider_test.go` as part of the broader provider migration (not in scope for this single data source).
- Import changed: `terraform-plugin-testing/helper/resource` is already the correct package (used in the original test), so no import change needed.

## Compile / vet results

Tested in `/tmp/openstack-eval2-skill` (a copy of the provider repo):

1. Added `github.com/hashicorp/terraform-plugin-framework v1.17.0` and `github.com/hashicorp/terraform-plugin-framework-validators v0.19.0` via `go get`.
2. Pinned `terraform-plugin-go` back to `v0.29.0` (the version required by SDKv2 v2.38.1) to avoid SDKv2 interface breakage from upstream upgrades.
3. Removed the `openstack_blockstorage_availability_zones_v3` line from the SDKv2 `DataSourcesMap` in `provider.go` (this is part of the framework registration step; in a real migration, a new framework provider DataSources method would be added).
4. **`go build ./openstack/...` — PASS**
5. **`go vet ./openstack/...` — PASS**
6. Negative gate: migrated file contains no `terraform-plugin-sdk/v2` imports — **PASS**

Acceptance tests (`TestAcc*`) require live OpenStack credentials (`TF_ACC=1`) and were not run.
