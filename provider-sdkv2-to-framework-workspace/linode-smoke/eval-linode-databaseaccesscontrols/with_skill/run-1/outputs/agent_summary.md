# Migration Summary: linode_database_access_controls

## Files Produced

| Path | Description |
|---|---|
| `migrated/resource.go` | Framework resource implementation; no SDKv2 import |
| `migrated/resource_test.go` | Updated acceptance tests using `ProtoV6ProviderFactories` and composite-ID import helper |

## Workflow Steps Run

- **Pre-flight 0**: No mux keywords in task; single-release path confirmed.
- **Pre-flight A**: `audit_sdkv2.sh` run against the provider. Key findings for this resource: `schema_resource.go` has `ForceNew:true` and `ValidateDiagFunc`; `resource.go` has a custom `Importer` flagged for manual review.
- **Pre-flight B**: Scope confirmed as single resource (`linode/databaseaccesscontrols`).
- **Pre-flight C**: Think pass completed (see Notable Decisions below).
- **Steps 1–6**: Source read, framework-migrated linode resources (firewall, vpc, databasemysqlv2) examined as style reference. No source files were modified.
- **Step 7**: Test file updated to use `ProtoV6ProviderFactories`, `TestCheckTypeSetElemAttr` (framework-compatible set element check), and an explicit `ImportStateIdFunc`.
- **Step 8**: Resource migrated (see below).
- **Steps 9–11**: `gofmt -e` confirmed no syntax errors on both output files. `go build ./linode/...` on the source repo is green (unmodified).

## Notable Decisions

### Block decision
`allow_list` was `TypeSet` with `Elem: &schema.Schema{Type: schema.TypeString}` — a flat set of strings, not a `MaxItems:1` block. It maps directly to `schema.SetAttribute{ElementType: types.StringType}`. No block-vs-attribute decision required.

### ForceNew → RequiresReplace
`database_id` and `database_type` had `ForceNew: true`. Translated to `int64planmodifier.RequiresReplace()` and `stringplanmodifier.RequiresReplace()` respectively.

### database_id type: Int → Int64
SDKv2 used `TypeInt`; the framework uses `Int64Attribute` / `types.Int64`. The `ResourceModel` struct holds `DatabaseID types.Int64`. All conversions from int64 to int use `helper.FrameworkSafeInt64ToInt`.

### ValidateDiagFunc → stringvalidator
`validation.ToDiagFunc(validation.StringInSlice(ValidDatabaseTypes, true))` (case-insensitive) became `stringvalidator.OneOfCaseInsensitive(databaseshared.ValidDatabaseTypes...)`.

### Custom Importer (composite ID)
The original `ImportStatePassthroughContext` importer with `parseID` logic was a passthrough in name only — the resource's `readResource` called `parseID(d.Id())`, meaning the full composite ID `"<dbID>:<dbType>"` had to be stored as the resource ID at all times. The migration keeps this design:
- `formatID`/`parseID` helpers are preserved verbatim.
- `ImportState` parses `req.ID` (the composite string), writes `id`, `database_id`, and `database_type` into partial state, and seeds `allow_list` with an empty set. `Read` runs automatically after `ImportState` and fills in the real allow list.
- The test adds `ImportStateIdFunc` that returns `rs.Primary.ID` (already the composite string) so the import step works end-to-end.

### Timeout handling
The SDKv2 resource had no `Timeouts:` declared but called `d.Timeout(schema.TimeoutUpdate)` (which returns 0 when no timeout is set, effectively making `WaitForFinished` use a zero-second timeout — a latent bug). The migration introduces a `defaultUpdateTimeout = 30 * time.Minute` constant passed explicitly to `updateDBAllowListByEngine`, fixing the latent bug without exposing a user-configurable `timeouts` block (the SKILL.md guidance: "if your provider didn't have Timeouts, don't add it during migration").

### Set equality check
SDKv2's `schema.Set.Equal()` was used to skip no-op updates. The framework model uses `[]string`; the migration provides `stringSlicesEqual` (order-independent map-based check) for the same purpose.

### getDBAllowListByEngine return type
Changed from `*schema.Set` to `[]string`. The API already returns `[]string` (via `db.AllowList`); the SDKv2 version was wrapping it in `schema.NewSet(schema.HashString, ...)` for no semantic reason other than to satisfy the SDKv2 schema type. The framework representation is the plain slice.

### Schema file
The original `schema_resource.go` (SDKv2 `resourceSchema` map) is replaced by `frameworkResourceSchema` declared inline in `resource.go`. A separate `schema_resource.go` output was not produced since the framework style in this provider (firewall, vpc) keeps the schema in the same file or a sibling file. The schema variable is defined at package level in `resource.go`.
