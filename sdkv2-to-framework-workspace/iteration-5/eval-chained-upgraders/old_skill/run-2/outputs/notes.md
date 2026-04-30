# Migration notes — `resource_widget_v2.go` (SDKv2 → plugin-framework)

## What this fixture exercises

A SchemaVersion-2 resource with **two chained SDKv2 state upgraders**:

```
V0 (id, name, address)        V1 (id, name, host_port)        V2 (id, name, host, port, tags)
        |  upgradeWidgetV0ToV1                  |  upgradeWidgetV1ToV2
        v                                       v
   address -> host_port                  host_port -> host + port
                                         tags defaults to {}
```

The migration trap is the chain semantics. SDKv2 chained upgraders compose
(V0 → V1 → V2). The framework requires **single-step upgraders**: each entry
in the `UpgradeState()` map must produce the **current (V2)** state directly
in one call.

## What I produced

`resource.ResourceWithUpgradeState` with **two** entries (matching the count
of *prior* versions, V0 and V1 — not three):

```go
func (r *widgetResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {PriorSchema: priorSchemaV0(), StateUpgrader: upgradeWidgetFromV0},
        1: {PriorSchema: priorSchemaV1(), StateUpgrader: upgradeWidgetFromV1},
    }
}
```

- **`upgradeWidgetFromV0`** reads V0 state (`id, name, address`) and produces
  V2 state directly: it inlines `address → host_port` and then
  `host_port → host + port` plus `tags = {}`. **It does NOT call
  `upgradeWidgetFromV1`** — the V0 upgrader stands alone.
- **`upgradeWidgetFromV1`** reads V1 state (`id, name, host_port`) and
  produces V2 state directly: split `host_port → host + port`, default
  empty `tags`.

Both upgraders use typed prior models (`widgetModelV0`, `widgetModelV1`) whose
`tfsdk:` tags exactly match their respective `PriorSchema`. The current model
(`widgetModel`) uses V2 tags.

## Why V0 doesn't chain into V1

`references/state-upgrade.md` is explicit:

> Keep V0 → current logic separate from V1 → current logic — don't try to call
> one from the other; the testability suffers.

It's also a correctness concern: framework upgraders take a `req` and `resp`
typed against a *specific* `PriorSchema`. Calling `upgradeWidgetFromV1(req, resp)`
from inside `upgradeWidgetFromV0` would feed V0-shaped state through a V1-shaped
deserialiser. So instead I extracted the shared bit (`splitHostPort`) into a
plain helper and called it from both upgraders.

## Other framework conversions in the file

These are mechanical and incidental to the upgrader logic, but the file has
to compile to be useful:

| SDKv2                                              | Framework                                                                   |
|----------------------------------------------------|-----------------------------------------------------------------------------|
| `schema.Resource{...}` constructor                 | `widgetResource` struct + `NewWidgetResource()` constructor                 |
| `CreateContext`, `ReadContext`, ...                | `Create`, `Read`, `Update`, `Delete` methods on the resource type           |
| `ForceNew: true` on `name`                         | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` |
| `Computed: true` on `id` (no UseStateForUnknown)   | Added `stringplanmodifier.UseStateForUnknown()` — common idiom for computed IDs |
| `Importer: ImportStatePassthroughContext`          | `ResourceWithImportState` + `resource.ImportStatePassthroughID(...)`        |
| `schema.TypeMap, Elem: TypeString`                 | `schema.MapAttribute{ElementType: types.StringType, Optional: true}`        |
| `schema.TypeInt`                                   | `schema.Int64Attribute`                                                     |
| `SchemaVersion: 2`                                 | `Version: 2` on the framework `schema.Schema{}`                             |
| `StateUpgraders: []schema.StateUpgrader{...}`      | `ResourceWithUpgradeState.UpgradeState()` returning `map[int64]resource.StateUpgrader` |

## Compile-time interface assertions

Per `references/state-upgrade.md` ("Implement `resource.ResourceWithUpgradeState`
and add `var _ resource.ResourceWithUpgradeState = &thingResource{}` so a
missing method is a compile error"):

```go
var (
    _ resource.Resource                 = &widgetResource{}
    _ resource.ResourceWithImportState  = &widgetResource{}
    _ resource.ResourceWithUpgradeState = &widgetResource{}
)
```

## Behaviour-preserving details

- **Empty `tags` default**: SDKv2 V1→V2 inserted `map[string]interface{}{}`
  when `tags` was absent. The framework upgrader writes
  `types.MapValueMust(types.StringType, map[string]attr.Value{})` — a
  *non-null, empty* map of strings. Using `types.MapNull` instead would round-
  trip differently (`null` vs `{}`), so I matched the SDKv2 semantic.
- **`port` type**: SDKv2 upgrader emitted `port` as a string and relied on
  framework coercion at re-type time. Since the V2 framework schema declares
  `port` as `Int64Attribute`, I do the parse explicitly in `splitHostPort`
  and write `types.Int64Value(...)`. Bad input (non-integer port) silently
  becomes 0, which matches the SDKv2 fallback (`raw["port"] = "0"`).
- **`name` ForceNew**: preserved as `RequiresReplace` plan modifier. Schema
  attribute names (`id`, `name`, `host`, `port`, `tags`) are unchanged
  — practitioner state shape is preserved.

## What I deliberately did NOT do

- Did not introduce `terraform-plugin-mux`. Single-release migration as
  scoped by SKILL.md.
- Did not rename any user-facing attribute. The V2 schema keeps the same
  attribute names as the SDKv2 V2 schema.
- Did not collapse the two upgraders into one. Even though the V0 upgrader's
  body subsumes V1's transformation, the framework requires distinct entries
  keyed by *prior* version.
