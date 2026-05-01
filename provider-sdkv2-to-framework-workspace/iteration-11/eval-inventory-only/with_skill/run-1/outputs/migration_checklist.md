# Migration plan: terraform-provider-openstack

## Pre-flight

- [ ] Audit complete — artefact: `audit_report.md`
- [ ] User has confirmed scope (whole provider? specific resources?)
- [ ] Decision: protocol v5 or v6 (default v6 — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end
- [ ] Test-side scope reviewed (see "Test-side migration scope" below — this is a provider-level prerequisite, not a per-resource step)

## Test-side migration scope

The audit report's "Test-file findings" section lists per-test-file counts of SDKv2 patterns plus any "Shared test infrastructure" files it identified. Test migration is a **provider-level prerequisite** — per-resource test rewriting (workflow step 7) cannot succeed until shared test plumbing has a framework-side path.

### Shared test infrastructure

List the files the audit flagged as shared test infrastructure (typically under `acceptance/`, `testutil/`, `internal/test/`, or files like `provider_test.go`). These hold globals like `TestAccProvider`, `testAccProvider`, `testAccProviders`, `TestAccProtoV6ProviderFactories`, helper functions like `GetTestClient()`, `Meta()`-derived accessors. Migrate these *first*; per-resource test rewrites depend on them.

- `openstack/provider_test.go` — purpose: provider-level test configuration, schema-provider-type references (schema-provider-type=1); holds `testAccProvider`/`TestAccProvider` globals and shared `PreCheck` logic
- [ ] Framework provider factory wired up (e.g. `testAccProtoV6ProviderFactories = map[string]func()(...){...}` using `providerserver.NewProtocol6WithError(NewFrameworkProvider())`)
- [ ] Shared `TestAccProvider`/`testAccProvider` references migrated to the framework provider, OR proxied via `terraform-plugin-mux` for a transitional period (note: this skill's scope is single-release; muxing is out of scope)
- [ ] Test-side client/meta accessors migrated (`acceptance.GetTestClient()`, `Meta()`-derived helpers) — many references in per-resource tests will break until these land

### Test-side counts (from audit)

- Test files audited: 300
- `ProviderFactories` references to flip → `ProtoV6ProviderFactories`: 526
- `resource.Test`/`UnitTest`/`ParallelTest` calls (must use `terraform-plugin-testing/helper/resource`): 526
- `*schema.ResourceData` and `d.X` calls in tests (rare; usually inside helper functions): 35 (d.Id=13, d.Get=11, d.Set=11)

### Test-side negative gate (run at workflow step 11)

- [ ] No surviving `terraform-plugin-sdk/v2/helper/resource` imports in any `*_test.go`
- [ ] No surviving `terraform-plugin-sdk/v2/terraform` imports in any `*_test.go`
- [ ] No `*schema.Provider`/`*schema.ResourceData` references in test files

## HashiCorp single-release-cycle steps

- [ ] **1.** Ensure the SDKv2 version of your provider has sufficient test coverage and that all tests pass.
- [ ] **2.** Review your provider code for SDKv2 resource data consistency errors, which might affect migrating to the framework. SDKv2 silently demotes data-consistency errors (e.g. d.Set mismatches, unset computed attributes) to warnings; the framework surfaces them as hard errors. Find and fix all data-consistency issues in SDKv2 form first — run `go vet ./...` and audit every `d.Set` call for type mismatches — before touching framework code.
- [ ] **3.** Serve your provider via the framework. (`main.go` swap; protocol v6 chosen)
- [ ] **4.** Update the provider definition to use the framework.
- [ ] **5.** Update the provider schema to use the framework.
- [ ] **6.** Update each of the provider's resources, data sources, and other Terraform features to use the framework.
- [ ] **7.** Update related tests to use the framework, and ensure that the tests fail. *(TDD gate: write or update the tests first, run them and confirm they are RED before writing any migration code. Red-then-green proves the test actually exercises the change. Quote the failing output verbatim in the per-resource checklist row. If no test exists for a resource, write a minimal one before proceeding — never skip the gate. See `references/workflow.md` for the 4-step procedure and acceptable/unacceptable failure shapes.)*
- [ ] **8.** Migrate the resource or data source.
- [ ] **9.** Verify that related tests now pass.
- [ ] **10.** Remove any remaining references to SDKv2 libraries.
- [ ] **11.** Verify that all of your tests continue to pass.
- [ ] **12.** Once you have completed the migration process and verified that your provider works as expected, release a new version of your provider.

## Provider-level migration notes

- **174** resource/data-source constructors detected (each is a `func ...() *schema.Resource`)
- **637** `ForceNew: true` occurrences → each becomes `stringplanmodifier.RequiresReplace()` (or typed equivalent)
- **140** `ValidateFunc`/`ValidateDiagFunc` → translate to framework validators
- **110** `ConflictsWith`/`ExactlyOneOf`/`AtLeastOneOf`/`RequiredWith` → framework validators package
- **121** `retry.StateChangeConf` usages → replace with inline ticker loops (no framework equivalent)
- **101** custom `Importer` blocks (14 non-passthrough; 87 passthrough via `ImportStatePassthroughContext`)
- **68** `Timeouts` fields → framework `timeouts` package
- **20** `Sensitive: true` attributes → check write-only candidacy
- **11** `MaxItems:1` + nested Elem → block-vs-nested-attribute decision required
- **8** `DiffSuppressFunc` → analyse intent; translate to custom type or plan modifier
- **4** `StateFunc` → translate to custom type
- **3** `CustomizeDiff` → translate to `ModifyPlan`
- **1** `StateUpgraders`/`SchemaVersion` → flatten to single-step framework upgrader
- **34** `schema.EnvDefaultFunc`/`MultiEnvDefaultFunc` in provider config path (`provider.go`)

### Priority order recommendation

1. **`openstack/provider.go`** — must be migrated before any resource (score 64.75; 34 EnvDefaultFunc, 31 d.Get)
2. **`openstack/provider_test.go`** — shared test infra; flip to `ProtoV6ProviderFactories` before per-resource test rewrites
3. **`openstack/resource_openstack_compute_instance_v2.go`** — highest complexity (score 162.75); 1 StateFunc, 1 CustomizeDiff, 13 retry.StateChangeConf, 1 MaxItems:1, 1 custom Importer; address last
4. Resources with only `ImportStatePassthroughContext` (87 of 101 importers) — lowest risk; good migration warm-up

## Per-resource checklist

Repeat one row per resource and per data source. For each, fill the audit-flagged hooks (state upgrader, MaxItems:1 block, custom importer, timeouts, sensitive/write-only) only when present. Mark each row only after `verify_tests.sh --migrated-files <file>` exits 0.

---

### openstack_bgpvpn_network_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_bgpvpn_port_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_bgpvpn_router_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_bgpvpn_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_blockstorage_qos_association_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_blockstorage_qos_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_blockstorage_quotaset_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_blockstorage_volume_attach_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (replace with inline ticker loop — no framework equivalent); full async state-change refactor required
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_blockstorage_volume_type_access_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_blockstorage_volume_type_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_blockstorage_volume_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_aggregate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_flavor_access_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_flavor_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_instance_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); CustomizeDiff with customdiff combinators → multi-leg ModifyPlan (read `references/plan-modifiers.md`); StateFunc (becomes custom type — read `references/state-and-types.md`); DiffSuppressFunc (analyse intent — read `references/state-and-types.md` + `references/plan-modifiers.md`); nested Elem &Resource (deep block-vs-attribute decision tree); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf × 13 (full async state-change refactor); TypeSet expansion → typed model
- **NOTE: Highest-complexity file (score 162.75). Migrate last. Requires Pre-flight C think pass before editing.**
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_interface_attach_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_keypair_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_quotaset_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_servergroup_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); nested Elem &Resource (block-vs-nested decision)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_compute_volume_attach_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource; retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_containerinfra_cluster_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_containerinfra_clustertemplate_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_containerinfra_nodegroup_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_db_configuration_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_db_database_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_db_instance_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (deep block-vs-attribute decision tree); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_db_user_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_dns_quota_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_dns_recordset_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_dns_transfer_accept_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_dns_transfer_request_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_dns_zone_share_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_dns_zone_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_fw_group_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor — also in `openstack/fw_group_v2.go`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_fw_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_fw_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_application_credential_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_ec2_credential_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_endpoint_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_group_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_inherit_role_assignment_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_limit_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_project_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_registered_limit_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_role_assignment_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_role_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_service_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_user_membership_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_identity_user_v3

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_images_image_access_accept_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_images_image_access_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_images_image_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); CustomizeDiff → ModifyPlan (read `references/plan-modifiers.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model; *schema.ResourceDiff in `openstack/images_image_v2.go` (port to ModifyPlan body)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_keymanager_container_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_keymanager_order_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource; retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_keymanager_secret_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); DiffSuppressFunc (analyse intent — read `references/state-and-types.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_flavor_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_flavorprofile_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); StateFunc (becomes custom type — read `references/state-and-types.md`); DiffSuppressFunc (analyse intent); helper/structure JSON normalisation (refactor to custom type or plan modifier)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_l7policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_l7rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_listener_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); DiffSuppressFunc (analyse intent — read `references/state-and-types.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_loadbalancer_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_member_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_members_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_monitor_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_pool_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource; ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_lb_quota_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_address_group_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_addressscope_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_bgp_peer_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); CustomizeDiff → ModifyPlan (read `references/plan-modifiers.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_bgp_speaker_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_floatingip_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_floatingip_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_network_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_port_secgroup_associate_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_port_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); StateFunc (becomes custom type — read `references/state-and-types.md`); DiffSuppressFunc (analyse intent); nested Elem &Resource; ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf; helper/structure JSON normalisation (refactor to custom type); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_portforwarding_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_qos_bandwidth_limit_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_qos_dscp_marking_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_qos_minimum_bandwidth_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_qos_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_quota_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_rbac_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_router_interface_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_router_route_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_router_routes_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_router_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource; ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_secgroup_rule_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); StateFunc (becomes custom type — read `references/state-and-types.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_secgroup_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_segment_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_subnet_route_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_subnet_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_subnetpool_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_networking_trunk_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_objectstorage_account_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); DiffSuppressFunc (analyse intent — read `references/state-and-types.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_objectstorage_container_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); **StateUpgraders/SchemaVersion** (single-step semantics — read `references/state-upgrade.md`; flatten V0→V1 inline, do not chain); custom Importer (composite ID parsing — read `references/import.md`); nested Elem &Resource; ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); TypeSet expansion → typed model; migration logic in `openstack/migrate_resource_openstack_objectstorage_container_v1.go`
- [ ] Pre-flight C think pass written (state upgrade chain composition)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_objectstorage_object_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: DiffSuppressFunc (analyse intent — read `references/state-and-types.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_objectstorage_tempurl_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: none flagged (lower complexity)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_orchestration_stack_v1

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_sharedfilesystem_securityservice_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_sharedfilesystem_share_access_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_sharedfilesystem_share_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_sharedfilesystem_sharenetwork_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_taas_tap_mirror_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: MaxItems:1 (block-vs-nested-attribute decision — read `references/blocks.md`); custom Importer (composite ID parsing — read `references/import.md`); nested Elem &Resource; ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_vpnaas_endpoint_group_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_vpnaas_ike_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_vpnaas_ipsec_policy_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_vpnaas_service_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); retry.StateChangeConf (full async state-change refactor)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_vpnaas_site_connection

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`); Timeouts (framework `timeouts` package — read `references/timeouts.md`); nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); retry.StateChangeConf (full async state-change refactor); TypeSet expansion → typed model
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### openstack_workflow_cron_trigger_v2

