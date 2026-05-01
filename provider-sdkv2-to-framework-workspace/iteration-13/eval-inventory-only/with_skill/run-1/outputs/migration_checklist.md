# Migration plan: terraform-provider-openstack

Source: `terraform-plugin-sdk/v2 v2.38.1` → target: `terraform-plugin-framework`.
Generated from `audit_sdkv2.sh` against `/Users/simon.lynch/git/terraform-provider-openstack` on 2026-05-01.
Inventory phase only — no code edits. Confirm scope with the team before any work begins.

## Pre-flight

- [x] Mux check — confirmed not a `terraform-plugin-mux` / multi-release / staged migration (Pre-flight 0). User asked for a single migration plan; `go.mod` has no `terraform-plugin-mux`.
- [x] Audit complete — artefact: `audit_report.md` (Pre-flight A)
- [ ] User has confirmed scope (whole provider? specific resources?) (Pre-flight B). **Default proposal: whole provider, single release.** 110 resources + 63 data sources = 173 registered objects (audit found 174 constructors — one extra is a util/internal `*schema.Resource`).
- [ ] Decision: protocol v5 or v6 — **default v6** (single-release migration; required for write-only and several framework-only attribute features). See `references/protocol-versions.md`.
- [ ] Files needing manual review have been read end-to-end (see "Manual-review queue" below)
- [ ] Per-resource think pass written for each audit-flagged resource (block decision / state upgrade / import shape) (Pre-flight C)
- [ ] Test-side scope reviewed (see "Test-side migration scope" below — provider-level prerequisite, not per-resource)

## Test-side migration scope

The audit's "Test-file findings" lists per-test-file pattern counts plus shared infra.
Per-resource test rewrites (workflow step 7) cannot succeed until shared plumbing has a framework path.

### Shared test infrastructure

- `openstack/provider_test.go` — purpose: holds `testAccProvider` (`*schema.Provider`), `testAccProviders` (`map[string]*schema.Provider`), `testAccPreCheck`, and is referenced by every `*_test.go`. **This is the single most important file to migrate first.**
- [ ] Framework provider factory wired up: declare `testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){"openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider())}` in `openstack/provider_test.go`.
- [ ] Shared `testAccProvider` references migrated to the framework provider (single-release path; muxing is out of scope per pre-flight 0).
- [ ] Test-side client/meta accessors migrated. Audit found 13 `d.Id()/d.SetId()`, 11 `d.Get/GetOk/GetOkExists`, 11 `d.Set` calls inside `*_test.go` — likely in helper closures (`Check*Func`, etc.).

### Test-side counts (from audit)

- Test files audited: **300**
- `ProviderFactories` references to flip → `ProtoV6ProviderFactories`: **526** (across all `*_test.go`)
- `resource.Test`/`UnitTest`/`ParallelTest` calls (must use `terraform-plugin-testing/helper/resource`): **526**
- `*schema.ResourceData` and `d.X` calls in tests: 13 SetId + 11 Get + 11 Set ≈ **35**
- `helper/acctest` utilities: **122** (replace with `terraform-plugin-testing/helper/acctest`)
- `PreCheck` hooks: **526** (most reuse `testAccPreCheck`; one update unblocks them all)

### Test-side negative gate (run at workflow step 11)

- [ ] No surviving `terraform-plugin-sdk/v2/helper/resource` imports in any `*_test.go`
- [ ] No surviving `terraform-plugin-sdk/v2/terraform` imports in any `*_test.go`
- [ ] No `*schema.Provider`/`*schema.ResourceData` references in test files

### Highest-pattern test files (audit top-10)

Plan acceptance-test rewrites in this order (highest first means biggest signal-to-noise on the new factory):

