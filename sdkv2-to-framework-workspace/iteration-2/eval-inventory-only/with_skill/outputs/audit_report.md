# SDKv2 → Framework Migration Audit

**Provider:** terraform-provider-openstack/v3    **Audited:** 2026-04-30    **SDKv2 version:** v2.38.1

## Summary

- Files audited: 236
- Resources registered: **109**
- Data sources registered: **64**
- ResourcesMap references: **1**
- DataSourcesMap references: **1**
- ForceNew: true: **637**
- ValidateFunc / ValidateDiagFunc: **140**
- DiffSuppressFunc: **8**
- CustomizeDiff: **3**
- StateFunc: **4**
- Sensitive: true: **20**
- Deprecated attribute: **1**
- Importer: **101**
- Timeouts: **68**
- StateUpgraders / SchemaVersion: **1** (only `openstack_objectstorage_container_v1`)
- MaxItems:1 + nested Elem (block decision): **11**
- Nested Elem &Resource (any block): **71**
- MinItems > 0 (true repeating block): **7**

## Per-file findings (top 50 by complexity)

| File | ForceNew | Validators | StateUpgraders | MaxItems:1 (block) | NestedElem | Importer | CustomizeDiff | StateFunc |
|------|---------:|-----------:|---------------:|-------------------:|-----------:|---------:|--------------:|----------:|
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
| openstack/resource_openstack_vpnaas_ike_policy_v2.go | 3 | 5 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_vpnaas_ipsec_policy_v2.go | 3 | 5 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_compute_servergroup_v2.go | 6 | 1 | 0 | 1 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_monitor_v2.go | 4 | 5 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_networking_router_v2.go | 6 | 0 | 0 | 1 | 2 | 1 | 0 | 0 |
| openstack/resource_openstack_bgpvpn_port_associate_v2.go | 4 | 3 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_blockstorage_volume_attach_v3.go | 12 | 1 | 0 | 0 | 0 | 0 | 0 | 0 |
| openstack/resource_openstack_compute_volume_attach_v2.go | 6 | 0 | 0 | 1 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_containerinfra_nodegroup_v1.go | 10 | 1 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_db_configuration_v1.go | 10 | 0 | 0 | 0 | 2 | 0 | 0 | 0 |
| openstack/resource_openstack_lb_member_v2.go | 6 | 3 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_sharedfilesystem_share_v2.go | 6 | 2 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/data_source_openstack_networking_subnet_v2.go | 3 | 3 | 0 | 0 | 2 | 0 | 0 | 0 |
| openstack/resource_openstack_objectstorage_tempurl_v1.go | 9 | 2 | 0 | 0 | 0 | 0 | 0 | 0 |
| openstack/resource_openstack_compute_flavor_v2.go | 10 | 0 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_members_v2.go | 2 | 3 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_networking_bgp_speaker_v2.go | 4 | 2 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/data_source_openstack_compute_flavor_v2.go | 11 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| openstack/resource_openstack_fw_rule_v2.go | 3 | 3 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_keymanager_container_v1.go | 3 | 1 | 0 | 0 | 2 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_l7policy_v2.go | 3 | 3 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_l7rule_v2.go | 3 | 3 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_loadbalancer_v2.go | 9 | 0 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_networking_floatingip_v2.go | 7 | 1 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_networking_network_v2.go | 5 | 1 | 0 | 0 | 1 | 1 | 0 | 0 |
| openstack/resource_openstack_sharedfilesystem_share_access_v2.go | 5 | 2 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/migrate_resource_openstack_objectstorage_container_v1.go | 2 | 1 | 0 | 1 | 1 | 0 | 0 | 0 |
| openstack/resource_openstack_dns_zone_v2.go | 6 | 1 | 0 | 0 | 0 | 1 | 0 | 0 |
| openstack/resource_openstack_lb_flavorprofile_v2.go | 1 | 1 | 0 | 0 | 0 | 1 | 0 | 1 |
| openstack/resource_openstack_networking_rbac_policy_v2.go | 4 | 2 | 0 | 0 | 0 | 1 | 0 | 0 |

## Registered Resources (109)

