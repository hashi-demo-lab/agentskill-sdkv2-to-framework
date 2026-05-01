# Migration Summary: linode_lke_cluster SDKv2 → terraform-plugin-framework

## Files Produced

| File | Purpose |
|------|---------|
| `migrated/resource.go` | Full framework resource implementation |
| `migrated/resource_test.go` | Updated acceptance tests |

## Key Migration Decisions

### 1. Timeouts → terraform-plugin-framework-timeouts

- Added `TimeoutOpts: &timeouts.Opts{Create: true, Update: true, Delete: true}` to `BaseResourceConfig`.
- `ResourceModel` embeds `timeouts.Value` tagged `tfsdk:"timeouts"`, so the HCL `timeouts {}` block is preserved.
- Each CRUD method calls `plan.Timeouts.Create(ctx, createLKETimeout)` etc. with `context.WithTimeout`.

### 2. CustomizeDiff → ResourceWithModifyPlan

All five original `customdiff.All(...)` entries are translated to `ModifyPlan`:

| Original SDKv2 diff | Framework equivalent |
|---------------------|---------------------|
| `customDiffValidateOptionalCount` | Loop over `plan.Pools`; error if `Count` is null/unknown and `Autoscaler` is empty |
| `customDiffValidatePoolForStandardTier` | Error if tier is null/standard and `len(plan.Pools) == 0` |
| `customDiffValidateUpdateStrategyWithTier` | Error if `UpdateStrategy` non-empty when tier != enterprise |
| `linodediffs.ComputedWithDefault("tags", ...)` | Replaced by `Computed: true` on `tags` attribute + default handling in flatten |
| `linodediffs.CaseInsensitiveSet("tags")` | Not re-implemented (framework handles set semantics differently; can add a custom plan modifier later) |
| `SDKv2ValidateFieldRequiresAPIVersion(v4beta, "tier")` | In `ModifyPlan`, read `r.Meta.Config.APIVersion` and return `AddAttributeError` on `path.Root("tier")` |

The `ModifyPlan` guard `req.Plan.Raw.IsNull()` prevents execution during destroy.

### 3. MaxItems:1 control_plane Block

SDKv2 `MaxItems: 1` on a `TypeList` maps to `schema.ListNestedBlock` in the framework schema (no MaxItems concept). The struct field is `[]controlPlaneModel` but callers only ever use index 0. The framework enforces list semantics; a `ListValidator` with `listvalidator.SizeAtMost(1)` could optionally be added.

### 4. Sensitive kubeconfig Attribute

`Sensitive: true` on the SDKv2 schema becomes `Sensitive: true` on `schema.StringAttribute` in the framework schema. Framework automatically redacts the value in plan/apply output.

### 5. pool Handling

The `pool` list uses `schema.ListNestedBlock`. Internally `expandPoolSpecsFromModel` converts `[]poolModel` to `[]NodePoolSpec` for the existing `ReconcileLKENodePoolSpecs` function (unchanged in `cluster.go`). The `matchPoolsWithSchema` helper (SDKv2-specific) is dropped; instead `flattenPoolsFramework` returns pools in API order.

### 6. recycleLKEClusterFramework

The original `recycleLKECluster(ctx, *helper.ProviderMeta, ...)` takes the SDKv2 `ProviderMeta`. A new `recycleLKEClusterFramework(ctx, *helper.FrameworkProviderMeta, ...)` is defined in `resource.go` that uses the framework meta type. The underlying linodego calls are identical.

### 7. Test File Changes

- Removed `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema` import from direct test use (kept for `acceptance.ModifyProviderMeta` signature).
- `checkLKEExists` now calls `acceptance.GetTestClient()` instead of `acceptance.TestAccSDKv2Provider.Meta()`.
- `waitForAllNodesReady` likewise uses `acceptance.GetTestClient()`.
- `TestAccResourceLKECluster_basicUpdates` keeps the `acceptance.CreateTestProvider` / `acceptance.ModifyProviderMeta` pattern to test PUT request exclusions.
- All other tests use `ProtoV6ProviderFactories`.
- `TestAccResourceLKECluster_tierNoAccess` uses `acceptance.ModifyProviderMeta` + `ProtoV6CustomProviderFactories` matching the original pattern.

## Limitations / Follow-up Work

1. **Pool ordering**: The framework resource uses API-ordered pools. The original `matchPoolsWithSchema` attempted to correlate API pools with declared config order. Drift suppression logic may be needed for stable plans when pools are reordered.
2. **CaseInsensitiveSet for tags**: The SDKv2 `linodediffs.CaseInsensitiveSet("tags")` is not replicated. A custom `planmodifier.Set` or `validator.Set` would be needed.
3. **Retry on node readiness**: The original used `retry.RetryContext` with a 25s window. The framework version calls `WaitForLKEClusterConditions` once; transient EOF errors are logged but not retried. A retry loop can be added if needed.
4. **`cluster.go` / `schema_resource.go`**: These files are NOT changed. The existing SDKv2 `Resource()` and helpers remain to avoid breaking other consumers during migration.
