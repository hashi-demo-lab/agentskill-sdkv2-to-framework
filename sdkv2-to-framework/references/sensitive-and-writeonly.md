# Sensitive and write-only attributes

## Quick summary
- `Sensitive: true` is unchanged in spelling and behaviour: the value is hidden in plans/logs but still stored in state.
- The framework adds **write-only** attributes (`WriteOnly: true`) — supplied by the practitioner but never persisted to state. Useful for credentials Terraform doesn't need to round-trip.
- Write-only requires `terraform-plugin-framework` **v1.14.0+** (technical preview) and Terraform **1.11+**, with v1.17.0+ recommended for production use. The v1.14 preview ships the field; v1.17 stabilises behaviour.
- **`WriteOnly` and `Computed` should not be combined on the same attribute.** A write-only value isn't persisted; making it computed would need the framework to materialise a value in state, which contradicts write-only's whole point. Terraform CLI rejects configurations that use both at apply time. If you need a "default-or-rotate" pattern, use a separate computed attribute that mirrors the rotation source rather than trying to mix the two on one attribute.
- **Nested `WriteOnly` cascades.** If a parent (`SingleNestedAttribute`, `ListNestedAttribute`, etc.) is `WriteOnly`, every child attribute should also be `WriteOnly`, and none should be `Computed`. The framework's `ValidateImplementation` does not currently catch every violation at provider-boot time — so test this end-to-end. Run an acceptance step that exercises the attribute and confirm there's no apply-time error.
- Sensitive attributes still appear in `terraform show -json`, just redacted in human-readable output. Don't rely on `Sensitive` alone for security.
- When migrating, sweep for SDKv2 patterns where `Sensitive: true` was being used as a poor man's write-only (e.g., setting state to a hash placeholder); these may be candidates for the new write-only.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Sensitive values](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes#sensitive)
- [Write-only attributes](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/write-only)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
