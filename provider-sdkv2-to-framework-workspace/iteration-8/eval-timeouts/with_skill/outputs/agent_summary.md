# Migration summary — openstack_db_database_v1

## Source file
`openstack/resource_openstack_db_database_v1.go`

## What was done

### Resource file (`resource_openstack_db_database_v1.go`)

- Removed all `terraform-plugin-sdk/v2` imports (`schema`, `diag`, `helper/retry`).
- Introduced a framework resource type `dbDatabaseV1Resource` satisfying `resource.Resource`, `resource.ResourceWithConfigure`, and `resource.ResourceWithImportState`.
- Defined a typed model struct `dbDatabaseV1Model` with `tfsdk:` tags matching schema attribute names exactly, including `Timeouts timeouts.Value \`tfsdk:"timeouts"\``.
- **Timeouts**: translated `schema.ResourceTimeout{Create, Delete}` to `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` in the schema's `Blocks:` map (not `Attributes:`), preserving the `timeouts { create = "..." }` HCL block syntax that existing practitioner configs use.
- In `Create`, read the configured timeout via `plan.Timeouts.Create(ctx, 10*time.Minute)` and applied it with `context.WithTimeout`.
- In `Delete`, read the configured timeout via `state.Timeouts.Delete(ctx, 10*time.Minute)`.
- Replaced `retry.StateChangeConf` / `WaitForStateContext` with an inline `waitForState` helper (ticker-based, context-aware), as required by the framework migration — `helper/retry` must not remain in migrated files.
- Added a local `databaseDatabaseV1StateRefreshFuncFramework` returning `func() (any, string, error)` (no named `retry.StateRefreshFunc` type) to avoid any `helper/retry` dependency.
- `Read` signals resource absence via `resp.State.RemoveResource(ctx)` (replaces `d.SetId("")`).
- `Delete` checks for 404 via `gophercloud.ResponseCodeIs` and silently succeeds rather than returning an error, matching the SDKv2 `CheckDeleted` behaviour.
- Import via `resource.ImportStatePassthroughID` on `path.Root("id")`.
- All `ForceNew: true` attributes translated to `stringplanmodifier.RequiresReplace()`.
- Computed `region` and `id` use `stringplanmodifier.UseStateForUnknown()` to suppress spurious diffs.

### Test file (`resource_openstack_db_database_v1_test.go`)

- Replaced `ProviderFactories: testAccProviders` with `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` (TDD gate: test will compile-fail until `testAccProtoV6ProviderFactories` is wired in `provider_test.go`).
- Added an `ImportState` step with `ImportStateVerifyIgnore: []string{"timeouts"}` because the `timeouts` block is not returned by the API and would otherwise cause a false diff on import verify.
- Added `testAccDatabaseV1DatabaseWithTimeout` config helper showing the `timeouts { create = "20m"; delete = "20m" }` block syntax.
- All other test logic (check functions, destroy check) is preserved unchanged.

## Key decisions

| Decision | Reason |
|---|---|
| `timeouts.Block(...)` in `Blocks:` (not `timeouts.Attributes(...)`) | Preserves `timeouts { ... }` HCL block syntax; switching to attribute syntax would break existing practitioner configs |
| Inline `waitForState` helper | `helper/retry` is SDKv2; the verify negative gate rejects it in migrated files |
| `Update` returns an error | All attributes are `RequiresReplace`; the framework calls Update only if at least one non-replace attribute changed. Returning an error makes the impossible code path explicit |
| `gophercloud.ResponseCodeIs(err, 404)` in Delete | Drop-in replacement for `CheckDeleted` that doesn't take `*schema.ResourceData` |

## SDKv2 imports remaining
None — the migrated file contains zero references to `terraform-plugin-sdk/v2`.
