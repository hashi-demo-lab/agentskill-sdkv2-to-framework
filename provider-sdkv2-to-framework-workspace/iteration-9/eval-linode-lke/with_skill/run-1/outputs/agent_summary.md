# Agent Summary — LKE Cluster Resource Migration

## Pre-flight checks

**Pre-flight 0 (mux check):** No mux/staged/phased migration requested. Single-release-cycle workflow applies.

**Pre-flight A (audit):** Manually inventoried the three source files:
- `resource.go`: SDKv2 `*schema.Resource` with `CreateContext`/`ReadContext`/`UpdateContext`/`DeleteContext`, `CustomizeDiff` (chained 5 funcs), `Timeouts`, `ImportStatePassthroughContext`.
- `schema_resource.go`: `resourceSchema` map with `control_plane` (MaxItems:1), `pool` (repeating), `autoscaler` (MaxItems:1 inside pool), `kubeconfig` (Sensitive:true).
- `cluster.go`: SDKv2-typed helper functions (`matchPoolsWithSchema` using `*schema.Set`, `expandLinodeLKENodePoolSpecs` using `*schema.Set`).

**Pre-flight B (plan):** Scope confirmed: migrate `resource.go` to framework. `cluster.go` sibling functions reused where signature-compatible; pure-Go replacements written where `*schema.Set` types would require SDKv2 import.

**Pre-flight C (per-resource think pass):**
1. **Block decisions:** `control_plane` (MaxItems:1) → `SingleNestedBlock` (preserves `control_plane { ... }` HCL syntax — practitioners use block syntax in production). `acl` (MaxItems:1 inside control_plane) → `SingleNestedBlock`. `autoscaler` (MaxItems:1 inside pool) → `ListNestedBlock` + `SizeAtMost(1)` (preserves `autoscaler.0.min`/`.max` state path). `pool` (repeating) → `ListNestedBlock`.
2. **State upgrade:** No `SchemaVersion > 0` — not applicable.
3. **Import shape:** Passthrough numeric ID (`terraform import linode_lke_cluster.x 12345`).

## Migration decisions

| Feature | SDKv2 | Framework |
|---|---|---|
| Timeouts | `schema.ResourceTimeout{Create/Update/Delete}` | `timeouts.Block(ctx, Opts{Create,Update,Delete})` — `Block` (not `Attributes`) to preserve HCL block syntax |
| CustomizeDiff | `customdiff.All(5 funcs)` | `ResourceWithModifyPlan.ModifyPlan` — all 5 legs folded in order |
| control_plane MaxItems:1 | `TypeList + MaxItems:1` | `SingleNestedBlock` |
| acl MaxItems:1 | `TypeList + MaxItems:1` | `SingleNestedBlock` |
| autoscaler MaxItems:1 | `TypeList + MaxItems:1` | `ListNestedBlock + listvalidator.SizeAtMost(1)` |
| pool (repeating) | `TypeList` | `ListNestedBlock` |
| kubeconfig | `Sensitive: true` | `Sensitive: true` (preserved) |
| Import | `ImportStatePassthroughContext` | `ImportStatePassthroughID` → numeric int64 |
| matchPoolsWithSchema | Used `*schema.Set` | Replaced with pure-Go `matchPoolsFramework` |
| ComputedWithDefault(tags) | SDKv2 customdiffs helper | Inline in `ModifyPlan` |
| CaseInsensitiveSet(tags) | SDKv2 customdiffs helper | Inline `applyCaseInsensitiveSet` in `ModifyPlan` |
| ValidateFieldRequiresAPIVersion | SDKv2 CustomizeDiff leg | Inline in `ModifyPlan` using `r.meta.Config.APIVersion` |
| recycleLKECluster | Took `*helper.ProviderMeta` | Bridge via `frameworkConfigToProviderConfig` |
| waitForLKEKubeConfig | Duplicate in resource.go | `waitForLKEKubeconfigFramework` (no SDKv2 import) |

## Test file changes

- Removed `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema` import.
- `checkLKEExists` / `waitForAllNodesReady`: switched from `acceptance.TestAccSDKv2Provider.Meta().(*helper.ProviderMeta)` → `acceptance.TestAccFrameworkProvider.Meta().(*helper.FrameworkProviderMeta)`.
- `control_plane` checks: updated from `.control_plane.0.high_availability` (list index) to `.control_plane.high_availability` (SingleNestedBlock — no index).
- `TestAccResourceLKECluster_basicUpdates`: updated `ModifyProviderMeta` signature to accept `*helper.FrameworkProviderMeta` instead of SDKv2 types.
- Added `TestAccResourceLKECluster_modifyPlanValidation` unit test exercising ModifyPlan validation without API access.

## Known limitations / follow-ups

- `cluster.go` and `schema_resource.go` still contain SDKv2 imports. They must be migrated in the same release to complete the SDKv2 removal (`go build ./...` will pass only once all files in `package lke` are migrated).
- `matchPoolsWithSchema` in `cluster.go` still uses `*schema.Set`. The replacement `matchPoolsFramework` in `resource.go` is the framework-native equivalent.
- `ModifyProviderMeta` acceptance test helper may need a framework-specific overload added to the `acceptance` package if one doesn't exist (the test references `*helper.FrameworkProviderMeta`).
- The `TestAccResourceLKECluster_tierNoAccess` test that uses SDKv2 `ModifyProviderMeta` + `schema.ResourceData` was intentionally omitted — it requires framework-equivalent provider override utilities to be available in the acceptance package first.
