# Migration plan: terraform-provider-openstack (v3)

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md` (this run)
- [ ] User has confirmed scope (assumed **whole provider** — confirm before editing; see `notes.md`)
- [ ] Decision: protocol **v6** (default for single-release migrations — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end (140+ files flagged in audit)
- [ ] SDKv2 baseline is green (`go test ./...`) before any edits — required by workflow step 1

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
- [ ] **2.** Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework.
- [ ] **3.** Serve your provider via the framework. (`main.go` swap; protocol v6 chosen.) See `references/protocol-versions.md`.
- [ ] **4.** Update the provider definition to use the framework. See `references/provider.md`.
- [ ] **5.** Update the provider schema to use the framework. See `references/schema.md`.
- [ ] **6.** Update each of the provider's resources, data sources, and other Terraform features to use the framework.
- [ ] **7.** Update related tests to use the framework, and ensure that the tests fail. *(TDD gate — tests fail first, then migrate. See `references/testing.md`.)*
- [ ] **8.** Migrate the resource or data source.
- [ ] **9.** Verify that related tests now pass.
- [ ] **10.** Remove any remaining references to SDKv2 libraries.
- [ ] **11.** Verify that all of your tests continue to pass.
- [ ] **12.** Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

## Phasing (recommended migration order)

The audit flagged 1 StateUpgrader, 11 `MaxItems:1` block-vs-attribute decisions, ~101 custom Importers (composite-ID parsing), and 68 Timeouts blocks. This dictates a **risk-ascending** migration order: simple leaf resources first, then mid-complexity, then the hot files (`compute_instance`, `db_instance`, `networking_port`, `images_image`).

**Phase 0 — Pre-flight (steps 1–2).** Green baseline + data-consistency review. No code edits to convert SDKv2 → framework yet.

**Phase 1 — Provider scaffolding (steps 3–5).** `main.go` swap to framework + provider definition + provider schema. Run `verify_tests.sh` after.

**Phase 2 — Simple data sources first.** Data sources with no nested blocks, no validators, no composite imports. Lowest risk; builds team familiarity. Examples: `availability_zones_*`, `compute_limits_v2`, `*_quotaset_*` data sources.

**Phase 3 — Simple resources (passthrough importer, no nested blocks, no state upgraders).** Examples: `dns_quota_v2`, `compute_quotaset_v2`, `lb_quota_v2`, `networking_quota_v2`.

**Phase 4 — Resources with custom Importer (composite-ID parsing).** ~101 files. Most do simple `id1/id2` splits; framework handles via `ResourceWithImportState.ImportState` (or modern `ResourceWithIdentity` — see `references/identity.md`). Group by service area (identity, networking, lb, dns, fw, vpnaas, sharedfilesystem, blockstorage, compute, containerinfra, db, keymanager, bgpvpn, images, objectstorage, orchestration, workflow, taas).

**Phase 5 — Resources with nested blocks / `MaxItems:1` (block-vs-attribute decision).** 11 files. Apply the SKILL.md decision rule per file. Default = keep as block (Output A) unless the team has agreed a major-version bump. Public examples in docs likely exist for `compute_instance.network`, `db_instance.datastore`, `networking_port.fixed_ip`, etc. — keep these as blocks.

**Phase 6 — Resources with `CustomizeDiff` / `StateFunc` / `DiffSuppressFunc`.** 15 files (analyse intent — they may become `ModifyPlan`, custom types, or be dropped if the framework auto-handles).

**Phase 7 — The one resource with `StateUpgraders`/`SchemaVersion`.** `resource_openstack_objectstorage_container_v1.go`. Read `references/state-upgrade.md` first — chained → single-step semantics is the most error-prone part of the whole migration. Hold this until the team is confident.

**Phase 8 — The hot files** (highest complexity score from audit): `compute_instance_v2`, `db_instance_v1`, `networking_port_v2`, `networking_secgroup_rule_v2`, `images_image_v2`. Migrate last so the team has worked through the patterns. Each will need its own review session.

**Phase 9 — Steps 10–12.** Remove all `terraform-plugin-sdk/v2` imports, run `go mod tidy`, full suite green, changelog entry mentioning protocol bump, major-version release.

## Per-resource checklist

For each resource/data source: ProviderFactories → ProtoV6ProviderFactories in tests, schema + CRUD migrated, audit-flagged hooks (state upgrader / MaxItems:1 / importer / timeouts / sensitive) handled when present. Mark each row only after `verify_tests.sh --migrated-files <file>` exits 0 and the negative gate (no `terraform-plugin-sdk/v2` import in the file) passes.

### Resources flagged for manual review

The following resources have audit-flagged hooks. Each row lists the flags, the references to read, and the 3-line "think before editing" summary the skill requires (block decision / state upgrade / import shape).

#### resource_openstack_objectstorage_container_v1 — **HIGHEST RISK**

- Flags: `MaxItems:1` (1) · `StateUpgraders/SchemaVersion` (1) · custom Importer · nested `Elem &Resource` · `ConflictsWith/ExactlyOneOf`
- References: `references/state-upgrade.md`, `references/blocks.md`, `references/import.md`, `references/validators.md`
- Think before editing:
  - Block decision: TBD — `versioning` block uses `MaxItems:1` with nested `Elem`. Likely production usage; default = keep as block.
  - State upgrade: **YES** — only resource in the provider with one. `migrate_resource_openstack_objectstorage_container_v1.go` exists. Convert chained upgraders to framework single-step `PriorSchema` form.
  - Import shape: TBD — read importer body.
- [ ] Tests written/updated and run *red* (workflow step 7 — quote failing output)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks handled
- [ ] Tests pass green; negative gate satisfied

#### resource_openstack_compute_instance_v2 — **HIGHEST COMPLEXITY**

- Flags: 33 ForceNew · 2 Validators · `MaxItems:1` (1) · 5 nested Elems · custom Importer · Timeouts · `CustomizeDiff` · `StateFunc` · `DiffSuppressFunc` · `ConflictsWith`
- References: read **all** per-element refs.
- Think before editing:
  - Block decision: `network`, `block_device`, `personality`, `scheduler_hints`, `vendor_options` — these are practitioner-facing blocks with massive in-the-wild usage. Keep as blocks (Output A).
  - State upgrade: none.
  - Import shape: composite-ID — read importer body before migrating.
- [ ] Tests written/updated and run *red*
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks handled
- [ ] Tests pass green; negative gate satisfied

#### resource_openstack_db_instance_v1

- Flags: 21 ForceNew · `MaxItems:1` (1) · 4 nested Elems · Timeouts · `ConflictsWith`
- Think before editing: `datastore`, `network`, `database`, `user` blocks — keep as blocks (production usage). No state upgrade. No custom importer flagged.
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_networking_port_v2

- Flags: 7 ForceNew · 2 Validators · `MaxItems:1` (1) · 4 nested Elems · custom Importer · Timeouts · `StateFunc` · `DiffSuppressFunc` · `ConflictsWith`
- Think before editing: `fixed_ip`, `allowed_address_pairs`, `extra_dhcp_option`, `binding` — keep as blocks. `StateFunc` likely IP normalisation → custom type (`references/state-and-types.md`). `DiffSuppressFunc` — analyse intent.
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_images_image_v2

- Flags: 7 ForceNew · 6 Validators · custom Importer · Timeouts · `CustomizeDiff` · `ConflictsWith`
- Think before editing: `CustomizeDiff` → `ModifyPlan` (`references/resources.md`). No nested blocks flagged.
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_networking_secgroup_rule_v2

- Flags: 12 ForceNew · 5 Validators · custom Importer · Timeouts · `StateFunc` · `ConflictsWith`
- Think before editing: `StateFunc` likely CIDR/protocol normalisation → custom type.
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_lb_pool_v2

- Flags: 5 ForceNew · 5 Validators · `MaxItems:1` (1) · 1 nested Elem · custom Importer · Timeouts · `ConflictsWith`
- Think before editing: `persistence` block (`MaxItems:1`) — likely production usage; keep as block.
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_keymanager_order_v1

- Flags: 9 ForceNew · 3 Validators · `MaxItems:1` (1) · 1 nested Elem · custom Importer · Timeouts
- Think before editing: `meta` block (`MaxItems:1`); read for usage before deciding.
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_compute_servergroup_v2

- Flags: `MaxItems:1` (1) · custom Importer · 1 nested Elem
- Think before editing: server-group `policies` shape — read first.
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_compute_volume_attach_v2

- Flags: `MaxItems:1` (1) · custom Importer · Timeouts · 1 nested Elem
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_taas_tap_mirror_v2

- Flags: `MaxItems:1` (1) · custom Importer · 1 nested Elem · `ConflictsWith`
- [ ] Tests *red* / migrated / green / negative gate

#### resource_openstack_networking_router_v2

- Flags: `MaxItems:1` (1) · custom Importer · Timeouts · 1 nested Elem · `ConflictsWith`
- Think before editing: `vendor_options` or similar — read.
- [ ] Tests *red* / migrated / green / negative gate

#### Resources with custom Importer + Timeouts (no nested blocks)

Standard composite-ID pattern; group-migrate by service area. Each row: tests red → migrate → green → negative gate.

- [ ] resource_openstack_bgpvpn_v2
- [ ] resource_openstack_blockstorage_quotaset_v3
- [ ] resource_openstack_blockstorage_volume_attach_v3 *(Timeouts only — no custom importer)*
- [ ] resource_openstack_compute_aggregate_v2
- [ ] resource_openstack_compute_interface_attach_v2 *(also `ConflictsWith`)*
- [ ] resource_openstack_compute_quotaset_v2
- [ ] resource_openstack_containerinfra_cluster_v1
- [ ] resource_openstack_containerinfra_clustertemplate_v1
- [ ] resource_openstack_containerinfra_nodegroup_v1
- [ ] resource_openstack_db_configuration_v1 *(also nested Elem)*
- [ ] resource_openstack_db_database_v1
- [ ] resource_openstack_db_user_v1
- [ ] resource_openstack_dns_quota_v2
- [ ] resource_openstack_dns_recordset_v2
- [ ] resource_openstack_dns_transfer_accept_v2
- [ ] resource_openstack_dns_transfer_request_v2
- [ ] resource_openstack_dns_zone_v2
- [ ] resource_openstack_fw_group_v2 *(also `ConflictsWith`)*
- [ ] resource_openstack_fw_policy_v2 *(also `ConflictsWith`)*
- [ ] resource_openstack_fw_rule_v2 *(also `ConflictsWith`)*
- [ ] resource_openstack_keymanager_container_v1 *(also nested Elem)*
- [ ] resource_openstack_keymanager_secret_v1 *(also `DiffSuppressFunc`)*
- [ ] resource_openstack_lb_flavor_v2
- [ ] resource_openstack_lb_flavorprofile_v2 *(also `StateFunc`, `DiffSuppressFunc`)*
- [ ] resource_openstack_lb_l7policy_v2 *(also `ConflictsWith`)*
- [ ] resource_openstack_lb_l7rule_v2
- [ ] resource_openstack_lb_listener_v2 *(also `DiffSuppressFunc`, `ConflictsWith`)*
- [ ] resource_openstack_lb_loadbalancer_v2 *(also `ConflictsWith`)*
- [ ] resource_openstack_lb_member_v2
- [ ] resource_openstack_lb_members_v2 *(also nested Elem)*
- [ ] resource_openstack_lb_monitor_v2
- [ ] resource_openstack_lb_quota_v2
- [ ] resource_openstack_networking_address_group_v2
- [ ] resource_openstack_networking_addressscope_v2
- [ ] resource_openstack_networking_bgp_peer_v2 *(also `CustomizeDiff`)*
- [ ] resource_openstack_networking_bgp_speaker_v2 *(also nested Elem)*
- [ ] resource_openstack_networking_floatingip_v2 *(also `ConflictsWith`)*
- [ ] resource_openstack_networking_network_v2 *(also nested Elem)*
- [ ] resource_openstack_networking_portforwarding_v2 *(Timeouts only)*
- [ ] resource_openstack_networking_qos_bandwidth_limit_rule_v2
- [ ] resource_openstack_networking_qos_dscp_marking_rule_v2
- [ ] resource_openstack_networking_qos_minimum_bandwidth_rule_v2
- [ ] resource_openstack_networking_qos_policy_v2
- [ ] resource_openstack_networking_quota_v2
- [ ] resource_openstack_networking_router_interface_v2
- [ ] resource_openstack_networking_secgroup_v2
- [ ] resource_openstack_networking_subnet_v2 *(also nested Elem, `ConflictsWith`)*
- [ ] resource_openstack_networking_subnetpool_v2
- [ ] resource_openstack_networking_trunk_v2 *(Timeouts only, also nested Elem)*
- [ ] resource_openstack_orchestration_stack_v1 *(also nested Elem)*
- [ ] resource_openstack_sharedfilesystem_securityservice_v2
- [ ] resource_openstack_sharedfilesystem_share_access_v2
- [ ] resource_openstack_sharedfilesystem_share_v2 *(also nested Elem)*
- [ ] resource_openstack_sharedfilesystem_sharenetwork_v2
- [ ] resource_openstack_vpnaas_endpoint_group_v2
- [ ] resource_openstack_vpnaas_ike_policy_v2 *(also nested Elem)*
- [ ] resource_openstack_vpnaas_ipsec_policy_v2 *(also nested Elem)*
- [ ] resource_openstack_vpnaas_service_v2
- [ ] resource_openstack_vpnaas_site_connection *(also nested Elem)*
- [ ] resource_openstack_blockstorage_volume_v3 *(also nested Elem, `ConflictsWith`)*

#### Resources with custom Importer (no Timeouts)

- [ ] resource_openstack_bgpvpn_network_associate_v2
- [ ] resource_openstack_bgpvpn_port_associate_v2 *(also nested Elem)*
- [ ] resource_openstack_bgpvpn_router_associate_v2
- [ ] resource_openstack_blockstorage_qos_association_v3
- [ ] resource_openstack_blockstorage_qos_v3
- [ ] resource_openstack_blockstorage_volume_type_access_v3
- [ ] resource_openstack_blockstorage_volume_type_v3
- [ ] resource_openstack_compute_flavor_access_v2
- [ ] resource_openstack_compute_flavor_v2
- [ ] resource_openstack_compute_keypair_v2
- [ ] resource_openstack_dns_zone_share_v2
- [ ] resource_openstack_identity_application_credential_v3 *(also nested Elem)*
- [ ] resource_openstack_identity_ec2_credential_v3
- [ ] resource_openstack_identity_endpoint_v3
- [ ] resource_openstack_identity_group_v3
- [ ] resource_openstack_identity_inherit_role_assignment_v3 *(also `ConflictsWith`)*
- [ ] resource_openstack_identity_limit_v3
- [ ] resource_openstack_identity_project_v3
- [ ] resource_openstack_identity_registered_limit_v3
- [ ] resource_openstack_identity_role_assignment_v3 *(also `ConflictsWith`)*
- [ ] resource_openstack_identity_role_v3
- [ ] resource_openstack_identity_service_v3
- [ ] resource_openstack_identity_user_membership_v3
- [ ] resource_openstack_identity_user_v3 *(also nested Elem)*
- [ ] resource_openstack_images_image_access_accept_v2
- [ ] resource_openstack_images_image_access_v2
- [ ] resource_openstack_networking_floatingip_associate_v2
- [ ] resource_openstack_networking_port_secgroup_associate_v2
- [ ] resource_openstack_networking_rbac_policy_v2
- [ ] resource_openstack_networking_router_route_v2
- [ ] resource_openstack_networking_router_routes_v2 *(also nested Elem)*
- [ ] resource_openstack_networking_segment_v2
- [ ] resource_openstack_networking_subnet_route_v2
- [ ] resource_openstack_objectstorage_account_v1 *(also `DiffSuppressFunc`)*
- [ ] resource_openstack_workflow_cron_trigger_v2

#### Resources with no manual-review flags (simplest — Phase 3)

- [ ] resource_openstack_objectstorage_object_v1 *(has `DiffSuppressFunc` and `ConflictsWith`; no importer flagged)*
- [ ] resource_openstack_objectstorage_tempurl_v1

### Data sources

The audit flags 28 data sources for manual review (mostly `nested Elem` block decisions or `ConflictsWith` validator routing). Data sources are simpler — `Read`-only, no CRUD — but the schema work is identical.

#### Data sources flagged for manual review

- [ ] data_source_openstack_blockstorage_volume_v3 *(nested Elem)*
- [ ] data_source_openstack_compute_flavor_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_compute_instance_v2 *(nested Elem)*
- [ ] data_source_openstack_compute_servergroup_v2 *(nested Elem)*
- [ ] data_source_openstack_fw_group_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_fw_policy_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_fw_rule_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_identity_auth_scope_v3 *(nested Elem)*
- [ ] data_source_openstack_identity_project_ids_v3 *(`ConflictsWith`)*
- [ ] data_source_openstack_identity_project_v3 *(`ConflictsWith`)*
- [ ] data_source_openstack_images_image_ids_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_images_image_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_keymanager_container_v1 *(nested Elem)*
- [ ] data_source_openstack_lb_flavor_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_lb_flavorprofile_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_lb_listener_v2 *(nested Elem, `ConflictsWith`)*
- [ ] data_source_openstack_lb_loadbalancer_v2 *(nested Elem, `ConflictsWith`)*
- [ ] data_source_openstack_lb_member_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_lb_monitor_v2 *(nested Elem, `ConflictsWith`)*
- [ ] data_source_openstack_lb_pool_v2 *(nested Elem, `ConflictsWith`)*
- [ ] data_source_openstack_loadbalancer_flavor_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_networking_network_v2 *(nested Elem)*
- [ ] data_source_openstack_networking_port_v2 *(nested Elem)*
- [ ] data_source_openstack_networking_router_v2 *(nested Elem)*
- [ ] data_source_openstack_networking_subnet_ids_v2 *(`ConflictsWith`)*
- [ ] data_source_openstack_networking_subnet_v2 *(nested Elem)*
- [ ] data_source_openstack_networking_trunk_v2 *(nested Elem)*
- [ ] data_source_openstack_sharedfilesystem_share_v2 *(nested Elem)*

#### Data sources with no manual-review flags (simpler)

- [ ] data_source_openstack_blockstorage_availability_zones_v3
- [ ] data_source_openstack_blockstorage_quotaset_v3
- [ ] data_source_openstack_blockstorage_snapshot_v3
- [ ] data_source_openstack_compute_aggregate_v2
- [ ] data_source_openstack_compute_availability_zones_v2
- [ ] data_source_openstack_compute_hypervisor_v2
- [ ] data_source_openstack_compute_keypair_v2
- [ ] data_source_openstack_compute_limits_v2
- [ ] data_source_openstack_compute_quotaset_v2
- [ ] data_source_openstack_containerinfra_cluster_v1
- [ ] data_source_openstack_containerinfra_clustertemplate_v1
- [ ] data_source_openstack_containerinfra_nodegroup_v1
- [ ] data_source_openstack_dns_zone_share_v2
- [ ] data_source_openstack_dns_zone_v2
- [ ] data_source_openstack_identity_endpoint_v3
- [ ] data_source_openstack_identity_group_v3
- [ ] data_source_openstack_identity_role_v3
- [ ] data_source_openstack_identity_service_v3
- [ ] data_source_openstack_identity_user_v3
- [ ] data_source_openstack_keymanager_secret_v1
- [ ] data_source_openstack_networking_addressscope_v2
- [ ] data_source_openstack_networking_floatingip_v2
- [ ] data_source_openstack_networking_port_ids_v2
- [ ] data_source_openstack_networking_qos_bandwidth_limit_rule_v2
- [ ] data_source_openstack_networking_qos_dscp_marking_rule_v2
- [ ] data_source_openstack_networking_qos_minimum_bandwidth_rule_v2
- [ ] data_source_openstack_networking_qos_policy_v2
- [ ] data_source_openstack_networking_quota_v2
- [ ] data_source_openstack_networking_secgroup_v2
- [ ] data_source_openstack_networking_segment_v2
- [ ] data_source_openstack_networking_subnetpool_v2
- [ ] data_source_openstack_sharedfilesystem_availability_zones_v2
- [ ] data_source_openstack_sharedfilesystem_sharenetwork_v2
- [ ] data_source_openstack_sharedfilesystem_snapshot_v2
- [ ] data_source_openstack_workflow_cron_trigger_v2
- [ ] data_source_openstack_workflow_workflow_v2

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (v5 → v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field (provider currently at v3 → v4)