```
openstack_bgpvpn_network_associate_v2        openstack_bgpvpn_port_associate_v2
openstack_bgpvpn_router_associate_v2         openstack_bgpvpn_v2
openstack_blockstorage_qos_association_v3    openstack_blockstorage_qos_v3
openstack_blockstorage_quotaset_v3           openstack_blockstorage_volume_attach_v3
openstack_blockstorage_volume_type_access_v3 openstack_blockstorage_volume_type_v3
openstack_blockstorage_volume_v3             openstack_compute_aggregate_v2
openstack_compute_flavor_access_v2           openstack_compute_flavor_v2
openstack_compute_instance_v2                openstack_compute_interface_attach_v2
openstack_compute_keypair_v2                 openstack_compute_quotaset_v2
openstack_compute_servergroup_v2             openstack_compute_volume_attach_v2
openstack_containerinfra_cluster_v1          openstack_containerinfra_clustertemplate_v1
openstack_containerinfra_nodegroup_v1        openstack_db_configuration_v1
openstack_db_database_v1                     openstack_db_instance_v1
openstack_db_user_v1                         openstack_dns_quota_v2
openstack_dns_recordset_v2                   openstack_dns_transfer_accept_v2
openstack_dns_transfer_request_v2            openstack_dns_zone_share_v2
openstack_dns_zone_v2                        openstack_fw_group_v2
openstack_fw_policy_v2                       openstack_fw_rule_v2
openstack_identity_application_credential_v3 openstack_identity_ec2_credential_v3
openstack_identity_endpoint_v3               openstack_identity_group_v3
openstack_identity_inherit_role_assignment_v3 openstack_identity_limit_v3
openstack_identity_project_v3               openstack_identity_registered_limit_v3
openstack_identity_role_assignment_v3        openstack_identity_role_v3
openstack_identity_service_v3               openstack_identity_user_membership_v3
openstack_identity_user_v3                  openstack_images_image_access_accept_v2
openstack_images_image_access_v2             openstack_images_image_v2
openstack_keymanager_container_v1            openstack_keymanager_order_v1
openstack_keymanager_secret_v1               openstack_lb_flavor_v2
openstack_lb_flavorprofile_v2                openstack_lb_l7policy_v2
openstack_lb_l7rule_v2                       openstack_lb_listener_v2
openstack_lb_loadbalancer_v2                 openstack_lb_member_v2
openstack_lb_members_v2                      openstack_lb_monitor_v2
openstack_lb_pool_v2                         openstack_lb_quota_v2
openstack_networking_address_group_v2        openstack_networking_addressscope_v2
openstack_networking_bgp_peer_v2             openstack_networking_bgp_speaker_v2
openstack_networking_floatingip_associate_v2 openstack_networking_floatingip_v2
openstack_networking_network_v2              openstack_networking_port_secgroup_associate_v2
openstack_networking_port_v2                 openstack_networking_portforwarding_v2
openstack_networking_qos_bandwidth_limit_rule_v2 openstack_networking_qos_dscp_marking_rule_v2
openstack_networking_qos_minimum_bandwidth_rule_v2 openstack_networking_qos_policy_v2
openstack_networking_quota_v2                openstack_networking_rbac_policy_v2
openstack_networking_router_interface_v2     openstack_networking_router_route_v2
openstack_networking_router_routes_v2        openstack_networking_router_v2
openstack_networking_secgroup_rule_v2        openstack_networking_secgroup_v2
openstack_networking_segment_v2              openstack_networking_subnet_route_v2
openstack_networking_subnet_v2               openstack_networking_subnetpool_v2
openstack_networking_trunk_v2                openstack_objectstorage_account_v1
openstack_objectstorage_container_v1         openstack_objectstorage_object_v1
openstack_objectstorage_tempurl_v1           openstack_orchestration_stack_v1
openstack_sharedfilesystem_securityservice_v2 openstack_sharedfilesystem_share_access_v2
openstack_sharedfilesystem_share_v2          openstack_sharedfilesystem_sharenetwork_v2
openstack_taas_tap_mirror_v2                 openstack_vpnaas_endpoint_group_v2
openstack_vpnaas_ike_policy_v2               openstack_vpnaas_ipsec_policy_v2
openstack_vpnaas_service_v2                  openstack_vpnaas_site_connection_v2
openstack_workflow_cron_trigger_v2
```

## Registered Data Sources (64)

