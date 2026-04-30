# Migration plan: {{provider_name}}

## Pre-flight

- [ ] Audit complete — artefact: `{{audit_file}}`
- [ ] User has confirmed scope (whole provider? specific resources?)
- [ ] Decision: protocol v5 or v6 (default v6 — see `references/protocol-versions.md`)
- [ ] Files needing manual review have been read end-to-end

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

Repeat one block per resource and one per data source. Mark each box only after the resource passes `verify_tests.sh --migrated-files <file>`.

### {{resource_name}}

- [ ] Tests written/updated and run *red* (workflow step 7)
- [ ] Schema converted (`references/schema.md`)
- [ ] CRUD methods implemented (`references/resources.md`)
- [ ] Validators translated (`references/validators.md`)
- [ ] Plan modifiers + defaults (note: `Default` is *not* a plan modifier) (`references/plan-modifiers.md`)
- [ ] State upgraders translated, if applicable — single-step semantics (`references/state-upgrade.md`)
- [ ] Import method implemented, if applicable (`references/import.md`)
- [ ] Timeouts wired up, if applicable (`references/timeouts.md`)
- [ ] Sensitive / write-only attributes handled (`references/sensitive-and-writeonly.md`)
- [ ] Tests pass green (`verify_tests.sh` exit 0)
- [ ] Negative gate satisfied — file no longer imports `terraform-plugin-sdk/v2`

### {{data_source_name}}

- [ ] Tests written/updated and run *red*
- [ ] Schema converted (`references/data-sources.md`)
- [ ] `Read` method implemented
- [ ] Validators translated
- [ ] Tests pass green
- [ ] Negative gate satisfied

## Final sweep (before release)

- [ ] `grep -rl 'terraform-plugin-sdk/v2' .` returns nothing outside `vendor/` and `.git/`
- [ ] `go mod tidy` and `go.mod` no longer requires `terraform-plugin-sdk/v2`
- [ ] Full suite green: `go test ./...` and (if creds) `TF_ACC=1 go test ./...`
- [ ] Changelog entry mentions the protocol bump (if v6) and minimum Terraform CLI version
- [ ] Major version bump in `version` field
