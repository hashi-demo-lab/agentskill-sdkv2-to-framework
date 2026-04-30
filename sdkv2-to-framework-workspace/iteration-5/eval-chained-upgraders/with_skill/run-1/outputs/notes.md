# Migration notes — `resource_widget_v2.go` (chained upgraders)

## Single-step semantics (the headline)

SDKv2's `StateUpgraders` is a chain: each entry transforms `Vn -> Vn+1`,
relying on the framework to compose them when state is more than one
version behind. The fixture has **two** chained entries:

- `Version: 0` → `upgradeWidgetV0ToV1` (rename `address` → `host_port`)
- `Version: 1` → `upgradeWidgetV1ToV2` (split `host_port` → `host` + `port`,
  default `tags` to `{}`)

The terraform-plugin-framework rejects this composition model. Its
`UpgradeState()` returns `map[int64]resource.StateUpgrader` keyed by the
**prior** schema version, and **each entry must produce the CURRENT (V2)
state in one call**. There is no chain: the framework calls each entry
independently with its matching `PriorSchema`. State that lands at the
key-`0` entry was *never* touched by the key-`1` entry.

That gives us **two** framework upgraders, not three:

- key `0`: `upgradeFromV0` — V0 state → V2 state directly.
- key `1`: `upgradeFromV1` — V1 state → V2 state directly.

The migration trap is the "chained habit": writing
`func upgradeFromV0(...) { ...; upgradeFromV1(...) }`. The skill calls this
out explicitly. We avoid it by composing the *transformations* (not the
upgrader functions) by hand inside `upgradeFromV0`, sharing the
`splitHostPort` helper between both entries so the V0 path doesn't need to
call the V1 path.

The current schema declares `Version: 2`; the prior schemas
(`priorSchemaV0` / `priorSchemaV1`) describe the SDKv2 shapes the framework
must deserialise prior state through.

## Quoted upgrader bodies

### V0 → current (V2) — the composed body

The framework upgrader produces V2 state directly from V0. The body is:

```go
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
    var prior widgetModelV0
    resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
    if resp.Diagnostics.HasError() {
        return
    }

    // Compose V0->V1->V2 transformations inline.
    //   Step 1 (was V0->V1): "address" carried the host:port string and was
    //   renamed to "host_port". We skip the intermediate name and read it
    //   directly out of prior.Address.
    //   Step 2 (was V1->V2): split it into "host" + "port"; default "tags" to {}.
    host, port := splitHostPort(prior.Address.ValueString())

    emptyTags, mapDiags := types.MapValue(types.StringType, map[string]attr.Value{})
    resp.Diagnostics.Append(mapDiags...)
    if resp.Diagnostics.HasError() {
        return
    }

    current := widgetModel{
        ID:   prior.ID,
        Name: prior.Name,
        Host: types.StringValue(host),
        Port: types.Int64Value(port),
        Tags: emptyTags,
    }
    resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
```

Note the V0 → V1 "rename" is collapsed into a direct read of
`prior.Address`: there's no intermediate `host_port` field because we never
materialise V1 state. We do **not** call `upgradeFromV1`.

For reference, the SDKv2 V0→V1 body the rename came from is:

```go
func upgradeWidgetV0ToV1(ctx context.Context, raw map[string]interface{}, m interface{}) (map[string]interface{}, error) {
    if addr, ok := raw["address"].(string); ok {
        raw["host_port"] = addr
        delete(raw, "address")
    }
    return raw, nil
}
```

### V1 → current (V2) — the direct port

This is the framework port of the SDKv2 V1→V2 upgrader. The body is:

```go
func upgradeFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
    var prior widgetModelV1
    resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
    if resp.Diagnostics.HasError() {
        return
    }

    host, port := splitHostPort(prior.HostPort.ValueString())

    emptyTags, mapDiags := types.MapValue(types.StringType, map[string]attr.Value{})
    resp.Diagnostics.Append(mapDiags...)
    if resp.Diagnostics.HasError() {
        return
    }

    current := widgetModel{
        ID:   prior.ID,
        Name: prior.Name,
        Host: types.StringValue(host),
        Port: types.Int64Value(port),
        Tags: emptyTags,
    }
    resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
```

For reference, the SDKv2 V1→V2 body it's a port of:

```go
func upgradeWidgetV1ToV2(ctx context.Context, raw map[string]interface{}, m interface{}) (map[string]interface{}, error) {
    if hp, ok := raw["host_port"].(string); ok {
        parts := strings.SplitN(hp, ":", 2)
        raw["host"] = parts[0]
        if len(parts) == 2 {
            raw["port"] = parts[1] // upgrader emits string; framework will coerce when re-typed
        } else {
            raw["port"] = "0"
        }
        delete(raw, "host_port")
    }
    if _, ok := raw["tags"]; !ok {
        raw["tags"] = map[string]interface{}{}
    }
    return raw, nil
}
```

## Type-correctness fix-up

The SDKv2 V1→V2 upgrader stored `port` as a *string* and relied on the
framework's coercion when writing back through the typed schema. The
framework upgrader writes the typed model directly, so `splitHostPort`
returns an `int64` (parsed via `strconv.ParseInt`, falling back to `0` on
parse error — same fallback the SDKv2 body had for the missing-colon case).

`tags` is initialised to an empty map via `types.MapValue(types.StringType,
map[string]attr.Value{})`, mirroring the SDKv2 default.

## Other migration choices (low-stakes)

- `ForceNew: true` on `name` becomes
  `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`.
- `id` gets `stringplanmodifier.UseStateForUnknown()` so refresh/plan don't
  oscillate.
- `schema.TypeInt` (Go `int`) is mapped to `Int64Attribute` per the skill's
  default — there's no API contract pinning this to 32-bit.
- `schema.TypeMap` of strings becomes
  `MapAttribute{ElementType: types.StringType}`.
- `Importer: ImportStatePassthroughContext` becomes
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` on
  the resource's `ImportState` method.
- Compile-time interface assertions (`var _ resource.ResourceWithUpgradeState
  = &widgetResource{}`) ensure a missing `UpgradeState` is a build error.
- CRUD bodies are stubs — content irrelevant to this eval; the upgrader
  logic is the focus.

## Skill-flagged anti-pattern, explicitly avoided

> "If you find yourself writing
> `func upgradeFromV0(...) { ...; return upgradeFromV1(...) }` (or any
> variation that calls one upgrader from inside another), stop." —
> `references/state-upgrade.md`

`upgradeFromV0` does not call `upgradeFromV1`. Both functions read prior
state via their own `PriorSchema`-typed model, transform, and write the
current model independently. The shared `splitHostPort` helper is not an
upgrader call — it's just the parse logic, used by both call sites.
