# SDKv2 → Framework Migration Audit

**Provider:** terraform-provider-tfe    **Audited:** 2026-05-01    **SDKv2 version:** v2.37.0

## Summary

- Production Go files audited: 126
- Test Go files audited: 105

- **Resource/data-source constructors detected: 56** (each is a `func ...() *schema.Resource` — direct migration count)

### Provider-level migration cost

These patterns indicate work in `provider.go` / Configure path, separate from per-resource cost. The framework provider type and Configure method must be set up before any resource migration can be tested.

- ResourcesMap references: **1**
- DataSourcesMap references: **1**
- Provider ConfigureContextFunc: **1**
- *schema.Provider type references: **2**

### Schema-level fields
- ForceNew: true: **75**
- ValidateFunc / ValidateDiagFunc: **28**
- DiffSuppressFunc: **2**
- CustomizeDiff: **15**
- Sensitive: true: **31**
- Deprecated attribute: **11**
- ConflictsWith / ExactlyOneOf / AtLeastOneOf / RequiredWith: **39**

### Resource-level fields
- Importer: **29**
- schema.ImportStatePassthroughContext (trivial importer): **9**
- StateUpgraders: **2**
- SchemaVersion: **4**

### Block / nested-attribute decisions
- MaxItems:1 + nested Elem (block decision): **6**
- Nested Elem &Resource (any block): **19**
- MinItems > 0 (true repeating block): **6**
- TypeList/Set/Map of primitive (Elem &Schema{Type:}): **41**

### Step 2 — data-consistency (fix in SDKv2 first)
- Optional+Computed without UseStateForUnknown (plan-noise / spurious replacement): **71**
- Default: without Computed:true (framework rejects at boot): **67**
- TypeList + MaxItems:1 without Elem (malformed schema): **2**

### Helper packages (need replacement)
- retry.RetryContext: **3**
- helper/validation.* calls (replace with framework-validators): **30**

### CRUD-body shape
- CreateContext/ReadContext/UpdateContext/DeleteContext: **9**
- *schema.ResourceData function-param references: **177**
- Resource constructor (count = resources to migrate): **56**
- *schema.ResourceDiff function (port to ModifyPlan body): **10**
- d.Id() / d.SetId() calls: **458**
- d.Get / d.GetOk / d.GetOkExists calls: **390**
- d.Set calls: **392**
- d.HasChange / d.GetChange / d.IsNewResource / d.Partial: **104**
- Inline *schema.Set cast from d.Get: **8**
- diag.FromErr / diag.Errorf: **21**

## Per-file findings (top 20 by complexity, production code)

| File | ForceNew | Validators | StateUpgr | MaxIt:1 | Imptr | CustDiff | StateFunc | retry.SCC | custdiff | CRUDctx | d.Get | d.Set |
|------|---------:|-----------:|----------:|--------:|------:|---------:|----------:|----------:|---------:|--------:|------:|------:|
| internal/provider/resource_tfe_workspace.go | 1 | 6 | 1 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 64 | 36 |
| internal/provider/resource_tfe_team.go | 1 | 2 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 10 | 6 |
| internal/provider/resource_tfe_team_project_access.go | 2 | 16 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 4 | 29 | 7 |
| internal/provider/resource_tfe_registry_module.go | 10 | 2 | 0 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 28 | 22 |
| internal/provider/resource_tfe_team_access.go | 2 | 12 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 18 | 6 |
| internal/provider/resource_tfe_policy_set.go | 4 | 4 | 0 | 1 | 1 | 1 | 0 | 0 | 0 | 0 | 23 | 12 |
| internal/provider/resource_tfe_policy.go | 3 | 4 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 17 | 6 |
| internal/provider/resource_tfe_oauth_client.go | 10 | 2 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 13 | 9 |
| internal/provider/resource_tfe_variable_set.go | 2 | 0 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 13 | 7 |
| internal/provider/resource_tfe_organization.go | 0 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 12 | 13 |
| internal/provider/resource_tfe_opa_version.go | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 16 | 8 |
| internal/provider/resource_tfe_sentinel_version.go | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 16 | 8 |
| internal/provider/resource_tfe_terraform_version.go | 0 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 16 | 8 |
| internal/provider/resource_tfe_workspace_migrate.go | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| internal/provider/resource_tfe_workspace_run.go | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 |
| internal/provider/resource_tfe_no_code_module.go | 2 | 0 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 4 | 8 | 5 |
| internal/provider/resource_tfe_sentinel_policy.go | 2 | 2 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 8 | 5 |
| internal/provider/resource_tfe_agent_pool.go | 1 | 0 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 4 | 3 |
| internal/provider/resource_tfe_organization_token.go | 3 | 0 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 2 | 2 |
| internal/provider/resource_tfe_organization_membership.go | 2 | 0 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 1 | 4 |

