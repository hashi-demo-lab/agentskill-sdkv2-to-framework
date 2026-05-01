# Migration summary — openstack_db_user_v1

## Source file
`openstack/resource_openstack_db_user_v1.go` — SDKv2 resource (Create/Read/Delete, no Update, no Importer).

## Key decisions

### password field
- `Sensitive: true`, `WriteOnly: true`, NOT `Computed` (as specified).
- `ForceNew` → `RequiresReplace()` plan modifier.
- In `Create`: read from `req.Config` (not `req.Plan`), since write-only values are present in config but null in plan/state.
- In `Read`: not populated (write-only values never appear in state — left as null, which is correct).
- `WriteOnly` requires framework v1.14+; go.mod shows v1.17.0 — satisfied.

### databases field
- SDKv2 `TypeSet` with `Set: schema.HashString` → framework `SetAttribute{ElementType: types.StringType}`. Hash function dropped (framework handles uniqueness internally).
- `Optional+Computed` retained; `ForceNew` → `RequiresReplace()` + `UseStateForUnknown()`.

### region field
- `Optional+Computed+ForceNew` → `RequiresReplace()` + `UseStateForUnknown()`. Falls back to `r.config.Region` when not set.

### retry.StateChangeConf
- Replaced with inline `waitForState` helper (no framework equivalent; SDKv2 `helper/retry` not imported).

### Import
- Original SDKv2 resource had no `Importer` set, but the composite ID format (`instanceID/userName`) is used throughout. Added `ImportState` method that parses the composite ID and sets `id`, `instance_id`, and `name` into state so `Read` can proceed.

### GetRegion / CheckDeleted
- `GetRegion(d, config)` replaced with direct `plan.Region.ValueString()` with fallback to `r.config.Region`.
- `CheckDeleted` not applicable (delete uses `databaseUserV1Exists` to detect absence before calling API, and `RemoveResource` in Read for drift).

## Test changes
- `ProviderFactories` → `ProtoV6ProviderFactories` (using `testAccProtoV6ProviderFactories`).
- Added import test step with `ImportStateVerifyIgnore: []string{"password"}` — required because write-only attributes are null in state and would cause import-verify to fail without this.

## Negative gate
No references to `github.com/hashicorp/terraform-plugin-sdk/v2` in either output file.

## Helpers reused from existing package
- `databaseUserV1StateRefreshFunc` (signature `func() (any, string, error)` — compatible with `waitForState`)
- `databaseUserV1Exists`
- `expandDatabaseUserV1Databases`
- `flattenDatabaseUserV1Databases`
- `parsePairedIDs`
