# The 12-step single-release-cycle workflow

## Quick summary
- This file expands HashiCorp's [single-release-cycle workflow](https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-in-a-single-release-cycle) with per-step "do not skip" notes.
- Steps 1, 2, 7 are the most commonly skipped. Step 1 establishes a green baseline; step 2 surfaces latent SDKv2 errors before they become hard framework errors; step 7 is the TDD gate (tests fail first).
- Steps 3–6 are mechanical conversions, paced by the audit.
- Steps 8–11 are per-resource migration loops with verification gates between each.
- Step 12 is release; do not version-bump until the full suite is green and no SDKv2 imports remain.

## The 12 steps (verbatim from HashiCorp, with notes)

### 1. Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
**Do not skip.** Migration from a red baseline makes every later failure ambiguous — was it the migration or pre-existing? Run `go test ./... -count=1` and (if creds are available) `TF_ACC=1 go test ./...` against `main` *before* touching code. If coverage is sparse, prefer adding tests now (in SDKv2 form) over leaping ahead.

### 2. Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework.
**Do not skip.** SDKv2 demotes many data-consistency errors (state ≠ plan, computed fields not set, etc.) to warnings. The framework treats them as hard errors. The most common offenders are:
- `Read` functions that don't repopulate every field that was set during `Create`/`Update`
- `Computed` attributes that get a non-null value during plan but a different value during apply
- `Set` attributes with non-deterministic ordering
- Nested-block fields that are sometimes-nil

`TF_LOG=warn` runs of the existing test suite will surface most of these. Fix in SDKv2 form first; do not carry the bug into the framework.

#### Concrete grep recipes (run all four against the provider before step 3)

These find the four common static patterns that become hard errors under the framework. Run from the provider root.

```sh
# 1. Optional + Computed without UseStateForUnknown — every plan shows
#    "(known after apply)" and may trigger spurious replacements downstream.
#    Under SDKv2 this is silent; under the framework it surfaces as
#    "Provider produced inconsistent result after apply" if the value drifts.
grep -rn -B2 -A6 'Optional:\s*true' --include='*.go' . \
  | grep -B5 -A5 'Computed:\s*true' \
  | grep -L 'UseStateForUnknown' \
  || true   # files matching the first two but missing the third

# 2. Default on a non-Computed attribute. The framework requires Computed: true
#    for any attribute with a Default; SDKv2 didn't enforce it.
grep -rn -B5 -A1 'Default:' --include='*.go' . \
  | grep -B5 'Default:' \
  | awk '/Default:/ {found=1} /Computed:\s*true/ {found=0} END {exit !found}'

# 3. TypeList + MaxItems: 1 with no Elem (or empty Elem). The framework's
#    block-vs-attribute decision needs the nested type to make a call;
#    SDKv2 silently treated this as a no-op block.
grep -rn -B1 -A6 'Type:\s*schema\.TypeList' --include='*.go' . \
  | grep -B3 -A3 'MaxItems:\s*1' \
  | grep -L 'Elem:'

# 4. ForceNew on a Computed attribute. Framework rejects this combination
#    at schema validation time; SDKv2 accepted it silently.
grep -rn -B5 -A1 'ForceNew:\s*true' --include='*.go' . \
  | grep -B5 'ForceNew:' \
  | awk '/Computed:\s*true/ {c=1} /ForceNew:\s*true/ {if (c) print; c=0}'
```

If any of the four return matches, fix in SDKv2 form first. Each one is a one-line correction that costs minutes now and hours later (the framework error often points at a downstream symptom, not the root cause).

For runtime data-consistency, also run the existing acceptance test suite with `TF_LOG=warn TF_ACC=1 go test ./... 2>&1 | grep -i 'inconsistent\|warning'` — surfaces the dynamic offenders the static greps can't see.

### 3. Serve your provider via the framework.
This is the `main.go` swap. Decide protocol v5 vs v6 here — see `protocol-versions.md`. Default v6. The provider doesn't yet have any framework resources, but it now serves through the framework.

### 4. Update the provider definition to use the framework.
Move from `&schema.Provider{...}` to a type implementing `provider.Provider`. See `provider.md`.

### 5. Update the provider schema to use the framework.
The provider's own configuration block (e.g., region, credentials) becomes a framework schema. See `schema.md`.

### 6. Update each of the provider's resources, data sources, and other Terraform features to use the framework.
This is the bulk of the work. Drive it with the audit-generated checklist (one block per resource). For each: see `resources.md` / `data-sources.md`. Convert in dependency order — leaf resources first, then ones that reference them.

### 7. Update related tests to use the framework, and ensure that the tests fail.
**The TDD gate. Do not skip.** Update tests *first*: change `r.Test`/`r.UnitTest` factories from `ProviderFactories` to `ProtoV6ProviderFactories`, swap any SDKv2 test helpers for framework equivalents (see `testing.md`). Run them — they should fail (because the resource is still SDKv2). Only then move to step 8.

The reason: a green test on a migrated resource means very little if the test was written *after* the migration. Tests written after inherit the migrator's blind spots. Red-then-green proves the test actually exercises the change.

### 8. Migrate the resource or data source.
Now do the actual conversion. Use the references on demand. Read the file flagged "needs manual review" by the audit before starting — there's nearly always a `MaxItems: 1` block, a state upgrader, or a custom importer that needs special handling.

### 9. Verify that related tests now pass.
Run the layered verification (`verify_tests.sh`). Always pass `--migrated-files` so the negative gate catches a no-op migration. Do not move on until green.

### 10. Remove any remaining references to SDKv2 libraries.
After all resources are migrated, sweep imports:
```sh
grep -rl 'terraform-plugin-sdk/v2' . | grep -v '\.git/'
```
Common stragglers: shared helper packages, custom validators, `helper/customdiff` users.

### 11. Verify that all of your tests continue to pass.
Full suite: `go test ./...`, then if creds are available `TF_ACC=1 go test ./...`. Also confirm `go.mod` no longer requires `terraform-plugin-sdk/v2` (run `go mod tidy`).

### 12. Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.
Version-bump the major version (this is a breaking *implementation* change even though user-facing schema is unchanged — the protocol version may have changed v5 → v6). Document the protocol bump in the changelog so practitioners on Terraform <0.15 know to upgrade.

## Where pre-flight fits

The skill imposes two pre-flight steps before HashiCorp's step 1:
- **Pre-flight A — Audit**: run `audit_sdkv2.sh` against the provider, populate `assets/audit_template.md`.
- **Pre-flight B — Plan**: populate `assets/checklist_template.md` from the audit. Confirm scope with the user.

These are skill-imposed scaffolding around step 1, not parallel phases. They make the rest of the workflow tractable.
