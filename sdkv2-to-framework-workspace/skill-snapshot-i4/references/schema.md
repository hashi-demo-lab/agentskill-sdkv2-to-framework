# Schema migration

## Quick summary
- SDKv2 schemas are inline `map[string]*schema.Schema{...}` on a `schema.Provider`/`schema.Resource`; framework schemas are returned by a dedicated `Schema(ctx, req, resp)` method on the provider/resource/data-source type.
- Framework splits attributes (`Attributes` field) from blocks (`Blocks` field); SDKv2 conflated them.
- Most primitive types map 1:1: `schema.TypeString` → `schema.StringAttribute{...}` etc. Nested types change shape — see `attributes.md`.
- `ForceNew` becomes a plan modifier; `Default` becomes a separate `defaults` package; validators become a slice.
- The framework has separate schema packages per concept: `provider/schema`, `resource/schema`, `datasource/schema`. Don't import the wrong one.

## Schema location

**SDKv2** (inline on the resource/provider):
```go
func resourceFoo() *schema.Resource {
    return &schema.Resource{
        Schema: map[string]*schema.Schema{
            "name": {Type: schema.TypeString, Required: true},
        },
    }
}
```

**Framework** (dedicated method):
```go
func (r *fooResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "name": schema.StringAttribute{Required: true},
        },
    }
}
```

The same shape applies to data sources (`datasource.SchemaRequest/Response`) and the provider itself (`provider.SchemaRequest/Response`).

## Import paths — the three schema packages

The framework has **three separate** schema packages. Importing the wrong one causes mysterious type errors:

| Use site | Import |
|---|---|
| Resource schemas | `github.com/hashicorp/terraform-plugin-framework/resource/schema` |
| Data source schemas | `github.com/hashicorp/terraform-plugin-framework/datasource/schema` |
| Provider config schema | `github.com/hashicorp/terraform-plugin-framework/provider/schema` |

In a file that defines both a resource and a data source, alias them:
```go
import (
    rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
    dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)
```

## Primitive attribute conversions

| SDKv2 (`schema.Schema{...}`) | Framework attribute |
|---|---|
| `Type: schema.TypeString` | `schema.StringAttribute{}` |
| `Type: schema.TypeInt` | `schema.Int64Attribute{}` is the default. If the underlying API is genuinely 32-bit (protobuf `int32`, JSON `format: int32`), use `schema.Int32Attribute{}` instead — added in framework v1.10.0. |
| `Type: schema.TypeFloat` | `schema.Float64Attribute{}` |
| `Type: schema.TypeBool` | `schema.BoolAttribute{}` |
| `Type: schema.TypeList, Elem: &schema.Schema{Type: schema.TypeString}` | `schema.ListAttribute{ElementType: types.StringType}` |
| `Type: schema.TypeSet, Elem: &schema.Schema{Type: schema.TypeString}` | `schema.SetAttribute{ElementType: types.StringType}` |
| `Type: schema.TypeMap, Elem: &schema.Schema{Type: schema.TypeString}` | `schema.MapAttribute{ElementType: types.StringType}` |
| `Type: schema.TypeList, Elem: &schema.Resource{...}` | **decision**: nested attribute (`schema.ListNestedAttribute`) or block (`schema.ListNestedBlock`) — see `blocks.md` |

`schema.TypeInt` is 32-bit in SDKv2 (Go `int`); the framework default is `Int64`. The framework also has `Int32Attribute` and `Float32Attribute` (added v1.10.0) for cases where the underlying API contract is genuinely 32-bit — prefer those when truthful 32-bit semantics matter (avoids silently allowing values outside `int32` range).

## Common field translations

| SDKv2 field | Framework equivalent |
|---|---|
| `Required: true` | `Required: true` (same) |
| `Optional: true` | `Optional: true` (same) |
| `Computed: true` | `Computed: true` (same) |
| `ForceNew: true` | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` — see `plan-modifiers.md` |
| `Default: "foo"` | a `Default:` field on the attribute, populated from the `defaults` package: `Default: stringdefault.StaticString("foo")` — NOT a plan modifier |
| `Sensitive: true` | `Sensitive: true` (same) — see `sensitive-and-writeonly.md` |
| `Description: "..."` | `Description: "..."` (same); also `MarkdownDescription:` |
| `Deprecated: "..."` | `DeprecationMessage: "..."` |
| `ValidateFunc: validation.StringLenBetween(1,255)` | `Validators: []validator.String{stringvalidator.LengthBetween(1,255)}` — see `validators.md` |
| `DiffSuppressFunc` | usually a plan modifier or custom type — see `plan-modifiers.md`/`state-and-types.md` |
| `StateFunc` | a custom type with normalisation in its `ValueFromString`/equivalent — see `state-and-types.md` |
| `ConflictsWith: []string{"other"}` | `Validators: []validator.String{stringvalidator.ConflictsWith(path.Expressions{path.MatchRoot("other")})}` |
| `ExactlyOneOf` / `AtLeastOneOf` / `RequiredWith` | corresponding `*validator.*` from `terraform-plugin-framework-validators` |
| `Set: schema.HashString` (and other `SchemaSetFunc`s) | **delete** — not needed, framework handles set uniqueness |

## Worked example — small resource schema

**SDKv2**:
```go
&schema.Resource{
    Schema: map[string]*schema.Schema{
        "name": {
            Type:         schema.TypeString,
            Required:     true,
            ForceNew:     true,
            ValidateFunc: validation.StringLenBetween(1, 64),
        },
        "tags": {
            Type:     schema.TypeMap,
            Optional: true,
            Elem:     &schema.Schema{Type: schema.TypeString},
        },
        "created_at": {
            Type:     schema.TypeString,
            Computed: true,
        },
    },
}
```

**Framework**:
```go
resp.Schema = schema.Schema{
    Attributes: map[string]schema.Attribute{
        "name": schema.StringAttribute{
            Required: true,
            PlanModifiers: []planmodifier.String{
                stringplanmodifier.RequiresReplace(),
            },
            Validators: []validator.String{
                stringvalidator.LengthBetween(1, 64),
            },
        },
        "tags": schema.MapAttribute{
            Optional:    true,
            ElementType: types.StringType,
        },
        "created_at": schema.StringAttribute{
            Computed: true,
        },
    },
}
```

## Where this differs by use site

- **Resource schemas** support `Blocks`, `Version` (for state-upgraders), and resource-specific plan modifiers.
- **Data source schemas** support `Blocks` but not `Version` or plan modifiers.
- **Provider schemas** are simpler still — typically only attributes, no blocks, no plan modifiers.
