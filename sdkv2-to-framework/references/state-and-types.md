# State, plan, and config access — typed values

## Quick summary
- SDKv2 used `*schema.ResourceData` with `d.Get("path")` and `interface{}` casting; the framework uses typed model structs and `(req|resp).{Plan|State|Config}.Get(ctx, &model)`.
- Every attribute corresponds to a typed field on the model: `types.String`, `types.Int64`, `types.Bool`, `types.List`, `types.Set`, `types.Map`, `types.Object`.
- Field names on the model use `tfsdk:"attribute_name"` struct tags to map to schema attribute names.
- The `types.*` values can be null, unknown, or known — *always* check `IsNull()`/`IsUnknown()` before calling `ValueString()`/`ValueInt64()`/etc., or you get the type's zero value.
- Custom types implement `attr.Type`/`basetypes.StringTypable` — useful for normalisation (replaces some `DiffSuppressFunc`/`StateFunc` uses).

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Accessing state/plan/config](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/accessing-values)
- [Custom types](https://developer.hashicorp.com/terraform/plugin/framework/handling-data/types/custom)
- [terraform-plugin-framework-jsontypes](https://github.com/hashicorp/terraform-plugin-framework-jsontypes)
- [terraform-plugin-framework-nettypes](https://github.com/hashicorp/terraform-plugin-framework-nettypes)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