### Score breakdown for top 5 files

- `internal/provider/resource_tfe_workspace.go` (score 117): default-on-non-computed×11=33, resource-data-get×64=16, resource-diff-signature×4=16, resource-data-set×36=9, optional-computed-without-usestateforunknown×9=9, schema-resource-data×7=7
- `internal/provider/resource_tfe_team.go` (score 72): default-on-non-computed×16=48, schema-resource-data×5=5, max-items-1-nested-block×1=4, optional-computed-without-usestateforunknown×3=3, resource-data-get×10=2.5, validate-func×1=2
- `internal/provider/resource_tfe_team_project_access.go` (score 71): validate-func×8=16, optional-computed-without-usestateforunknown×14=14, helper-validation×8=8, resource-data-get×29=7.25, nested-elem-resource×2=4, customize-diff×1=4
- `internal/provider/resource_tfe_registry_module.go` (score 58.5): force-new×10=10, optional-computed-without-usestateforunknown×8=8, schema-resource-data×7=7, resource-data-get×28=7, resource-data-set×22=5.5, max-items-1-nested-block×1=4
- `internal/provider/resource_tfe_team_access.go` (score 54): validate-func×6=12, resource-diff-signature×2=8, helper-validation×6=6, state-upgraders×1=5, schema-resource-data×5=5, resource-data-get×18=4.5

### Cross-rule correlations (files combining judgment-rich patterns)

Files hitting multiple high-judgment patterns at once. Read both/all references *before* editing.

- `internal/provider/resource_tfe_team_access.go`:
  - state-upgrade + composite-ID importer
- `internal/provider/resource_tfe_workspace.go`:
  - state-upgrade + composite-ID importer

## Needs manual review

Read these files directly. Even with semgrep's AST-aware matching, the *decision* (block vs nested attribute, single-step state upgrade, composite-ID importer parsing, customdiff structure) requires human/LLM judgment.

