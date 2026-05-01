---
name: provider-sdkv2-to-framework
description: 'Use only when the source SDK is `terraform-plugin-sdk/v2`. Does NOT apply to SDK v1 (upgrade to v2 first), `terraform-plugin-go`-only providers, intra-framework version bumps, or any `terraform-plugin-mux` / multi-release / staged / phased migration. Use this skill whenever a user wants to move any Terraform resource, data source, or whole provider from `terraform-plugin-sdk/v2` to `terraform-plugin-framework` — including partial migrations of individual resources — even if they say "rewrite", "port", "convert", or describe the work without naming the SDKs explicitly. Triggers on phrases like "move this provider to the framework", "rewrite this resource using terraform-plugin-framework", "port this resource to plugin-framework". Single-release-cycle workflow with two pre-flight steps (audit + plan).'
---

# SDKv2 → Plugin Framework migration

Migrates a Terraform provider from `github.com/hashicorp/terraform-plugin-sdk/v2` to `github.com/hashicorp/terraform-plugin-framework`, following [HashiCorp's single-release-cycle workflow](https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-in-a-single-release-cycle).

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

The script drives [semgrep](https://semgrep.dev) with the rules at `<skill-path>/scripts/audit_sdkv2.semgrep.yml` (AST-aware Go pattern matching) and emits a summary table per rule + a per-file complexity ranking + a "needs manual review" bucket. The bucket flags patterns where the decision is judgment-rich and a human/LLM should read the file directly before proposing edits — not patterns the script couldn't parse. Current judgment-required signals: `MaxItems: 1` block-vs-attribute, `StateUpgraders` (single-step composition), custom `Importer` (composite-ID parsing), `Timeouts`, `CustomizeDiff`, `StateFunc`, `DiffSuppressFunc`, nested `Elem &Resource`, cross-attribute constraints (`ConflictsWith`/etc.), legacy `MigrateState`.

The audit script emits a fully populated report ready to commit; `<skill-path>/assets/audit_template.md` documents the expected shape if you need to inspect or hand-edit it.

</workflow_step>

<workflow_step number="B" gate="plan">

### Pre-flight B — Plan

Generate a checklist from `<skill-path>/assets/checklist_template.md`, populated from the audit. Confirm scope with the user (whole provider? specific resources?) before editing anything.

<example name="inventory_artefact_shape">
**Pre-flight outputs — produce exactly these and nothing extra.**

| Output | Source | Size budget |
|---|---|---|
| `audit_report.md` | verbatim output of `audit_sdkv2.sh` (no reformatting) | ≤ 25 KB |
| `migration_checklist.md` | populated `<skill-path>/assets/checklist_template.md`, one per-resource section per resource in scope (ask if scope is "whole provider" — don't assume) | ≤ 30 KB for ~50 resources |
| `summary.md` (optional) | one paragraph: counts, highest-risk file, the one StateUpgrader if any, recommended order | ≤ 1 KB |

If a file blows its budget, the audit is over-firing — cap with `--max-files N`, do not hand-summarise. Do not append your own analysis sections, additional grep passes, or service-area breakdowns. If you find yourself producing more than these three files, stop and ask.
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
7. **Update related tests to use the framework, and ensure that the tests fail.** This is the TDD gate. Write/update tests *first*, run them red, *then* migrate the implementation. Red-then-green proves the test actually exercises the change — a test written *after* migration inherits the migrator's blind spots.

   <workflow_step number="7" gate="TDD">
   **Concrete procedure** (do not skip any sub-step):
   1. Edit the test file: switch `ProviderFactories` → `ProtoV6ProviderFactories`, swap any SDKv2 helpers (see `references/testing.md`). **If no test exists for the resource, write a minimal one before proceeding** — never skip the gate.
   2. Run: `go test -run '^TestAcc<ResourceName>_basic$' ./... 2>&1 | tail -30` (or the unit-test name if no acceptance tests).
   3. **Quote the failing output verbatim** in the per-resource checklist row. Acceptable failure shapes:
      - Compile error citing an SDKv2 type that no longer exists (e.g. `undefined: schema.Provider`).
      - `protocol version mismatch` from the test framework.
      - `schema for resource X not found` (runtime — the test references the resource but the provider hasn't registered it under the framework yet).
      - Schema-shape assertion mismatch (e.g. `expected attribute "foo" to be Computed, got Required`).

      Unacceptable: the test passed unchanged. If it does, the test does not exercise the migration — rewrite it.
   4. Only after step 3 is satisfied, proceed to step 8.
   </workflow_step>
8. Migrate the resource or data source.
9. Verify that related tests now pass.
10. Remove any remaining references to SDKv2 libraries.
11. Verify that all of your tests continue to pass.
12. Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

For per-step "do not skip" notes, read `references/workflow.md`. Consult `references/deprecations.md` before emitting any new symbol — it lists removed/renamed APIs you might emit by mistake.

## Reference index — open on demand

Each reference file opens with a 5-bullet summary so you can pull a quick lookup without reading the whole file. Open only what you need:

| You're working on… | Read |
|---|---|
| Provider schema, resource schema, data-source schema | `references/schema.md` |
| The provider type itself (`provider.Provider`, `Metadata`, `Resources`, `DataSources`) | `references/provider.md` |
| Provider configuration & client plumbing (`Configure`, `req.ProviderData`, type-asserting `*Client`) | `references/provider.md` |
| Resource CRUD methods (`Create`, `Read`, `Update`, `Delete`) | `references/resources.md` |
| Data sources (`datasource.DataSource`, `Read`) | `references/data-sources.md` |
| Primitive and nested attribute types (`StringAttribute`, `ListNestedAttribute`, `SingleNestedAttribute`) | `references/attributes.md` |
| When to use a block vs a nested attribute (incl. `MaxItems: 1` → single nested attribute) | `references/blocks.md` |
| Validators (`ValidateFunc` / `ValidateDiagFunc` → `Validators`, plus `terraform-plugin-framework-validators`) | `references/validators.md` |
| Plan modifiers (`ForceNew` → `RequiresReplace`; **`UseStateForUnknown`** for computed-after-apply attributes) **AND defaults** (`stringdefault.StaticString` etc. — `Default` is its own package now, not a plan modifier) | `references/plan-modifiers.md` |
| Reading from / writing to state (`d.Get` → `req.Plan.Get`, `d.Set` → `resp.State.Set`, `types.String`/`Int64`/...) and `basetypes` / custom types | `references/state-and-types.md` |
| Schema versions and state upgraders (`SchemaVersion` + `StateUpgraders` → `ResourceWithUpgradeState`) | `references/state-upgrade.md` |
| Resource renames, splits, or cross-provider moves (`ResourceWithMoveState`) | `references/move-state.md` |
| Resource import (`Importer` / `ImportStatePassthroughContext` → `ResourceWithImportState.ImportState`) | `references/import.md` |
| Resource identity (composite-ID resources — framework's modern alternative to manual import-string parsing; `ResourceWithIdentity`, `identityschema`, `ImportStatePassthroughWithIdentity`) | `references/identity.md` |
| Timeouts (`schema.ResourceTimeout` → `terraform-plugin-framework-timeouts` package) | `references/timeouts.md` |
| `Sensitive`, write-only attributes | `references/sensitive-and-writeonly.md` |
| Protocol v5 vs v6 selection, `providerserver.NewProtocol6WithError`, `main.go` swap | `references/protocol-versions.md` |
| Acceptance tests (`ProtoV6ProviderFactories`, `TestProvider`/`InternalValidate`, `ImportStateVerify`, `PlanOnly`, `ConfigStateChecks`, `r.Test` vs `r.UnitTest`, TDD ordering) | `references/testing.md` |
| Framework version-floor for any feature (Int32, identity, WriteOnly, UseNonNullStateForUnknown, etc.) | `references/compatibility.md` |

### Block-vs-attribute decision (`MaxItems: 1`)

For each `TypeList`/`TypeSet` of `&schema.Resource{...}` with `MaxItems: 1`, answer in this order:

1. **Are practitioners using block syntax (`foo { ... }`) in production configs?** Confirmed yes → keep as block (`ListNestedBlock` + `listvalidator.SizeAtMost(1)`, or `SingleNestedBlock`). Switching is a breaking HCL change (`foo { ... }` → `foo = { ... }`).
2. **Major-version bump or greenfield resource?** Convert to `SingleNestedAttribute`.
3. **Can't confirm either?** Keep as block; note "switch to single nested attribute on next major once usage confirmed safe" in the per-resource checklist row.

The reason the order matters: switching block→attribute is *practitioner-visible HCL*. Answer "does breaking the syntax matter here?" before "what does the framework prefer?". Full decision tree, code samples for both outputs, and `SingleNestedBlock` guidance: `references/blocks.md`.

<example name="state_upgrader_collapse">

### Worked example — collapsing chained state upgraders

SDKv2 chained upgraders V0→V1→V2. Framework upgraders are **single-step**: each map entry takes a `PriorSchema` and produces the *current* schema's state directly. There is no chain.

**SDKv2 (chained)**:
```go
SchemaVersion: 2,
StateUpgraders: []schema.StateUpgrader{
    {Version: 0, Type: resourceThingV0().CoreConfigSchema().ImpliedType(), Upgrade: upgradeV0ToV1},
    {Version: 1, Type: resourceThingV1().CoreConfigSchema().ImpliedType(), Upgrade: upgradeV1ToV2},
}
```

**Framework (each entry produces current state)**:
```go
func (r *thingResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {PriorSchema: priorSchemaV0(), StateUpgrader: upgradeFromV0}, // composes V0→V1→current
        1: {PriorSchema: priorSchemaV1(), StateUpgrader: upgradeFromV1}, // V1→current only
    }
}
```

The V0 entry must produce *current* state, not V1 state — compose the V0→V1 and V1→current transformations inside `upgradeFromV0`'s body. Don't call one upgrader from another. Full pattern (typed prior models, `tfsdk:` tag matching the prior schema, closing over `r.client` when API calls are needed, acceptance-test recipe with `ExternalProviders`): `references/state-upgrade.md`.

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
