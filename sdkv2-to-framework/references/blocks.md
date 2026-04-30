# Blocks vs nested attributes

## Quick summary
- The framework has both **nested attributes** (`{ ... }` configuration syntax) and **blocks** (`block_name { ... }` configuration syntax). Practitioners write them differently.
- SDKv2 `TypeList`/`TypeSet` with `Elem: &schema.Resource{...}` is a block in user-facing HCL. The migration choice is whether to keep it a block or convert to a nested attribute.
- **`MaxItems: 1` blocks should usually become `SingleNestedAttribute`** — except when backward-compat with existing user configs forbids the syntactic change.
- **True repeating blocks** (`MinItems > 0`, `MaxItems > 1` or unset) should usually stay as blocks (`ListNestedBlock` / `SetNestedBlock`) for backward-compat — converting changes the HCL syntax users wrote.
- New-greenfield resources should prefer nested attributes; migration of existing resources should prefer the option that doesn't change user HCL.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Blocks vs nested attributes](https://developer.hashicorp.com/terraform/plugin/framework/migrating/attributes-blocks/blocks)
- [Nested attributes](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes/list-nested)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
