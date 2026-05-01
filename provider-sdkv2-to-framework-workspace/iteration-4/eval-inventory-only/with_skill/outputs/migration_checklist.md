# Migration plan: terraform-provider-openstack

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md`
- [ ] User has confirmed scope (whole provider? specific resources?)
- [ ] Decision: protocol v5 or v6 (default v6 — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end (see "Needs manual review" in audit_report.md — 113 files flagged)

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
- [ ] **2.** Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework.
- [ ] **3.** Serve your provider via the framework. (`main.go` swap; protocol v6 chosen)
- [ ] **4.** Update the provider definition to use the framework.
- [ ] **5.** Update the provider schema to use the framework.
- [ ] **6.** Update each of the provider's resources, data sources, and other Terraform features to use the framework.
- [ ] **7.** Update related tests to use the framework, and ensure that the tests fail. *(TDD gate — tests fail first, then migrate.)*
- [ ] **8.** Migrate the resource or data source.
- [ ] **9.** Verify that related tests now pass.
- [ ] **10.** Remove any remaining references to SDKv2 libraries.
- [ ] **11.** Verify that all of your tests continue to pass.
- [ ] **12.** Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

---

## Per-resource checklist

Repeat one block per resource and one per data source. Mark each box only after the resource passes `verify_tests.sh --migrated-files <file>`.

> **Suggested migration order (highest-risk first):** Start with the single StateUpgrader resource (`openstack_objectstorage_container_v1`), then tackle the highest-complexity resources (`openstack_compute_instance_v2`, `openstack_db_instance_v1`, `openstack_networking_port_v2`) before proceeding service-area by service-area.

---

### Resource: openstack_objectstorage_container_v1
*Flags: StateUpgraders/SchemaVersion, MaxItems:1, custom Importer, nested Elem*

- [ ] **Block decision**: `website` attribute is `MaxItems:1` + nested Elem — keep as `ListNestedBlock` with `SizeAtMost(1)` validator (backward-compat; no major-version bump planned).
- [ ] **State upgrade**: 1 StateUpgrader present (`migrate_resource_openstack_objectstorage_container_v1.go`) — rewrite as single-step `ResourceWithUpgradeState` targeting the final schema version. Read `references/state-upgrade.md`.
- [ ] **Import shape**: Verify importer in `resource_openstack_objectstorage_container_v1.go` — check for composite-ID parsing.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] State upgraders translated — single-step semantics (`references/state-upgrade.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_compute_instance_v2
*Flags: MaxItems:1 (1), ForceNew (33), Validators (2), CustomizeDiff, StateFunc, DiffSuppressFunc, nested Elem (5), custom Importer, Timeouts — highest complexity score*

- [ ] **Block decision**: `network`, `block_device`, `personality`, `scheduler_hints` nested blocks — keep as `ListNestedBlock`/`SetNestedBlock` (backward-compat). The single `MaxItems:1` block (likely `vendor_options`) → `ListNestedBlock` with `SizeAtMost(1)`.
- [ ] **State upgrade**: No StateUpgraders found.
- [ ] **Import shape**: Importer present — verify whether it is passthrough or composite-ID.
- [ ] `CustomizeDiff` → `ModifyPlan` — read `references/resources.md` for `ResourceWithModifyPlan`.
- [ ] `StateFunc` → custom type — read `references/attributes.md`.
- [ ] `DiffSuppressFunc` — analyse intent before converting; may become `PlanModifier` or custom type.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_db_instance_v1
*Flags: MaxItems:1 (1), ForceNew (21), nested Elem (4), Timeouts*

- [ ] **Block decision**: `datastore`, `database`, `user`, `network` nested blocks — keep as `ListNestedBlock` (backward-compat).
- [ ] **State upgrade**: None.
- [ ] **Import shape**: No importer detected.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_networking_port_v2
*Flags: MaxItems:1 (1), ForceNew (7), Validators (2), StateFunc, DiffSuppressFunc, nested Elem (4), custom Importer, Timeouts*

