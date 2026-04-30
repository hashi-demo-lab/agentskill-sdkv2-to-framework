# The 12-step single-release-cycle workflow

## Quick summary
- This file expands HashiCorp's [single-release-cycle workflow](https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-in-a-single-release-cycle) with per-step "do not skip" notes.
- Steps 1, 2, 7 are the most commonly skipped. Step 1 establishes a green baseline; step 2 surfaces latent SDKv2 errors before they become hard framework errors; step 7 is the TDD gate (tests fail first).
- Steps 3–6 are mechanical conversions, paced by the audit.
- Steps 8–11 are per-resource migration loops with verification gates between each.
- Step 12 is release; do not version-bump until the full suite is green and no SDKv2 imports remain.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [HashiCorp single-release workflow](https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
