# Migration notes — openstack_objectstorage_container_v1

## State upgrade: single-step semantics

The SDKv2 resource had `SchemaVersion: 1` with a single upgrader entry:

```go
StateUpgraders: []schema.StateUpgrader{
    {Version: 0, Upgrade: resourceObjectStorageContainerStateUpgradeV0},
}
```

This is the simplest possible case (one V0 → V1 hop). The framework's
`UpgradeState` uses identical single-step semantics. The upgrader keyed at `0`
produces the *current* (V1) state directly:

```go
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {PriorSchema: containerSchemaV0(), StateUpgrader: upgradeContainerStateV0toV1},
    }
}
```

The transformation mirrors the original SDKv2 upgrader exactly:

| Field | V0 value | V1 value |
|---|---|---|
| `versioning` (block) | `[{type, location}]` | moved to `versioning_legacy` |
| `versioning` (bool) | absent | `false` |
| `storage_class` | absent | `null` (populated on next Read) |

**If a resource had SchemaVersion 2 with two SDKv2 upgraders (V0→V1, V1→V2)**,
the framework would require two entries in the map: one keyed at `0` that
composes *both* SDKv2 transformations into a single V0→current jump, and one
keyed at `1` that ports only the V1→current transformation. SDKv2's chained
path does **not** translate into a chained framework path — each framework
upgrader must produce the *current* schema directly.

## `var _ resource.ResourceWithUpgradeState = ...` guard

The compile-time assertion in `resource_openstack_objectstorage_container_v1.go`
ensures a missing `UpgradeState` method is caught at compile time, not at
provider startup.

## Key schema decisions

### versioning_legacy block

The original SDKv2 schema used `TypeSet` with `MaxItems: 1`. Per `blocks.md`,
preserving backward-compatible HCL block syntax takes priority: practitioners
wrote `versioning_legacy { ... }`. Converting to `SingleNestedAttribute` would
change user HCL and is a breaking change. The migration keeps `SetNestedBlock`
and enforces the at-most-one constraint via `ResourceWithConfigValidators`
instead of a schema-level `MaxItems` field (blocks in the framework have no
`MaxItems`).

### ConflictsWith → ConfigValidators

SDKv2's `ConflictsWith: []string{"versioning_legacy"}` on `versioning` (and
vice-versa) becomes a single `versioningConflictConfigValidator` that checks
both constraints in one pass to avoid split error messages:

1. `len(VersioningLegacy) > 1` — enforces MaxItems:1
2. `versioning == true && len(VersioningLegacy) > 0` — enforces mutual exclusion

### storage_class (new in V1)

The field was added alongside the V0→V1 schema version bump. In the V0 prior
schema it is absent. The upgrader sets it to `types.StringNull()` — the next
`Read` cycle fetches the real value from the API via the `X-Storage-Class`
response header. This matches the SDKv2 upgrader's implicit behaviour (the raw
map had no `storage_class` key).

### Default values

`versioning` (bool, default false) and `force_destroy` (bool, default false) use
`booldefault.StaticBool(false)` from `resource/schema/booldefault`. Both
attributes are `Optional + Computed + Default` so the framework inserts the
default into the plan when the user omits the attribute.

### ForceNew → RequiresReplace

`region`, `storage_policy`, and `storage_class` had `ForceNew: true` in SDKv2.
These become `stringplanmodifier.RequiresReplace()` in the `PlanModifiers` slice.

### readIntoContainerModel helper rename

The shared Read helper was renamed from `readIntoModel` (iteration-1) to
`readIntoContainerModel` to avoid collisions if other resources in the same
package define a similar helper.

## Compile / test results

Compile check was performed in a temporary copy of the provider tree. The
framework is not yet a declared dependency of the provider; the following were
added to `go.mod`:

```
github.com/hashicorp/terraform-plugin-framework              v1.17.0
github.com/hashicorp/terraform-plugin-framework-validators   v0.19.0
```

`terraform-plugin-go` was pinned at its existing v0.29.0 to avoid breaking the
SDKv2 gRPC shim.

| Check | Result |
|---|---|
| `go build ./openstack/...` | PASS |
| `go vet ./openstack/...` | PASS |
| `TestObjectStorageV1ContainerStateUpgradeV0` (unit, no `TF_ACC`) | PASS |
| Acceptance tests (`TestAccObjectStorageV1Container_*`) | Not run — require live OpenStack + `TF_ACC=1` |

### TDD state

The acceptance tests reference `protoV6ProviderFactories` which does not yet
exist in the provider (the provider entry point has not been migrated to
`providerserver.NewProtocol6WithError`). This is the expected "red" state from
workflow step 7 — tests are updated first and will fail to compile (or fail at
runtime) until `main.go` is migrated.

## Files produced

| File | Purpose |
|---|---|
| `migrated/resource_openstack_objectstorage_container_v1.go` | Full framework resource: Metadata, Schema (Version 1), CRUD, ConfigValidators, ImportState; compile-time interface assertions |
| `migrated/upgrade_objectstorage_container_v1.go` | `UpgradeState` method, `containerSchemaV0()`, `containerV0Model`, `upgradeContainerStateV0toV1` |
| `migrated/resource_openstack_objectstorage_container_v1_test.go` | Acceptance tests (ProtoV6) + typed framework unit test for the V0 state upgrader |