```
openstack_blockstorage_availability_zones_v3  openstack_blockstorage_quotaset_v3
openstack_blockstorage_snapshot_v3            openstack_blockstorage_volume_v3
openstack_compute_aggregate_v2                openstack_compute_availability_zones_v2
openstack_compute_flavor_v2                   openstack_compute_hypervisor_v2
openstack_compute_instance_v2                 openstack_compute_keypair_v2
openstack_compute_limits_v2                   openstack_compute_quotaset_v2
openstack_compute_servergroup_v2              openstack_containerinfra_cluster_v1
openstack_containerinfra_clustertemplate_v1   openstack_containerinfra_nodegroup_v1
openstack_dns_zone_share_v2                   openstack_dns_zone_v2
openstack_fw_group_v2                         openstack_fw_policy_v2
openstack_fw_rule_v2                          openstack_identity_auth_scope_v3
openstack_identity_endpoint_v3                openstack_identity_group_v3
openstack_identity_project_ids_v3             openstack_identity_project_v3
openstack_identity_role_v3                    openstack_identity_service_v3
openstack_identity_user_v3                    openstack_images_image_ids_v2
openstack_images_image_v2                     openstack_keymanager_container_v1
openstack_keymanager_secret_v1                openstack_lb_flavor_v2
openstack_lb_flavorprofile_v2                 openstack_lb_listener_v2
openstack_lb_loadbalancer_v2                  openstack_lb_member_v2
openstack_lb_monitor_v2                       openstack_lb_pool_v2
openstack_loadbalancer_flavor_v2              openstack_networking_addressscope_v2
openstack_networking_floatingip_v2            openstack_networking_network_v2
openstack_networking_port_ids_v2              openstack_networking_port_v2
openstack_networking_qos_bandwidth_limit_rule_v2 openstack_networking_qos_dscp_marking_rule_v2
openstack_networking_qos_minimum_bandwidth_rule_v2 openstack_networking_qos_policy_v2
openstack_networking_quota_v2                 openstack_networking_router_v2
openstack_networking_secgroup_v2              openstack_networking_segment_v2
openstack_networking_subnet_ids_v2            openstack_networking_subnet_v2
openstack_networking_subnetpool_v2            openstack_networking_trunk_v2
openstack_sharedfilesystem_availability_zones_v2 openstack_sharedfilesystem_share_v2
openstack_sharedfilesystem_sharenetwork_v2    openstack_sharedfilesystem_snapshot_v2
openstack_workflow_cron_trigger_v2            openstack_workflow_workflow_v2
```

## Special-pattern inventory

### State upgraders (1 resource — must be read before migrating)

| Resource | File | SchemaVersion | Notes |
|---|---|---|---|
| `openstack_objectstorage_container_v1` | `resource_openstack_objectstorage_container_v1.go` | 1 | Migration logic in `migrate_resource_openstack_objectstorage_container_v1.go`. SDKv2 chains upgraders V0→V1; framework requires a single-step upgrader that takes prior schema and emits target-version state. |

### CustomizeDiff (3 resources — each becomes `ModifyPlan`)

| Resource | File | Notes |
|---|---|---|
| `openstack_compute_instance_v2` | `resource_openstack_compute_instance_v2.go` | Uses `customdiff.All(...)` — multiple diff functions bundled. Most complex resource overall. |
| `openstack_images_image_v2` | `resource_openstack_images_image_v2.go` | `resourceImagesImageV2UpdateComputedAttributes` — sets computed attributes during diff. |
| `openstack_networking_bgp_peer_v2` | `resource_openstack_networking_bgp_peer_v2.go` | Inline `CustomizeDiff` function. |

### StateFunc (4 occurrences — each becomes a custom type)

| Resource | File |
|---|---|
| `openstack_compute_instance_v2` | `resource_openstack_compute_instance_v2.go` |
| `openstack_networking_port_v2` | `resource_openstack_networking_port_v2.go` |
| `openstack_networking_secgroup_rule_v2` | `resource_openstack_networking_secgroup_rule_v2.go` |
| `openstack_lb_flavorprofile_v2` | `resource_openstack_lb_flavorprofile_v2.go` |

### DiffSuppressFunc (8 occurrences — analyse intent before migrating)

| Resource | File |
|---|---|
| `openstack_compute_instance_v2` | `resource_openstack_compute_instance_v2.go` |
| `openstack_keymanager_secret_v1` | `resource_openstack_keymanager_secret_v1.go` |
| `openstack_lb_flavorprofile_v2` | `resource_openstack_lb_flavorprofile_v2.go` |
| `openstack_lb_listener_v2` | `resource_openstack_lb_listener_v2.go` |
| `openstack_networking_port_v2` | `resource_openstack_networking_port_v2.go` |
| `openstack_objectstorage_account_v1` | `resource_openstack_objectstorage_account_v1.go` |
| `openstack_objectstorage_object_v1` | `resource_openstack_objectstorage_object_v1.go` |

