# Migration Summary: linode_user resource

## Pre-flight checks

- **Mux check**: No mux / multi-release intent. Single-release migration applies.
- **SDKv2 confirmed**: `schema_resource.go` imports `terraform-plugin-sdk/v2/helper/schema`.

## Per-resource think pass (Pre-flight C)

1. **Block decision**:
   - `global_grants`: `TypeList MaxItems:1` with nested `schema.Resource`. Templates (`grants.gotf`, `grants_update.gotf`) show practitioners using `global_grants { ... }` block syntax. → **SingleNestedBlock** (preserves HCL syntax; breaking to convert to SingleNestedAttribute).
   - Entity grants (`domain_grant`, `firewall_grant`, etc.): `TypeSet` without MaxItems → **SetNestedBlock** (repeating, block syntax in prod configs).

2. **State upgrade**: No `SchemaVersion > 0` / `StateUpgraders`. No upgrader needed.

3. **Import shape**: `ImportStatePassthroughContext` on `username` (string). → `IDType: types.StringType` passthrough via `BaseResource`.

## Key migration decisions

| SDKv2 pattern | Framework output |
|---|---|
| `Default: false` on bool fields | `booldefault.StaticBool(false)` via `Default:` field (NOT PlanModifiers) |
| `ForceNew: true` on `email` | `stringplanmodifier.RequiresReplace()` in PlanModifiers |
| `TypeList MaxItems:1 global_grants` | `schema.SingleNestedBlock` in `Blocks:` map |
| `TypeSet entity_grant` (repeating) | `schema.SetNestedBlock` in `Blocks:` map |
| `ImportStatePassthroughContext` | Custom `ImportState` setting `state.ID = req.ID` then `readInto` |
| `d.Get("foo")` string-path access | Typed `ResourceModel` struct via `req.Plan.Get / req.State.Get` |
| `schema.Set.List()` iteration | `types.Set.Elements()` iteration |

## Default: false fields translated

13 total: `restricted` (top-level), plus 12 inside `global_grants` block (`add_domains`, `add_databases`, `add_firewalls`, `add_images`, `add_linodes`, `add_longview`, `add_nodebalancers`, `add_stackscripts`, `add_volumes`, `add_vpcs`, `cancel_account`, `longview_subscription`). All use `booldefault.StaticBool(false)` with `Optional: true, Computed: true`.

## Test file changes

- Removed SDKv2 `*schema.ResourceData` usage (was already using `ProtoV6ProviderFactories`).
- Updated grant check paths: `global_grants.0.add_domains` → `global_grants.add_domains` (SingleNestedBlock uses no index).
- Added `ImportState: true` step to `TestAccResourceUser_basic`.
- All tests use `acceptance.ProtoV6ProviderFactories` (already present).

## Files produced

- `migrated/resource.go`: Full framework resource (Create/Read/Update/Delete/ImportState). No SDKv2 imports.
- `migrated/resource_test.go`: Updated acceptance tests with corrected attribute paths for SingleNestedBlock.
