# Agent summary

## What was migrated

- `resource_openstack_objectstorage_container_v1.go` — full CRUD resource migrated from SDKv2 to terraform-plugin-framework. No SDKv2 import.
- `migrate_resource_openstack_objectstorage_container_v1.go` — the SDKv2 state upgrader logic is subsumed into the `UpgradeState()` method on the new resource type; the sibling file is not reproduced as a separate output file (its content lives in the main resource file).
- `resource_openstack_objectstorage_container_v1_test.go` — test file updated: `ProviderFactories` → `ProtoV6ProviderFactories`, state-upgrade acceptance test added.

## Key decisions

1. **UpgradeState single-step**: SchemaVersion was 1 in SDKv2 with one V0 upgrader. The framework `UpgradeState()` map has one entry keyed at `0` that directly produces V1 (current) state. `priorSchemaObjectStorageContainerV0()` mirrors the SDKv2 V0 schema so the framework can deserialise old state bytes.

2. **versioning_legacy as SetNestedAttribute**: kept as a set-of-nested-attributes (block syntax) rather than converting to `SingleNestedAttribute` — switching would be a breaking HCL change for practitioners who write `versioning_legacy { ... }`.

3. **GetRegionFromFramework helper**: a local helper is added since the existing `GetRegion(d *schema.ResourceData, config)` accepts an SDKv2 `ResourceData` parameter. The helper reads `types.String` directly and falls back to `config.Region`.

4. **Delete reads from State**: the Delete handler reads only from `req.State` (not `req.Plan`, which is null on delete) per the framework requirement.

5. **ProtoV6ProviderFactories flip**: all test cases use `ProtoV6ProviderFactories` — no `ProviderFactories:` field remains. `testAccProtoV6ProviderFactories` is declared with a stub that references `NewFrameworkProvider` (the provider's framework entry point, to be wired when the full provider migration is complete).

## Files produced

- `outputs/migrated/resource_openstack_objectstorage_container_v1.go`
- `outputs/migrated/resource_openstack_objectstorage_container_v1_test.go`
- `outputs/notes.md`
- `outputs/agent_summary.md`
