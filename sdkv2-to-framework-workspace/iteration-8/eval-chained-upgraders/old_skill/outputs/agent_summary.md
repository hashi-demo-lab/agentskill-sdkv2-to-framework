# Migration Summary: resource_widget_v2.go

## Task

Migrate the SDKv2 `ResourceWidget` (SchemaVersion: 2, two chained upgraders V0→V1 and V1→V2) to terraform-plugin-framework with correct single-step upgrade semantics.

## Key decisions

### State upgrader restructuring

The SDKv2 resource used chained upgraders:
- `upgradeWidgetV0ToV1`: renamed `address` → `host_port`
- `upgradeWidgetV1ToV2`: split `host_port` → `host` + `port`, added `tags`

The framework requires single-step semantics. `UpgradeState()` returns a `map[int64]resource.StateUpgrader` with entries at keys **0** and **1**, each producing current (V2) state directly:

- Key `0` (`upgradeFromV0`): composes both transformations inline — renames `address` to the intermediate `hostPort` variable, then splits into `host`/`port` and initialises `tags`. Does **not** call `upgradeFromV1`.
- Key `1` (`upgradeFromV1`): ports the V1→V2 transformation directly (split `host_port`, add `tags`).

### PriorSchema definitions

Two `PriorSchema` functions (`priorSchemaV0`, `priorSchemaV1`) describe the historical schema shapes so the framework can deserialise prior state correctly.

### Type models

Three typed models:
- `widgetModelV0` (id, name, address)
- `widgetModelV1` (id, name, host_port)
- `widgetModel` (id, name, host, port, tags) — current

### Other migration points

- `ForceNew: true` on `name` → `stringplanmodifier.RequiresReplace()` plan modifier
- `TypeInt` → `schema.Int64Attribute` (port); SDKv2 upgrader emitted a string, framework upgrader now parses to `int64` via `fmt.Sscanf`
- `TypeMap` with string `Elem` → `schema.MapAttribute{ElementType: types.StringType}`
- `Importer` passthrough → `resource.ImportStatePassthroughID`
- `schema.Version: 2` set on the framework schema
- Compile-time interface assertion: `var _ resource.ResourceWithUpgradeState = &widgetResource{}`
- No SDKv2 imports remain

## Output file

`migrated/resource_widget_v2.go`
