# Schema migration reasoning: openstack_lb_pool_v2

## The MaxItems:1 block decision

### The attribute in question

```go
// SDKv2 original
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

### The two framework options

| Option | Framework type | User HCL syntax |
|---|---|---|
| A — backward-compat | `ListNestedBlock` + `listvalidator.SizeAtMost(1)` | `persistence { type = "HTTP_COOKIE" }` |
| B — greenfield | `SingleNestedAttribute` | `persistence = { type = "HTTP_COOKIE" }` |

The syntax difference matters: blocks use `name { }` (no `=`); nested attributes use
`name = { }`. Changing from block to nested attribute is a **breaking change** in
practitioner HCL — existing configs using the old block syntax will produce a parse
error against the new schema.

### Decision: Option A — ListNestedBlock

`openstack_lb_pool_v2` is a mature, widely-deployed resource in the
terraform-provider-openstack. Practitioners already have Terraform configurations
that write:

```hcl
resource "openstack_lb_pool_v2" "pool" {
  # ...
  persistence {
    type        = "HTTP_COOKIE"
    cookie_name = "testCookie"
  }
}
```

This migration is happening within a single release cycle, with no major-version bump
(the resource name and all attribute IDs are unchanged). Converting `persistence` to
`SingleNestedAttribute` would silently break those existing configs the next time users
run `terraform plan`.

The backward-compatible path is `ListNestedBlock` with `listvalidator.SizeAtMost(1)`,
which:
- Preserves the `persistence { }` block syntax.
- Re-encodes the `MaxItems: 1` constraint as a framework validator (the framework has
  no `MaxItems` field on blocks; validators are the canonical replacement).
- Produces a hard validation error — not a silent truncation — if more than one block
  is supplied.

**Rule from the skill**: *"Mature resource with practitioner configs already using block
syntax, and no major-version bump → keep as block."*

If this resource were ever to get a major-version bump (e.g., a `v3` variant), that
would be the right time to convert `persistence` to `SingleNestedAttribute` and
document the breaking change in the changelog.

---

## Other notable conversion decisions

### ForceNew → RequiresReplace plan modifier

`region`, `tenant_id`, `protocol`, `loadbalancer_id`, and `listener_id` all had
`ForceNew: true`. In the framework, `ForceNew` does not exist as a schema field; it
becomes `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`.

### Computed + Optional → UseStateForUnknown

`region` and `tenant_id` are `Optional: true, Computed: true`. The `UseStateForUnknown`
plan modifier is added so that Terraform does not show a spurious diff for these fields
when they haven't changed. Without it, every plan would show them as `(known after
apply)`.

### Default: true → booldefault.StaticBool(true)

`admin_state_up` had `Default: true`. In the framework, `Default` is not a schema
field or a plan modifier — it lives in the `defaults` package:
`Default: booldefault.StaticBool(true)`. Because a default is set, `Computed: true` is
also required (the framework enforces that defaulted optional attributes are also
computed).

### ExactlyOneOf → stringvalidator.ExactlyOneOf

SDKv2's `ExactlyOneOf: []string{"loadbalancer_id", "listener_id"}` is replaced by
`stringvalidator.ExactlyOneOf(path.MatchRoot("loadbalancer_id"), path.MatchRoot("listener_id"))`
from `terraform-plugin-framework-validators`. The validator is placed on both
attributes (as it was in SDKv2).

### Set: schema.HashString dropped

SDKv2 required an explicit hash function for `TypeSet` attributes. The framework
handles set uniqueness internally — `schema.HashString` is dropped with no replacement
needed.

### alpn_protocols, tls_versions: TypeSet → SetAttribute

Both were `TypeSet` with a `ValidateFunc` on the element type. The framework
equivalent is `schema.SetAttribute{ElementType: types.StringType}` with a
`setvalidator.ValueStringsAre(stringvalidator.OneOf(...))` validator. `Computed: true`
is preserved on both because unsetting them causes the API to apply a default value.

### Timeouts

The SDKv2 `Timeouts` block (10 min create/update/delete) is not part of the schema
output — it is handled by the `terraform-plugin-framework-timeouts` package in the
CRUD methods, which is out of scope for this schema-only migration.
