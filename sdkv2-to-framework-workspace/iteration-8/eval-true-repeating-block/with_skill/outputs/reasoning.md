# Schema Migration Reasoning: openstack_compute_volume_attach_v2

## Scope

This document covers the schema-only migration of `openstack_compute_volume_attach_v2` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. CRUD methods and tests are out of scope per the task definition.

---

## Attribute-by-attribute decisions

### Primitive attributes

| Attribute | SDKv2 type | Framework type | Key decisions |
|---|---|---|---|
| `region` | `TypeString`, Optional, Computed, ForceNew | `StringAttribute` | `ForceNew` → `stringplanmodifier.RequiresReplace()`; `Computed` adds `UseStateForUnknown()` to suppress noisy `(known after apply)` |
| `instance_id` | `TypeString`, Required, ForceNew | `StringAttribute` | `ForceNew` → `stringplanmodifier.RequiresReplace()` |
| `volume_id` | `TypeString`, Required, ForceNew | `StringAttribute` | `ForceNew` → `stringplanmodifier.RequiresReplace()` |
| `device` | `TypeString`, Optional, Computed | `StringAttribute` | No plan modifier needed; both Optional and Computed are preserved |
| `multiattach` | `TypeBool`, Optional, ForceNew | `BoolAttribute` | `ForceNew` → `boolplanmodifier.RequiresReplace()` |
| `tag` | `TypeString`, Optional, ForceNew | `StringAttribute` | `ForceNew` → `stringplanmodifier.RequiresReplace()` |

### The `vendor_options` block — TypeSet → SetNestedBlock

#### SDKv2 shape

```hcl
vendor_options {
  ignore_volume_confirmation = true
}
```

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

#### Decision: TypeSet → SetNestedBlock (not SetNestedAttribute)

The task explicitly flags this as a **true repeating block** (`MinItems > 0`). Under the `references/blocks.md` decision tree, `TypeSet` of `&schema.Resource{...}` maps to `SetNestedBlock` in the framework when practitioner HCL backward-compatibility must be preserved.

**Why SetNestedBlock and not SetNestedAttribute?**

The practitioner-facing HCL syntax is the determining factor:

- **Block syntax** (what practitioners currently write):
  ```hcl
  vendor_options {
    ignore_volume_confirmation = true
  }
  ```

- **Attribute syntax** (what `SetNestedAttribute` would require):
  ```hcl
  vendor_options = [
    {
      ignore_volume_confirmation = true
    }
  ]
  ```

Converting `TypeSet + Elem: &schema.Resource{}` to `SetNestedAttribute` changes the HCL syntax practitioners must write. Any existing Terraform configuration using the block form would fail to parse after the change. This is a **backward-incompatible breaking change** for all practitioners who have deployed this resource.

`SetNestedBlock` preserves the `vendor_options { ... }` block syntax exactly, maintaining full practitioner HCL backward compatibility.

#### Mapping summary

```
SDKv2: TypeSet + Elem: &schema.Resource{} → Framework: SetNestedBlock
```

This follows `references/blocks.md` rule: "True repeating blocks (`MinItems > 0`, `MaxItems > 1` or unset) should usually stay as blocks (`ListNestedBlock` / `SetNestedBlock`) for backward-compat — converting changes the HCL syntax users wrote."

The `MinItems: 1` constraint cannot be directly expressed on the block itself (blocks in the framework have no `Required`/`Optional`/`Computed`/`MinItems` fields); this would need to be enforced at the CRUD level (checking that `vendor_options` is non-empty in Create) or via a block-level validator if needed. The schema accurately reflects the SDKv2 shape at the structural level.

#### `ignore_volume_confirmation` inside the block

This inner attribute has `Default: false` in SDKv2. In the framework, `Default` is not a plan modifier — it lives in the `defaults` package:

```go
"ignore_volume_confirmation": schema.BoolAttribute{
    Optional: true,
    Computed: true,                        // required when Default is set
    Default:  booldefault.StaticBool(false),
},
```

An attribute with a `Default` must also be `Computed: true` — the framework requires this to insert the default into the plan. The `Optional: true` is preserved so practitioners can still set the value explicitly.

---

## Block placement

In the framework, attributes and blocks live in separate top-level maps on `schema.Schema`:

```go
resp.Schema = schema.Schema{
    Attributes: map[string]schema.Attribute{ /* primitives and nested attributes */ },
    Blocks:     map[string]schema.Block{      /* SetNestedBlock, ListNestedBlock, etc. */ },
}
```

Attempting to place a block type inside `Attributes` is a compile error. `vendor_options` goes in `Blocks`.

---

## What was NOT migrated

- CRUD methods (`Create`, `Read`, `Delete`) — out of scope
- `Importer` / import logic — out of scope
- `Timeouts` — out of scope
- Tests — out of scope

---

## Key references consulted

- `references/blocks.md` — block-vs-attribute decision tree; TypeSet→SetNestedBlock mapping
- `references/schema.md` — primitive attribute conversions, ForceNew→RequiresReplace
- `references/plan-modifiers.md` — defaults package, UseStateForUnknown
- `references/attributes.md` — nested attribute shapes
