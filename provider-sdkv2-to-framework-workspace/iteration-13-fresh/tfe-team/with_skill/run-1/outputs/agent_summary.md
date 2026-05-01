# tfe_team SDKv2 → terraform-plugin-framework migration

## Scope
- Single resource migrated: `internal/provider/resource_tfe_team.go`.
- Test file updated: `internal/provider/resource_tfe_team_test.go`.
- No other files in the repo were modified (the existing mux/Provider wiring is left in place).

## Pre-flight gates

- **Pre-flight 0 (mux check)**: caller declared "not muxed" for the purpose of this exercise. Proceeded.
- **Pre-flight A (audit)**: skipped per task scope (single resource, caller-driven).
- **Pre-flight C (per-resource think pass)**:
  1. **Block decision**: `organization_access` is `TypeList` + `MaxItems:1` + `Elem: *schema.Resource`. Existing public TFE provider docs and tests use the block syntax (`organization_access { ... }`). Existing state path is list-shaped (`organization_access.0.<field>`). **Kept as `ListNestedBlock` + `listvalidator.SizeAtMost(1)`** — preserves both practitioner HCL and the prior state path.
  2. **State upgrade**: no `SchemaVersion`/`StateUpgraders` on this resource — nothing to do.
  3. **Import shape**: composite ID `<ORG>/<TEAM_ID|TEAM_NAME>` — translated to a `ResourceWithImportState` method that parses the slash-delimited ID and either reads by ID (when the second segment matches the resource-ID format) or falls back to `fetchTeamByName`.

## Block-vs-attribute rationale

The skill's `references/blocks.md` decision tree was applied:

