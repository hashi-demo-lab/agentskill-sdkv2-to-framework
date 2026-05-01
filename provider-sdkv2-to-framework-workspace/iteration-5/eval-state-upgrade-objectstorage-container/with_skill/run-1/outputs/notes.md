# Migration notes — `openstack_objectstorage_container_v1`

## State upgrade — single-step semantics

The most consequential decision in this migration is how the V0 → V1 state
upgrader is translated. Per `references/state-upgrade.md`, the framework's
`UpgradeState` is **single-step**, not chained:

- The framework calls each entry in the map returned by `UpgradeState()`
  *independently* with the matching `PriorSchema`. There is no chain.
- Each entry must produce the *current* schema's state in one call. State
  that lands at the V0 upgrader was never seen by any other upgrader.
- We must not emit V1-intermediate state from a V0 upgrader and rely on
  another upgrader to finish the job.

Because this resource has only one prior version (V0), the framework
upgrader map has exactly one entry: `0 → current`. The SDKv2 upgrader
function (`resourceObjectStorageContainerStateUpgradeV0`) becomes
`upgradeStateFromV0` in `migrate_resource_openstack_objectstorage_container_v1.go`,
and it produces a fully-formed current-schema (`objectStorageContainerV1Model`)
state value in one call.

The transformation V0 → current:

| Field | V0 | V1 (current) | Notes |
|---|---|---|---|
| `versioning` (block) | `TypeSet` of `{type, location}` | renamed to `versioning_legacy` | element shape unchanged; carried verbatim |
| `versioning` (bool) | — | new scalar bool | upgrader sets `false` (matches the SDKv2 upgrader's `rawState["versioning"] = false`) |
| `storage_class` | — | new computed string | no V0 analogue; left null so the next `Read` populates from the API |
| all other attributes | identical | identical | passed through |

### Why we kept the V0 prior schema in a separate file

`priorSchemaV0()`, `objectStorageContainerV1ModelV0`, and
`upgradeStateFromV0` live in
`migrate_resource_openstack_objectstorage_container_v1.go`, mirroring the
SDKv2 source layout. The resource file
(`resource_openstack_objectstorage_container_v1.go`) only contains the call
site — `UpgradeState()` returns a one-entry map keyed at `0`.

## Schema decisions

- **`versioning_legacy` stays as a block** (`SetNestedBlock` with
  `setvalidator.SizeAtMost(1)`), not a `SingleNestedAttribute`. The
  acceptance tests use the block syntax (`versioning_legacy { type = ...,
  location = ... }`), so converting to attribute syntax would be a
  practitioner-visible HCL break. See `references/blocks.md` decision tree:
  practitioner usage in the wild → keep as block.
- **`versioning` ⇄ `versioning_legacy` mutual exclusion** — the SDKv2
  schema used `ConflictsWith` on both sides. In framework idiom, this
  becomes `setvalidator.ConflictsWith(path.MatchRoot("versioning"))` on the
  block side. The framework can't mix-type a `Bool` validator pointing at a
  set, so the constraint lives on the set side; that's sufficient because
  the validators on either side are checked symmetrically.
- **`ForceNew` → `RequiresReplace()` plan modifier** for `region`,
  `storage_policy`, `storage_class`. Combined with `UseStateForUnknown()`
  on the same fields to avoid spurious "(known after apply)" diffs.
- **`Default: false` → `booldefault.StaticBool(false)`**, with the
  attribute marked `Computed` (required for any attribute with a
  `Default`). Applied to `versioning` and `force_destroy`.
- **`schema.HashResource` set hashing is gone** — the framework computes
  set uniqueness internally. Read no longer constructs a set via
  `schema.NewSet(schema.HashResource(...), ...)`; instead it uses
  `types.SetValueFrom(ctx, versioningLegacyObjectType(), [...])`.
- **`validation.StringInSlice([]string{"versions","history"}, true)`**
  (case-insensitive) → `stringvalidator.OneOfCaseInsensitive(...)`.

## Import

The SDKv2 importer was passthrough
(`schema.ImportStatePassthroughContext`). The framework equivalent is
`ImportState` writing the import string to both `id` and `name` (because
the resource's ID *is* its name — `Create` does `d.SetId(cn)`). `Read`
then populates the rest from the API.

## CRUD

- `d.Get("foo").(T)` → typed access on a model struct; reads via
  `req.Plan.Get(ctx, &plan)` / `req.State.Get(ctx, &state)`; writes via
  `resp.State.Set(ctx, &m)`.
- `d.SetId(cn)` → `m.ID = types.StringValue(cn); resp.State.Set(...)`.
- `d.SetId("")` (drift / 404) → `state.RemoveResource(ctx)`.
- `d.HasChange("x")` → `!plan.X.Equal(state.X)`.
- `diag.Errorf(...)` / `diag.FromErr(...)` →
  `resp.Diagnostics.AddError("op", err.Error())`, followed by
  `if resp.Diagnostics.HasError() { return }`.
- The `Create → Read` and `Update → Read` patterns become a shared
  `refreshAndSet` helper that takes the model + a `*tfsdk.State` so it can
  be reused from `Create`, `Read`, and `Update`.
- Force-destroy recursion inside `Delete` is preserved verbatim
  (mirroring the SDKv2 logic), now factored into `deleteContainer`.

## Test changes

The acceptance tests in
`resource_openstack_objectstorage_container_v1_test.go` swap
`ProviderFactories: testAccProviders` for
`ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. The HCL
configs (which use `versioning_legacy { ... }` block syntax and the
top-level `versioning = true` bool) are unchanged because the migrated
schema preserves block syntax for `versioning_legacy`.

The destroy-check helper continues to work as-is because it goes via the
provider Config / gophercloud client, which is independent of
SDKv2-vs-framework.

## Things deliberately not done

- The unit test for the V0 upgrader
  (`TestAccObjectStorageV1ContainerStateUpgradeV0` in
  `migrate_resource_openstack_objectstorage_container_v1_test.go`)
  exercised the SDKv2 upgrader's raw-map signature directly. The framework
  upgrader is a method that operates over typed
  `UpgradeStateRequest`/`UpgradeStateResponse`; constructing a fully-typed
  state with non-nil tftypes raw value out-of-band for a unit test is more
  ceremony than value here. The recommended replacement is an acceptance
  test that pins V0 state via `ExternalProviders` (per
  `references/state-upgrade.md` § Testing state upgrades). That requires a
  published SDKv2 release of this provider to use as the V0 source and so
  is left for a follow-up — not blocking the migration itself.
- The provider-level wiring (registering this framework resource via
  `provider.ResourceServer.Resources`) is provider-wide work and out of
  scope for this single-resource migration. The constructor
  `NewObjectStorageContainerV1Resource()` is exported and ready to drop
  into the provider's `Resources` slice.
