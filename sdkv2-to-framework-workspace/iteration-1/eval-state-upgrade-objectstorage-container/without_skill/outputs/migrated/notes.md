# Migration Notes: openstack_objectstorage_container_v1

## Overview

Migrated `resource_openstack_objectstorage_container_v1.go` and
`migrate_resource_openstack_objectstorage_container_v1.go` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

---

## Key Decisions

### 1. State-upgrade semantics (SDK v2 → framework)

| SDK v2 | Framework |
|--------|-----------|
| `SchemaVersion: 1` + `StateUpgraders []schema.StateUpgrader{{Version: 0, ...}}` | `schema.Schema{Version: 1}` + `ResourceWithUpgradeState` returning `map[int64]resource.StateUpgrader{0: {...}}` |

The SDK v2 upgrade function (`resourceObjectStorageContainerStateUpgradeV0`) operated
on `map[string]any` raw state. In the framework the equivalent is implemented in
`UpgradeState` with a `PriorSchema` (describing the V0 shape) so the framework can
deserialise the raw JSON into typed structs. The logic is identical:

- move `versioning` (old block list) → `versioning_legacy`
- set `versioning` (new bool) to `false`

The `storage_class` attribute did not exist in the V0 schema; when upgrading from V0
state it is set to `types.StringUnknown()` so a subsequent plan/apply refresh
populates it.

### 2. `versioning_legacy` block — MaxItems:1 as `ListNestedBlock`

The SDK v2 `TypeSet` with `MaxItems:1` is represented as a `schema.ListNestedBlock`
with a `listvalidator.SizeAtMost(1)` validator. This preserves the existing block
HCL syntax (no `= {...}` assignment, no index brackets), which is a
non-breaking change for practitioners.

### 3. `versioning` bool — ConflictsWith

The SDK v2 `ConflictsWith: []string{"versioning_legacy"}` constraint between
`versioning` (bool) and `versioning_legacy` (block) is enforced at apply time
inside `Create`/`Update` rather than via a plan-time `ConflictsWith` validator,
because `versioning_legacy` is a block (not an attribute) and the framework's
`ConflictsWith` validator only accepts attribute paths.

### 4. State model (`containerV1Model`)

A single Go struct with `tfsdk` field tags replaces all `d.Get()`/`d.Set()` calls.
`types.List` is used for `versioning_legacy` so that `ElementsAs` can decode it into
`[]versioningLegacyModel`.

### 5. `force_destroy`, `content_type`, `container_sync_to`, `container_sync_key`

These fields are write-only or not returned by the API GET response. During `Read`
the current state values are preserved by re-reading the state before overwriting it.
This mirrors the original SDK behaviour of leaving those fields unchanged on read.

### 6. `ImportState`

`schema.ImportStatePassthroughContext` is replaced by a custom `ImportState` method
that sets both `id` and `name` from the import ID (the container name).

### 7. `checkDeletedDiag` helper

Replaces the SDK `CheckDeleted` helper. Returns `true` if the error is a 404, which
allows callers to call `state.RemoveResource(ctx)` and continue silently.

---

## Files Changed

- **removed**: `migrate_resource_openstack_objectstorage_container_v1.go` — its logic
  is inlined into the `UpgradeState` handler in the main resource file.
- **migrated**: `resource_openstack_objectstorage_container_v1.go`
- **updated**: `resource_openstack_objectstorage_container_v1_test.go`
  - SDK `TestAccObjectStorageV1ContainerStateUpgradeV0` (raw-map unit test) replaced by
    `TestObjectStorageContainerStateUpgradeV0` which drives the framework
    `UpgradeState` handler via `tfsdk.State` directly.
  - All acceptance tests preserved with identical HCL fixtures.
  - `TestAccObjectStorageV1Container_importBasic` merged from the separate import test
    file.

---

## New dependencies (go.mod additions required)

```
github.com/hashicorp/terraform-plugin-framework
github.com/hashicorp/terraform-plugin-framework-validators
github.com/hashicorp/terraform-plugin-go
```