- [ ] Tests written/updated and run *red* (workflow step 7 TDD gate)
- [ ] Schema + CRUD migrated
- [ ] Audit-flagged hooks: custom Importer (composite ID parsing — read `references/import.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

---

## Data source checklist

### openstack_blockstorage_availability_zones_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

### openstack_blockstorage_quotaset_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_blockstorage_snapshot_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_blockstorage_volume_v3 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_aggregate_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_availability_zones_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_flavor_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_hypervisor_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_instance_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_keypair_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_limits_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_quotaset_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_compute_servergroup_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_containerinfra_cluster_v1 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_containerinfra_clustertemplate_v1 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_containerinfra_nodegroup_v1 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_dns_zone_share_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_dns_zone_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_fw_group_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_fw_policy_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_fw_rule_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_auth_scope_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_endpoint_v3 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_group_v3 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_project_ids_v3

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_project_v3 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_role_v3 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_service_v3 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_identity_user_v3 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_images_image_ids_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); TypeSet expansion → typed model
- [ ] Tests pass green; negative gate satisfied

### openstack_images_image_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`); TypeSet expansion → typed model
- [ ] Tests pass green; negative gate satisfied

### openstack_keymanager_container_v1 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_keymanager_secret_v1 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_lb_flavor_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_lb_flavorprofile_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_lb_listener_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_lb_loadbalancer_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_lb_member_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_lb_monitor_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_lb_pool_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_loadbalancer_flavor_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_addressscope_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_floatingip_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_network_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_port_ids_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: TypeSet expansion → typed model
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_port_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`); TypeSet expansion → typed model
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_qos_bandwidth_limit_rule_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_qos_dscp_marking_rule_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_qos_minimum_bandwidth_rule_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_qos_policy_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_quota_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_router_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_secgroup_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_segment_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_subnet_ids_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: ConflictsWith/ExactlyOneOf/etc. (read `references/validators.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_subnet_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_subnetpool_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_networking_trunk_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_sharedfilesystem_availability_zones_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_sharedfilesystem_share_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Audit-flagged hooks: nested Elem &Resource (block-vs-nested decision — read `references/blocks.md`)
- [ ] Tests pass green; negative gate satisfied

### openstack_sharedfilesystem_sharenetwork_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_sharedfilesystem_snapshot_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_workflow_cron_trigger_v2 (data source)

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

### openstack_workflow_workflow_v2

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated
- [ ] Tests pass green; negative gate satisfied

---

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
