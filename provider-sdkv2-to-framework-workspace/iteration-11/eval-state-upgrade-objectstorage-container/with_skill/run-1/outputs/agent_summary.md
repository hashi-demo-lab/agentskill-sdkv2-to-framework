# Agent summary — openstack_objectstorage_container_v1 migration

## What was migrated

- `resource_openstack_objectstorage_container_v1.go` — full SDKv2 resource migrated to terraform-plugin-framework.
- `migrate_resource_openstack_objectstorage_container_v1.go` — the SDKv2 state upgrader is subsumed into the main resource file as `UpgradeState()` + `priorSchemaV0ObjectStorageContainer()` + `upgradeObjectStorageContainerFromV0()`. The sibling file is no longer needed.
- `resource_openstack_objectstorage_container_v1_test.go` — unit tests for the upgrader + acceptance tests updated to use framework provider factories and a new `TestAccObjectStorageV1Container_stateUpgrade` test.

## State upgrader — single-step semantics applied

The SDKv2 resource had `SchemaVersion: 1` with one upgrader (`Version: 0`). The SDKv2 upgrader moved `"versioning"` (TypeSet block) → `"versioning_legacy"` and set `"versioning"` to `false`.

In the framework:
- `schema.Schema.Version` is set to `1` (current version).
- `UpgradeState()` returns a single-entry map keyed at `0` (the prior version).
- The entry carries a `PriorSchema` that describes exactly what SDKv2 V0 stored on disk — the framework needs this to deserialise prior state before calling the upgrader.
- The upgrader function `upgradeObjectStorageContainerFromV0` reads a `objectStorageContainerModelV0` struct (matching the prior schema) and writes a `objectStorageContainerModel` struct (matching the current schema) in one call. It does not produce intermediate state.
- `"storage_class"`, added in V1 and absent in V0, defaults to `""` in the upgrader.

**Single-step semantics**: the entry keyed at `0` produces the *current* (V1) state directly. No chaining, no call from one upgrader into another. If a future V2 were added, a new entry keyed at `1` would be added — and the existing entry keyed at `0` would need to be updated to produce V2 state directly (composing both transformations inline).

## Key decisions

| Decision | Choice | Reason |
|---|---|---|
| `versioning_legacy` block vs attribute | `SetNestedAttribute` | Preserves existing `versioning_legacy { ... }` HCL syntax; resource is deprecated so no reason to break practitioners |
| `ForceNew` fields | `RequiresReplace` plan modifier | Correct framework equivalent; `ForceNew` does not map to `Required: true` |
| `Default: false` | `booldefault.StaticBool(false)` | Framework `Default` field, not `PlanModifiers` — incompatible types would fail at compile time |
| Delete handler input | `req.State` | `req.Plan` is null on Delete; reading it would panic |
| `ConflictsWith` (versioning ↔ versioning_legacy) | Not re-encoded | SDKv2 `ConflictsWith` has no direct framework equivalent; the mutual exclusion is enforced by OpenStack itself. Document in schema descriptions if needed. |

## Files produced

- `outputs/migrated/resource_openstack_objectstorage_container_v1.go` — no SDKv2 import; full CRUD; `UpgradeState` method; `PriorSchema` on the V0 upgrader; `schema.Schema.Version: 1`; valid Go.
- `outputs/migrated/resource_openstack_objectstorage_container_v1_test.go` — unit tests for the upgrader (no network); acceptance tests with `testAccProviders`; `TestAccObjectStorageV1Container_stateUpgrade` using `ExternalProviders` pattern from `references/state-upgrade.md`.
- `outputs/notes.md` — single-step / target-version / current-version semantics explained.
- `outputs/agent_summary.md` — this file.
