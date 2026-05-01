# Migration Notes: openstack_objectstorage_container_v1

## State upgrade semantics

### Single-step rule
The SDKv2 resource had `SchemaVersion: 1` with one upgrader entry for V0. In the framework, `UpgradeState()` returns a `map[int64]resource.StateUpgrader`. Each map entry is keyed at the *prior* version and must produce the **current** (target) schema state in a single call — there is no chaining between entries.

- **Current version**: 1 (set as `schema.Schema{Version: 1, ...}`)
- **Prior version handled**: 0 (map key `0:`)
- **Single-step**: the V0 upgrader directly produces V1 (current) state. If there were additional versions (e.g., V0 and V1 → V2), each upgrader would independently produce the current V2 state without calling one from the other.

### The V0→V1 transformation
The original SDKv2 upgrader (`resourceObjectStorageContainerStateUpgradeV0`) did:
```
rawState["versioning_legacy"] = rawState["versioning"]  // rename old set block
rawState["versioning"] = false                          // new bool, defaults false
```

The framework upgrader (`upgradeObjectStorageContainerStateFromV0`) does the same transformation but typed:
1. Reads prior V0 state into `containerModelV0` (where `versioning` is `types.Set`)
2. Writes current V1 state into `containerModel` (where `versioning_legacy` gets the set value, `versioning` becomes `types.BoolValue(false)`)
3. `storage_class` was added in V1 (not present in V0 SDKv2 schema); defaults to `types.StringValue("")` for existing resources.

### PriorSchema requirement
Each upgrader entry must provide `PriorSchema` — the framework uses it to deserialize the old state before calling the upgrader function. The `PriorSchema` for V0 reflects the SDKv2 V0 schema as a framework `schema.Schema`, including the set-nested `versioning` attribute (type + location fields).

### tfsdk tag discipline
The `containerModelV0` struct uses `tfsdk:` tags that exactly match V0's attribute names (e.g., `tfsdk:"versioning"` for the old set block, not `tfsdk:"versioning_legacy"`). Tag mismatch is the #1 silent state-mapping bug — a mismatched tag causes the field to read as zero/null with no compile error.

## Schema changes: V0 → V1 (current)

| Attribute | V0 | V1 (current) |
|---|---|---|
| `versioning` | `TypeSet` (block with type + location) | `types.Bool` (simple bool, default false) |
| `versioning_legacy` | absent | `SetNestedAttribute` (same shape as old `versioning`) |
| `storage_class` | absent | `StringAttribute` (Computed+Optional, ForceNew) |

## Block vs attribute decision
`versioning_legacy` was kept as a `SetNestedAttribute` (not converted to `SingleNestedAttribute`) because:
1. Practitioners use block syntax in production configs (`versioning_legacy { ... }`)
2. The attribute is deprecated and should not be changed further

## ConflictsWith
The SDKv2 `ConflictsWith: []string{"versioning"}` constraint between `versioning` and `versioning_legacy` is enforced at the API layer (the Update handler explicitly sequences legacy→new and new→legacy transitions). A framework `validator.Conflicting(...)` could be added via `framework-validators` but is omitted here to keep parity with the SDKv2 behavior (which relied on API rejection, not SDK validation).