| Test file | Pattern count |
|---|---:|
| `openstack/resource_openstack_compute_instance_v2_test.go` | 81 |
| `openstack/resource_openstack_networking_port_v2_test.go` | 72 |
| `openstack/resource_openstack_networking_network_v2_test.go` | 48 |
| `openstack/resource_openstack_networking_subnet_v2_test.go` | 36 |
| `openstack/resource_openstack_networking_router_v2_test.go` | 27 |
| `openstack/networking_port_v2_test.go` | 26 |
| `openstack/resource_openstack_compute_servergroup_v2_test.go` | 24 |
| `openstack/resource_openstack_images_image_v2_test.go` | 24 |
| `openstack/import_openstack_containerinfra_nodegroup_v1_test.go` | 21 |
| `openstack/resource_openstack_blockstorage_volume_v3_test.go` | 21 |

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

## Provider-level findings (work that precedes any per-resource migration)

- `*schema.Provider` references: **2** (`openstack/provider.go` constructs the SDKv2 provider; `openstack/provider_test.go` holds `testAccProvider`).
- `ResourcesMap`: **1** (110 entries in `provider.go`)
- `DataSourcesMap`: **1** (63 entries in `provider.go`)
- `schema.EnvDefaultFunc` / `MultiEnvDefaultFunc` (provider config): **34** — must port to framework `Schema` `Attribute` defaults via `defaults` package or compute in `Configure`. **Note: `provider.go` has `Default:` without `Computed: true`**, which the framework rejects at boot — fix in SDKv2 baseline first (Step 2).
- `helper/validation.*` calls: **114** — replace with `github.com/hashicorp/terraform-plugin-framework-validators` equivalents (string/int64/list/map). See `references/validators.md`.
- `retry.StateChangeConf`: **121**, `retry.RetryContext`: **33** — no framework equivalent. **Replace with inline ticker loops or wrap as a generic helper.** This is the largest single piece of mechanical work in the migration; ~20+ resources use it.
- `helper/customdiff` combinators: **3** — port to `ModifyPlan`. See `references/plan-modifiers.md`.
- `helper/structure` JSON helpers: **2** — port to custom string-type or plan modifier. See `references/state-and-types.md`.

### Step 2 — data-consistency findings (fix in SDKv2 first, before any framework work)

The framework rejects these patterns at provider boot. Fixing them now keeps the SDKv2 baseline green and avoids debugging two bugs at once after migration.

| Pattern | Count | Action |
|---|---:|---|
| Optional+Computed without `UseStateForUnknown` | **577** | Carry plan modifier across in framework form. Most-noisy migration trap; touches almost every resource. |
| `Default:` without `Computed: true` | **88** | Add `Computed: true` in SDKv2 *now*, then mirror Default + UseStateForUnknown in framework. |
| `ForceNew` on a pure-Computed attribute | **5** | Drop ForceNew or remove Computed in SDKv2 first; framework rejects at boot. Three resources hit: `containerinfra_cluster_v1`, `containerinfra_clustertemplate_v1`, `containerinfra_nodegroup_v1`. |
| `TypeList` + `MaxItems: 1` without `Elem` | **3** | Malformed schema; fix in SDKv2 first. |

## Block / nested-attribute decisions (`MaxItems: 1`)

11 occurrences. Per the skill's decision rule, default to **keep as block** (`ListNestedBlock` + `listvalidator.SizeAtMost(1)` or `SingleNestedBlock`) unless we are doing a major version bump. Switching to `SingleNestedAttribute` is a breaking HCL change (`foo { ... }` → `foo = { ... }`).

Files with `MaxItems: 1`:

- `openstack/migrate_resource_openstack_objectstorage_container_v1.go` (state-upgrade prior schema; mirror whatever decision is taken on the live resource)
- `openstack/resource_openstack_compute_instance_v2.go` (multiple — the deepest case in the codebase)
- `openstack/resource_openstack_compute_servergroup_v2.go`
- `openstack/resource_openstack_compute_volume_attach_v2.go`
- `openstack/resource_openstack_db_instance_v1.go`
- `openstack/resource_openstack_keymanager_order_v1.go`
- `openstack/resource_openstack_lb_pool_v2.go`
- `openstack/resource_openstack_networking_port_v2.go`
- `openstack/resource_openstack_networking_router_v2.go`
- `openstack/resource_openstack_objectstorage_container_v1.go`
- `openstack/resource_openstack_taas_tap_mirror_v2.go`

