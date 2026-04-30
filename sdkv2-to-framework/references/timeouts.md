# Timeouts

## Quick summary
- SDKv2's `Timeouts: &schema.ResourceTimeout{Create: ...}` field is gone. The framework moves timeouts to a separate package: `github.com/hashicorp/terraform-plugin-framework-timeouts`.
- Timeouts are exposed to practitioners as a nested attribute on the resource (`timeouts { create = "30m" }` block in HCL).
- Inside CRUD methods, you read the configured timeout from state/plan via the timeouts helper, then use it in your context (`context.WithTimeout`).
- Defaults are set on the schema attribute itself; per-operation timeouts (Create/Read/Update/Delete) are independent.
- This is opt-in — if your provider didn't have `Timeouts` before, you don't need to add it now.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Timeouts migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/timeouts)
- [terraform-plugin-framework-timeouts](https://github.com/hashicorp/terraform-plugin-framework-timeouts)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
