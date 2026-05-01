# Migration notes — openstack_objectstorage_container_v1

## Chained-vs-single-step state upgrade

The SDKv2 resource had `SchemaVersion: 1` with **one** upgrader entry:

```go
StateUpgraders: []schema.StateUpgrader{
    {Version: 0, Upgrade: resourceObjectStorageContainerStateUpgradeV0},
}
```

This is the simplest possible case: a single V0 → V1 chain.  The framework's
`UpgradeState` uses identical single-step semantics here.  The upgrader keyed
at `0` produces the *current* (V1) state directly:

```go
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {PriorSchema: containerSchemaV0(), StateUpgrader: upgradeContainerStateV0toV1},
    }
}
```

The transformation is the same as the original SDKv2 upgrader:
- `versioning` (V0 TypeSet block) → `versioning_legacy` (renamed block)
- `versioning` (new bool field) → `false`
- `storage_class` (absent in V0) → `null` (populated on next Read)

If the resource had been SchemaVersion 2 with two SDKv2 upgraders (V0→V1,
V1→V2), the framework would require **two** entries in the map: one keyed at
`0` that composes both SDKv2 transformations into one V0→current jump, and one
keyed at `1` that ports just the V1→V2 transformation.  The "chained" SDKv2
path does NOT translate into a chained framework path — each framework upgrader
must produce the current schema.

## Key schema decisions

### versioning_legacy block

The original SDKv2 schema used `TypeSet` with `MaxItems: 1`.  Per the
`blocks.md` reference, backward-compat HCL syntax takes priority: practitioners
wrote `versioning_legacy { ... }` (block syntax).  Converting to
`SingleNestedAttribute` would change user HCL and is a breaking change.  The
migration keeps `SetNestedBlock` and enforces at-most-one via a
`ConfigValidators` implementation instead of a schema-level `MaxItems` field
(which blocks don't have in the framework).

### ConflictsWith

SDKv2's `ConflictsWith: []string{"versioning_legacy"}` on `versioning` and
vice-versa becomes a `ResourceWithConfigValidators` implementation
(`versioningConflictConfigValidator`) that checks both constraints
(`len(VersioningLegacy) <= 1` and `versioning == true && len(VersioningLegacy)
> 0`) in one validator to avoid split error messages.

### storage_class (new in V1)

The field was added alongside the V0→V1 schema version bump.  In the V0 prior
schema it is absent.  The upgrader sets it to `types.StringNull()` — the next
`Read` cycle fetches the real value from the API via the `X-Storage-Class`
response header.  This matches the SDKv2 upgrader's implicit behaviour (the raw
map simply had no `storage_class` key).

### Default values

`versioning` (bool, default false) and `force_destroy` (bool, default false)
use `booldefault.StaticBool(false)` from the `resource/schema/booldefault`
package — NOT a plan modifier.  Both attributes are `Optional + Computed +
Default` so the framework inserts the default into the plan when the user omits
the attribute.

### ForceNew attributes

`region`, `storage_policy`, `storage_class` all had `ForceNew: true` in SDKv2.
These become `stringplanmodifier.RequiresReplace()` in the `PlanModifiers`
slice.

## Compile / test results

Compile check was performed in `/tmp/openstack-compile-check/` (a copy of the
provider tree).  The framework is not yet a dependency of the provider, so
the following were added to `go.mod`:

```
github.com/hashicorp/terraform-plugin-framework         v1.17.0
github.com/hashicorp/terraform-plugin-framework-validators v0.19.0
```

`terraform-plugin-go` was pinned at v0.29.0 (its existing version) to avoid
breaking SDKv2's `grpc_provider.go` which has an interface compatibility
requirement on that specific version.

Results:

| Check | Result |
|---|---|
| `go build ./openstack/...` | PASS |
| `go vet ./openstack/...` | PASS |
| `TestObjectStorageV1ContainerStateUpgradeV0` (unit test, no TF_ACC) | PASS |
| Acceptance tests (`TestAccObjectStorageV1Container_*`) | Not run — require live OpenStack + `TF_ACC=1` |

### TDD state

The acceptance tests reference `protoV6ProviderFactories` which does not yet
exist in the provider (the provider entry point has not been migrated).  This
is the expected "red" state from workflow step 7: tests are updated first and
will fail to compile (or fail at runtime) until the provider's `main.go` is
migrated to serve via `providerserver.NewProtocol6WithError`.  A test-only stub
was added during compile verification and then removed.

## Files produced

| File | Purpose |
|---|---|
| `migrated/resource_openstack_objectstorage_container_v1.go` | Full framework resource (Metadata, Schema, CRUD, ConfigValidators, ImportState, UpgradeState interfaces declared) |
| `migrated/upgrade_objectstorage_container_v1.go` | `UpgradeState` method, `containerSchemaV0()`, `containerV0Model`, `upgradeContainerStateV0toV1` |
| `migrated/resource_openstack_objectstorage_container_v1_test.go` | Acceptance tests (ProtoV6) + typed framework unit test for the state upgrader |
