# Terraform Provider OpenStack — SDKv2 Migration Audit Report

**Date:** 2026-04-30
**Provider module:** `github.com/terraform-provider-openstack/terraform-provider-openstack/v3`
**Go version:** 1.25.8
**SDKv2 version:** `github.com/hashicorp/terraform-plugin-sdk/v2 v2.38.1`
**Terraform Plugin Framework:** not present (zero imports detected)

---

## 1. High-Level Statistics

| Metric | Count |
|--------|-------|
| Registered managed resources | 109 |
| Registered data sources | 64 |
| Non-test Go source files | ~175 |
| Total non-test lines of Go | ~52,000 |
| Acceptance test files | 272 |
| Import test files | 97 |
| Shared helper files | ~57 |

---

## 2. Service Areas and Resource/Data Source Counts

| Service Area | Resources | Data Sources | Approx. LoC (resources) | Notes |
|---|---|---|---|---|
| networking | 27 | 17 | ~18,900 | Largest service; complex shared helpers |
| lb (Octavia LB) | 11 | 8 | ~7,900 | Complex polling; AtLeastOneOf; DiffSuppress |
| compute | 9 | 9 | ~7,100 | Largest single resource (compute_instance, 1758 lines) |
| identity | 13 | 8 | ~4,600 | |
| blockstorage | 7 | 4 | ~3,500 | |
| vpnaas | 5 | 0 | ~3,100 | |
| dns | 6 | 2 | ~2,800 | |
| sharedfilesystem | 4 | 4 | ~2,600 | |
| containerinfra | 3 | 3 | ~2,700 | Sensitive fields; complex schema |
| keymanager | 3 | 2 | ~2,100 | Sensitive fields |
| images | 3 | 2 | ~1,900 | CustomizeDiff; DiffSuppressFunc |
| objectstorage | 4 | 0 | ~2,400 | One StateUpgrader |
| fw | 3 | 3 | ~2,600 | |
| bgpvpn | 4 | 0 | ~1,400 | Multiple TypeSet w/ HashString |
| db | 4 | 0 | ~1,500 | |
| orchestration | 1 | 0 | ~750 | |
| taas | 1 | 0 | ~460 | AtLeastOneOf |
| workflow | 1 | 2 | ~290 | |

---

## 3. Plugin Architecture

**Entry point:** `main.go` — uses `plugin.Serve(&plugin.ServeOpts{...})` from `terraform-plugin-sdk/v2/plugin`. No mux server. No framework server. Protocol version declared as `5.0` in `terraform-registry-manifest.json`.

**Provider function:** `openstack.Provider()` in `openstack/provider.go` returns `*schema.Provider`. Configuration uses `ConfigureContextFunc`. All 109 resources and 64 data sources are registered via `ResourcesMap` and `DataSourcesMap` respectively.

---

## 4. Schema Feature Inventory (Migration Complexity Signals)

Each feature listed below requires a specific migration treatment in terraform-plugin-framework.

| Feature | Files Affected | Framework Equivalent |
|---|---|---|
| `CustomizeDiff` | 3 | `resource.ModifyPlan` |
| `DiffSuppressFunc` | 8 | Custom `PlanModifier` |
| `StateFunc` | 4 | Custom `PlanModifier` + `StateUpgrader` or inline normalization |
| `ValidateFunc` | 63 | `validator.String` / `validator.Int64` etc. |
| `ForceNew: true` | 139 | `planmodifier.RequiresReplace()` |
| `Sensitive: true` | 19 | `schema.SensitiveAttribute` |
| `TypeSet` usage | 67 | `types.Set` + `SetNestedAttribute` |
| `TypeList` usage | 52 | `types.List` + `ListNestedAttribute` |
| `Set:` (custom hash) | 7 custom, many `schema.HashString` | No equivalent; TypeSet becomes ListSet or SetNestedAttribute with framework-managed ordering |
| `ConflictsWith` | 29 files | `validator.Conflicts` |
| `AtLeastOneOf` | 10 files | `validator.AtLeastOneOf` |
| `ExactlyOneOf` | ~5 files | `validator.ExactlyOneOf` |
| `Timeouts` | 69 files | `resource.WithTimeouts()` |
| `StateUpgraders` | 1 (objectstorage_container_v1) | `resource.StateUpgraders` in framework |
| `retry.StateChangeConf` (polling) | 91 files | unchanged (gophercloud helper; not SDK-specific) |

---

## 5. Shared Helper Files That Must Be Migrated

These files contain logic shared by multiple resources; they use SDK types directly and will need framework-compatible equivalents.

