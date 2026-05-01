# Migration Summary: resource_widget_v2.go

## Source

SDKv2 resource `ResourceWidget()` at `evals/fixtures/resource_widget_v2.go`.

- `SchemaVersion: 2`
- Two chained `StateUpgraders`:
  - V0 (address field) → V1 (host_port field)
  - V1 (host_port field) → V2 (host + port + tags fields)

## Migration decisions

### Schema (Version: 2)

The current (V2) schema is faithfully reproduced as a `schema.Schema{Version: 2, ...}` with:
- `id`: `StringAttribute{Computed: true}`
- `name`: `StringAttribute{Required: true}` with `stringplanmodifier.RequiresReplace()` (SDKv2 `ForceNew: true`)
- `host`: `StringAttribute{Required: true}`
- `port`: `Int64Attribute{Required: true}` (SDKv2 `TypeInt` → framework `Int64`)
- `tags`: `MapAttribute{Optional: true, ElementType: types.StringType}`

### State upgraders — single-step semantics

The SDKv2 chained pattern V0→V1→V2 is converted to two independent single-step upgraders per framework requirements:

| Map key | PriorSchema | Produces |
|---------|-------------|---------|
| `0` | `priorSchemaV0()` (id, name, address) | Current V2 state directly |
| `1` | `priorSchemaV1()` (id, name, host_port) | Current V2 state directly |

**V0 upgrader** composes both transformations inline:
1. Renames `address` → `host_port` (the V0→V1 step)
2. Splits `host_port` into `host` + `port` and sets `tags` to empty map (the V1→V2 step)

It does **not** call `upgradeWidgetFromV1`. This avoids the chain-habit anti-pattern.

**V1 upgrader** performs only the V1→V2 transformation: splits `host_port` and initialises `tags`.

Both upgraders use typed prior models (`widgetModelV0`, `widgetModelV1`) with `tfsdk:` tags matching `PriorSchema` attribute names exactly.

### Interface implementation

- `var _ resource.ResourceWithUpgradeState = &widgetResource{}` compile-time assertion
- `UpgradeState()` returns `map[int64]resource.StateUpgrader` with entries at keys `0` and `1`
- `≥2 PriorSchema` references: `priorSchemaV0()` and `priorSchemaV1()`

### Import

Passthrough import implemented via `ImportState` (replaces SDKv2 `schema.ImportStatePassthroughContext`).

## No SDKv2 imports

The migrated file imports only `terraform-plugin-framework` packages. No `terraform-plugin-sdk/v2` import is present.

## Files produced

- `migrated/resource_widget_v2.go` — fully migrated framework resource