## State upgraders

**Exactly 1** in production code: `openstack/resource_openstack_objectstorage_container_v1.go` (`SchemaVersion: 1`, with prior schema in `migrate_resource_openstack_objectstorage_container_v1.go`).

- [ ] Read `references/state-upgrade.md` before touching this resource.
- [ ] Confirm chain length (V0 → current). If only V0→V1, the framework upgrader is direct; if longer, **compose the chain inline** in the V0 entry (do NOT delegate one upgrader to another — silent state corruption).

## Manual-review queue (judgment-rich files; read before editing)

The audit flagged ~180 files for manual review. Group by complexity tier; team should pair-review the highest-tier files before scoping a sprint.

### Tier A — highest complexity (combine 4+ judgment-rich patterns)

| File | Patterns flagged |
|---|---|
| `openstack/resource_openstack_compute_instance_v2.go` | MaxItems:1, custom Importer, Timeouts, CustomizeDiff, StateFunc, DiffSuppressFunc, nested blocks, validators (Conflicts/etc.), retry.StateChangeConf, customdiff combinators, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_networking_port_v2.go` | MaxItems:1, custom Importer, Timeouts, StateFunc, DiffSuppressFunc, nested blocks, validators, retry.StateChangeConf, helper/structure JSON, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_objectstorage_container_v1.go` | MaxItems:1, **StateUpgraders/SchemaVersion**, custom Importer, nested blocks, validators, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_lb_pool_v2.go` | MaxItems:1, custom Importer, Timeouts, nested blocks, validators, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_networking_router_v2.go` | MaxItems:1, custom Importer, Timeouts, nested blocks, validators, retry.StateChangeConf, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_db_instance_v1.go` | MaxItems:1, Timeouts, nested blocks, validators, retry.StateChangeConf, Optional+Computed |
| `openstack/resource_openstack_blockstorage_volume_v3.go` | custom Importer, Timeouts, nested blocks, validators, retry.StateChangeConf, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_images_image_v2.go` | custom Importer, Timeouts, CustomizeDiff, validators, retry.StateChangeConf, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_keymanager_order_v1.go` | MaxItems:1, custom Importer, Timeouts, nested blocks, retry.StateChangeConf, Optional+Computed |
| `openstack/resource_openstack_compute_volume_attach_v2.go` | MaxItems:1, custom Importer, Timeouts, nested blocks, retry.StateChangeConf, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_compute_servergroup_v2.go` | MaxItems:1, custom Importer, nested blocks, Optional+Computed |
| `openstack/resource_openstack_taas_tap_mirror_v2.go` | MaxItems:1, custom Importer, nested blocks, validators, Optional+Computed |
| `openstack/resource_openstack_lb_listener_v2.go` | custom Importer, Timeouts, DiffSuppressFunc, validators, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_lb_flavorprofile_v2.go` | custom Importer, Timeouts, StateFunc, DiffSuppressFunc, helper/structure JSON, Optional+Computed |
| `openstack/resource_openstack_networking_secgroup_rule_v2.go` | custom Importer, Timeouts, StateFunc, validators, retry.StateChangeConf, Optional+Computed |
| `openstack/resource_openstack_networking_subnet_v2.go` | custom Importer, Timeouts, nested blocks, validators, retry.StateChangeConf, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_fw_group_v2.go` | custom Importer, Timeouts, validators, retry.StateChangeConf, set casts, Optional+Computed, Default-without-Computed |
| `openstack/resource_openstack_keymanager_secret_v1.go` | custom Importer, Timeouts, DiffSuppressFunc, retry.StateChangeConf, Optional+Computed |
| `openstack/resource_openstack_bgpvpn_v2.go` | custom Importer, Timeouts, set casts, Optional+Computed |
| `openstack/resource_openstack_bgpvpn_port_associate_v2.go` | custom Importer, nested blocks, set casts, Optional+Computed |

