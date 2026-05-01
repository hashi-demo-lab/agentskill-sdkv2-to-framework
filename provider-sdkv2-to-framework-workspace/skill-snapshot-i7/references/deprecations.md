# Deprecations and renames

## Quick summary
- This is the "things you might emit by mistake" reference. It catalogues SDKv2 names the LLM is likely to keep typing, with the correct framework replacement.
- Most of the SDKv2 identifiers in this list are not just renamed — they're **removed** without a one-to-one replacement; you have to rethink the pattern.
- Sweep imports as part of workflow step 10. Anything still in `go.mod` from the SDKv2 era should be examined.
- Some helpers (e.g., `validation.*`) have direct ports in `terraform-plugin-framework-validators`; others (e.g., `helper/customdiff`) need a different pattern entirely.
- When in doubt, search the framework docs for the SDKv2 identifier — many removals are documented under "deprecations" or "feature comparison".

## Package name collisions to be careful of

Two name collisions cause confusion during migration:

1. **`tfsdk` is *not* `terraform-plugin-sdk`.** The framework has a package at `github.com/hashicorp/terraform-plugin-framework/tfsdk` containing core types (`Plan`, `State`, `Config`, `EphemeralResultData`). It is part of the framework. Importing it is fine and expected. The **legacy SDK** lives at `github.com/hashicorp/terraform-plugin-sdk/v2` (and its subpackages `helper/schema`, `helper/validation`, etc.). That's the one you're removing.

   - ✅ `import "github.com/hashicorp/terraform-plugin-framework/tfsdk"` — framework, fine
   - ❌ `import "github.com/hashicorp/terraform-plugin-sdk/v2/..."` — legacy, must be removed

2. **The struct tag `tfsdk:"attribute_name"` is named after the framework `tfsdk` package.** It is *not* a leftover from the legacy SDK. Don't try to rename the struct tag during migration — it is the correct framework-era spelling.

## Removed / replaced packages

| SDKv2 import | Framework replacement |
|---|---|
| `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema` | `github.com/hashicorp/terraform-plugin-framework/{provider,resource,datasource}/schema` (three packages — see `schema.md`) |
| `github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation` | `github.com/hashicorp/terraform-plugin-framework-validators/...` |
| `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` | `github.com/hashicorp/terraform-plugin-testing/helper/resource` |
| `github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff` | resource-level `ResourceWithModifyPlan.ModifyPlan` (no direct port) |
| `github.com/hashicorp/terraform-plugin-sdk/v2/diag` | the framework's `resp.Diagnostics` API |
| `github.com/hashicorp/terraform-plugin-sdk/v2/plugin` | `github.com/hashicorp/terraform-plugin-framework/providerserver` |
| `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` | mostly gone; type-specific replacements in `terraform-plugin-framework/types` |

## Removed types and fields

| SDKv2 | Status |
|---|---|
| `*schema.Provider` | replaced by `provider.Provider` interface |
| `*schema.Resource` | replaced by `resource.Resource` / `datasource.DataSource` interfaces |
| `*schema.ResourceData` | replaced by `req.{Plan,State,Config}` typed accessors |
| `schema.ResourceImporter` | replaced by `resource.ResourceWithImportState.ImportState` |
| `schema.ResourceTimeout` | replaced by `terraform-plugin-framework-timeouts` package |
| `schema.SchemaValidateFunc` / `SchemaValidateDiagFunc` | replaced by `Validators` slice (kind-typed) |
| `schema.SchemaDiffSuppressFunc` | no direct port — use custom types or plan modifiers (see `plan-modifiers.md`) |
| `schema.SchemaStateFunc` | no direct port — use custom types (see `state-and-types.md`) |
| `schema.SchemaSetFunc` (e.g., `HashString`) | **deleted entirely** — framework handles set uniqueness |
| `schema.Resource.Exists` | gone — use `RemoveResource(ctx)` from `Read` instead |
| `schema.Resource.MigrateState` (V0/V1 mechanism from SDKv1) | long obsolete; use `UpgradeState` |
| `schema.Resource.SchemaVersion` field | becomes `schema.Schema.Version` |
| `schema.Resource.StateUpgraders` field | becomes `ResourceWithUpgradeState.UpgradeState` method |
| `d.Get("path")` | typed `req.Plan.Get(ctx, &model)` |
| `d.Set("path", value)` | model field assignment + `resp.State.Set(ctx, model)` |
| `d.Id()` | `state.ID.ValueString()` |
| `d.SetId(s)` | `m.ID = types.StringValue(s)` + `resp.State.Set` |
| `d.SetId("")` | `resp.State.RemoveResource(ctx)` |
| `d.HasChange(p)` | `!plan.X.Equal(state.X)` |
| `d.GetChange(p)` | compare model fields directly |
| `d.IsNewResource()` | check `req.State.Raw.IsNull()` |
| `d.Partial` / `d.SetPartial` | gone — write each field as it succeeds |
| `diag.FromErr(err)` | `resp.Diagnostics.AddError("op failed", err.Error())` |
| `diag.Errorf(...)` | `resp.Diagnostics.AddError("...", "...")` |

