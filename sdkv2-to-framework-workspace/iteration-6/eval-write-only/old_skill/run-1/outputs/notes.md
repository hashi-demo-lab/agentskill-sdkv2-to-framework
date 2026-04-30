# Migration notes — `openstack_db_user_v1`

Migrating a single SDKv2 resource to `terraform-plugin-framework`. Major version
bump, so practitioner-visible breaking changes to credentials handling are
acceptable.

## Decisions

### `password`: `Sensitive` → `Sensitive + WriteOnly`
- Previously `Required + ForceNew + Sensitive` and stored in state. Convert to
  `Required + Sensitive + WriteOnly`. Per
  `references/sensitive-and-writeonly.md`:
  - Available since framework v1.14 (technical preview), v1.17+ recommended
    for production. Repo's `go.mod` already pins
    `terraform-plugin-framework v1.17.0` — ✓ no version bump needed.
  - `WriteOnly` cannot be combined with `Computed` (framework rejects at
    boot). Kept as `Required` only.
  - `WriteOnly` value is read from `req.Config` in `Create`, never from
    `req.Plan`/`req.State`. State is set with `Password: types.StringNull()`.
  - This is a breaking practitioner change (any test that
    `TestCheckResourceAttr(...,"password", ...)` will now fail because the
    attribute is null in state). Acceptable per task brief — major version
    bump.
- `ForceNew` semantics preserved via
  `stringplanmodifier.RequiresReplace()` plan modifier.

### `databases`: `TypeSet + Set: schema.HashString` → `SetAttribute`
- Drop `Set: schema.HashString` — framework `SetAttribute` handles uniqueness
  internally. Per "common pitfalls" in SKILL.md.
- Element type is `types.StringType`. Optional+Computed retained (server may
  return additional databases).

### `region`: `Optional + Computed + ForceNew`
- Plan modifiers: `RequiresReplace()` (preserve ForceNew) plus
  `UseStateForUnknown()` so plans don't show
  `(known after apply)` every refresh.

### `name`, `instance_id`, `host`: `ForceNew` → `RequiresReplace`
- One-to-one plan-modifier translation per `references/plan-modifiers.md`.

### `id`: explicit `Computed: true` with `UseStateForUnknown()`
- SDKv2 inferred `id` automatically; framework requires it as an attribute on
  the schema and the typed model.

### `Timeouts`: SDKv2 `&schema.ResourceTimeout{...}` → framework
  `terraform-plugin-framework-timeouts`
- Used `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` to
  preserve the existing HCL block syntax (`timeouts { create = "10m" }`).
  `timeouts.Attributes(...)` would be a quieter HCL change (`timeouts =
  {...}`) but a syntactic break for any practitioner already using the
  block. Per `references/timeouts.md`.
- Defaults (10m create, 10m delete) live in CRUD methods, not on the schema.

### Import support
- The SDKv2 file did not have `Importer:` set. The task brief said to add
  `ImportStateVerifyIgnore` to the test, which implies an import-verify step.
  Implemented `ImportState` parsing the existing composite ID
  (`<instance_id>/<user_name>`) and writing `id`, `instance_id`, `name` into
  state. `Read` then populates the rest from the API.
- This adds capability that wasn't in the SDKv2 version. It's strictly
  additive and matches the resource's existing ID shape.

### Resource interface assertions
- Compile-time interface guards via `var _ resource.Resource = ...` etc., per
  `references/resources.md`. Catches a missing method as a build failure
  rather than a panic at provider start.

## Things deliberately NOT changed

- Helper functions `expandDatabaseUserV1Databases`, `flattenDatabaseUserV1Databases`,
  `databaseUserV1Exists`, `databaseUserV1StateRefreshFunc`, `parsePairedIDs`,
  `Config.DatabaseV1Client` are reused as-is — they don't depend on SDKv2
  types except that `databaseUserV1StateRefreshFunc` returns
  `retry.StateRefreshFunc` from `terraform-plugin-sdk/v2/helper/retry`. The
  framework migration keeps this dependency because `retry.StateChangeConf` is
  a perfectly reasonable polling helper that doesn't conflict with framework
  resources. Migrating `helper/retry` away is a provider-wide concern out of
  scope for this single resource.
- The provider entry point (`provider.go`) is not modified. The framework
  resource is exported as `NewDatabaseUserV1Resource` so the provider can
  register it via plugin-mux later.
- The test still uses `ProviderFactories: testAccProviders`. In a fully
  migrated provider this would become `ProtoV6ProviderFactories`; for a
  single-resource partial migration before plugin-mux is wired in, the
  practitioner-facing harness is unchanged. Note added in the test file.

## Test changes

- Added a `ImportState` step with `ImportStateVerifyIgnore: []string{"password"}`
  per the task brief — this is the canonical pairing for write-only attributes.
  Without it, ImportStateVerify would fail comparing the post-import null
  against the config-supplied value (`references/sensitive-and-writeonly.md`
  "Test treatment").
- Added `resource.TestCheckNoResourceAttr(..., "password")` to the basic
  step's `Check` to assert WriteOnly's contract end-to-end (the value is null
  in state).

## Verification gates that would normally run

I cannot run `bash <skill-path>/scripts/verify_tests.sh` against this output
directory because the migrated files live outside the provider repo (the
eval is read-only on the openstack repo). In a real run I would:

1. Drop the migrated `.go` next to the SDKv2 file in
   `terraform-provider-openstack/openstack/`.
2. Wire `NewDatabaseUserV1Resource` into the provider (plugin-mux —
   out of scope here).
3. `go build ./...` — must compile.
4. `go vet ./...`.
5. `TestProvider` — boots the provider, surfaces `WriteOnly + Computed`
   conflicts, missing-attribute errors, etc. Critical for write-only because
   the framework rejects `WriteOnly + Computed` at boot.
6. `TF_ACC=1 go test -run '^TestAccDatabaseV1User_basic$' ./openstack/...`
   if creds available.
7. Negative gate via `verify_tests.sh --migrated-files
   resource_openstack_db_user_v1.go`: confirms the migrated file no longer
   imports `terraform-plugin-sdk/v2` for resource concerns. (The test file
   still imports `terraform-plugin-testing` which is fine; the resource file
   imports `terraform-plugin-sdk/v2/helper/retry` which is documented above
   as a deliberate carry-over.)

## Pitfalls hit during migration (none surprising; recorded for the eval)

- Initial draft tried to reuse `flattenDatabaseUserV1Databases`'s `[]string`
  return value via a one-shot helper — collapsed to a direct
  `types.SetValueFrom(ctx, types.StringType, names)` call instead.
- Empty `PlanModifiers: []planmodifier.Set{}` left in the schema as a
  placeholder. Removed — empty slices don't trigger errors but are noise.
- `WriteOnly + Required` is correct. Initially considered `Optional` to mirror
  the "may not always be set" intent — but the resource's CreateOpts requires
  a password value, so leave it `Required`.
