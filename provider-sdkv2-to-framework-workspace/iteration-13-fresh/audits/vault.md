# SDKv2 → Framework Migration Audit

**Provider:** terraform-provider-vault    **Audited:** 2026-05-01    **SDKv2 version:** v2.36.1

## Summary

- Production Go files audited: 279
- Test Go files audited: 236

- **Resource/data-source constructors detected: 194** (each is a `func ...() *schema.Resource` — direct migration count)

### Provider-level migration cost

These patterns indicate work in `provider.go` / Configure path, separate from per-resource cost. The framework provider type and Configure method must be set up before any resource migration can be tested.

- ResourcesMap references: **1**
- DataSourcesMap references: **1**
- *schema.Provider type references: **4**

### Schema-level fields
- ForceNew: true: **422**
- ValidateFunc / ValidateDiagFunc: **99**
- DiffSuppressFunc: **15**
- CustomizeDiff: **28**
- StateFunc: **63**
- Sensitive: true: **122**
- Deprecated attribute: **1**
- Default: ... (defaults package, NOT PlanModifiers): **5**
- ConflictsWith / ExactlyOneOf / AtLeastOneOf / RequiredWith: **65**

### Resource-level fields
- Importer: **122**
- schema.ImportStatePassthroughContext (trivial importer): **66**
- StateUpgraders: **6**
- SchemaVersion: **13**
- MigrateState (legacy SDKv2 v1.x): **2**
- Exists callback (gone in framework): **28**

### Block / nested-attribute decisions
- MaxItems:1 + nested Elem (block decision): **3**
- Nested Elem &Resource (any block): **36**
- TypeList/Set/Map of primitive (Elem &Schema{Type:}): **279**

### Step 2 — data-consistency (fix in SDKv2 first)
- Optional+Computed without UseStateForUnknown (plan-noise / spurious replacement): **234**
- Default: without Computed:true (framework rejects at boot): **258**
- ForceNew on pure-Computed attribute (framework rejects at boot): **1**
- TypeList + MaxItems:1 without Elem (malformed schema): **3**
- Sensitive + StateFunc (hash-placeholder — candidate WriteOnly migration): **1**

### Helper packages (need replacement)
- helper/customdiff combinators: **6**
- helper/validation.* calls (replace with framework-validators): **28**

### CRUD-body shape
- CreateContext/ReadContext/UpdateContext/DeleteContext: **338**
- *schema.ResourceData function-param references: **702**
- Resource constructor (count = resources to migrate): **194**
- *schema.ResourceDiff function (port to ModifyPlan body): **7**
- d.Id() / d.SetId() calls: **810**
- d.Get / d.GetOk / d.GetOkExists calls: **1509**
- d.Set calls: **808**
- d.HasChange / d.GetChange / d.IsNewResource / d.Partial: **187**
- Inline *schema.Set cast from d.Get: **34**
- diag.FromErr / diag.Errorf: **1009**

## Per-file findings (top 20 by complexity, production code)

