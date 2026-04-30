# Migration notes — `resource_widget_v2.go`

## Scope

Single-resource migration from `terraform-plugin-sdk/v2` to
`terraform-plugin-framework`. The resource is at `SchemaVersion: 2` with two
**chained** SDKv2 state upgraders (V0→V1, V1→V2). The interesting part of
the migration is collapsing the chain into the framework's single-step
`UpgradeState` model.

## Schema versions at a glance

| Version | Schema | SDKv2 upgrader |
|---|---|---|
| V0 | `id`, `name`, `address` | `upgradeWidgetV0ToV1` (rename `address`→`host_port`) |
| V1 | `id`, `name`, `host_port` | `upgradeWidgetV1ToV2` (split `host_port`→`host`+`port`; default `tags`) |
| V2 (current) | `id`, `name`, `host`, `port`, `tags` | — |

## Chain-collapse — the load-bearing change

SDKv2 chained upgraders work because the framework / SDKv2 calls them in
sequence: V0 state goes through V0→V1, then the result goes through V1→V2.

The plugin-framework's `UpgradeState()` is **not** a chain. The framework
matches each map entry against the schema version of the state on disk and
calls **exactly one** upgrader to take that state directly to the current
version. So a chain of length 2 in SDKv2 becomes **two** independent
upgraders in the framework (`0:` and `1:`), **not** three, and the V0
upgrader must produce V2 state directly.

Concretely in this file:

- `UpgradeState()` returns a `map[int64]resource.StateUpgrader` with keys
  `0` and `1` only.
- Entry `0` (`upgradeFromV0`) reads V0 state through `priorSchemaV0()`
  (`id`, `name`, `address`) and writes the current `widgetModel` (`id`,
  `name`, `host`, `port`, `tags`). The V0→V1 step (rename `address` to a
  host:port string) is inlined as `hostPort := prior.Address.ValueString()`,
  immediately followed by the V1→V2 split via `splitHostPort`. **The V0
  upgrader does not call `upgradeFromV1`.**
- Entry `1` (`upgradeFromV1`) reads V1 state through `priorSchemaV1()`
  (`id`, `name`, `host_port`) and writes the current `widgetModel`. This is
  a direct port of the SDKv2 V1→V2 logic.

If you ever add a V3, you'll need to revisit *both* `upgradeFromV0` and
`upgradeFromV1` so each still terminates in current-schema state — that's
the framework's deliberate trade: more code per upgrader, but each upgrader
is a complete transformation that's locally reasoning-friendly.

## Type changes that fall out of this

- `port` is now an `Int64Attribute` in the V2 schema (was `schema.TypeInt`
  in SDKv2 → 32-bit Go `int`; framework default is 64-bit). The SDKv2
  upgrader stored `parts[1]` as a *string* (`raw["port"] = parts[1]`)
  relying on `helper/schema` coercion. The framework will **not** coerce,
  so the upgrader now does an explicit `strconv.ParseInt`. If parsing
  fails (e.g., legacy non-numeric port), the upgrader writes `0` — same
  fallback semantics as the SDKv2 `else { raw["port"] = "0" }` branch.
- `tags` defaults to a known *empty* map (not null), mirroring the SDKv2
  upgrader's `raw["tags"] = map[string]interface{}{}`. Constructed via
  `types.MapValueMust(types.StringType, map[string]attr.Value{})`.

## Other framework idioms applied

- `ForceNew: true` on `name` → `stringplanmodifier.RequiresReplace()`.
- `Computed: true` on `id` gains `stringplanmodifier.UseStateForUnknown()`
  so plans don't churn the ID.
- Importer becomes `ResourceWithImportState.ImportState` using
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` —
  the framework equivalent of `schema.ImportStatePassthroughContext` for
  a single-string ID.
- Compile-time interface assertions:
  `var _ resource.ResourceWithUpgradeState = &widgetResource{}` (and the
  same for `Resource` and `ResourceWithImportState`) — a missing method
  becomes a compile error rather than a runtime surprise.

## Things I deliberately did **not** do

- **Did not** put `Default: stringdefault.StaticString(...)` (or any
  `Default:`) on `tags`. Defaults belong on the *schema*, not state
  upgraders, but the source resource didn't have a schema-level default
  for `tags` — the empty-map behaviour was specifically a state-upgrade
  concern. Replicating that as a schema default would be a behaviour
  change for new resources; replicating it inside the upgraders preserves
  the SDKv2 semantics exactly.
- **Did not** call `upgradeFromV1` from `upgradeFromV0`. The transformations
  (`address` → split host:port, default tags) are inlined inside
  `upgradeFromV0`. The two upgraders share the `splitHostPort` helper but
  are otherwise independent — the chain anti-pattern stays out of the
  code.
- **Did not** carry over the SDKv2 `m interface{}` provider-meta
  parameter into the upgraders. The framework's `UpgradeStateRequest` does
  not carry provider data, and the V0/V1→V2 transformations here are
  pure (no API client needed). If a future schema version needs the
  client, the upgraders would need to become methods on `widgetResource`
  closing over `r.client`.

## Verification

This is a fixture-only migration; no real provider tree, no `go.mod`. The
verification I would run on a real repo:

1. `go build ./...` — catches the typed-model / `tfsdk:` tag mismatches
   first.
2. `go vet ./...`.
3. `TestProvider` calling `provider.InternalValidate()` — surfaces a wide
   class of schema-shape errors.
4. An acceptance test that pins V0 state via `ExternalProviders` (last
   SDKv2 release of the provider) and then runs a `PlanOnly: true` step
   against the migrated provider to assert no plan diff. Same shape for
   V1 state. This is the single most valuable test for a state-upgrader
   migration — see `references/state-upgrade.md` "Testing state upgrades".
5. Negative gate: `grep -l 'terraform-plugin-sdk/v2'` on the migrated
   file should return empty (it does — the migrated file imports only
   `terraform-plugin-framework` paths).
