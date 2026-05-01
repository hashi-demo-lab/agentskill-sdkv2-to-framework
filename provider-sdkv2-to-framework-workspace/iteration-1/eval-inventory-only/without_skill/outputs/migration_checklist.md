# SDKv2 → Terraform Plugin Framework: Phased Migration Checklist

**Provider:** terraform-provider-openstack
**Total resources to migrate:** 109 managed resources + 64 data sources = 173 total
**Estimated effort:** 12–18 months team effort (varies by resourcing and parallelism)

---

## Pre-Migration Decisions (Review Before Any Code Changes)

- [ ] Confirm Go version minimum (currently Go 1.25); framework requires 1.21+. Already satisfied.
- [ ] Decide on new internal package layout (`internal/provider/`, `internal/resources/`, etc.) vs staying in `openstack/`.
- [ ] Decide whether to keep a single flat package or split by service (recommended: split by service).
- [ ] Confirm testing strategy: all acceptance tests must continue passing throughout every phase.
- [ ] Set up a branch strategy (long-lived feature branches per service, or PRs per resource).
- [ ] Assign resource ownership by service area to team members.

---

## Phase 0: Foundation & Tooling (Week 1–3)

This phase has no user-facing changes. All work is infrastructure.

### 0.1 — Add Framework Dependencies

- [ ] Add `github.com/hashicorp/terraform-plugin-framework` to `go.mod`
- [ ] Add `github.com/hashicorp/terraform-plugin-framework-validators` to `go.mod`
- [ ] Add `github.com/hashicorp/terraform-plugin-mux` to `go.mod`
- [ ] Run `go mod tidy` and verify build succeeds
- [ ] Update `terraform-registry-manifest.json` protocol_versions to `["5.0", "6.0"]`

### 0.2 — Introduce Mux Server

- [ ] Create new `internal/provider/` package (or equivalent layout)
- [ ] Implement a framework `Provider` struct that implements `provider.Provider` (initially empty resource/data-source lists)
- [ ] Rewrite `main.go` to use `tfmux.NewSchemaServerFactory` combining the existing SDKv2 `plugin.Serve` server and the new framework `providerserver.NewProviderServer`
- [ ] Verify `go build` and `TF_ACC=1 go test` still pass with zero resources on framework side

### 0.3 — Establish Framework Provider Configure

- [ ] Port provider schema attributes (auth_url, region, password, etc.) to framework `Schema()` method
- [ ] Port `configureProvider` logic to framework `Configure(ctx, req, resp)` — reads `*Config` from SDKv2 side initially, then transitions to framework-native
- [ ] Write at least one framework unit test for provider configure
- [ ] Document how `GetRegion` will work in framework context (pass `*Config` directly; no `schema.ResourceData`)

### 0.4 — Shared Infrastructure for Framework Resources

- [ ] Create `internal/common/region.go` — framework-safe equivalent of `GetRegion`
- [ ] Create `internal/common/errors.go` — framework-safe equivalent of `CheckDeleted` using `diag.Diagnostics`
- [ ] Create `internal/common/config.go` — wrapper to extract `*Config` from framework request metadata
- [ ] Create `internal/common/tags.go` — framework-safe tag helpers (currently in `networking_v2_shared.go`)
- [ ] Write unit tests for all new common helpers

---

## Phase 1: Tier 1 Resources — Pilot Migration (Week 4–8)

Migrate the smallest, simplest resources to prove out the pattern and frameworks, and to train the team.

**Target resources (all Tier 1):**

| Resource | File | LoC |
|---|---|---|
| `openstack_identity_role_v3` | `resource_openstack_identity_role_v3.go` | 132 |
| `openstack_bgpvpn_network_associate_v2` | `resource_openstack_bgpvpn_network_associate_v2.go` | 126 |
| `openstack_bgpvpn_router_associate_v2` | `resource_openstack_bgpvpn_router_associate_v2.go` | 168 |
| `openstack_networking_rbac_policy_v2` | `resource_openstack_networking_rbac_policy_v2.go` | 157 |
| `openstack_networking_addressscope_v2` | `resource_openstack_networking_addressscope_v2.go` | 199 |
| `openstack_workflow_cron_trigger_v2` | `resource_openstack_workflow_cron_trigger_v2.go` | 150 |

