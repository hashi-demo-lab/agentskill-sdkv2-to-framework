# Schema migration

## Quick summary
- SDKv2 schemas are inline `map[string]*schema.Schema{...}` on a `schema.Provider`/`schema.Resource`; framework schemas are returned by a dedicated `Schema(ctx, req, resp)` method on the provider/resource/data-source type.
- Framework splits attributes (`Attributes` field) from blocks (`Blocks` field); SDKv2 conflated them.
- Most primitive types map 1:1: `schema.TypeString` → `schema.StringAttribute{...}` etc. Nested types change shape — see `attributes.md`.
- `ForceNew` becomes a plan modifier; `Default` becomes a separate `defaults` package; validators become a slice.
- The framework has separate schema packages per concept: `provider/schema`, `resource/schema`, `datasource/schema`. Don't import the wrong one.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Schema migration overview](https://developer.hashicorp.com/terraform/plugin/framework/migrating/schema)
- [resource/schema package source](https://github.com/hashicorp/terraform-plugin-framework/tree/main/resource/schema)
- [datasource/schema package source](https://github.com/hashicorp/terraform-plugin-framework/tree/main/datasource/schema)
- [provider/schema package source](https://github.com/hashicorp/terraform-plugin-framework/tree/main/provider/schema)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
