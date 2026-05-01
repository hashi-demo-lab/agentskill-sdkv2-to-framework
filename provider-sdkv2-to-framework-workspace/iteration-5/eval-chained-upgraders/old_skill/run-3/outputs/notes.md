# Migration notes — resource_widget_v2.go (chained upgraders → single-step)

## Source shape (SDKv2)

- `SchemaVersion: 2` with two CHAINED `StateUpgraders`:
  - `Version: 0` → `upgradeWidgetV0ToV1` (rename `address` → `host_port`)
  - `Version: 1` → `upgradeWidgetV1ToV2` (split `host_port` → `host` + `port`; default `tags` to `{}`)
- Current (V2) schema: `id`, `name` (ForceNew), `host`, `port` (TypeInt), `tags` (TypeMap of strings).
- Passthrough importer.

## The load-bearing rule

> SDKv2 chained upgraders V0→V1→V2; framework upgraders take a `PriorSchema`
> and produce the *target* version's state in one call per upgrader function.
> (SKILL.md, Common pitfalls + `references/state-upgrade.md`.)

So the SDKv2 chain V0 → V1 → V2 (three states, two transitions) becomes
**TWO** framework upgrader entries:

| Map key | Prior schema | Result | Strategy |
|---|---|---|---|
| `0` | V0 (`id`, `name`, `address`) | V2 directly | **Compose** V0→V1 and V1→V2 inline |
| `1` | V1 (`id`, `name`, `host_port`) | V2 directly | **Port** V1→V2 directly |

The V0 entry MUST NOT call `upgradeWidgetFromV1` — that's the chained-upgrader
habit the framework explicitly forbids. Inlining keeps each upgrader independently
testable, per `references/state-upgrade.md`.

## Per-upgrader composition

### Key 0 (`upgradeWidgetFromV0`) — V0 → V2 direct

V0 has `address` (a "host:port" string). The V0→V1 step renames it to
`host_port`; V1→V2 splits that string into `host` + `port` and seeds `tags`
to an empty map. Composed inline:

1. Read `prior.Address` (V0 value).
2. Treat it as the host:port string (the V0→V1 rename is now a no-op variable
   name change, so we just use `prior.Address.ValueString()` directly as
   the host:port input).
3. Split into host + numeric port (with a 0 fallback for malformed input,
   matching the original V1→V2 behaviour, but converted to `Int64` since the
   V2 schema is `TypeInt`/`Int64Attribute` — the SDKv2 upgrader's note
   "framework will coerce when re-typed" is acknowledged here by parsing
   to `int64` ourselves rather than emitting a string).
4. Default `tags` to an empty `MapValue` of strings.

### Key 1 (`upgradeWidgetFromV1`) — V1 → V2 direct

Direct port of `upgradeWidgetV1ToV2`: split `host_port` → `host` + `port`,
default `tags` to empty map.

## Other migration decisions

- **Importer**: `schema.ImportStatePassthroughContext` → `ImportStatePassthroughID`
  on `path.Root("id")` (per `references/import.md`).
- **`ForceNew: true` on `name`**: → `stringplanmodifier.RequiresReplace()` plan
  modifier (per the "ForceNew does NOT translate to RequiresReplace: true"
  pitfall in SKILL.md).
- **`port` type**: SDKv2 `TypeInt` → framework `Int64Attribute` (Int32 needs
  framework v1.10+; safer default is Int64). The upgrader parses the string
  port to `int64` rather than relying on framework coercion of a string.
- **`tags`**: `TypeMap` with `Elem: TypeString` → `MapAttribute{ElementType: types.StringType}`.
- **Empty map default in upgrader**: `types.MapValueMust(types.StringType, map[string]attr.Value{})`.
- **Interface assertions**: declared at top of file so a missing
  `UpgradeState`/`ImportState` method is a compile error (per
  `references/state-upgrade.md` and `references/resources.md`).
- **CRUD bodies**: stubbed (the original fixture's bodies were also stubs).

## What was NOT changed

- User-facing schema attribute names and IDs are unchanged (`id`, `name`,
  `host`, `port`, `tags`), preserving practitioner state. Per SKILL.md
  pitfalls: "Don't change user-facing schema names or attribute IDs."
- The `port` parsing fallback (0 on malformed input) preserves the original
  upgrader's semantics, just expressed in the now-typed `int64` form.

## Verification gates I'd run next

Per SKILL.md "Verification gates", after this migration:

```sh
bash <skill-path>/scripts/verify_tests.sh <provider-repo-path> \
  --migrated-files internal/widget/resource_widget_v2.go
```

Layered checks:
1. `go build ./...` — catches the import/type errors.
2. `go vet ./...`.
3. `TestProvider` calling `provider.InternalValidate()` — schema sanity.
4. Acceptance test (per `references/state-upgrade.md` testing pattern):
   write V0 state via the published SDKv2 provider in step 1, then run the
   migrated provider with `PlanOnly: true` in step 2 and assert no diff.
   Repeat for V1 → V2.
5. Negative gate: this file no longer imports `terraform-plugin-sdk/v2`. Confirmed.
