# Migration Reasoning: `persistence` block — ListNestedBlock vs SingleNestedAttribute

## The SDK v2 definition

```go
"persistence": {
    Type:     schema.TypeList,
    Optional: true,
    MaxItems: 1,
    Elem: &schema.Resource{
        Schema: map[string]*schema.Schema{ ... },
    },
},
```

In HCL this is written as a **block**, not an attribute assignment:

```hcl
persistence {
  type        = "HTTP_COOKIE"
  cookie_name = "my-cookie"
}
```

## The two framework candidates

| Option | HCL syntax produced | Notes |
|---|---|---|
| `schema.SingleNestedAttribute` | `persistence = { type = "..." }` | Written with `=`; treated as an object attribute |
| `schema.ListNestedBlock` | `persistence { type = "..." }` | Written without `=`; identical to current HCL |

## Decision: use `ListNestedBlock`

### 1. Configuration compatibility is the primary concern

The SDK v2 `TypeList/MaxItems:1` pattern generates block syntax (`persistence { … }`) in user configuration files. Changing to `SingleNestedAttribute` would require every existing configuration to be rewritten to attribute syntax (`persistence = { … }`), which is a **breaking change** for users.

`ListNestedBlock` preserves the block syntax exactly, so existing `.tf` files continue to work without modification.

### 2. State compatibility

Terraform stores `TypeList` blocks as a JSON array in state. `ListNestedBlock` also stores its value as a list in state, so state migration is not required. `SingleNestedAttribute` stores an object, which would break all existing state files.

### 3. Enforcing MaxItems: 1

The framework does not have a built-in `MaxItems` on `ListNestedBlock`. The constraint is re-expressed using the `listvalidator.SizeAtMost(1)` validator, which produces an equivalent validation error at plan time.

### 4. When would SingleNestedAttribute be correct?

`SingleNestedAttribute` is the right choice when the **original** SDK schema defined the field as a `TypeMap` or `TypeList` that was **always treated as a single structured object**, AND the provider author is willing to make a breaking change (e.g., as part of a major version bump with a migration guide). It can also be used for **new** attributes that have no existing users.

For this resource, which has public users and existing state files, `ListNestedBlock` is the correct, non-breaking choice.

### 5. Null vs empty-list semantics

With `ListNestedBlock`, omitting the block entirely leaves the list null/empty, matching the SDK v2 behaviour when `persistence` is not set. `SingleNestedAttribute` with `Optional: true` would also be null when omitted, but the HCL syntax difference remains the deciding factor.

## Summary

> Use **`ListNestedBlock` + `listvalidator.SizeAtMost(1)`** for any `TypeList/MaxItems:1` block that has existing users or state. Use `SingleNestedAttribute` only for genuinely new attributes or when a breaking change is intentional and documented.