## Renamed validators (most-common)

| SDKv2 | Framework |
|---|---|
| `validation.StringLenBetween(min, max)` | `stringvalidator.LengthBetween(min, max)` |
| `validation.StringIsNotEmpty` | `stringvalidator.LengthAtLeast(1)` |
| `validation.StringMatch(re, msg)` | `stringvalidator.RegexMatches(re, msg)` |
| `validation.StringInSlice(opts, false)` | `stringvalidator.OneOf(opts...)` |
| `validation.IntBetween(min, max)` | `int64validator.Between(int64(min), int64(max))` |
| `validation.IntAtLeast(min)` | `int64validator.AtLeast(int64(min))` |
| `validation.IntInSlice(opts)` | `int64validator.OneOf(opts...)` |
| `validation.ToDiagFunc(...)` | not needed; framework validators are diagnostic-aware |

## Renamed schema fields

| SDKv2 | Framework |
|---|---|
| `Required` | `Required` (same) |
| `Optional` | `Optional` (same) |
| `Computed` | `Computed` (same) |
| `Description` | `Description` (same); `MarkdownDescription` is new |
| `Sensitive: true` | `Sensitive: true` (same) |
| `Deprecated: "..."` | `DeprecationMessage: "..."` |
| `ForceNew: true` | `PlanModifiers: ...stringplanmodifier.RequiresReplace()` (NOT a field, a plan modifier) |
| `Default: x` | `Default: stringdefault.StaticString(x)` (NOT in `PlanModifiers`) |
| `ValidateFunc` / `ValidateDiagFunc` | `Validators: []validator.X{...}` |
| `ConflictsWith: []string{...}` | `Validators: []validator.X{stringvalidator.ConflictsWith(...)}` |
| `RequiredWith` | `Validators: []validator.X{stringvalidator.AlsoRequires(...)}` |
| `ExactlyOneOf` | `Validators: []validator.X{stringvalidator.ExactlyOneOf(...)}` |
| `AtLeastOneOf` | `Validators: []validator.X{stringvalidator.AtLeastOneOf(...)}` |
| `Set: schema.HashString` | **delete** — not needed |
| `MaxItems: 1` (with `Elem: &schema.Resource{...}`) | usually `SingleNestedAttribute` (see `blocks.md`) |
| `Elem: &schema.Schema{...}` | becomes `ElementType:` for collection attributes |
| `Elem: &schema.Resource{...}` | becomes nested attribute or block (see `blocks.md`) |

## Things you should never emit (after migration)

If `git grep` finds any of these in your migrated codebase, something went wrong:

- `terraform-plugin-sdk/v2` — any import path containing this
- `*schema.ResourceData`, `*schema.Provider`, `*schema.Resource` (the SDKv2 types)
- `d.Get(`, `d.Set(`, `d.Id()`, `d.SetId(`, `d.HasChange(`
- `diag.FromErr(`, `diag.Errorf(`
- `validation.StringLenBetween`, `validation.StringInSlice`, etc. (the SDKv2 validators package)
- `schema.HashString`, `schema.HashSchema` (set hashing — gone)
- `Importer: &schema.ResourceImporter` (replaced by `ImportState` method)
- `Timeouts: &schema.ResourceTimeout` (replaced by the framework-timeouts package)
- `schema.Resource{}.Exists` (the existence check — gone)
- `schema.DefaultTimeout(` (replaced by `plan.Timeouts.Create(ctx, defaultDuration)` from `terraform-plugin-framework-timeouts`, which returns `(time.Duration, diag.Diagnostics)`; see `timeouts.md`)
