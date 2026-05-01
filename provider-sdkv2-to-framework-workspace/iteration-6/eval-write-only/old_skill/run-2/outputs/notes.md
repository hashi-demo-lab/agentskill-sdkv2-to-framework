# Migration notes — `openstack_db_user_v1` (sdkv2 → plugin-framework)

## Scope

Single-resource migration of `openstack/resource_openstack_db_user_v1.go`
from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`, with a
**major-version-bump-only** schema change: `password` becomes
`Sensitive` + `WriteOnly` (NOT `Computed`).

## Key decisions

### password: Sensitive + WriteOnly, NOT Computed

- The framework forbids `WriteOnly` + `Computed` on the same attribute. A
  write-only value isn't persisted, so making it computed would require the
  framework to materialise it in state — contradiction. (See
  `references/sensitive-and-writeonly.md` "Hard rules".)
- `WriteOnly` requires `terraform-plugin-framework` v1.14.0+ (technical
  preview); v1.17.0+ recommended. Terraform 1.11+ is required at the CLI side.
- Read the value from `req.Config` inside `Create`. `req.Plan` and `req.State`
  carry it as null, by design.
- This is a practitioner-test-breaking change — any existing acceptance test
  that asserted `resource.TestCheckResourceAttr(..., "password", "...")` will
  now fail. The migrated test uses `TestCheckNoResourceAttr` for the apply
  step and `ImportStateVerifyIgnore: []string{"password"}` for the import
  step. This is non-optional pairing.

### ForceNew → RequiresReplace plan modifier

Every attribute on the original resource was `ForceNew: true`. Each translates
to `stringplanmodifier.RequiresReplace()` (or the equivalent for set/etc.).
`ForceNew` does NOT translate to a `RequiresReplace: true` schema field —
common pitfall called out in SKILL.md.

### Region: Optional + Computed + RequiresReplace + UseStateForUnknown

Mirrors the SDKv2 shape (`Optional + Computed + ForceNew`). The
`UseStateForUnknown` plan modifier prevents spurious replace-on-no-op when the
practitioner relies on the provider-level region default.

### `databases`: SetAttribute (not block)

The SDKv2 schema used `TypeSet` of strings with `schema.HashString`. The
hash function is gone in the framework — `SetAttribute` handles uniqueness
internally. (Common pitfall called out in SKILL.md.) `Optional + Computed`
matches the original shape.

### `Set: schema.HashString` removed

Confirmed dropped per SKILL.md "Common pitfalls": the framework's
`SetAttribute` handles uniqueness internally.

### Timeouts: `timeouts.Attributes(...)` (not Block)

The original `Timeouts: &schema.ResourceTimeout{Create: 10m, Delete: 10m}`
is migrated to the framework timeouts package
(`terraform-plugin-framework-timeouts`) with `Create` + `Delete` only —
matching the original surface.

### Import: passthrough with composite ID (instance_id/user_name)

`ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`. `Read` already
parses the composite via `parsePairedIDs`, so passthrough is sufficient.

### Update method

Every attribute is RequiresReplace, so `Update` is unreachable in practice.
The `resource.Resource` interface still requires the method, so it's
implemented as a state-passthrough no-op.

### 404 handling in Delete

Mirrors the SDKv2 `CheckDeleted` 404-swallowing behaviour: if the API
returns 404 during `users.Delete`, the resource is treated as already-gone.

## Bundled helpers reused unchanged

The following helpers are defined in `db_user_v1.go` and called as-is from
the migrated CRUD code:

- `expandDatabaseUserV1Databases([]any) databases.BatchCreateOpts`
- `flattenDatabaseUserV1Databases([]databases.Database) []string`
- `databaseUserV1StateRefreshFunc(...) sdkretry.StateRefreshFunc`
- `databaseUserV1Exists(...) (bool, users.User, error)`

The migrated resource still imports `terraform-plugin-sdk/v2/helper/retry`
because `databaseUserV1StateRefreshFunc` returns
`retry.StateRefreshFunc` and `StateChangeConf` lives in the same SDKv2
package. Eliminating that import requires either re-exporting the type via
a small wrapper or migrating to a framework-native polling helper — out of
scope for this single-file migration. It will need to be addressed when
finishing the provider-wide migration before step 10 (remove all SDKv2
references).

A small local helper `setStringsToAnySlice(ctx, types.Set) ([]any, diag.Diagnostics)`
adapts the framework set into the `[]any` shape the existing
`expandDatabaseUserV1Databases` expects, so the helper file does not need
to be touched.

## Test changes

- `ProviderFactories` → `ProtoV6ProviderFactories` (assumes
  `testAccProtoV6ProviderFactories` exists at the provider level — it will
  once the provider is served via the framework, which is workflow step 3).
- `testAccProvider.Meta().(*Config)` → `testAccFrameworkProviderConfig()`
  (helper assumed to exist at the provider level; if not, the test calls
  for a small helper that builds a `*Config` from the same env vars the
  framework provider uses in its `Configure`).
- Added a second test step exercising
  `ImportState + ImportStateVerify + ImportStateVerifyIgnore: []string{"password"}`.
- Added `resource.TestCheckNoResourceAttr(..., "password")` on the apply
  step — proves WriteOnly behaviour: the value is in config but null in
  state.

## Known gaps / follow-ups (provider-wide work, not this file)

1. The provider must be served via protocol v6 (`main.go` swap to
   `providerserver.NewProtocol6WithError`). Without this, registering this
   resource has no effect.
2. `testAccProtoV6ProviderFactories` and `testAccFrameworkProviderConfig`
   need to exist alongside the existing SDKv2 `testAccProviders`. During the
   single-release migration both should coexist until step 10.
3. The `terraform-plugin-sdk/v2/helper/retry` import will need to be
   removed once all callers of `databaseUserV1StateRefreshFunc` are
   migrated (it can be replaced with a small framework-friendly polling
   helper).
4. Documentation (`docs/resources/db_user_v1.md` if present) should call
   out the breaking change: `password` is no longer readable from state.
   Add a CHANGELOG entry: `BREAKING CHANGE: openstack_db_user_v1.password is
   now write-only and is not persisted to state. Reading the value via
   data sources or `terraform output` is no longer supported.`