**Per-resource checklist (repeat for each resource in this phase):**

- [ ] Create `internal/resources/<service>/<resource_name>/resource.go`
- [ ] Implement `resource.Resource` interface: `Metadata`, `Schema`, `Create`, `Read`, `Update`, `Delete`
- [ ] Implement `resource.ResourceWithImportState` if the resource had `Importer`
- [ ] Map all `schema.TypeString/Bool/Int` fields to framework `schema.StringAttribute` / `schema.BoolAttribute` / `schema.Int64Attribute`
- [ ] Replace `ForceNew: true` with `planmodifier.RequiresReplace()`
- [ ] Replace `ValidateFunc` with framework validators from `framework-validators` package
- [ ] Replace `d.Set(...)` / `d.Get(...)` with `resp.State.Set(ctx, &state)` / `req.State.Get(ctx, &state)`
- [ ] Port `CheckDeleted` pattern to `diag.Diagnostics` with `resp.State.RemoveResource(ctx)`
- [ ] Port `GetRegion` to use the new common helper
- [ ] Register the new framework resource in the framework `Provider.Resources()` list
- [ ] Remove the SDKv2 resource from `provider.go` `ResourcesMap` (mux will no longer see it)
- [ ] Run acceptance tests for the resource and confirm all pass
- [ ] Peer review with at least one other team member

**Phase 1 sign-off:**
- [ ] All 6 pilot resources migrated and acceptance-tested
- [ ] Framework pattern documented in `CONTRIBUTING.md` or equivalent
- [ ] No regression in other resources' acceptance tests

---

## Phase 2: Simple Data Sources (Week 9–13)

Migrate Tier 1 and Tier 2 data sources before tackling complex resources. Data sources are simpler (no Create/Update/Delete) and build confidence with `datasource.DataSource`.

**Target services (in suggested order):**

- [ ] `identity` data sources (8): role, project, project_ids, user, auth_scope, endpoint, service, group
- [ ] `dns` data sources (2): zone_v2, zone_share_v2
- [ ] `fw` data sources (3): group_v2, policy_v2, rule_v2
- [ ] `workflow` data sources (2): cron_trigger_v2, workflow_v2
- [ ] `blockstorage` data sources (4): availability_zones_v3, snapshot_v3, volume_v3, quotaset_v3
- [ ] `images` data sources (2): image_v2, image_ids_v2
- [ ] `keymanager` data sources (2): secret_v1, container_v1
- [ ] `sharedfilesystem` data sources (4)
- [ ] `containerinfra` data sources (3)

**Per data-source checklist:**

- [ ] Create `internal/datasources/<service>/<datasource_name>/datasource.go`
- [ ] Implement `datasource.DataSource` interface: `Metadata`, `Schema`, `Read`
- [ ] Map all attributes to framework types
- [ ] Handle `TypeSet` with `schema.HashString` as `types.Set` of `types.String`
- [ ] Remove SDKv2 data source from `provider.go` `DataSourcesMap`
- [ ] Register in framework `Provider.DataSources()`
- [ ] Run acceptance tests for the data source

**Phase 2 sign-off:**
- [ ] All targeted data sources migrated
- [ ] `provider.go` DataSourcesMap now contains only remaining un-migrated data sources

---

## Phase 3: Tier 2 Resources — Bulk Migration (Week 14–26)

Migrate the mid-complexity resources service by service. Work in parallel across services.

**Suggested order (by complexity and inter-dependency):**

### 3.1 — DNS Resources (6 resources)
- [ ] `openstack_dns_quota_v2`
- [ ] `openstack_dns_recordset_v2`
- [ ] `openstack_dns_zone_v2`
- [ ] `openstack_dns_zone_share_v2`
- [ ] `openstack_dns_transfer_request_v2`
- [ ] `openstack_dns_transfer_accept_v2`

### 3.2 — Firewall Resources (3 resources)
- [ ] `openstack_fw_rule_v2`
- [ ] `openstack_fw_policy_v2`
- [ ] `openstack_fw_group_v2` (TypeSet with HashString; standard)