| File | ForceNew | Validators | StateUpgr | MaxIt:1 | Imptr | CustDiff | StateFunc | retry.SCC | custdiff | CRUDctx | d.Get | d.Set |
|------|---------:|-----------:|----------:|--------:|------:|---------:|----------:|----------:|---------:|--------:|------:|------:|
| vault/resource_database_secret_backend_connection.go | 1 | 8 | 0 | 0 | 1 | 0 | 1 | 0 | 0 | 4 | 111 | 3 |
| vault/resource_pki_secret_backend_role.go | 2 | 3 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 4 | 24 | 14 |
| vault/resource_pki_secret_backend_root_cert.go | 35 | 8 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 4 | 10 | 1 |
| vault/resource_transit_secret_backend_key.go | 5 | 1 | 0 | 0 | 1 | 1 | 0 | 0 | 5 | 0 | 21 | 10 |
| vault/resource_pki_secret_backend_root_sign_intermediate.go | 31 | 3 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 4 | 7 | 3 |
| vault/resource_pki_secret_backend_intermediate_cert_request.go | 28 | 8 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 3 | 6 | 4 |
| vault/resource_ldap_auth_backend.go | 1 | 0 | 1 | 0 | 1 | 1 | 2 | 0 | 0 | 4 | 10 | 8 |
| vault/resource_aws_auth_backend_role.go | 3 | 0 | 0 | 0 | 1 | 1 | 1 | 0 | 0 | 4 | 47 | 30 |
| vault/resource_pki_secret_backend_cert.go | 11 | 4 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 4 | 22 | 3 |
| vault/resource_consul_secret_backend.go | 1 | 0 | 0 | 0 | 1 | 1 | 1 | 0 | 0 | 4 | 21 | 7 |
| vault/resource_jwt_auth_backend_role.go | 3 | 0 | 0 | 0 | 1 | 0 | 1 | 0 | 0 | 4 | 21 | 20 |
| vault/resource_ad_secret_backend.go | 0 | 0 | 0 | 0 | 1 | 1 | 1 | 0 | 0 | 0 | 61 | 28 |
| vault/resource_ssh_secret_backend_role.go | 2 | 1 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 26 | 4 |
| vault/resource_aws_secret_backend.go | 1 | 1 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 4 | 31 | 18 |
| vault/resource_jwt_auth_backend.go | 2 | 3 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 4 | 10 | 6 |
| vault/resource_identity_group.go | 1 | 0 | 1 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | 25 | 1 |
| vault/resource_kv_secret_v2.go | 2 | 1 | 0 | 0 | 1 | 1 | 1 | 0 | 0 | 4 | 12 | 6 |
| vault/resource_gcp_secret_backend.go | 1 | 2 | 0 | 0 | 1 | 1 | 1 | 0 | 0 | 4 | 15 | 7 |
| vault/resource_quota_rate_limit.go | 2 | 10 | 0 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 20 | 1 |
| vault/resource_gcp_auth_backend.go | 1 | 1 | 0 | 1 | 1 | 1 | 2 | 0 | 0 | 4 | 11 | 5 |

### Score breakdown for top 5 files

- `vault/resource_database_secret_backend_connection.go` (score 144.5): default-on-non-computed×18=54, resource-data-get×111=27.75, schema-resource-data×25=25, nested-elem-resource×7=14, validate-func×5=10, crud-context-fields×4=4
- `vault/resource_pki_secret_backend_role.go` (score 100.5): default-on-non-computed×21=63, optional-computed-without-usestateforunknown×9=9, resource-data-get×24=6, validate-func×2=4, crud-context-fields×4=4, schema-resource-data×4=4
- `vault/resource_pki_secret_backend_root_cert.go` (score 86.75): force-new×35=35, default-on-non-computed×5=15, validate-func×4=8, optional-computed-without-usestateforunknown×7=7, state-upgraders×1=5, customize-diff×1=4
- `vault/resource_transit_secret_backend_key.go` (score 68.75): default-on-non-computed×9=27, customdiff-helper×5=15, resource-data-get×21=5.25, force-new×5=5, schema-resource-data×5=5, customize-diff×1=4
- `vault/resource_pki_secret_backend_root_sign_intermediate.go` (score 66.5): force-new×31=31, default-on-non-computed×4=12, schema-resource-data×6=6, state-upgraders×1=5, validate-func×2=4, crud-context-fields×4=4

### Cross-rule correlations (files combining judgment-rich patterns)

Files hitting multiple high-judgment patterns at once. Read both/all references *before* editing.

- `vault/resource_approle_auth_backend_role_secret_id.go`:
  - StateFunc + DiffSuppressFunc (custom-type with normalisation — destructive-type trap)
- `vault/resource_consul_secret_backend.go`:
  - StateFunc + DiffSuppressFunc (custom-type with normalisation — destructive-type trap)
- `vault/resource_gcp_secret_backend.go`:
  - StateFunc + DiffSuppressFunc (custom-type with normalisation — destructive-type trap)
- `vault/resource_gcp_secret_roleset.go`:
  - CustomizeDiff with customdiff combinators (multi-leg ModifyPlan)
- `vault/resource_identity_group.go`:
  - state-upgrade + composite-ID importer
- `vault/resource_ldap_auth_backend.go`:
  - state-upgrade + composite-ID importer
- `vault/resource_ssh_secret_backend_ca.go`:
  - state-upgrade + composite-ID importer
- `vault/resource_terraform_cloud_secret_backend.go`:
  - StateFunc + DiffSuppressFunc (custom-type with normalisation — destructive-type trap)
- `vault/resource_transit_secret_backend_key.go`:
  - CustomizeDiff with customdiff combinators (multi-leg ModifyPlan)

## Needs manual review

Read these files directly. Even with semgrep's AST-aware matching, the *decision* (block vs nested attribute, single-step state upgrade, composite-ID importer parsing, customdiff structure) requires human/LLM judgment.

