# Migration notes — openstack_db_database_v1

## Scope

Single-resource partial migration of
`openstack/resource_openstack_db_database_v1.go` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The shared helper
file `db_database_v1.go` and its sibling resource files remain on SDKv2 in
this iteration.

## Pre-flight summary (per-resource think-before-editing)

- **Block decision**: no `MaxItems: 1 + nested Elem` attributes in this
  resource. The only block in the migrated form is the `timeouts` block,
  emitted via `timeouts.Block(ctx, ...)` to preserve practitioner HCL syntax
  (`timeouts { create = "10m" }`) instead of attribute syntax (`timeouts =
  { ... }`). This matches the task's explicit instruction
  ("framework-timeouts package, block form") and the `references/timeouts.md`
  rule that mature SDKv2 providers should keep block form during migration.
- **State upgrade**: none. The SDKv2 resource has no `SchemaVersion` field,
  so no upgraders are required and `ResourceWithUpgradeState` is not
  implemented.
- **Import shape**: passthrough on the resource ID. The composite ID is
  `<instance_id>/<dbName>`, but the SDKv2 form used
  `schema.ImportStatePassthroughContext` and parsed the ID inside `Read`/
  `Delete`. The framework form keeps that shape: `ImportState` calls
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` and
  `Read` parses the ID into `instance_id` + `name`. No identity schema
  introduced — this preserves the user-facing import string semantics.

## Conversion details

### Schema

| SDKv2 | Framework |
|---|---|
| `schema.TypeString` `Optional`, `Computed`, `ForceNew: true` (`region`) | `schema.StringAttribute{Optional, Computed, PlanModifiers: RequiresReplace + UseStateForUnknown}` |
| `schema.TypeString` `Required`, `ForceNew: true` (`name`, `instance_id`) | `schema.StringAttribute{Required, PlanModifiers: RequiresReplace}` |
| Implicit `id` (SDKv2) | Explicit `schema.StringAttribute{Computed, UseStateForUnknown}` |
| `Timeouts: &schema.ResourceTimeout{Create, Delete}` | `Blocks: { "timeouts": timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true}) }` |

`ForceNew` on every user-controlled attribute means Update is effectively
unreachable. The framework still requires the method, so it re-emits the
plan as state — which also keeps timeout-block-only changes from causing
spurious diffs.

### Timeouts

- Imported `github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts`.
- `Timeouts timeouts.Value \`tfsdk:"timeouts"\`` field on the model.
- Inside `Create` and `Delete`: read configured timeout via
  `plan.Timeouts.Create(ctx, 10*time.Minute)` /
  `state.Timeouts.Delete(ctx, 10*time.Minute)`, then derive a
  `context.WithTimeout` and pass that ctx to the API helpers and the
  poll wait.
- The 10-minute defaults match the SDKv2 `schema.DefaultTimeout(10*time.Minute)`
  values exactly, so practitioners get unchanged behaviour when the
  `timeouts` block is omitted.
- Block form (not attribute form) selected per task requirement and the
  skill's backward-compatibility guidance for migrations.

### CRUD

- `CreateContext` etc. became `Create`/`Read`/`Update`/`Delete` with
  `(ctx, req, resp)` signatures and typed `databaseDatabaseV1Model` access.
- `diag.Errorf(...)` and `diag.FromErr(...)` translate to
  `resp.Diagnostics.AddError("summary", err.Error())`.
- `d.SetId(fmt.Sprintf("%s/%s", instanceID, dbName))` becomes
  `plan.ID = types.StringValue(...)`.
- `d.SetId("")` in the SDKv2 `Read` drift path becomes
  `resp.State.RemoveResource(ctx)`.
- `GetRegion(d, config)` (which takes `*schema.ResourceData`) cannot be used
  directly; replaced with explicit logic: prefer the configured plan/state
  region, fall back to `r.config.Region`. This preserves the same precedence.
- `CheckDeleted` (which mutates `*schema.ResourceData`) is replaced inline in
  `Delete` with a direct `gophercloud.ResponseCodeIs(err, 404)` check; this
  matches the helper's behaviour (404 → no-op, otherwise surface error).
  It is OK that `Read` does not call `CheckDeleted`: the existing
  `databaseDatabaseV1Exists` helper already returns `false` (no error) when
  the parent instance returns 404, so the drift path is exercised.

### `retry.StateChangeConf` replacement

- The migrated file does **not** import `terraform-plugin-sdk/v2/helper/retry`
  (negative-gate-clean).
- Replaced `retry.StateChangeConf{...}.WaitForStateContext` with a small
  inline poller (`waitForDatabaseV1Database`) following the canonical pattern
  in `references/resources.md`.
- Defined a fresh refresh function `databaseDatabaseV1FrameworkRefreshFunc`
  that returns `func() (any, string, error)` (no `retry.StateRefreshFunc`).
  The original `databaseDatabaseV1StateRefreshFunc` in `db_database_v1.go`
  is left untouched because the rest of the package is still SDKv2; renaming
  avoids a duplicate-symbol compile error during the partial migration.
  When the rest of the database package is migrated, the SDKv2 helper can be
  deleted and the framework refresh func renamed back.

### Import

- `Importer: &schema.ResourceImporter{StateContext:
  schema.ImportStatePassthroughContext}` becomes
  `resource.ResourceWithImportState.ImportState` calling
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
- After import, framework calls `Read`, which parses the composite ID via
  `parsePairedIDs` and re-populates `instance_id` / `name` / `region`.

## Tests

`resource_openstack_db_database_v1_test.go` is updated to use
`ProtoV6ProviderFactories` (referenced as `testAccProtoV6ProviderFactories`).
The existing helper functions (`testAccCheckDatabaseV1DatabaseExists`,
`...Destroy`, `testAccDatabaseV1DatabaseBasic`) are unchanged in behaviour
beyond the test-step factories switch. The HCL config gains a `timeouts`
block to exercise the new block form.

The import test file (`import_openstack_db_database_v1_test.go`) is **not**
included in this output but should receive the same factories switch when
the rest of the database resources are migrated; it currently still uses
`ProviderFactories: testAccProviders`.

### Outstanding TDD-gate dependency

The repo today defines only `testAccProviders` (SDKv2). For this single-
resource migration to actually compile and run, the project needs a
`testAccProtoV6ProviderFactories` map exposing the muxed provider — that
introduction belongs to the broader provider-level migration step (workflow
step 3, "Serve your provider via the framework") and is therefore outside
this resource's scope. The migrated test file is written against that
forthcoming symbol so the TDD red-then-green ordering holds: it will fail
to compile until step 3 is in place, then pass.

## Out of scope (deliberately not changed)

- `db_database_v1.go` (helper file) — still SDKv2, still imports
  `terraform-plugin-sdk/v2/helper/retry`. Rather than mutate a file shared
  with as-yet-unmigrated resources, the migrated resource defines its own
  retry-free refresh helper.
- `import_openstack_db_database_v1_test.go` — left as SDKv2; needs the same
  factories switch later.
- Provider registration (`provider.go`, `main.go`) — needs the
  `NewDatabaseDatabaseV1Resource` constructor to be added to the framework
  provider's `Resources` list and the SDKv2 entry removed, but that edit
  belongs to the provider-level migration step.
