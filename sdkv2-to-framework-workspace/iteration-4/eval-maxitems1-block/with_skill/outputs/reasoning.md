# Migration reasoning: resource_openstack_lb_pool_v2

## The MaxItems:1 block decision — `persistence`

### SDKv2 shape

```go
"persistence": {
    Type:     schema.TypeList,
    Optional: true,
    MaxItems: 1,
    Elem: &schema.Resource{
        Schema: map[string]*schema.Schema{
            "type":        { Type: schema.TypeString, Required: true, ... },
            "cookie_name": { Type: schema.TypeString, Optional: true },
        },
    },
},
```

Practitioners using this resource write HCL **block syntax**:

```hcl
resource "openstack_lb_pool_v2" "example" {
  ...
  persistence {
    type        = "HTTP_COOKIE"
    cookie_name = "SRV_ID"
  }
}
```

### The two framework options

| Option | Framework type | HCL that practitioners write |
|---|---|---|
| A — keep as block | `ListNestedBlock` + `listvalidator.SizeAtMost(1)` | `persistence { type = "..." }` (unchanged) |
| B — convert to single nested attribute | `SingleNestedAttribute` | `persistence = { type = "..." }` (breaking change) |

### Decision: Option A — `ListNestedBlock`

**Rationale:**

1. **This is an existing, mature resource.** `openstack_lb_pool_v2` has been in the OpenStack Terraform provider for several major releases. Practitioners have production Terraform configurations that use `persistence { ... }` block syntax.

2. **No major-version bump is accompanying this migration.** The task is a within-resource schema migration only, not a v4→v5 provider bump. Without a major version bump there is no acceptable mechanism to document and communicate a breaking HCL change to end-users.

3. **The HCL syntactic change is breaking.** Converting `persistence` from a block to a `SingleNestedAttribute` changes how practitioners write it: `persistence { ... }` becomes `persistence = { ... }`. The `=` is not cosmetic — Terraform's HCL parser treats them differently and the old form stops parsing. Any practitioner configuration not updated will fail with a parse error on the next `terraform init`/`plan`.

4. **Skill guidance is unambiguous.** The SKILL.md and `references/blocks.md` both state: "Mature resource with practitioner configs already using block syntax, and no major-version bump → keep as block." The default when in doubt is also Option A with a changelog note.

5. **`listvalidator.SizeAtMost(1)` preserves the `MaxItems:1` constraint.** The functional guarantee (at most one persistence block) is maintained without the syntactic break.

### What changes for each field

| SDKv2 field | SDKv2 value | Framework equivalent | Notes |
|---|---|---|---|
| `persistence.type` | `Required: true`, `ValidateFunc: StringInSlice(...)` | `Required: true`, `Validators: []validator.String{stringvalidator.OneOf(...)}` | Direct port |
| `persistence.cookie_name` | `Optional: true` | `Optional: true` | Direct port |

---

## Other schema decisions

### Primitive attribute conversions

| Attribute | SDKv2 type | Framework type | Notes |
|---|---|---|---|
| `region` | `TypeString, Optional, Computed, ForceNew` | `StringAttribute, Optional, Computed` + `RequiresReplace()` + `UseStateForUnknown()` | `UseStateForUnknown` avoids noisy unknown in plan |
| `tenant_id` | `TypeString, Optional, Computed, ForceNew` | Same pattern as `region` | |
| `name` | `TypeString, Optional` | `StringAttribute, Optional` | |
| `description` | `TypeString, Optional` | `StringAttribute, Optional` | |
| `protocol` | `TypeString, Required, ForceNew, ValidateFunc` | `StringAttribute, Required` + `RequiresReplace()` + `stringvalidator.OneOf(...)` | |
| `loadbalancer_id` | `TypeString, Optional, ForceNew, ExactlyOneOf` | `StringAttribute, Optional` + `RequiresReplace()` + `stringvalidator.ExactlyOneOf(...)` | Cross-attribute check via validator |
| `listener_id` | same as `loadbalancer_id` | same pattern | |
| `lb_method` | `TypeString, Required, ValidateFunc` | `StringAttribute, Required` + `stringvalidator.OneOf(...)` | No ForceNew in original |
| `alpn_protocols` | `TypeSet, Optional, Computed, Elem: TypeString, ValidateFunc` | `SetAttribute{ElementType: types.StringType}, Optional, Computed` + `setvalidator.ValueStringsAre(stringvalidator.OneOf(...))` | |
| `ca_tls_container_ref` | `TypeString, Optional` | `StringAttribute, Optional` | |
| `crl_container_ref` | `TypeString, Optional` | `StringAttribute, Optional` | |
| `tls_enabled` | `TypeBool, Optional` | `BoolAttribute, Optional` | |
| `tls_ciphers` | `TypeString, Optional, Computed` | `StringAttribute, Optional, Computed` + `UseStateForUnknown()` | Computed because API supplies default when unset |
| `tls_container_ref` | `TypeString, Optional` | `StringAttribute, Optional` | |
| `tls_versions` | `TypeSet, Optional, Computed, Elem: TypeString, ValidateFunc` | `SetAttribute{ElementType: types.StringType}, Optional, Computed` + `setvalidator.ValueStringsAre(...)` | |
| `admin_state_up` | `TypeBool, Default: true, Optional` | `BoolAttribute, Optional, Computed` + `Default: booldefault.StaticBool(true)` | Default moves to `defaults` package; must be `Computed` for framework to inject it into the plan |
| `tags` | `TypeSet, Optional, Elem: TypeString, Set: schema.HashString` | `SetAttribute{ElementType: types.StringType}, Optional` | `schema.HashString` dropped — framework handles set uniqueness natively |

### Key migration rules applied

- **`ForceNew: true` → `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`** — not a field on the attribute.
- **`Default: true` → `Default: booldefault.StaticBool(true)`** — lives in the `defaults` package, not `PlanModifiers`. Attribute must also be `Computed: true`.
- **`ValidateFunc: validation.StringInSlice(opts, false)` → `Validators: []validator.String{stringvalidator.OneOf(opts...)}`** — from `terraform-plugin-framework-validators/stringvalidator`.
- **`ExactlyOneOf` → `stringvalidator.ExactlyOneOf(path.Expressions{...})`** — becomes a per-attribute validator on both attributes.
- **`Set: schema.HashString` → deleted** — framework handles set uniqueness internally; emitting this would be a compile error.
- **`TypeSet` of primitive → `SetAttribute{ElementType: types.StringType}`** — not a block, a primitive collection attribute.
- **Blocks vs Attributes live in separate map fields** on `schema.Schema` — mixing them is a compile error.
