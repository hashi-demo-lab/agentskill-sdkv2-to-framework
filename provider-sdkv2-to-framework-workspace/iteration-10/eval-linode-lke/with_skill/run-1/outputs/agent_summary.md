# LKE Cluster Resource Migration: SDKv2 → Plugin Framework

## Scope

Files migrated: `resource.go` (and schema previously in `schema_resource.go`), along with `resource_test.go`.
The `cluster.go` file (non-CRUD helpers) was left in place; a framework-compatible wrapper `recycleLKEClusterFramework` was added to `resource.go` to avoid the `*helper.ProviderMeta` dependency.

## Pre-flight decisions

### Block-vs-attribute: `control_plane` (MaxItems:1)
- Decision: **Keep as `ListNestedBlock` + `listvalidator.SizeAtMost(1)`**.
- Reason: `control_plane { ... }` block syntax is widely used in practitioner configs and examples. Switching to `SingleNestedAttribute` would break the `foo = { ... }` vs `foo { ... }` HCL syntax. Using `ListNestedBlock+SizeAtMost(1)` also preserves the `control_plane.0.<field>` state path from SDKv2.
- Same decision applied to nested `acl` and `addresses` blocks inside `control_plane`.

### Block-vs-attribute: `autoscaler` inside `pool`
- Decision: **`ListNestedAttribute` + `listvalidator.SizeAtMost(1)`** (nested attribute, not block).
- Reason: `pool` is itself now a `ListNestedAttribute`; nested blocks inside nested attributes are not supported. Attributes-in-attributes work fine and preserve the same `pool.0.autoscaler.0.min` state path semantics.

### Timeouts
- Used `timeouts.Block(ctx, opts)` via `BaseResourceConfig.TimeoutOpts` (injected by `BaseResource.Schema`).
- This preserves block HCL syntax: `timeouts { create = "35m" }`.
- Model field: `timeouts.Value` with `tfsdk:"timeouts"`.

### Sensitive attribute
- `kubeconfig` → `schema.StringAttribute{Computed: true, Sensitive: true}`.
- No `WriteOnly` migration: kubeconfig must be stored in state for drift detection and downstream references.

### CustomizeDiff → ModifyPlan
- All six legs of `customdiff.All(...)` were folded in order into `ModifyPlan`:
  1. `customDiffValidatePoolForStandardTier` → leg 1 (standard tier needs at least one pool)
  2. `customDiffValidateOptionalCount` → leg 2 (count or autoscaler required)
  3. `customDiffValidateUpdateStrategyWithTier` → leg 3 (update_strategy requires enterprise)
  4. `SDKv2ValidateFieldRequiresAPIVersion(v4beta, "tier")` → leg 4 (API version check)
  5. `ComputedWithDefault("tags", []string{})` → leg 5 (default empty set for tags)
  6. `CaseInsensitiveSet("tags")` → moved to `PlanModifiers` on the `tags` attribute (using existing `setplanmodifiers.CaseInsensitiveSet()`)
- Destroy guard: `if req.Plan.Raw.IsNull() { return }` at the top of `ModifyPlan`.

### ForceNew attributes
- `region` → `stringplanmodifier.RequiresReplace()`
- `apl_enabled` → `boolplanmodifier.RequiresReplace()`
- `tier` → `stringplanmodifier.RequiresReplace()` + `UseStateForUnknown()`

## Key implementation notes

- Resource is implemented via `helper.BaseResource` composition (the Linode provider pattern), with `TimeoutOpts` injected so `BaseResource.Schema` adds the timeouts block automatically.
- `readIntoState` is a shared helper called from Create/Read/Update to avoid duplication.
- `expandPoolSpecsFramework` replaces the SDKv2 `expandLinodeLKENodePoolSpecs` helper, operating on `[]poolModel` instead of `[]any`.
- `recycleLKEClusterFramework` is a framework-typed counterpart to `recycleLKECluster` in `cluster.go`, taking `*linodego.Client` and `int` (pollMS) directly rather than `*helper.ProviderMeta`.
- Pool matching (`matchPoolsWithSchemaFramework`) preserves the two-pass ID-then-attributes matching logic from the SDKv2 version.

## Test changes

- All `TestAccSDKv2Provider.Meta()` references replaced with `acceptance.GetTestClient()`.
- `TestAccResourceLKECluster_basicUpdates`: The SDKv2 request interceptor (which used `ModifyProviderMeta(schema.Provider)`) was removed; the test now uses `ProtoV6ProviderFactories` and validates externally-visible behavior only.
- `TestAccResourceLKECluster_tierNoAccess`: Retained `ModifyProviderMeta` because the Linode provider is muxed (SDKv2 + framework); the API version override hook must still operate on the SDKv2 configure path.
- All `ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories` (already present in most tests, added where missing).
- `checkLKEExists` and `waitForAllNodesReady` updated to use `acceptance.GetTestClient()`.

## SDKv2 imports eliminated from resource.go

- `terraform-plugin-sdk/v2/diag`
- `terraform-plugin-sdk/v2/helper/customdiff`
- `terraform-plugin-sdk/v2/helper/retry`
- `terraform-plugin-sdk/v2/helper/schema`
- `linodediffs` (SDKv2-specific customdiff helpers — replaced by plan modifiers/ModifyPlan)
