# notes.md — `openstack_db_database_v1` SDKv2 → plugin-framework

## Scope

Per the task, only `resource_openstack_db_database_v1.go` and its test file
were migrated. The rest of the provider remains on SDKv2 — it would still
need to be served via `terraform-plugin-mux` or fully migrated for these
files to compile in-tree. The migrated artefacts are written so that, once
the surrounding provider migration lands, they drop in unchanged.

## Source resource at a glance

The SDKv2 resource (`resource_openstack_db_database_v1.go`) is small and
straightforward:

- 3 user-facing attributes: `region` (Optional+Computed+ForceNew), `name`
  (Required+ForceNew), `instance_id` (Required+ForceNew).
- Importer: `schema.ImportStatePassthroughContext` (composite ID is
  `<instance_id>/<name>`).
- `Timeouts: &schema.ResourceTimeout{Create: 10m, Delete: 10m}`.
- CRUD: Create blocks on `retry.StateChangeConf` (BUILD → ACTIVE),
  Read calls `databaseDatabaseV1Exists`, Delete calls `databases.Delete`.

Because every attribute is `ForceNew`, the resource has no real Update
path under SDKv2; the framework version still has to implement the
`Update` method for interface compliance, but it is never invoked.

## Translation decisions

### Timeouts — `Block`, not `Attributes`

The task explicitly calls for preserving the existing
`timeouts { create = "..." }` HCL block syntax. The skill's
`references/timeouts.md` covers this exactly:

> To preserve [block] syntax across the migration, use `timeouts.Block(...)`
> instead — same `Opts` argument, but it goes into the schema's
> `Blocks:` map rather than `Attributes:`.

So the schema places `timeouts` under `Blocks`, with
`timeouts.Opts{Create: true, Delete: true}` to match the SDKv2
`ResourceTimeout`'s populated fields exactly. Update/Read are *not*
enabled, mirroring the SDKv2 shape.

The `databaseDatabaseV1Model` struct's `Timeouts` field is
`timeouts.Value` with the `tfsdk:"timeouts"` tag.

In `Create`, the timeout is read with
`plan.Timeouts.Create(ctx, 10*time.Minute)` — the `10m` matches the
SDKv2 `schema.DefaultTimeout(10 * time.Minute)`. The same pattern applies
to `Delete` (`state.Timeouts.Delete(ctx, 10*time.Minute)`). Both wrap the
context with `context.WithTimeout` so downstream gophercloud calls and the
`retry.StateChangeConf` honour the deadline.

The `retry.StateChangeConf.Timeout` is set to the same `createTimeout`
duration — this was previously `d.Timeout(schema.TimeoutCreate)` in
SDKv2 and is the load-bearing equivalent in the framework idiom.

### ForceNew → `RequiresReplace()` plan modifier

Per the skill's "common pitfalls" and `plan-modifiers.md`:

> `ForceNew: true` does NOT translate to `RequiresReplace: true`. It
> becomes a plan modifier:
> `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`.

All three user-facing attributes (`region`, `name`, `instance_id`)
get `stringplanmodifier.RequiresReplace()`. `region` additionally gets
`UseStateForUnknown()` so unconfigured-region practitioners don't see
`(known after apply)` on every plan after the first.

### Computed `id`

SDKv2 implicitly provides an `id` field; the framework requires it to be
declared explicitly. It is `Computed: true` with
`stringplanmodifier.UseStateForUnknown()` so the composite
`<instance_id>/<name>` value persists across plans without the noisy
`(known after apply)` line.

### Importer → `ResourceWithImportState`

Per `references/import.md`:
- SDKv2 `Importer.StateContext: ImportStatePassthroughContext` →
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
- The composite ID is treated as opaque at import time (matching SDKv2's
  passthrough). `Read` is what splits `instance_id/name` and populates the
  model fields — no API call from `ImportState`, which `references/import.md`
  warns against.

### Read — drift handling

SDKv2's `d.SetId("")` becomes `resp.State.RemoveResource(ctx)` per
`references/resources.md`'s "Read drift handling" section. This signals
to Terraform that the resource is gone and should be re-created on the
next plan, rather than leaving stale state.

### Delete — `CheckDeleted` behaviour preserved

