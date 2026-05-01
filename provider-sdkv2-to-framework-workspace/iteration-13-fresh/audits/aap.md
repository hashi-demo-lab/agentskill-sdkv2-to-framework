# SDKv2 → Framework Migration Audit

**Provider:** terraform-provider-aap    **Audited:** 2026-05-01    **SDKv2 version:** v2.38.1

## Summary

- Production Go files audited: 32
- Test Go files audited: 25

### Schema-level fields
- Sensitive: true: **2**

### Helper packages (need replacement)
- retry.StateChangeConf (no framework equivalent): **1**
- retry.RetryContext: **6**

### CRUD-body shape
- d.Get / d.GetOk / d.GetOkExists calls: **11**
- d.Set calls: **23**

## Per-file findings (top 20 by complexity, production code)

| File | ForceNew | Validators | StateUpgr | MaxIt:1 | Imptr | CustDiff | StateFunc | retry.SCC | custdiff | CRUDctx | d.Get | d.Set |
|------|---------:|-----------:|----------:|--------:|------:|---------:|----------:|----------:|---------:|--------:|------:|------:|
| internal/provider/retry.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 |
| internal/provider/client.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 3 | 2 |
| internal/provider/host_resource.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 2 | 3 |
| internal/provider/base_datasource.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 2 | 2 |
| internal/provider/group_resource.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 3 |
| internal/provider/inventory_resource.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 3 |
| internal/provider/job_resource.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 3 |
| internal/provider/workflow_job_resource.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 3 |
| internal/provider/organization_data_source.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 1 |
| internal/provider/base_eda_datasource.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 |
| internal/provider/client_authenticators.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 |
| internal/provider/eda_eventstream_post_action.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 |
| internal/provider/job_launch_action.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| internal/provider/provider.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| internal/provider/workflow_job_launch_action.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |

### Score breakdown for top 5 files

- `internal/provider/retry.go` (score 3): retry-state-change-conf×1=3
- `internal/provider/client.go` (score 1.25): resource-data-get×3=0.75, resource-data-set×2=0.5
- `internal/provider/host_resource.go` (score 1.25): resource-data-set×3=0.75, resource-data-get×2=0.5
- `internal/provider/base_datasource.go` (score 1): resource-data-get×2=0.5, resource-data-set×2=0.5
- `internal/provider/group_resource.go` (score 1): resource-data-set×3=0.75, resource-data-get×1=0.25

## Needs manual review

Read these files directly. Even with semgrep's AST-aware matching, the *decision* (block vs nested attribute, single-step state upgrade, composite-ID importer parsing, customdiff structure) requires human/LLM judgment.

- internal/provider/retry.go — retry.StateChangeConf (replace with inline ticker loop)

## Test-file findings

Scanned 25 test files. Test migration is a **provider-level prerequisite** — per-resource test rewrites (workflow step 7) cannot succeed until shared test plumbing has a framework path. Plan this work *before* touching per-resource tests.

- resource.Test/UnitTest/ParallelTest (must use terraform-plugin-testing): **39**
- PreCheck: (test pre-check, often references *schema.Provider plumbing): **39**
- helper/acctest test utilities: **12**
- d.Get / d.GetOk / d.GetOkExists calls: **12**

### Shared test infrastructure (migrate first — per-resource tests depend on these)

Files matching test-infra path conventions (acceptance/, testutil/, provider_test.go, etc.). Every migrated test file references something here; flipping ProviderFactories per resource is wasted effort if the factory isn't framework-aware yet.

- `internal/provider/provider_test.go` [provider_test.go] — resource-data-get=1

### Top 10 per-resource test files by SDKv2-pattern count

- `internal/provider/job_resource_test.go`: 24 patterns
- `internal/provider/workflow_job_resource_test.go`: 16 patterns
- `internal/provider/job_launch_action_test.go`: 15 patterns
- `internal/provider/host_resource_test.go`: 8 patterns
- `internal/provider/organization_data_source_test.go`: 8 patterns
- `internal/provider/eda_eventstream_post_action_test.go`: 6 patterns
- `internal/provider/workflow_job_launch_action_test.go`: 6 patterns
- `internal/provider/group_resource_test.go`: 4 patterns
- `internal/provider/retry_test.go`: 4 patterns
- `internal/provider/inventory_data_source_test.go`: 3 patterns

## Next steps

1. Read every file listed under 'Needs manual review' before proposing edits.
2. Populate `assets/checklist_template.md` from this audit (one entry per resource).
3. Confirm scope with the user before starting workflow step 1.
4. For test files: factor in `ProviderFactories: → ProtoV6ProviderFactories` and `helper/resource → terraform-plugin-testing/helper/resource` swaps when sizing step 7 (TDD gate).
