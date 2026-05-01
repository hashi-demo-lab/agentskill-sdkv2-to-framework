# Agent Summary

## Task
Migrate `openstack_objectstorage_container_v1` from terraform-plugin-sdk/v2 to terraform-plugin-framework, including V0 state upgrader translation.

## Files produced
- `migrated/resource_openstack_objectstorage_container_v1.go` — full framework resource (no SDKv2 import)
- `migrated/resource_openstack_objectstorage_container_v1_test.go` — unit + acceptance tests
- `notes.md` — state-upgrade semantics documentation
- `agent_summary.md` — this file

## Key decisions

### State upgrade (single-step)
The SDKv2 resource had `SchemaVersion: 1` with one `StateUpgrader` for V0. The framework `UpgradeState()` method returns `map[int64]resource.StateUpgrader{0: {...}}`. The V0 upgrader directly produces current (V1) state in one call — no chaining. `PriorSchema` is provided so the framework can deserialize the old state.

The V0 transformation: old `versioning` set-block → `versioning_legacy`; new `versioning` bool → `false`. The `storage_class` field (added in the V1 schema) defaults to `""` for upgraded resources.

### versioning_legacy as SetNestedAttribute
`versioning_legacy` (the deprecated block) is kept as a `SetNestedAttribute` because practitioners already use block syntax (`versioning_legacy { ... }`) in production configs. Converting to `SingleNestedAttribute` would be a breaking HCL change.

### CRUD implementation
- `Create/Read/Update/Delete` methods use a typed `containerModel` struct with `tfsdk:` tags
- A shared `readIntoModel` helper handles 404 (removes from state) and populates all computed fields
- Delete handles `force_destroy` (409 Conflict) by listing and deleting all object versions then retrying
- Import uses passthrough semantics (reads state by ID)

### No SDKv2 imports
The migrated file imports only framework packages (`terraform-plugin-framework`, `terraform-plugin-framework-validators`) plus gophercloud — zero references to `terraform-plugin-sdk/v2`.

## What was NOT changed
The sibling `objectstorage_container_v1.go` (which defines `containerCreateOpts`) is shared and remains unchanged. The migrated resource still uses it.

## Test coverage
- Two unit tests exercise the V0 upgrader via the framework's `tfsdk.State` deserialization path (one with a versioning item, one with empty versioning)
- Acceptance tests use `ProtoV6ProviderFactories` (replacing SDKv2 `ProviderFactories`)
- A `TestAccObjectStorageV1Container_stateUpgradeV0` acceptance test exercises the full upgrade path using `ExternalProviders` to write V0 state, then asserting no plan diff with the migrated provider