- internal/identity/group/group.go — *schema.Set cast (TypeSet expansion → typed model)
- internal/identity/mfa/duo.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/identity/mfa/mfa.go — custom Importer (composite ID parsing?)
- internal/identity/mfa/okta.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/identity/mfa/totp.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- internal/provider/auth.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/auth_aws.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/auth_azure.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/auth_gcp.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/auth_kerberos.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/auth_userpass.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- internal/provider/provider.go — MaxItems:1 (block-vs-nested-attribute decision); nested Elem &Resource (block-vs-nested decision)
- internal/provider/schema_util.go — Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/auth_mount.go — nested Elem &Resource (block-vs-nested decision)
- vault/auth_token.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model)
- vault/data_identity_entity.go — nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/data_identity_group.go — Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/data_source_approle_auth_backend_role_id.go — StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_aws_access_credentials.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_azure_access_credentials.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_gcp_auth_backend_role.go — StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_generic_secret.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_kubernetes_auth_backend_config.go — StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_kubernetes_auth_backend_role.go — StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_kubernetes_credentials.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_namespaces.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/data_source_pki_secret_backend_config_cmpv2.go — nested Elem &Resource (block-vs-nested decision)
- vault/data_source_pki_secret_backend_config_est.go — nested Elem &Resource (block-vs-nested decision)
- vault/data_source_policy_document.go — nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/data_source_transform_decode.go — StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/data_source_transform_encode.go — StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/data_source_transit_cmac.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/data_source_transit_sign.go — Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/data_source_transit_verify.go — Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ad_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ad_secret_library.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_ad_secret_roles.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type)
- vault/resource_alicloud_auth_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_approle_auth_backend_login.go — StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_approle_auth_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_approle_auth_backend_role_secret_id.go — StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_audit.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_audit_request_header.go — Exists callback (gone — use RemoveResource in Read)
- vault/resource_auth_backend.go — MigrateState (upgrade to StateUpgraders first); custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); DiffSuppressFunc (analyse intent); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_aws_auth_backend_cert.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_auth_backend_client.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_auth_backend_config_identity.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_auth_backend_identity_whitelist.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_auth_backend_login.go — Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_aws_auth_backend_role.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); *schema.ResourceDiff function (port to ModifyPlan); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_auth_backend_role_tag.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_auth_backend_roletag_blacklist.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_auth_backend_sts_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); DiffSuppressFunc (analyse intent); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_aws_secret_backend_role.go — custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent); Exists callback (gone — use RemoveResource in Read); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_aws_secret_backend_static_role.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_azure_auth_backend_config.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_azure_auth_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_azure_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); DiffSuppressFunc (analyse intent); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_azure_secret_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_cert_auth_backend_role.go — StateFunc (becomes custom type); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_config_ui_custom_message.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); *schema.Set cast (TypeSet expansion → typed model); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_consul_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); *schema.ResourceDiff function (port to ModifyPlan); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_consul_secret_backend_role.go — custom Importer (composite ID parsing?); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_database_secret_backend_connection.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_database_secret_backend_role.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_database_secret_backend_static_role.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_database_secrets_mount.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); *schema.ResourceDiff function (port to ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_egp_policy.go — custom Importer (composite ID parsing?)
- vault/resource_gcp_auth_backend.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_gcp_auth_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_gcp_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Default without Computed (framework rejects at boot — add Computed in SDKv2 first); Sensitive + StateFunc hash-placeholder (migrate to WriteOnly)
- vault/resource_gcp_secret_impersonated_account.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_gcp_secret_roleset.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); nested Elem &Resource (block-vs-nested decision); customdiff helper combinators (refactor into ModifyPlan); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_gcp_secret_static_account.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); nested Elem &Resource (block-vs-nested decision); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_generic_endpoint.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_generic_secret.go — MigrateState (upgrade to StateUpgraders first); custom Importer (composite ID parsing?); StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_github_auth_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_github_team.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_github_user.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_entity.go — custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent); Exists callback (gone — use RemoveResource in Read); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_entity_alias.go — custom Importer (composite ID parsing?)
- vault/resource_identity_entity_policies.go — *schema.Set cast (TypeSet expansion → typed model); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_group.go — StateUpgraders/SchemaVersion (single-step semantics); custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_group_alias.go — custom Importer (composite ID parsing?)
- vault/resource_identity_group_member_entity_ids.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_group_member_group_ids.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_group_policies.go — *schema.Set cast (TypeSet expansion → typed model); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_oidc.go — Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_identity_oidc_client.go — Optional+Computed without UseStateForUnknown (carry plan modifier across); ForceNew + Computed (framework rejects at boot — fix in SDKv2 first)
- vault/resource_identity_oidc_key.go — custom Importer (composite ID parsing?); Exists callback (gone — use RemoveResource in Read); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_oidc_provider.go — Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_oidc_role.go — custom Importer (composite ID parsing?); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_identity_oidc_scope.go — custom Importer (composite ID parsing?)
- vault/resource_jwt_auth_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.ResourceDiff function (port to ModifyPlan); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_jwt_auth_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_kmip_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_kmip_secret_role.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_kmip_secret_scope.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_kubernetes_auth_backend_config.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_kubernetes_auth_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_kubernetes_secret_backend.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_kubernetes_secret_backend_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_kv_secret.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type)
- vault/resource_kv_secret_backend_v2.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_kv_secret_v2.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.ResourceDiff function (port to ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ldap_auth_backend.go — StateUpgraders/SchemaVersion (single-step semantics); custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ldap_auth_backend_group.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ldap_auth_backend_user.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ldap_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ldap_secret_backend_dynamic_role.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ldap_secret_backend_library_set.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ldap_secret_backend_static_role.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_managed_keys.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_mfa_duo.go — custom Importer (composite ID parsing?)
- vault/resource_mfa_okta.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_mfa_pingid.go — custom Importer (composite ID parsing?)
- vault/resource_mfa_totp.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_mongodbatlas_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan)
- vault/resource_mongodbatlas_secret_role.go — custom Importer (composite ID parsing?)
- vault/resource_mount.go — custom Importer (composite ID parsing?); *schema.Set cast (TypeSet expansion → typed model); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_namespace.go — custom Importer (composite ID parsing?)
- vault/resource_nomad_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_nomad_secret_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_okta_auth_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_okta_auth_backend_group.go — custom Importer (composite ID parsing?); Exists callback (gone — use RemoveResource in Read)
- vault/resource_pki_secret_backend_cert.go — CustomizeDiff (becomes ModifyPlan); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.ResourceDiff function (port to ModifyPlan); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_pki_secret_backend_config_acme.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_pki_secret_backend_config_auto_tidy.go — custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent)
- vault/resource_pki_secret_backend_config_cluster.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type)
- vault/resource_pki_secret_backend_config_cmpv2.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_pki_secret_backend_config_est.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_pki_secret_backend_config_issuers.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_pki_secret_backend_config_urls.go — custom Importer (composite ID parsing?)
- vault/resource_pki_secret_backend_crl_config.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_pki_secret_backend_intermediate_cert_request.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_pki_secret_backend_issuer.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_pki_secret_backend_key.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_pki_secret_backend_role.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_pki_secret_backend_root_cert.go — StateUpgraders/SchemaVersion (single-step semantics); CustomizeDiff (becomes ModifyPlan); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_pki_secret_backend_root_sign_intermediate.go — StateUpgraders/SchemaVersion (single-step semantics); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_pki_secret_backend_sign.go — StateUpgraders/SchemaVersion (single-step semantics); CustomizeDiff (becomes ModifyPlan); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_plugin.go — custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent)
- vault/resource_plugin_pinned_version.go — custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent)
- vault/resource_policy.go — custom Importer (composite ID parsing?)
- vault/resource_quota_lease_count.go — custom Importer (composite ID parsing?); Exists callback (gone — use RemoveResource in Read)
- vault/resource_quota_rate_limit.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); Exists callback (gone — use RemoveResource in Read); *schema.ResourceDiff function (port to ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_rabbitmq_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_rabbitmq_secret_backend_role.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_raft_autopilot.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_raft_snapshot_agent_config.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_rgp_policy.go — custom Importer (composite ID parsing?)
- vault/resource_saml_auth_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_saml_auth_backend_role.go — custom Importer (composite ID parsing?); Optional+Computed without UseStateForUnknown (carry plan modifier across)
- vault/resource_secrets_sync_association.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- vault/resource_secrets_sync_aws_destination.go — custom Importer (composite ID parsing?)
- vault/resource_secrets_sync_azure_destination.go — custom Importer (composite ID parsing?)
- vault/resource_secrets_sync_config.go — custom Importer (composite ID parsing?); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_secrets_sync_gcp_destination.go — custom Importer (composite ID parsing?)
- vault/resource_secrets_sync_gh_destination.go — custom Importer (composite ID parsing?)
- vault/resource_secrets_sync_github_apps.go — custom Importer (composite ID parsing?)
- vault/resource_secrets_sync_vercel_destination.go — custom Importer (composite ID parsing?)
- vault/resource_ssh_secret_backend_ca.go — StateUpgraders/SchemaVersion (single-step semantics); custom Importer (composite ID parsing?); StateFunc (becomes custom type); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_ssh_secret_backend_role.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_terraform_cloud_secret_backend.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_terraform_cloud_secret_role.go — custom Importer (composite ID parsing?); Exists callback (gone — use RemoveResource in Read); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_token.go — custom Importer (composite ID parsing?); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_token_auth_backend_role.go — custom Importer (composite ID parsing?); *schema.Set cast (TypeSet expansion → typed model); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_transform_alphabet.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read)
- vault/resource_transform_role.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read)
- vault/resource_transform_template.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read)
- vault/resource_transform_transformation.go — custom Importer (composite ID parsing?); StateFunc (becomes custom type); Exists callback (gone — use RemoveResource in Read); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)
- vault/resource_transit_cache_config.go — StateFunc (becomes custom type)
- vault/resource_transit_secret_backend_key.go — custom Importer (composite ID parsing?); CustomizeDiff (becomes ModifyPlan); customdiff helper combinators (refactor into ModifyPlan); Optional+Computed without UseStateForUnknown (carry plan modifier across); Default without Computed (framework rejects at boot — add Computed in SDKv2 first)

