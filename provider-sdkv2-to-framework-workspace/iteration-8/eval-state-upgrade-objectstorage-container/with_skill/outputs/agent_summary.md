# Agent summary: openstack_objectstorage_container_v1 migration

## What was migrated

Migrated `openstack_objectstorage_container_v1` from `terraform-plugin-sdk/v2` to
`terraform-plugin-framework`, including its state upgrade path.

### Source files analysed
- `openstack/resource_openstack_objectstorage_container_v1.go` — SDKv2 resource (SchemaVersion: 1)
- `openstack/migrate_resource_openstack_objectstorage_container_v1.go` — V0 upgrader + V0 schema
- `openstack/resource_openstack_objectstorage_container_v1_test.go` — SDKv2 acceptance tests
- `openstack/migrate_resource_openstack_objectstorage_container_v1_test.go` — SDKv2 upgrader unit test

### Output files produced
- `migrated/resource_openstack_objectstorage_container_v1.go` — framework resource
- `migrated/resource_openstack_objectstorage_container_v1_test.go` — updated tests
- `notes.md` — per-resource think pass and decisions
- `agent_summary.md` — this file

## Key decisions

### State upgrade — single-step semantics

The SDKv2 resource had `SchemaVersion: 1` and a single `StateUpgraders` entry for V0.
The V0 upgrader renamed `versioning` (a Set block) to `versioning_legacy` and set `versioning`
to `false` (bool).

The framework `UpgradeState` implementation:
- Returns a `map[int64]resource.StateUpgrader` with one entry keyed at `0`
- Each entry specifies a `PriorSchema` (the V0 schema shape) for deserialisation
- Each entry's `StateUpgrader` function produces **current-schema state directly** (V0 → V1 in
  one step) — not an intermediate version
- There is no chain; V0 → current is a single transformation
- `schema.Schema{Version: 1}` is set on the resource schema so the framework knows to invoke
  the upgrader when it encounters an older state file

### Block vs attribute

`versioning_legacy` is kept as `schema.SetNestedBlock` (not `SingleNestedAttribute`) because
practitioners use block HCL syntax (`versioning_legacy { ... }`) and converting would break
existing configs.

### No SDKv2 imports

The migrated file imports only:
- `terraform-plugin-framework` packages
- `terraform-plugin-framework-validators`
- gophercloud API libraries

## Interface assertions

```go
var (
    _ resource.Resource                 = &objectStorageContainerV1Resource{}
    _ resource.ResourceWithConfigure    = &objectStorageContainerV1Resource{}
    _ resource.ResourceWithImportState  = &objectStorageContainerV1Resource{}
    _ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}
)
```

## Test changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: protoV6ProviderFactories`
- Added `TestAccObjectStorageV1Container_stateUpgradeV0` acceptance test using `ExternalProviders`
  to write V0 state and `PlanOnly: true` to verify no drift after upgrade
- Added `TestObjectStorageV1ContainerUpgradeStateV0` unit test (no TF_ACC) that exercises the
  model transformation directly

## Workflow steps completed

- Pre-flight 0: not a muxed migration — single-release path applies
- Pre-flight C: block decision (keep SetNestedBlock), state upgrade (single-step V0→current),
  import (passthrough)
- Step 6: resource migrated (no SDKv2 import)
- Step 7: tests updated to ProtoV6ProviderFactories; state-upgrade acceptance test added
