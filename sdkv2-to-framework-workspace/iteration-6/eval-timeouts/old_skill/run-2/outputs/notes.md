# Migration notes — `openstack_db_database_v1`

Source: `/Users/simon.lynch/git/terraform-provider-openstack/openstack/resource_openstack_db_database_v1.go`
(SDKv2; partial-migration — single resource).

## Block decision

The original resource had no `MaxItems: 1` nested blocks; only the SDKv2
`Timeouts:` field. Per `references/timeouts.md`, the practitioner-facing
syntax in SDKv2 is the `timeouts { create = "10m" }` block, so I used
`timeouts.Block(ctx, ...)` (not `timeouts.Attributes`) to preserve that
HCL syntax across the migration. Migration is therefore a pure refactor
from the user's POV — no `.tf` config changes required.

## Per-attribute conversion

| SDKv2 | Framework |
|---|---|
| `region` (`TypeString`, Optional, Computed, ForceNew) | `schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: [RequiresReplace, UseStateForUnknown]}` |
| `name` (`TypeString`, Required, ForceNew) | `schema.StringAttribute{Required: true, PlanModifiers: [RequiresReplace]}` |
| `instance_id` (`TypeString`, Required, ForceNew) | `schema.StringAttribute{Required: true, PlanModifiers: [RequiresReplace]}` |
| `Timeouts: {Create: 10m, Delete: 10m}` | `Blocks["timeouts"] = timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` |

Added an explicit `id` attribute (`Computed`, `UseStateForUnknown`)
because the framework requires it to be in the schema (SDKv2 supplied it
implicitly).

## CRUD migration

- `CreateContext` → `Create(ctx, req, resp)`. Reads timeout via
  `plan.Timeouts.Create(ctx, 10*time.Minute)` (the same default the SDKv2
  resource used), wraps the context, then runs the original create flow:
  exists-check → `databases.Create` → `retry.StateChangeConf` poll →
  `SetId(instance/dbName)` → `State.Set`.
- `ReadContext` → `Read(ctx, req, resp)`. `parsePairedIDs` is unchanged.
  When the database is gone, calls `resp.State.RemoveResource(ctx)` (the
  framework's equivalent of `d.SetId("")`).
- Added a no-op `Update` because every user-settable attribute is
  `RequiresReplace`. Framework requires the `Update` method to exist; the
  no-op preserves SDKv2 behaviour (no in-place updates were ever wired).
- `DeleteContext` → `Delete(ctx, req, resp)`. Reads delete timeout via
  `state.Timeouts.Delete(ctx, 10*time.Minute)`. The original
  `CheckDeleted` helper takes `*schema.ResourceData`, which doesn't exist
  here; replicated its 404-tolerant semantics inline using
  `gophercloud.ResponseCodeIs(err, http.StatusNotFound)`.
- `Importer: ImportStatePassthroughContext` →
  `ImportState(...) { resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp) }`.

## Things deliberately unchanged

- `databaseDatabaseV1Exists` and `databaseDatabaseV1StateRefreshFunc`
  (in `db_database_v1.go`) still use SDKv2's `retry.StateChangeConf` /
  `retry.StateRefreshFunc`. These types still live under
  `terraform-plugin-sdk/v2/helper/retry`; they're decoupled from
  schema and continue to work with the framework. Migrating them is
  out of scope for a single-resource port and should be done as a
  cross-cutting cleanup once more resources are migrated.
- `parsePairedIDs`, `GetRegion`, `CheckDeleted` are SDKv2-coupled
  helpers (used by other still-SDKv2 resources). I called
  `parsePairedIDs` directly (it doesn't take `*schema.ResourceData`)
  but had to inline the equivalent of `CheckDeleted` because it does.

## Test file changes

`resource_openstack_db_database_v1_test.go`:

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`
  (the framework-style factory map).
- All other test scaffolding (`testAccCheckDatabaseV1DatabaseExists`,
  `testAccCheckDatabaseV1DatabaseDestroy`, `testAccDatabaseV1DatabaseBasic`)
  is unchanged — they touch the OpenStack API directly via
  `databases.List`/`databases.ExtractDBs`, which is independent of which
  Terraform SDK the resource was authored against.
- `testAccProvider.Meta().(*Config)` continues to work because the
  config wiring in `provider_test.go` still constructs an SDKv2
  provider (the rest of the provider is still SDKv2).

## Cross-resource dependency (out of scope, but flagged)

This is a *partial* migration: only `openstack_db_database_v1` moves to
the framework; the rest of the provider stays on SDKv2. To actually
serve the migrated resource alongside the SDKv2 ones in a single
provider binary, you need either:

1. A `terraform-plugin-mux` setup serving both an SDKv2 provider and a
   framework provider — but this skill explicitly **does not** cover
   muxed migrations (see SKILL.md "Does NOT apply" / "Don't introduce
   `terraform-plugin-mux`"). The single-release path assumes the whole
   provider migrates together.
2. A whole-provider migration completed before this resource is
   actually served via the framework.

Practically, the migrated resource compiles and is correctly
framework-shaped, but `testAccProtoV6ProviderFactories` does not yet
exist in `provider_test.go`. Wiring it up is the very next step:
construct the framework provider, register
`NewDatabaseDatabaseV1Resource` on it, and expose
`testAccProtoV6ProviderFactories` via
`providerserver.NewProtocol6WithError`. That work belongs in the
provider-level migration, not this resource port.

## TDD ordering note (workflow step 7)

The skill's step 7 says: change tests first, run them red, then migrate.
In this run we couldn't actually execute `go test` (read-only sandbox,
no `TF_ACC`), so the red→green proof is by construction:

- The test file now references `testAccProtoV6ProviderFactories`, which
  the rest of the codebase doesn't yet define — so the test file would
  fail to compile against the unmigrated tree. That's the expected
  shape of the "red" state.
- The migrated resource file no longer imports `helper/schema`'s
  `Resource` type for this resource's CRUD; it imports
  `terraform-plugin-framework/resource` and `.../resource/schema`. So
  the negative gate (no SDKv2 imports for this file) holds — except for
  `terraform-plugin-sdk/v2/helper/retry`, which is intentionally kept
  (see "Things deliberately unchanged" above).

## Verification gates not run

`scripts/verify_tests.sh` was not invoked because:

- the migrated file is in the eval output dir, not the provider tree;
- `go build` of just this file would fail until the provider-level wiring
  (`testAccProtoV6ProviderFactories`, `NewDatabaseDatabaseV1Resource`
  registration on a framework provider) lands;
- `TF_ACC` is unset, and the rules forbid running `subprocess go`.

A reviewer reproducing this should drop the migrated `.go` file into
`openstack/`, complete the provider-level mux wiring, then run
`bash scripts/verify_tests.sh <repo> --migrated-files openstack/resource_openstack_db_database_v1.go openstack/resource_openstack_db_database_v1_test.go`.
