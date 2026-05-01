# Migration plan: terraform-provider-openstack

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md`
- [ ] User has confirmed scope (whole provider? specific resources?)
- [ ] Decision: protocol v5 or v6 (default v6 — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
- [ ] **2.** Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework.
- [ ] **3.** Serve your provider via the framework. (`main.go` swap; protocol v6 chosen)
- [ ] **4.** Update the provider definition to use the framework.
- [ ] **5.** Update the provider schema to use the framework.
- [ ] **6.** Update each of the provider's resources, data sources, and other Terraform features to use the framework.
- [ ] **7.** Update related tests to use the framework, and ensure that the tests fail. *(TDD gate — write/update tests first, run them red to confirm they exercise the change, then migrate the implementation. Only proceed to step 8 after confirming the tests fail with a compile error or protocol-mismatch assertion.)*
- [ ] **8.** Migrate the resource or data source.
- [ ] **9.** Verify that related tests now pass.
- [ ] **10.** Remove any remaining references to SDKv2 libraries.
- [ ] **11.** Verify that all of your tests continue to pass.
- [ ] **12.** Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

## Per-resource checklist

Repeat one row per resource and per data source. For each, fill the audit-flagged hooks (state upgrader, MaxItems:1 block, custom importer, timeouts, sensitive/write-only) only when present. Mark each row only after `verify_tests.sh --migrated-files <file>` exits 0.

---

### resource_openstack_bgpvpn_network_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_bgpvpn_port_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_bgpvpn_router_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_bgpvpn_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_blockstorage_qos_association_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_blockstorage_qos_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_blockstorage_quotaset_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_blockstorage_volume_attach_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_blockstorage_volume_type_access_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_blockstorage_volume_type_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_blockstorage_volume_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_aggregate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_flavor_access_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_flavor_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_instance_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); CustomizeDiff (becomes ModifyPlan); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_interface_attach_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_keypair_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_quotaset_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_servergroup_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_compute_volume_attach_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_containerinfra_cluster_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_containerinfra_clustertemplate_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_containerinfra_nodegroup_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_db_configuration_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_db_database_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_db_instance_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_db_user_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_dns_quota_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_dns_recordset_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_dns_transfer_accept_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_dns_transfer_request_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_dns_zone_share_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_dns_zone_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_fw_group_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_fw_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_fw_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_application_credential_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_ec2_credential_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_endpoint_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_group_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_inherit_role_assignment_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_limit_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_project_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_registered_limit_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_role_assignment_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_role_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_service_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_user_membership_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_identity_user_v3

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_images_image_access_accept_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_images_image_access_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_images_image_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); CustomizeDiff (becomes ModifyPlan); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_keymanager_container_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_keymanager_order_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_keymanager_secret_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); DiffSuppressFunc (analyse intent)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_flavor_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_flavorprofile_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_l7policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_l7rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_listener_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); DiffSuppressFunc (analyse intent); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_loadbalancer_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_member_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_members_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_monitor_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_pool_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_lb_quota_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_address_group_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_addressscope_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_bgp_peer_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); CustomizeDiff (becomes ModifyPlan)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_bgp_speaker_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_floatingip_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_floatingip_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_network_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_port_secgroup_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_port_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); StateFunc (becomes custom type); DiffSuppressFunc (analyse intent); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_portforwarding_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_qos_bandwidth_limit_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_qos_dscp_marking_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_qos_minimum_bandwidth_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_qos_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_quota_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_rbac_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_router_interface_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_router_route_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_router_routes_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_router_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_secgroup_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); StateFunc (becomes custom type); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_secgroup_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_segment_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_subnet_route_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_subnet_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_subnetpool_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_networking_trunk_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_objectstorage_account_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); DiffSuppressFunc (analyse intent)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_objectstorage_container_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); StateUpgraders/SchemaVersion (single-step semantics — see `references/state-upgrade.md`); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_objectstorage_object_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: DiffSuppressFunc (analyse intent); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_objectstorage_tempurl_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: (none flagged by audit — standard migration)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_orchestration_stack_v1

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_sharedfilesystem_securityservice_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_sharedfilesystem_share_access_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_sharedfilesystem_share_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_sharedfilesystem_sharenetwork_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_taas_tap_mirror_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: MaxItems:1 (block-vs-nested-attribute decision); custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_vpnaas_endpoint_group_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_vpnaas_ike_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_vpnaas_ipsec_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_vpnaas_service_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_vpnaas_site_connection

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?); Timeouts (separate framework package); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### resource_openstack_workflow_cron_trigger_v2

- [ ] Tests written/updated and run *red* (workflow step 7 — tests must fail before implementation changes)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled: custom Importer (composite ID parsing?)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

## Per-data-source checklist

### data_source_openstack_blockstorage_availability_zones_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_blockstorage_quotaset_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_blockstorage_snapshot_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_blockstorage_volume_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_aggregate_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_availability_zones_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_flavor_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_hypervisor_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_instance_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_keypair_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_limits_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_quotaset_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_compute_servergroup_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_containerinfra_cluster_v1

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_containerinfra_clustertemplate_v1

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_containerinfra_nodegroup_v1

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_dns_zone_share_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_dns_zone_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_fw_group_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_fw_policy_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_fw_rule_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_auth_scope_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_endpoint_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_group_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_project_ids_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_project_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_role_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_service_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_identity_user_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_images_image_ids_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_images_image_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_keymanager_container_v1

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_keymanager_secret_v1

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_lb_flavor_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_lb_flavorprofile_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_lb_listener_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_lb_loadbalancer_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_lb_member_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_lb_monitor_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_lb_pool_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision); ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_loadbalancer_flavor_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_addressscope_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_floatingip_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_network_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_port_ids_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_port_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_qos_bandwidth_limit_rule_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_qos_dscp_marking_rule_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_qos_minimum_bandwidth_rule_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_qos_policy_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_quota_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_router_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_secgroup_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_segment_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_subnet_ids_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (validator routing decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_subnet_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_subnetpool_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_networking_trunk_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_sharedfilesystem_availability_zones_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_sharedfilesystem_share_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision) — needs manual review
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_sharedfilesystem_sharenetwork_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_sharedfilesystem_snapshot_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_workflow_cron_trigger_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### data_source_openstack_workflow_workflow_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

---

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (if v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