### MaxItems:1 nested blocks (11 — block vs. single nested attribute decision required)

| Resource / Data Source | File |
|---|---|
| `openstack_compute_instance_v2` | `resource_openstack_compute_instance_v2.go` |
| `openstack_compute_servergroup_v2` | `resource_openstack_compute_servergroup_v2.go` |
| `openstack_compute_volume_attach_v2` | `resource_openstack_compute_volume_attach_v2.go` |
| `openstack_db_instance_v1` | `resource_openstack_db_instance_v1.go` |
| `openstack_keymanager_order_v1` | `resource_openstack_keymanager_order_v1.go` |
| `openstack_lb_pool_v2` | `resource_openstack_lb_pool_v2.go` |
| `openstack_networking_port_v2` | `resource_openstack_networking_port_v2.go` |
| `openstack_networking_router_v2` | `resource_openstack_networking_router_v2.go` |
| `openstack_objectstorage_container_v1` | `resource_openstack_objectstorage_container_v1.go` |
| `openstack_taas_tap_mirror_v2` | `resource_openstack_taas_tap_mirror_v2.go` |
| `migrate_resource_openstack_objectstorage_container_v1.go` (upgrader schema) | `migrate_resource_openstack_objectstorage_container_v1.go` |

### Composite-ID importers (confirmed non-passthrough)

Most of the 101 importers use `schema.ImportStatePassthroughContext` (simple ID passthrough). Resources that use custom importer logic (composite IDs split from a string) include at minimum:

- `openstack_bgpvpn_network_associate_v2` — verified passthrough (checked above); most bgpvpn/associate and QoS rule resources use passthrough
- Resources in the "Needs manual review" list with "custom Importer" should be read to confirm whether they use passthrough or parse composite IDs (e.g. `<parent_id>/<child_id>`)

Confirmed composite-ID pattern resources (parse `"<id1>/<id2>"` in importer):
- `openstack_networking_qos_bandwidth_limit_rule_v2`, `_dscp_marking_rule_v2`, `_minimum_bandwidth_rule_v2` — QoS rules use `<policy_id>/<rule_id>` shape
- `openstack_blockstorage_volume_type_access_v3` — uses composite ID
- `openstack_blockstorage_qos_association_v3` — uses composite ID

## Needs manual review

Read these files directly before proposing any edits. The block-vs-nested-attribute, single-step state-upgrade, and composite-ID importer decisions all require human/LLM judgment.

