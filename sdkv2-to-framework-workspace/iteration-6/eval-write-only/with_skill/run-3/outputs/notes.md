# Migration notes — `openstack_db_user_v1` (SDKv2 → terraform-plugin-framework)

## Scope

Single-resource migration of `openstack/resource_openstack_db_user_v1.go` plus
its acceptance test. This is part of a **major version bump** for the provider,
which lets us tighten the `password` attribute beyond a pure refactor.

## Key decisions

### 1. `password` → `Sensitive` + `WriteOnly` (not `Computed`)

- The previous SDKv2 schema had `password` as `Required + ForceNew + Sensitive`,
  which meant the plaintext password was persisted to state for round-tripping.
- Per `references/sensitive-and-writeonly.md`, the framework adds `WriteOnly`,
  which keeps the value out of state entirely. This is the correct shape for a
  one-time credential the provider does not need to read back.
- **Hard rule**: `WriteOnly` and `Computed` cannot coexist. The framework rejects
  this at provider boot. We deliberately do not set `Computed: true`.
- **Hard rule**: a top-level `WriteOnly` attribute must be `Required` or
  `Optional`. We keep `Required: true`.
- This is a breaking change vs the SDKv2 schema. It is acceptable here because
  the user explicitly framed this migration as a major version bump.
- The value is read from `req.Config` in `Create` (it is *not* in
  `req.Plan`/`req.State`). We then issue the API call and write the rest of the
  state without persisting `password`.

### 2. Test changes for WriteOnly

- Added `ImportStateVerifyIgnore: []string{"password"}` to the import step —
  required because the importer cannot read the value back from state, so the
  default `ImportStateVerify` diff would always fail.
- Added `resource.TestCheckNoResourceAttr(..., "password")` in the apply step
  to assert the value is genuinely absent from state (regression guard against
  accidentally persisting it via `resp.State.Set`).
- Switched `ProviderFactories` → `ProtoV6ProviderFactories`. The factory name
  used (`testAccProtoV6ProviderFactories`) is the conventional name for the
  framework provider's protocol-v6 factory map and is expected to exist as part
  of the broader provider migration.

### 3. `ForceNew` → `RequiresReplace` plan modifier

Every immutable attribute (`region`, `name`, `instance_id`, `password`, `host`)
gets `stringplanmodifier.RequiresReplace()`. Per `references/plan-modifiers.md`,
this is the correct framework idiom — `ForceNew: true` does **not** map to a
top-level `RequiresReplace: true` field.

Because every user-facing attribute is replace-on-change, `Update` is logically
unreachable. We still implement it (the `resource.Resource` interface requires
all CRUD methods) and have it write the plan to state for safety.

### 4. `databases` set: drop `schema.HashString`

Per the common-pitfalls section: framework `SetAttribute` handles uniqueness
internally. The SDKv2 `Set: schema.HashString` field is dropped.

`databases` keeps `Optional + Computed` semantics (the API echoes the list back)
plus `setplanmodifier.UseStateForUnknown()` to avoid spurious "(known after
apply)" diffs on no-op refresh.

### 5. Composite-ID import (`instance_id/name`)

The previous SDKv2 implementation had no explicit `Importer`, but the
`d.SetId(fmt.Sprintf("%s/%s", instanceID, userName))` shape and the existing
`parsePairedIDs` helper indicate composite IDs are expected. Since the test now
adds `ImportState: true`, we implement `ResourceWithImportState` and parse the
`instance_id/name` form. Per `references/import.md`, `ImportState` only writes
enough state for `Read` to find the resource.

### 6. Timeouts preserved as `Block` (not attribute)

The SDKv2 resource defines `Timeouts: &schema.ResourceTimeout{Create: 10m,
Delete: 10m}`. To preserve the existing HCL block syntax (`timeouts { create =
"30m" }`), we use `timeouts.Block(ctx, ...)` from
`terraform-plugin-framework-timeouts` rather than `timeouts.Attributes(...)`.
Default values (10 minutes) are passed at the call site in CRUD methods, per
the package contract.

### 7. Replacing `retry.StateChangeConf`

Per `references/resources.md`, the framework has no equivalent helper. We
inline a 20-line `waitForDatabaseUserV1State` polling loop and avoid importing
`terraform-plugin-sdk/v2/helper/retry` from the migrated file (this would
otherwise fail the negative gate in `verify_tests.sh`).

The shared SDKv2 helper `databaseUserV1StateRefreshFunc` (in
`db_user_v1.go`) currently returns `retry.StateRefreshFunc`. Two options exist:

1. **Quick** — leave the helper untouched and pass an inline closure (chosen
   here, because we are migrating only this resource).
2. **Clean** — change the helper's declared return type to
   `func() (any, string, error)` so neither file imports `helper/retry`. This
   should be done as a follow-up when the surrounding shared helpers are also
   migrated.

The chosen approach keeps the shared helper file out of scope for this
single-resource migration.

### 8. `CheckDeleted` replacement

The SDKv2 `CheckDeleted(d, err, msg)` takes `*schema.ResourceData`, which we
no longer have. Replaced with a small in-file `checkDeletedFramework(err)`
helper that checks for HTTP 404 via `gophercloud`'s `StatusCode()`/`Error404()`
interfaces. This avoids importing SDKv2 from the migrated file.

## Files produced

- `migrated/resource_openstack_db_user_v1.go` — full framework resource
- `migrated/resource_openstack_db_user_v1_test.go` — protocol-v6 test
- `notes.md` — this file

## Out of scope / follow-ups

- Migrating the shared helpers in `db_user_v1.go` (currently returns
  `retry.StateRefreshFunc`). Should be done as a sweep when sibling resources
  are migrated.
- Wiring `NewDatabaseUserV1Resource` into the framework provider's `Resources`
  list — depends on the broader provider migration.
- Removing the `*schema.ResourceData`-shaped `GetRegion` / `CheckDeleted`
  helpers from `util.go` once all callers are migrated.
- Adding a `TestProvider` test that boots the framework provider via
  `protoV6ProviderFactories["openstack"]()` — useful as a cheap schema-validity
  gate. Not required for this single resource but recommended for the overall
  migration.

## Verification

After integrating into the broader provider migration, run:

```sh
bash <skill-path>/scripts/verify_tests.sh /path/to/terraform-provider-openstack \
    --migrated-files openstack/resource_openstack_db_user_v1.go \
                     openstack/resource_openstack_db_user_v1_test.go
```

This runs `go build`, `go vet`, `TestProvider`, the non-acceptance unit tests,
and asserts the migrated files no longer import
`github.com/hashicorp/terraform-plugin-sdk/v2`. With `TF_ACC=1` and live
OpenStack credentials, it also runs `TestAccDatabaseV1User_basic`.
