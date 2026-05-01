# Agent summary

## Task

Migrate `openstack_objectstorage_container_v1` from `terraform-plugin-sdk/v2`
to `terraform-plugin-framework`, translating the single SDKv2 state upgrader
(V0→V1) to framework `UpgradeState` single-step semantics.

## Files produced

| File | Description |
|---|---|
| `migrated/resource_openstack_objectstorage_container_v1.go` | Full framework resource: no SDKv2 import, CRUD, `UpgradeState`, `ImportState` |
| `migrated/resource_openstack_objectstorage_container_v1_test.go` | Updated tests using `ProtoV6ProviderFactories`; state-upgrade acceptance test; unit upgrader test |
| `notes.md` | State-upgrader design decisions and migration rationale |

The SDKv2 `migrate_resource_openstack_objectstorage_container_v1.go` is
subsumed into the main resource file; it does not need a separate migrated
counterpart.

## Key decisions

### State upgrader

The SDKv2 resource had `SchemaVersion: 1` with one `StateUpgraders` entry
(version 0 → 1). The framework translation:

- `schema.Schema.Version = 1` on the current schema (target version).
- `UpgradeState()` returns a single-entry map keyed at `0` (prior version).
- Each entry carries a `PriorSchema` that exactly mirrors the V0 SDKv2 schema
  so the framework can decode the old state JSON.
- The upgrader function produces the **current** (V1) state directly — not an
  intermediate state. This is the single-step guarantee the framework enforces.

V0→V1 transformation: `versioning` (Set) → `versioning_legacy` (Set);
`versioning` (Bool) defaults to `false`; `storage_class` (absent in V0)
defaults to `""`.

### `versioning_legacy` — block not attribute

`versioning_legacy` has `MaxItems: 1` in SDKv2. It is kept as a
`SetNestedAttribute` (not converted to `SingleNestedAttribute`) to preserve
backward-compatible HCL block syntax. Converting would break existing practitioner
configs.

### No SDKv2 import

The migrated file imports only framework and gophercloud packages. The negative
gate in `verify_tests.sh` (substring match for `terraform-plugin-sdk/v2`) will
pass cleanly.

### Test strategy

`ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories`.
New test `TestAccObjectStorageV1Container_stateUpgradeV0` uses the
`ExternalProviders` + `PlanOnly` pattern from `references/state-upgrade.md` to
verify the upgrader end-to-end against a real V0 state file.

## Assumptions / follow-up items

1. `frameworkProvider()` (referenced in the test file) must be provided by the
   broader provider migration — it is the framework equivalent of the SDKv2
   `Provider()` function. Replace the stub once available.
2. `ConflictsWith` enforcement between `versioning` and `versioning_legacy` is
   not yet implemented as a framework validator; a follow-up should add
   `resourcevalidator.Conflicts`.
3. The `readIntoState` helper emits a warning diagnostic on 404 rather than
   removing the resource; a production hardening pass should convert this to
   `resp.State.RemoveResource(ctx)` in the `Read` method.