- [ ] **Block decision**: `fixed_ip`, `allowed_address_pairs`, `binding` nested structures — keep as blocks (backward-compat).
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID or passthrough.
- [ ] `StateFunc` → custom type. `DiffSuppressFunc` → analyse intent.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_networking_secgroup_rule_v2
*Flags: ForceNew (12), Validators (5), StateFunc, custom Importer, Timeouts*

- [ ] **Block decision**: No nested Elem blocks.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify.
- [ ] `StateFunc` → custom type.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_images_image_v2
*Flags: ForceNew (7), Validators (6), CustomizeDiff, custom Importer, Timeouts*

- [ ] **Block decision**: No MaxItems:1 nested blocks.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify.
- [ ] `CustomizeDiff` → `ModifyPlan`.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_keymanager_order_v1
*Flags: MaxItems:1 (1), ForceNew (9), Validators (3), nested Elem (1), custom Importer, Timeouts*

- [ ] **Block decision**: `meta` nested block — keep as `ListNestedBlock` with `SizeAtMost(1)` (backward-compat).
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID parsing.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_lb_pool_v2
*Flags: MaxItems:1 (1), ForceNew (5), Validators (5), nested Elem (1), custom Importer, Timeouts*

- [ ] **Block decision**: `persistence` block — keep as `ListNestedBlock` with `SizeAtMost(1)` (backward-compat).
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_keymanager_secret_v1
*Flags: ForceNew (10), Validators (4), DiffSuppressFunc, custom Importer, Timeouts*

- [ ] **Block decision**: No MaxItems:1 nested blocks.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID.
- [ ] `DiffSuppressFunc` — analyse intent before converting.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_networking_subnet_v2
*Flags: ForceNew (10), Validators (4), nested Elem (1), custom Importer, Timeouts*

- [ ] **Block decision**: `allocation_pool` nested block — keep as `ListNestedBlock`.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_blockstorage_volume_v3
*Flags: ForceNew (13), Validators (1), nested Elem (2), custom Importer, Timeouts*

- [ ] **Block decision**: `attachment`, `multiattach` nested structures — keep as blocks.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_identity_application_credential_v3
*Flags: ForceNew (12), Validators (2), nested Elem (1), custom Importer*

- [ ] **Block decision**: `access_rules` nested block — keep as `SetNestedBlock`.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_containerinfra_cluster_v1
*Flags: ForceNew (17), custom Importer, Timeouts*

- [ ] **Block decision**: No nested Elem blocks.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_networking_bgp_peer_v2
*Flags: ForceNew (5), Validators (3), CustomizeDiff, custom Importer, Timeouts*

- [ ] **Block decision**: No nested Elem blocks.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID.
- [ ] `CustomizeDiff` → `ModifyPlan`.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_lb_flavorprofile_v2
*Flags: StateFunc, DiffSuppressFunc, custom Importer, Timeouts*

- [ ] **Block decision**: No nested Elem blocks.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify.
- [ ] `StateFunc` → custom type. `DiffSuppressFunc` → analyse intent.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_taas_tap_mirror_v2
*Flags: MaxItems:1 (1), ForceNew (8), Validators (1), nested Elem (1), custom Importer*

- [ ] **Block decision**: `directions` block — keep as `ListNestedBlock` with `SizeAtMost(1)` (backward-compat).
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_compute_servergroup_v2
*Flags: MaxItems:1 (1), nested Elem (1), custom Importer*

- [ ] **Block decision**: `rules` block — keep as `ListNestedBlock` with `SizeAtMost(1)` (backward-compat).
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_compute_volume_attach_v2
*Flags: MaxItems:1 (1), nested Elem (1), custom Importer, Timeouts*

- [ ] **Block decision**: Nested block — keep as `ListNestedBlock` with `SizeAtMost(1)` (backward-compat).
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify composite-ID.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Resource: openstack_networking_router_v2
*Flags: MaxItems:1 (1), nested Elem (1), custom Importer, Timeouts*