### 3.3 — Identity Resources (13 resources)
- [ ] `openstack_identity_endpoint_v3`
- [ ] `openstack_identity_service_v3`
- [ ] `openstack_identity_group_v3`
- [ ] `openstack_identity_user_v3` (Sensitive field)
- [ ] `openstack_identity_project_v3` (TypeSet HashString)
- [ ] `openstack_identity_role_assignment_v3`
- [ ] `openstack_identity_inherit_role_assignment_v3`
- [ ] `openstack_identity_user_membership_v3`
- [ ] `openstack_identity_application_credential_v3` (Sensitive)
- [ ] `openstack_identity_ec2_credential_v3` (Sensitive)
- [ ] `openstack_identity_registered_limit_v3`
- [ ] `openstack_identity_limit_v3`

### 3.4 — BGP VPN Resources (4 resources)
- [ ] `openstack_bgpvpn_v2` (multiple TypeSet with HashString)
- [ ] `openstack_bgpvpn_port_associate_v2`

### 3.5 — VPNaaS Resources (5 resources)
- [ ] `openstack_vpnaas_ike_policy_v2`
- [ ] `openstack_vpnaas_ipsec_policy_v2`
- [ ] `openstack_vpnaas_endpoint_group_v2` (TypeSet HashString)
- [ ] `openstack_vpnaas_service_v2`
- [ ] `openstack_vpnaas_site_connection`

### 3.6 — Shared Filesystem Resources (4 resources)
- [ ] `openstack_sharedfilesystem_securityservice_v2` (Sensitive)
- [ ] `openstack_sharedfilesystem_sharenetwork_v2` (TypeSet HashString)
- [ ] `openstack_sharedfilesystem_share_v2`
- [ ] `openstack_sharedfilesystem_share_access_v2` (Sensitive)

### 3.7 — Key Manager Resources (3 resources)
- [ ] `openstack_keymanager_container_v1`
- [ ] `openstack_keymanager_order_v1`
- [ ] `openstack_keymanager_secret_v1` (DiffSuppressFunc — needs custom PlanModifier)

### 3.8 — Database Resources (4 resources)
- [ ] `openstack_db_configuration_v1`
- [ ] `openstack_db_database_v1`
- [ ] `openstack_db_user_v1` (TypeSet HashString; Sensitive)
- [ ] `openstack_db_instance_v1` (Sensitive; complex nested block)

### 3.9 — Images Resources (3 resources)
- [ ] `openstack_images_image_access_v2`
- [ ] `openstack_images_image_access_accept_v2`
- [ ] `openstack_images_image_v2` (CustomizeDiff → ModifyPlan; DiffSuppressFunc → PlanModifier)

### 3.10 — Object Storage Resources (4 resources)
- [ ] `openstack_objectstorage_tempurl_v1` (Sensitive)
- [ ] `openstack_objectstorage_account_v1` (DiffSuppressFunc)
- [ ] `openstack_objectstorage_object_v1` (DiffSuppressFunc)
- [ ] `openstack_objectstorage_container_v1` (StateUpgrader — requires `resource.StateUpgraders` in framework)

### 3.11 — Block Storage Resources (7 resources)
- [ ] `openstack_blockstorage_quotaset_v3`
- [ ] `openstack_blockstorage_qos_v3`
- [ ] `openstack_blockstorage_qos_association_v3`
- [ ] `openstack_blockstorage_volume_type_v3`
- [ ] `openstack_blockstorage_volume_type_access_v3`
- [ ] `openstack_blockstorage_volume_attach_v3` (Sensitive)
- [ ] `openstack_blockstorage_volume_v3` (custom hash functions → ListNestedAttribute)

### 3.12 — Workflow / Orchestration (2 resources)
- [ ] `openstack_workflow_workflow_v2` (if it becomes a resource; currently data source only)
- [ ] `openstack_orchestration_stack_v1`
- [ ] `openstack_taas_tap_mirror_v2` (AtLeastOneOf)

**Per-resource checklist for Phase 3 (in addition to Phase 1 checklist):**

