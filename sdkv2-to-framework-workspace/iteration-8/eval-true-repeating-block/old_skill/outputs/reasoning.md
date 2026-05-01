# Reasoning: openstack_compute_volume_attach_v2 schema migration

## vendor_options: TypeSet → SetNestedBlock

### SDKv2 definition

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

### Decision: SetNestedBlock (not SetNestedAttribute)

The block-vs-attribute decision follows the order from SKILL.md's `blocks.md` reference:

1. **Is this TypeSet with Elem: &schema.Resource{}?** Yes — this is a nested block in SDKv2, not a flat collection.

2. **What did practitioners write?**

   ```hcl
   vendor_options {
     ignore_volume_confirmation = false
   }
   ```

   This is block syntax. Converting to `SetNestedAttribute` would require:

   ```hcl
   vendor_options = [{
     ignore_volume_confirmation = false
   }]
   ```

   That is a breaking HCL change — existing Terraform configurations would fail to parse.

3. **Is this a major-version bump or greenfield resource?** No — this is an existing mature resource in the OpenStack provider. Backward compatibility is required.

**Conclusion**: Keep as `SetNestedBlock`. The TypeSet (unordered) maps directly to `SetNestedBlock` (not `ListNestedBlock`) because the original used `schema.TypeSet`, which provides set semantics and a hash function for deduplication. The framework's `SetNestedBlock` handles uniqueness internally without needing a hash function.

### MinItems/MaxItems enforcement

SDKv2 enforced `MinItems: 1` and `MaxItems: 1` natively. In the framework, blocks do not have `MinItems`/`MaxItems` fields. These are replaced with:

```go
Validators: []validator.Set{
    setvalidator.SizeBetween(1, 1),
}
```

`setvalidator.SizeBetween(1, 1)` enforces both the minimum (≥ 1) and maximum (≤ 1) in a single validator, preserving the original semantics.

### ForceNew on vendor_options

The SDKv2 `vendor_options` block had `ForceNew: true`, meaning changing it forces resource replacement. In the framework:

- Blocks **cannot** carry `RequiresReplace` plan modifiers — plan modifiers apply to attributes, not blocks.
- The replacement logic must be handled at the resource level via `ResourceWithModifyPlan.ModifyPlan`, checking whether the `vendor_options` block changed between plan and state.

This is noted in the schema comments; it is out of scope for schema-only migration but must be addressed when CRUD methods are migrated.

## Primitive attribute mappings

| SDKv2 attribute | SDKv2 type | Framework attribute | Notes |
|---|---|---|---|
| `region` | `TypeString`, Optional+Computed, ForceNew | `StringAttribute`, Optional+Computed, `RequiresReplace()+UseStateForUnknown()` | UseStateForUnknown keeps region stable across plans when unset |
| `instance_id` | `TypeString`, Required, ForceNew | `StringAttribute`, Required, `RequiresReplace()` | |
| `volume_id` | `TypeString`, Required, ForceNew | `StringAttribute`, Required, `RequiresReplace()` | |
| `device` | `TypeString`, Optional+Computed | `StringAttribute`, Optional+Computed | No ForceNew — value can drift |
| `multiattach` | `TypeBool`, Optional, ForceNew | `BoolAttribute`, Optional, `RequiresReplace()` | |
| `tag` | `TypeString`, Optional, ForceNew | `StringAttribute`, Optional, `RequiresReplace()` | |

## Default value on ignore_volume_confirmation

SDKv2 `Default: false` on a `TypeBool` field maps to:

```go
"ignore_volume_confirmation": schema.BoolAttribute{
    Optional: true,
    Computed: true,               // required when Default is set
    Default:  booldefault.StaticBool(false),
},
```

The framework requires `Computed: true` on any attribute with a `Default` value — this allows the framework to insert the default into the plan when the practitioner omits the field. Without `Computed: true`, the framework rejects the schema at provider boot via `ValidateImplementation`.

## HCL backward-compatibility justification

Choosing `SetNestedBlock` over `SetNestedAttribute` means the practitioner's Terraform configuration files are unchanged:

```hcl
# Before migration (SDKv2 provider)
resource "openstack_compute_volume_attach_v2" "example" {
  instance_id = "..."
  volume_id   = "..."
  vendor_options {
    ignore_volume_confirmation = true
  }
}

# After migration (framework provider) — identical HCL, no changes needed
resource "openstack_compute_volume_attach_v2" "example" {
  instance_id = "..."
  volume_id   = "..."
  vendor_options {
    ignore_volume_confirmation = true
  }
}
```

This is the correct choice for a non-major-version migration of a mature provider. The framework-preferred `SetNestedAttribute` would be appropriate only on a major version bump where breaking HCL changes are documented.

## Timeouts

SDKv2 `Timeouts: &schema.ResourceTimeout{Create: ..., Delete: ...}` would typically migrate to the `terraform-plugin-framework-timeouts` helper package (schema-only migration). This is omitted from the migrated schema output here because the task scopes to the `Schema` field of the SDKv2 resource — the timeouts helper adds its own schema attribute automatically and requires coordination with CRUD methods. It should be added when CRUD methods are migrated.
