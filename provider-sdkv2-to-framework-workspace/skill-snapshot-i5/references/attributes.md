# Attribute types

## Quick summary
- Primitive attributes: `StringAttribute`, `Int64Attribute`, `Float64Attribute`, `BoolAttribute`, `NumberAttribute` (arbitrary precision).
- Collection attributes (homogeneous): `ListAttribute`, `SetAttribute`, `MapAttribute` — each takes an `ElementType`.
- Nested attributes (heterogeneous, structured): `SingleNestedAttribute`, `ListNestedAttribute`, `SetNestedAttribute`, `MapNestedAttribute`.
- Object attribute (rare, fixed-shape, no list/set wrapping): `ObjectAttribute` — usually replace with `SingleNestedAttribute` for clarity.
- The blocks-vs-nested-attributes decision is in `blocks.md`. Default to nested attributes for new work; blocks exist mostly for backward compatibility.

## Table of contents
1. Primitive attributes
2. Collection attributes (`ListAttribute`, `SetAttribute`, `MapAttribute`)
3. Nested attributes (single/list/set/map nested)
4. `ObjectAttribute` and when not to use it
5. Choosing between collections and nested attributes
6. SDKv2 → framework cheatsheet

## 1. Primitive attributes

| Framework | Maps from SDKv2 | Notes |
|---|---|---|
| `schema.StringAttribute` | `Type: schema.TypeString` | |
| `schema.Int64Attribute` | `Type: schema.TypeInt` | default — SDKv2 `int` widens to `int64` |
| `schema.Int32Attribute` | `Type: schema.TypeInt` (when API contract is genuinely 32-bit) | added in framework v1.10.0 (July 2024). Use for fields backed by protobuf `int32`, or APIs where the JSON schema says `int32` |
| `schema.Float64Attribute` | `Type: schema.TypeFloat` | default |
| `schema.Float32Attribute` | `Type: schema.TypeFloat` (when API contract is 32-bit) | added in v1.10.0 alongside `Int32Attribute` |
| `schema.BoolAttribute` | `Type: schema.TypeBool` | |
| `schema.NumberAttribute` | (no direct equivalent; rare) | for arbitrary-precision numbers; almost never needed |
| `schema.DynamicAttribute` | rare — SDKv2 `TypeAny`-shaped passthrough fields | added in v1.7.0 (March 2024). Use only when the attribute is a deliberate untyped passthrough (raw JSON blob the provider re-emits without inspection). Most "I don't know the schema" cases are actually nested attributes in disguise — push back before reaching for `DynamicAttribute`. |

All take the same lifecycle fields: `Required` / `Optional` / `Computed`, plus `Description`, `MarkdownDescription`, `DeprecationMessage`, `Sensitive`, `Validators`, `PlanModifiers`, `Default`.

**Populate `MarkdownDescription`, not just `Description`.** `terraform-plugin-docs` (the registry-docs generator) reads `MarkdownDescription` first and falls back to `Description`. Migrating from SDKv2's `Description` is a free opportunity to upgrade the field — supply both, with the markdown form being the richer of the two. Generated registry docs will surface the markdown variant.

```go
"name": schema.StringAttribute{
    Required:    true,
    Description: "Human-readable name.",
    Validators:  []validator.String{stringvalidator.LengthBetween(1, 64)},
}
```

## 2. Collection attributes (homogeneous element type)

`ListAttribute`, `SetAttribute`, `MapAttribute` — for collections where every element has the *same* primitive type:

```go
"tags": schema.MapAttribute{
    Optional:    true,
    ElementType: types.StringType,
}
```

`ElementType` accepts any `attr.Type`: `types.StringType`, `types.Int64Type`, `types.BoolType`, `types.Float64Type`, `types.NumberType`.

These map from SDKv2:
```go
// SDKv2
"tags": {Type: schema.TypeMap, Elem: &schema.Schema{Type: schema.TypeString}}
```

## 3. Nested attributes (heterogeneous, structured)

When the element is a struct (multiple typed fields), use a *nested* attribute. There are four shapes:

| Framework | Shape |
|---|---|
| `SingleNestedAttribute` | exactly one nested object — replaces SDKv2 `MaxItems: 1` blocks |
| `ListNestedAttribute` | ordered list of nested objects |
| `SetNestedAttribute` | unordered set of nested objects (order doesn't matter, duplicates not allowed) |
| `MapNestedAttribute` | map keyed by string of nested objects |

```go
"endpoint": schema.SingleNestedAttribute{
    Optional: true,
    Attributes: map[string]schema.Attribute{
        "url":  schema.StringAttribute{Required: true},
        "port": schema.Int64Attribute{Optional: true},
    },
}
```

Lists/sets/maps of nested:
```go
"rules": schema.ListNestedAttribute{
    Optional: true,
    NestedObject: schema.NestedAttributeObject{
        Attributes: map[string]schema.Attribute{
            "action": schema.StringAttribute{Required: true},
            "cidr":   schema.StringAttribute{Required: true},
        },
    },
}
```

Note the wrapper: `NestedObject: schema.NestedAttributeObject{Attributes: ...}` — a small bit of extra ceremony compared to `Attributes: ...` directly.

## 4. `ObjectAttribute`

`ObjectAttribute` exists for cases where you want a fixed-shape object as an `attr.Type` value, but you don't want plan/validators/etc. on individual nested fields. It's rare in real providers — almost always `SingleNestedAttribute` is what you want, because it gives you per-field validation and plan modifiers.

## 5. Choosing between collection and nested attributes

The rule of thumb:

- All elements have the **same primitive type**? → collection (`ListAttribute` / `SetAttribute` / `MapAttribute`).
- Elements have **multiple fields with different types**? → nested (`ListNestedAttribute` / etc.).

The migration trap: a SDKv2 `TypeList` with `Elem: &schema.Resource{...}` is a *nested* shape, not a collection. Don't translate it as `ListAttribute`. See `blocks.md` for the further block-vs-nested-attribute decision.

## 6. SDKv2 → framework cheatsheet

| SDKv2 | Framework |
|---|---|
| `Type: TypeString` | `StringAttribute{}` |
| `Type: TypeInt` | `Int64Attribute{}` |
| `Type: TypeFloat` | `Float64Attribute{}` |
| `Type: TypeBool` | `BoolAttribute{}` |
| `Type: TypeList, Elem: &schema.Schema{Type: TypeString}` | `ListAttribute{ElementType: types.StringType}` |
| `Type: TypeSet, Elem: &schema.Schema{Type: TypeString}` | `SetAttribute{ElementType: types.StringType}` |
| `Type: TypeMap, Elem: &schema.Schema{Type: TypeString}` | `MapAttribute{ElementType: types.StringType}` |
| `Type: TypeList, Elem: &schema.Resource{...}, MaxItems: 1` | `SingleNestedAttribute{Attributes: ...}` (or `ListNestedBlock` if backward-compat requires it — see `blocks.md`) |
| `Type: TypeList, Elem: &schema.Resource{...}` (no MaxItems) | `ListNestedAttribute{NestedObject: schema.NestedAttributeObject{Attributes: ...}}` *or* `ListNestedBlock` |
| `Type: TypeSet, Elem: &schema.Resource{...}` | `SetNestedAttribute{NestedObject: ...}` *or* `SetNestedBlock` |

The italicised "or block" choices are decided in `blocks.md`.
