# Migration notes — `resource_widget_v2.go`

Source: `sdkv2-to-framework/evals/fixtures/resource_widget_v2.go` (SDKv2,
`SchemaVersion: 2` with two **chained** state upgraders V0→V1→V2).
Target: `terraform-plugin-framework`, single-step upgraders.

## The single critical decision: chained → single-step

SDKv2's `StateUpgraders` slice runs in order — V0→V1 then V1→V2 — so each
upgrader only had to advance the state by one version. The framework's
`UpgradeState()` does **not** chain. Every entry in the returned
`map[int64]resource.StateUpgrader` is invoked **independently** with prior
state already deserialised through that entry's `PriorSchema`. State that
lands at the V0 upgrader was never seen by the V1 upgrader.

Concretely, the migration produces:

```
UpgradeState() map[int64]resource.StateUpgrader{
    0: { PriorSchema: priorSchemaV0(), StateUpgrader: upgradeWidgetFromV0 }, // V0 → V2
    1: { PriorSchema: priorSchemaV1(), StateUpgrader: upgradeWidgetFromV1 }, // V1 → V2
}
```

Two entries (one per prior version), **not three**. Each emits the *current*
(V2) `widgetModel` directly via `resp.State.Set(ctx, &current)`.

### V0 must NOT call V1's upgrader

The skill calls this out explicitly as the chained-habit anti-pattern. In
this migration `upgradeWidgetFromV0` performs the work of *both* legacy
transformations inline:

1. (was V0→V1) the V0 `address` field is conceptually renamed to V1's
   `host_port` — captured as a local string variable, no separate call.
2. (was V1→V2) that string is split via `splitHostPort` into `host` +
   `int64` port; `tags` is initialised to an empty map.

`upgradeWidgetFromV0` never invokes `upgradeWidgetFromV1`. Each upgrader is a
complete, independent V*x* → V2 transformation.

## Other notable conversions

| SDKv2 | Framework |
|---|---|
| `SchemaVersion: 2` on `*schema.Resource` | `Version: 2` on `schema.Schema` returned from `Schema()` |
| `StateUpgraders: []schema.StateUpgrader{...}` | `UpgradeState(ctx) map[int64]resource.StateUpgrader` (and `var _ resource.ResourceWithUpgradeState = &widgetResource{}`) |
| `Type: resourceWidgetV0().CoreConfigSchema().ImpliedType()` | `PriorSchema: priorSchemaV0()` returning a `*schema.Schema` matching V0's shape |
| `Upgrade: upgradeWidgetV0ToV1` (`func(ctx, raw map[string]interface{}, m) (map[string]interface{}, error)`) | `StateUpgrader: upgradeWidgetFromV0` (`func(ctx, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse)`) |
| `raw["port"] = "0"` (string-typed in upgrader, framework "will coerce") | typed `types.Int64Value(int64)` — V2's `port` is `Int64Attribute`, so emit a real int64 from the upgrader rather than relying on coercion. |
| `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` | `ImportState()` calling `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| `ForceNew: true` on `name` | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` |
| `Type: schema.TypeMap, Elem: &schema.Schema{Type: schema.TypeString}` | `schema.MapAttribute{ElementType: types.StringType, Optional: true}` |
| `CreateContext`/`ReadContext`/etc. returning `diag.Diagnostics` | `Create`/`Read`/`Update`/`Delete` mutating `resp.Diagnostics` |
| `func ResourceWidget() *schema.Resource` | `func NewWidgetResource() resource.Resource` returning a `&widgetResource{}` |

## Prior models

`widgetModelV0` and `widgetModelV1` carry `tfsdk:` tags exactly matching
`priorSchemaV0()` and `priorSchemaV1()`. The framework deserialises prior
state through the prior schema, so the prior-model field tags must match the
*prior* schema's attribute names (e.g., `tfsdk:"address"`, `tfsdk:"host_port"`),
not the current names.

## Things deliberately not changed

- User-facing attribute names (`id`, `name`, `host`, `port`, `tags`) are
  preserved exactly. Any rename here would be a state-breaking change for
  practitioners.
- CRUD bodies are kept as trivial round-trip stubs to mirror the fixture's
  intent ("the upgrader logic is the focus"). A real migration would port
  the API client calls; that work is out of scope for this eval.
- No timeout / sensitive / write-only / identity handling — the SDKv2 source
  had none.

## What I'd do next on a real codebase

1. Update the provider's `Resources()` list to register
   `NewWidgetResource` and remove the SDKv2 `ResourceWidget` entry.
2. Remove the SDKv2-only helpers (`resourceWidgetV0`, `resourceWidgetV1`,
   `upgradeWidgetV0ToV1`, `upgradeWidgetV1ToV2`) once nothing references
   them — the per-version *shape* now lives in `priorSchemaV0/V1` instead.
3. Add an acceptance test that pins a V0-shaped state file (via
   `ExternalProviders` at the last SDKv2 release), then plans on the new
   provider with `PlanOnly: true` to assert no diff. Repeat for V1.
   Without that test the V0 → V2 path is unverified.
4. Run `verify_tests.sh` with `--migrated-files
   internal/.../resource_widget_v2.go` so the negative gate confirms the
   file no longer imports `terraform-plugin-sdk/v2`.
