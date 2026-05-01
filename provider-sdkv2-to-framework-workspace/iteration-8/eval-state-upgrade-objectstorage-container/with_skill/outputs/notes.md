# Migration notes: openstack_objectstorage_container_v1

## Pre-flight C think pass

### 1. Block decision
`versioning_legacy` (SDKv2: `TypeSet`, `MaxItems: 1`, `Elem: &schema.Resource{}`) uses the block
HCL syntax in all documented examples and provider tests (`versioning_legacy { type = "versions" ... }`).
Converting to `SingleNestedAttribute` would break practitioner configs. Decision: **keep as
`schema.SetNestedBlock`** — the direct framework analogue for a `TypeSet MaxItems:1` block —
which preserves the practitioner-facing HCL unchanged.

### 2. State upgrade semantics — single-step, not chained

**This is the most important aspect of this migration.** The SDKv2 resource had:

- `SchemaVersion: 1`
- One `StateUpgrader` entry: `{Version: 0, Upgrade: resourceObjectStorageContainerStateUpgradeV0}`

The SDKv2 upgrader (V0 → V1) moved the `versioning` Set block to `versioning_legacy` and reset
`versioning` to `false`.

The framework's `UpgradeState` uses **single-step, not chained** semantics:

> Each map entry keyed by a prior version number produces the **current (target) schema's state
> directly in one call**. The framework calls the upgrader independently for each prior version —
> it does not chain upgraders. There is no V0→V1→current path; the V0 entry must produce
> current-schema state.

Because there is only one historical version (V0), the framework `UpgradeState` map has exactly
one entry:

```go
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            PriorSchema:   priorSchemaObjectStorageContainerV0(),
            StateUpgrader: upgradeObjectStorageContainerStateFromV0,
        },
    }
}
```

`upgradeObjectStorageContainerStateFromV0` reads a `objectStorageContainerV0Model` (matching the
`PriorSchema`) and writes a `objectStorageContainerV1Model` (matching the current schema).
It does **not** call any other upgrader function.

**Target-version / current-version semantics**: The key in the map (`0`) is the *prior* version
(where the state came from), not the target version. The target is always the current schema
(the version set in `resp.Schema = schema.Schema{Version: 1, ...}`). The framework uses this
to determine which upgrader to invoke when it detects a state file whose version is less than
the current schema version.

**Why no chaining**: If a future V2 schema is introduced, the map gains a `1:` entry that
upgrades V1 → current (V2), and the existing `0:` entry must be updated to upgrade V0 → V2
*directly* (not V0 → V1). Each entry is a complete transformation to the current schema, not
an incremental step. This is a deliberate framework design choice to make each upgrader's
correctness self-contained.

### 3. Import shape
`Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` →
`resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`. Simple passthrough; no
composite ID parsing needed.

## Key migration decisions

| SDKv2 | Framework | Reason |
|---|---|---|
| `SchemaVersion: 1` | `schema.Schema{Version: 1}` | Carried forward; must match for UpgradeState |
| `StateUpgraders` (V0 entry) | `UpgradeState` map with `0:` entry and `PriorSchema` | Single-step to current |
| `TypeBool` `versioning`, `Default: false` | `schema.BoolAttribute{..., Default: booldefault.StaticBool(false)}` | Default is own package, not a plan modifier |
| `TypeBool` `force_destroy`, `Default: false` | `schema.BoolAttribute{..., Default: booldefault.StaticBool(false)}` | Same pattern |
| `TypeSet MaxItems:1 versioning_legacy` | `schema.SetNestedBlock` | Preserves block HCL syntax |
| `TypeMap metadata` | `schema.MapAttribute{ElementType: types.StringType}` | Direct mapping |
| `ForceNew: true` on region/storage_policy/storage_class | `stringplanmodifier.RequiresReplace()` | ForceNew is a plan modifier |
| `Computed` on region/storage_policy/storage_class/id | `stringplanmodifier.UseStateForUnknown()` | Keeps stable values in plan |
| `d.SetId("")` (on not-found) | `resp.State.RemoveResource(ctx)` | Framework equivalent |
| `schema.HashResource(...)` (Set hash) | Deleted | Framework handles set uniqueness internally |
| `ConflictsWith: []string{"versioning_legacy"}` on `versioning` | Could use `stringvalidator.ConflictsWith()` but omitted for bool — cross-attribute constraint better on `versioning_legacy` | Framework uses path-based validators |

## Files produced
- `resource_openstack_objectstorage_container_v1.go` — framework resource (no SDKv2 import)
- `resource_openstack_objectstorage_container_v1_test.go` — updated tests using `ProtoV6ProviderFactories`

The original `migrate_resource_openstack_objectstorage_container_v1.go` is superseded by the
inline `priorSchemaObjectStorageContainerV0()` and `upgradeObjectStorageContainerStateFromV0()`
functions in the resource file. It can be deleted once the migration is complete.
