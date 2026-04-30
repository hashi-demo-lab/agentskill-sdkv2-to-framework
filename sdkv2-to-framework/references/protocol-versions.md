# Protocol v5 vs v6

## Quick summary
- The framework supports both Terraform Plugin Protocol v5 and v6. SDKv2 defaults to v5; `terraform-plugin-framework` defaults to v6.
- **Default to v6 for single-release migrations.** It's the framework's native protocol and unlocks attribute features that v5 cannot represent natively.
- v5 vs v6 is decided in `main.go` (the `providerserver.Serve` call). Pick at workflow step 3.
- Choose v5 only if you need backward compatibility with Terraform <0.15.4. Modern Terraform (0.15.4+) speaks both, transparently.
- v6 requires a Terraform CLI version that supports it. The release notes for the major version bump should call this out so practitioners on ancient Terraform versions know to upgrade.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Provider protocols](https://developer.hashicorp.com/terraform/plugin/framework/migrating/provider-protocols)
- [providerserver source](https://github.com/hashicorp/terraform-plugin-framework/blob/main/providerserver/serve.go)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