### Tier B — moderate complexity (Importer + Timeouts + retry.StateChangeConf or set casts)

Read the audit's "Needs manual review" section for the full per-file flag list. Representative files:

`openstack/resource_openstack_blockstorage_volume_attach_v3.go`, `openstack/resource_openstack_compute_aggregate_v2.go`, `openstack/resource_openstack_compute_interface_attach_v2.go`, `openstack/resource_openstack_containerinfra_cluster_v1.go`, `openstack/resource_openstack_containerinfra_clustertemplate_v1.go`, `openstack/resource_openstack_containerinfra_nodegroup_v1.go`, `openstack/resource_openstack_db_configuration_v1.go`, `openstack/resource_openstack_db_database_v1.go`, `openstack/resource_openstack_db_user_v1.go`, `openstack/resource_openstack_dns_recordset_v2.go`, `openstack/resource_openstack_dns_transfer_accept_v2.go`, `openstack/resource_openstack_dns_transfer_request_v2.go`, `openstack/resource_openstack_dns_zone_v2.go`, `openstack/resource_openstack_fw_policy_v2.go`, `openstack/resource_openstack_keymanager_container_v1.go`, `openstack/resource_openstack_lb_loadbalancer_v2.go`, `openstack/resource_openstack_lb_l7policy_v2.go`, `openstack/resource_openstack_lb_l7rule_v2.go`, `openstack/resource_openstack_lb_member_v2.go`, `openstack/resource_openstack_lb_members_v2.go`, `openstack/resource_openstack_lb_monitor_v2.go`, `openstack/resource_openstack_networking_address_group_v2.go`, `openstack/resource_openstack_networking_addressscope_v2.go`, `openstack/resource_openstack_networking_bgp_peer_v2.go`, `openstack/resource_openstack_networking_bgp_speaker_v2.go`, `openstack/resource_openstack_networking_floatingip_v2.go`, `openstack/resource_openstack_networking_network_v2.go`, `openstack/resource_openstack_networking_port_secgroup_associate_v2.go`, `openstack/resource_openstack_networking_portforwarding_v2.go`, `openstack/resource_openstack_networking_qos_*`, `openstack/resource_openstack_networking_rbac_policy_v2.go`, `openstack/resource_openstack_networking_router_interface_v2.go`, `openstack/resource_openstack_networking_router_routes_v2.go`, `openstack/resource_openstack_networking_secgroup_v2.go`, `openstack/resource_openstack_networking_subnetpool_v2.go`, `openstack/resource_openstack_networking_trunk_v2.go`, `openstack/resource_openstack_orchestration_stack_v1.go`, `openstack/resource_openstack_sharedfilesystem_*`, `openstack/resource_openstack_vpnaas_*`.

### Tier C — low complexity (mostly Optional+Computed-without-UseStateForUnknown)

All 63 data sources. Each is small (Read-only, no Importer beyond ID, no Timeouts, no state upgraders). Migrate in batches once provider scaffolding lands.

## Recommended migration order

