# Migration Summary: resource_widget_v2.go (SDKv2 → terraform-plugin-framework)

## Task

Migrate a SDKv2 resource (`ResourceWidget`) at SchemaVersion 2 with two chained
state upgraders (V0→V1, V1→V2) to terraform-plugin-framework using single-step
upgrader semantics.

## Key Decisions

### 1. Single-step semantics in UpgradeState()

The SDKv2 resource used chained upgraders — the framework applies them
sequentially when a resource is at an older version. In the framework, each
`UpgradeState()` entry must produce the **current** (V2) state directly.

- **Key `0` (V0→V2):** Reads the V0 schema (`id`, `name`, `address`), applies
  both the V0→V1 rename (`address` → `host_port`) and the V1→V2 split
  (`host_port` → `host` + `port`) inline in a single function. Does **not** call
  the V1 upgrader.
- **Key `1` (V1→V2):** Reads the V1 schema (`id`, `name`, `host_port`) and
  applies only the V1→V2 split inline.

### 2. Port type coercion

The original V1→V2 SDKv2 upgrader emitted port as a string (noting "framework
will coerce when re-typed"). In the framework upgrader, `strconv.ParseInt` is
used to convert the port string to `int64`, defaulting to `0` on parse error.
This is placed in both upgraders to correctly produce a typed `types.Int64Value`.

### 3. Tags default

V2 adds a `tags` map that did not exist in V0 or V1. Both upgraders default
`tags` to an empty `types.Map` (`types.MapValueMust(types.StringType, map[string]attr.Value{})`)
matching the original SDKv2 upgrader behavior.

### 4. Schema structure

- `widgetV2Schema()` is extracted as a helper function and reused in both
  `Schema()` and the `resp.State` assignments inside each upgrader, avoiding
  duplication.
- `schema.Schema.Version` is set to `2` in `widgetV2Schema()`.

### 5. Raw state decoding

Used `req.RawState.Unmarshal(tftypesObjectType)` with explicit `tftypes.Object`
definitions for each prior schema version, then `.As(&map[string]tftypes.Value)`
to extract individual typed values. This is the correct framework pattern for
decoding prior-version JSON state.

### 6. ImportState

Translated `schema.ImportStatePassthroughContext` to
`resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Files Produced

- `migrated/resource_widget_v2.go` — the complete framework resource

## Conformance Checklist

| Requirement | Status |
|---|---|
| `schema.Schema.Version` = 2 | PASS |
| `UpgradeState()` returns map with key `0` | PASS |
| `UpgradeState()` returns map with key `1` | PASS |
| Key `0` produces V2 state in one call | PASS |
| Key `1` produces V2 state in one call | PASS |
| V0 upgrader does NOT call V1 upgrader | PASS |
| Both transformations applied inline in V0 entry | PASS |
| Tags defaulted to empty map in both upgraders | PASS |
| `port` emitted as `int64` (not string) | PASS |
