# Blocks vs nested attributes

## Quick summary
- The framework has both **nested attributes** (`{ ... }` configuration syntax) and **blocks** (`block_name { ... }` configuration syntax). Practitioners write them differently.
- SDKv2 `TypeList`/`TypeSet` with `Elem: &schema.Resource{...}` is a block in user-facing HCL. The migration choice is whether to keep it a block or convert to a nested attribute.
- **`MaxItems: 1` blocks should usually become `SingleNestedAttribute`** — except when backward-compat with existing user configs forbids the syntactic change.
- **True repeating blocks** (`MinItems > 0`, `MaxItems > 1` or unset) should usually stay as blocks (`ListNestedBlock` / `SetNestedBlock`) for backward-compat — converting changes the HCL syntax users wrote.
- New-greenfield resources should prefer nested attributes; migration of existing resources should prefer the option that doesn't change user HCL.

## The decision tree

For each `TypeList`/`TypeSet` of `&schema.Resource{...}`, the question to answer first is *"does breaking the HCL syntax matter here?"*, not "what does the framework prefer?". Order the questions accordingly:

```
Was this a TypeList/TypeSet of &schema.Resource{} in SDKv2?
├── No → it's a primitive collection or scalar; not a block-vs-attribute decision
└── Yes:
    ├── MaxItems: 1
    │   ├── Q1: Are practitioners using block syntax (`foo { ... }`) in production
    │   │       configs? (production usage, examples in the docs, public modules
    │   │       referencing it)
    │   │   └── Yes → keep as block (Output A: ListNestedBlock + listvalidator.SizeAtMost(1),
    │   │            or SingleNestedBlock). Switching is a breaking HCL change.
    │   ├── Q2: Major-version bump or greenfield resource?
    │   │   └── Yes → convert to SingleNestedAttribute (Output B). Document the
    │   │            syntactic change in CHANGELOG.
    │   └── Q3: Can't confirm either?
    │       └── Keep as block (Output A); note in the per-resource checklist row:
    │            "kept as block; switch to single nested attribute on next major
    │            once usage confirmed safe."
    └── No MaxItems (true repeating)
        └── Practitioners wrote: rule { ... } rule { ... }
            Keep as ListNestedBlock or SetNestedBlock — converting changes HCL.
```

## Why the syntax matters

Blocks and nested attributes are semantically equivalent for most providers but **syntactically different in HCL**:

```hcl
# block syntax (SDKv2 shape)
network_interface {
  network_id = "n-123"
  subnet_id  = "s-456"
}

# nested-attribute syntax (framework default)
network_interface = {
  network_id = "n-123"
  subnet_id  = "s-456"
}
```

If users have configurations using block syntax and you convert to a nested attribute, their HCL no longer parses. That's a breaking change even though you didn't intend it.

## When `MaxItems: 1` should become `SingleNestedAttribute`

The framework prefers single nested attributes for "exactly one" relationships because they're cleaner and support per-field plan modifiers/validators. Convert to `SingleNestedAttribute` when:

- The provider is on a major-version bump anyway.
- Practitioners haven't widely adopted the `MaxItems: 1` block (e.g., it's a recently added field).
- You're willing to document the syntactic change in the changelog.

```go
// SDKv2
"endpoint": {
    Type:     schema.TypeList,
    Optional: true,
    MaxItems: 1,
    Elem: &schema.Resource{
        Schema: map[string]*schema.Schema{
            "url":  {Type: schema.TypeString, Required: true},
            "port": {Type: schema.TypeInt,    Optional: true},
        },
    },
}

// Framework
"endpoint": schema.SingleNestedAttribute{
    Optional: true,
    Attributes: map[string]schema.Attribute{
        "url":  schema.StringAttribute{Required: true},
        "port": schema.Int64Attribute{Optional: true},
    },
}
```

## When `MaxItems: 1` should stay a block

Use `ListNestedBlock` with `Validators: []validator.List{listvalidator.SizeAtMost(1)}` when:

- Practitioners depend on the block syntax in production configs.
- You can't bump the major version yet.
- Other constraints require keeping the HCL identical.

```go
// SDKv2 stays-as-block migration
import (
    "github.com/hashicorp/terraform-plugin-framework/resource/schema"
    "github.com/hashicorp/terraform-plugin-framework/schema/validator"
    "github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
)

resp.Schema = schema.Schema{
    // attributes...
    Blocks: map[string]schema.Block{
        "endpoint": schema.ListNestedBlock{
            Validators: []validator.List{listvalidator.SizeAtMost(1)},
            NestedObject: schema.NestedBlockObject{
                Attributes: map[string]schema.Attribute{
                    "url":  schema.StringAttribute{Required: true},
                    "port": schema.Int64Attribute{Optional: true},
                },
            },
        },
    },
}
```

## True repeating blocks → keep as block

For `TypeList` of `&schema.Resource` without `MaxItems: 1` (or with `MaxItems > 1`), default to `ListNestedBlock`:

```go
// SDKv2
"rule": {
    Type:     schema.TypeList,
    Optional: true,
    Elem: &schema.Resource{ /* fields */ },
}

// Framework — keep as block (preserves practitioner HCL)
Blocks: map[string]schema.Block{
    "rule": schema.ListNestedBlock{
        NestedObject: schema.NestedBlockObject{
            Attributes: map[string]schema.Attribute{ /* ... */ },
        },
    },
}
```

`SetNestedBlock` for `TypeSet`. `MapNestedBlock` does not exist — `TypeMap` of struct is rare and uses `MapNestedAttribute`.

## `SingleNestedBlock` — the third option for `MaxItems: 1`

Alongside `ListNestedBlock + listvalidator.SizeAtMost(1)`, the framework ships a `SingleNestedBlock` that's exactly "a block that holds at most one nested object". For new conservative migrations it's often more ergonomic than a list-with-validator and avoids the practitioner ever seeing the list shape in state diagnostics:

```go
Blocks: map[string]schema.Block{
    "persistence": schema.SingleNestedBlock{
        Attributes: map[string]schema.Attribute{ /* ... */ },
    },
},
```

HCL syntax is identical to `ListNestedBlock` — `persistence { ... }` — so it preserves practitioner configs the same way. Pick `SingleNestedBlock` over `ListNestedBlock + SizeAtMost(1)` when:
- The block can be entirely omitted (it's optional) AND can appear at most once.
- You don't need `MinItems: 1` semantics ("must be present") — `SingleNestedBlock` is always optional in the practitioner-facing sense.

Pick `ListNestedBlock + SizeAtMost(1)` when you genuinely need the list-shaped state path (e.g., for backward state compatibility where existing state was written under `block.0.field`).

## Block field placement

Attributes and blocks live in *separate fields* on the schema:

```go
resp.Schema = schema.Schema{
    Attributes: map[string]schema.Attribute{ /* primitive + nested attributes */ },
    Blocks:     map[string]schema.Block{      /* blocks only */ },
}
```

You cannot mix them in one map. Putting a block in `Attributes` is a compile error.

## Things blocks can't do

Blocks have lifecycle fields (`Validators`, `PlanModifiers`) but they apply to the block as a whole, not to individual fields inside. For per-field plan modifiers/validators, you need an attribute.

Blocks cannot be `Required` / `Optional` / `Computed` — block presence is determined by user HCL alone. If you need `Computed`-like semantics, use a nested attribute instead.
