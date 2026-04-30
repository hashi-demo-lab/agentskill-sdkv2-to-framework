---
name: sdkv2-to-framework
description: 'Use this skill whenever the user wants to migrate, port, or upgrade a Terraform provider from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`, including partial migrations of individual resources or data sources, even if they don''t use the word "migrate". Triggers on phrases like "move this provider to the framework", "rewrite this resource using terraform-plugin-framework", "upgrade off SDKv2", "convert this schema to framework attributes", "port this resource to plugin-framework". Drives the canonical 12-step single-release-cycle migration workflow with audit-first ordering, TDD gating at step 7, layered verification, and bundled SDKv2→framework conversion references. Does NOT cover SDK v1 (upgrade to v2 first), `terraform-plugin-go`-only providers, intra-framework version bumps, or multi-release `terraform-plugin-mux` workflows.'
---

# SDKv2 → Plugin Framework migration

This skill migrates a Terraform provider from `github.com/hashicorp/terraform-plugin-sdk/v2` to `github.com/hashicorp/terraform-plugin-framework`, following [HashiCorp's single-release-cycle workflow](https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-in-a-single-release-cycle).

The migration is mechanical-but-error-prone. Each provider has hundreds of schema fields, every resource needs CRUD method rewrites, validators and plan modifiers move to new APIs, state upgraders restructure (single-step, not chained), import handlers move to a new method, and the test suite must keep passing. The skill exists because doing this by hand drifts on naming, skips ordering, and silently glosses over the highest-risk parts (state-upgrade semantics, protocol v5 vs v6, data-consistency errors). The bundled references and scripts make those mistakes much harder.

## Prerequisites

The audit script (`scripts/audit_sdkv2.sh`) drives [semgrep](https://semgrep.dev) for AST-aware multi-line pattern matching across Go source. Install once before first use:

```sh
pip install semgrep      # any platform
brew install semgrep     # macOS
```

Without semgrep, `audit_sdkv2.sh` exits 127 with install instructions. `verify_tests.sh` and `lint_skill.sh` need only standard shell tools.

## When this skill applies

The repo imports `github.com/hashicorp/terraform-plugin-sdk/v2`. Confirm with:

```sh
grep -l 'terraform-plugin-sdk/v2' go.mod
```

**Before triggering, sanity-check the user's wording.** If the request mentions any of *mux*, *muxed*, *muxing*, *staged migration*, *two-release*, *multi-release*, or *terraform-plugin-mux* — **stop**. That's the multiplexed-migration path which is explicitly out of scope (see "Does NOT apply" below). Refer the user to HashiCorp's mux docs and exit instead of applying the single-release workflow.

### Does NOT apply (refer the user elsewhere)
- **SDK v1**: imports `github.com/hashicorp/terraform-plugin-sdk` (no `/v2`). Tell the user to upgrade to v2 first; v1→framework directly is not a supported path.
- **`terraform-plugin-go`-only providers**: a different, lower-level SDK. The framework's premise (a higher-level abstraction) doesn't apply.
- **Intra-framework version bumps**: e.g., `v1.10` → `v1.13` of the framework itself. Use `go get -u` and the framework's own changelog.
- **`terraform-plugin-mux` multi-release migrations**: out of scope for this skill; the user wants the single-release path.
- **Non-Terraform IaC**: Pulumi, CDKTF, OpenTofu, Terragrunt — different tooling.
- **Framework-only feature additions**: new `function`, `ephemeral`, `list`, `action` resources; nothing to migrate from.
- **Cross-type or cross-provider state moves via `ResourceWithMoveState`**: a separate task that *can* overlap with migration but is documented separately. See `references/move-state.md` if the user is renaming or splitting a resource as part of the migration.

## Workflow

The skill imposes two pre-flight steps (audit, plan) *around* HashiCorp's step 1 — they are scaffolding, not a competing scheme. The 12 steps below are reproduced verbatim from the HashiCorp docs so the migration plan/checklist regex-matches actual coverage, not paraphrases.

### Pre-flight A — Audit

Before HashiCorp's step 1, run the bundled audit script to inventory the provider:

```sh
bash <skill-path>/scripts/audit_sdkv2.sh <provider-repo-path>
```

The script is grep-based (POSIX) and emits a summary plus a "needs manual review" bucket for patterns it can't analyse safely (multi-line nested `Elem: &schema.Resource{...}`, `MaxItems: 1` candidates, `StateUpgraders`, custom `Importer`, `Timeouts`). Read every flagged file directly before proposing edits — the audit is rough inventory, not authoritative AST analysis.

Populate the audit using `<skill-path>/assets/audit_template.md`.

### Pre-flight B — Plan

Generate a checklist from `<skill-path>/assets/checklist_template.md`, populated from the audit. Confirm scope with the user (whole provider? specific resources?) before editing anything.

<example name="inventory_artefact_shape">
**For pre-flight outputs (audit + checklist), produce exactly these artefacts and nothing extra.** Do not append your own analysis sections, additional grep passes, or service-area breakdowns unless the user asks — the audit script and checklist template are sufficient and additional context is what blows up token cost on this task.

**Output 1 — `audit_report.md`**: the verbatim output of `audit_sdkv2.sh`. Copy it directly. No reformatting, no extra sections. Target ≤ 25 KB; if larger, the audit script is over-firing and needs a `--max-files N` cap, not a hand-summarisation pass.

**Output 2 — `migration_checklist.md`**: populate `assets/checklist_template.md`. Fill in the `{{...}}` placeholders. The "Per-resource checklist" section repeats once per resource the user wants to migrate (ask if scope is "whole provider" — don't assume). Target ≤ 30 KB for ~50 resources.

**Output 3 — `summary.md`** (optional): one paragraph ≤ 1 KB. Headline counts (resources, data sources, files), highest-risk file by complexity score, the one StateUpgrader if any, recommended migration order. Skip the verbose service-area tables.

If you find yourself producing more than these three files, or any single one above the size targets, stop and ask the user whether they want the extra detail before continuing.
</example>

### The 12 single-release-cycle steps (verbatim from HashiCorp)

1. Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
2. Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework.
3. Serve your provider via the framework. **Decision point — protocol v5 vs v6**: default v6 for single-release migrations. v6 is required for some framework-only attribute features. See `references/protocol-versions.md` for the `main.go` swap.
4. Update the provider definition to use the framework.
5. Update the provider schema to use the framework.
6. Update each of the provider's resources, data sources, and other Terraform features to use the framework.
7. **Update related tests to use the framework, and ensure that the tests fail.** This is the TDD gate. Write/update tests *first*, run them red, *then* migrate the implementation. The reason this is step 7 (and not step 8 or 9) is that a green test on a migrated resource means very little if the test was written *after* the migration — the test inherits the migrator's blind spots. Red-then-green proves the test actually exercises the change.

   <workflow_step number="7" gate="TDD">
   **Concrete procedure** (do not skip any sub-step):
   1. Edit the test file: switch `ProviderFactories` → `ProtoV6ProviderFactories`, swap any SDKv2 helpers (see `references/testing.md`).
   2. Run: `go test -run '^TestAcc<ResourceName>_basic$' ./... 2>&1 | tail -30` (or the unit-test name if no acceptance tests).
   3. **Quote the failing output verbatim** in the per-resource checklist row. The expected failure shape: a compile error citing the SDKv2 type that no longer exists, or an assertion failure about protocol mismatch. If the test passes on first run, the test does not exercise the migration — rewrite it before continuing.
   4. Only after step 3 is satisfied, proceed to step 8 (the actual code migration).
   </workflow_step>
8. Migrate the resource or data source.
9. Verify that related tests now pass.
10. Remove any remaining references to SDKv2 libraries.
11. Verify that all of your tests continue to pass.
12. Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

For per-step "do not skip" notes, read `references/workflow.md`.

## Per-element conversion (read on demand)

Each reference file opens with a 5-bullet summary so you can pull a quick lookup without reading the whole file. Open only what you need:

| You're working on… | Read |
|---|---|
| Provider schema, resource schema, data-source schema | `references/schema.md` |
| The provider type itself (`provider.Provider`, `Configure`, `Metadata`, `Resources`, `DataSources`) | `references/provider.md` |
| Resource CRUD methods (`Create`, `Read`, `Update`, `Delete`) | `references/resources.md` |
| Data sources (`datasource.DataSource`, `Read`) | `references/data-sources.md` |
| Primitive and nested attribute types (`StringAttribute`, `ListNestedAttribute`, `SingleNestedAttribute`) | `references/attributes.md` |
| When to use a block vs a nested attribute (incl. `MaxItems: 1` → single nested attribute) | `references/blocks.md` |
| Validators (`ValidateFunc` / `ValidateDiagFunc` → `Validators`, plus `terraform-plugin-framework-validators`) | `references/validators.md` |
| Plan modifiers (`ForceNew` → `RequiresReplace`, `UseStateForUnknown`) **AND defaults** (`stringdefault.StaticString` etc. — `Default` is its own package now, not a plan modifier) | `references/plan-modifiers.md` |
| Reading from / writing to state (`d.Get` → `req.Plan.Get`, `d.Set` → `resp.State.Set`, `types.String`/`Int64`/...) and `basetypes` / custom types | `references/state-and-types.md` |
| Schema versions and state upgraders (`SchemaVersion` + `StateUpgraders` → `ResourceWithUpgradeState`) | `references/state-upgrade.md` |
| Resource renames, splits, or cross-provider moves (`ResourceWithMoveState`) | `references/move-state.md` |
| Resource import (`Importer` / `ImportStatePassthroughContext` → `ResourceWithImportState.ImportState`) | `references/import.md` |
| **Resource identity** (composite-ID resources — framework's modern alternative to manual import-string parsing; `ResourceWithIdentity`, `identityschema`, `ImportStatePassthroughWithIdentity`) | `references/identity.md` |
| Timeouts (`schema.ResourceTimeout` → `terraform-plugin-framework-timeouts` package) | `references/timeouts.md` |
| `Sensitive`, write-only attributes | `references/sensitive-and-writeonly.md` |
| Protocol v5 vs v6 selection, `providerserver.NewProtocol6WithError`, `main.go` swap | `references/protocol-versions.md` |
| Acceptance tests (`ProtoV6ProviderFactories`, `TestProvider`/`InternalValidate`, `ImportStateVerify`, `PlanOnly`, `ConfigStateChecks`, `r.Test` vs `r.UnitTest`, TDD ordering) | `references/testing.md` |
| Removed/renamed APIs you might emit by mistake | `references/deprecations.md` |
| Framework version-floor for any feature (Int32, identity, WriteOnly, UseNonNullStateForUnknown, etc.) | `references/compatibility.md` |

### Worked example — the load-bearing block-vs-attribute decision

The pointer above is "see `references/blocks.md`" but the decision is judgment-heavy enough that the rule deserves to live in SKILL.md too:

<example name="MaxItems_1_decision">
**Input** (SDKv2):
```go
"persistence": {
    Type:     schema.TypeList,
    Optional: true,
    MaxItems: 1,
    Elem: &schema.Resource{
        Schema: map[string]*schema.Schema{ /* ... */ },
    },
},
```

**Decision rule** — answer in this order:

1. **Are there practitioner configs in the wild using block syntax (`persistence { ... }`) for this attribute?** If you can confirm yes (production usage, examples in the docs, public modules referencing it), keep as block — Output A. Switching is a breaking HCL change.
2. **Is this a major-version bump that already documents breaking changes, OR a greenfield resource with no users yet?** Convert to single nested attribute — Output B. The attribute syntax is the modern framework idiom and is what greenfield resources should use.
3. **Can you not confirm either?** (You can't see the resource's usage; the README doesn't show examples.) Keep as block (Output A) and write a one-line caveat in the per-resource checklist row: "kept as block; switch to single nested attribute on next major version once usage is confirmed safe."

The reason the order matters: switching block→attribute is a *practitioner-visible* HCL change — `persistence { ... }` becomes `persistence = { ... }`. That's fine for greenfield, breaking for mature. So the question to answer first is "does breaking the syntax matter here?", not "what does the framework prefer?".

**Output A — backward-compat (most common during migration)**:
```go
Blocks: map[string]schema.Block{
    "persistence": schema.ListNestedBlock{
        Validators: []validator.List{listvalidator.SizeAtMost(1)},
        NestedObject: schema.NestedBlockObject{
            Attributes: map[string]schema.Attribute{ /* ... */ },
        },
    },
},
```

**Output B — greenfield / major bump**:
```go
Attributes: map[string]schema.Attribute{
    "persistence": schema.SingleNestedAttribute{
        Optional: true,
        Attributes: map[string]schema.Attribute{ /* ... */ },
    },
},
```

Read `references/blocks.md` for the full decision tree including `MinItems > 0` true repeating blocks.
</example>

### Think before editing

Before editing any resource the audit flagged for manual review, write a 3-line summary in the per-resource checklist row:

1. **Block decision**: which (if any) attributes are `MaxItems: 1 + nested Elem` → block or nested attribute? (Apply the rule above.)
2. **State upgrade**: is there a `SchemaVersion > 0` with upgraders to flatten into single-step? (See `references/state-upgrade.md`.)
3. **Import shape**: is the importer passthrough or composite-ID parsing? (See `references/import.md`.)

Do not start editing the file until that summary exists. Skipping this step is the most common cause of missed-pattern migrations.

## Verification gates

After each migrated resource (and at the end of the migration), run:

```sh
bash <skill-path>/scripts/verify_tests.sh <provider-repo-path> --migrated-files <space-separated-list-of-files>
```

It runs layered checks so signal is recovered even when `TF_ACC` is unset:

1. `go build ./...`
2. `go vet ./...`
3. `TestProvider` — calls `provider.InternalValidate()`, catches a huge class of schema errors
4. Non-`TestAcc*` unit tests
5. (Optional) protocol-v6 smoke that boots the provider via `providerserver.NewProtocol6WithError` and asserts it serves
6. (Optional, if creds available) `TF_ACC=1` for the resource's `TestAcc*` tests
7. **Negative gate**: none of the files passed via `--migrated-files` may still import `github.com/hashicorp/terraform-plugin-sdk/v2`. This closes the "all-green-on-an-unmigrated-tree" loophole.

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

</common_pitfalls>

<never_do>

## What to never do

Each rule has a *because* clause. The reasoning matters as much as the rule — when an edge case arises that the rule doesn't literally cover, judge it against the *because*.

- **Don't introduce `terraform-plugin-mux`.** *Because* muxing changes the migration shape entirely (incremental over many releases vs the single-release scope this skill targets); applying single-release patterns to a muxed provider creates an inconsistent half-state.
- **Don't skip the audit.** *Because* editing without an inventory means missing state upgraders, custom validators, or import logic that would break silently — the audit catalogues exactly the patterns that need special handling and pre-empts the "I didn't know that resource had a state upgrader" class of failure.
- **Don't edit before the SDKv2 baseline is green.** *Because* starting from a red baseline makes every subsequent failure ambiguous — was it the migration or pre-existing? You lose your bisect signal immediately.
- **Don't skip step 2 (data-consistency review).** *Because* SDKv2 silently demotes inconsistencies to warnings; the framework surfaces them as hard errors after migration. Find and fix them in SDKv2 form first, or you're debugging two bugs at once.
- **Don't write tests *after* the code change.** *Because* tests written after a migration inherit the migrator's blind spots — the same person who got the migration wrong will get the test wrong in the same way. Red-then-green proves the test actually exercises the change.
- **Don't run `verify_tests.sh` without `--migrated-files`.** *Because* without the negative gate, a no-op run passes all checks (build/vet/TestProvider all stay green if you didn't touch the code), creating the illusion of a successful migration when nothing happened.

</never_do>
