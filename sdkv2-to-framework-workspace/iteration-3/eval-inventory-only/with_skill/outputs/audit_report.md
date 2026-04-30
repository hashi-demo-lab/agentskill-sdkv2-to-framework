# SDKv2 → Framework Migration Audit

**Provider:** terraform-provider-openstack/v3
**Audited:** 2026-04-30
**SDKv2 version:** v2.38.1
**Audit tool:** semgrep (AST-aware)

---

## Summary

| Metric | Count |
|---|---|
| Go source files audited (excl. tests/vendor) | 236 |
| Resources registered | 109 |
| Data sources registered | 64 |
| `ForceNew: true` occurrences | 637 |
| `ValidateFunc` / `ValidateDiagFunc` | 140 |
| `DiffSuppressFunc` | 8 |
| `CustomizeDiff` | 3 |
| `StateFunc` | 4 |
| `Sensitive: true` | 20 |
| `Deprecated` attribute | 1 |
| `Importer` declarations | 101 |
| — of which are `ImportStatePassthroughContext` | 87 |
| — of which use a custom `StateContext` function | 14 |
| `Timeouts` blocks | 68 |
| `StateUpgraders` / `SchemaVersion` | 1 (objectstorage_container_v1) |
| `MaxItems: 1` + nested `Elem` (block decision) | 11 |
| Nested `Elem: &schema.Resource{}` (any block) | 71 |
| `MinItems > 0` (true repeating blocks) | 7 |

---

## Per-file findings (top 20 by complexity score)

Complexity score weights: ForceNew ×1, Validators ×2, StateUpgraders ×5, MaxItems:1 ×4, NestedElem ×2, Importer ×2, CustomizeDiff ×4, StateFunc ×3, DiffSuppressFunc ×2.

| File | ForceNew | Validators | StateUpgraders | MaxItems:1 | NestedElem | Importer | CustomizeDiff | StateFunc |
|------|---------:|-----------:|---------------:|----------:|-----------:|---------:|--------------:|----------:|
| openstack/resource_openstack_compute_instance_v2.go | 33 | 2 | 0 | 1 | 5 | 1 | 1 | 1 |
| openstack/resource_openstack_db_instance_v1.go | 21 | 0 | 0 | 1 | 4 | 0 | 0 | 0 |
| openstack/resource_openstack_networking_port_v2.go | 7 | 2 | 0 | 1 | 4 | 1 | 0 | 1 |
| openstack/resource_openstack_networking_secgroup_rule_v2.go | 12 | 5 | 0 | 0 | 0 | 1 | 0 | 1 |
| openstack/resource_openstack_images_image_v2.go | 7 | 6 | 0 | 0 | 0 | 1 | 1 | 0 |
| openstack/data_source_openstack_networking_subnet_ids_v2.go | 13 | 5 | 0 | 0 | 0 | 0 | 0 | 0 |
| openstack/resource_openstack_keymanager_order_v1.go | 9 | 3 | 0 | 1 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_pool_v2.go | 5 | 5 | 0 | 1 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_keymanager_secret_v1.go | 10 | 4 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_networking_subnet_v2.go | 10 | 4 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/data_source_openstack_networking_port_ids_v2.go | 17 | 2 | 0 | 0 | 0 | 0 | 0 | 0 |
| openstack/resource_openstack_blockstorage_volume_v3.go | 13 | 1 | 0 | 0 | 2 | 1 | 0 | 0 |
| openstack/data_source_openstack_images_image_v2.go | 12 | 4 | 0 | 0 | 0 | 0 | 0 | 0 |
| openstack/resource_openstack_identity_application_credential_v3.go | 12 | 2 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/data_source_openstack_images_image_ids_v2.go | 11 | 4 | 0 | 0 | 0 | 0 | 0 | 0 |
| openstack/resource_openstack_containerinfra_cluster_v1.go | 17 | 0 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_objectstorage_container_v1.go | 3 | 1 | 1 | 1 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_taas_tap_mirror_v2.go | 8 | 1 | 0 | 1 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_listener_v2.go | 5 | 4 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_networking_bgp_peer_v2.go | 5 | 3 | 0 | 0 | 0 | 1 | 1 | 0 |

---

## Pattern inventory by service area

### Blockstorage (7 resources, 4 data sources)
- Resources: `qos_association_v3`, `qos_v3`, `quotaset_v3`, `volume_v3`, `volume_attach_v3`, `volume_type_access_v3`, `volume_type_v3`
- Data sources: `availability_zones_v3`, `snapshot_v3`, `volume_v3`, `quotaset_v3`
- Key patterns: `Timeouts` on volume_v3 and quotaset_v3; nested `Elem` in volume_v3 (2 occurrences); composite-ID importers on qos_association_v3 and volume_type_access_v3

