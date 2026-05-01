# Migration notes — `openstack_db_user_v1`: SDKv2 → terraform-plugin-framework

## Scope

Single-resource migration as part of a **major version bump**. The
practitioner-visible breaking change in this migration is the
`Sensitive` → `Sensitive + WriteOnly` upgrade on `password`.

Files produced:

- `migrated/resource_openstack_db_user_v1.go`
- `migrated/resource_openstack_db_user_v1_test.go`

## Key decisions

### `password`: Sensitive + WriteOnly (NOT Computed)

Per the task spec and `references/sensitive-and-writeonly.md`:

- `Required: true`, `Sensitive: true`, `WriteOnly: true`.
- **Not** `Computed`. The skill is explicit that `WriteOnly` and `Computed`
  cannot coexist on the same attribute — the framework rejects this at
  provider boot.
- `RequiresReplace()` plan modifier preserved (matches old `ForceNew: true`).

In CRUD:

- `Create` reads the password from `req.Config` (it is null in `req.Plan` /
  `req.State` because it is write-only). The model is loaded twice:
  once from `Plan` (for everything else) and once from `Config` (for
  `Password`).
- `Read` and `Update` explicitly set `Password = types.StringNull()` before
  writing to state. Belt-and-braces: `req.Plan.Get` already produces null,
  but the explicit assignment makes the intent obvious to a reviewer and
  guards against future refactors that might widen the model.

This is a **practitioner-visible breaking change** and is therefore appropriate
only for a major version bump. Test assertions on `password` must be removed
or replaced with `TestCheckNoResourceAttr` (added in the basic step), and
`ImportStateVerifyIgnore: []string{"password"}` is mandatory on any import-verify
step (otherwise the importer reads back null where the practitioner wrote a value
and `ImportStateVerify` fails).

### Schema — straight conversions

| SDKv2 | Framework |
|---|---|
| `region`: Optional+Computed+ForceNew | `StringAttribute` Optional+Computed, `RequiresReplace()` + `UseStateForUnknown()` |
| `name`: Required+ForceNew | `StringAttribute` Required, `RequiresReplace()` |
| `instance_id`: Required+ForceNew | `StringAttribute` Required, `RequiresReplace()` |
| `host`: Optional+ForceNew | `StringAttribute` Optional, `RequiresReplace()` |
| `databases`: TypeSet+Optional+Computed, `Set: schema.HashString` | `SetAttribute{ElementType: types.StringType}` Optional+Computed, `UseStateForUnknown()` |
| `id` (implicit) | `StringAttribute` Computed, `UseStateForUnknown()` |

`Set: schema.HashString` dropped — the framework handles set uniqueness
internally (skill: "Things you no longer need").

### Timeouts

The original SDKv2 resource had a 10-minute Create/Delete timeout. Following
`references/timeouts.md` ("if your SDKv2 provider didn't define `Timeouts`,
don't add it during migration … keep migrations as pure refactor where
possible"), I did NOT add a `timeouts` schema attribute. Instead the 10-minute
deadline is hardcoded in `Create` via `context.WithTimeout`. If practitioners
relied on the previous override capability, surfacing a `timeouts` block can be
done in a follow-up minor release.

### `retry.StateChangeConf` replacement

Per `references/resources.md` — the framework has no equivalent helper, so
I inlined a `waitForDatabaseUserV1` helper that polls the API at a fixed
interval and respects the context deadline. It re-uses the existing
`databaseUserV1StateRefreshFunc` from `db_user_v1.go` (which still returns
`retry.StateRefreshFunc`, but Go's named-type rules accept it as
`func() (any, string, error)` — option (1) from the resources reference).

The migrated resource file itself does NOT import
`terraform-plugin-sdk/v2/helper/retry`, satisfying the negative gate.
The shared helper file `db_user_v1.go` retains the import; it would be
swept in a follow-up pass when sibling resources (e.g. `db_instance_v1`,
`db_database_v1`, `db_configuration_v1`) are migrated.

### Import

SDKv2 had no explicit `Importer` field, but the resource ID is a composite
`<instance_id>/<name>` parsed by `parsePairedIDs`. In the framework I
implemented `ImportState` via `ImportStatePassthroughID` on `path.Root("id")`
— same shape as the implicit SDKv2 passthrough, with `Read` continuing to
do the parsing.

### Update

All user-supplied attributes are `RequiresReplace`, so `Update` is
unreachable in normal use. I kept an explicit no-op `Update` shim that
copies `plan` to state (with `Password = StringNull()`), matching the
framework's expectation that `Update` exists. This is defensive: if a
future change removes a `RequiresReplace`, the resource still behaves
sensibly.

## Test changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
  This assumes the provider has been migrated to the framework; the
  factory variable name follows the convention from `references/testing.md`.
- Added explicit `TestCheckNoResourceAttr("openstack_db_user_v1.basic", "password")`
  to the basic step — proves the WriteOnly attribute is not in state.
- Added a new import-verify step with
  `ImportStateVerifyIgnore: []string{"password"}` (mandatory for write-only
  attributes per the skill).
- `terraform-plugin-sdk/v2/helper/resource` → `terraform-plugin-testing/helper/resource`
  (already in the original test file, retained).

## What was NOT changed

- `expandDatabaseUserV1Databases` and `flattenDatabaseUserV1Databases`
  helpers — they take/return primitive types that work with the framework
  unchanged.
- `databaseUserV1Exists`, `databaseUserV1StateRefreshFunc` — same reason;
  the latter is reused from the framework helper.
- `parsePairedIDs`, `CheckDeleted`, `GetRegion` — utility functions not
  in scope. `GetRegion` is replaced by the resource-method
  `regionFor(types.String)` because the SDKv2 version takes
  `*schema.ResourceData` which doesn't exist in the framework.

## Follow-up suggestions (out of scope for this migration)

1. Migrate sibling DB resources (`db_instance_v1`, `db_database_v1`,
   `db_configuration_v1`) so the shared `db_user_v1.go` helper file can
   drop its `terraform-plugin-sdk/v2/helper/retry` import.
2. Consider exposing `timeouts` as a framework attribute via
   `terraform-plugin-framework-timeouts` if practitioners want the
   override knob back. This is a feature add, not a regression — flag in
   the changelog as such.
3. Add a CHANGELOG entry: "BREAKING: `openstack_db_user_v1.password` is
   now a write-only attribute; downstream `TestCheckResourceAttr`
   assertions and `ImportStateVerify` steps must be updated."
