# Migration plan: terraform-provider-openstack

**Provider module:** github.com/terraform-provider-openstack/terraform-provider-openstack/v3
**SDKv2 version:** v2.38.1
**Audit date:** 2026-04-30
**Resources:** 105 | **Data sources:** 64 | **Total Go source files (non-test):** 236

---

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md`
- [ ] User has confirmed scope (whole provider? specific resources?)
- [ ] Decision: protocol v5 or v6 (default v6 — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end (see "Needs manual review" section of audit_report.md; 120+ files flagged)

---

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
- [ ] **2.** Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework.
      - Priority: `compute_instance_v2`, `images_image_v2`, `networking_port_v2` (high CustomizeDiff/DiffSuppressFunc/StateFunc density)
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

Repeat one block per resource. Mark each box only after the resource passes `verify_tests.sh --migrated-files <file>`.

> **Complexity legend:**
> `[HIGH]` = StateUpgraders, CustomizeDiff, StateFunc, or DiffSuppressFunc present; requires extra pre-migration reading.
> `[MED]` = Importer + Timeouts + nested Elem or MaxItems:1.
> `[STD]` = standard CRUD with Importer/Timeouts only.

---

### Phase 1 — Provider skeleton (do first, unlocks everything else)

#### openstack_provider (provider.go)

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`, `references/provider.md`)
- [ ] `Configure` method implemented
- [ ] `Resources` and `DataSources` lists populated
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Phase 2 — High-complexity resources (migrate first; contain gotchas that affect the rest)