### Compute (10 resources, 9 data sources)
- Resources: `aggregate_v2`, `flavor_v2`, `flavor_access_v2`, `instance_v2`, `interface_attach_v2`, `keypair_v2`, `servergroup_v2`, `quotaset_v2`, `volume_attach_v2`
- Data sources: `aggregate_v2`, `availability_zones_v2`, `instance_v2`, `flavor_v2`, `hypervisor_v2`, `servergroup_v2`, `keypair_v2`, `quotaset_v2`, `limits_v2`
- Key patterns: `compute_instance_v2` is the most complex file in the provider — 33 ForceNew, 1 MaxItems:1, 5 nested Elems, `CustomizeDiff`, `StateFunc`; `servergroup_v2` has MaxItems:1 + nested Elem

### Container Infra (3 resources, 3 data sources)
- Resources: `cluster_v1`, `clustertemplate_v1`, `nodegroup_v1`
- Key patterns: all have `Timeouts`; `cluster_v1` has 17 ForceNew attributes

### Database (4 resources)
- Resources: `configuration_v1`, `database_v1`, `instance_v1`, `user_v1`
- Key patterns: `db_instance_v1` has 21 ForceNew and 4 nested Elems plus MaxItems:1 (second most complex)

### DNS (6 resources, 2 data sources)
- Resources: `quota_v2`, `recordset_v2`, `transfer_accept_v2`, `transfer_request_v2`, `zone_share_v2`, `zone_v2`
- Key patterns: all with Timeouts; `transfer_accept_v2` and `transfer_request_v2` have custom StateContext importers

### Firewall (3 resources, 3 data sources)
- Key patterns: all have Timeouts; `fw_group_v2` has both Timeout and custom Importer

### Identity (14 resources, 8 data sources)
- Resources: `endpoint_v3`, `project_v3`, `role_v3`, `role_assignment_v3`, `inherit_role_assignment_v3`, `service_v3`, `user_v3`, `user_membership_v3`, `group_v3`, `application_credential_v3`, `ec2_credential_v3`, `registered_limit_v3`, `limit_v3`
- Key patterns: all have custom `ImportStatePassthroughContext`; `user_v3` has nested Elem; `application_credential_v3` has nested Elem + 12 ForceNew

### Images (3 resources, 2 data sources)
- Key patterns: `images_image_v2` has CustomizeDiff (JSON metadata handling), StateFunc in data sources

### Key Manager (3 resources, 2 data sources)
- Resources: `container_v1`, `order_v1`, `secret_v1`
- Key patterns: `order_v1` has MaxItems:1; `secret_v1` has DiffSuppressFunc (whitespace trim on payload); `container_v1` has nested Elem

### Load Balancer (10 resources, 8 data sources)
- Resources: `flavor_v2`, `flavorprofile_v2`, `loadbalancer_v2`, `listener_v2`, `pool_v2`, `member_v2`, `members_v2`, `monitor_v2`, `l7policy_v2`, `l7rule_v2`, `quota_v2`
- Key patterns: `lb_pool_v2` has MaxItems:1 (persistence block) + nested Elem; `lb_flavorprofile_v2` has StateFunc (JSON normalization) + DiffSuppressFunc; all LB resources have Timeouts

### Networking (22 resources, 15 data sources)
- Key patterns: `networking_port_v2` (MaxItems:1 + nested Elem + StateFunc + DiffSuppressFunc — JSON normalization on `allowed_address_pairs`); `networking_router_v2` has MaxItems:1 + nested Elem; `networking_bgp_peer_v2` has CustomizeDiff

### Object Storage (4 resources)
- Key patterns: `objectstorage_container_v1` is the ONLY resource with StateUpgraders (V0→V1, schema version 1); has custom Importer + MaxItems:1

### Orchestration (1 resource)
- Key patterns: `orchestration_stack_v1` has Timeouts + nested Elem

### Shared Filesystem (4 resources, 4 data sources)
- Key patterns: all have Timeouts + custom Importer

### VPNaaS (5 resources)
- Key patterns: all have Timeouts + custom Importer; `ike_policy_v2` and `ipsec_policy_v2` have nested Elems (phase1/2 negotiation mode blocks)

