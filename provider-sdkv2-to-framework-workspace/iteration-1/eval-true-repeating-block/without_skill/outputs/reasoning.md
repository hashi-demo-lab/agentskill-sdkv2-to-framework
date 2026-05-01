# Migration Reasoning: openstack_compute_volume_attach_v2 Schema

## Source

`resource_openstack_compute_volume_attach_v2.go` — terraform-plugin-sdk/v2

## Target

terraform-plugin-framework (`schema.Schema` in `resource/schema`)

---

## Attribute-by-attribute decisions

| SDK attribute | SDK type | Framework equivalent | Notes |
|---|---|---|---|
| `region` | `TypeString`, Optional, Computed, ForceNew | `schema.StringAttribute` Optional+Computed + `RequiresReplace()` | |
| `instance_id` | `TypeString`, Required, ForceNew | `schema.StringAttribute` Required + `RequiresReplace()` | |
| `volume_id` | `TypeString`, Required, ForceNew | `schema.StringAttribute` Required + `RequiresReplace()` | |
| `device` | `TypeString`, Optional, Computed | `schema.StringAttribute` Optional+Computed | No ForceNew → no RequiresReplace |
| `multiattach` | `TypeBool`, Optional, ForceNew | `schema.BoolAttribute` Optional + `RequiresReplace()` | |
| `tag` | `TypeString`, Optional, ForceNew | `schema.StringAttribute` Optional + `RequiresReplace()` | |
| `vendor_options` | `TypeSet`, Optional, ForceNew, MinItems:1, MaxItems:1, Elem:Resource | `schema.SetNestedBlock` | **See block decision below** |
| `vendor_options.ignore_volume_confirmation` | `TypeBool`, Optional, Default:false | `schema.BoolAttribute` Optional+Computed, `booldefault.StaticBool(false)` | |

---

## Why vendor_options stays a block (SetNestedBlock), not a nested attribute

The original SDK definition is:

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

`MinItems: 1` means the block must appear at least once when present; this
qualifies as a **true repeating block** under the migration rule.

Practitioner HCL for a block looks like:

```hcl
resource "openstack_compute_volume_attach_v2" "example" {
  instance_id = "..."
  volume_id   = "..."

  vendor_options {
    ignore_volume_confirmation = true
  }
}
```

If `vendor_options` were migrated to a `schema.SetNestedAttribute`, the HCL
syntax would change to object/set-of-objects attribute syntax:

```hcl
vendor_options = [{
  ignore_volume_confirmation = true
}]
```

This would be a **breaking change** for all existing practitioner configurations,
requiring a rewrite. To preserve backward compatibility and HCL syntax
compatibility, `vendor_options` is kept as `schema.SetNestedBlock`.

### MaxItems enforcement in the framework

The framework does not have a native `MaxItems` or `MinItems` constraint on
blocks. To replicate `MinItems:1 / MaxItems:1` behaviour at the framework layer,
a custom `ValidateConfig` or `ConfigValidators` implementation should be added
(outside the scope of this schema migration, but noted for completeness).

---

## ForceNew → RequiresReplace

Every attribute that had `ForceNew: true` in the SDK is given the
`planmodifier.RequiresReplace()` plan modifier in the framework. This preserves
the destroy-and-recreate behaviour on attribute changes.

## Default values

`ignore_volume_confirmation` had `Default: false`. In the framework this is
expressed with `booldefault.StaticBool(false)` and the attribute must also be
marked `Computed: true` so the framework can store the default in state.

## id attribute

The framework automatically manages the `id` attribute; no explicit declaration
is required in the schema.

## Timeouts

The SDK `Timeouts` block does not map to a schema attribute in the framework.
Timeouts are instead handled via the `resource.ResourceWithConfigValidators`
or `timeouts` helper package. The values (10 minutes each) are preserved as
constants in the migrated file for use in CRUD implementations.
