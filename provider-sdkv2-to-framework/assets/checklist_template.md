# Migration plan: {{provider_name}}

## Pre-flight

- [ ] Mux check — confirmed not a `terraform-plugin-mux` / multi-release / staged migration (Pre-flight 0)
- [ ] Audit complete — artefact: `{{audit_file}}` (Pre-flight A)
- [ ] User has confirmed scope (whole provider? specific resources?) (Pre-flight B)
- [ ] Decision: protocol v5 or v6 (default v6 — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end
- [ ] Per-resource think pass written for each audit-flagged resource (block decision / state upgrade / import shape) (Pre-flight C)
- [ ] Test-side scope reviewed (see "Test-side migration scope" below — this is a provider-level prerequisite, not a per-resource step)

## Test-side migration scope

The audit report's "Test-file findings" section lists per-test-file counts of SDKv2 patterns plus any "Shared test infrastructure" files it identified. Test migration is a **provider-level prerequisite** — per-resource test rewriting (workflow step 7) cannot succeed until shared test plumbing has a framework-side path.

### Shared test infrastructure

List the files the audit flagged as shared test infrastructure (typically under `acceptance/`, `testutil/`, `internal/test/`, or files like `provider_test.go`). These hold globals like `TestAccProvider`, `testAccProvider`, `testAccProviders`, `TestAccProtoV6ProviderFactories`, helper functions like `GetTestClient()`, `Meta()`-derived accessors. Migrate these *first*; per-resource test rewrites depend on them.

- {{shared_test_helper_path_1}} — purpose:
- {{shared_test_helper_path_2}} — purpose:
- {{shared_test_helper_path_3}} — purpose:
- [ ] Framework provider factory wired up (e.g. `testAccProtoV6ProviderFactories = map[string]func()(...){...}` using `providerserver.NewProtocol6WithError(NewFrameworkProvider())`)
- [ ] Shared `TestAccProvider`/`testAccProvider` references migrated to the framework provider, OR proxied via `terraform-plugin-mux` for a transitional period (note: this skill's scope is single-release; muxing is out of scope)
- [ ] Test-side client/meta accessors migrated (`acceptance.GetTestClient()`, `Meta()`-derived helpers) — many references in per-resource tests will break until these land

### Test-side counts (from audit)

- Test files audited: {{test_file_count}}
- `ProviderFactories` references to flip → `ProtoV6ProviderFactories`: {{test_provider_factories_count}}
- `resource.Test`/`UnitTest`/`ParallelTest` calls (must use `terraform-plugin-testing/helper/resource`): {{test_resource_test_count}}
- `*schema.ResourceData` and `d.X` calls in tests (rare; usually inside helper functions): {{test_d_calls_count}}

### Test-side negative gate (run at workflow step 11)

- [ ] No surviving `terraform-plugin-sdk/v2/helper/resource` imports in any `*_test.go`
- [ ] No surviving `terraform-plugin-sdk/v2/terraform` imports in any `*_test.go`
- [ ] No `*schema.Provider`/`*schema.ResourceData` references in test files

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

## Per-resource checklist

Repeat one row per resource and per data source. For each, fill the audit-flagged hooks (state upgrader, MaxItems:1 block, custom importer, timeouts, sensitive/write-only) only when present. Mark each row only after `verify_tests.sh --migrated-files <file>` exits 0.

### {{resource_name}}

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema + CRUD migrated (per-element refs in `SKILL.md` table)
- [ ] Audit-flagged hooks handled (state upgrader / MaxItems:1 / importer / timeouts / sensitive — only those present)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### {{data_source_name}}

- [ ] Tests written/updated and run *red*
- [ ] Schema + `Read` migrated (`references/data-sources.md`)
- [ ] Validators translated (only if SDKv2 had any)
- [ ] Tests pass green; negative gate satisfied

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (if v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
