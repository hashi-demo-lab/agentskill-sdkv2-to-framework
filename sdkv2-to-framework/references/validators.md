# Validator migration

## Quick summary
- SDKv2 `ValidateFunc` / `ValidateDiagFunc` → framework `Validators` field, a typed slice per attribute kind.
- The companion module `github.com/hashicorp/terraform-plugin-framework-validators` provides ports of every common SDKv2 validator (length, regex, one-of, between, etc.).
- Cross-attribute checks (`ConflictsWith`, `RequiredWith`, `ExactlyOneOf`, `AtLeastOneOf`) now live in the validators package too — no special schema fields.
- For schema-wide cross-attribute logic, implement `ResourceWithConfigValidators` (returns config validators) or `ResourceWithValidateConfig` (one-off function).
- Keep validators pure: validate config syntactically. For "is this value reachable in the API?" checks, do those in `Read`/`Create` and return diagnostics there.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Validator migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/validators)
- [terraform-plugin-framework-validators](https://github.com/hashicorp/terraform-plugin-framework-validators)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
