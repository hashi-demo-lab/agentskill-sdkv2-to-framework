# `persistence`: `ListNestedBlock` (kept as block), not `SingleNestedAttribute`

## TL;DR

I kept `persistence` as a `ListNestedBlock` with
`listvalidator.SizeAtMost(1)` instead of converting it to a
`SingleNestedAttribute`. This is the **backward-compatible** choice and is
what the skill's `MaxItems: 1` decision rule (in `SKILL.md` and
`references/blocks.md`) prescribes for a mature, in-production resource where
practitioners are already using block syntax in their HCL.

## How the skill's decision tree applies

`SKILL.md`'s `MaxItems_1_decision` example asks three questions in order:

1. **Are there practitioner configs in the wild using block syntax for this
   attribute?** — Yes. Evidence:
   - `docs/resources/lb_pool_v2.md` (the registry docs) shows the example
     usage with `persistence { ... }` block syntax (line 25).
   - The acceptance test fixture in
     `openstack/resource_openstack_lb_pool_v2_test.go` (line 256) writes
     `persistence { ... }`.
   - Test assertions reference the list-flavoured state path
     `persistence.#` / `persistence.0.type` / `persistence.0.cookie_name`,
     which is the shape practitioners' own state files will have.
   - The provider has multiple changelog entries about
     `openstack_lb_pool_v2.persistence` going back to PR #57
     (`cookie_name` made optional) — i.e., this attribute has been in the
     public schema for years and there will be many production configs.
2. **Is this a major-version bump documenting breaking HCL changes, or a
   greenfield resource?** — No. The eval task is a single-resource
   schema-only port, not a major-version bump, and the resource is
   long-established.
3. **Default action when (1) is true:** keep as block.

So the rule lands on the first question: `Output A — backward-compat`.

## What "switching to `SingleNestedAttribute`" would actually break

```hcl
# What practitioners write today (block syntax)
resource "openstack_lb_pool_v2" "pool_1" {
  # ...
  persistence {
    type        = "APP_COOKIE"
    cookie_name = "testCookie"
  }
}

# What they would have to rewrite to if we switched to SingleNestedAttribute
resource "openstack_lb_pool_v2" "pool_1" {
  # ...
  persistence = {
    type        = "APP_COOKIE"
    cookie_name = "testCookie"
  }
}
```

Every existing config in the wild would fail to parse on first apply after
upgrade. That is a practitioner-visible HCL breaking change, ruled out by the
"Don't change user-facing schema names or attribute IDs" pitfall in
`SKILL.md` — switching the *syntax* a user must write is morally the same
class of change.

## State-path stability

State paths also stay the same: practitioners' state files (and any external
tooling reading `persistence.#` / `persistence.0.*`) keep working unchanged
when we use `ListNestedBlock`. A `SingleNestedAttribute` would change the
state path to `persistence.type` / `persistence.cookie_name`, which is a
silent breaking change for any tooling that grew up around the SDKv2 shape.

## How the chosen shape encodes `MaxItems: 1`

```go
Blocks: map[string]schema.Block{
    "persistence": schema.ListNestedBlock{
        Validators: []validator.List{
            listvalidator.SizeAtMost(1),
        },
        NestedObject: schema.NestedBlockObject{
            Attributes: map[string]schema.Attribute{
                "type":        schema.StringAttribute{Required: true, /* OneOf */ },
                "cookie_name": schema.StringAttribute{Optional: true},
            },
        },
    },
},
```

Three things to note:

- It lives under `Blocks`, not `Attributes`. Mixing them is a compile error.
- `MaxItems: 1` does not exist on framework blocks; the equivalent is
  `Validators: []validator.List{listvalidator.SizeAtMost(1)}` from
  `terraform-plugin-framework-validators/listvalidator`.
- Inside the block, primitive fields go under `NestedObject.Attributes` of
  type `schema.NestedBlockObject` (note the *Block*Object, not the
  `NestedAttributeObject` you'd use inside a `ListNestedAttribute`).

## When this decision should be revisited

Per the skill, on the next provider major version bump that already
documents breaking changes, this is a good candidate to convert to
`SingleNestedAttribute`:
- per-field plan modifiers and validators become possible (currently
  blocks only have whole-block validators / plan modifiers);
- the configuration syntax aligns with the modern framework idiom;
- a single place in the changelog can announce the syntactic move.

That's a future-version decision, not this migration's.
