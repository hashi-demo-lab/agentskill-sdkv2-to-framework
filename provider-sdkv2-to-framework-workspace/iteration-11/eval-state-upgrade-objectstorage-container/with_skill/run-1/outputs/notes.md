# Migration notes — openstack_objectstorage_container_v1

## State upgrader semantics

### SDKv2 shape (source)

```
SchemaVersion: 1
StateUpgraders: []schema.StateUpgrader{
    {Version: 0, Type: ..., Upgrade: resourceObjectStorageContainerStateUpgradeV0},
}
```

The SDKv2 upgrader for V0→V1 did a single rename+type change:
- Renamed `"versioning"` (TypeSet of {type, location}) → `"versioning_legacy"`
- Set `"versioning"` to `false` (now a TypeBool)

### Framework shape (target)

The framework uses **single-step semantics**: each entry in the `UpgradeState()` map is keyed by a *prior* version number and must produce the **current** (target) schema state directly in one call. There is no chain.

```go
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            PriorSchema:   priorSchemaV0ObjectStorageContainer(),
            StateUpgrader: upgradeObjectStorageContainerFromV0,
        },
    }
}
```

Because there is only one prior version (V0), there is only one map entry. The entry is keyed at `0` (the *prior* version) and writes the **current V1 state** directly.

### Single-step vs chained semantics

- **SDKv2 (chained)**: V0 → V1. Each upgrader sees intermediate state.
- **Framework (single-step)**: Entry keyed at `0` receives V0 state and writes current (V1) state in one call. No intermediate state exists; no upgrader calls another.

This resource had only one upgrader (V0→V1 = V0→current), so there is no composition to do. The SDKv2 upgrader body was ported directly.

### Current version vs prior version

- `schema.Schema.Version: 1` — the **current** schema version (what the provider now serves).
- Each `UpgradeState` key is a **prior** version. Key `0` means "here is how to upgrade state that was written by schema version 0".
- The framework automatically routes state with `schema_version=0` to the entry keyed at `0`.

### PriorSchema requirement

`PriorSchema` must describe exactly what SDKv2 stored on disk at that version. The framework deserialises prior state through `PriorSchema` before calling the upgrader function. Without it, the framework returns a runtime error when encountering old state.

The V0 schema (`priorSchemaV0ObjectStorageContainer`) matches `resourceObjectStorageContainerV1V0()` field-for-field, with SDKv2 TypeSet → framework `SetNestedAttribute` for the `"versioning"` block.

## Schema changes V0 → V1 (current)

| Attribute | V0 | V1 (current) |
|---|---|---|
| `versioning` | `TypeSet` of `{type, location}` blocks | `TypeBool` (default false) |
| `versioning_legacy` | (absent) | `SetNestedAttribute` of `{type, location}` |
| `storage_class` | (absent) | `StringAttribute` (Optional/Computed) |

## versioning_legacy block-vs-attribute decision

`versioning_legacy` was a `TypeSet` (MaxItems: 1 equivalent) in the SDKv2 V1 schema. In the framework it is rendered as `SetNestedAttribute` (not `SingleNestedBlock`) because:
- The set already has block-like HCL syntax (`versioning_legacy { ... }`) in existing configs.
- Changing to `SingleNestedBlock` would be compatible; changing to `SingleNestedAttribute` would break `foo { ... }` → `foo = { ... }` for existing practitioners.
- The resource is deprecated (`Deprecated: "Use newer versioning implementation"`), so the conservative choice is `SetNestedAttribute` preserving the existing HCL shape.

## ForceNew → RequiresReplace

SDKv2 `ForceNew: true` on `region`, `storage_policy`, and `storage_class` became:
```go
PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}
```

## Default values

SDKv2 `Default: false` on `versioning` and `force_destroy` became:
```go
Default: booldefault.StaticBool(false)
```
`Default` goes in the `Default` field, **not** in `PlanModifiers` — the framework's `Default` type is `defaults.Bool`, which is incompatible with `planmodifier.Bool`.

## Delete reads from State

`req.Plan` is null on Delete. The Delete handler reads from `req.State`.
