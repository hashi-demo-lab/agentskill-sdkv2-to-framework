# Migration notes — openstack_objectstorage_container_v1

## State upgrade semantics

### SDKv2 shape (before)

The SDKv2 resource had `SchemaVersion: 1` and one `StateUpgraders` entry for version 0:

- **V0 schema**: `versioning` was a `TypeSet` block with nested `type` and `location` attributes.
- **V0 → V1 upgrader**: renamed `versioning` (the set block) to `versioning_legacy`, and set `versioning` to `false` (a new boolean field introduced in V1 that enables Swift's newer versioning API).

### Framework shape (after)

The framework uses `UpgradeState()` returning `map[int64]resource.StateUpgrader`.

**Single-step semantics**: each map entry is keyed at a *prior* version and must produce the *current* (target) schema's state in one call. There is no chaining — the framework calls each upgrader independently with the matching `PriorSchema`. This resource has only one prior version (V0), so the map has one entry:

```
map[int64]resource.StateUpgrader{
    0: { PriorSchema: priorSchemaObjectStorageContainerV0(), StateUpgrader: ... },
}
```

**Current version / target version**: `schema.Schema.Version` is set to `1` (matching the SDKv2 `SchemaVersion: 1`). The V0 upgrader's function body produces the full current (V1) state — not an intermediate state. This is mandatory: the framework writes whatever `resp.State.Set` receives; returning an intermediate-version value would leave state permanently behind the current schema.

**PriorSchema requirement**: `PriorSchema` is mandatory. It tells the framework how to deserialise the prior state bytes before passing them to the upgrader function. Without it, loading any V0 state at runtime yields a hard error. The prior schema for V0 mirrors the SDKv2 V0 schema exactly (same attribute names, types, optional/computed/required flags).

**tfsdk tag correctness**: the `objectStorageContainerV0Model` struct uses `tfsdk:` tags that match the V0 attribute names exactly (`versioning` for the set, no `versioning_legacy`). A tag mismatch silently drops the field — the framework deserialises through `PriorSchema`, not the current schema, so the compiler cannot catch this.

## versioning_legacy block decision

`versioning_legacy` was a `TypeSet` with `MaxItems: 1` in SDKv2. Practitioners write it as a block (`versioning_legacy { type = "..." location = "..." }`). Switching to `SingleNestedAttribute` would be a breaking HCL change. It is kept as `SetNestedAttribute` to preserve the block syntax. The element type uses `versioningLegacyAttrTypes` for `types.Set` construction in Read/UpgradeState so the framework can compare elements correctly.

## Test file changes

- `ProviderFactories: testAccProviders` replaced with `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` in all test cases (per the SKILL.md common pitfall about flipping `ProviderFactories`).
- No `ProviderFactories:` field remains in any test case.
- A `TestAccObjectStorageV1ContainerStateUpgradeV0` acceptance test is added: Step 1 uses `ExternalProviders` to write V0 state with the last SDKv2 provider release; Step 2 inherits the TestCase-level `ProtoV6ProviderFactories` and asserts no plan diff after the upgrade.
- The migrate test (`TestAccObjectStorageV1ContainerStateUpgradeV0` unit variant) from the SDKv2 file has been superseded by the acceptance test above, which tests the real framework upgrade path.
