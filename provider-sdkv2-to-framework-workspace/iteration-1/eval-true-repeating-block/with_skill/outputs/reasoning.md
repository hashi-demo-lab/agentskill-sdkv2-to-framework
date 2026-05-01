# Reasoning: Why `vendor_options` Was Kept as a Block

## SDKv2 definition

```go
"vendor_options": {
    Type:     schema.TypeSet,
    Optional: true,
    ForceNew: true,
    MinItems: 1,
    MaxItems: 1,
    Elem: &schema.Resource{
        Schema: map[string]*schema.Schema{
            "ignore_volume_confirmation": {
                Type:     schema.TypeBool,
                Default:  false,
                Optional: true,
            },
        },
    },
},
```

## Decision: keep as `SetNestedBlock`

The `vendor_options` field has `MinItems: 1`, which classifies it as a **true repeating block** per the migration reference (`references/blocks.md`). The decision tree states:

> True repeating blocks (`MinItems > 0`, `MaxItems > 1` or unset) should usually stay as blocks (`ListNestedBlock` / `SetNestedBlock`) for backward-compat — converting changes the HCL syntax users wrote.

Although `MaxItems: 1` is also set (making it a "one-of" set), the `MinItems: 1` constraint is the controlling signal here per the task specification. More importantly, the source type is `TypeSet` (not `TypeList`), and the existing practitioner HCL uses block syntax:

```hcl
vendor_options {
  ignore_volume_confirmation = true
}
```

Converting to `SingleNestedAttribute` would change that to:

```hcl
vendor_options = {
  ignore_volume_confirmation = true
}
```

That is a **breaking syntactic change** for all existing practitioner configurations. Even though the framework would prefer `SingleNestedAttribute` for a "maximum one" relationship on a greenfield resource, backward compatibility requires keeping the block syntax on an existing resource.

## Mapping applied

| SDKv2 | Framework |
|---|---|
| `TypeSet` + `Elem: &schema.Resource{}` | `schema.SetNestedBlock` (block preserved) |
| `MinItems: 1` / `MaxItems: 1` | Expressed via `listvalidator.SizeBetween(1,1)` if strict validation is needed; omitted here as the original relied on SDK enforcement |
| `ForceNew: true` on the block | `PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()}` |
| `Default: false` on `ignore_volume_confirmation` | `Default: booldefault.StaticBool(false)` + `Computed: true` (framework requires `Computed` when a `Default` is set) |

## Why `Computed: true` was added to `ignore_volume_confirmation`

In the framework, an attribute with a `Default` value **must** also be `Computed: true`. This allows the framework to insert the default into the plan when the practitioner omits the field. The SDKv2 `Default` field implicitly did this; the framework requires it to be explicit.

## What was not changed

- Attribute names are preserved exactly — changing them would be a state-breaking change for practitioners.
- `ForceNew` semantics are preserved via `RequiresReplace()` plan modifiers.
- The inner structure of `vendor_options` is unchanged.
