# Plan modifiers AND defaults

> **The single biggest type-error trap in this migration:** `Default` is a *separate field* on the attribute struct — it's the `defaults` package (`stringdefault.StaticString(...)`), not a value you put inside `PlanModifiers`. SDKv2 had no such split, so it's tempting to bundle them. The compiler will catch it, but you'll lose minutes hunting the cause if you don't internalise this first. See "Defaults — separate package, NOT a plan modifier" below.

## Quick summary
- SDKv2 `ForceNew: true` → framework `PlanModifiers: []planmodifier.X{xplanmodifier.RequiresReplace()}` — NOT a `RequiresReplace: true` field.
- `Default` is **not** a plan modifier in the framework. It's a separate `defaults` package: `Default: stringdefault.StaticString("foo")`. Wiring `Default` into `PlanModifiers` is a common compile error.
- `UseStateForUnknown` keeps a computed value stable across plans when nothing changed — use on every `Computed` attribute that doesn't actually need re-derivation each plan.
- Plan modifiers are kind-typed (`[]planmodifier.String`, `[]planmodifier.Int64`, etc.); writing a custom one means implementing `PlanModify*` on a struct.
- `CustomizeDiff` migrates to the resource-level `ResourceWithModifyPlan.ModifyPlan` method, not to per-attribute plan modifiers.

## ForceNew → RequiresReplace

```go
// SDKv2
"name": {Type: schema.TypeString, Required: true, ForceNew: true}

// Framework
"name": schema.StringAttribute{
    Required: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(),
    },
}
```

`RequiresReplace()` lives in the per-type planmodifier package — one per attr type (`stringplanmodifier`, `int64planmodifier`, `boolplanmodifier`, `listplanmodifier`, etc., under `terraform-plugin-framework/resource/schema/...planmodifier`). Conditional variants: `RequiresReplaceIf(condition, ...)`, `RequiresReplaceIfConfigured()`.

## Defaults — separate package, NOT a plan modifier

The framework's `Default` attribute field uses the `defaults` package:

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
    "github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
    "github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
)

"region": schema.StringAttribute{
    Optional: true,
    Computed: true, // attributes with Default must be Computed
    Default:  stringdefault.StaticString("us-east-1"),
}
```

Per-type packages under `terraform-plugin-framework/resource/schema/...default`: `stringdefault.StaticString`, `int64default.StaticInt64`, `int32default.StaticInt32`, `booldefault.StaticBool`, `float64default.StaticFloat64`, `numberdefault.StaticBigFloat`. Collections: `{list,set,map,object}default.StaticValue(...)`. Dynamic-typed: `dynamicdefault.StaticValue(...)`. For computed/derived defaults, implement the `defaults.String` (etc.) interface.

**A common mistake**: putting `Default` *inside* the `PlanModifiers` slice. That's a type error — `Default` is its own field on the attribute struct, separate from `PlanModifiers`.

**Another rule**: an attribute with a `Default` must be `Computed: true`. The framework rejects this at provider boot (during `GetProviderSchema`) via `ValidateImplementation` — your `TestProvider` test will catch it. Practitioners can still set the value (because it's also `Optional: true`), but `Computed` is what lets the framework insert the default into the plan.

## UseStateForUnknown

For `Computed` attributes whose value rarely changes (created-once IDs, timestamps, generated names): keep the prior state value in the plan unless something forces re-derivation.

```go
"id": schema.StringAttribute{
    Computed: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.UseStateForUnknown(),
    },
}
```

Without this, every plan shows `(known after apply)` for these fields, which is noisy and sometimes triggers spurious replacements downstream.

### `UseStateForUnknown` vs `UseNonNullStateForUnknown` (nested attributes)

Framework v1.15.1 changed `UseStateForUnknown` to preserve null prior-state values. On nested attributes — where a child `Computed` field could legitimately have been null in a prior state but should be unknown after a parent object is added — this can produce "Provider produced inconsistent result after apply" errors.

Framework v1.17.0+ adds `UseNonNullStateForUnknown` which only carries the prior value forward when it was non-null:

```go
// On a top-level Computed scalar — UseStateForUnknown is fine.
"id": schema.StringAttribute{
    Computed:      true,
    PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
}