1. **Step 1 baseline** — full SDKv2 test pass (acceptance + unit). Don't start until this is green.
2. **Step 2 fixups in SDKv2** — fix the 5 ForceNew+Computed cases, the 88 `Default` without `Computed`, and the 3 malformed `MaxItems:1`-without-`Elem` cases. Re-run tests; commit as a clean baseline.
3. **Step 3** — `main.go`: add framework provider server alongside SDKv2 (single-release path means a brief overlap during dev; no `terraform-plugin-mux` introduced).
4. **Steps 4-5** — `NewFrameworkProvider()` skeleton: provider type, `Schema`, `Configure` plumbing for the existing OpenStack client. Migrate the 34 `EnvDefaultFunc` calls to framework defaults.
5. **Test-side scaffolding (provider-level prerequisite)** — wire `testAccProtoV6ProviderFactories` in `provider_test.go` *before* any per-resource migration so step 7's TDD-red signal is meaningful.
6. **Step 6 — per-resource, by ascending complexity:**
   1. **Wave 1 — Tier C data sources** (proves the framework data-source path; touches all the Optional+Computed work). Suggested first: `openstack_compute_availability_zones_v2`, `openstack_blockstorage_availability_zones_v3`, `openstack_compute_keypair_v2`, `openstack_identity_role_v3`.
   2. **Wave 2 — Tier B simple resources** (Importer + small attribute set). Suggested first: `openstack_compute_keypair_v2`, `openstack_identity_role_v3`, `openstack_identity_group_v3`, `openstack_dns_quota_v2`, `openstack_lb_quota_v2`. These exercise composite-ID importer and Timeouts without state-change machinery.
   3. **Wave 3 — Tier B with retry.StateChangeConf** (build a shared waiter helper in `internal/waiter/` or similar; first user pays the cost). Start with `openstack_blockstorage_volume_v3` or `openstack_dns_zone_v2`.
   4. **Wave 4 — Tier A high-complexity files** in this order: `objectstorage_container_v1` (state upgrader — proves the upgrader path), `lb_pool_v2`, `networking_router_v2`, `networking_subnet_v2`, `networking_port_v2`, then `compute_instance_v2` last (deepest CustomizeDiff/StateFunc/DiffSuppressFunc tangle).
7. **Step 10** — sweep `grep -rl 'terraform-plugin-sdk/v2' .` (excluding `vendor/`); should return nothing. Run `go mod tidy`.
8. **Step 11** — full suite (`go test ./...`, then `TF_ACC=1 go test ./...` if creds available).
9. **Step 12** — major version bump (protocol v6 + breaking-change baseline); changelog mentions Terraform CLI floor.

## Per-resource checklist

For each item in scope, fill the audit-flagged hooks (state upgrader / MaxItems:1 / custom importer / timeouts / sensitive / write-only) only when present. Mark each row only after `verify_tests.sh --migrated-files <file>` exits 0. Negative gate (no `terraform-plugin-sdk/v2` substring) must hold per file.

### Resources (110)

Audit-derived flag legend: **MI1**=MaxItems:1, **SU**=StateUpgrader, **CI**=Custom Importer (composite-ID parse), **TO**=Timeouts, **CD**=CustomizeDiff, **SF**=StateFunc, **DSF**=DiffSuppressFunc, **NB**=Nested blocks, **VR**=Conflicts/Exactly/AtLeastOneOf/RequiredWith, **RSC**=retry.StateChangeConf, **CDC**=customdiff combinators, **HSJ**=helper/structure JSON, **SC**=*schema.Set cast, **OC**=Optional+Computed-without-UseStateForUnknown, **DwC**=Default-without-Computed (Step 2 fix), **FNC**=ForceNew+Computed (Step 2 fix).

