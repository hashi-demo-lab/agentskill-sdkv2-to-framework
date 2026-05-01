# SDKv2 → Framework Migration Audit

**Provider:** v3    **Audited:** 2026-05-01    **SDKv2 version:** v2.38.1

## Summary

- Production Go files audited: 236
- Test Go files audited: 300

- **Resource/data-source constructors detected: 174** (each is a `func ...() *schema.Resource` — direct migration count)

### Provider-level migration cost

These patterns indicate work in `provider.go` / Configure path, separate from per-resource cost. The framework provider type and Configure method must be set up before any resource migration can be tested.

- ResourcesMap references: **1**
- DataSourcesMap references: **1**
- *schema.Provider type references: **2**
- schema.EnvDefaultFunc / MultiEnvDefaultFunc (provider-config): **34**

### Schema-level fields
- ForceNew: true: **637**
- ValidateFunc / ValidateDiagFunc: **140**
- DiffSuppressFunc: **8**
- CustomizeDiff: **3**
- StateFunc: **4**
- Sensitive: true: **20**
- Deprecated attribute: **1**
- Default: ... (defaults package, NOT PlanModifiers): **1**
- ConflictsWith / ExactlyOneOf / AtLeastOneOf / RequiredWith: **110**

### Resource-level fields
- Importer: **101**
- schema.ImportStatePassthroughContext (trivial importer): **87**
- Timeouts: **68**
- StateUpgraders: **1**
- SchemaVersion: **1**

### Block / nested-attribute decisions
- MaxItems:1 + nested Elem (block decision): **11**
- Nested Elem &Resource (any block): **71**
- MinItems > 0 (true repeating block): **7**
- TypeList/Set/Map of primitive (Elem &Schema{Type:}): **157**

### Helper packages (need replacement)
- retry.StateChangeConf (no framework equivalent): **121**
- retry.RetryContext: **33**
- helper/customdiff combinators: **3**
- helper/validation.* calls (replace with framework-validators): **114**
- helper/structure JSON normalisation helpers: **2**

### CRUD-body shape
- CreateContext/ReadContext/UpdateContext/DeleteContext: **469**
- *schema.ResourceData function-param references: **534**
- Resource constructor (count = resources to migrate): **174**
- *schema.ResourceDiff function (port to ModifyPlan body): **1**
- d.Id() / d.SetId() calls: **1151**
- d.Get / d.GetOk / d.GetOkExists calls: **1667**
- d.Set calls: **1712**
- d.HasChange / d.GetChange / d.IsNewResource / d.Partial: **483**
- Inline *schema.Set cast from d.Get: **75**
- diag.FromErr / diag.Errorf: **1620**
- schema.DefaultTimeout / d.Timeout (timeouts): **309**

## Per-file findings (top 20 by complexity, production code)

