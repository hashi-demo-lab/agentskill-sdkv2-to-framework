# Resource identity (`ResourceWithIdentity`)

## Quick summary
- **Resource identity** is the framework's first-class answer to composite-ID resources (region+id, project+region+name, etc.). Shipped in `terraform-plugin-framework` v1.15.0 (May 2025).
- A resource defines an *identity schema* alongside its main schema. The identity carries the practitioner-facing addressing data (region, account, project) separately from configuration.
- Implement `resource.ResourceWithIdentity` (and optionally `ResourceWithUpgradeIdentity` for identity-schema versioning).
- Every CRUD request/response now has an `Identity` field. `ImportStatePassthroughWithIdentity` is a helper that mirrors a *single* identity attribute into a *single* state attribute — useful for the simple "ID is the only addressing field" case. For multi-segment composite IDs you implement `ImportState` manually using `req.Identity.GetAttribute` / `resp.State.SetAttribute` per attribute.
- **When to migrate to identity rather than parse a composite ID by hand**: when practitioners are on Terraform 1.12+ and want to write `import { identity = { ... } }` blocks, identity is the framework-idiomatic answer. **Keep the legacy `req.ID` string-parse path in your `ImportState` too** — practitioners on Terraform <1.12 and CLI-driven `terraform import` still need the legacy form.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Resource identity](https://developer.hashicorp.com/terraform/plugin/framework/resources/identity)
- [identityschema package source](https://github.com/hashicorp/terraform-plugin-framework/tree/main/resource/identityschema)
- [ImportStatePassthroughWithIdentity source](https://github.com/hashicorp/terraform-plugin-framework/blob/main/resource/import_state.go)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
