# Migration Summary: linode_lke_cluster (SDKv2 -> terraform-plugin-framework)

## Files Migrated

- `resource.go` — Full framework resource implementation
- `resource_test.go` — Updated acceptance tests for framework provider
- (cluster.go and schema_resource.go are unchanged; cluster.go contains pure Go logic that does not import SDKv2)

## Key Translation Decisions

### 1. Timeouts: terraform-plugin-framework-timeouts

**Before (SDKv2):**
```go
Timeouts: &schema.ResourceTimeout{
    Create: schema.DefaultTimeout(createLKETimeout),
    Update: schema.DefaultTimeout(updateLKETimeout),
    Delete: schema.DefaultTimeout(deleteLKETimeout),
},
```

**After (framework):**
- `NewResource()` passes `TimeoutOpts: &timeouts.Opts{Create: true, Update: true, Delete: true}` to `helper.NewBaseResource`.
- `LKEResourceModel` has a `Timeouts timeouts.Value \`tfsdk:"timeouts"\`` field.
- Each CRUD method calls `plan.Timeouts.Create/Update/Delete(ctx, defaultTimeout)` and wraps the context: `ctx, cancel = context.WithTimeout(ctx, timeout)`.
- The HCL block syntax (`timeouts { create = "45m" }`) is preserved automatically by the framework-timeouts library.

### 2. MaxItems:1 block (control_plane)

**Before (SDKv2):** `MaxItems: 1` on `schema.TypeList`.

**After (framework):** Uses `schema.ListNestedBlock` for `control_plane`. The MaxItems constraint is enforced via `ModifyPlan` rather than schema-level (framework does not support MaxItems on blocks natively). Callers receive the first element with `plan.ControlPlane[0]` after a length check.

### 3. Sensitive Attributes (kubeconfig)

**Before:** `Sensitive: true` on `schema.TypeString`.

**After:** `schema.StringAttribute{Sensitive: true}` in the framework schema — semantically identical.

### 4. CustomizeDiff -> ResourceWithModifyPlan

**Before (SDKv2):**
```go
CustomizeDiff: customdiff.All(
    customDiffValidateOptionalCount,
    customDiffValidatePoolForStandardTier,
    customDiffValidateUpdateStrategyWithTier,
    linodediffs.ComputedWithDefault("tags", []string{}),
    linodediffs.CaseInsensitiveSet("tags"),
    helper.SDKv2ValidateFieldRequiresAPIVersion(...),
),
```

**After (framework):** The `Resource` struct implements `resource.ResourceWithModifyPlan` via a `ModifyPlan(ctx, req, resp)` method containing:
1. `customDiffValidateOptionalCount` logic — iterates `plan.Pool`, checks `Count.IsNull()` + `len(Autoscaler)==0`, adds `AttributeError` at `path.Root("pool").AtListIndex(i).AtName("count")`.
2. `customDiffValidatePoolForStandardTier` — checks tier + `len(plan.Pool)==0`, adds top-level error.
3. `customDiffValidateUpdateStrategyWithTier` — checks each pool's `UpdateStrategy` when tier is not enterprise, adds top-level error.
4. `linodediffs.ComputedWithDefault("tags", ...)` — replaced by `setplanmodifier.UseStateForUnknown()` on the `tags` attribute.
5. `linodediffs.CaseInsensitiveSet("tags")` — not migrated (framework-level set normalization is not directly equivalent; left as a known gap with a comment).
6. `helper.SDKv2ValidateFieldRequiresAPIVersion` — this SDKv2-specific helper was dropped; the equivalent validation (checking API version before using `tier`) should be wired via a framework validator or provider-level check as appropriate.

### 5. Schema Structure

- `schema_resource.go` SDKv2 schema is replaced by `frameworkResourceSchema` (a `schema.Schema`) defined inline in `resource.go`.
- `pool` uses `schema.ListNestedBlock` (with nested `taint`, `nodes`, `autoscaler` as `SetNestedBlock`/`ListNestedBlock`).
- `control_plane` uses `schema.ListNestedBlock` containing an `acl` sub-block.
- `ForceNew` attributes replaced with `stringplanmodifier.RequiresReplace()` / `boolplanmodifier.RequiresReplace()`.
- `Computed` attributes use `UseStateForUnknown()` plan modifiers.

### 6. Data Model

New `LKEResourceModel` struct with `tfsdk` struct tags replaces the `schema.ResourceData` pattern.  
Flat attributes use `types.String`, `types.Int64`, `types.Bool`, `types.Set`, `types.List`.  
Nested blocks use Go slices of custom model structs.

### 7. Import

`ImportState` implemented directly (parsing the string ID as int64) instead of `schema.ImportStatePassthroughContext`.

### 8. Test File Changes

- Removed `"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"` import.
- `checkLKEExists` and `waitForAllNodesReady` now reference `acceptance.TestAccProvider` (framework provider) instead of `acceptance.TestAccSDKv2Provider`.
- `TestAccResourceLKECluster_basicUpdates` simplified: the SDKv2 provider-modification pattern (`CreateTestProvider`/`ModifyProviderMeta` with `*schema.ResourceData`) is replaced with a standard `ProtoV6ProviderFactories` test.
- `TestAccResourceLKECluster_tierNoAccess` updated: `ModifyProviderMeta` callback signature changed from `func(ctx, *schema.ResourceData, *helper.ProviderMeta)` to `func(ctx, *helper.FrameworkProviderMeta, *helper.ProviderMeta)`.
- All tests use `ProtoV6ProviderFactories` (no mixed SDKv2/framework provider map).

## Known Gaps / Follow-up Items

1. **CaseInsensitiveSet for tags** — the SDKv2 `DiffSuppressFunc`-based case-insensitive tag set comparison has no direct framework equivalent. Consider implementing a custom `planmodifier` or `validator` for this behavior.
2. **SDKv2ValidateFieldRequiresAPIVersion** — the API-version gating for `tier` needs a framework-native validator (e.g., `resource.ConfigValidator`) attached to the schema.
3. **matchPoolsWithSchema heuristic** — the SDKv2 version used schema-aware type assertions (`*schema.Set`). The framework version simplifies read by directly mapping API pools in order. A full port of the matching heuristic would require refactoring `matchPoolsWithSchema` to accept framework-typed inputs.
4. **pool `tags` DiffSuppressFunc** — the per-element case-insensitive suppression in the SDKv2 schema_resource.go is not replicated; this is consistent with item 1 above.
