# Migration checklist: terraform-provider-openstack/v3

**Audit artefact:** `audit_report.md`    **Date:** 2026-04-30

---

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md`
- [ ] Team has confirmed scope (whole provider in one release? by service group?)
- [ ] Decision: protocol **v6** (recommended default for single-release migration — see `references/protocol-versions.md`)
- [ ] All files listed in audit "Needs manual review" have been read end-to-end
- [ ] SDKv2 baseline is green: `go test ./...` passes before any edits are made
- [ ] Step 2 (data-consistency review) scheduled — SDKv2 silently demotes consistency errors to warnings; the framework surfaces them as hard errors. Fix in SDKv2 form first.

---

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of the provider has sufficient test coverage and that all tests pass.
- [ ] **2.** Review provider code for SDKv2 resource data consistency errors (pay particular attention to `openstack_compute_instance_v2`, `openstack_networking_port_v2`, and any resource with `DiffSuppressFunc`).
- [ ] **3.** Serve the provider via the framework — swap `main.go` from `plugin.Serve` to `providerserver.NewProtocol6WithError` (protocol v6 chosen). See `references/protocol-versions.md`.
- [ ] **4.** Update the provider definition to use the framework (`provider.Provider`, `Metadata`, `Resources`, `DataSources`, `Configure`). See `references/provider.md`.
- [ ] **5.** Update the provider schema to use the framework. See `references/schema.md`.
- [ ] **6.** Update each resource, data source, and other Terraform feature to use the framework. (Per-resource rows below.)
- [ ] **7.** Update related tests to use the framework and ensure tests **fail** first. *(TDD gate: write tests red before migrating code.)* See `references/testing.md`.
- [ ] **8.** Migrate each resource or data source.
- [ ] **9.** Verify that related tests now pass.
- [ ] **10.** Remove any remaining references to SDKv2 libraries.
- [ ] **11.** Verify all tests continue to pass.
- [ ] **12.** Release a new version of the provider.

---

## Global pre-migration decisions

| Decision | Recommendation | Notes |
|---|---|---|
| Protocol version | **v6** | Required for some framework-only attribute features; default for single-release. |
| `MaxItems:1` + nested `Elem` | Keep as **ListNestedBlock** with `listvalidator.SizeAtMost(1)` | This is a mature provider with existing practitioner configs — no major version bump expected. Default to backward-compat block form. Per-resource exceptions noted below. |
| `MinItems > 0` blocks | Keep as `ListNestedBlock` (true repeating blocks — no `MaxItems:1` ambiguity) | 7 occurrences. |
| `ForceNew` | Replace with `stringplanmodifier.RequiresReplace()` / `int64planmodifier.RequiresReplace()` etc. | 637 occurrences — the most common mechanical change. |
| `ValidateFunc` / `ValidateDiagFunc` | Migrate to `Validators: []validator.String{...}` using `terraform-plugin-framework-validators` | 140 occurrences. |
| `StateFunc` | Implement custom type (`basetypes.StringValuable` etc.) | 4 occurrences. Read `references/state-and-types.md`. |
| `DiffSuppressFunc` | Analyse intent per file; becomes custom type or plan modifier logic | 8 occurrences. No mechanical translation. |
| `CustomizeDiff` | Becomes `ModifyPlan` on the resource type | 3 resources. |
| `Timeouts` | Add `terraform-plugin-framework-timeouts` package | 68 occurrences. See `references/timeouts.md`. |
| `Sensitive: true` | Becomes `Sensitive: true` in framework (same field name, different package) | 20 occurrences. |
| `StateUpgraders` | `openstack_objectstorage_container_v1` only — implement `ResourceWithUpgradeState` (single-step) | 1 resource. See `references/state-upgrade.md`. |
| `d.Get` / `d.Set` string paths | Replace with typed `Plan.Get` / `State.Set` into model structs | Pervasive across all resources. |

---

## Per-resource checklist

Repeat the TDD gate at step 7 for **every** resource: write/update tests first, run them **red**, then migrate the code. Mark each box only after `verify_tests.sh --migrated-files <file>` exits 0.

---

### TIER 1 — Straightforward resources (pilot candidates)

These have no nested blocks, passthrough importers, and no special patterns. Migrate these first to establish the team's pattern.

---

#### openstack_identity_role_v3

- [ ] Pre-migration 3-line summary written (block decision / state upgrade / import shape)
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`) — passthrough
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate: file no longer imports `terraform-plugin-sdk/v2`