| File | ForceNew | Validators | StateUpgr | MaxIt:1 | Imptr | CustDiff | StateFunc | retry.SCC | custdiff | CRUDctx | d.Get | d.Set |
|------|---------:|-----------:|----------:|--------:|------:|---------:|----------:|----------:|---------:|--------:|------:|------:|
| openstack/resource_openstack_compute_instance_v2.go | 33 | 4 | 0 | 1 | 1 | 1 | 1 | 13 | 3 | 4 | 32 | 23 |
| openstack/provider.go | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 31 | 0 |
| openstack/resource_openstack_networking_port_v2.go | 7 | 3 | 0 | 1 | 1 | 0 | 1 | 2 | 0 | 4 | 25 | 18 |
| openstack/resource_openstack_blockstorage_volume_v3.go | 13 | 2 | 0 | 0 | 1 | 0 | 0 | 5 | 0 | 4 | 24 | 13 |
| openstack/resource_openstack_containerinfra_cluster_v1.go | 17 | 0 | 0 | 0 | 1 | 0 | 0 | 4 | 0 | 4 | 11 | 27 |
| openstack/resource_openstack_networking_subnet_v2.go | 10 | 8 | 0 | 0 | 1 | 0 | 0 | 2 | 0 | 4 | 31 | 21 |
| openstack/resource_openstack_db_instance_v1.go | 21 | 0 | 0 | 1 | 0 | 0 | 0 | 2 | 0 | 4 | 9 | 5 |
| openstack/resource_openstack_keymanager_secret_v1.go | 10 | 8 | 0 | 0 | 1 | 0 | 0 | 3 | 0 | 4 | 12 | 18 |
| openstack/resource_openstack_lb_pool_v2.go | 5 | 10 | 0 | 1 | 1 | 0 | 0 | 0 | 0 | 4 | 30 | 18 |
| openstack/resource_openstack_images_image_v2.go | 7 | 12 | 0 | 0 | 1 | 1 | 0 | 1 | 0 | 4 | 20 | 23 |
| openstack/resource_openstack_lb_listener_v2.go | 5 | 8 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 4 | 51 | 28 |
| openstack/resource_openstack_lb_monitor_v2.go | 4 | 10 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 4 | 28 | 16 |
| openstack/resource_openstack_vpnaas_ike_policy_v2.go | 3 | 5 | 0 | 0 | 1 | 0 | 0 | 3 | 0 | 4 | 17 | 10 |
| openstack/resource_openstack_vpnaas_ipsec_policy_v2.go | 3 | 5 | 0 | 0 | 1 | 0 | 0 | 3 | 0 | 4 | 17 | 10 |
| openstack/resource_openstack_containerinfra_clustertemplate_v1.go | 3 | 2 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 4 | 55 | 33 |
| openstack/resource_openstack_networking_secgroup_rule_v2.go | 12 | 7 | 0 | 0 | 1 | 0 | 1 | 1 | 0 | 3 | 12 | 12 |
| openstack/resource_openstack_containerinfra_nodegroup_v1.go | 10 | 2 | 0 | 0 | 1 | 0 | 0 | 3 | 0 | 4 | 16 | 14 |
| openstack/resource_openstack_keymanager_order_v1.go | 9 | 6 | 0 | 1 | 1 | 0 | 0 | 2 | 0 | 3 | 2 | 12 |
| openstack/resource_openstack_vpnaas_site_connection.go | 6 | 0 | 0 | 0 | 1 | 0 | 0 | 3 | 0 | 4 | 30 | 18 |
| openstack/resource_openstack_dns_zone_v2.go | 6 | 2 | 0 | 0 | 1 | 0 | 0 | 3 | 0 | 4 | 14 | 10 |

### Score breakdown for top 5 files

- `openstack/resource_openstack_compute_instance_v2.go` (score 162.75): retry-state-change-conf×13=39, force-new×33=33, schema-default-timeout×16=16, schema-resource-data×15=15, nested-elem-resource×5=10, customdiff-helper×3=9
- `openstack/provider.go` (score 64.75): env-default-func×28=56, resource-data-get×31=7.75, schema-resource-data×1=1
- `openstack/resource_openstack_networking_port_v2.go` (score 61.75): nested-elem-resource×4=8, force-new×7=7, resource-data-get×25=6.25, retry-state-change-conf×2=6, resource-data-set×18=4.5, validate-func×2=4
- `openstack/resource_openstack_blockstorage_volume_v3.go` (score 61.25): retry-state-change-conf×5=15, force-new×13=13, resource-data-get×24=6, schema-default-timeout×6=6, schema-resource-data×5=5, nested-elem-resource×2=4
- `openstack/resource_openstack_containerinfra_cluster_v1.go` (score 57.5): force-new×17=17, retry-state-change-conf×4=12, schema-default-timeout×7=7, resource-data-set×27=6.75, crud-context-fields×4=4, schema-resource-data×4=4

### Cross-rule correlations (files combining judgment-rich patterns)

Files hitting multiple high-judgment patterns at once. Read both/all references *before* editing.

- `openstack/resource_openstack_blockstorage_volume_attach_v3.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_blockstorage_volume_v3.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_compute_instance_v2.go`:
  - CustomizeDiff with customdiff combinators (multi-leg ModifyPlan)
  - MaxItems:1 + many nested blocks (deep block-vs-attribute decision tree)
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
  - StateFunc + DiffSuppressFunc (custom-type with normalisation — destructive-type trap)
- `openstack/resource_openstack_compute_interface_attach_v2.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_compute_volume_attach_v2.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_containerinfra_cluster_v1.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_containerinfra_nodegroup_v1.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_db_configuration_v1.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_db_database_v1.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_db_instance_v1.go`:
  - MaxItems:1 + many nested blocks (deep block-vs-attribute decision tree)
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_db_user_v1.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_dns_recordset_v2.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_dns_transfer_accept_v2.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_dns_transfer_request_v2.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)
- `openstack/resource_openstack_dns_zone_v2.go`:
  - retry.StateChangeConf + Timeouts (full async state-change refactor)

## Needs manual review