The SDKv2 path used `CheckDeleted(d, err, "...")` which silently absorbs
HTTP 404s (gophercloud's `ResponseCodeIs(err, http.StatusNotFound)`). The
framework version inlines that check after `databases.Delete(...).ExtractErr()`
so a 404-on-delete still returns no error.

### Region resolution

`util.go`'s `GetRegion(d, config)` checks `d.GetOk("region")` then falls
back to `config.Region`. The framework helper `getRegionFromPlan` (defined
locally in this file to avoid coupling to `util.go`'s SDKv2-typed
signature) does the same against the typed model: if `planRegion` is null
or unknown or empty, fall back to `config.Region`.

This avoids editing `util.go` (which is shared with the rest of the
SDKv2 provider) and keeps the migration scoped to this resource.

### `Configure` — type-asserting `*Config`

Standard pattern from `references/resources.md`. Returns early on null
provider data (which happens during validation passes), and produces a
clear diagnostic if the assertion fails.

## Test file changes

- `ProviderFactories: testAccProviders` →
  `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. The new
  factory map name follows the convention used by HashiCorp's docs and the
  skill's `references/testing.md`. It does not exist in this branch yet —
  it will be added by the surrounding provider migration; flagged in the
  migrated test file with a comment.
- Added an `ImportState`/`ImportStateVerify` step to round-trip the
  composite ID through the new `ImportStatePassthroughID` path. The
  expected fidelity comparison covers `region`, `name`, `instance_id`,
  and the `id` itself. (No `ImportStateIdFunc` is needed because the
  resource's primary ID *is* the composite — the
  `ImportStatePassthroughContext` in the SDKv2 version did the same.)
- Added an explicit `timeouts { create = "10m" delete = "10m" }` block
  in the test HCL to exercise the new `timeouts.Block(...)` schema entry
  end-to-end. Confirms the practitioner-facing block syntax is preserved.
- Helper functions (`testAccCheckDatabaseV1DatabaseExists`,
  `testAccCheckDatabaseV1DatabaseDestroy`, `testAccDatabaseV1DatabaseBasic`)
  are unchanged in shape — they take a `*terraform.State` and reach into
  the gophercloud API, which is unaffected by the framework migration.

## What was deliberately NOT done

- No edits to `provider.go`, `util.go`, or `provider_test.go`. The task
  scoped this to a single resource. The migrated file is therefore a
  bring-your-own-provider artefact — it cannot be wired in until the
  surrounding provider exposes a framework provider type and the
  `testAccProtoV6ProviderFactories` map.
- No edits to `db_database_v1.go` (the helpers
  `databaseDatabaseV1Exists` / `databaseDatabaseV1StateRefreshFunc`).
  They still take `context.Context` and a `*gophercloud.ServiceClient`,
  which is provider-agnostic; they work unchanged from the framework
  resource.
- No removal of the SDKv2 import. `db_database_v1.go` still imports
  `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry` for
  `retry.StateChangeConf` and `retry.StateRefreshFunc`. The migrated
  resource reuses these same types — `retry.StateChangeConf` is also
  re-exported in newer framework-compatible packages, but switching it
  is out-of-scope for this single-resource migration. (See
  `references/deprecations.md` for the eventual replacement when the
  whole provider moves off SDKv2.)
- No `Update` body. Every attribute is `ForceNew` (and now uses
  `RequiresReplace()`), so any change replaces the resource. The empty
  `Update` method satisfies the `resource.Resource` interface and will
  never run in practice.

## Verification (suggested next steps when the harness allows)

Following the skill's verification gates (read-only mode here, but for
a real run):

1. `go build ./...` — would fail in this branch because the surrounding
   provider isn't migrated.
2. `go vet ./...` — same.
3. `TestProvider` — n/a until the framework provider type exists.
4. The negative gate: `resource_openstack_db_database_v1.go` no longer
   imports `terraform-plugin-sdk/v2/helper/schema` or
   `terraform-plugin-sdk/v2/diag`. (It does still import
   `terraform-plugin-sdk/v2/helper/retry`, which is intentional — see
   above.)
5. `TF_ACC=1 go test -run TestAccDatabaseV1Database_basic` — only
   meaningful with credentials and a migrated provider.

The negative gate above is the relevant signal: this resource file no
longer pulls in any of the SDKv2 schema/diag types — every type comes
from `terraform-plugin-framework*` packages, except `retry` which is a
deliberately-retained helper-package dependency.