| Resource | Flags |
|---|---|
| openstack_bgpvpn_network_associate_v2 | CI, OC |
| openstack_bgpvpn_port_associate_v2 | CI, NB, SC, OC |
| openstack_bgpvpn_router_associate_v2 | CI, OC |
| openstack_bgpvpn_v2 | CI, TO, SC, OC |
| openstack_blockstorage_qos_association_v3 | CI, OC |
| openstack_blockstorage_qos_v3 | CI, OC |
| openstack_blockstorage_quotaset_v3 | CI, TO, OC |
| openstack_blockstorage_volume_attach_v3 | TO, RSC, OC |
| openstack_blockstorage_volume_type_access_v3 | CI, OC |
| openstack_blockstorage_volume_type_v3 | CI, OC |
| openstack_blockstorage_volume_v3 | CI, TO, NB, VR, RSC, SC, OC, DwC |
| openstack_compute_aggregate_v2 | CI, TO, SC, OC |
| openstack_compute_flavor_access_v2 | CI, OC |
| openstack_compute_flavor_v2 | CI, OC, DwC |
| openstack_compute_instance_v2 | **MI1, CI, TO, CD, SF, DSF, NB, VR, RSC, CDC, SC, OC, DwC** |
| openstack_compute_interface_attach_v2 | CI, TO, VR, RSC, OC |
| openstack_compute_keypair_v2 | CI, OC |
| openstack_compute_quotaset_v2 | CI, TO, OC |
| openstack_compute_servergroup_v2 | MI1, CI, NB, OC |
| openstack_compute_volume_attach_v2 | MI1, CI, TO, NB, RSC, SC, OC, DwC |
| openstack_containerinfra_cluster_v1 | CI, TO, RSC, OC, DwC, **FNC** |
| openstack_containerinfra_clustertemplate_v1 | CI, TO, OC, **FNC** |
| openstack_containerinfra_nodegroup_v1 | CI, TO, RSC, OC, DwC, **FNC** |
| openstack_db_configuration_v1 | TO, NB, RSC, OC |
| openstack_db_database_v1 | CI, TO, RSC, OC |
| openstack_db_instance_v1 | MI1, TO, NB, VR, RSC, OC |
| openstack_db_user_v1 | TO, RSC, SC, OC |
| openstack_dns_quota_v2 | CI, TO, OC |
| openstack_dns_recordset_v2 | CI, TO, RSC, OC, DwC |
| openstack_dns_transfer_accept_v2 | CI, TO, RSC, OC, DwC |
| openstack_dns_transfer_request_v2 | CI, TO, RSC, OC, DwC |
| openstack_dns_zone_share_v2 | CI, OC |
| openstack_dns_zone_v2 | CI, TO, RSC, SC, OC, DwC |
| openstack_fw_group_v2 | CI, TO, VR, RSC, SC, OC, DwC |
| openstack_fw_policy_v2 | CI, TO, VR, RSC, OC |
| openstack_fw_rule_v2 | CI, VR, OC, DwC |
| openstack_identity_application_credential_v3 | CI, NB, SC, OC, DwC |
| openstack_identity_ec2_credential_v3 | CI, OC |
| openstack_identity_endpoint_v3 | CI, OC, DwC |
| openstack_identity_group_v3 | CI, OC |
| openstack_identity_inherit_role_assignment_v3 | CI, VR, OC |
| openstack_identity_limit_v3 | CI, OC |
| openstack_identity_project_v3 | CI, OC, DwC |
| openstack_identity_registered_limit_v3 | CI, OC |
| openstack_identity_role_assignment_v3 | CI, VR, OC |
| openstack_identity_role_v3 | CI, OC |
| openstack_identity_service_v3 | CI, OC, DwC |
| openstack_identity_user_membership_v3 | CI, OC |
| openstack_identity_user_v3 | CI, NB, OC, DwC |
| openstack_images_image_access_accept_v2 | CI, OC |
| openstack_images_image_access_v2 | CI, OC |
| openstack_images_image_v2 | CI, TO, CD, VR, RSC, SC, OC, DwC |
| openstack_keymanager_container_v1 | CI, TO, NB, RSC, SC, OC |
| openstack_keymanager_order_v1 | MI1, CI, TO, NB, RSC, OC |
| openstack_keymanager_secret_v1 | CI, TO, DSF, RSC, OC |
| openstack_lb_flavor_v2 | CI, TO, OC |
| openstack_lb_flavorprofile_v2 | CI, TO, SF, DSF, HSJ, OC |
| openstack_lb_l7policy_v2 | CI, TO, VR, OC, DwC |
| openstack_lb_l7rule_v2 | CI, TO, OC, DwC |
| openstack_lb_listener_v2 | CI, TO, DSF, VR, SC, OC, DwC |
| openstack_lb_loadbalancer_v2 | CI, TO, VR, OC, DwC |
| openstack_lb_member_v2 | CI, TO, OC, DwC |
| openstack_lb_members_v2 | CI, TO, NB, SC, OC, DwC |
| openstack_lb_monitor_v2 | CI, TO, OC, DwC |
| openstack_lb_pool_v2 | MI1, CI, TO, NB, VR, SC, OC, DwC |
| openstack_lb_quota_v2 | CI, TO, OC |
| openstack_networking_address_group_v2 | CI, TO, RSC, SC, OC |
| openstack_networking_addressscope_v2 | CI, TO, RSC, OC, DwC |
| openstack_networking_bgp_peer_v2 | CI, TO, CD, OC, DwC |
| openstack_networking_bgp_speaker_v2 | CI, TO, NB, SC, OC, DwC |
| openstack_networking_floatingip_associate_v2 | CI, OC |
| openstack_networking_floatingip_v2 | CI, TO, VR, RSC, OC |
| openstack_networking_network_v2 | CI, TO, NB, RSC, SC, OC |
| openstack_networking_port_secgroup_associate_v2 | CI, SC, OC, DwC |
| openstack_networking_port_v2 | MI1, CI, TO, SF, DSF, NB, VR, RSC, HSJ, SC, OC, DwC |
| openstack_networking_portforwarding_v2 | TO, RSC, OC |
| openstack_networking_qos_bandwidth_limit_rule_v2 | CI, TO, RSC, OC, DwC |
| openstack_networking_qos_dscp_marking_rule_v2 | CI, TO, RSC, OC |
| openstack_networking_qos_minimum_bandwidth_rule_v2 | CI, TO, RSC, OC, DwC |
| openstack_networking_qos_policy_v2 | CI, TO, RSC, OC |
| openstack_networking_quota_v2 | CI, TO, OC |
| openstack_networking_rbac_policy_v2 | CI, OC |
| openstack_networking_router_interface_v2 | CI, TO, RSC, OC, DwC |
| openstack_networking_router_route_v2 | CI, OC |
| openstack_networking_router_routes_v2 | CI, NB, SC, OC |
| openstack_networking_router_v2 | MI1, CI, TO, NB, VR, RSC, SC, OC, DwC |
| openstack_networking_secgroup_rule_v2 | CI, TO, SF, VR, RSC, OC |
| openstack_networking_secgroup_v2 | CI, TO, RSC, OC |
| openstack_networking_segment_v2 | CI, OC |
| openstack_networking_subnet_route_v2 | CI, OC |
| openstack_networking_subnet_v2 | CI, TO, NB, VR, RSC, SC, OC, DwC |
| openstack_networking_subnetpool_v2 | CI, TO, RSC, OC |
| openstack_networking_trunk_v2 | TO, NB, RSC, SC, OC |
| openstack_objectstorage_account_v1 | CI, DSF, OC |
| openstack_objectstorage_container_v1 | MI1, **SU**, CI, NB, VR, SC, OC, DwC |
| openstack_objectstorage_object_v1 | DSF, VR, OC, DwC |
| openstack_objectstorage_tempurl_v1 | OC, DwC |
| openstack_orchestration_stack_v1 | CI, TO, NB, RSC, OC |
| openstack_sharedfilesystem_securityservice_v2 | CI, TO, OC |
| openstack_sharedfilesystem_share_access_v2 | CI, TO, RSC, OC |
| openstack_sharedfilesystem_share_v2 | CI, TO, NB, RSC, OC, DwC |
| openstack_sharedfilesystem_sharenetwork_v2 | CI, TO, SC, OC |
| openstack_taas_tap_mirror_v2 | MI1, CI, NB, VR, OC |
| openstack_vpnaas_endpoint_group_v2 | CI, TO, RSC, SC, OC |
| openstack_vpnaas_ike_policy_v2 | CI, TO, NB, RSC, SC, OC, DwC |
| openstack_vpnaas_ipsec_policy_v2 | CI, TO, NB, RSC, SC, OC |
| openstack_vpnaas_service_v2 | CI, TO, RSC, OC, DwC |
| openstack_vpnaas_site_connection_v2 | CI, TO, NB, RSC, SC, OC, DwC |
| openstack_workflow_cron_trigger_v2 | CI, OC |
| openstack_compute_quotaset_v2 (alt) | (covered above) |
| openstack_lb_quota_v2 (alt) | (covered above) |
| openstack_networking_quota_v2 (alt) | (covered above) |
| openstack_dns_quota_v2 (alt) | (covered above) |

