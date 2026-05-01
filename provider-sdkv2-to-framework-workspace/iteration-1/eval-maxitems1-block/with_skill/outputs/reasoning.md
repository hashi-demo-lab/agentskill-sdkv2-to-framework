# MaxItems:1 Block Decision Reasoning

## Resource: openstack_lb_pool_v2

### Block inventory

Only one `MaxItems:1` block exists in this resource:

| SDKv2 field | Type | MaxItems | Decision |
|---|---|---|---|
| `persistence` | `TypeList, Elem: &schema.Resource{}` | `MaxItems: 1` | `ListNestedBlock` with `SizeAtMost(1)` |

---

## `persistence` — Decision: `ListNestedBlock` with `listvalidator.SizeAtMost(1)`

### What the SDKv2 schema says

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

### The two options

**Option A — `SingleNestedAttribute`**

Would produce the nested-attribute HCL syntax:

```hcl
persistence = {
  type        = "HTTP_COOKIE"
  cookie_name = "my-cookie"
}
```

**Option B — `ListNestedBlock` with `listvalidator.SizeAtMost(1)`**

Preserves the block syntax practitioners already write:

```hcl
persistence {
  type        = "HTTP_COOKIE"
  cookie_name = "my-cookie"
}
```

### Why Option B was chosen

1. **Backward compatibility is the overriding concern.** `openstack_lb_pool_v2` is a mature, widely-used resource in a community provider. Practitioners across many real deployments use the block syntax. Converting to `SingleNestedAttribute` would change the HCL syntax from `persistence { ... }` to `persistence = { ... }`, breaking every existing configuration without any functional benefit — a purely cosmetic breakage that forces every user to update their Terraform code.

2. **No major-version bump is indicated.** The skill guidance (blocks.md) states: convert `MaxItems:1` to `SingleNestedAttribute` when the provider is on a major-version bump anyway, or the block is newly added and not yet widely adopted. Neither condition applies here.

3. **`persistence` is a well-established attribute.** It maps directly to the OpenStack Load Balancer API's `session_persistence` object, which has existed since Octavia v1. This is not a recently-added or lightly-used field.

4. **The block has no `Computed` semantics.** Blocks cannot be `Required`/`Optional`/`Computed`. The `persistence` block is entirely optional and user-driven, so there's no need for `Computed`-style behaviour that would otherwise force us to use `SingleNestedAttribute`.

5. **`SizeAtMost(1)` reproduces `MaxItems: 1`.** The list validator `listvalidator.SizeAtMost(1)` enforces the same constraint at plan validation time, so the user-visible cardinality contract is preserved.

### `SingleNestedAttribute` would be correct if…

- The provider were cutting a new major version (e.g. `v2.0.0`) and accepting a breaking change window.
- The `persistence` block had been added in the same release as the framework migration (so no existing configs use block syntax).
- The team explicitly decided to modernise the HCL shape and documented it in the changelog.

None of those apply here, so `ListNestedBlock` with `SizeAtMost(1)` is the right call.

---

## Other notable migrations

These are not MaxItems:1 decisions but are worth documenting:

| SDKv2 pattern | Framework equivalent | Notes |
|---|---|---|
| `ForceNew: true` on `region`, `tenant_id`, `protocol`, `loadbalancer_id`, `listener_id` | `stringplanmodifier.RequiresReplace()` | Each ForceNew field gets a RequiresReplace plan modifier |
| `Default: true` on `admin_state_up` | `booldefault.StaticBool(true)` + `Computed: true` | Default is its own field (not a plan modifier); attribute must also be Computed |
| `ExactlyOneOf: []string{"loadbalancer_id", "listener_id"}` | `stringvalidator.ExactlyOneOf(path.MatchRoot(...))` on both attributes | Cross-attribute validation moves to validators |
| `validation.StringInSlice(...)` | `stringvalidator.OneOf(...)` | Direct port |
| `TypeSet, Elem: &schema.Schema{Type: TypeString}` (alpn_protocols, tls_versions, tags) | `schema.SetAttribute{ElementType: types.StringType}` | Primitive set — not a block decision |
| `Set: schema.HashString` on `tags` | Deleted | Framework handles set uniqueness internally |
| `Computed: true` on `region`, `tenant_id`, `alpn_protocols`, `tls_ciphers`, `tls_versions` | `Computed: true` + `stringplanmodifier.UseStateForUnknown()` on stable-after-create fields | UseStateForUnknown reduces noisy "(known after apply)" on subsequent plans |
