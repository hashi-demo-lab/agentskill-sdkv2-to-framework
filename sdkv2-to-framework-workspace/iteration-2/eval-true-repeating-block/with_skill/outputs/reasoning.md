# Schema migration reasoning — openstack_compute_volume_attach_v2

## Source audit summary

| Concern | Finding |
|---|---|
| Block decision | `vendor_options` is `TypeSet` with `MinItems:1, MaxItems:1` — true repeating block, kept as block (see below) |
| State upgrade | No `SchemaVersion` / `StateUpgraders` — no action needed |
| Import shape | `schema.ImportStatePassthroughContext` — passthrough, not migrated here (schema-only task) |
| Timeouts | `Create` and `Delete` each 10 minutes — migrated via `terraform-plugin-framework-timeouts` |

---

## Decision 1 — vendor_options: block vs SingleNestedAttribute

### SDKv2 original

```go
"vendor_options": {
    Type:     schema.TypeSet,
    Optional: true,
    ForceNew: true,
    MinItems: 1,
    MaxItems: 1,
    Elem: &schema.Resource{ ... },
},
```

`MinItems: 1` makes this a **true repeating block** — even though `MaxItems: 1` caps it at one
instance, practitioners must supply it as a `vendor_options { ... }` HCL block, not as an
assignment. Per the skill's `blocks.md` rule:

> True repeating blocks (MinItems > 0, MaxItems > 1 or unset) should usually stay as blocks
> (ListNestedBlock / SetNestedBlock) for backward-compat — converting changes the HCL syntax users wrote.

A `TypeSet` (as opposed to `TypeList`) maps to `SetNestedBlock`, not `ListNestedBlock`, to
preserve set-semantics (order-independent, deduplication).

Converting to `SingleNestedAttribute` would change the practitioner HCL from:

```hcl
vendor_options {
  ignore_volume_confirmation = true
}
```

to:

```hcl
vendor_options = {
  ignore_volume_confirmation = true
}
```

This is a breaking syntactic change that must not be introduced without a major-version bump.

### Migration output

```go
Blocks: map[string]schema.Block{
    "vendor_options": schema.SetNestedBlock{
        NestedObject: schema.NestedBlockObject{
            Attributes: map[string]schema.Attribute{
                "ignore_volume_confirmation": schema.BoolAttribute{
                    Optional: true,
                    Computed: true,
                    Default:  booldefault.StaticBool(false),
                },
            },
        },
    },
},
```

Note: `ForceNew` on the `vendor_options` block itself is **not** expressible as a block-level
plan modifier in the framework (plan modifiers live on attributes, not blocks). The replacement
strategy is to add `RequiresReplace()` to each attribute inside the block, or handle it at
the resource level via `ModifyPlan`. For this schema-only migration the inner attribute
`ignore_volume_confirmation` does not individually carry `ForceNew` in the SDKv2 source;
the `ForceNew` was on the block container. This nuance should be addressed when the CRUD
methods are migrated (add a resource-level `ModifyPlan` that calls `resp.RequiresReplace()`
when `vendor_options` changes).

---

## Decision 2 — Default for ignore_volume_confirmation

SDKv2: `Default: false` on a `TypeBool` attribute.

Framework rule (from `plan-modifiers.md`): `Default` is **not** a plan modifier. It lives in
the `defaults` package:

```go
Default: booldefault.StaticBool(false),
```

Wiring this into `PlanModifiers` instead would be a compile error.

Because the attribute now has a `Default`, it must also be `Computed: true` — the framework
requires `Computed: true` on any attribute that can be set by the provider (including via
`Default`) when the user omits it.

---

## Decision 3 — ForceNew attributes

All attributes with `ForceNew: true` in SDKv2 become `RequiresReplace()` plan modifiers:

| Attribute | SDKv2 | Framework |
|---|---|---|
| `region` | `ForceNew: true` | `stringplanmodifier.RequiresReplace()` |
| `instance_id` | `ForceNew: true` | `stringplanmodifier.RequiresReplace()` |
| `volume_id` | `ForceNew: true` | `stringplanmodifier.RequiresReplace()` |
| `multiattach` | `ForceNew: true` | `boolplanmodifier.RequiresReplace()` |
| `tag` | `ForceNew: true` | `stringplanmodifier.RequiresReplace()` |

`region` also gets `UseStateForUnknown()` because it is `Computed` and should not be
re-derived on each plan when nothing changed.

---

## Decision 4 — Timeouts

SDKv2 `Timeouts: &schema.ResourceTimeout{Create: ..., Delete: ...}` becomes the
`timeouts.Attributes(ctx, timeouts.Opts{...})` helper from
`github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts`. This is the
canonical framework replacement and keeps the `timeouts { create = "..." }` block syntax
practitioners expect.

---

## What is NOT in this file (schema-only scope)

- CRUD method rewrites (`Create`, `Read`, `Update`, `Delete`)
- `ImportState` method (replacing `schema.ImportStatePassthroughContext`)
- Model struct definitions (`computeVolumeAttachV2Model`)
- `vendor_options` block-level `ForceNew` → `ModifyPlan` (flagged above, needs CRUD pass)
- Unit / acceptance tests
