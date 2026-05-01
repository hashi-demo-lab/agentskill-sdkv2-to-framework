# MaxItems:1 Block Decision — openstack_lb_pool_v2 `persistence`

## The SDKv2 shape

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

`persistence` is a `TypeList` of `&schema.Resource` with `MaxItems: 1` — exactly the pattern that `references/blocks.md` flags for a conscious decision.

---

## Decision: `ListNestedBlock` + `listvalidator.SizeAtMost(1)`

**Output A** (keep as block) was chosen. The chosen code is:

```go
Blocks: map[string]schema.Block{
    "persistence": schema.ListNestedBlock{
        Validators: []validator.List{
            listvalidator.SizeAtMost(1),
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

---

## Reasoning, following the `references/blocks.md` decision tree

### Q1: Are practitioners using block syntax in production configs?

**YES.** `openstack_lb_pool_v2` is one of the core Terraform OpenStack provider resources, present since early provider versions and referenced in production infrastructure across many organisations. The `persistence` block is an established, user-visible part of its interface. HCL configs written against this resource today use:

```hcl
resource "openstack_lb_pool_v2" "example" {
  protocol    = "HTTP"
  lb_method   = "ROUND_ROBIN"
  loadbalancer_id = var.lb_id

  persistence {
    type        = "HTTP_COOKIE"
    cookie_name = "session"
  }
}
```

Converting `persistence` to a `SingleNestedAttribute` would require practitioners to rewrite this as:

```hcl
  persistence = {
    type        = "HTTP_COOKIE"
    cookie_name = "session"
  }
```

This is a **breaking change** for practitioner HCL. The `=` assignment syntax is not syntactically equivalent to the block invocation syntax — existing configurations would fail to parse after the provider upgrade. `references/blocks.md` is explicit: *"Switching is a breaking HCL change."*

### Q2: Major-version bump or greenfield resource?

**NO.** This migration is a single-release-cycle SDK-to-framework port of an existing resource. There is no indication of a concurrent major-version bump of the provider. `references/blocks.md` states: *"Major-version bump or greenfield resource? → Convert to SingleNestedAttribute."* Neither condition applies here.

### Q3 (tie-breaker): Can't confirm either?

Not reached — Q1 fires clearly.

**Conclusion from the decision tree: keep as block (Output A).**

---

## Why `ListNestedBlock` over `SingleNestedBlock`

`references/blocks.md` notes that `SingleNestedBlock` is another valid "keep-as-block" option. Both produce the same practitioner HCL syntax (`persistence { ... }`). The choice between them is:

> *Pick `ListNestedBlock + SizeAtMost(1)` when you genuinely need the list-shaped state path (e.g., for backward state compatibility where existing state was written under `block.0.field`).*

SDKv2 stores block state under list-indexed paths: `persistence.0.type`, `persistence.0.cookie_name`. Practitioners with existing state files — or module authors who reference `resource.openstack_lb_pool_v2.pool.persistence[0].type` in their outputs — depend on the list-indexed path. Using `ListNestedBlock` preserves this path shape in the framework state, avoiding backward state compatibility issues without requiring a state upgrader.

`SingleNestedBlock` would instead store state under `persistence.type`, `persistence.cookie_name` — a different path that would break any expression referencing the old list-indexed form.

---

## Summary of key terms applied

| Term | How it applies here |
|---|---|
| **backward-compat** | Existing practitioner HCL uses block syntax; preserving it is required. |
| **practitioner HCL** | `persistence { type = "..." }` is the user-visible syntax that must not change. |
| **breaking change** | Block → SingleNestedAttribute changes `foo { }` to `foo = { }` — a parse-level break. |
| **major version** | No major-version bump is in scope; conversion cannot be justified on that ground. |
| **greenfield** | `openstack_lb_pool_v2` is not greenfield; it has production users. |