- [ ] **Block decision**: `external_fixed_ip` — keep as `ListNestedBlock` with `SizeAtMost(1)`.
- [ ] **State upgrade**: None.
- [ ] **Import shape**: Importer present — verify.
- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

<!-- STANDARD RESOURCES (no special flags beyond Importer/Timeouts/ForceNew) -->
<!-- Each still requires the full per-resource checklist — abbreviated here for brevity -->

### Resource: openstack_bgpvpn_network_associate_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_bgpvpn_port_associate_v2
*Flags: custom Importer (composite-ID), nested Elem*

- [ ] **Block decision**: Nested Elem blocks — keep as blocks (backward-compat).
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_bgpvpn_router_associate_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_bgpvpn_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_blockstorage_qos_association_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_blockstorage_qos_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_blockstorage_quotaset_v3
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_blockstorage_volume_attach_v3
*Flags: Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_blockstorage_volume_type_access_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_blockstorage_volume_type_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_compute_aggregate_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_compute_flavor_access_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_compute_flavor_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_compute_interface_attach_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_compute_keypair_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_compute_quotaset_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_containerinfra_clustertemplate_v1
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_containerinfra_nodegroup_v1
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_db_configuration_v1
*Flags: nested Elem, Timeouts*

- [ ] **Block decision**: `datastore` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_db_database_v1
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_db_user_v1
*Flags: Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_dns_quota_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_dns_recordset_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_dns_transfer_accept_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_dns_transfer_request_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_dns_zone_share_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_dns_zone_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_fw_group_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_fw_policy_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_fw_rule_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_ec2_credential_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Sensitive / write-only attributes handled
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_endpoint_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_group_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_inherit_role_assignment_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_limit_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_project_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_registered_limit_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_role_assignment_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_role_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_service_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_user_membership_v3
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_identity_user_v3
*Flags: custom Importer (composite-ID), nested Elem*

- [ ] **Block decision**: `multi_factor_auth_rule` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_images_image_access_accept_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_images_image_access_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_keymanager_container_v1
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `secret_refs` nested block — keep as `SetNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_flavor_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_l7policy_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_l7rule_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_listener_v2
*Flags: ForceNew (5), Validators (4), DiffSuppressFunc, custom Importer, Timeouts*

- [ ] `DiffSuppressFunc` — analyse intent before converting.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Validators translated; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_loadbalancer_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_member_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_members_v2
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `member` nested block — keep as `SetNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_monitor_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_lb_quota_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_address_group_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_addressscope_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_bgp_speaker_v2
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `advertised_networks` nested block — keep as `SetNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_floatingip_associate_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_floatingip_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_network_v2
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `segments` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_port_secgroup_associate_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_portforwarding_v2
*Flags: Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_qos_bandwidth_limit_rule_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_qos_dscp_marking_rule_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_qos_minimum_bandwidth_rule_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_qos_policy_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_quota_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_rbac_policy_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_router_interface_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_router_route_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_router_routes_v2
*Flags: custom Importer (composite-ID), nested Elem*

- [ ] **Block decision**: `route` nested block — keep as `SetNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_secgroup_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_segment_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_subnet_route_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_subnetpool_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_networking_trunk_v2
*Flags: Timeouts, nested Elem*

