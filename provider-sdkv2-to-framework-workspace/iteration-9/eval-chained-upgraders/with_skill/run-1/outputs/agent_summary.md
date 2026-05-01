# Migration Summary — resource_widget_v2.go

## What was done

Migrated `ResourceWidget()` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The resource had `SchemaVersion: 2` with two **chained** SDKv2 upgraders (V0→V1, V1→V2). These were rewritten as **single-step** framework upgraders per the skill's state-upgrade semantics.

## Key decisions

### Single-step vs chained
SDKv2 chained V0→V1 (rename `address`→`host_port`) then V1→V2 (split `host_port`→`host`+`port`, add `tags`). The framework's `UpgradeState()` returns a map where each entry produces the *current* (V2) state directly. The V0 upgrader composes both transformations inline without calling `upgradeFromV1`.

### UpgradeState map
`map[int64]resource.StateUpgrader` with exactly two entries:
- Key `0`: uses `priorSchemaV0()` (attributes: `id`, `name`, `address`) and `upgradeFromV0`
- Key `1`: uses `priorSchemaV1()` (attributes: `id`, `name`, `host_port`) and `upgradeFromV1`

### Type models
- `widgetModelV0` — `tfsdk` tags match `priorSchemaV0` (`address`)
- `widgetModelV1` — `tfsdk` tags match `priorSchemaV1` (`host_port`)
- `widgetModel`  — current V2 model (`id`, `name`, `host`, `port`, `tags`)

### Port parsing
The SDKv2 V1→V2 upgrader emitted the port as a string (`parts[1]`). The framework migration uses `strconv.ParseInt` to produce a true `types.Int64Value`, matching the current schema's `Int64Attribute`.

### Tags default
Both upgraders default `tags` to an empty `types.Map` (element type `types.StringType`) when no prior value exists, matching what the SDKv2 upgrader did (`raw["tags"] = map[string]interface{}{}`).

### schema.Schema.Version
Set to `2` on the current schema, matching `SchemaVersion: 2` in the SDKv2 resource.

### ForceNew → RequiresReplace
`name` had `ForceNew: true`; translated to `stringplanmodifier.RequiresReplace()`.

### Import
`ResourceWithImportState` implemented with `path.Root("id")` passthrough (equivalent to `schema.ImportStatePassthroughContext`).

## Criteria check

| Criterion | Status |
|---|---|
| No `terraform-plugin-sdk/v2` import | PASS |
| `UpgradeState` map has entries keyed at `0` and `1` | PASS |
| ≥2 `PriorSchema` references | PASS (2: `priorSchemaV0()` and `priorSchemaV1()`) |
| `schema.Schema.Version = 2` | PASS |
| Framework import present | PASS |
| V0 upgrader does NOT call V1 upgrader | PASS |
| Valid Go (no SDKv2 types, typed models, correct imports) | PASS |
