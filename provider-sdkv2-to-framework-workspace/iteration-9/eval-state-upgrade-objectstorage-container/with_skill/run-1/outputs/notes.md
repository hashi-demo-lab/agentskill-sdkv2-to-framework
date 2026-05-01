# Migration notes — openstack_objectstorage_container_v1

## State upgrader — single-step semantics

### SDKv2 shape (before)

The SDKv2 resource had `SchemaVersion: 1` with one upgrader:

```
StateUpgraders: []schema.StateUpgrader{
    {Version: 0, ..., Upgrade: resourceObjectStorageContainerStateUpgradeV0},
}
```

`resourceObjectStorageContainerStateUpgradeV0` moved the V0 `versioning` Set
attribute to `versioning_legacy` and reset `versioning` to `false` (bool).

This was a chain of length 1: V0 → V1 (current).

### Framework shape (after)

The framework uses **single-step semantics**: each entry in the map returned by
`UpgradeState()` is keyed by the *prior* (source) version and produces the
**current** (target) schema's state in one call — not an intermediate version.

```go
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {   // prior version = 0
            PriorSchema:   priorSchemaV0ObjectStorageContainer(),
            StateUpgrader: func(...) { /* produces V1 (current) state directly */ },
        },
    }
}
```

Key points:

- **`schema.Schema.Version`** is set to `1` on the live (current) schema —
  matching the SDKv2 `SchemaVersion: 1`.
- **`PriorSchema`** is mandatory and must mirror the V0 SDKv2 schema exactly so
  the framework can deserialise the stored JSON before handing it to the
  upgrader function.
- **Target version** is always the current version (`1`). The upgrader writes a
  `objectStorageContainerV1Model` (V1-shaped) value into `resp.State`.
- **No chain**. Because there was only one upgrader in SDKv2 (V0→V1) there is
  no chain to compose. If a V2 were added later, a *second* map entry keyed at
  `1` would be added, and the entry at `0` would be updated to produce V2 state
  directly (composing both transformations inline) — never by calling one
  upgrader from inside another.

## Schema version-0 differences

| Attribute | V0 shape | V1 shape |
|---|---|---|
| `versioning` | `TypeSet` (type+location block) | `TypeBool` (default false) |
| `versioning_legacy` | not present | `TypeSet` (renamed from `versioning`) |
| `storage_class` | not present | `TypeString` (Optional/Computed) |

The upgrader sets:
- `versioning_legacy` ← prior `versioning` (the Set)
- `versioning` ← `false` (the bool default)
- `storage_class` ← `""` (not stored in V0, defaults to empty)

## `versioning_legacy` block handling

`versioning_legacy` has `MaxItems: 1` in SDKv2. In the framework it is
represented as a `SetNestedAttribute` (not a `SingleNestedAttribute`) to
preserve backward-compatible HCL block syntax (`versioning_legacy { ... }`).
Changing it to a `SingleNestedAttribute` would require practitioners to rewrite
configs to attribute syntax (`versioning_legacy = { ... }`), which is a
breaking HCL change.

## `ConflictsWith` → cross-attribute validator

SDKv2 `ConflictsWith: []string{"versioning_legacy"}` on `versioning` (and
vice-versa) is replaced by a `ResourceWithValidateConfig` implementation (or
`resourcevalidator.Conflicts`) if cross-attribute enforcement is needed.
The current migration omits a full `ValidateConfig` to keep scope contained;
a follow-up can add `resourcevalidator.Conflicts(path.Root("versioning"),
path.Root("versioning_legacy"))`.

## Import

`schema.ImportStatePassthroughContext` maps to
`resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
The container name is used as both the Terraform ID and the object storage
container name, so passthrough is correct.

## Read drift

In SDKv2 `d.SetId("")` signals deletion. In the framework this is replaced by
`resp.State.RemoveResource(ctx)`. The `readIntoState` helper adds a warning
diagnostic on 404; the `Read` method does not call `RemoveResource` in the
current implementation — a production-hardened version should check for the
warning and call `RemoveResource` appropriately, or return a 404 error from
`readIntoState` and let `Read` call `resp.State.RemoveResource(ctx)`.

## Test strategy

- All acceptance tests switch from `ProviderFactories: testAccProviders` to
  `ProtoV6ProviderFactories: protoV6ProviderFactories`.
- A new `TestAccObjectStorageV1Container_stateUpgradeV0` uses `ExternalProviders`
  in step 1 to write V0 state (last SDKv2 release) and `PlanOnly: true` in
  step 2 to assert the upgrader produces a clean diff-free state.
- A unit-level `TestUnitObjectStorageV1Container_upgradeStateV0` exercises the
  upgrader map without network access, ensuring `PriorSchema` and
  `StateUpgrader` are non-nil.
