# Migration Summary — linode_domain resource

## Source files
- `linode/domain/resource.go` (SDKv2 CRUD + CustomizeDiff)
- `linode/domain/schema_resource.go` (SDKv2 schema with DiffSuppressFunc on 4 int fields)

## Output files
- `migrated/resource.go` — full framework resource, zero SDKv2 imports
- `migrated/resource_test.go` — updated test file

---

## Key translation decisions

### DiffSuppressFunc → plan modifier (non-destructive)

`helper.DomainSecondsDiffSuppressor()` was applied to `ttl_sec`, `retry_sec`, `expire_sec`, and `refresh_sec`. The suppressor compared the API-returned value (rounded to a valid boundary) with the declared value and suppressed the diff when they matched.

**Translation**: `domainSecondsPlanModifier` (implements `planmodifier.Int64`).  
The modifier adjusts the *planned* value to the rounded value the API will return, so Terraform's state comparison sees equivalent values. This is **non-destructive**: the Create/Update methods still forward the user's original value to the API; the rounding only affects the plan so no spurious perpetual diffs appear. A destructive approach (normalising in `ValueFromX` / writing rounded value to state before the API call) was explicitly rejected per the skill's hard rules.

Each field also gets `int64planmodifier.UseStateForUnknown()` (listed first) to keep computed values stable across plans.

### CustomizeDiff → ModifyPlan + attribute-level plan modifiers

The `customdiff.All(...)` chain had two legs:

| SDKv2 CustomizeDiff leg | Framework translation |
|---|---|
| `linodediffs.ComputedWithDefault("tags", []string{})` | `Default: setdefault.StaticValue(emptyStringSet)` on the `tags` attribute — the framework's `Default` field handles the "use this value when the practitioner omits the attribute" case declaratively. `ModifyPlan` is still implemented (to satisfy `ResourceWithModifyPlan`) but its body only contains the destroy short-circuit guard. |
| `linodediffs.CaseInsensitiveSet("tags")` | `PlanModifiers: []planmodifier.Set{setplanmodifiers.CaseInsensitiveSet()}` on the `tags` attribute — the provider already ships this plan modifier at `linode/helper/setplanmodifiers/caseinsensitiveset.go`. |

`ModifyPlan` is declared with the `var _ resource.ResourceWithModifyPlan = &DomainResource{}` compile-time check.

### ConflictsWith → framework validators

The SDKv2 schema had no explicit `ConflictsWith` on the resource attributes — the data-source schema (`framework_schema_datasource.go`) uses `int64validator.ConflictsWith` on `id` vs `domain`, which is already framework-native and untouched. The resource schema had no cross-attribute conflicts to migrate.

### ForceNew → RequiresReplace

`type` had `ForceNew: true` in SDKv2. Translated to:
```go
PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}
```

### Import

`schema.ImportStatePassthroughContext` becomes `ImportState` setting `path.Root("id")` via `resp.State.SetAttribute`. The `BaseResource.ImportState` could handle this automatically (it reads `IDAttr = "id"`), but an explicit override is provided for clarity and to emit the debug log.

### ResourceModel

`MasterIPs` and `AXFRIPs` use `types.Set` (not `[]types.String`) to match the schema's `SetAttribute`. Other slice fields (`Tags`) remain `[]types.String` because the datasource model uses that pattern and `Plan.Get` handles the conversion.

### Computed fields with UseStateForUnknown

All `Computed`-only or `Optional+Computed` scalar attributes that the API determines (group, status, description, soa_email, ttl_sec, retry_sec, expire_sec, refresh_sec) carry `UseStateForUnknown()` so they don't show `(known after apply)` on every plan when unchanged.

---

## Test file changes

- Removed import of `"github.com/linode/terraform-provider-linode/v3/linode/helper"` (no longer needed).
- `checkDomainExists` and `checkDestroy` now use `acceptance.GetTestClient()` instead of `acceptance.TestAccSDKv2Provider.Meta().(*helper.ProviderMeta).Client` — removes the SDKv2 provider dependency from the check functions.
- All test cases already used `ProtoV6ProviderFactories` — no change needed there.
- Removed the orphaned `configBasic` / `configRoundedSec` helper functions (they were not called by any test; tests use `tmpl.*` instead).
- Build tag `//go:build integration || domain` preserved.

---

## What was NOT changed

- `framework_datasource.go`, `framework_schema_datasource.go`, `framework_models.go` — already framework-native.
- `tmpl/template.go` — no changes required.
- The `linode/domain` package's `DomainModel` (used by the datasource) is kept separate from the new `ResourceModel` (used by the resource) to avoid coupling.

---

## Registration note

To complete the migration, add `domain.NewResource` to the framework provider's `Resources()` list in `linode/framework_provider.go` (alongside the existing `domain.NewDataSource`), and remove `domain.Resource()` from the SDKv2 provider's `ResourcesMap` in `linode/provider.go`.