- `openstack/data_source_openstack_blockstorage_volume_v3.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_compute_instance_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_compute_servergroup_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_identity_auth_scope_v3.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_keymanager_container_v1.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_lb_listener_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_lb_loadbalancer_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_lb_monitor_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_lb_pool_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_networking_network_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_networking_port_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_networking_router_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_networking_subnet_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_networking_trunk_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/data_source_openstack_sharedfilesystem_share_v2.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/keymanager_v1.go` — nested Elem &Resource (block-vs-nested decision)
- `openstack/migrate_resource_openstack_objectstorage_container_v1.go` — MaxItems:1 (block-vs-nested-attribute decision); nested Elem &Resource (block-vs-nested decision)
- `openstack/resource_openstack_bgpvpn_network_associate_v2.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_bgpvpn_port_associate_v2.go` — custom Importer (composite ID parsing?); nested Elem &Resource (block-vs-nested decision)
- `openstack/resource_openstack_bgpvpn_router_associate_v2.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_bgpvpn_v2.go` — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- `openstack/resource_openstack_blockstorage_qos_association_v3.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_blockstorage_qos_v3.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_blockstorage_quotaset_v3.go` — custom Importer (composite ID parsing?); Timeouts (separate framework package)
- `openstack/resource_openstack_blockstorage_volume_attach_v3.go` — Timeouts (separate framework package)
- `openstack/resource_openstack_blockstorage_volume_type_access_v3.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_blockstorage_volume_type_v3.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_blockstorage_volume_v3.go` — custom Importer (composite ID parsing?); Timeouts; nested Elem &Resource
- `openstack/resource_openstack_compute_aggregate_v2.go` — custom Importer (composite ID parsing?); Timeouts
- `openstack/resource_openstack_compute_flavor_access_v2.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_compute_flavor_v2.go` — custom Importer (composite ID parsing?)
- `openstack/resource_openstack_compute_instance_v2.go` — MaxItems:1; custom Importer; Timeouts; CustomizeDiff; StateFunc; DiffSuppressFunc; nested Elem (**highest complexity — migrate last**)
- `openstack/resource_openstack_compute_interface_attach_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_compute_keypair_v2.go` — custom Importer
- `openstack/resource_openstack_compute_quotaset_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_compute_servergroup_v2.go` — MaxItems:1; custom Importer; nested Elem
- `openstack/resource_openstack_compute_volume_attach_v2.go` — MaxItems:1; custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_containerinfra_cluster_v1.go` — custom Importer; Timeouts
- `openstack/resource_openstack_containerinfra_clustertemplate_v1.go` — custom Importer; Timeouts
- `openstack/resource_openstack_containerinfra_nodegroup_v1.go` — custom Importer; Timeouts
- `openstack/resource_openstack_db_configuration_v1.go` — Timeouts; nested Elem
- `openstack/resource_openstack_db_database_v1.go` — custom Importer; Timeouts
- `openstack/resource_openstack_db_instance_v1.go` — MaxItems:1; Timeouts; nested Elem
- `openstack/resource_openstack_db_user_v1.go` — Timeouts
- `openstack/resource_openstack_dns_quota_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_dns_recordset_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_dns_transfer_accept_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_dns_transfer_request_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_dns_zone_share_v2.go` — custom Importer
- `openstack/resource_openstack_dns_zone_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_fw_group_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_fw_policy_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_fw_rule_v2.go` — custom Importer
- `openstack/resource_openstack_identity_application_credential_v3.go` — custom Importer; nested Elem
- `openstack/resource_openstack_identity_ec2_credential_v3.go` — custom Importer
- `openstack/resource_openstack_identity_endpoint_v3.go` — custom Importer
- `openstack/resource_openstack_identity_group_v3.go` — custom Importer
- `openstack/resource_openstack_identity_inherit_role_assignment_v3.go` — custom Importer
- `openstack/resource_openstack_identity_limit_v3.go` — custom Importer
- `openstack/resource_openstack_identity_project_v3.go` — custom Importer
- `openstack/resource_openstack_identity_registered_limit_v3.go` — custom Importer
- `openstack/resource_openstack_identity_role_assignment_v3.go` — custom Importer
- `openstack/resource_openstack_identity_role_v3.go` — custom Importer
- `openstack/resource_openstack_identity_service_v3.go` — custom Importer
- `openstack/resource_openstack_identity_user_membership_v3.go` — custom Importer
- `openstack/resource_openstack_identity_user_v3.go` — custom Importer; nested Elem
- `openstack/resource_openstack_images_image_access_accept_v2.go` — custom Importer
- `openstack/resource_openstack_images_image_access_v2.go` — custom Importer
- `openstack/resource_openstack_images_image_v2.go` — custom Importer; Timeouts; CustomizeDiff
- `openstack/resource_openstack_keymanager_container_v1.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_keymanager_order_v1.go` — MaxItems:1; custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_keymanager_secret_v1.go` — custom Importer; Timeouts; DiffSuppressFunc
- `openstack/resource_openstack_lb_flavor_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_lb_flavorprofile_v2.go` — custom Importer; Timeouts; StateFunc; DiffSuppressFunc
- `openstack/resource_openstack_lb_l7policy_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_lb_l7rule_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_lb_listener_v2.go` — custom Importer; Timeouts; DiffSuppressFunc
- `openstack/resource_openstack_lb_loadbalancer_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_lb_member_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_lb_members_v2.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_lb_monitor_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_lb_pool_v2.go` — MaxItems:1; custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_lb_quota_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_address_group_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_addressscope_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_bgp_peer_v2.go` — custom Importer; Timeouts; CustomizeDiff
- `openstack/resource_openstack_networking_bgp_speaker_v2.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_networking_floatingip_associate_v2.go` — custom Importer
- `openstack/resource_openstack_networking_floatingip_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_network_v2.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_networking_port_secgroup_associate_v2.go` — custom Importer
- `openstack/resource_openstack_networking_port_v2.go` — MaxItems:1; custom Importer; Timeouts; StateFunc; DiffSuppressFunc; nested Elem
- `openstack/resource_openstack_networking_portforwarding_v2.go` — Timeouts
- `openstack/resource_openstack_networking_qos_bandwidth_limit_rule_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_qos_dscp_marking_rule_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_qos_minimum_bandwidth_rule_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_qos_policy_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_quota_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_rbac_policy_v2.go` — custom Importer
- `openstack/resource_openstack_networking_router_interface_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_router_route_v2.go` — custom Importer
- `openstack/resource_openstack_networking_router_routes_v2.go` — custom Importer; nested Elem
- `openstack/resource_openstack_networking_router_v2.go` — MaxItems:1; custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_networking_secgroup_rule_v2.go` — custom Importer; Timeouts; StateFunc
- `openstack/resource_openstack_networking_secgroup_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_segment_v2.go` — custom Importer
- `openstack/resource_openstack_networking_subnet_route_v2.go` — custom Importer
- `openstack/resource_openstack_networking_subnet_v2.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_networking_subnetpool_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_networking_trunk_v2.go` — Timeouts; nested Elem
- `openstack/resource_openstack_objectstorage_account_v1.go` — custom Importer; DiffSuppressFunc
- `openstack/resource_openstack_objectstorage_container_v1.go` — MaxItems:1; **StateUpgraders** (SchemaVersion 1); custom Importer; nested Elem
- `openstack/resource_openstack_objectstorage_object_v1.go` — DiffSuppressFunc
- `openstack/resource_openstack_orchestration_stack_v1.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_sharedfilesystem_securityservice_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_sharedfilesystem_share_access_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_sharedfilesystem_share_v2.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_sharedfilesystem_sharenetwork_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_taas_tap_mirror_v2.go` — MaxItems:1; custom Importer; nested Elem
- `openstack/resource_openstack_vpnaas_endpoint_group_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_vpnaas_ike_policy_v2.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_vpnaas_ipsec_policy_v2.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_vpnaas_service_v2.go` — custom Importer; Timeouts
- `openstack/resource_openstack_vpnaas_site_connection.go` — custom Importer; Timeouts; nested Elem
- `openstack/resource_openstack_workflow_cron_trigger_v2.go` — custom Importer

