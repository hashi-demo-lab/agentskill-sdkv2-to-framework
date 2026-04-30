# Reasoning — `openstack_compute_volume_attach_v2` schema migration

This document explains the decisions taken in `migrated_schema.go`, focusing
on the block-vs-attribute decision, the `TypeSet` → `SetNestedBlock` choice,
and the per-field SDKv2 → framework mappings.

## 1. Block preservation — why `vendor_options` stays a block

The user's task specification states explicitly:

> Note that this resource uses a true repeating block (MinItems > 0 or no
> MaxItems): the migration should keep block syntax to preserve practitioner
> HCL.

The `vendor_options` field in the SDKv2 source has `MinItems: 1` (and
`MaxItems: 1`), and the directive is to treat it as a block to preserve
backward-compatibility with existing practitioner HCL.

In SDKv2 the block is written as:

```hcl
resource "openstack_compute_volume_attach_v2" "x" {
  instance_id = "..."
  volume_id   = "..."

  vendor_options {
    ignore_volume_confirmation = true
  }
}
```

Converting to a nested attribute would force practitioners to rewrite that as
`vendor_options = { ignore_volume_confirmation = true }`. That is a
practitioner-visible HCL change — a breaking change in any deployment that
already uses block syntax. Per `references/blocks.md` "Why the syntax
matters", we keep block syntax.

The skill's decision tree (`references/blocks.md`):

```
Was this a TypeList/TypeSet of &schema.Resource{} in SDKv2?
└── Yes:
    ├── MaxItems: 1 → usually SingleNestedAttribute (breaking HCL change)
    └── No MaxItems / MaxItems > 1 → keep as ListNestedBlock or SetNestedBlock
```

The literal SDKv2 shape here has `MaxItems: 1`, which would normally point at
`SingleNestedAttribute`. **However**, the user has explicitly told us this is
the "true repeating block" arm and that we should keep block syntax. We
follow the user's directive: keep as block. This is also the
"backward-compat is sacred" branch the blocks.md decision tree calls out
("If backward compat is sacred, keep as ListNestedBlock with MaxItems: 1.")

## 2. `TypeSet` → `SetNestedBlock` (NOT `ListNestedBlock`)

SDKv2 has two collection kinds for repeating blocks:

| SDKv2          | Framework          |
|----------------|--------------------|
| `TypeList` of `&schema.Resource{}` | `ListNestedBlock` |
| `TypeSet`  of `&schema.Resource{}` | `SetNestedBlock`  |

The `vendor_options` SDKv2 schema uses `Type: schema.TypeSet`, so the
framework equivalent is `schema.SetNestedBlock`, not `schema.ListNestedBlock`.

Why the distinction matters even with `MaxItems: 1`:

- **Element identity / equality semantics**: `Set` treats elements as
  unordered and unique, `List` treats them as ordered. With `MaxItems: 1`
  the practical difference is small, but the underlying state representation
  and the diff output differ. Switching kinds risks spurious diffs on
  upgrade.
- **State compatibility**: existing state files were written with set
  semantics. Reading them back as a list would either fail or produce
  unexpected diffs.
- **Framework auto-uniqueness**: the framework `SetNestedBlock`
  computes set uniqueness internally; you do **not** wire up a hash
  function (the SDKv2 `Set: hashFunc` field has no equivalent and is
  not needed — see `references/schema.md` "The Set: hashFunc trap").

`MaxItems: 1` and `MinItems: 1` are therefore expressed as block-level
validators on the `SetNestedBlock`, not as a kind change:

```go
Validators: []validator.Set{
    setvalidator.SizeAtLeast(1),
    setvalidator.SizeAtMost(1),
},
```

The validators come from
`github.com/hashicorp/terraform-plugin-framework-validators/setvalidator`.

## 3. `ForceNew` on the block

SDKv2's `ForceNew: true` on `vendor_options` becomes a block-level plan
modifier:

```go
PlanModifiers: []planmodifier.Set{
    setplanmodifier.RequiresReplace(),
},
```

Blocks support `Validators` and `PlanModifiers` (which apply to the block as
a whole). They cannot be `Required` / `Optional` / `Computed` — see
`references/blocks.md` "Things blocks can't do". Block presence is
determined by the user's HCL alone, which is correct here: the SDKv2
attribute was `Optional: true`, and the framework expresses "optional block
presence" simply by the practitioner including or omitting the block.