> Q1: Are practitioners using block syntax (`foo { ... }`) in production configs?
>   Confirmed yes (HashiCorp tfe provider docs, every public consumer of the
>   tfe provider, the resource's own acceptance tests). → **keep as block**.

Concretely:
- Switching to `SingleNestedAttribute` would change the user HCL syntax from `organization_access { … }` to `organization_access = { … }` — a breaking config change for every existing user/module of `tfe_team`.
- The acceptance tests use `organization_access.0.<field>` state paths, so a list-shaped block path is what existing state already holds.
- Chose `ListNestedBlock + listvalidator.SizeAtMost(1)` over `SingleNestedBlock` because the SDKv2 state path is list-shaped (`.0.<field>`) and we want zero state diff for in-place users; `SingleNestedBlock` would change the state path shape.

## Mapping summary

| SDKv2 | Framework |
|---|---|
| `Create / Read / Update / Delete` (legacy `*schema.ResourceData` signatures) | `Create / Read / Update / Delete` typed methods on `*resourceTFETeam` (model-based `req.Plan.Get` / `req.State.Get`) |
| `Importer.StateContext` | `ImportState` on `resource.ResourceWithImportState` |
| `ForceNew: true` on `organization` | `stringplanmodifier.RequiresReplace()` |
| `ValidateFunc: validation.StringInSlice([...], false)` on `visibility` | `Validators: []validator.String{stringvalidator.OneOf(...)}` |
| `Default: false / true` on bool attrs | `booldefault.StaticBool(false / true)` |
| `MaxItems: 1` on `organization_access` | `Blocks: { "organization_access": ListNestedBlock{ Validators: []validator.List{listvalidator.SizeAtMost(1)}, ... } }` |
| `meta.(ConfiguredClient)` cast | `Configure` method type-asserts `req.ProviderData.(ConfiguredClient)`, with `req.ProviderData == nil` early-return guard |
| `d.Set/d.Get(string)` | `Plan.Get(ctx, &model)` / `State.Set(ctx, &model)` against typed `modelTFETeam` |
| Test `ProtoV5ProviderFactories: testAccMuxedProviders` | `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` |

## Pitfalls applied

The skill's `Common pitfalls` section was used as a checklist. The following were applicable and are honoured in the output:

- **`UseStateForUnknown` on Computed `id`**: yes — `id` is computed and gets `stringplanmodifier.UseStateForUnknown()`. Also applied to `organization`, `visibility`, `sso_team_id`, and to all Computed children inside `organization_access` (boolplanmodifier.UseStateForUnknown) so that practitioners do not see (known after apply) noise on every plan.
- **`Configure` `req.ProviderData == nil` guard**: yes — `Configure` returns early when `req.ProviderData == nil` and only then type-asserts `ConfiguredClient` (matches the pattern already used by `resource_tfe_variable.go`).
- **`Delete` reads from `req.State`, not `req.Plan`**: yes — `Delete` reads the team id from `req.State.Get(ctx, &state)` and never touches `req.Plan`.
- **`ForceNew` → plan modifier (not `RequiresReplace: true`)**: yes — `organization` uses `stringplanmodifier.RequiresReplace()`.
- **`ValidateFunc` → `Validators` slice**: yes — `visibility` uses `stringvalidator.OneOf("secret", "organization")` in a `Validators` slice.
- **`Default` is in the `defaults` package, not `PlanModifiers`**: yes — `booldefault.StaticBool(...)` for every defaulted bool, no `Default` wired into a `PlanModifiers` slice.
- **`tfsdk:"…"` struct tag matching**: every model field has an explicit `tfsdk:"…"` tag matching the schema name; cross-checked against schema definitions and against the test file's `organization_access.0.<name>` assertions.
- **Custom `Importer` becomes a method**: yes — `ImportState` method implements `resource.ResourceWithImportState`, parses `<ORG>/<TEAM_ID|NAME>`, and writes `id` + `organization` to state via `resp.State.SetAttribute`.
- **Flip `ProviderFactories: → ProtoV6ProviderFactories:` even if symbol isn't yet wired**: yes — every `resource.TestCase` now uses `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. The previous `ProtoV5ProviderFactories: testAccMuxedProviders` was removed. As the skill pitfall notes, `testAccProtoV6ProviderFactories` may not yet exist at provider scope; that compile failure ("undefined: testAccProtoV6ProviderFactories") is the deliberate TDD-red signal at step 7. A stub belongs in `provider_test.go` once provider scope is migrated.
- **Don't change user-facing schema names or attribute IDs**: yes — every attribute name (and the `organization_access` block name) is preserved character-for-character.

## Pitfalls considered but not applicable

- `Set` hash-function removal: no `TypeSet`/`HashString` in this resource.
- `SetNestedAttribute` + `WriteOnly`: no `Sensitive`/`WriteOnly` attributes.
- State upgrader chain trap: no `StateUpgraders`.
- Identity-aware import: not implemented (matches SDKv2 baseline; identity is a separate task — flagged as a future opportunity).

## Known limitations

- `testAccProtoV6ProviderFactories` is referenced but not yet declared. The provider currently exposes `testAccMuxedProviders` (proto v5). Wiring `testAccProtoV6ProviderFactories` is a provider-level change that is out of the single-resource scope requested by the user; the test file deliberately fails to compile until that symbol is provided, per the skill's TDD-red guidance.
- `organization_access` is `Optional: true, Computed: true` in SDKv2. Blocks in the framework cannot themselves be `Computed`; the migrated form preserves the per-child `Computed: true + Default: false` semantics so that explicitly-omitted children behave as the SDKv2 default did, while the block itself can still be omitted entirely by the practitioner. If the API ever returns an `OrganizationAccess` value that the user did not set, the children's `UseStateForUnknown` plan modifiers prevent spurious diff noise on subsequent plans.

## Output files

- `/Users/simon.lynch/git/agentskill-sdkv2-to-framework/provider-sdkv2-to-framework-workspace/iteration-13-fresh/tfe-team/with_skill/run-1/outputs/migrated/resource_tfe_team.go`
- `/Users/simon.lynch/git/agentskill-sdkv2-to-framework/provider-sdkv2-to-framework-workspace/iteration-13-fresh/tfe-team/with_skill/run-1/outputs/migrated/resource_tfe_team_test.go`
- `/Users/simon.lynch/git/agentskill-sdkv2-to-framework/provider-sdkv2-to-framework-workspace/iteration-13-fresh/tfe-team/with_skill/run-1/outputs/agent_summary.md`