Read these files directly. Even with semgrep's AST-aware matching, the *decision* (block vs nested attribute, single-step state upgrade, composite-ID importer parsing, customdiff structure) requires human/LLM judgment.

- openstack/data_source_openstack_blockstorage_volume_v3.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_compute_flavor_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_compute_instance_v2.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_compute_servergroup_v2.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_fw_group_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_fw_policy_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_fw_rule_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_identity_auth_scope_v3.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_identity_project_ids_v3.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_identity_project_v3.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_images_image_ids_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/data_source_openstack_images_image_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/data_source_openstack_keymanager_container_v1.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_lb_flavor_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_lb_flavorprofile_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_lb_listener_v2.go — nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_lb_loadbalancer_v2.go — nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_lb_member_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_lb_monitor_v2.go — nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_lb_pool_v2.go — nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_loadbalancer_flavor_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_networking_network_v2.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_networking_port_ids_v2.go — *schema.Set cast (TypeSet expansion → typed model)
- openstack/data_source_openstack_networking_port_v2.go — nested Elem &Resource (block-vs-nested decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/data_source_openstack_networking_router_v2.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_networking_subnet_ids_v2.go — ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/data_source_openstack_networking_subnet_v2.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_networking_trunk_v2.go — nested Elem &Resource (block-vs-nested decision)
- openstack/data_source_openstack_sharedfilesystem_share_v2.go — nested Elem &Resource (block-vs-nested decision)
- openstack/fw_group_v2.go — retry.StateChangeConf (replace with inline ticker loop)
- openstack/images_image_v2.go — *schema.ResourceDiff function (port to ModifyPlan)
- openstack/keymanager_v1.go — nested Elem &Resource (block-vs-nested decision)
- openstack/lb_v2_shared.go — retry.StateChangeConf (replace with inline ticker loop)
- openstack/migrate_resource_openstack_objectstorage_container_v1.go — MaxItems:1 (block-vs-nested-attribute decision); nested Elem &Resource (block-vs-nested decision)
- openstack/resource_openstack_bgpvpn_network_associate_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_bgpvpn_port_associate_v2.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_bgpvpn_router_associate_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_bgpvpn_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_blockstorage_qos_association_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_blockstorage_qos_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_blockstorage_quotaset_v3.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_blockstorage_volume_attach_v3.go — Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_blockstorage_volume_type_access_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_blockstorage_volume_type_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_blockstorage_volume_v3.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_compute_aggregate_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_compute_flavor_access_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_compute_flavor_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_compute_instance_v2.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop); customdiff helper combinators (refactor into ModifyPlan); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_compute_interface_attach_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_compute_keypair_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_compute_quotaset_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_compute_servergroup_v2.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- openstack/resource_openstack_compute_volume_attach_v2.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_containerinfra_cluster_v1.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_containerinfra_clustertemplate_v1.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_containerinfra_nodegroup_v1.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_db_configuration_v1.go — Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_db_database_v1.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_db_instance_v1.go — MaxItems:1 (block-vs-nested-attribute decision); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_db_user_v1.go — Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_dns_quota_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_dns_recordset_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_dns_transfer_accept_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_dns_transfer_request_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_dns_zone_share_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_dns_zone_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_fw_group_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_fw_policy_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_fw_rule_v2.go — custom Importer (composite ID parsing?); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/resource_openstack_identity_application_credential_v3.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_identity_ec2_credential_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_endpoint_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_group_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_inherit_role_assignment_v3.go — custom Importer (composite ID parsing?); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/resource_openstack_identity_limit_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_project_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_registered_limit_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_role_assignment_v3.go — custom Importer (composite ID parsing?); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/resource_openstack_identity_role_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_service_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_user_membership_v3.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_identity_user_v3.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- openstack/resource_openstack_images_image_access_accept_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_images_image_access_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_images_image_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); CustomizeDiff (becomes ModifyPlan); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_keymanager_container_v1.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_keymanager_order_v1.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_keymanager_secret_v1.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); DiffSuppressFunc (analyse intent); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_lb_flavor_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_lb_flavorprofile_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); helper/structure JSON normalisation (refactor to custom type or plan modifier)
- openstack/resource_openstack_lb_l7policy_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/resource_openstack_lb_l7rule_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_lb_listener_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); DiffSuppressFunc (analyse intent); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_lb_loadbalancer_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/resource_openstack_lb_member_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_lb_members_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_lb_monitor_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_lb_pool_v2.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_lb_quota_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_networking_address_group_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_addressscope_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_bgp_peer_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); CustomizeDiff (becomes ModifyPlan)
- openstack/resource_openstack_networking_bgp_speaker_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_floatingip_associate_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_networking_floatingip_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_network_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_port_secgroup_associate_v2.go — custom Importer (composite ID parsing?); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_port_v2.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop); helper/structure JSON normalisation (refactor to custom type or plan modifier); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_portforwarding_v2.go — Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_qos_bandwidth_limit_rule_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_qos_dscp_marking_rule_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_qos_minimum_bandwidth_rule_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_qos_policy_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_quota_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_networking_rbac_policy_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_networking_router_interface_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_router_route_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_networking_router_routes_v2.go — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_router_v2.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_secgroup_rule_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); StateFunc (becomes custom type); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_secgroup_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_segment_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_networking_subnet_route_v2.go — custom Importer (composite ID parsing?)
- openstack/resource_openstack_networking_subnet_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_networking_subnetpool_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_networking_trunk_v2.go — Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_objectstorage_account_v1.go — custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent)
- openstack/resource_openstack_objectstorage_container_v1.go — MaxItems:1 (block-vs-nested-attribute decision); StateUpgraders/SchemaVersion (single-step semantics); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_objectstorage_object_v1.go — DiffSuppressFunc (analyse intent); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/resource_openstack_orchestration_stack_v1.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_sharedfilesystem_securityservice_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- openstack/resource_openstack_sharedfilesystem_share_access_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_sharedfilesystem_share_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_sharedfilesystem_sharenetwork_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_taas_tap_mirror_v2.go — MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- openstack/resource_openstack_vpnaas_endpoint_group_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_vpnaas_ike_policy_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_vpnaas_ipsec_policy_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_vpnaas_service_v2.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); retry.StateChangeConf (replace with inline ticker loop)
- openstack/resource_openstack_vpnaas_site_connection.go — custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); retry.StateChangeConf (replace with inline ticker loop); *schema.Set cast (TypeSet expansion → typed model)
- openstack/resource_openstack_workflow_cron_trigger_v2.go — custom Importer (composite ID parsing?)
- openstack/util.go — *schema.Set cast (TypeSet expansion → typed model)