- [ ] For `TypeSet` with `schema.HashString`: convert to `types.Set` attribute with element type `types.StringType`
- [ ] For `TypeSet` with custom hash function: convert to `ListNestedAttribute` or `SetNestedAttribute` and remove hash function — document any state migration implications
- [ ] For `DiffSuppressFunc`: implement a custom `planmodifier` in `internal/common/planmodifiers/` and add unit tests
- [ ] For `CustomizeDiff`: implement `ModifyPlan` on the resource struct
- [ ] For `StateFunc`: normalize the value during Create/Read rather than at plan time where possible; otherwise use a PlanModifier
- [ ] For `Sensitive: true`: use `schema.SensitiveAttribute` modifier
- [ ] For `Timeouts`: add `resource.WithTimeouts()` and define timeout defaults
- [ ] For `StateUpgrader` (`objectstorage_container_v1`): implement `resource.StateUpgraders` interface — this is the only instance

**Phase 3 sign-off:**
- [ ] All Tier 2 resources migrated and acceptance-tested
- [ ] `provider.go` ResourcesMap contains only Tier 3 resources
- [ ] SDKv2 no longer needed for any migrated resource

---

## Phase 4: Tier 3 Resources — High-Complexity Migration (Week 27–42)

These resources require the most careful handling. Allocate senior engineers.

### 4.1 — Container Infra Resources (3 resources)
- [ ] `openstack_containerinfra_clustertemplate_v1`
- [ ] `openstack_containerinfra_nodegroup_v1`
- [ ] `openstack_containerinfra_cluster_v1` (Sensitive field; complex nested schema)

### 4.2 — Load Balancer Resources (11 resources)
- [ ] `openstack_lb_flavor_v2`
- [ ] `openstack_lb_flavorprofile_v2` (DiffSuppressFunc)
- [ ] `openstack_lb_quota_v2`
- [ ] `openstack_lb_monitor_v2`
- [ ] `openstack_lb_pool_v2` (AtLeastOneOf; TypeSet HashString)
- [ ] `openstack_lb_member_v2` (TypeSet HashString)
- [ ] `openstack_lb_members_v2`
- [ ] `openstack_lb_l7rule_v2`
- [ ] `openstack_lb_l7policy_v2`
- [ ] `openstack_lb_listener_v2` (DiffSuppressFunc; TypeSet HashString; AtLeastOneOf)
- [ ] `openstack_lb_loadbalancer_v2` (AtLeastOneOf; multiple TypeSet HashString)
- [ ] Migrate `lb_v2_shared.go` polling helpers to framework-safe versions

### 4.3 — Compute Resources (9 resources)
- [ ] `openstack_compute_keypair_v2` (Sensitive)
- [ ] `openstack_compute_aggregate_v2`
- [ ] `openstack_compute_servergroup_v2`
- [ ] `openstack_compute_flavor_v2`
- [ ] `openstack_compute_flavor_access_v2`
- [ ] `openstack_compute_quotaset_v2`
- [ ] `openstack_compute_volume_attach_v2`
- [ ] `openstack_compute_interface_attach_v2`
- [ ] `openstack_compute_instance_v2` — **highest complexity**: 1758 lines; CustomizeDiff; DiffSuppressFunc; StateFunc (SHA1 hash for user_data); custom TypeSet hash functions (`resourceComputeSchedulerHintsHash`, `resourceComputeInstancePersonalityHash`); large number of nested blocks

### 4.4 — Networking Resources (27 resources)
Migrate in dependency order (simpler resources first):