#### openstack_identity_service_v3

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Plan modifiers + defaults
- [ ] Import method implemented — passthrough
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_group_v3

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Plan modifiers + defaults
- [ ] Import method implemented — passthrough
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_ec2_credential_v3

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented — passthrough
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_zone_share_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented — passthrough
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_segment_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented — passthrough
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_floatingip_associate_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_port_secgroup_associate_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### TIER 1 — Straightforward data sources (pilot candidates)

Data sources have no CRUD write path, no importer, and are typically the simplest to migrate. Use these to validate the provider + test framework scaffolding.

#### openstack_blockstorage_availability_zones_v3 (data source)

- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted (`references/data-sources.md`)
- [ ] `Read` method implemented
- [ ] Validators translated
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_availability_zones_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_limits_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_role_v3 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_availability_zones_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### TIER 2 — Moderate resources (timeouts + importers)

Resources with timeouts and/or importers but minimal nested blocks.

---

#### openstack_networking_secgroup_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Plan modifiers + defaults
- [ ] Timeouts wired (`references/timeouts.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_zone_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_loadbalancer_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_v3

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Timeouts wired
- [ ] Import method implemented — confirm: composite ID or passthrough?
- [ ] Nested Elem blocks reviewed (block-vs-nested-attribute decision documented)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_images_image_v2

- [ ] Pre-migration 3-line summary written (note: CustomizeDiff → ModifyPlan)
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (6 validators)
- [ ] Plan modifiers + defaults
- [ ] `CustomizeDiff` → `ModifyPlan` implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_share_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Nested Elem blocks reviewed
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_cluster_v1

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_clustertemplate_v1

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_nodegroup_v1

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_group_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_policy_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_rule_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (3 validators)
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_keymanager_secret_v1

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (4 validators)
- [ ] `DiffSuppressFunc` intent analysed — document replacement approach
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Sensitive attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_policy_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_bandwidth_limit_rule_v2

- [ ] Pre-migration 3-line summary written (composite importer: `<policy_id>/<rule_id>`)
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented — composite ID: `ImportStatePassthroughWithIdentity` or manual parse
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_dscp_marking_rule_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method — composite ID
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_minimum_bandwidth_rule_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method — composite ID
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_l7policy_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_l7rule_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_monitor_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5 validators)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_member_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_project_v3

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_application_credential_v3

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Nested Elem blocks reviewed (block-vs-nested decision for `access_rules`)
- [ ] Import method implemented
- [ ] Sensitive attributes handled
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_user_v3

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Nested Elem blocks reviewed
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_orchestration_stack_v1

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Nested Elem blocks reviewed
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_ike_policy_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5 validators)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Nested Elem block reviewed
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_ipsec_policy_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5 validators)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Nested Elem block reviewed
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_service_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_endpoint_group_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_site_connection_v2

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Nested Elem blocks reviewed
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### TIER 2 — Moderate data sources (nested blocks to decide)

#### openstack_blockstorage_volume_v3 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted (`references/data-sources.md`)
- [ ] Nested Elem blocks reviewed (block-vs-nested decision)
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnet_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] Nested Elem blocks reviewed
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_port_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] Nested Elem blocks reviewed
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] Nested Elem blocks reviewed
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_network_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] Nested Elem blocks reviewed
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_pool_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] Nested Elem blocks reviewed
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_listener_v2 (data source)

- [ ] Tests updated and run **red**
- [ ] Schema converted
- [ ] Nested Elem blocks reviewed
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### TIER 3 — Complex resources (special patterns — migrate after Tier 1/2 establishes conventions)

---

#### openstack_networking_port_v2 (**MaxItems:1 + StateFunc + DiffSuppressFunc**)

- [ ] Pre-migration 3-line summary written:
  - Block decision: `MaxItems:1` nested attributes — keep as `ListNestedBlock` (backward-compat)
  - State upgrade: none
  - Import shape: confirm passthrough vs. composite
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (2 validators)
- [ ] Plan modifiers + defaults
- [ ] `StateFunc` → custom type implemented (`references/state-and-types.md`)
- [ ] `DiffSuppressFunc` intent analysed and replaced
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_pool_v2 (**MaxItems:1 persistence block**)

- [ ] Pre-migration 3-line summary written:
  - Block decision: `persistence` MaxItems:1 block → keep as `ListNestedBlock` with `listvalidator.SizeAtMost(1)`
  - State upgrade: none
  - Import shape: passthrough
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5 validators)
- [ ] Plan modifiers + defaults
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_v2 (**MaxItems:1 nested block**)

- [ ] Pre-migration 3-line summary written:
  - Block decision: `external_gateway_info` MaxItems:1 → keep as `ListNestedBlock`
  - State upgrade: none
  - Import shape: confirm
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Nested Elem blocks reviewed
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_keymanager_order_v1 (**MaxItems:1 + nested block**)

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] MaxItems:1 block decision documented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_keymanager_container_v1

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Nested Elem blocks reviewed (includes `keymanager_v1.go` helper)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_listener_v2 (**DiffSuppressFunc**)

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (4 validators)
- [ ] `DiffSuppressFunc` intent analysed and replaced
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_flavorprofile_v2 (**StateFunc + DiffSuppressFunc**)

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] `StateFunc` → custom type implemented
- [ ] `DiffSuppressFunc` intent analysed and replaced
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_db_instance_v1 (**MaxItems:1 + 4 nested Elem**)

- [ ] Pre-migration 3-line summary written:
  - Block decisions: multiple `MaxItems:1` nested attributes — document each
  - State upgrade: none
  - Import shape: no importer in this file — check
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired
- [ ] Nested Elem blocks reviewed (4 nested Elem)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_bgp_peer_v2 (**CustomizeDiff**)

- [ ] Pre-migration 3-line summary written:
  - Block decision: none
  - State upgrade: none
  - Import shape: passthrough
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] `CustomizeDiff` → `ModifyPlan` implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_secgroup_rule_v2 (**StateFunc + 5 validators**)

- [ ] Pre-migration 3-line summary written
- [ ] Tests updated and run **red** (step 7)
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated (5 validators)
- [ ] `StateFunc` → custom type implemented
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### TIER 3 — Remaining resources (same pattern, apply established conventions)

For each of the following, apply the same per-resource block as above. All share the moderate-to-complex profile (timeouts + importer + validators). Group by service for team assignment.

**BgpVPN:**
- [ ] `openstack_bgpvpn_v2`
- [ ] `openstack_bgpvpn_network_associate_v2`
- [ ] `openstack_bgpvpn_port_associate_v2`
- [ ] `openstack_bgpvpn_router_associate_v2`

**BlockStorage:**
- [ ] `openstack_blockstorage_qos_v3`
- [ ] `openstack_blockstorage_qos_association_v3`
- [ ] `openstack_blockstorage_quotaset_v3`
- [ ] `openstack_blockstorage_volume_attach_v3`
- [ ] `openstack_blockstorage_volume_type_v3`
- [ ] `openstack_blockstorage_volume_type_access_v3`

**Compute:**
- [ ] `openstack_compute_aggregate_v2`
- [ ] `openstack_compute_flavor_v2`
- [ ] `openstack_compute_flavor_access_v2`
- [ ] `openstack_compute_interface_attach_v2`
- [ ] `openstack_compute_keypair_v2`
- [ ] `openstack_compute_quotaset_v2`
- [ ] `openstack_compute_servergroup_v2` — MaxItems:1 block decision required
- [ ] `openstack_compute_volume_attach_v2` — MaxItems:1 block decision required

**Database:**
- [ ] `openstack_db_configuration_v1`
- [ ] `openstack_db_database_v1`
- [ ] `openstack_db_user_v1`

**DNS:**
- [ ] `openstack_dns_quota_v2`
- [ ] `openstack_dns_recordset_v2`
- [ ] `openstack_dns_transfer_accept_v2`
- [ ] `openstack_dns_transfer_request_v2`

**Identity:**
- [ ] `openstack_identity_endpoint_v3`
- [ ] `openstack_identity_inherit_role_assignment_v3`
- [ ] `openstack_identity_limit_v3`
- [ ] `openstack_identity_registered_limit_v3`
- [ ] `openstack_identity_role_assignment_v3`
- [ ] `openstack_identity_user_membership_v3`

**Images:**
- [ ] `openstack_images_image_access_v2`
- [ ] `openstack_images_image_access_accept_v2`

**LB:**
- [ ] `openstack_lb_flavor_v2`
- [ ] `openstack_lb_members_v2`
- [ ] `openstack_lb_quota_v2`

**Networking:**
- [ ] `openstack_networking_address_group_v2`
- [ ] `openstack_networking_addressscope_v2`
- [ ] `openstack_networking_bgp_speaker_v2`
- [ ] `openstack_networking_floatingip_v2`
- [ ] `openstack_networking_network_v2`
- [ ] `openstack_networking_portforwarding_v2`
- [ ] `openstack_networking_quota_v2`
- [ ] `openstack_networking_rbac_policy_v2`
- [ ] `openstack_networking_router_interface_v2`
- [ ] `openstack_networking_router_route_v2`
- [ ] `openstack_networking_router_routes_v2`
- [ ] `openstack_networking_subnet_v2`
- [ ] `openstack_networking_subnet_route_v2`
- [ ] `openstack_networking_subnetpool_v2`
- [ ] `openstack_networking_trunk_v2`

**Object Storage:**
- [ ] `openstack_objectstorage_account_v1` — DiffSuppressFunc
- [ ] `openstack_objectstorage_object_v1` — DiffSuppressFunc
- [ ] `openstack_objectstorage_tempurl_v1`

**Shared Filesystem:**
- [ ] `openstack_sharedfilesystem_securityservice_v2`
- [ ] `openstack_sharedfilesystem_share_access_v2`
- [ ] `openstack_sharedfilesystem_sharenetwork_v2`

**TaaS:**
- [ ] `openstack_taas_tap_mirror_v2` — MaxItems:1 block decision required

**Workflow:**
- [ ] `openstack_workflow_cron_trigger_v2`

---

### TIER 4 — State upgrader (highest risk — plan separately)

#### openstack_objectstorage_container_v1 (**sole StateUpgrader**)

- [ ] Pre-migration 3-line summary written:
  - Block decision: `MaxItems:1` container metadata block — keep as `ListNestedBlock`
  - **State upgrade: SchemaVersion 1 — must implement `ResourceWithUpgradeState`**; V0→V1 migration logic currently in `migrate_resource_openstack_objectstorage_container_v1.go`. Framework requires a single-step upgrader (not chained); read `references/state-upgrade.md` before any edits.
  - Import shape: confirm (flagged as custom importer)
- [ ] Tests updated and run **red** (step 7) — test must exercise state upgrade path
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Plan modifiers + defaults
- [ ] **`ResourceWithUpgradeState` implemented** — single-step semantics verified against `migrate_resource_openstack_objectstorage_container_v1.go`
- [ ] Import method implemented
- [ ] Timeouts wired (if present)
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### TIER 3 — Highest complexity (migrate last)

#### openstack_compute_instance_v2 (**MaxItems:1 + CustomizeDiff + StateFunc + DiffSuppressFunc + 5 nested Elem + Timeouts**)

This is the single most complex resource in the provider. Recommend treating it as a mini-project with its own PR after all other resources are migrated.

- [ ] Pre-migration 3-line summary written:
  - Block decisions: `MaxItems:1` `scheduler_hints` and other nested attributes — document each one individually; default backward-compat block form unless breaking change is acceptable
  - State upgrade: none
  - Import shape: confirm (flagged as custom importer)
- [ ] Extra review: `customdiff.All(...)` — inventory all diff functions; each becomes a separate `ModifyPlan` sub-function or a custom type
- [ ] Tests updated and run **red** (step 7) — all `TestAccComputeInstance_*` variants
- [ ] Schema converted (33 ForceNew, 2 validators)
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Plan modifiers + defaults (33 `RequiresReplace` conversions)
- [ ] `CustomizeDiff` → `ModifyPlan` implemented (all functions from `customdiff.All`)
- [ ] `StateFunc` → custom type implemented
- [ ] `DiffSuppressFunc` intent analysed and replaced (multiple occurrences)
- [ ] Timeouts wired
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Remaining data sources (Tier 2/3)

Apply the standard data-source block (tests red → schema → Read → tests green → negative gate) to each:

**Compute:**
- [ ] `openstack_compute_aggregate_v2` (data)
- [ ] `openstack_compute_flavor_v2` (data)
- [ ] `openstack_compute_hypervisor_v2` (data)
- [ ] `openstack_compute_instance_v2` (data) — nested Elem
- [ ] `openstack_compute_keypair_v2` (data)
- [ ] `openstack_compute_quotaset_v2` (data)
- [ ] `openstack_compute_servergroup_v2` (data) — nested Elem

**BlockStorage:**
- [ ] `openstack_blockstorage_quotaset_v3` (data)
- [ ] `openstack_blockstorage_snapshot_v3` (data)

**ContainerInfra:**
- [ ] `openstack_containerinfra_cluster_v1` (data)
- [ ] `openstack_containerinfra_clustertemplate_v1` (data)
- [ ] `openstack_containerinfra_nodegroup_v1` (data)

**DNS:**
- [ ] `openstack_dns_zone_v2` (data)
- [ ] `openstack_dns_zone_share_v2` (data)

**Firewall:**
- [ ] `openstack_fw_group_v2` (data)
- [ ] `openstack_fw_policy_v2` (data)
- [ ] `openstack_fw_rule_v2` (data)

**Identity:**
- [ ] `openstack_identity_auth_scope_v3` (data) — nested Elem
- [ ] `openstack_identity_endpoint_v3` (data)
- [ ] `openstack_identity_group_v3` (data)
- [ ] `openstack_identity_project_ids_v3` (data)
- [ ] `openstack_identity_project_v3` (data)
- [ ] `openstack_identity_service_v3` (data)
- [ ] `openstack_identity_user_v3` (data)

**Images:**
- [ ] `openstack_images_image_ids_v2` (data)
- [ ] `openstack_images_image_v2` (data)

**Key Manager:**
- [ ] `openstack_keymanager_container_v1` (data) — nested Elem
- [ ] `openstack_keymanager_secret_v1` (data)

**LB:**
- [ ] `openstack_lb_flavor_v2` (data)
- [ ] `openstack_lb_flavorprofile_v2` (data)
- [ ] `openstack_lb_loadbalancer_v2` (data) — nested Elem
- [ ] `openstack_lb_member_v2` (data)
- [ ] `openstack_lb_monitor_v2` (data) — nested Elem
- [ ] `openstack_loadbalancer_flavor_v2` (data)

**Networking:**
- [ ] `openstack_networking_addressscope_v2` (data)
- [ ] `openstack_networking_floatingip_v2` (data)
- [ ] `openstack_networking_port_ids_v2` (data)
- [ ] `openstack_networking_qos_bandwidth_limit_rule_v2` (data)
- [ ] `openstack_networking_qos_dscp_marking_rule_v2` (data)
- [ ] `openstack_networking_qos_minimum_bandwidth_rule_v2` (data)
- [ ] `openstack_networking_qos_policy_v2` (data)
- [ ] `openstack_networking_quota_v2` (data)
- [ ] `openstack_networking_secgroup_v2` (data)
- [ ] `openstack_networking_segment_v2` (data)
- [ ] `openstack_networking_subnet_ids_v2` (data)
- [ ] `openstack_networking_subnetpool_v2` (data)
- [ ] `openstack_networking_trunk_v2` (data) — nested Elem

**Shared Filesystem:**
- [ ] `openstack_sharedfilesystem_share_v2` (data) — nested Elem
- [ ] `openstack_sharedfilesystem_sharenetwork_v2` (data)
- [ ] `openstack_sharedfilesystem_snapshot_v2` (data)

**Workflow:**
- [ ] `openstack_workflow_cron_trigger_v2` (data)
- [ ] `openstack_workflow_workflow_v2` (data)

---

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` — `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...`
- [ ] Full acceptance suite green (if credentials available): `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions:
  - Protocol v6 bump
  - Minimum Terraform CLI version (v1.0+ for protocol v6)
  - Any MaxItems:1 blocks that were converted to single-nested attributes (if any — state-breaking)
- [ ] Version bump to reflect migration scope

---

## Key pitfalls to brief the team on

1. **`Default` is not a plan modifier.** Use `stringdefault.StaticString(...)` from the `defaults` package, not `PlanModifiers`.
2. **`ForceNew: true` → `stringplanmodifier.RequiresReplace()`** (or `int64planmodifier`, etc.). 637 occurrences — automate a search-and-replace pass per type.
3. **`d.Get("foo.0.bar")` string-path access is gone.** Replace with typed `Plan.Get` / `State.Get` into a model struct.
4. **State upgrader is single-step.** The V0→V1 objectstorage container upgrader must produce V1 state directly from V0 state — do not chain.
5. **SDKv2 demotes consistency errors silently; the framework does not.** Fix data consistency issues in SDKv2 form (step 2) before migrating.
6. **Do not rename user-facing attribute IDs.** Any change to attribute names or block names is a state-breaking change for practitioners.
7. **`Set` no longer needs a hash function.** Drop any `Set: schema.HashString` / custom hashers.
