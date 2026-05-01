# Validator migration

## Quick summary
- SDKv2 `ValidateFunc` / `ValidateDiagFunc` → framework `Validators` field, a typed slice per attribute kind.
- The companion module `github.com/hashicorp/terraform-plugin-framework-validators` provides ports of every common SDKv2 validator (length, regex, one-of, between, etc.).
- Cross-attribute checks (`ConflictsWith`, `RequiredWith`, `ExactlyOneOf`, `AtLeastOneOf`) now live in the validators package too — no special schema fields.
- For schema-wide cross-attribute logic, implement `ResourceWithConfigValidators` (returns config validators) or `ResourceWithValidateConfig` (one-off function).
- Keep validators pure: validate config syntactically. For "is this value reachable in the API?" checks, do those in `Read`/`Create` and return diagnostics there.

## SDKv2 shape

```go
"name": {
    Type:         schema.TypeString,
    Required:     true,
    ValidateFunc: validation.StringLenBetween(1, 64),
}
```

Or the diagnostic-aware form:
```go
"name": {
    Type:             schema.TypeString,
    Required:         true,
    ValidateDiagFunc: validation.ToDiagFunc(validation.StringLenBetween(1, 64)),
}
```

## Framework shape

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/schema/validator"
    "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

"name": schema.StringAttribute{
    Required: true,
    Validators: []validator.String{
        stringvalidator.LengthBetween(1, 64),
    },
}
```

The slice is typed per attribute kind: `[]validator.String`, `[]validator.Int64`, `[]validator.Bool`, `[]validator.List`, `[]validator.Set`, `[]validator.Map`, `[]validator.Object`, etc. Importing the wrong package gives a clear compile error.

## Common SDKv2 → framework validator mappings

From `terraform-plugin-framework-validators`. Module path: `github.com/hashicorp/terraform-plugin-framework-validators`.

| SDKv2 | Framework |
|---|---|
| `validation.StringLenBetween(min, max)` | `stringvalidator.LengthBetween(min, max)` |
| `validation.StringIsNotEmpty` | `stringvalidator.LengthAtLeast(1)` |
| `validation.NoZeroValues` (string) | `stringvalidator.LengthAtLeast(1)`. **Don't** also flip `Optional: true` → `Required: true`; that's a breaking schema change. SDKv2's `NoZeroValues` only fires on configured zero values, which `LengthAtLeast(1)` already does — `Required` is a separate concern (presence) handled by the existing schema flag. |
| `validation.NoZeroValues` (int / float) | `int64validator.NoneOf(0)` / `float64validator.NoneOf(0)`. |
| `validation.NoZeroValues` (bool) | `boolvalidator.OneOf(true)` is the literal port (since the bool zero value is `false`), but it's almost always a bug in the original SDKv2 schema — the validator forces the field to always be `true`, at which point why is it user-configurable? Double-check the original intent before mechanically porting. |
| `validation.StringMatch(re, msg)` | `stringvalidator.RegexMatches(re, msg)` |
| `validation.StringInSlice(opts, false)` | `stringvalidator.OneOf(opts...)` |
| `validation.IntBetween(min, max)` | `int64validator.Between(int64(min), int64(max))` |
| `validation.IntAtLeast(min)` | `int64validator.AtLeast(int64(min))` |
| `validation.IntInSlice(opts)` | `int64validator.OneOf(opts...)` |
| `validation.IsCIDR` | `cidrtypes.IPv4Prefix` / `cidrtypes.IPv6Prefix` (custom types from `terraform-plugin-framework-nettypes`). Do **not** use regex — CIDR can't be regex-validated correctly. |
| `validation.IsUUID` | `stringvalidator.RegexMatches(uuidRE, "must be a UUID")` (anchor the regex), or a custom validator that calls `uuid.Parse`. **Don't substitute a length check** — a 36-char string is not necessarily a valid UUID. |
| `validation.IsRFC3339Time` | custom validator (or use a `customtype` with parse semantics — see `state-and-types.md`) |

## Cross-attribute validators

In SDKv2 these were schema fields:
```go
"primary":   {ConflictsWith: []string{"secondary"}},
"secondary": {ConflictsWith: []string{"primary"}},
```

In the framework they're regular validators:
```go
import "github.com/hashicorp/terraform-plugin-framework/path"

