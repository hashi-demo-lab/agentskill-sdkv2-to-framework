# `digitalocean_firewall` SDKv2 → plugin-framework migration notes

Scope: just `digitalocean/firewall/resource_firewall.go` and its test
file. The wider provider migration (provider.go, datasources, the
`acceptance` package, the SDKv2 schema helpers in
`digitalocean/firewall/firewalls.go`) is intentionally out of scope —
this is a single-resource migration so it can land independently.

## What changed at a glance

| Aspect | SDKv2 (before) | Framework (after) |
|---|---|---|
| Resource container | `func ResourceDigitalOceanFirewall() *schema.Resource` | `firewallResource` struct + `NewFirewallResource()` constructor |
| CRUD | `CreateContext`/`ReadContext`/`UpdateContext`/`DeleteContext` package-level funcs | Methods on `*firewallResource`: `Create`, `Read`, `Update`, `Delete` |
| Configure | `meta.(*config.CombinedConfig)` cast inside every CRUD func | `Configure(...)` once, stores `*godo.Client` on the resource struct |
| Importer | `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` | `ImportState` method using `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| Schema | inline `firewallSchema()` (SDKv2 `*schema.Schema`) | `Schema(ctx, req, resp)` populating `resp.Schema = schema.Schema{Attributes:..., Blocks:...}` |
| Cross-attribute validation | `CustomizeDiff: func(...)` on the resource | `ResourceWithModifyPlan.ModifyPlan` method (see below) |

## The load-bearing translation: `CustomizeDiff` → `ModifyPlan`

The SDKv2 implementation used a single `CustomizeDiff` closure to
enforce three cross-attribute rules:

1. **At least one rule must be specified** — `!hasInbound && !hasOutbound` ⇒ error.
2. **Inbound `port_range` is required when `protocol != "icmp"`** — iterates the inbound set, checks each rule.
3. **Outbound `port_range` is required when `protocol != "icmp"`** — same shape for outbound.

These are all *cross-attribute* checks (rules 2 & 3 inspect two
attributes of the same nested object; rule 1 spans two top-level
blocks), so per-attribute `Validators` cannot express them. Per
`references/plan-modifiers.md` ("CustomizeDiff → ModifyPlan"), the
translation is the resource-level `ResourceWithModifyPlan.ModifyPlan`
method.

The migrated `ModifyPlan`:

- Asserts the interface at compile time:

  ```go
  var _ resource.ResourceWithModifyPlan = &firewallResource{}
  ```

- Short-circuits the **destroy phase** with the
  `req.Plan.Raw.IsNull()` sentinel from the four-state cheat sheet in
  the reference (Plan-null + State-non-null = destroy; nothing to
  validate).

- Reads the typed model with `req.Plan.Get(ctx, &plan)` and bails
  early on diagnostic errors.

- Folds the three SDKv2 legs into a single body, in the original
  order. The original was a single function (not a `customdiff.All`
  chain), so no flattening was needed — just translation. Each rule
  emits its own diagnostic with the same wording the SDKv2 user saw
  (`At least one rule must be specified`, `port_range\` of inbound rules
  is required if protocol is \`tcp\` or \`udp\``).

- Uses `resp.Diagnostics.AddAttributeError(path.Root(...).AtListIndex(i).AtName("port_range"), ...)`
  for the per-rule errors so the diagnostic carries an attribute
  location, which is strictly better than the SDKv2 form (which only
  returned a top-level `error`).

## Other notable schema decisions

- **`inbound_rule` / `outbound_rule` stayed as blocks**
  (`schema.SetNestedBlock`) rather than becoming `SetNestedAttribute`s.
  Per `references/blocks.md`, true repeating blocks (no `MaxItems: 1`)
  should stay as blocks during migration to avoid breaking
  practitioner HCL — `rule { ... } rule { ... }` would otherwise have
  to become `rules = [{...}, {...}]`. The skill's decision rule says
  "keep block when production usage exists"; for the official
  DigitalOcean firewall resource that is unambiguously the case.

- **`pending_changes` could not stay a block.**
  In SDKv2 it was a `Computed: true` `TypeList` of nested object;
  framework blocks have no `Computed` field (per `blocks.md`: "Blocks
  cannot be Required/Optional/Computed"). Migrated to a
  `Computed: true` `ListNestedAttribute`. Its values come from the
  API, never from the user, so no HCL is broken.

- **`Set: util.HashStringIgnoreCase` on the tags schema was dropped.**
  Framework `SetAttribute` handles uniqueness internally; the
  case-insensitive uniqueness semantics are the API's responsibility.
  Per `references/schema.md` "the `Set: hashFunc` trap": delete it.

- **Validators ported per the reference table:**
  - `validation.NoZeroValues` → `stringvalidator.LengthAtLeast(1)` (for strings) — the SDKv2 helper rejected empty strings only.
  - `validation.StringInSlice([...], false)` → `stringvalidator.OneOf(...)`.
  - `tag.ValidateTag` (regex) → inline `stringvalidator.RegexMatches` wrapped in `setvalidator.ValueStringsAre` so each element is checked.

- **`id` is now an explicit Computed attribute** with
  `stringplanmodifier.UseStateForUnknown()` — SDKv2 synthesized the
  `id` field implicitly; the framework requires it in the schema.

## Test file changes

- `ProviderFactories` (returning `*schema.Provider`) ⇒
  `ProtoV6ProviderFactories` (returning `tfprotov6.ProviderServer`),
  per `references/testing.md`.
- `terraform-plugin-sdk/v2/helper/resource` ⇒
  `terraform-plugin-testing/helper/resource`.
- `terraform-plugin-sdk/v2/terraform` ⇒
  `terraform-plugin-testing/terraform` (the `*terraform.State` type
  used by `CheckDestroy` / `TestCheckFunc` is now exported from
  plugin-testing).
- Inline minimal framework provider stub (`firewallTestProvider`) so
  the migrated resource can be exercised without waiting on the rest
  of the provider migration. **TODO**: delete the stub once the parent
  provider migrates and switch to the real factory.
- Added three new unit-test-mode cases that pin the migrated
  `ModifyPlan` behaviour (`_ModifyPlan_NoRules`,
  `_ModifyPlan_InboundMissingPort`,
  `_ModifyPlan_OutboundMissingPort`). These are TDD coverage for the
  three legs of the original `CustomizeDiff` and would have caught a
  refactor that lost any of them.

## Things deliberately out of scope

- The SDKv2 helper file `digitalocean/firewall/firewalls.go` (the
  `firewallSchema()`, `firewallRuleSchema()`, expand/flatten helpers)
  still imports SDKv2. It is consumed by `datasource_firewall.go`
  which is still SDKv2. Resource-only migration leaves both alone;
  the new framework resource ships its own typed conversions
  (`buildFirewallRequest`, `applyFirewallToModel`, `flattenInbound/
  OutboundRules`, etc.) so it has no SDKv2 imports.

- `provider.go` still registers the SDKv2 form
  (`ResourceDigitalOceanFirewall()`). When the provider itself
  migrates, swap that line for `firewall.NewFirewallResource` in the
  framework provider's `Resources()`.

- `acceptance.TestAccProviderFactories` is still SDKv2 (it builds
  `*schema.Provider`). The migrated test file uses its own
  `protoV6ProviderFactories` rather than reaching into the acceptance
  package; this is the right scope for a single-resource migration.
