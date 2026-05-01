# Migration plan: terraform-provider-openstack

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md`
- [ ] User has confirmed scope (whole provider? specific resources?) — **PENDING: team review required; scope below covers whole provider**
- [ ] Decision: protocol v5 or v6 (default v6 — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
- [ ] **2.** Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework. *(Data-consistency review: SDKv2 silently demotes inconsistencies to warnings; the framework surfaces them as hard errors. Scan all resources for inconsistencies — mismatched Computed/Required/Optional flags, Set calls that write attributes not declared in the schema, and any d.Set return errors currently being silently swallowed — and fix them in SDKv2 form before proceeding. This is the data-consistency gate.)*
- [ ] **3.** Serve your provider via the framework. (`main.go` swap; protocol v6 chosen)
- [ ] **4.** Update the provider definition to use the framework.
- [ ] **5.** Update the provider schema to use the framework.
- [ ] **6.** Update each of the provider's resources, data sources, and other Terraform features to use the framework.
- [ ] **7.** Update related tests to use the framework, and ensure that the tests fail. *(TDD gate — write/update tests first, run them red to confirm they exercise the change, then migrate the implementation. Tests written after migration inherit the migrator's blind spots; red-before-green ordering is mandatory. For each resource: switch `ProviderFactories` → `ProtoV6ProviderFactories`, run the test, quote the failing output verbatim in the per-resource row. Only proceed to step 8 after observing a valid red failure.)*
- [ ] **8.** Migrate the resource or data source.
- [ ] **9.** Verify that related tests now pass.
- [ ] **10.** Remove any remaining references to SDKv2 libraries.
- [ ] **11.** Verify that all of your tests continue to pass.
- [ ] **12.** Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

---

## Per-resource checklist

Repeat one row per resource and per data source. For each, fill the audit-flagged hooks (state upgrader, MaxItems:1 block, custom importer, timeouts, sensitive/write-only) only when present. Mark each row only after `verify_tests.sh --migrated-files <file>` exits 0.

Audit flags key: **SU** = StateUpgrader, **M1** = MaxItems:1 block decision, **CI** = custom Importer, **TO** = Timeouts, **CD** = CustomizeDiff, **SF** = StateFunc, **DS** = DiffSuppressFunc, **CW** = ConflictsWith/etc., **NE** = nested Elem &Resource

---

### Resources (219 files → 86 registered resources)

#### openstack_bgpvpn_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red* (workflow step 7 — quote failing output verbatim)
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_bgpvpn_network_associate_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_bgpvpn_port_associate_v2
Audit flags: **CI**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_bgpvpn_router_associate_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_blockstorage_qos_association_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_blockstorage_qos_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_blockstorage_quotaset_v3
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_blockstorage_volume_attach_v3
Audit flags: **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_blockstorage_volume_type_access_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_blockstorage_volume_type_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_blockstorage_volume_v3
Audit flags: **CI**, **TO**, **NE**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to `terraform-plugin-framework-validators`
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_aggregate_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_flavor_access_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_flavor_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_instance_v2
Audit flags: **M1**, **CI**, **TO**, **CD**, **SF**, **DS**, **NE**, **CW** *(highest complexity — migrate last)*
- [ ] Pre-flight C think pass: block decision / state upgrade / import shape written before touching file
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: confirm block syntax in production configs; keep as `SingleNestedBlock` or convert to `SingleNestedAttribute`
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CD: translate `CustomizeDiff` → `ModifyPlan`
- [ ] SF: translate `StateFunc` → custom attribute type
- [ ] DS: analyse `DiffSuppressFunc` intent and translate to plan modifier or validator
- [ ] NE (×5): block-vs-nested-attribute decisions for all nested Elem blocks
- [ ] CW: route ConflictsWith/etc. to `terraform-plugin-framework-validators`
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_interface_attach_v2
Audit flags: **CI**, **TO**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_keypair_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_quotaset_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_servergroup_v2
Audit flags: **M1**, **CI**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] CI: read importer — composite ID or passthrough?
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_compute_volume_attach_v2
Audit flags: **M1**, **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_containerinfra_cluster_v1
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_containerinfra_clustertemplate_v1
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_containerinfra_nodegroup_v1
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_db_configuration_v1
Audit flags: **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_db_database_v1
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_db_instance_v1
Audit flags: **M1**, **TO**, **NE**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE (×4): block-vs-nested-attribute decisions for all nested Elem blocks
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_db_user_v1
Audit flags: **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_dns_quota_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_dns_recordset_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_dns_transfer_accept_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_dns_transfer_request_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_dns_zone_share_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_dns_zone_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_fw_group_v2
Audit flags: **CI**, **TO**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_fw_policy_v2
Audit flags: **CI**, **TO**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_fw_rule_v2
Audit flags: **CI**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_application_credential_v3
Audit flags: **CI**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_ec2_credential_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_endpoint_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_group_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_inherit_role_assignment_v3
Audit flags: **CI**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_limit_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_project_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_registered_limit_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_role_assignment_v3
Audit flags: **CI**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_role_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_service_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_user_membership_v3
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_identity_user_v3
Audit flags: **CI**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_images_image_access_accept_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_images_image_access_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_images_image_v2
Audit flags: **CI**, **TO**, **CD**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CD: translate `CustomizeDiff` → `ModifyPlan`
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_keymanager_container_v1
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_keymanager_order_v1
Audit flags: **M1**, **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_keymanager_secret_v1
Audit flags: **CI**, **TO**, **DS**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] DS: analyse `DiffSuppressFunc` intent and translate to plan modifier or validator
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_flavor_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_flavorprofile_v2
Audit flags: **CI**, **TO**, **SF**, **DS**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] SF: translate `StateFunc` → custom attribute type
- [ ] DS: analyse `DiffSuppressFunc` intent and translate to plan modifier or validator
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_l7policy_v2
Audit flags: **CI**, **TO**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_l7rule_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_listener_v2
Audit flags: **CI**, **TO**, **DS**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] DS: analyse `DiffSuppressFunc` intent and translate to plan modifier or validator
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_loadbalancer_v2
Audit flags: **CI**, **TO**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_member_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_members_v2
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_monitor_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_pool_v2
Audit flags: **M1**, **CI**, **TO**, **NE**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_lb_quota_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_address_group_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_addressscope_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_bgp_peer_v2
Audit flags: **CI**, **TO**, **CD**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CD: translate `CustomizeDiff` → `ModifyPlan`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_bgp_speaker_v2
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_floatingip_associate_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_floatingip_v2
Audit flags: **CI**, **TO**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_network_v2
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_port_secgroup_associate_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_port_v2
Audit flags: **M1**, **CI**, **TO**, **SF**, **DS**, **NE**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] SF: translate `StateFunc` → custom attribute type
- [ ] DS: analyse `DiffSuppressFunc` intent and translate to plan modifier or validator
- [ ] NE (×4): block-vs-nested-attribute decisions for all nested Elem blocks
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_portforwarding_v2
Audit flags: **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_qos_bandwidth_limit_rule_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_qos_dscp_marking_rule_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_qos_minimum_bandwidth_rule_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_qos_policy_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_quota_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_rbac_policy_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_router_interface_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_router_route_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_router_routes_v2
Audit flags: **CI**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_router_v2
Audit flags: **M1**, **CI**, **TO**, **NE**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_secgroup_rule_v2
Audit flags: **CI**, **TO**, **SF**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] SF: translate `StateFunc` → custom attribute type
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_secgroup_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_segment_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_subnet_route_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_subnet_v2
Audit flags: **CI**, **TO**, **NE**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_subnetpool_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_networking_trunk_v2
Audit flags: **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_objectstorage_account_v1
Audit flags: **CI**, **DS**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] DS: analyse `DiffSuppressFunc` intent and translate to plan modifier or validator
- [ ] Tests pass green; negative gate satisfied

#### openstack_objectstorage_container_v1
Audit flags: **M1**, **SU** (SchemaVersion present — single StateUpgrader), **CI**, **NE**, **CW** *(only resource with StateUpgrader — collapse to single-step)*
- [ ] Pre-flight C think pass: block decision / state upgrade / import shape written before touching file
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] SU: collapse chained upgraders to single-step `ResourceWithUpgradeState` entries (see `references/state-upgrade.md`); also read `openstack/migrate_resource_openstack_objectstorage_container_v1.go`
- [ ] CI: read importer — composite ID or passthrough?
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_objectstorage_object_v1
Audit flags: **DS**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] DS: analyse `DiffSuppressFunc` intent and translate to plan modifier or validator
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_objectstorage_tempurl_v1
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] Tests pass green; negative gate satisfied

#### openstack_orchestration_stack_v1
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_sharedfilesystem_securityservice_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_sharedfilesystem_share_access_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_sharedfilesystem_share_v2
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_sharedfilesystem_sharenetwork_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_taas_tap_mirror_v2
Audit flags: **M1**, **CI**, **NE**, **CW**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] M1: block-vs-nested-attribute decision
- [ ] CI: read importer — composite ID or passthrough?
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### openstack_vpnaas_endpoint_group_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_vpnaas_ike_policy_v2
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_vpnaas_ipsec_policy_v2
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_vpnaas_service_v2
Audit flags: **CI**, **TO**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] Tests pass green; negative gate satisfied

#### openstack_vpnaas_site_connection_v2
Audit flags: **CI**, **TO**, **NE**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] TO: migrate to `terraform-plugin-framework-timeouts`
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### openstack_workflow_cron_trigger_v2
Audit flags: **CI**
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] CI: read importer — composite ID or passthrough?
- [ ] Tests pass green; negative gate satisfied

---

### Data Sources

#### data.openstack_blockstorage_availability_zones_v3
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_blockstorage_snapshot_v3
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_blockstorage_volume_v3
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_compute_availability_zones_v2
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_compute_flavor_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_compute_hypervisor_v2
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_compute_instance_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_compute_limits_v2
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_compute_servergroup_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_fw_group_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_fw_policy_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_fw_rule_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_identity_auth_scope_v3
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_identity_project_ids_v3
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_identity_project_v3
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_images_image_ids_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_images_image_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_keymanager_container_v1
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_lb_flavor_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_lb_flavorprofile_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_lb_listener_v2
Audit flags: **NE**, **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_lb_loadbalancer_v2
Audit flags: **NE**, **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_lb_member_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_lb_monitor_v2
Audit flags: **NE**, **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_lb_pool_v2
Audit flags: **NE**, **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_loadbalancer_flavor_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_networking_network_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_networking_port_ids_v2
*(no special audit flags — but high ForceNew count; verify attributes)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_networking_port_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_networking_router_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_networking_subnet_ids_v2
Audit flags: **CW** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] CW: route ConflictsWith/etc. to validators
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_networking_subnet_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_networking_trunk_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_sharedfilesystem_availability_zones_v2
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_sharedfilesystem_share_v2
Audit flags: **NE** (needs manual review)
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] NE: block-vs-nested-attribute decision for nested Elem
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_sharedfilesystem_snapshot_v2
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

#### data.openstack_workflow_workflow_v2
*(no special audit flags)*
- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

---

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
