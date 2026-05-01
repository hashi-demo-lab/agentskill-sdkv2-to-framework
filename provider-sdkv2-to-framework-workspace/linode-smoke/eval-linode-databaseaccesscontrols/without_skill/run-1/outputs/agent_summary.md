# Migration Summary: linode_database_access_controls

## Source Files

- `linode/databaseaccesscontrols/resource.go` — SDKv2 implementation
- `linode/databaseaccesscontrols/schema_resource.go` — SDKv2 schema definition

## What Was Produced

### `migrated/resource.go`

A full terraform-plugin-framework implementation with **no SDKv2 imports**. Key design decisions:

| Aspect | SDKv2 | Framework |
|--------|-------|-----------|
| Resource constructor | `Resource() *schema.Resource` | `NewResource() resource.Resource` returning `*Resource` embedding `helper.BaseResource` |
| Schema | `map[string]*schema.Schema` | `schema.Schema{Attributes: ...}` |
| CRUD methods | `readResource(ctx, d, meta)` etc. | `(r *Resource) Read/Create/Update/Delete(ctx, req, resp)` |
| State | `*schema.ResourceData` | `ResourceModel` struct with `tfsdk` tags |
| Import | `schema.ImportStatePassthroughContext` (passthrough composite ID to `d.Id()`) | Custom `ImportState` that parses `"<db_id>:<db_type>"` and sets `id`, `database_id`, and `database_type` attributes |
| allow_list type | `*schema.Set` (HashString) | `types.Set` with `ElementType: types.StringType` |
| Timeout | `schema.TimeoutUpdate` via `d.Timeout(...)` | `timeouts.Opts{Update: true}` block; `plan.Timeouts.Update(ctx, defaultUpdateTimeout)` |

**Composite ID handling:** The resource ID remains `"<database_id>:<database_type>"` (e.g. `"123:mysql"`). `formatID` / `parseID` helpers produce and consume this format. `ImportState` explicitly populates `id`, `database_id`, and `database_type` from the parsed import ID — allowing the subsequent Read to succeed with full state.

**Idempotent allow_list application:** `applyAllowList` fetches the current list before issuing an API PUT. If the lists are already equal, no update is sent (preserving the original SDKv2 comment about avoiding spurious `database_update` events).

**Validation:** `database_type` is validated with `stringvalidator.OneOfCaseInsensitive(databaseshared.ValidDatabaseTypes...)` (replacing the SDKv2 `validation.StringInSlice` with `true` for case-insensitivity).

**BaseResource:** Embeds `helper.BaseResource` which provides `Configure`, `Metadata`, and `Schema` methods — matching the pattern used by `firewall`, `databasemysqlv2`, and other migrated Linode resources.

### `migrated/resource_test.go`

The test file is functionally unchanged from the original because:

- It already uses `acceptance.ProtoV6ProviderFactories` (mux-compatible).
- No SDKv2 provider-level imports are used inside the test logic.
- `checkMySQLDatabaseExists` and `checkDestroy` access the SDKv2 provider meta only for the MySQL _database_ resource (not the access controls resource itself), so they remain valid.
- `ImportState: true` / `ImportStateVerify: true` steps exercise the new `ImportState` implementation.

The build tags and package declaration (`databaseaccesscontrols_test`) are preserved unchanged.

## Interfaces Satisfied

```go
var _ resource.Resource               = &Resource{}
var _ resource.ResourceWithImportState = &Resource{}
```

Both are declared as compile-time guards in the file.

## No SDKv2 Imports

The migrated `resource.go` imports only:
- `terraform-plugin-framework` and its sub-packages
- `terraform-plugin-framework-timeouts`
- `terraform-plugin-framework-validators`
- `terraform-plugin-log/tflog`
- `linodego`
- Internal `linode/helper` and `linode/helper/databaseshared`
