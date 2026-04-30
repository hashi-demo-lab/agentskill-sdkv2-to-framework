# Acceptance and unit testing

## Quick summary
- Use `terraform-plugin-testing` (current name; replaces SDKv2's `helper/resource`) — the same package the framework uses.
- `ProviderFactories` → `ProtoV6ProviderFactories` (or `ProtoV5ProviderFactories` for v5).
- `r.Test(t, resource.TestCase{...})` is unchanged in shape; the factories field is what differs.
- TDD ordering matters here (workflow step 7): change the test first, run it red, then migrate the resource. Tests written *after* the migration inherit the migrator's blind spots.
- `TestProvider` (calling `provider.InternalValidate()`) is your cheap, fast schema-validity check that catches a huge class of errors without `TF_ACC`.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Testing migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/testing)
- [terraform-plugin-testing repo](https://github.com/hashicorp/terraform-plugin-testing)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
