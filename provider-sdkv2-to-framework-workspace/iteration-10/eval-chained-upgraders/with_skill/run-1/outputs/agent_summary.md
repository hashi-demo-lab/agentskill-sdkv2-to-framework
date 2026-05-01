# Agent Summary — eval-chained-upgraders (with_skill, iteration-10)

## Task

Migrate `resource_widget_v2.go` from SDKv2 (SchemaVersion 2, two chained upgraders V0→V1 and V1→V2) to terraform-plugin-framework with single-step upgrader semantics.

## Skill guidance applied

- Loaded `SKILL.md` first; identified `StateUpgraders` / `SchemaVersion > 0` pattern → loaded `references/state-upgrade.md` per the frugal-mode lookup table.
- Key rule from `state-upgrade.md`: framework upgraders are **single-step**, not chained. Each `UpgradeState` map entry keyed at a prior version must produce the *current* (V2) state directly in one call.
- Anti-pattern avoided: V0 upgrader must NOT call `upgradeFromV1` (the chained-habit anti-pattern). Instead, the V0→V1 and V1→V2 transformations are composed inline inside `upgradeFromV0`.

## What was migrated

### Removed (SDKv2)
- All `github.com/hashicorp/terraform-plugin-sdk/v2` imports
- `*schema.Resource` return type, `StateUpgraders []schema.StateUpgrader`
- Raw `map[string]interface{}` upgrader functions (`upgradeWidgetV0ToV1`, `upgradeWidgetV1ToV2`)
- `resourceWidgetV0()` and `resourceWidgetV1()` returning `*schema.Resource`

### Added (framework)
- Framework imports: `resource`, `resource/schema`, `types`, `attr`, `diag`, `planmodifier`, `stringplanmodifier`
- `widgetResource` struct implementing `resource.Resource`, `resource.ResourceWithImportState`, `resource.ResourceWithUpgradeState`
- `schema.Schema{Version: 2, ...}` with typed attributes; `ForceNew: true` translated to `stringplanmodifier.RequiresReplace()`
- `priorSchemaV0()` and `priorSchemaV1()` — two `*schema.Schema` PriorSchema definitions (≥2 PriorSchema refs)
- Typed prior models `widgetModelV0` (tfsdk: id, name, address) and `widgetModelV1` (tfsdk: id, name, host_port)
- `UpgradeState()` returning `map[int64]resource.StateUpgrader` with entries keyed at `0` and `1`
- `upgradeFromV0`: reads V0 prior state, composes V0→V1 rename (`address`→`host_port`) and V1→V2 split (`host_port`→`host`+`port`) inline, writes current V2 `widgetModel` — does NOT call `upgradeFromV1`
- `upgradeFromV1`: reads V1 prior state, splits `host_port` into typed `host` (string) + `port` (int64), writes current V2 `widgetModel`
- `splitHostPort()` helper using `strconv.ParseInt` (the SDKv2 upgrader emitted strings; the framework model is now typed `int64`)
- Full CRUD stubs and passthrough `ImportState`

## Key decisions

| Decision | Choice | Reason |
|---|---|---|
| Chain composition | Inline in V0 body | Framework calls each upgrader independently; no chain possible |
| Port type | `types.Int64` / `schema.Int64Attribute` | SDKv2 had `TypeInt`; proper typed translation |
| Tags default | `types.MapValueMust(types.StringType, map[string]attr.Value{})` | SDKv2 upgrader defaulted to empty map; preserved semantics |
| ForceNew → RequiresReplace | `stringplanmodifier.RequiresReplace()` | Correct framework translation; not a plan modifier `Default` |

## Output files

- `migrated/resource_widget_v2.go` — no SDKv2 import; UpgradeState map with entries 0 and 1; 2 PriorSchema refs (`priorSchemaV0`, `priorSchemaV1`); `schema.Schema.Version = 2`; framework imports only; V0 does not call upgradeFromV1; valid Go.