"primary": schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("secondary")),
    },
},
"secondary": schema.StringAttribute{
    Optional: true,
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("primary")),
    },
},
```

| SDKv2 | Framework |
|---|---|
| `ConflictsWith` | `stringvalidator.ConflictsWith(path.Expressions...)` (or `int64validator`, etc.) |
| `RequiredWith` | `stringvalidator.AlsoRequires(path.Expressions...)` |
| `ExactlyOneOf` | `stringvalidator.ExactlyOneOf(path.Expressions...)` |
| `AtLeastOneOf` | `stringvalidator.AtLeastOneOf(path.Expressions...)` |

These run on every attribute they're attached to, so `ConflictsWith` only needs to be on *one* of the attributes, but it's idiomatic to put it on both for clarity.

## Schema-wide validators (replaces SDKv2 `CustomizeDiff` for validation)

For checks that span the whole config and don't fit on a single attribute:

```go
var (
    _ resource.Resource                = &thingResource{}
    _ resource.ResourceWithConfigValidators = &thingResource{}
)

func (r *thingResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
    return []resource.ConfigValidator{
        resourcevalidator.ExactlyOneOf(path.MatchRoot("a"), path.MatchRoot("b")),
    }
}
```

For one-off cross-attribute logic, implement `ResourceWithValidateConfig.ValidateConfig(ctx, req, resp)` and add diagnostics yourself.

### Schema-level validator packages — choose the right one

`terraform-plugin-framework-validators` ships kind-specific schema-level packages. Use the one that matches your use site:

| Package | Use site | Common builders |
|---|---|---|
| `resourcevalidator` | `ResourceWithConfigValidators.ConfigValidators` | `ExactlyOneOf`, `AtLeastOneOf`, `RequiredTogether`, `Conflicting` |
| `datasourcevalidator` | `DataSourceWithConfigValidators.ConfigValidators` | same set as `resourcevalidator` |
| `providervalidator` | `ProviderWithConfigValidators.ConfigValidators` | same set, scoped to provider config |
| `actionvalidator` / `listresourcevalidator` / `ephemeralvalidator` | newer framework primitives | mirror set; only reach for them if you're already using the corresponding primitive |
| `dynamicvalidator` | `DynamicAttribute.Validators` | type-shape checks specific to dynamic values |

Use these schema-level builders (rather than per-attribute `ConflictsWith` / `AlsoRequires`) when the constraint is genuinely about the whole resource, not one attribute. Per-attribute placement is fine when one attribute is the "primary" — but if A, B, C are symmetric alternatives, a single `resourcevalidator.ExactlyOneOf(A, B, C)` reads cleaner than three reciprocal `ConflictsWith` lists.

## Custom validators

If nothing in the validators package fits, write your own:

```go
type lowercaseValidator struct{}

func (v lowercaseValidator) Description(ctx context.Context) string         { return "must be lowercase" }
func (v lowercaseValidator) MarkdownDescription(ctx context.Context) string { return "must be lowercase" }

func (v lowercaseValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
    if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
        return
    }
    s := req.ConfigValue.ValueString()
    if s != strings.ToLower(s) {
        resp.Diagnostics.AddAttributeError(req.Path, "must be lowercase", "got "+s)
    }
}
```

Implement the corresponding `Validate*` for the kind: `ValidateString`, `ValidateInt64`, `ValidateBool`, `ValidateList`, etc.

## Validators run early — `ProviderData` is nil

Validators run during config validation, before plan. The framework *does* call `Configure` on the resource before validation, but it passes `req.ProviderData == nil` because the provider's own `Configure` (which produces the client) hasn't run yet. So `r.client` is unset when a validator runs. Don't reach for the API client inside a validator — it's nil, and the validator should be pure config inspection anyway. Put API-touching checks in `Read`/`Create`/`Update` and surface them via diagnostics.
