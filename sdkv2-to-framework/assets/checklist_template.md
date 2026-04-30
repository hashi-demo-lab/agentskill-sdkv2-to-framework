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
