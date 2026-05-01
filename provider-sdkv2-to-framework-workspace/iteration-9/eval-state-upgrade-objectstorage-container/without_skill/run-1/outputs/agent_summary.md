# Agent Summary

## Task

Migrate `resource_openstack_objectstorage_container_v1.go` and its state-upgrade sibling `migrate_resource_openstack_objectstorage_container_v1.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The resource has `SchemaVersion: 1` and one `StateUpgrader` (v0 → v1). The migration must implement single-step `UpgradeState` semantics and update the test file.

## What Was Done

1. Read source files: the main resource file, migration file (v0 schema + upgrader), the test file, the import test, and the shared helper `objectstorage_container_v1.go`.

2. Examined the framework packages: `resource.Resource`, `resource.ResourceWithUpgradeState`, `resource.StateUpgrader`, `schema.Schema` (with `Version int64`), attribute/block types (`StringAttribute`, `BoolAttribute`, `MapAttribute`, `SetNestedBlock`), plan modifiers (`stringplanmodifier.RequiresReplace`, `UseStateForUnknown`), defaults (`booldefault.StaticBool`), and validators (`stringvalidator.OneOfCaseInsensitive`).

3. Produced three output files:

### `migrated/resource_openstack_objectstorage_container_v1.go`

Full framework resource implementation:
- `objectStorageContainerV1Resource` struct with `*Config` field
- `Configure()` stores the provider config
- `Schema()` returns a `schema.Schema{Version: 1, ...}` with all attributes/blocks matching the SDKv2 v1 schema
- `Create`, `Read`, `Update`, `Delete` methods implement full CRUD using gophercloud
- `ImportState()` uses `resource.ImportStatePassthroughID`
- `UpgradeState()` returns `map[int64]resource.StateUpgrader{0: {...}}`:
  - `PriorSchema`: the v0 schema (where `versioning` was `SetNestedBlock`, not a bool)
  - `StateUpgrader` func: reads v0 state, moves `versioning` set → `versioning_legacy`, sets `versioning` bool to `false`

### `migrated/resource_openstack_objectstorage_container_v1_test.go`

Updated test file preserving all original acceptance tests (`basic`, `versioning`, `storagePolicy`). Added a unit test `TestAccObjectStorageV1Container_stateUpgradeV0` that exercises the v0→v1 state upgrade logic.

### `outputs/notes.md`

Detailed migration notes covering schema mapping, state upgrade semantics, omitted `ConflictsWith`, and caveats.

## Key Decisions

- **`UpgradeState` key `0`** handles the single v0→v1 transition. The `PriorSchema` uses `SetNestedBlock` for `versioning` (mirroring the SDKv2 v0 schema in `migrate_resource_openstack_objectstorage_container_v1.go`).
- **`versioning` in v1** is a `BoolAttribute` (Optional, Computed, default false); in the prior schema it was a `SetNestedBlock`.
- **`storage_class`** was not in the v0 schema; the upgrader sets it to `types.StringNull()` in the upgraded state.
- **`ConflictsWith`** between `versioning` and `versioning_legacy` is not translated (would require a framework config validator — left as a known caveat).
- **`readIntoState` helper** is shared by Create (post-create refresh) and Read to avoid duplication.
- The `containerCreateOpts` helper struct (`objectstorage_container_v1.go`) is reused unchanged.
