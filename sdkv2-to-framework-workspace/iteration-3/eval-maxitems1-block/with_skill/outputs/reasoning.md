# Schema migration reasoning — openstack_lb_pool_v2

## MaxItems: 1 block decision: `persistence`

### SDKv2 shape

```go
"persistence": {
    Type:     schema.TypeList,
    Optional: true,
    MaxItems: 1,
    Elem: &schema.Resource{
        Schema: map[string]*schema.Schema{
            "type":        {Type: schema.TypeString, Required: true, ...},
            "cookie_name": {Type: schema.TypeString, Optional: true},
        },
    },
},
```

### Decision: keep as `ListNestedBlock` with `SizeAtMost(1)` — do NOT convert to `SingleNestedAttribute`

**Why not `SingleNestedAttribute`?**

`SingleNestedAttribute` would be the cleanest framework-native representation for a "zero or one" relationship, and the framework documentation recommends it for greenfield resources. However, converting a `MaxItems: 1` block to a nested attribute is a **syntactic breaking change for practitioners**:

| Shape | Practitioner HCL |
|---|---|
| Block (current) | `persistence { type = "HTTP_COOKIE" cookie_name = "x" }` |
| Nested attribute | `persistence = { type = "HTTP_COOKIE" cookie_name = "x" }` |

The `openstack_lb_pool_v2` resource is not new. It has been in the OpenStack provider for years and practitioners have production configs using the block syntax. A silent syntactic change would:

1. Break `terraform validate` / `terraform plan` for all existing configurations.
2. Require every user to update their HCL, even when the underlying resource state has not changed.
3. Constitute a breaking change that should only accompany a provider major-version bump.

This migration is a within-major-version framework port — there is no major-version bump that would licence breaking HCL syntax. The skill's decision rule is explicit: when backward compatibility with existing user configs is required and there is no major-version bump, keep `MaxItems: 1` blocks as `ListNestedBlock`.

**Framework mapping:**

```go
Blocks: map[string]schema.Block{
    "persistence": schema.ListNestedBlock{
        Validators: []validator.List{
            listvalidator.SizeAtMost(1), // replaces MaxItems: 1
        },
        NestedObject: schema.NestedBlockObject{
            Attributes: map[string]schema.Attribute{
                "type":        schema.StringAttribute{Required: true, Validators: [...]},
                "cookie_name": schema.StringAttribute{Optional: true},
            },
        },
    },
},
```

`SizeAtMost(1)` is the exact semantic equivalent of `MaxItems: 1` — it is enforced at plan time by the framework validator, and the error message mirrors the SDKv2 error.

The Go model struct field type for this block is `[]lbPoolV2PersistenceModel` (a slice), matching the list-of-zero-or-one representation. CRUD code reads `model.Persistence[0]` after checking `len(model.Persistence) > 0`, which is identical in intent to the SDKv2 `d.Get("persistence").([]any)` pattern.

---

## Other schema decisions

### Primitive attributes

All SDKv2 `TypeString` / `TypeBool` / `TypeInt` scalars map directly to `schema.StringAttribute` / `schema.BoolAttribute` / `schema.Int64Attribute`. No judgment calls required.

### `ForceNew: true` → `RequiresReplace` plan modifier

`region`, `tenant_id`, `protocol`, `loadbalancer_id`, `listener_id` all had `ForceNew: true`. In the framework this becomes `stringplanmodifier.RequiresReplace()` in the attribute's `PlanModifiers` slice. Critically, `ForceNew` is **not** translated by setting a boolean on the attribute — the framework has no such boolean; it is exclusively a plan modifier.

### `Computed: true` without a `Default` → `UseStateForUnknown`

Attributes that are `Computed` and can drift (`region`, `tenant_id`, `tls_ciphers`, `tls_enabled`) get `UseStateForUnknown()` to suppress unnecessary plan noise. Attributes that are `Computed` because the API can return defaults (`alpn_protocols`, `tls_versions`) keep `Computed: true` only.

### `Default: true` on `admin_state_up`

The SDKv2 `Default: true` translates to `Default: booldefault.StaticBool(true)` — this is the `defaults` package, not a plan modifier. A common migration mistake is putting defaults into `PlanModifiers`; this schema avoids that.

### `Set` + `Elem: &schema.Schema{Type: schema.TypeString}` → `schema.SetAttribute`

`alpn_protocols`, `tls_versions`, `tags` were all `TypeSet` of `TypeString`. The framework equivalent is `schema.SetAttribute{ElementType: types.StringType}`. The `Set: schema.HashString` function on `tags` is dropped — framework set attributes handle uniqueness internally without a hash function.

### `ExactlyOneOf` on `loadbalancer_id` / `listener_id`

SDKv2's `ExactlyOneOf` is replicated with `stringvalidator.ExactlyOneOf("loadbalancer_id", "listener_id")` placed on `loadbalancer_id`. Only one side needs the validator; the constraint is symmetric.

### Timeouts

`schema.ResourceTimeout` with `Create`, `Update`, `Delete` (all 10 minutes) maps to the `terraform-plugin-framework-timeouts` package: `timeouts.Block(ctx, timeouts.Opts{Create: true, Update: true, Delete: true})` placed in the `Blocks` map. The `Timeouts` field on the model struct is `timeouts.Value`.

### Tags — dropped `Set: schema.HashString`

Framework `SetAttribute` does not accept a hash function. The `Set: schema.HashString` field on the SDKv2 `tags` attribute is simply omitted; the framework's set implementation handles element uniqueness natively.

### No state upgrader, no composite import ID

The SDKv2 resource has no `SchemaVersion` > 0, so no state upgrader is needed. The importer (`resourcePoolV2Import`) uses a custom function but resolves to a simple passthrough once `listener_id` or `loadbalancer_id` is set — this belongs in the CRUD layer (out of scope for this schema-only migration).
