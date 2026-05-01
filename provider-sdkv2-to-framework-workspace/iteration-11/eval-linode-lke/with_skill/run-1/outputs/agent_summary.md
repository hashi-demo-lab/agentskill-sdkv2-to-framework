# LKE Cluster Migration: SDKv2 → Plugin Framework

## Pre-flight checks

- **Pre-flight 0 (mux check)**: Not a muxed migration. Single-release path confirmed.
- **Pre-flight A (audit)**: Source files reviewed: `resource.go`, `schema_resource.go`, `cluster.go`. Flagged patterns: `Timeouts`, `CustomizeDiff`, `MaxItems:1` blocks (`control_plane`, nested `acl`, `addresses`, `autoscaler`), `Sensitive` attribute (`kubeconfig`), `ForceNew` attributes.
- **Pre-flight B (scope)**: Three files in `linode/lke/` package migrated: `resource.go` + `schema_resource.go` (combined into single framework `resource.go`) and `cluster.go` (helper functions preserved in-package).
- **Pre-flight C (think pass)**:
  1. **Block decision**: `control_plane` (`MaxItems:1`) → kept as `ListNestedBlock + listvalidator.SizeAtMost(1)` to preserve HCL block syntax. Nested `acl` and `addresses` likewise kept as `ListNestedBlock`. `autoscaler` (per pool, `MaxItems:1`) → `ListNestedBlock + SizeAtMost(1)`. `taint` (repeating set) → `SetNestedBlock`. `pool` (repeating list) → `ListNestedBlock`.
  2. **State upgrade**: No `SchemaVersion > 0` / `StateUpgraders` in the source. Not applicable.
  3. **Import shape**: `ImportStatePassthroughContext` → `resource.ImportStatePassthroughID` on `path.Root("id")`.

## Migration decisions

### Timeouts
- Source: `&schema.ResourceTimeout{Create, Update, Delete}` with named constants.
- Target: `timeouts.Block(ctx, timeouts.Opts{Create:true, Update:true, Delete:true})` in the `Blocks` map of the schema (preserves HCL block syntax `timeouts { create = "35m" }`).
- Model field: `Timeouts timeouts.Value \`tfsdk:"timeouts"\``.
- CRUD reads timeout from plan/state via `plan.Timeouts.Create(ctx, createLKETimeout)` and wraps context with `context.WithTimeout`.

### CustomizeDiff → ModifyPlan
Three SDKv2 `customdiff.All(...)` legs translated to a single `ModifyPlan` method on the resource type:
1. `customDiffValidateOptionalCount` → iterates `plan.Pool`, errors if `count` is null/unknown and no `autoscaler` block is present.
2. `customDiffValidatePoolForStandardTier` → errors if tier is standard (or null) and `len(plan.Pool) == 0`.
3. `customDiffValidateUpdateStrategyWithTier` → errors if any pool has `update_strategy` set when tier is not enterprise.
4. `helper.SDKv2ValidateFieldRequiresAPIVersion("tier")` → errors at plan time if `tier` is configured but `r.Meta.Config.APIVersion != helper.APIVersionV4Beta`.

Short-circuit ordering preserved: each leg returns early on error before the next runs.

### MaxItems:1 blocks
All kept as blocks (not converted to `SingleNestedAttribute`) because practitioners use block syntax (`control_plane { ... }`) in production configs. Used `ListNestedBlock + listvalidator.SizeAtMost(1)` for `control_plane`, `acl`, `addresses`, and `autoscaler`.

### Sensitive attribute
`kubeconfig` → `schema.StringAttribute{Sensitive: true}` with `stringplanmodifier.UseStateForUnknown()` (value is set once on create, stable thereafter).

### ForceNew attributes
- `region` → `stringplanmodifier.RequiresReplace()`
- `tier` → `stringplanmodifier.RequiresReplace()`
- `apl_enabled` → `boolplanmodifier.RequiresReplace()`

### ID type
Framework resource uses `types.String` for `id` (stored as strconv of the integer cluster ID) to match the framework convention. `IDType: types.Int64Type` in `BaseResourceConfig` is used for import passthrough parsing.

### Computed attributes with UseStateForUnknown
`kubeconfig`, `dashboard_url`, `status`, `api_endpoints`, `tags`, `apl_enabled`, `subnet_id`, `vpc_id`, `stack_type`, `tier` all get `UseStateForUnknown()` where appropriate to suppress noisy `(known after apply)` for stable values.

## Output files

- `migrated/resource.go`: Full framework resource (no SDKv2 import; full CRUD; `timeouts.Block`; `ModifyPlan`; `ListNestedBlock+SizeAtMost(1)` for MaxItems:1 blocks; `Sensitive:true` on `kubeconfig`).
- `migrated/resource_test.go`: Updated test file using `ProtoV6ProviderFactories` throughout; SDKv2-specific test helpers (`TestAccSDKv2Provider`, `schema.ResourceData`) removed; two new tests added (`TestAccResourceLKECluster_timeouts`, `TestAccResourceLKECluster_sensitiveKubeconfig`).

## SDKv2 imports removed from migrated files
- `github.com/hashicorp/terraform-plugin-sdk/v2/diag`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation`