### BGPVPN (4 resources)
- Key patterns: `bgpvpn_v2` has Timeouts; `bgpvpn_network_associate_v2`, `bgpvpn_router_associate_v2`, `bgpvpn_port_associate_v2` all have custom StateContext importers

### Workflow (1 resource, 2 data sources)
- Key patterns: `workflow_cron_trigger_v2` has custom Importer

### TaaS (1 resource)
- Key patterns: `taas_tap_mirror_v2` has MaxItems:1 + nested Elem + Importer

---

## Special-case patterns that require pre-edit review

### 1. The only `StateUpgraders` resource
**File:** `openstack/resource_openstack_objectstorage_container_v1.go`
**Migration note:** Single-step semantics required. The current V0→V1 upgrader chains through `resourceObjectStorageContainerStateUpgradeV0`. In the framework this becomes `ResourceWithUpgradeState` returning a map of version → upgrader function. The upgrader function must directly produce the current (V1) schema's state from the V0 state — no chaining. Read `references/state-upgrade.md` before editing.

### 2. `CustomizeDiff` resources (become `ModifyPlan`)
- `compute_instance_v2` — drives disk config / image ID consistency checks
- `images_image_v2` — metadata JSON normalization
- `networking_bgp_peer_v2` — BGP peer attribute consistency

Each `CustomizeDiff` must be reimplemented as a `resource.ResourceWithModifyPlan` method. The method receives a `ModifyPlanRequest` / `ModifyPlanResponse` instead of the SDKv2 `*schema.ResourceDiff`.

### 3. `StateFunc` resources (become custom types or `PlanModifier`)
- `networking_port_v2` — JSON normalization on `allowed_address_pairs.ip_address`
- `networking_secgroup_rule_v2` — CIDR normalization
- `lb_flavorprofile_v2` — JSON normalization on `flavor_data`
- `compute_instance_v2` — image name/ID normalization

Framework approach: implement `basetypes.StringValuable` (custom type) to embed the normalisation, or use a `PlanModifier`. JSON normalisation is the most common pattern here.

### 4. `DiffSuppressFunc` resources (analyse intent before converting)
- `keymanager_secret_v1` — whitespace trim (`strings.TrimSpace`)
- `lb_flavorprofile_v2` — JSON equivalence (`diffSuppressJSONObject`)
- `lb_listener_v2` — confirm what it suppresses
- `networking_port_v2` — JSON equivalence on allowed_address_pairs
- `objectstorage_account_v1` — confirm what it suppresses
- `objectstorage_object_v1` — confirm what it suppresses
- `networking_bgp_speaker_v2` — confirm what it suppresses (1 occurrence)

DiffSuppressFunc has no direct framework equivalent. Depending on intent: whitespace/normalization → `PlanModifier`; semantic equivalence (JSON) → custom type; content-addressable → custom type or validator.

### 5. `MaxItems: 1` nested blocks (block-vs-nested-attribute decision)
All are mature resources with existing practitioner configs; default decision is **block** (backward-compatible `ListNestedBlock` with `listvalidator.SizeAtMost(1)`).

| Resource | Attribute | Recommendation |
|---|---|---|
| `compute_instance_v2` | `block_device` sub-struct | Keep as ListNestedBlock |
| `compute_servergroup_v2` | `rules` | Keep as ListNestedBlock |
| `compute_volume_attach_v2` | inner block | Keep as ListNestedBlock |
| `db_instance_v1` | `datastore` | Keep as ListNestedBlock |
| `keymanager_order_v1` | `meta` | Keep as ListNestedBlock |
| `lb_pool_v2` | `persistence` | Keep as ListNestedBlock |
| `networking_port_v2` | `binding` | Keep as ListNestedBlock |
| `networking_router_v2` | `external_fixed_ip` | Confirm; likely ListNestedBlock |
| `objectstorage_container_v1` | `versioning` | Keep as ListNestedBlock |
| `taas_tap_mirror_v2` | inner block | Keep as ListNestedBlock |

### 6. Custom StateContext importers (composite ID parsing required)
14 resources use a custom `StateContext` function instead of `ImportStatePassthroughContext`. Each must be converted to a `ResourceWithImportState` method with explicit ID parsing logic.