| File | Purpose | Consumers |
|---|---|---|
| `util.go` | `CheckDeleted`, `GetRegion`, `BuildRequest`, retry helpers | Nearly all resources |
| `types.go` | Custom schema types (mapStringString etc.) | Many resources |
| `networking_v2_shared.go` | Tag helpers, common networking functions | All networking resources |
| `networking_port_v2.go` | Port build/flatten helpers | networking_port and compute_instance |
| `networking_network_v2.go` | Network build/flatten helpers | networking_network and others |
| `lb_v2_shared.go` | LB polling helpers, custom CRUD wrappers | All lb resources |
| `compute_instance_v2.go` | Instance block helpers, scheduler hints hash | compute_instance |
| `blockstorage_volume_v3.go` | Attachment/scheduler hint hash functions | blockstorage_volume |
| `keymanager_v1.go` | Key manager shared helpers | All keymanager resources |
| `sharedfilesystem_shared_v2.go` | Manila shared helpers | All sharedfilesystem resources |
| `containerinfra_shared_v1.go` | Magnum shared helpers | All containerinfra resources |
| `images_image_v2.go` | Image build/flatten helpers | images_image and data source |

---

## 6. Complexity Classification of Resources

### Tier 1 — Simple (good pilot candidates)
Fewer than ~200 lines, no CustomizeDiff, no DiffSuppressFunc, no TypeSet with custom hash, no StateUpgraders.

- `openstack_identity_role_v3` (132 lines)
- `openstack_workflow_cron_trigger_v2` (150 lines)
- `openstack_networking_rbac_policy_v2` (157 lines)
- `openstack_bgpvpn_network_associate_v2` (126 lines)
- `openstack_bgpvpn_router_associate_v2` (168 lines)
- `openstack_networking_addressscope_v2` (199 lines)

### Tier 2 — Moderate
200–500 lines, standard TypeSet with `schema.HashString`, standard ValidateFunc, no CustomizeDiff.

Examples: `openstack_dns_quota_v2`, `openstack_dns_zone_v2`, `openstack_fw_rule_v2`, `openstack_networking_secgroup_v2`, `openstack_identity_project_v3`, most identity resources, most DNS resources, most fw resources.

### Tier 3 — Complex
500+ lines, or uses CustomizeDiff / DiffSuppressFunc / StateFunc / custom hash / StateUpgraders / heavy shared helpers.

- `openstack_compute_instance_v2` (1758 lines; CustomizeDiff; DiffSuppressFunc; StateFunc; complex TypeList/TypeSet)
- `openstack_networking_port_v2` (~1100 lines; DiffSuppressFunc; custom hash; shared helpers)
- `openstack_lb_loadbalancer_v2` (~400 lines; AtLeastOneOf; TypeSet with HashString; heavy polling)
- `openstack_lb_listener_v2` (DiffSuppressFunc; TypeSet)
- `openstack_images_image_v2` (558 lines; CustomizeDiff; DiffSuppressFunc)
- `openstack_objectstorage_container_v1` (StateUpgrader)
- `openstack_keymanager_secret_v1` (DiffSuppressFunc)
- All `networking_*` resources relying on `networking_port_v2.go` shared helpers

---

## 7. Test Infrastructure

- Acceptance tests use `terraform-plugin-testing v1.14.0` (compatible with both SDKv2 and framework during mux period).
- `provider_test.go` sets up `testAccProvider` via `Provider()` — must be updated to use `providerserver.NewProviderServer` once mux is introduced.
- 97 import test files (`import_openstack_*.go`) use `resource.TestCheckResourceAttrSet` patterns that work unchanged with framework.
- No unit tests for schema validation logic (all testing is acceptance-based).

---

## 8. Dependencies to Add for Migration

```
github.com/hashicorp/terraform-plugin-framework         (core framework)
github.com/hashicorp/terraform-plugin-framework-validators (common validators)
github.com/hashicorp/terraform-plugin-mux               (mux server for incremental migration)
github.com/hashicorp/terraform-plugin-go                (already indirect; will become direct)
```

The `terraform-registry-manifest.json` protocol version will need updating to `["5.0", "6.0"]` once any framework resource is registered.

---

## 9. Notable Special Cases

| Item | Detail |
|---|---|
| `openstack_objectstorage_container_v1` | Only resource with `StateUpgraders`; schema changed from old `versioning` TypeSet to new shape |
| `openstack_compute_instance_v2` | Uses `resourceComputeSchedulerHintsHash` and `resourceComputeInstancePersonalityHash` custom hash functions; both must be replaced |
| `openstack_blockstorage_volume_v3` | Uses `blockStorageVolumeV3AttachmentHash` and `blockStorageVolumeV3SchedulerHintsHash` custom hashes |
| `openstack_networking_port_v2` | Uses `resourceNetworkingPortV2AllowedAddressPairsHash` custom hash |
| Provider `Config` struct | Embeds `auth.Config` from `terraform-provider-openstack/utils/v2`; framework provider will need a different `Configure` method signature |
| `mutexkv.MutexKV` | Used in Config for concurrency control; survives migration unchanged |
| `GetRegion` helper | Called by ~100 resources via `d *schema.ResourceData`; will need a framework variant taking `*Config` directly |
