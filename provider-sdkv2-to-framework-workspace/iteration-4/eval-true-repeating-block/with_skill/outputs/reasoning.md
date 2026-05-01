# Reasoning: openstack_compute_volume_attach_v2 schema migration

## Source analysis

The SDKv2 schema for `openstack_compute_volume_attach_v2` has seven attributes:

| Attribute | SDKv2 type | Flags |
|---|---|---|
| `region` | TypeString | Optional, Computed, ForceNew |
| `instance_id` | TypeString | Required, ForceNew |
| `volume_id` | TypeString | Required, ForceNew |
| `device` | TypeString | Optional, Computed |
| `multiattach` | TypeBool | Optional, ForceNew |
| `tag` | TypeString | Optional, ForceNew |
| `vendor_options` | TypeSet of &schema.Resource | Optional, ForceNew, MinItems:1, MaxItems:1 |

`vendor_options` contains one nested field:

| Field | Type | Flags |
|---|---|---|
| `ignore_volume_confirmation` | TypeBool | Optional, Default:false |

## Key decisions

### 1. vendor_options — block vs nested attribute

`vendor_options` is `TypeSet` with `Elem: &schema.Resource{...}`. Per `references/blocks.md` the decision tree:

- `MaxItems: 1` alone would suggest converting to `SingleNestedAttribute`.
- BUT `MinItems: 1` is set — this is explicitly the "true repeating block" case (`MinItems > 0`).
- Practitioners already write `vendor_options { ... }` block syntax.
- No major-version bump is in scope.

Decision: **keep as `SetNestedBlock`**. This preserves the practitioner HCL unchanged.

The `TypeSet` (not `TypeList`) maps to `SetNestedBlock`, not `ListNestedBlock`.

### 2. MinItems:1 / MaxItems:1 enforcement

The framework cannot mark blocks `Required`. The equivalent constraint is a `Validators` field on the block using `setvalidator.SizeBetween(1, 1)` from `terraform-plugin-framework-validators`.

### 3. ForceNew on vendor_options

`ForceNew: true` on the `TypeSet` becomes `setplanmodifier.RequiresReplace()` on the `SetNestedBlock`'s `PlanModifiers`.

### 4. ignore_volume_confirmation Default

SDKv2 `Default: false` on a `TypeBool` becomes:
- `Computed: true` (required by the framework when `Default` is set, so the provider can insert the default into the plan)
- `Default: booldefault.StaticBool(false)` (from `resource/schema/booldefault`)

This is a common pitfall: omitting `Computed: true` when adding `Default` causes a framework runtime error.

### 5. ForceNew on primitive attributes

All `ForceNew: true` scalar fields become `RequiresReplace()` plan modifiers from the type-specific planmodifier package (e.g., `stringplanmodifier.RequiresReplace()`).

### 6. Computed+Optional fields — UseStateForUnknown

`device` and `region` are `Optional + Computed`. Without `UseStateForUnknown()`, every plan shows these as `(known after apply)` even when the value is stable. Adding `stringplanmodifier.UseStateForUnknown()` suppresses unnecessary churn.

For `region`, both `UseStateForUnknown()` and `RequiresReplace()` are needed (Computed+Optional+ForceNew). `UseStateForUnknown` is placed first so it fires before the replacement check.

## Import paths

| Package | Path |
|---|---|
| Resource schema types | `github.com/hashicorp/terraform-plugin-framework/resource/schema` |
| planmodifier interfaces | `github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier` |
| stringplanmodifier | `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier` |
| boolplanmodifier | `github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier` |
| setplanmodifier | `github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier` |
| booldefault | `github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault` |
| validator interfaces | `github.com/hashicorp/terraform-plugin-framework/schema/validator` |
| setvalidator | `github.com/hashicorp/terraform-plugin-framework-validators/setvalidator` |

## What is NOT in this file

Per the task constraints, this file contains only the `Schema()` method. CRUD methods (`Create`, `Read`, `Delete`), the resource type struct, `Metadata`, `Configure`, `ImportState`, `Timeouts`, and tests are excluded.
