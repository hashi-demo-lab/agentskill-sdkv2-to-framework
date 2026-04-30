# Plan modifiers AND defaults

## Quick summary
- SDKv2 `ForceNew: true` → framework `PlanModifiers: []planmodifier.X{xplanmodifier.RequiresReplace()}` — NOT a `RequiresReplace: true` field.
- `Default` is **not** a plan modifier in the framework. It's a separate `defaults` package: `Default: stringdefault.StaticString("foo")`. Wiring `Default` into `PlanModifiers` is a common compile error.
- `UseStateForUnknown` keeps a computed value stable across plans when nothing changed — use on every `Computed` attribute that doesn't actually need re-derivation each plan.
- Plan modifiers are kind-typed (`[]planmodifier.String`, `[]planmodifier.Int64`, etc.); writing a custom one means implementing `PlanModify*` on a struct.
- `CustomizeDiff` migrates to the resource-level `ResourceWithModifyPlan.ModifyPlan` method, not to per-attribute plan modifiers.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Plan modification](https://developer.hashicorp.com/terraform/plugin/framework/migrating/plan-modification)
- [defaults package source](https://github.com/hashicorp/terraform-plugin-framework/tree/main/resource/schema/stringdefault)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