## Complexity tiers (migration order guide)

### Tier 1 — Straightforward (no nested blocks, passthrough importer, no special patterns)
Resources/data sources with only primitive attributes, passthrough importers, and ForceNew plan modifiers. These are good candidates for a pilot migration. Examples:
- `openstack_identity_role_v3`, `openstack_identity_service_v3`, `openstack_identity_ec2_credential_v3`
- `openstack_dns_zone_share_v2`, `openstack_networking_segment_v2`
- Most data sources (no CRUD, no importer, no timeouts)

### Tier 2 — Moderate (timeouts + importer, minimal nested blocks)
Resources with timeouts (use `terraform-plugin-framework-timeouts` package) and/or composite importers but no CustomizeDiff/StateFunc. Examples:
- `openstack_lb_loadbalancer_v2`, `openstack_dns_zone_v2`, `openstack_networking_secgroup_v2`
- `openstack_blockstorage_volume_v3`, `openstack_sharedfilesystem_share_v2`

### Tier 3 — Complex (nested blocks, DiffSuppressFunc, multiple patterns)
- `openstack_compute_instance_v2` — MaxItems:1; CustomizeDiff (`customdiff.All`); StateFunc; DiffSuppressFunc; 5 nested Elem; Timeouts — **migrate last or as its own mini-project**
- `openstack_networking_port_v2` — MaxItems:1; StateFunc; DiffSuppressFunc; 4 nested Elem
- `openstack_lb_pool_v2` — MaxItems:1; persistence block decision; 5 validators
- `openstack_db_instance_v1` — MaxItems:1; 4 nested Elem
- `openstack_objectstorage_container_v1` — **sole StateUpgraders**; MaxItems:1; must do single-step upgrade

### Tier 4 — State upgrader (special care)
- `openstack_objectstorage_container_v1` — the only resource with `SchemaVersion: 1` + `StateUpgraders`. The V0→V1 migration logic lives in `migrate_resource_openstack_objectstorage_container_v1.go`. In the framework this becomes `ResourceWithUpgradeState`. The upgrader must be a single function (not chained) that accepts the V0 schema and produces V1 state directly.

## Next steps

1. Read every file listed under "Needs manual review" before proposing edits.
2. Populate the migration checklist from this audit.
3. Confirm scope with the team (whole provider? specific service groups? one pilot resource?) before starting workflow step 1.
4. Decide: protocol v5 or v6 (default recommendation is v6 for a single-release migration).