## Test-file findings

Scanned 236 test files. Test migration is a **provider-level prerequisite** — per-resource test rewrites (workflow step 7) cannot succeed until shared test plumbing has a framework path. Plan this work *before* touching per-resource tests.

- resource.Test/UnitTest/ParallelTest (must use terraform-plugin-testing): **473**
- Providers: (older test field — pre-SDKv2.5): **2**
- PreCheck: (test pre-check, often references *schema.Provider plumbing): **470**
- helper/acctest test utilities: **699**
- d.Id() / d.SetId() calls: **1**
- d.Get / d.GetOk / d.GetOkExists calls: **7**
- d.Set calls: **11**

### Shared test infrastructure (migrate first — per-resource tests depend on these)

Files matching test-infra path conventions (acceptance/, testutil/, provider_test.go, etc.). Every migrated test file references something here; flipping ProviderFactories per resource is wasted effort if the factory isn't framework-aware yet.

- `vault/provider_test.go` [provider_test.go] — helper-acctest=2, resource-data-set=9, schema-provider-type=1, test-pre-check=6, test-providers-field=2, test-resource-test-helper=7
- `testutil/testutil.go` [testutil/ dir] — resource-data-set=3
- `testutil/testutil_test.go` [testutil/ dir] — no audit-rule hits
- `testutil/postgresqlhelper.go` [testutil/ dir] — no audit-rule hits
- `testutil/mssqlhelper.go` [testutil/ dir] — no audit-rule hits
- `testutil/consulhelper.go` [testutil/ dir] — no audit-rule hits

### Top 10 per-resource test files by SDKv2-pattern count

- `vault/resource_database_secret_backend_connection_test.go`: 115 patterns
- `vault/resource_database_secret_backend_static_role_test.go`: 42 patterns
- `vault/resource_jwt_auth_backend_test.go`: 36 patterns
- `vault/resource_aws_auth_backend_role_test.go`: 32 patterns
- `vault/resource_approle_auth_backend_role_secret_id_test.go`: 28 patterns
- `vault/resource_jwt_auth_backend_role_test.go`: 28 patterns
- `vault/resource_mount_test.go`: 28 patterns
- `vault/resource_approle_auth_backend_role_test.go`: 24 patterns
- `vault/resource_aws_auth_backend_client_test.go`: 24 patterns
- `vault/resource_identity_group_member_entity_ids_test.go`: 24 patterns

## Next steps

1. Read every file listed under 'Needs manual review' before proposing edits.
2. Populate `assets/checklist_template.md` from this audit (one entry per resource).
3. Confirm scope with the user before starting workflow step 1.
4. For test files: factor in `ProviderFactories: → ProtoV6ProviderFactories` and `helper/resource → terraform-plugin-testing/helper/resource` swaps when sizing step 7 (TDD gate).
