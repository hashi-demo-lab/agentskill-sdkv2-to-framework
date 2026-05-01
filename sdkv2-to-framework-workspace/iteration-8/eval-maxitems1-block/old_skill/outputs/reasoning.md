# MaxItems:1 Decision Reasoning — openstack_lb_pool_v2 `persistence` block

## Source attribute (SDKv2)

```go
"persistence": {
    Type:     schema.TypeList,
    Optional: true,
    MaxItems: 1,
    Elem: &schema.Resource{
        Schema: map[string]*schema.Schema{
            "type":        {Type: schema.TypeString, Required: true, ValidateFunc: ...},
            "cookie_name": {Type: schema.TypeString, Optional: true},
        },
    },
},
```

## Decision: ListNestedBlock + SizeAtMost(1) (backward-compat / Output A)

### Applying the decision rule from SKILL.md / references/blocks.md

The skill documents three ordered questions:

1. **Are there practitioner configs in the wild using block syntax?**
   Yes. `openstack_lb_pool_v2` is a mature, widely-used resource in the
   `terraform-provider-openstack` project (tracked by many production OpenStack
   deployments). The OpenStack Terraform provider registry page, its official
   documentation examples, and the resource's own test fixtures all use the
   block syntax:
   ```hcl
   persistence {
     type        = "HTTP_COOKIE"
     cookie_name = "srv_id"
   }
   ```
   This is the answer to question 1: **yes, block syntax is in production use**.

2. Consequently, question 2 (major-version bump / greenfield) is not reached,
   and question 3 (can't confirm) is moot.

### Why converting to SingleNestedAttribute would be a breaking change

As `blocks.md` explains, block syntax and nested-attribute syntax are
syntactically different in HCL:

```hcl
# block syntax — what practitioners currently write
persistence {
  type = "HTTP_COOKIE"
}

# nested-attribute syntax — what SingleNestedAttribute would require
persistence = {
  type = "HTTP_COOKIE"
}
```

The `=` sign on the outer key is not optional in HCL; its presence or absence
is determined by the schema type (block vs attribute), not by Terraform version.
Any existing config using the block form would fail to parse after the switch —
that is a **breaking change for practitioners** even though the provider's
semantic behaviour is identical.

### Why not SingleNestedBlock instead of ListNestedBlock?

`blocks.md` (section "SingleNestedBlock — the third option for MaxItems:1")
notes that `SingleNestedBlock` is ergonomic when the block is entirely optional
and appears at most once, AND when you don't need the `block.0.field` state
path for backward state compatibility.

`persistence` in this resource is written to state as a list (`flattenLBPoolPersistenceV2` returns `[]map[string]any`), meaning existing state on the wire already encodes the path as `persistence.0.type`. Switching to `SingleNestedBlock` changes the state address to `persistence.type`, which would require a state migration step. Keeping `ListNestedBlock` preserves the `persistence.0.*` state path without a schema version bump.

Therefore `ListNestedBlock + listvalidator.SizeAtMost(1)` is the safest
backward-compatible choice.

### Guidance citations

| Guidance | Source |
|---|---|
| "Practitioners wrote: `foo { name = "x" }` / Convert to `SingleNestedAttribute` → practitioners now write: `foo = { name = "x" }` / This IS a syntactic change; document in CHANGELOG." | `references/blocks.md` — decision tree, MaxItems:1 branch |
| "Use `ListNestedBlock` with `Validators: []validator.List{listvalidator.SizeAtMost(1)}` when: Practitioners depend on the block syntax in production configs. You can't bump the major version yet." | `references/blocks.md` — "When MaxItems:1 should stay a block" |
| "If backward compat is sacred, keep as `ListNestedBlock` with `MaxItems: 1`." | `references/blocks.md` — decision tree annotation |
| "Pick `ListNestedBlock + SizeAtMost(1)` when you genuinely need the list-shaped state path (e.g., for backward state compatibility where existing state was written under `block.0.field`)." | `references/blocks.md` — SingleNestedBlock section |
| Decision rule step 1: "Are there practitioner configs in the wild using block syntax? If you can confirm yes … keep as block — Output A. Switching is a breaking HCL change." | `SKILL.md` — MaxItems_1_decision example |

## Other schema decisions

| SDKv2 pattern | Framework translation | Rationale |
|---|---|---|
| `ForceNew: true` | `stringplanmodifier.RequiresReplace()` in `PlanModifiers` | Per `references/plan-modifiers.md`; `RequiresReplace` is NOT a top-level field. |
| `Default: true` on `admin_state_up` | `booldefault.StaticBool(true)` + `Computed: true` | `Default` is a separate package in the framework, not a plan modifier. Attribute must be `Computed` for the framework to insert the default into the plan. |
| `Set: schema.HashString` on `tags` | Dropped entirely | Framework `SetAttribute` handles uniqueness internally; no `Set:` field exists. |
| `TypeSet, Elem: &schema.Schema{Type: TypeString}` (alpn_protocols, tls_versions, tags) | `SetAttribute{ElementType: types.StringType}` | Primitive set with homogeneous element type. `ValidateFunc` on the element maps to `setvalidator.ValueStringsAre(stringvalidator.OneOf(...))`. |
| `ExactlyOneOf: []string{"loadbalancer_id", "listener_id"}` | `stringvalidator.ExactlyOneOf(path.MatchRoot("loadbalancer_id"), path.MatchRoot("listener_id"))` on both attributes | Cross-attribute constraint moved from schema field to per-attribute validator slice. |
| `Computed: true` on `tls_ciphers` (API sets default) | `Computed: true` + `UseStateForUnknown()` | Keeps the prior state value stable across plans when the practitioner hasn't changed the field, avoiding spurious `(known after apply)` churn. |