## 4. Per-attribute mappings

| Attribute | SDKv2 | Framework |
|---|---|---|
| `region` | TypeString, Optional, Computed, ForceNew | `StringAttribute{Optional, Computed}` + `RequiresReplace()` + `UseStateForUnknown()` |
| `instance_id` | TypeString, Required, ForceNew | `StringAttribute{Required}` + `RequiresReplace()` |
| `volume_id` | TypeString, Required, ForceNew | `StringAttribute{Required}` + `RequiresReplace()` |
| `device` | TypeString, Computed, Optional | `StringAttribute{Optional, Computed}` + `UseStateForUnknown()` |
| `multiattach` | TypeBool, Optional, ForceNew | `BoolAttribute{Optional}` + `RequiresReplace()` |
| `tag` | TypeString, Optional, ForceNew | `StringAttribute{Optional}` + `RequiresReplace()` |

`UseStateForUnknown()` is added on `region` and `device` (both `Computed`
attributes that rarely change after the initial Create) to avoid noisy
`(known after apply)` plans — see `references/plan-modifiers.md`.

`UseStateForUnknown()` is also added on the implicit `id` attribute. The
framework requires resources to declare `id` explicitly when the SDKv2
resource relied on `d.SetId(...)`; making it `Computed` with
`UseStateForUnknown()` matches normal SDKv2 ID behaviour.

### Inside the block

`ignore_volume_confirmation`: SDKv2 had `TypeBool, Default: false, Optional`.
The framework requires that any attribute with a `Default` also be
`Computed` (`references/plan-modifiers.md`), so the framework form is:

```go
"ignore_volume_confirmation": schema.BoolAttribute{
    Optional: true,
    Computed: true,
    Default:  booldefault.StaticBool(false),
},
```

`Default` lives on the attribute struct itself, NOT inside `PlanModifiers`
(this is the single biggest type-error trap called out in
`references/plan-modifiers.md`).

## 5. Timeouts

SDKv2's `Timeouts: &schema.ResourceTimeout{Create: ..., Delete: ...}` is
removed and replaced with a `timeouts.Block(...)` from
`github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts`,
configured with `Create: true, Delete: true` to match the SDKv2 shape.

I used `timeouts.Block(...)` rather than `timeouts.Attributes(...)` so
practitioners' existing `timeouts { create = "10m" }` HCL keeps working
(see `references/timeouts.md` "Block vs Attributes"). The actual
`context.WithTimeout` plumbing inside CRUD methods is out of scope for this
schema-only migration step.

## 6. What was NOT changed (by design)

- No CRUD method migration (Create / Read / Delete logic stays in SDKv2 form).
- No test migration.
- No import handler migration.
- No schema name changes — every attribute and the block keep their original
  names so existing state and HCL stay valid.

## 7. Imports actually needed

| Symbol | Package |
|---|---|
| `resource.SchemaRequest`, `resource.SchemaResponse` | `terraform-plugin-framework/resource` |
| `schema.Schema`, `schema.StringAttribute`, `schema.BoolAttribute`, `schema.SetNestedBlock`, `schema.NestedBlockObject`, `schema.Block`, `schema.Attribute` | `terraform-plugin-framework/resource/schema` |
| `planmodifier.String`, `planmodifier.Bool`, `planmodifier.Set` | `terraform-plugin-framework/resource/schema/planmodifier` |
| `stringplanmodifier.RequiresReplace`, `stringplanmodifier.UseStateForUnknown` | `terraform-plugin-framework/resource/schema/stringplanmodifier` |
| `boolplanmodifier.RequiresReplace` | `terraform-plugin-framework/resource/schema/boolplanmodifier` |
| `setplanmodifier.RequiresReplace` | `terraform-plugin-framework/resource/schema/setplanmodifier` |
| `booldefault.StaticBool` | `terraform-plugin-framework/resource/schema/booldefault` |
| `validator.Set` | `terraform-plugin-framework/schema/validator` |
| `setvalidator.SizeAtLeast`, `setvalidator.SizeAtMost` | `terraform-plugin-framework-validators/setvalidator` |
| `timeouts.Block`, `timeouts.Opts`, `timeouts.Value` | `terraform-plugin-framework-timeouts/resource/timeouts` |
| `types.String`, `types.Bool`, `types.Set` | `terraform-plugin-framework/types` |
