# Agent Summary — SDKv2 → Framework Migration (resource_widget_v2.go)

## Task

Migrated the `resource_widget_v2.go` fixture from `terraform-plugin-sdk/v2` to
`terraform-plugin-framework`. The resource had `SchemaVersion: 2` with two
chained SDKv2 state upgraders (V0→V1, V1→V2), which had to be translated into
the framework's single-step `UpgradeState()` semantics.

## Key decisions

### State upgrader collapse

The SDKv2 chain `V0→V1→V2` was flattened into two independent framework upgraders:

| Map key | Prior schema fn | Upgrader fn | What it does |
|---------|----------------|-------------|--------------|
| `0` | `priorSchemaV0()` | `upgradeFromV0` | Composes V0→V1 (rename `address`→`host_port`) **and** V1→V2 (split into `host`/`port`, init `tags`) in a single body — does **not** call `upgradeFromV1` |
| `1` | `priorSchemaV1()` | `upgradeFromV1` | Ports V1→V2 directly: splits `host_port` into typed `host` + `port`, initialises `tags` |

Both entries produce the **current (V2) schema state** in one call. No chaining.

### Schema version

`schema.Schema.Version` is set to `2` (the current version), matching the original
`SchemaVersion: 2` on the SDKv2 resource.

### Chain-habit avoidance

`upgradeFromV0` composes both transformation steps inline. It does **not** call
`upgradeFromV1`. A shared helper `splitHostPort()` encapsulates the string-split
logic used by both upgraders without introducing any cross-upgrader dependency.

### Prior-schema typed models

- `widgetModelV0` — `id`, `name`, `address` (`tfsdk` tags match `priorSchemaV0`)
- `widgetModelV1` — `id`, `name`, `host_port` (`tfsdk` tags match `priorSchemaV1`)
- `widgetModel` — current V2 shape: `id`, `name`, `host`, `port`, `tags`

### Other migration choices

- `ForceNew: true` on `name` → `stringplanmodifier.RequiresReplace()`
- `TypeMap` of `TypeString` → `schema.MapAttribute{ElementType: types.StringType}`
- `TypeInt` → `schema.Int64Attribute` (standard int64 in the framework)
- `ImportStatePassthroughContext` → `resource.ImportStatePassthroughID`
- No SDKv2 imports remain; all `diag`/`schema` references are framework packages

## Output file

`migrated/resource_widget_v2.go` — valid Go, no `terraform-plugin-sdk/v2` import,
`UpgradeState()` returns map with entries at keys `0` and `1`, two distinct
`PriorSchema:` references (`priorSchemaV0()` and `priorSchemaV1()`),
`schema.Schema.Version = 2`.
