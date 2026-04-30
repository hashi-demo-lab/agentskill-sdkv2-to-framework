# Attribute types

## Quick summary
- Primitive attributes: `StringAttribute`, `Int64Attribute`, `Float64Attribute`, `BoolAttribute`, `NumberAttribute` (arbitrary precision).
- Collection attributes (homogeneous): `ListAttribute`, `SetAttribute`, `MapAttribute` — each takes an `ElementType`.
- Nested attributes (heterogeneous, structured): `SingleNestedAttribute`, `ListNestedAttribute`, `SetNestedAttribute`, `MapNestedAttribute`.
- Object attribute (rare, fixed-shape, no list/set wrapping): `ObjectAttribute` — usually replace with `SingleNestedAttribute` for clarity.
- The blocks-vs-nested-attributes decision is in `blocks.md`. Default to nested attributes for new work; blocks exist mostly for backward compatibility.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Attributes/blocks migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/attributes-blocks)
- [Attribute types (Int32, Float32, Dynamic added in v1.10/v1.7)](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/attributes)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