Per resource, mark off when:

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled (state upgrader / MaxItems:1 / importer / timeouts / sensitive — only those present)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### Data sources (63)

All data sources predominantly trip the **OC** (Optional+Computed-without-UseStateForUnknown) flag. Audit-flagged additional patterns called out below. Migrate in waves once `NewFrameworkProvider()` is wired.

| Data source | Additional flags beyond OC |
|---|---|
| openstack_blockstorage_availability_zones_v3 | DwC |
| openstack_blockstorage_quotaset_v3 | — |
| openstack_blockstorage_snapshot_v3 | — |
| openstack_blockstorage_volume_v3 | NB |
| openstack_compute_aggregate_v2 | — |
| openstack_compute_availability_zones_v2 | DwC |
| openstack_compute_flavor_v2 | VR |
| openstack_compute_hypervisor_v2 | — |
| openstack_compute_instance_v2 | NB |
| openstack_compute_keypair_v2 | — |
| openstack_compute_limits_v2 | — |
| openstack_compute_quotaset_v2 | — |
| openstack_compute_servergroup_v2 | NB |
| openstack_containerinfra_cluster_v1 | — |
| openstack_containerinfra_clustertemplate_v1 | — |
| openstack_containerinfra_nodegroup_v1 | — |
| openstack_dns_zone_share_v2 | — |
| openstack_dns_zone_v2 | — |
| openstack_fw_group_v2 | VR |
| openstack_fw_policy_v2 | VR |
| openstack_fw_rule_v2 | VR |
| openstack_identity_auth_scope_v3 | NB |
| openstack_identity_endpoint_v3 | DwC |
| openstack_identity_group_v3 | — |
| openstack_identity_project_ids_v3 | VR, DwC |
| openstack_identity_project_v3 | VR, DwC |
| openstack_identity_role_v3 | — |
| openstack_identity_service_v3 | — |
| openstack_identity_user_v3 | — |
| openstack_images_image_ids_v2 | VR, SC, DwC |
| openstack_images_image_v2 | VR, SC, DwC |
| openstack_keymanager_container_v1 | NB |
| openstack_keymanager_secret_v1 | (no extra flags from audit) |
| openstack_lb_flavor_v2 | VR |
| openstack_lb_flavorprofile_v2 | VR |
| openstack_lb_listener_v2 | NB, VR |
| openstack_lb_loadbalancer_v2 | NB, VR |
| openstack_lb_member_v2 | VR |
| openstack_lb_monitor_v2 | NB, VR |
| openstack_lb_pool_v2 | NB, VR |
| openstack_loadbalancer_flavor_v2 | VR |
| openstack_networking_addressscope_v2 | (no extra flags from audit) |
| openstack_networking_floatingip_v2 | (no extra flags from audit) |
| openstack_networking_network_v2 | NB |
| openstack_networking_port_ids_v2 | SC |
| openstack_networking_port_v2 | NB, SC |
| openstack_networking_qos_bandwidth_limit_rule_v2 | — |
| openstack_networking_qos_dscp_marking_rule_v2 | — |
| openstack_networking_qos_minimum_bandwidth_rule_v2 | — |
| openstack_networking_qos_policy_v2 | — |
| openstack_networking_quota_v2 | — |
| openstack_networking_router_v2 | NB |
| openstack_networking_secgroup_v2 | — |
| openstack_networking_segment_v2 | — |
| openstack_networking_subnet_ids_v2 | VR |
| openstack_networking_subnet_v2 | NB |
| openstack_networking_subnetpool_v2 | — |
| openstack_networking_trunk_v2 | NB |
| openstack_sharedfilesystem_availability_zones_v2 | (no extra flags from audit) |
| openstack_sharedfilesystem_share_v2 | NB |
| openstack_sharedfilesystem_sharenetwork_v2 | — |
| openstack_sharedfilesystem_snapshot_v2 | — |
| openstack_workflow_cron_trigger_v2 | — |
| openstack_workflow_workflow_v2 | — |

Per data source, mark off when:

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any — VR flag)
- [ ] Tests pass green; negative gate satisfied

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