Files:
- `resource_openstack_bgpvpn_network_associate_v2.go`
- `resource_openstack_bgpvpn_port_associate_v2.go`
- `resource_openstack_bgpvpn_router_associate_v2.go`
- `resource_openstack_bgpvpn_v2.go`
- `resource_openstack_blockstorage_qos_association_v3.go`
- `resource_openstack_blockstorage_qos_v3.go`
- `resource_openstack_blockstorage_quotaset_v3.go`
- `resource_openstack_blockstorage_volume_type_access_v3.go`
- `resource_openstack_blockstorage_volume_type_v3.go`
- `resource_openstack_blockstorage_volume_v3.go`
- `resource_openstack_compute_aggregate_v2.go`
- `resource_openstack_compute_flavor_access_v2.go`
- `resource_openstack_compute_flavor_v2.go`
- plus others (see full list in "Needs manual review" below)

---

## Files requiring manual review before editing

(127 files total — listed by signal type)

**StateUpgraders/SchemaVersion:**
- `openstack/resource_openstack_objectstorage_container_v1.go`
- `openstack/migrate_resource_openstack_objectstorage_container_v1.go`

**MaxItems:1 block decision:**
- `openstack/resource_openstack_compute_instance_v2.go`
- `openstack/resource_openstack_compute_servergroup_v2.go`
- `openstack/resource_openstack_compute_volume_attach_v2.go`
- `openstack/resource_openstack_db_instance_v1.go`
- `openstack/resource_openstack_keymanager_order_v1.go`
- `openstack/resource_openstack_lb_pool_v2.go`
- `openstack/resource_openstack_networking_port_v2.go`
- `openstack/resource_openstack_networking_router_v2.go`
- `openstack/resource_openstack_objectstorage_container_v1.go`
- `openstack/resource_openstack_taas_tap_mirror_v2.go`
- `openstack/migrate_resource_openstack_objectstorage_container_v1.go`

**CustomizeDiff → ModifyPlan:**
- `openstack/resource_openstack_compute_instance_v2.go`
- `openstack/resource_openstack_images_image_v2.go`
- `openstack/resource_openstack_networking_bgp_peer_v2.go`

**StateFunc → custom type:**
- `openstack/networking_port_v2.go` (shared helper)
- `openstack/resource_openstack_compute_instance_v2.go`
- `openstack/resource_openstack_lb_flavorprofile_v2.go`
- `openstack/resource_openstack_networking_port_v2.go`
- `openstack/resource_openstack_networking_secgroup_rule_v2.go`

**DiffSuppressFunc → analyse intent:**
- `openstack/resource_openstack_keymanager_secret_v1.go`
- `openstack/resource_openstack_lb_flavorprofile_v2.go`
- `openstack/resource_openstack_lb_listener_v2.go`
- `openstack/resource_openstack_networking_port_v2.go`
- `openstack/resource_openstack_networking_bgp_speaker_v2.go`
- `openstack/resource_openstack_objectstorage_account_v1.go`
- `openstack/resource_openstack_objectstorage_object_v1.go`

**Nested Elem &Resource (all block decisions):**
Full list produced by audit — 71 occurrences across ~35 files. See "Needs manual review" section of raw audit output. Key files: `compute_instance_v2`, `db_instance_v1`, `networking_port_v2`, `lb_members_v2`, `lb_pool_v2`, `networking_router_v2`, `networking_subnet_v2`, `objectstorage_container_v1`, `orchestration_stack_v1`, `vpnaas_ike_policy_v2`, `vpnaas_ipsec_policy_v2`, `vpnaas_site_connection`, `sharedfilesystem_share_v2`.

**Data sources with nested Elem:**
- `data_source_openstack_blockstorage_volume_v3.go`
- `data_source_openstack_compute_instance_v2.go`
- `data_source_openstack_compute_servergroup_v2.go`
- `data_source_openstack_identity_auth_scope_v3.go`
- `data_source_openstack_keymanager_container_v1.go`
- `data_source_openstack_lb_listener_v2.go`
- `data_source_openstack_lb_loadbalancer_v2.go`
- `data_source_openstack_lb_monitor_v2.go`
- `data_source_openstack_lb_pool_v2.go`
- `data_source_openstack_networking_network_v2.go`
- `data_source_openstack_networking_port_v2.go`
- `data_source_openstack_networking_router_v2.go`
- `data_source_openstack_networking_subnet_v2.go`
- `data_source_openstack_networking_trunk_v2.go`
- `data_source_openstack_sharedfilesystem_share_v2.go`
- `openstack/keymanager_v1.go` (shared helper)

---

## Next steps

1. Read every file listed under "Files requiring manual review" before proposing edits.
2. Populate `migration_checklist.md` from this audit (one block per resource/data source).
3. Confirm scope with the team before starting workflow step 1 (baseline test run).