// On a Computed child inside a SingleNestedAttribute / ListNestedAttribute —
// prefer UseNonNullStateForUnknown to avoid the null-leak.
"created_at": schema.StringAttribute{
    Computed:      true,
    PlanModifiers: []planmodifier.String{stringplanmodifier.UseNonNullStateForUnknown()},
}
```

If you're targeting framework <v1.17.0, you don't have `UseNonNullStateForUnknown` available and may need to express the same logic via a custom plan modifier or live with the noisier plans on nested attributes.

## CustomizeDiff → ModifyPlan

SDKv2's `CustomizeDiff` (resource-level diff manipulation) becomes a method on the resource type. Both single functions and chained `customdiff.All(...)` migrate to a single `ModifyPlan` method — fold each leg into the new method body in order:

```go
var _ resource.ResourceWithModifyPlan = &thingResource{}

func (r *thingResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
    if req.Plan.Raw.IsNull() {
        return // resource is being destroyed
    }
    var plan, state thingModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if !req.State.Raw.IsNull() {
        resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    }
    if resp.Diagnostics.HasError() { return }

    if /* condition that should force replace */ {
        resp.RequiresReplace = path.Paths{path.Root("name")}
    }
    // or set computed values on resp.Plan
}
```

`ModifyPlan` runs after attribute-level plan modifiers. Use it for cross-attribute logic, not per-field replacement triggers (those belong on the attribute).

For SDKv2 providers that used `customdiff.All(checkA, checkB, checkC)` to chain multiple diff customizations, the framework has no `customdiff` helper — fold every leg into a single `ModifyPlan` method. Order the legs the same way `customdiff.All` did (it short-circuited on the first error); each leg should `return` early on `resp.Diagnostics.HasError()` so subsequent legs don't run with partial state.

### The four `ModifyPlan` states (cheat-sheet)

`req.Plan.Raw.IsNull()` and `req.State.Raw.IsNull()` together tell you which lifecycle phase you're in:

| `Plan.IsNull()` | `State.IsNull()` | Meaning |
|---|---|---|
| `false` | `true` | **Create** — no prior state |
| `false` | `false` | **Update** — both populated |
| `true` | `false` | **Destroy** — practitioner removed the resource |
| `true` | `true` | unreachable in practice |

Reach for these sentinels at the top of `ModifyPlan` to short-circuit the destroy case (no plan modifications to make) and to distinguish create-vs-update for cross-attribute logic that only applies on one of those.

## DiffSuppressFunc — not directly portable

SDKv2 `DiffSuppressFunc` was used for two distinct things:

1. **Equivalent representations**: e.g., uppercase/lowercase comparisons, JSON whitespace normalisation. → migrate to a **custom type** (see `state-and-types.md`) that normalises in `ValueFromString`/equivalent. The framework will then compare normalised values directly.
2. **"Don't show this diff"**: e.g., suppressing changes when an external system rewrites the value. → usually a **plan modifier** that pulls the old value forward when conditions are met (e.g., a custom plan modifier that calls `resp.PlanValue = req.StateValue` when appropriate).

Don't try to translate `DiffSuppressFunc` 1:1 — analyse what it was actually doing and pick the right framework primitive.

## Custom plan modifiers

Implement the kind-specific interface:

```go
type idleStableModifier struct{}

func (m idleStableModifier) Description(ctx context.Context) string         { return "..." }
func (m idleStableModifier) MarkdownDescription(ctx context.Context) string { return "..." }

func (m idleStableModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
    if req.StateValue.IsNull() {
        return
    }
    if /* keep state value */ {
        resp.PlanValue = req.StateValue
    }
}
```

Use `planmodifier.StringRequest`, `Int64Request`, etc., and the corresponding response. The request gives you `ConfigValue`, `StateValue`, `PlanValue`, and request metadata.

## Plan modifier ordering

Plan modifiers on an attribute run in the order they appear in the slice. Put `UseStateForUnknown()` *before* any custom modifier that might return early on null state — otherwise the unknown-suppression logic never fires.
