---
name: provider-sdkv2-to-framework
description: 'Use only when the source SDK is `terraform-plugin-sdk/v2`. Does NOT apply to SDK v1 (upgrade to v2 first), `terraform-plugin-go`-only providers, intra-framework version bumps, or any `terraform-plugin-mux` / multi-release / staged / phased migration. Use this skill whenever a user wants to move any Terraform resource, data source, or whole provider from `terraform-plugin-sdk/v2` to `terraform-plugin-framework` — including partial migrations of individual resources — even if they say "rewrite", "port", "convert", or describe the work without naming the SDKs explicitly. Triggers on phrases like "move this provider to the framework", "rewrite this resource using terraform-plugin-framework", "port this resource to plugin-framework". Single-release-cycle workflow with four pre-flight gates (mux check, audit, plan, per-resource think pass).'
---

# SDKv2 → Plugin Framework migration

Follows [HashiCorp's single-release-cycle workflow](https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-in-a-single-release-cycle).

## When this skill applies

The repo imports `github.com/hashicorp/terraform-plugin-sdk/v2`. Confirm with:

```sh
grep -l 'terraform-plugin-sdk/v2' go.mod
```

### Does NOT apply (refer the user elsewhere)
- **SDK v1**: imports `github.com/hashicorp/terraform-plugin-sdk` (no `/v2`). Tell the user to upgrade to v2 first; v1→framework directly is not a supported path.
- **`terraform-plugin-go`-only providers**: a different, lower-level SDK. The framework's premise (a higher-level abstraction) doesn't apply.
- **Intra-framework version bumps**: e.g., `v1.10` → `v1.13` of the framework itself. Use `go get -u` and the framework's own changelog.
- **Multi-release / muxed / staged / phased migrations** (any `terraform-plugin-mux` workflow): out of scope. **If the user wants the migration spread across more than one provider release — regardless of vocabulary — this skill does not apply.** Refer them to HashiCorp's mux docs and exit. The audit and verification gates here assume a single-server tree; running them against a muxed provider produces false-greens on the SDKv2-routed half.
- **Framework-only feature additions**: new `function`, `ephemeral`, `list`, `action` resources; nothing to migrate from.
- **Cross-type or cross-provider state moves via `ResourceWithMoveState`**: a separate task that *can* overlap with migration but is documented separately. See `references/move-state.md` if the user is renaming or splitting a resource as part of the migration.

## Prerequisites

The audit script (`<skill-path>/scripts/audit_sdkv2.sh`) drives [semgrep](https://semgrep.dev) for AST-aware multi-line pattern matching across Go source. Install once before first use:

```sh
pip install semgrep      # any platform
brew install semgrep     # macOS
```

Without semgrep, `audit_sdkv2.sh` exits 127 with install instructions. `<skill-path>/scripts/verify_tests.sh` and `<skill-path>/scripts/lint_skill.sh` need only standard shell tools.

## Workflow

Migration progress (copy and check off):

```
- [ ] Pre-flight 0: mux check (exit if multi-release)
- [ ] Pre-flight A: audit_sdkv2.sh complete
- [ ] Pre-flight B: checklist populated, scope confirmed with user
- [ ] Pre-flight C: per-resource think pass written
- [ ] Step 1:  SDKv2 baseline tests green
- [ ] Step 2:  data-consistency review complete
- [ ] Step 3:  serve via framework (protocol v5/v6 chosen)
- [ ] Step 4:  provider definition migrated
- [ ] Step 5:  provider schema migrated
- [ ] Step 6:  resources / data sources / features migrated
- [ ] Step 7:  tests rewritten, run RED (TDD gate)
- [ ] Step 8:  each resource / data source migrated
- [ ] Step 9:  tests pass per-resource
- [ ] Step 10: SDKv2 references removed
- [ ] Step 11: full test suite green
- [ ] Step 12: release
```

<workflow_step number="0" gate="mux">

### Pre-flight 0 — Exit if mux

Before anything else, hard-exit if this is a muxed migration. Apply two checks:

1. **Keyword check** — if the user's request mentions any of `mux`, `muxed`, `muxing`, `terraform-plugin-mux`, `staged`, `phased`, `two-release`, `multi-release` → stop.
2. **Semantic check** — if the user wants the migration spread across more than one provider release (regardless of vocabulary), stop. The phrasing may not contain any of the keywords above.

If either fires, do not run Pre-flight A. Tell the user this skill targets the single-release path only and refer them to HashiCorp's `terraform-plugin-mux` docs.

</workflow_step>

<workflow_step number="A" gate="audit">

### Pre-flight A — Audit

Run the bundled audit script to inventory the provider:

```sh
bash <skill-path>/scripts/audit_sdkv2.sh <provider-repo-path>
```

The script drives [semgrep](https://semgrep.dev) with the rules at `<skill-path>/scripts/audit_sdkv2.semgrep.yml` (AST-aware Go pattern matching). Output: per-rule summary table, per-file complexity ranking, and a "needs manual review" bucket flagging judgment-rich patterns (MaxItems:1, StateUpgraders, custom Importer, Timeouts, CustomizeDiff, StateFunc, DiffSuppressFunc, nested Elem, cross-attribute constraints) — read those files directly before proposing edits. The audit also flags step-2 data-consistency patterns (Optional+Computed without UseStateForUnknown, Default on non-Computed, ForceNew+Computed, hash-placeholder secret pattern) so the most-skipped HashiCorp step is detected at audit time rather than after migration.

</workflow_step>

<workflow_step number="B" gate="plan">

### Pre-flight B — Plan

Generate a checklist from `<skill-path>/assets/checklist_template.md`, populated from the audit. Confirm scope with the user (whole provider? specific resources?) before editing anything.

<example name="inventory_artefact_shape">
Produce exactly these files, nothing extra (over-summarising blows token cost):
- `audit_report.md` — verbatim output of `audit_sdkv2.sh`, ≤25 KB. If larger, the audit is over-firing — cap with `--max-files N`, don't hand-trim.
- `migration_checklist.md` — populated `<skill-path>/assets/checklist_template.md`, one per-resource section per resource in scope (ask user if scope is "whole provider"). ≤30 KB for ~50 resources.
- `summary.md` (optional) — one paragraph: counts, highest-risk file, state upgraders if any, recommended order. ≤1 KB.
</example>

</workflow_step>

<workflow_step number="C" gate="think">

### Pre-flight C — Per-resource think pass

Before editing any resource the audit flagged for manual review, write a 3-line summary in the per-resource checklist row:

1. **Block decision**: which (if any) attributes are `MaxItems: 1 + nested Elem` → block or nested attribute? (See decision rule below.)
2. **State upgrade**: is there a `SchemaVersion > 0` with upgraders to flatten into single-step? (See `references/state-upgrade.md`.)
3. **Import shape**: is the importer passthrough, composite-ID parsing, or identity-aware? (See `references/import.md` and `references/identity.md`.)

Do not start editing the file until that summary exists. Skipping this is the most common cause of missed-pattern migrations.

</workflow_step>

### The 12 single-release-cycle steps (verbatim from HashiCorp)

1. Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
2. Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework.
3. Serve your provider via the framework. **Decision point — protocol v5 vs v6**: default v6 for single-release migrations. v6 is required for some framework-only attribute features. See `references/protocol-versions.md` for the `main.go` swap.
4. Update the provider definition to use the framework.
5. Update the provider schema to use the framework.
6. Update each of the provider's resources, data sources, and other Terraform features to use the framework.
7. **Update related tests to use the framework, and ensure that the tests fail.** TDD gate: write/update tests *first*, run them red, *then* migrate. Red-then-green proves the test actually exercises the change. Quote the failing output verbatim in the checklist. If no test exists, write a minimal one before proceeding — never skip the gate. See `references/workflow.md` for the 4-step procedure and acceptable/unacceptable failure shapes.
8. Migrate the resource or data source.
9. Verify that related tests now pass.
10. Remove any remaining references to SDKv2 libraries.
11. Verify that all of your tests continue to pass.
12. Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

For per-step "do not skip" notes, read `references/workflow.md`. Consult `references/deprecations.md` before emitting any new symbol — it lists removed/renamed APIs you might emit by mistake.

## Reference index — load on need, not on principle

**Frugal rule.** Most resources need ≤2 references; many need zero. The audit's "needs manual review" bucket is the gate — load only references that map to patterns it flagged. A simple resource (no `MaxItems:1`, no state upgrader, no custom importer, no `Timeouts`, no `CustomizeDiff`, no `Sensitive`/write-only) typically needs no references at all; the workflow above plus your training knowledge of `terraform-plugin-framework` is enough. Each reference is 1–3k tokens; pre-loading 6 of them just-in-case adds 10–15k tokens for no benefit.

Pattern-driven lookup (read these only when the audit flagged the corresponding pattern):

| Audit flag / pattern present | Read |
|---|---|
| `MaxItems: 1` block (block-vs-attribute decision) | `references/blocks.md` |
| `StateUpgraders` / `SchemaVersion > 0` | `references/state-upgrade.md` |
| Custom `Importer` (composite ID parsing) | `references/import.md` |
| Adding identity for composite-ID resource | `references/identity.md` |
| `Timeouts` field | `references/timeouts.md` |
| `CustomizeDiff` (translate to `ModifyPlan`) | `references/plan-modifiers.md` |
| `Computed` attribute that should keep prior-state value (no `(known after apply)` noise) — `UseStateForUnknown` / `UseNonNullStateForUnknown` | `references/plan-modifiers.md` |
| `StateFunc` / `DiffSuppressFunc` (translate without mutating user input) | `references/state-and-types.md` + `references/plan-modifiers.md` |
| `ConflictsWith` / `ExactlyOneOf` / `AtLeastOneOf` / `RequiredWith` | `references/validators.md` |
| `Sensitive` attribute, possible WriteOnly migration | `references/sensitive-and-writeonly.md` |
| `ResourceWithMoveState` (rename / split / cross-provider) | `references/move-state.md` |

Provider-level (vs resource-only) migrations also need: `references/provider.md` (provider type, `Configure`, client plumbing), `references/schema.md` (top-level schema), `references/protocol-versions.md` (`main.go` swap to v6), `references/compatibility.md` (framework feature/version floor), `references/testing.md` (`ProtoV6ProviderFactories`, `ImportStateVerify`, etc.).

Other references (consult only on specific need): `references/resources.md` (full CRUD shape — usually unnecessary), `references/attributes.md` (primitive/nested attribute types), `references/data-sources.md` (only for data-source migrations), `references/workflow.md` (per-step "do not skip" notes), `references/deprecations.md` (reactive — consult only when a build error names a removed SDKv2 symbol).

### Block-vs-attribute decision (`MaxItems: 1`)

For each `TypeList`/`TypeSet` of `&schema.Resource{...}` with `MaxItems: 1`, answer in this order:

1. **Are practitioners using block syntax (`foo { ... }`) in production configs?** Confirmed yes → keep as block (`ListNestedBlock` + `listvalidator.SizeAtMost(1)`, or `SingleNestedBlock`). Switching is a breaking HCL change (`foo { ... }` → `foo = { ... }`).
2. **Major-version bump or greenfield resource?** Convert to `SingleNestedAttribute`.
3. **Can't confirm either?** Keep as block; note "switch to single nested attribute on next major once usage confirmed safe" in the per-resource checklist row.

The reason the order matters: switching block→attribute is *practitioner-visible HCL*. Answer "does breaking the syntax matter here?" before "what does the framework prefer?". Full decision tree, code samples for both outputs, and `SingleNestedBlock` guidance: `references/blocks.md`.

### State upgrader rule (the chain trap)

SDKv2 chained upgraders (V0→V1→V2). Framework upgraders are **single-step** — each map entry keyed at a prior version produces the *current* schema's state directly in one call. Compose chains inline inside the V0 upgrader's body; don't call one upgrader from another. Full pattern with typed prior models, code samples, `tfsdk:` tag matching, and `ExternalProviders` test recipe: `references/state-upgrade.md`.

<example name="state_upgrader_collapse">
SDKv2 V0→V1→V2 chain (each link assumed the previous had run):

```go
// SDKv2
SchemaVersion: 2,
StateUpgraders: []schema.StateUpgrader{
    {Version: 0, Type: ..., Upgrade: upgradeV0ToV1}, // chain link 1
    {Version: 1, Type: ..., Upgrade: upgradeV1ToV2}, // chain link 2
},
```

Framework — every map entry produces the *current* schema's state directly. The V0 entry must NOT delegate to V1's transform; compose the SDKv2 V0→V1 and V1→V2 logic inline inside V0's body so the result is V2-shaped state.

```go
// Framework — each entry stands alone, no chain
func (r *thingResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {PriorSchema: priorSchemaV0(), StateUpgrader: upgradeFromV0}, // V0 → V2 directly
        1: {PriorSchema: priorSchemaV1(), StateUpgrader: upgradeFromV1}, // V1 → V2 directly
    }
}
```

Anti-pattern (silent state corruption): `func upgradeFromV0(...) { ...; upgradeFromV1(...) }`. The framework calls each entry independently with the matching `PriorSchema`; state landing at V0 was never seen by V1's upgrader, so chaining either short-reads the V1 prior schema (panic) or writes V1-shaped state into a V2 schema (drift). Compose the two transformations *inline* inside V0's body instead — read V0 fields, transform to V2 fields, write V2 model.
</example>


<verification_gates>

## Verification gates

After each migrated resource (and at the end of the migration), run:

```sh
bash <skill-path>/scripts/verify_tests.sh <provider-repo-path> --migrated-files <space-separated-list-of-files>
```

It runs six layered checks so signal is recovered even when `TF_ACC` is unset:

1. `go build ./...`
2. `go vet ./...`
3. `TestProvider` — runs the provider's own `^TestProvider$` test if present (the conventional place to call `provider.InternalValidate()`); skipped with a notice if no such test exists. **If skipped**, note in the per-resource checklist row that `InternalValidate` was not exercised; consider adding a minimal `TestProvider` test.
4. Non-`TestAcc*` unit tests.
5. **Negative gate**: none of the files passed via `--migrated-files` may still mention `github.com/hashicorp/terraform-plugin-sdk/v2` (matched as a substring, so comments and string literals also fail the gate). Closes the "all-green-on-an-unmigrated-tree" loophole.
6. (Optional, with `--with-acc`, if creds available) `TF_ACC=1 go test -count=1 ./...`.

If any gate fails, fix before moving on. Do not move past step 9 of the workflow until the resource's tests are green.

</verification_gates>

<common_pitfalls>

## Common pitfalls

- **SDKv2 silently demotes some errors to warnings; the framework does not.** A migration can surface latent errors that were always present but invisible. Fix the underlying error rather than suppressing it.
- **`Set` no longer needs a hash function.** If the SDKv2 schema used `Set: schema.HashString` or a custom hasher, drop it; framework `SetAttribute` / `SetNestedAttribute` handle uniqueness internally.
- **`d.Get("foo.0.bar")` string-path access is gone.** Replace with typed `Plan.Get`/`State.Get` calls into a typed model struct. The compiler now catches typos.
- **State upgraders are single-step, not chained.** SDKv2 chained upgraders V0→V1→V2; framework upgraders take a `PriorSchema` and produce the *target* version's state in one call per upgrader function. Read `references/state-upgrade.md` before touching this.
- **`Default` is not a plan modifier in the framework.** It's the `defaults` package (`stringdefault.StaticString("foo")`, `int64default.StaticInt64(42)`, etc.). A common mistake is wiring `Default` into `PlanModifiers` and getting *compile-time* type errors — `Default` is typed `defaults.String` (etc.), `PlanModifiers` is `[]planmodifier.String`; the compiler catches the mix.
- **`ForceNew: true` does NOT translate to `RequiresReplace: true`.** It becomes a plan modifier: `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`.
- **Don't change user-facing schema names or attribute IDs.** That's a state-breaking change — practitioners will see drift or have to re-import. Migration must be pure refactor from the user's POV.
- **`Delete` reads from `req.State`, not `req.Plan`.** `req.Plan` is null on Delete; reading from it panics. Same applies to any handler whose semantic input is the prior state, not the proposed plan.
- **`tfsdk:"foo"` struct-tag mismatch silently drops the field.** Wrong or missing tag is the #1 silent state-mapping bug — there's no compile error; the field reads as zero/null and writes nothing. Cross-check tags against the schema attribute names every time, including for prior-version models in state upgraders.
- **`Description` and `MarkdownDescription` should not both be set with diverging content.** Set one and leave the other empty (the framework falls back), or set both to the same content. Diverging text drives docs drift between the JSON-rendered docs and Markdown-rendered docs.
- **`ImportState` must handle both `req.ID` (legacy) and `req.Identity` (modern).** Branch on `req.ID == ""` to dispatch. `req.ID` is set for `terraform import myprov_thing FOO` (Terraform <1.12 or any CLI use); `req.Identity` is populated for the `import { identity = {...} }` block (Terraform 1.12+). Handling only one breaks either the CLI flow or the new HCL flow. See `references/identity.md`.
- **Always flip `ProviderFactories:` → `ProtoV6ProviderFactories:` in the test file, even if `testAccProtoV6ProviderFactories` isn't yet wired at provider scope.** The compile failure ("undefined: testAccProtoV6ProviderFactories") is the TDD-red signal you want at step 7. Leaving the SDKv2 field is the most common silent regression — the test compiles against SDKv2 plumbing forever and step 9 (tests pass green) is unreachable without later coming back to flip it. If the symbol genuinely doesn't exist yet, declare a stub `testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){...}` in the test file or in `provider_test.go` and reference it; the stub fails at runtime with a clear "framework provider not configured" error rather than passing the SDKv2 path silently.
- **Computed attributes need `UseStateForUnknown` (or `UseNonNullStateForUnknown`) unless the value really is recomputed each plan.** Without it, every plan shows `(known after apply)` for that field, which is noisy and can trigger spurious replacements in dependent resources. The most-missed case is `id` itself — every Computed `id` should have `UseStateForUnknown` unless it genuinely changes per plan. On Computed children inside a `SingleNestedAttribute`/`ListNestedAttribute`, prefer `UseNonNullStateForUnknown` (framework v1.17+) — `UseStateForUnknown` preserves prior nulls and can produce "Provider produced inconsistent result after apply" errors on those nested children.
- **In a resource's `Configure` method, `req.ProviderData == nil` on the first RPCs.** The framework calls resource `Configure` on every RPC including `ValidateResourceConfig`, which runs *before* the provider's own `Configure`. On those early calls `ProviderData` is nil; type-asserting it panics. Always guard: `if req.ProviderData == nil { return }`. Same for data sources. Validators don't get `ProviderData` at all — don't reach for `r.client` from a validator; it'll be nil regardless.
- **`SetNestedAttribute` cannot contain `WriteOnly` children.** The framework rejects this combination at provider boot (`ValidateImplementation`). If your secret lives inside a set-shaped attribute, restructure to `ListNestedAttribute` or `SingleNestedAttribute` before adding `WriteOnly`. Same trap on `SingleNestedAttribute`/`ListNestedAttribute`/`MapNestedAttribute`: if the *parent* is `WriteOnly`, every child must also be `WriteOnly` and none may be `Computed`.

</common_pitfalls>

<never_do>

## What to never do

Each rule has a *because* clause. The reasoning matters as much as the rule — when an edge case arises that the rule doesn't literally cover, judge it against the *because*.

- **Don't introduce `terraform-plugin-mux`.** *Because* muxing routes some resources to SDKv2 and some to framework simultaneously; this skill's audit and verification gates assume a single-server tree and will produce false-greens on the SDKv2-routed half.
- **Don't skip the audit.** *Because* editing without an inventory means missing state upgraders, custom validators, or import logic that would break silently — the audit catalogues exactly the patterns that need special handling and pre-empts the "I didn't know that resource had a state upgrader" class of failure.
- **Don't edit before the SDKv2 baseline is green.** *Because* starting from a red baseline makes every subsequent failure ambiguous — was it the migration or pre-existing? You lose your bisect signal immediately.
- **Don't skip step 2 (data-consistency review).** *Because* SDKv2 silently demotes inconsistencies to warnings; the framework surfaces them as hard errors after migration. Find and fix them in SDKv2 form first, or you're debugging two bugs at once.
- **Don't write tests *after* the code change.** *Because* tests written after a migration inherit the migrator's blind spots — the same person who got the migration wrong will get the test wrong in the same way. Red-then-green proves the test actually exercises the change.
- **Don't run `verify_tests.sh` without `--migrated-files`.** *Because* without the negative gate, a no-op run passes all checks (build/vet/TestProvider all stay green if you didn't touch the code), creating the illusion of a successful migration when nothing happened.

</never_do>
