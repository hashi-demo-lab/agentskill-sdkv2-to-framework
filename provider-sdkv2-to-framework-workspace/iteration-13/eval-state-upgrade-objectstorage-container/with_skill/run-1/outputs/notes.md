# Migration notes — `openstack_objectstorage_container_v1`

## State upgrader semantics: SDKv2 chained vs framework single-step

The most consequential difference between the two SDKs for this resource is the
shape of `StateUpgraders`.

### SDKv2 — chained upgraders

In SDKv2, `StateUpgraders` is an ordered slice of upgrader entries. Each entry
is a step from version `N` to version `N+1`. The framework runtime applies them
in sequence: V0 → V1 → V2 → … → current. Each step assumes the previous one
has already run.

```go
SchemaVersion: 1,
StateUpgraders: []schema.StateUpgrader{
    {Version: 0, Type: ..., Upgrade: resourceObjectStorageContainerStateUpgradeV0},
    // a hypothetical V1→V2 entry would chain off the result of V0→V1.
},
```

The function signature reflects the chained-step assumption:

```go
func resourceObjectStorageContainerStateUpgradeV0(_ context.Context, rawState map[string]any, _ any) (map[string]any, error)
```

— rawState comes in as the **prior version's shape**, leaves as the **next
version's shape**, and the framework will keep applying upgraders until it
reaches `SchemaVersion`.

### Framework — single-step upgraders

`terraform-plugin-framework` deliberately broke the chain. `UpgradeState`
returns `map[int64]resource.StateUpgrader` keyed by **prior version**, and each
entry must produce the **current (target)** schema's state directly in one
call. There is no chaining; the framework calls *exactly one* upgrader for any
given input version, and that one upgrader is responsible for getting all the
way to the current schema.

Concretely, three terms come up:

- **prior-version**: the state's version when it was last written. The map key
  in `UpgradeState`.
- **current-version**: the value of `Version` on the resource's `schema.Schema`
  — the version the upgrader must produce.
- **target-version**: synonym for current-version in the framework's docs;
  emphasises that *every* entry in the map produces this version, not the
  next-up version.

So an SDKv2 chain V0 → V1 → V2 becomes a framework map with **two** entries:

```text
{0: priorSchemaV0 → produces V2 directly,
 1: priorSchemaV1 → produces V2 directly}
```

…not three, and *not* one entry per hop. The transformations the SDKv2 chain
expressed link-by-link must be **composed inline** inside each framework
upgrader — V0's body has to do the work of both the SDKv2 V0→V1 *and* V1→V2
hops in one shot.

### What this resource needed

The original SDKv2 schema declared `SchemaVersion: 1` with a single V0
upgrader, so the chain has only one hop. That makes the framework translation
mechanical — there is exactly one entry in `UpgradeState`, keyed `0`, that
produces V1 (the current/target version) state directly:

```go
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            PriorSchema:   priorSchemaV0(),
            StateUpgrader: upgradeFromV0,
        },
    }
}
```

The transformation itself replicates the original `rawState["versioning_legacy"]
= rawState["versioning"]; rawState["versioning"] = false` — but instead of
operating on a `map[string]any`, the framework upgrader reads a typed prior
model (matching `priorSchemaV0()`'s `tfsdk:` tags) and writes a typed current
model.

If a future schema bump adds V2, this single entry **must be revisited** so
that V0 → V2 is a complete transformation, *not* a step toward V1 — adding a
second entry keyed at `1` would not be enough on its own.

## Other framework-vs-SDKv2 deltas applied

- `MaxItems: 1` on the `versioning_legacy` set → kept as a `SetNestedBlock`
  (preserving the practitioner-visible HCL block syntax) with a `SizeAtMost(1)`
  validator.
- `ForceNew: true` on `region`, `storage_policy`, `storage_class` →
  `stringplanmodifier.RequiresReplace()` plan modifiers.
- `Optional + Computed` attributes get `UseStateForUnknown` so reading back the
  region/policy/class doesn't show as `(known after apply)` on every plan.
- `Default: false` on `versioning` and `force_destroy` → `booldefault.StaticBool(false)`.
- `validation.StringInSlice([...]string{"versions","history"}, true)` → a
  small case-insensitive validator on the `type` attribute.
- `schema.HashResource` / `schema.NewSet` calls dropped — the framework's
  `types.SetValue` handles uniqueness internally.
- Tests now use `ProtoV6ProviderFactories` (and assume a `testAccProtoV6ProviderFactories`
  symbol is in scope at provider-test level); the SDKv2 `ProviderFactories`
  field has been fully removed.
