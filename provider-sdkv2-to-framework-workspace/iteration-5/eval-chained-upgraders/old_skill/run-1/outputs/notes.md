# Migration notes — `resource_widget_v2.go`

## Scope

Single resource: `widget`, currently at `SchemaVersion: 2` with two CHAINED
SDKv2 state upgraders (`V0 → V1`, `V1 → V2`). Migrating to
`terraform-plugin-framework` while preserving state-upgrade behaviour.

## The load-bearing decision: chained → single-step

SDKv2 chains state upgraders: each `StateUpgrader` only has to take state
one version forward, and the framework runtime composes them. The
plugin-framework deliberately broke that pattern — `UpgradeState()` returns
a `map[int64]resource.StateUpgrader` keyed by the **prior** version, and
each entry must produce the **current** (target) state in one call.

So an SDKv2 chain `V0 → V1 → V2` becomes **two** framework upgraders, not
three:

| Prior version | Framework entry         | Produces |
| ------------- | ----------------------- | -------- |
| 0             | `0: { upgradeFromV0 }`  | V2       |
| 1             | `1: { upgradeFromV1 }`  | V2       |

There is no entry keyed `2` — that's the current schema, nothing to
upgrade from.

### Why `upgradeFromV0` does NOT call `upgradeFromV1`

It would be tempting to write `upgradeFromV0` as "convert the V0 raw map
into the shape `upgradeFromV1` expects, then call `upgradeFromV1`". I
deliberately did not do that. Reasons:

1. **Framework single-step contract.** Each upgrader is invoked once by
   the framework; chaining inside the upgrader body re-introduces SDKv2
   semantics in a place the framework no longer expects them. Future
   maintainers reading `UpgradeState()` should be able to assume each
   entry is independent.
2. **Testability.** Each upgrader can be unit-tested with a pinned prior
   state and an expected V2 result. If V0 chained into V1, a bug in V1
   would also break V0's test, masking the actual fault.
3. **Skill guidance.** `references/state-upgrade.md` explicitly calls out
   this pitfall: "Returning V1 from the V0 upgrader (chain habit) … leaves
   the state two versions behind."

The transformation is therefore **inlined** — `upgradeFromV0` reads the V0
prior model (`id`, `name`, `address`), splits `address` directly into
`host` and `port`, and supplies an empty `tags` map. That composes the
legacy V0→V1 (rename `address` to `host_port`) and V1→V2 (split
`host_port`, default `tags`) transformations into one direct V0→V2 step
without sharing code with `upgradeFromV1`.

## Per-version mapping

### V0 prior schema (`priorSchemaV0`)

| Attribute | Type   | Notes                |
| --------- | ------ | -------------------- |
| `id`      | string | computed             |
| `name`    | string | required, ForceNew   |
| `address` | string | required (host:port) |

### V1 prior schema (`priorSchemaV1`)

| Attribute   | Type   | Notes              |
| ----------- | ------ | ------------------ |
| `id`        | string | computed           |
| `name`      | string | required, ForceNew |
| `host_port` | string | required           |

### V2 current schema

| Attribute | Type        | Notes              |
| --------- | ----------- | ------------------ |
| `id`      | string      | computed           |
| `name`    | string      | required, ForceNew |
| `host`    | string      | required           |
| `port`    | int64       | required           |
| `tags`    | map[string] | optional           |

## Subtleties handled

- **`port` typing.** The SDKv2 V1→V2 upgrader stored `port` as a string
  ("upgrader emits string; framework will coerce when re-typed"). The
  framework typed model is `types.Int64`, and prior-state deserialisation
  goes through `PriorSchema`, not the current one — so the V1 prior model
  reads `host_port` as a string and we parse it to `int64` in
  `splitHostPort` before assigning to the V2 `Port` field. Same for V0.
- **`ForceNew: true` → `RequiresReplace`.** The SDKv2 `name` field had
  `ForceNew: true`. In the framework that's a plan modifier:
  `stringplanmodifier.RequiresReplace()`. Carried through on the current
  schema and on both prior schemas (so prior-state diffs of `name` would
  also flag a replace, matching SDKv2 behaviour).
- **Importer.** `schema.ImportStatePassthroughContext` becomes
  `resource.ImportStatePassthroughID` keyed at `path.Root("id")`, wired
  via `ImportState` on the resource type.
- **Empty `tags` default.** The legacy V1→V2 upgrader defaulted `tags` to
  `map[string]interface{}{}`. The framework equivalent is
  `types.MapValue(types.StringType, map[string]attr.Value{})`, returned
  from `emptyStringMap()` as a non-null empty map (not `types.MapNull`).
  The V0 → current path uses the same helper, since V0 had no `tags`
  whatsoever.
- **Compile-time interface assertions.** Added the trio
  `_ resource.Resource`, `_ resource.ResourceWithImportState`,
  `_ resource.ResourceWithUpgradeState` so a missing method is a compile
  error, not a runtime surprise.

## What I deliberately did NOT do

- Did **not** introduce `terraform-plugin-mux` — single-release migration
  per the skill's scope rules.
- Did **not** rename any user-facing attribute. `id`, `name`, `host`,
  `port`, `tags` are preserved exactly so practitioner state files map
  one-to-one.
- Did **not** retire the V0 upgrader by stitching into V1's body; see
  "Why `upgradeFromV0` does NOT call `upgradeFromV1`" above.

## Outstanding follow-ups (out of scope of this fixture)

- The original CRUD methods are stubs in the fixture, so the migrated
  CRUD methods are also stubs (read plan, write to state). Real provider
  CRUD logic must be ported in a separate pass.
- Acceptance tests for the upgrade path: pin a V0 state with the last
  SDKv2 release as `ExternalProviders`, then run `PlanOnly` against the
  migrated provider to assert no plan diff. Pattern in
  `references/state-upgrade.md` § "Testing state upgrades". Repeat for
  V1 prior state.