## Test-file findings

Scanned 300 test files. Test migration is a **provider-level prerequisite** — per-resource test rewrites (workflow step 7) cannot succeed until shared test plumbing has a framework path. Plan this work *before* touching per-resource tests.

- ProviderFactories: (test config — must become ProtoV6ProviderFactories): **526**
- resource.Test/UnitTest/ParallelTest (must use terraform-plugin-testing): **526**
- PreCheck: (test pre-check, often references *schema.Provider plumbing): **526**
- helper/acctest test utilities: **122**
- d.Id() / d.SetId() calls: **13**
- d.Get / d.GetOk / d.GetOkExists calls: **11**
- d.Set calls: **11**

### Shared test infrastructure (migrate first — per-resource tests depend on these)

Files matching test-infra path conventions (acceptance/, testutil/, provider_test.go, etc.). Every migrated test file references something here; flipping ProviderFactories per resource is wasted effort if the factory isn't framework-aware yet.

- `openstack/provider_test.go` [provider_test.go] — schema-provider-type=1

### Top 10 per-resource test files by SDKv2-pattern count

- `openstack/resource_openstack_compute_instance_v2_test.go`: 81 patterns
- `openstack/resource_openstack_networking_port_v2_test.go`: 72 patterns
- `openstack/resource_openstack_networking_network_v2_test.go`: 48 patterns
- `openstack/resource_openstack_networking_subnet_v2_test.go`: 36 patterns
- `openstack/resource_openstack_networking_router_v2_test.go`: 27 patterns
- `openstack/networking_port_v2_test.go`: 26 patterns
- `openstack/resource_openstack_compute_servergroup_v2_test.go`: 24 patterns
- `openstack/resource_openstack_images_image_v2_test.go`: 24 patterns
- `openstack/import_openstack_containerinfra_nodegroup_v1_test.go`: 21 patterns
- `openstack/resource_openstack_blockstorage_volume_v3_test.go`: 21 patterns

## Next steps

1. Read every file listed under 'Needs manual review' before proposing edits.
2. Populate `assets/checklist_template.md` from this audit (one entry per resource).
3. Confirm scope with the user before starting workflow step 1.
4. For test files: factor in `ProviderFactories: → ProtoV6ProviderFactories` and `helper/resource → terraform-plugin-testing/helper/resource` swaps when sizing step 7 (TDD gate).