- [ ] **Block decision**: `sub_port` nested block — keep as `SetNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_objectstorage_account_v1
*Flags: custom Importer (composite-ID), DiffSuppressFunc*

- [ ] `DiffSuppressFunc` — analyse intent before converting.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_objectstorage_object_v1
*Flags: DiffSuppressFunc*

- [ ] `DiffSuppressFunc` — analyse intent before converting.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_objectstorage_tempurl_v1
*(No special flags)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_orchestration_stack_v1
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `outputs` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_sharedfilesystem_securityservice_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_sharedfilesystem_share_access_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_sharedfilesystem_share_v2
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `export_locations` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_sharedfilesystem_sharenetwork_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_vpnaas_endpoint_group_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_vpnaas_ike_policy_v2
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `lifetime` nested block — keep as `ListNestedBlock` with `SizeAtMost(1)`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_vpnaas_ipsec_policy_v2
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `lifetime` nested block — keep as `ListNestedBlock` with `SizeAtMost(1)`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_vpnaas_service_v2
*Flags: custom Importer (composite-ID), Timeouts*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_vpnaas_site_connection
*Flags: custom Importer (composite-ID), Timeouts, nested Elem*

- [ ] **Block decision**: `ipsecpolicy`, `ikepolicy` nested blocks — keep as `ListNestedBlock` with `SizeAtMost(1)`.
- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented; Timeouts wired up
- [ ] Tests pass green; Negative gate satisfied

---

### Resource: openstack_workflow_cron_trigger_v2
*Flags: custom Importer (composite-ID)*

- [ ] Tests written/updated and run *red*
- [ ] Schema converted; CRUD implemented; Import method implemented
- [ ] Tests pass green; Negative gate satisfied

---

## Data sources (64 total)

Data sources need schema + Read method migration. None have StateUpgraders or Importers. Several have nested Elem blocks requiring block-vs-attribute decisions.

### Data source: openstack_blockstorage_availability_zones_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_blockstorage_quotaset_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_blockstorage_snapshot_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_blockstorage_volume_v3
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: Nested Elem — keep as `ListNestedBlock` (backward-compat).
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_aggregate_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_availability_zones_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_flavor_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_hypervisor_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_instance_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `network` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_keypair_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_limits_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_quotaset_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_compute_servergroup_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `rules` nested block — keep as `ListNestedBlock` with `SizeAtMost(1)`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_containerinfra_cluster_v1
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_containerinfra_clustertemplate_v1
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_containerinfra_nodegroup_v1
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_dns_zone_share_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_dns_zone_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_fw_group_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_fw_policy_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_fw_rule_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_auth_scope_v3
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `roles`, `service_catalog` nested blocks — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_endpoint_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_group_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_project_ids_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_project_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_role_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_service_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_identity_user_v3
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_images_image_ids_v2
*Flags: ForceNew (11), Validators (4)*
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Validators translated; Tests pass green; Negative gate satisfied

### Data source: openstack_images_image_v2
*Flags: ForceNew (12), Validators (4)*
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Validators translated; Tests pass green; Negative gate satisfied

### Data source: openstack_keymanager_container_v1
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `secret_refs` nested block — keep as `SetNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_keymanager_secret_v1
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_lb_flavor_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_lb_flavorprofile_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_lb_listener_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `connection_limit` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_lb_loadbalancer_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: Nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_lb_member_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_lb_monitor_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: Nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_lb_pool_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `persistence` nested block — keep as `ListNestedBlock` with `SizeAtMost(1)`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_loadbalancer_flavor_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_addressscope_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_floatingip_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_network_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `subnets` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_port_ids_v2
*Flags: ForceNew (17), Validators (2)*
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Validators translated; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_port_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `fixed_ip`, `allowed_address_pairs` nested blocks — keep as blocks.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_qos_bandwidth_limit_rule_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_qos_dscp_marking_rule_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_qos_minimum_bandwidth_rule_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_qos_policy_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_quota_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_router_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `external_fixed_ip` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_secgroup_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_segment_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_subnet_ids_v2
*Flags: ForceNew (13), Validators (5)*
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Validators translated; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_subnet_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `allocation_pools` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_subnetpool_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_networking_trunk_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `sub_port` nested block — keep as `SetNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_sharedfilesystem_availability_zones_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_sharedfilesystem_share_v2
*Flags: nested Elem (block-vs-nested decision)*
- [ ] **Block decision**: `export_locations` nested block — keep as `ListNestedBlock`.
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_sharedfilesystem_sharenetwork_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_sharedfilesystem_snapshot_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_workflow_cron_trigger_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

### Data source: openstack_workflow_workflow_v2
- [ ] Tests written/updated and run *red*; Schema converted; `Read` implemented; Tests pass green; Negative gate satisfied

---

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