#### openstack_compute_instance_v2 `[HIGH]` — ForceNew:42, CustomizeDiff, StateFunc, DiffSuppressFunc, 5×nested Elem, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (note: `Default` is *not* a plan modifier) (`references/plan-modifiers.md`)
- [ ] State upgraders translated, if applicable — single-step semantics (`references/state-upgrade.md`) — N/A
- [ ] CustomizeDiff converted to ModifyPlan (`references/plan-modifiers.md`)
- [ ] StateFunc converted to custom type or plan modifier (`references/state-and-types.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (block vs SingleNestedAttribute) (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_images_image_v2 `[HIGH]` — ValidateFunc:6, CustomizeDiff:4, DiffSuppressFunc, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] CustomizeDiff converted to ModifyPlan (`references/plan-modifiers.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_port_v2 `[HIGH]` — ForceNew:16, StateFunc, DiffSuppressFunc, 4×nested Elem, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] StateFunc converted to custom type or plan modifier (`references/state-and-types.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_objectstorage_container_v1 `[HIGH]` — StateUpgraders/SchemaVersion:1 (V0→V1 upgrade), MaxItems:1, nested Elem, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] **State upgrader translated — single-step semantics, not chained** (`references/state-upgrade.md`)
      - Upgrader file: `migrate_resource_openstack_objectstorage_container_v1.go` (V0 schema)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_bgp_peer_v2 `[HIGH]` — CustomizeDiff, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] CustomizeDiff converted to ModifyPlan (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_lb_flavorprofile_v2 `[HIGH]` — StateFunc, DiffSuppressFunc, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] StateFunc converted to custom type or plan modifier (`references/state-and-types.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_secgroup_rule_v2 `[HIGH]` — ValidateFunc:5, StateFunc, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] StateFunc converted to custom type or plan modifier (`references/state-and-types.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_keymanager_secret_v1 `[HIGH]` — ValidateFunc:4, DiffSuppressFunc, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Phase 3 — Medium-complexity resources (Importer + Timeouts + nested blocks)

#### openstack_db_instance_v1 `[MED]` — ForceNew:23, 4×nested Elem, MaxItems:1, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_containerinfra_clustertemplate_v1 `[MED]` — ForceNew:33, ValidateFunc:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_containerinfra_cluster_v1 `[MED]` — ForceNew:27, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Plan modifiers + defaults (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_subnet_v2 `[MED]` — ForceNew:17, ValidateFunc:4, nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_blockstorage_volume_v3 `[MED]` — ForceNew:16, 2×nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_router_v2 `[MED]` — nested Elem, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_network_v2 `[MED]` — nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_keymanager_container_v1 `[MED]` — nested Elem:2, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_keymanager_order_v1 `[MED]` — nested Elem, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_lb_pool_v2 `[MED]` — ValidateFunc:5, nested Elem, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_vpnaas_ike_policy_v2 `[MED]` — ValidateFunc:5, nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_vpnaas_ipsec_policy_v2 `[MED]` — ValidateFunc:5, nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_vpnaas_site_connection `[MED]` — nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_compute_servergroup_v2 `[MED]` — nested Elem, MaxItems:1×2, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_compute_volume_attach_v2 `[MED]` — nested Elem, MaxItems:1, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_db_configuration_v1 `[MED]` — nested Elem:2, MaxItems:1, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_taas_tap_mirror_v2 `[MED]` — nested Elem, MaxItems:1, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] MaxItems:1 blocks converted to SingleNestedAttribute (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_bgp_speaker_v2 `[MED]` — nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_trunk_v2 `[MED]` — nested Elem, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_lb_members_v2 `[MED]` — ValidateFunc:3, nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_bgpvpn_port_associate_v2 `[MED]` — nested Elem, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_networking_router_routes_v2 `[MED]` — nested Elem, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_identity_application_credential_v3 `[MED]` — nested Elem, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_identity_user_v3 `[MED]` — nested Elem, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_sharedfilesystem_share_v2 `[MED]` — nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_orchestration_stack_v1 `[MED]` — nested Elem, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Nested Elem &schema.Resource blocks evaluated (`references/blocks.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_lb_listener_v2 `[MED]` — ValidateFunc:4, DiffSuppressFunc, Importer, Timeouts

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Timeouts wired up (`references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_objectstorage_object_v1 `[MED]` — DiffSuppressFunc

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

#### openstack_objectstorage_account_v1 `[MED]` — DiffSuppressFunc, Importer

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] DiffSuppressFunc analysed and converted (`references/plan-modifiers.md`)
- [ ] Import method implemented (`references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

### Phase 4 — Standard resources (Importer + Timeouts, no nested blocks or special patterns)

Each entry below follows the standard resource checklist. Entries with Importer and/or Timeouts are noted.

#### openstack_networking_subnetpool_v2 `[STD]` — ForceNew:16, ValidateFunc:1, Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Plan modifiers + defaults
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_v2 — additional standard items (already covered in Phase 3 above)

#### openstack_containerinfra_nodegroup_v1 `[STD]` — ForceNew:14, Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Plan modifiers + defaults
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_secgroup_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_floatingip_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_quota_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_zone_v2 `[STD]` — ValidateFunc:1, Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_recordset_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_transfer_request_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_transfer_accept_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_quota_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_zone_share_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_loadbalancer_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_member_v2 `[STD]` — ValidateFunc:3, Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_monitor_v2 `[STD]` — ValidateFunc:5, Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Validators translated
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_l7policy_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_l7rule_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_flavor_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_quota_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_flavor_v2 `[STD]` — ForceNew:10, Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_aggregate_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_keypair_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Sensitive / write-only attributes handled
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_quotaset_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_interface_attach_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_flavor_access_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_attach_v3 `[STD]` — ForceNew:12, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_quotaset_v3 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_type_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_type_access_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_qos_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_qos_association_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_rule_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_policy_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_group_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_project_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_role_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_role_assignment_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_inherit_role_assignment_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_group_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_service_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_endpoint_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_ec2_credential_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Sensitive / write-only attributes handled
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_limit_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_registered_limit_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_user_membership_v3 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_images_image_access_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_images_image_access_accept_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_address_group_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_addressscope_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_floatingip_associate_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_port_secgroup_associate_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_portforwarding_v2 `[STD]` — Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_policy_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_bandwidth_limit_rule_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_dscp_marking_rule_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_minimum_bandwidth_rule_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_rbac_policy_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_interface_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_route_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnet_route_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_segment_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_objectstorage_tempurl_v1 `[STD]` — ForceNew:9

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_securityservice_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Sensitive / write-only attributes handled
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_share_access_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Sensitive / write-only attributes handled
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_sharenetwork_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_bgpvpn_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_bgpvpn_network_associate_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_bgpvpn_router_associate_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_endpoint_group_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_vpnaas_service_v2 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_db_database_v1 `[STD]` — Importer, Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_db_user_v1 `[STD]` — Timeouts

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Timeouts wired up
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_workflow_cron_trigger_v2 `[STD]` — Importer

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] CRUD methods implemented
- [ ] Import method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

### Phase 5 — Data sources (64 total)

For each data source the standard pattern applies: schema converted, `Read` method, validators, tests green, negative gate satisfied.

#### openstack_blockstorage_availability_zones_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted (`references/data-sources.md`)
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_quotaset_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_snapshot_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_blockstorage_volume_v3 `[STD]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_aggregate_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_availability_zones_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_flavor_v2 `[STD]` — ForceNew:11, nested Elem

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_hypervisor_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_instance_v2 `[MED]` — nested Elem:many (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_keypair_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_limits_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_quotaset_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_compute_servergroup_v2 `[STD]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_cluster_v1 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_clustertemplate_v1 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_containerinfra_nodegroup_v1 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_zone_share_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_dns_zone_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_group_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_policy_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_fw_rule_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_auth_scope_v3 `[MED]` — nested Elem:3 (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_endpoint_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_group_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_project_ids_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_project_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_role_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_service_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_identity_user_v3 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_images_image_ids_v2 `[STD]` — ValidateFunc:4

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Validators translated
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_images_image_v2 `[STD]` — ValidateFunc:4

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Validators translated
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_keymanager_container_v1 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_keymanager_secret_v1 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_flavor_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_flavorprofile_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_listener_v2 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_loadbalancer_v2 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_member_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_monitor_v2 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_lb_pool_v2 `[MED]` — nested Elem:4 (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_loadbalancer_flavor_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_addressscope_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_floatingip_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_network_v2 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_port_ids_v2 `[STD]` — ValidateFunc:2

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Validators translated
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_port_v2 `[MED]` — nested Elem:3 (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_bandwidth_limit_rule_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_dscp_marking_rule_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_minimum_bandwidth_rule_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_qos_policy_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_quota_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_router_v2 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_secgroup_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_segment_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnet_ids_v2 `[STD]` — ValidateFunc:5

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Validators translated
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnet_v2 `[MED]` — nested Elem:2 (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_subnetpool_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_networking_trunk_v2 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_availability_zones_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_share_v2 `[MED]` — nested Elem (manual review)

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Nested Elem blocks evaluated (`references/blocks.md`)
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_sharenetwork_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_sharedfilesystem_snapshot_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_workflow_cron_trigger_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

#### openstack_workflow_workflow_v2 `[STD]`

- [ ] Tests written/updated and run *red*
- [ ] Schema converted
- [ ] `Read` method implemented
- [ ] Tests pass green
- [ ] Negative gate satisfied

---

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