- internal/provider/data_source_github_app_installation.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/data_source_oauth_client.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/data_source_organization_members.go — nested Elem &Resource (block-vs-nested decision)
- internal/provider/data_source_organization_membership.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/data_source_organization_tags.go — nested Elem &Resource (block-vs-nested decision)
- internal/provider/data_source_organizations.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/data_source_policy_set.go — nested Elem &Resource (block-vs-nested decision)
- internal/provider/data_source_team_access.go — nested Elem &Resource (block-vs-nested decision)
- internal/provider/data_source_team_project_access.go — nested Elem &Resource (block-vs-nested decision)
- internal/provider/data_source_variable_set.go — Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/data_source_workspace.go — nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/data_source_workspace_ids.go — MaxItems:1 (block-vs-nested-attribute decision); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model)
- internal/provider/provider.go — Provider ConfigureContextFunc (provider-level migration)
- internal/provider/provider_custom_diffs.go — *schema.ResourceDiff function (port to ModifyPlan)
- internal/provider/resource_tfe_admin_organization_settings.go — CustomizeDiff (becomes ModifyPlan); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_agent_pool.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_agent_pool_allowed_workspaces.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_no_code_module.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_oauth_client.go — CustomizeDiff (becomes ModifyPlan); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_opa_version.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_organization.go — custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_organization_membership.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_organization_module_sharing.go — CustomizeDiff (becomes ModifyPlan); DiffSuppressFunc (analyse intent); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_organization_token.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_policy.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_policy_set.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_project_oauth_client_.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_project_policy_set.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_project_variable_set.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_registry_module.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.ResourceDiff function (port to ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_run_trigger.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_sentinel_policy.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_sentinel_version.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_team.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_team_access.go — StateUpgraders/SchemaVersion (single-step semantics); custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.ResourceDiff function (port to ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_team_member.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_team_members.go — custom Importer (composite ID parsing?); *schema.Set cast (TypeSet expansion → typed model)
- internal/provider/resource_tfe_team_organization_member.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_team_organization_members.go — custom Importer (composite ID parsing?); *schema.Set cast (TypeSet expansion → typed model)
- internal/provider/resource_tfe_team_project_access.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); nested Elem &Resource (block-vs-nested decision); *schema.ResourceDiff function (port to ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- internal/provider/resource_tfe_terraform_version.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_variable_set.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.ResourceDiff function (port to ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_workspace.go — MaxItems:1 (block-vs-nested-attribute decision); StateUpgraders/SchemaVersion (single-step semantics); custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.ResourceDiff function (port to ModifyPlan); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_workspace_migrate.go — MaxItems:1 (block-vs-nested-attribute decision); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_workspace_policy_set.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_workspace_policy_set_exclusion.go — custom Importer (composite ID parsing?)
- internal/provider/resource_tfe_workspace_run.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/resource_tfe_workspace_variable_set.go — custom Importer (composite ID parsing?)

## Test-file findings

Scanned 105 test files. Test migration is a **provider-level prerequisite** — per-resource test rewrites (workflow step 7) cannot succeed until shared test plumbing has a framework path. Plan this work *before* touching per-resource tests.

- resource.Test/UnitTest/ParallelTest (must use terraform-plugin-testing): **447**
- PreCheck: (test pre-check, often references *schema.Provider plumbing): **441**
- diag.FromErr / diag.Errorf: **2**
- d.Id() / d.SetId() calls: **2**
- d.Set calls: **7**

### Shared test infrastructure (migrate first — per-resource tests depend on these)

Files matching test-infra path conventions (acceptance/, testutil/, provider_test.go, etc.). Every migrated test file references something here; flipping ProviderFactories per resource is wasted effort if the factory isn't framework-aware yet.

- `internal/provider/provider_test.go` [provider_test.go] — diag-helpers=2

### Top 10 per-resource test files by SDKv2-pattern count

- `internal/provider/resource_tfe_workspace_test.go`: 127 patterns
- `internal/provider/resource_tfe_registry_module_test.go`: 62 patterns
- `internal/provider/resource_tfe_team_notification_configuration_test.go`: 36 patterns
- `internal/provider/resource_tfe_notification_configuration_test.go`: 34 patterns
- `internal/provider/resource_tfe_policy_set_test.go`: 34 patterns
- `internal/provider/data_source_workspace_ids_test.go`: 25 patterns
- `internal/provider/resource_tfe_team_project_access_test.go`: 22 patterns
- `internal/provider/resource_tfe_team_test.go`: 22 patterns
- `internal/provider/resource_tfe_team_token_test.go`: 22 patterns
- `internal/provider/data_source_oauth_client_test.go`: 20 patterns

## Next steps

1. Read every file listed under 'Needs manual review' before proposing edits.
2. Populate `assets/checklist_template.md` from this audit (one entry per resource).
3. Confirm scope with the user before starting workflow step 1.
4. For test files: factor in `ProviderFactories: → ProtoV6ProviderFactories` and `helper/resource → terraform-plugin-testing/helper/resource` swaps when sizing step 7 (TDD gate).
