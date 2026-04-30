# Migration checklist: terraform-provider-openstack/v3

**Audit artefact:** `audit_report.md` (2026-04-30)
**SDKv2 version migrating from:** v2.38.1
**Scale:** 109 resources, 64 data sources, 236 source files

---

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md`
- [ ] Team has confirmed scope (whole provider vs. incremental service-area batches)
- [ ] Decision: **protocol v6** (default for single-release migrations — required for framework-only attribute features; see `references/protocol-versions.md`)
- [ ] All files listed in "Needs manual review" in `audit_report.md` have been read end-to-end before any editing starts
- [ ] Baseline test run is green: `go test ./...` (non-ACC) passes with zero failures
- [ ] Step 2 data-consistency review completed — no silent SDKv2-demoted errors remain

---

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of the provider has sufficient test coverage and that all tests pass (`go test ./...`; `TF_ACC=1 go test ./...` with OpenStack creds)
- [ ] **2.** Review provider code for SDKv2 resource data consistency errors (pay special attention to resources with `CustomizeDiff` — `compute_instance_v2`, `images_image_v2`, `networking_bgp_peer_v2`)
- [ ] **3.** Serve the provider via the framework — swap `main.go` from `plugin.Serve` + `ProviderFunc` to `providerserver.NewProtocol6WithError` (see `references/protocol-versions.md`)
- [ ] **4.** Update the provider definition to use the framework (`provider.Provider` interface, `Metadata`, `Configure`, `Resources`, `DataSources`)
- [ ] **5.** Update the provider schema to use the framework (all attributes in `provider.go`)
- [ ] **6.** Update each resource and data source (work through per-resource checklist below)
- [ ] **7.** Update related tests to use the framework and **ensure they fail first** (TDD gate — `ProtoV6ProviderFactories`, swap SDKv2 helpers)
- [ ] **8.** Migrate each resource/data source implementation
- [ ] **9.** Verify that each resource's tests now pass
- [ ] **10.** Remove all remaining references to SDKv2 libraries from migrated files
- [ ] **11.** Verify that the full test suite continues to pass
- [ ] **12.** Release new provider version (changelog entry, protocol v6 note, minimum Terraform CLI version)

---

## Recommended migration order

Migrate in complexity order — simple resources first to establish the pattern, complex/high-risk last.

**Tier 1 — Simple (ForceNew only, passthrough importer, no nested blocks)**
Start here: establish the framework pattern, verify `verify_tests.sh` workflow, confirm protocol v6 smoke works.

**Tier 2 — Moderate (Timeouts, nested Elems, validators)**
Most of the provider falls here.

**Tier 3 — High (CustomizeDiff, StateFunc, DiffSuppressFunc, MaxItems:1 decisions)**
`compute_instance_v2`, `networking_port_v2`, `db_instance_v1`, `lb_pool_v2`, `images_image_v2`.

**Tier 4 — Special (StateUpgraders)**
`objectstorage_container_v1` — migrate last; single-step semantics require explicit care.

---

## Per-resource checklist

Mark each box only after the resource passes `verify_tests.sh --migrated-files <file>`.

Before editing any flagged resource, write a 3-line summary:
1. **Block decision** — which attributes with `MaxItems:1 + nested Elem` become blocks vs. nested attributes?
2. **State upgrade** — is there a `SchemaVersion > 0`? (only `objectstorage_container_v1`)
3. **Import shape** — passthrough or custom composite-ID parsing?

---

### Provider (provider.go)

- [ ] Tests updated and run red (step 7)
- [ ] `provider.Provider` interface implemented (`Metadata`, `Schema`, `Configure`, `Resources`, `DataSources`)
- [ ] `main.go` swapped to `providerserver.NewProtocol6WithError`
- [ ] `TestProvider` / `InternalValidate` equivalent passing
- [ ] Tests pass green
- [ ] Negative gate: file no longer imports `terraform-plugin-sdk/v2`

---

### Blockstorage service

#### openstack_blockstorage_volume_v3
- [ ] Pre-edit summary written (block decision: 2 nested Elems; import: passthrough; no state upgrade)
- [ ] Tests updated and run red
- [ ] Schema converted (2 nested Elem blocks, 13 ForceNew, Timeouts)
- [ ] CRUD methods implemented
- [ ] Validators translated (1)
- [ ] Plan modifiers (13 `RequiresReplace` from ForceNew)
- [ ] Timeouts wired (`references/timeouts.md`)
- [ ] Import method implemented (passthrough)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_attach_v3
- [ ] Tests updated and run red
- [ ] Schema converted (Timeouts)
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_type_v3
- [ ] Pre-edit summary written (import: custom StateContext — verify composite ID)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_type_access_v3
- [ ] Pre-edit summary written (import: custom StateContext — composite ID)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_qos_v3
- [ ] Pre-edit summary written (import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_qos_association_v3
- [ ] Pre-edit summary written (import: custom StateContext — composite ID)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_quotaset_v3
- [ ] Pre-edit summary written (import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Compute service

#### openstack_compute_aggregate_v2
- [ ] Pre-edit summary written (import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_flavor_v2
- [ ] Pre-edit summary written (import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_flavor_access_v2
- [ ] Pre-edit summary written (import: custom StateContext — composite ID)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_instance_v2 (TIER 3 — most complex)
- [ ] **Pre-edit summary written** (MaxItems:1 on `block_device`: keep as `ListNestedBlock`; 5 nested Elems: all blocks; no state upgrader; import: custom StateContext; CustomizeDiff → ModifyPlan; StateFunc on image_name → custom type or PlanModifier; DiffSuppressFunc → analyse and convert)
- [ ] Tests updated and run red
- [ ] Schema converted (33 ForceNew, 1 MaxItems:1, 5 nested Elems, 2 validators)
- [ ] CRUD methods implemented
- [ ] Validators translated (2)
- [ ] Plan modifiers (33 `RequiresReplace` from ForceNew)
- [ ] `ModifyPlan` implemented (replaces `CustomizeDiff`)
- [ ] `StateFunc` → custom type or PlanModifier
- [ ] `DiffSuppressFunc` intent analysed and converted
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_interface_attach_v2
- [ ] Pre-edit summary written (import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_keypair_v2
- [ ] Pre-edit summary written (import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Sensitive attribute (`private_key`)
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_servergroup_v2
- [ ] Pre-edit summary written (MaxItems:1 on `rules`: keep as `ListNestedBlock`; nested Elem; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_quotaset_v2
- [ ] Pre-edit summary written (import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_volume_attach_v2
- [ ] Pre-edit summary written (MaxItems:1: keep as `ListNestedBlock`; nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Container Infra service

#### openstack_containerinfra_cluster_v1
- [ ] Pre-edit summary written (17 ForceNew; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_clustertemplate_v1
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_nodegroup_v1
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Database service

#### openstack_db_instance_v1 (TIER 3 — second most complex)
- [ ] **Pre-edit summary written** (MaxItems:1 on `datastore`: keep as `ListNestedBlock`; 4 nested Elems; Timeouts; no custom importer)
- [ ] Tests updated and run red
- [ ] Schema converted (21 ForceNew, 1 MaxItems:1, 4 nested Elems)
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_db_configuration_v1
- [ ] Pre-edit summary written (nested Elem; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_db_database_v1
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_db_user_v1
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### DNS service

#### openstack_dns_zone_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_recordset_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_zone_share_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_transfer_request_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_transfer_accept_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_quota_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Firewall service

#### openstack_fw_group_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_policy_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_rule_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Identity service

#### openstack_identity_endpoint_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (passthrough)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_project_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext — verify composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_role_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_role_assignment_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (passthrough)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_inherit_role_assignment_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_service_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_user_v3
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_user_membership_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_group_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_application_credential_v3
- [ ] Pre-edit summary written (12 ForceNew, nested Elem, 2 validators; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Sensitive attributes handled
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_ec2_credential_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Sensitive attributes handled
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_registered_limit_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_limit_v3
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Images service

#### openstack_images_image_v2 (TIER 3)
- [ ] **Pre-edit summary written** (7 ForceNew, 6 validators, CustomizeDiff → ModifyPlan for metadata JSON; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (6)
- [ ] `ModifyPlan` implemented (replaces CustomizeDiff)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_images_image_access_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_images_image_access_accept_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Key Manager service

#### openstack_keymanager_secret_v1
- [ ] Pre-edit summary written (10 ForceNew, 4 validators, DiffSuppressFunc: whitespace trim on `payload` → PlanModifier or custom type; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (4)
- [ ] DiffSuppressFunc → PlanModifier (whitespace normalization)
- [ ] Sensitive attributes handled
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_keymanager_container_v1
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_keymanager_order_v1
- [ ] **Pre-edit summary written** (MaxItems:1 on `meta`: keep as `ListNestedBlock`; nested Elem; 9 ForceNew, 3 validators; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (3)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Load Balancer service

#### openstack_lb_loadbalancer_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_listener_v2
- [ ] Pre-edit summary written (5 ForceNew, 4 validators, DiffSuppressFunc — analyse intent; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (4)
- [ ] DiffSuppressFunc → analyse and convert
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_pool_v2 (TIER 3)
- [ ] **Pre-edit summary written** (MaxItems:1 on `persistence`: keep as `ListNestedBlock`; nested Elem; 5 validators; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted (5 ForceNew, 1 MaxItems:1, 1 nested Elem, 5 validators)
- [ ] CRUD methods implemented
- [ ] Validators translated (5)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_member_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_members_v2
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (3)
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_monitor_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_l7policy_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_l7rule_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_flavor_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_flavorprofile_v2 (TIER 3)
- [ ] **Pre-edit summary written** (StateFunc: JSON normalization on `flavor_data` → custom type; DiffSuppressFunc: JSON equivalence → remove with custom type; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] `StateFunc` → custom JSON type
- [ ] `DiffSuppressFunc` removed (subsumed by custom type)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_quota_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Networking service

#### openstack_networking_network_v2
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnet_v2
- [ ] Pre-edit summary written (10 ForceNew, 4 validators, nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (4)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_port_v2 (TIER 3)
- [ ] **Pre-edit summary written** (MaxItems:1 on `binding`: keep as `ListNestedBlock`; 4 nested Elems; StateFunc: JSON normalization on `allowed_address_pairs.ip_address` → custom type; DiffSuppressFunc: JSON equivalence → remove; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted (7 ForceNew, 2 validators, 1 MaxItems:1, 4 nested Elems)
- [ ] CRUD methods implemented
- [ ] Validators translated (2)
- [ ] `StateFunc` → custom JSON normalization type
- [ ] `DiffSuppressFunc` removed
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_v2
- [ ] Pre-edit summary written (MaxItems:1 on `external_fixed_ip`: keep as `ListNestedBlock`; nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_interface_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_route_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_routes_v2
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_secgroup_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_secgroup_rule_v2 (TIER 3)
- [ ] **Pre-edit summary written** (12 ForceNew, 5 validators, StateFunc on CIDR normalization → custom type; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5)
- [ ] `StateFunc` → custom CIDR normalization type
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_floatingip_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_floatingip_associate_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_port_secgroup_associate_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnetpool_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_addressscope_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_trunk_v2
- [ ] Pre-edit summary written (nested Elem; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_rbac_policy_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_address_group_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnet_route_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_segment_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_portforwarding_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_policy_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_bandwidth_limit_rule_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_dscp_marking_rule_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_minimum_bandwidth_rule_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_quota_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_bgp_peer_v2
- [ ] **Pre-edit summary written** (5 ForceNew, 3 validators, CustomizeDiff → ModifyPlan; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (3)
- [ ] `ModifyPlan` implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_bgp_speaker_v2
- [ ] Pre-edit summary written (nested Elem; DiffSuppressFunc — analyse intent; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] DiffSuppressFunc analysed and converted
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Object Storage service

#### openstack_objectstorage_account_v1
- [ ] Pre-edit summary written (DiffSuppressFunc — analyse intent; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] DiffSuppressFunc analysed and converted
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_objectstorage_container_v1 (TIER 4 — state upgrade)
- [ ] **Pre-edit summary written** (MaxItems:1 on `versioning`: keep as `ListNestedBlock`; **StateUpgraders V0→V1**: implement `ResourceWithUpgradeState`, single-step semantics; nested Elem; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted (3 ForceNew, 1 validator, 1 MaxItems:1, 1 nested Elem)
- [ ] CRUD methods implemented
- [ ] **`ResourceWithUpgradeState` implemented** — V0 upgrader produces target V1 schema state directly (no chaining). Read `references/state-upgrade.md` before touching this.
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied (also: `migrate_resource_openstack_objectstorage_container_v1.go` migrated)

#### openstack_objectstorage_object_v1
- [ ] Pre-edit summary written (DiffSuppressFunc — analyse intent)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] DiffSuppressFunc analysed and converted
- [ ] Sensitive attributes handled
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_objectstorage_tempurl_v1
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Orchestration service

#### openstack_orchestration_stack_v1
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext — composite ID; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (custom StateContext)
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Shared Filesystem service

#### openstack_sharedfilesystem_securityservice_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_sharenetwork_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_share_v2
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_share_access_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented (custom StateContext — composite ID)
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### VPNaaS service

#### openstack_vpnaas_endpoint_group_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_ike_policy_v2
- [ ] Pre-edit summary written (nested Elem — phase1 negotiation mode block; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_ipsec_policy_v2
- [ ] Pre-edit summary written (nested Elem — phase2 negotiation mode block; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_service_v2
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_site_connection_v2
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### BGPVPN service

#### openstack_bgpvpn_v2
- [ ] Pre-edit summary written (import: custom StateContext; Timeouts)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_bgpvpn_network_associate_v2
- [ ] Pre-edit summary written (import: custom StateContext — verify if composite ID is encoded in the state)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_bgpvpn_router_associate_v2
- [ ] Pre-edit summary written (import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_bgpvpn_port_associate_v2
- [ ] Pre-edit summary written (nested Elem; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Workflow service

#### openstack_workflow_cron_trigger_v2
- [ ] Pre-edit summary written (import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### TaaS service

#### openstack_taas_tap_mirror_v2
- [ ] Pre-edit summary written (MaxItems:1: keep as `ListNestedBlock`; nested Elem; import: custom StateContext)
- [ ] Tests updated and run red
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

## Data source checklist

For brevity, data sources are listed in service-area groups. Each needs:
- [ ] Tests updated and run red
- [ ] Schema converted (`references/data-sources.md`)
- [ ] `Read` method implemented
- [ ] Validators translated (where present)
- [ ] Tests pass green
- [ ] Negative gate satisfied

**Data sources with nested Elem (require block-vs-nested decision):**
- `data_source_openstack_blockstorage_volume_v3`
- `data_source_openstack_compute_instance_v2`
- `data_source_openstack_compute_servergroup_v2`
- `data_source_openstack_identity_auth_scope_v3`
- `data_source_openstack_keymanager_container_v1`
- `data_source_openstack_lb_listener_v2`
- `data_source_openstack_lb_loadbalancer_v2`
- `data_source_openstack_lb_monitor_v2`
- `data_source_openstack_lb_pool_v2`
- `data_source_openstack_networking_network_v2`
- `data_source_openstack_networking_port_v2`
- `data_source_openstack_networking_router_v2`
- `data_source_openstack_networking_subnet_v2`
- `data_source_openstack_networking_trunk_v2`
- `data_source_openstack_sharedfilesystem_share_v2`

**Data sources with high validator counts (subnet_ids_v2: 5, port_ids_v2: 2):**
Write block decision before editing; these have flat schemas but dense validators.

All 64 data sources are listed in `audit_report.md` per-service-area inventory.

---

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] `go build ./...` is clean
- [ ] `go vet ./...` is clean
- [ ] Full suite green: `go test ./...`
- [ ] (If creds available) `TF_ACC=1 go test ./...` green
- [ ] Changelog entry: protocol v6 bump, minimum Terraform CLI version (≥ 1.0 for protocol v6)
- [ ] Version bump in module path (already `/v3` — confirm if a v4 bump is required for the protocol change)
- [ ] `verify_tests.sh` run with `--migrated-files` covering all migrated files, negative gate exit 0