- [ ] `openstack_networking_segment_v2`
- [ ] `openstack_networking_router_route_v2`
- [ ] `openstack_networking_router_routes_v2`
- [ ] `openstack_networking_subnet_route_v2`
- [ ] `openstack_networking_floatingip_associate_v2`
- [ ] `openstack_networking_address_group_v2`
- [ ] `openstack_networking_addressscope_v2` (already done in Phase 1)
- [ ] `openstack_networking_subnetpool_v2`
- [ ] `openstack_networking_secgroup_v2`
- [ ] `openstack_networking_secgroup_rule_v2`
- [ ] `openstack_networking_qos_policy_v2`
- [ ] `openstack_networking_qos_bandwidth_limit_rule_v2`
- [ ] `openstack_networking_qos_dscp_marking_rule_v2`
- [ ] `openstack_networking_qos_minimum_bandwidth_rule_v2`
- [ ] `openstack_networking_quota_v2`
- [ ] `openstack_networking_bgp_speaker_v2`
- [ ] `openstack_networking_bgp_peer_v2` (CustomizeDiff; Sensitive)
- [ ] `openstack_networking_rbac_policy_v2` (already done in Phase 1)
- [ ] `openstack_networking_trunk_v2`
- [ ] `openstack_networking_portforwarding_v2`
- [ ] `openstack_networking_floatingip_v2`
- [ ] `openstack_networking_subnet_v2` (large; many nested attributes)
- [ ] `openstack_networking_router_interface_v2`
- [ ] `openstack_networking_router_v2`
- [ ] `openstack_networking_network_v2`
- [ ] `openstack_networking_port_secgroup_associate_v2` (TypeSet HashString)
- [ ] `openstack_networking_port_v2` (DiffSuppressFunc; custom TypeSet hash; large shared helpers)
- [ ] Migrate `networking_port_v2.go`, `networking_network_v2.go`, `networking_v2_shared.go` helpers

**Phase 4 sign-off:**
- [ ] All 109 managed resources migrated
- [ ] `provider.go` ResourcesMap is empty (can be removed)
- [ ] All 64 data sources migrated (DataSourcesMap is empty)
- [ ] SDKv2 import removed from `main.go`

---

## Phase 5: Remove SDKv2 and Clean Up (Week 43–46)

- [ ] Remove `github.com/hashicorp/terraform-plugin-sdk/v2` from `go.mod` (will only remain as transitive dep if needed)
- [ ] Remove `github.com/hashicorp/terraform-plugin-mux` once only one server remains
- [ ] Rewrite `main.go` to use only `providerserver.Serve`
- [ ] Remove the now-empty `openstack/provider.go` SDKv2 `Provider()` function (or retain it as a thin wrapper if mux is still needed)
- [ ] Update `provider_test.go` to use `providerserver.NewProviderServer`
- [ ] Update `terraform-registry-manifest.json` to `["6.0"]` only
- [ ] Run full acceptance test suite
- [ ] Update `CHANGELOG.md` with migration notice
- [ ] Update `README.md` and documentation index if necessary
- [ ] Bump module major version if breaking schema changes were introduced
- [ ] Tag release

---

## Cross-Cutting Concerns (All Phases)

### State Compatibility
- [ ] For every resource, verify that the framework-generated state JSON is compatible with SDKv2-generated state (field names, types). Framework uses `null` instead of `""` for unset strings in some cases.
- [ ] Where state shape changes (e.g., removing custom TypeSet hash), write and register `StateUpgrader` in the framework resource.

### Sensitive Data
- [ ] Audit all `Sensitive: true` fields (19 identified); confirm `schema.SensitiveAttribute` is applied in framework equivalents. Framework requires explicit sensitive marking; it is not inherited automatically.

### ValidateFunc → Validators
- [ ] 63 files use `ValidateFunc`; audit all for equivalents in `terraform-plugin-framework-validators`:
  - `validation.StringInSlice` → `stringvalidator.OneOf`
  - `validation.IntBetween` → `int64validator.Between`
  - `validation.StringIsJSON` → `stringvalidator.IsJSON`
  - Custom `ValidateFunc` → implement `validator.String` or `validator.Int64`

### Custom Plan Modifiers
- [ ] Centralize all `DiffSuppressFunc` replacements in `internal/common/planmodifiers/`
- [ ] Write unit tests for each custom plan modifier

### Documentation
- [ ] All resource docs in `docs/resources/` and `docs/data-sources/` should be regenerated using `tfplugindocs` after migration of each resource (framework generates docs differently)

### CI/CD
- [ ] Ensure CI runs `go vet ./...` and `golangci-lint` after each phase
- [ ] Add a CI check that verifies no SDKv2 resource is accidentally re-added to `ResourcesMap` after it has been migrated
