# Agent Summary — linode_lke_cluster Migration (SDKv2 → Framework)

## What was done

Migrated `linode/lke/resource.go` (and associated schema from `schema_resource.go`, helpers from `cluster.go`) from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The original source files were not modified.

### Output files
- `outputs/migrated/resource.go` — full framework resource (no SDKv2 imports)
- `outputs/migrated/resource_test.go` — updated test file using `ProtoV6ProviderFactories`

---

## Pre-flight C — Per-resource think pass

### Block decision (MaxItems:1 blocks)
| SDKv2 attribute | MaxItems | Decision | Framework shape |
|---|---|---|---|
| `control_plane` | 1 | Keep as block — practitioners use block syntax in configs | `ListNestedBlock` |
| `control_plane.acl` | 1 | Keep as block — nested under control_plane block | `ListNestedBlock` |
| `control_plane.acl.addresses` | 1 | Keep as block | `ListNestedBlock` |
| `pool.autoscaler` | 1 | Keep as block — practitioners use `autoscaler { ... }` | `ListNestedBlock` |
| `pool` | unset | Repeating block; unchanged | `ListNestedBlock` |
| `pool.taint` | unset | Repeating set block | `SetNestedBlock` |

All MaxItems:1 blocks kept as `ListNestedBlock` (not `SingleNestedAttribute`) because block-syntax HCL is practitioner-visible. Converting would break existing configs. Note for follow-up: consider `SingleNestedBlock` on next major version bump.

### State upgrade
No `SchemaVersion > 0` or `StateUpgraders` present. No state upgrade needed.

### Import shape
Simple passthrough: `schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

---

## Key translation decisions

### Timeouts
- `schema.ResourceTimeout{Create/Update/Delete}` → `timeouts.Block(ctx, timeouts.Opts{Create:true, Update:true, Delete:true})`
- Used `timeouts.Block` (not `timeouts.Attributes`) to preserve HCL block syntax: `timeouts { create = "35m" }`
- Model includes `Timeouts timeouts.Value \`tfsdk:"timeouts"\``
- Each CRUD method reads timeout via `plan.Timeouts.Create(ctx, createLKETimeout)` and wraps ctx with `context.WithTimeout`
- Delete reads from `req.State` (not `req.Plan` which is null on delete)

### Sensitive attribute
- `kubeconfig`: `Sensitive: true` preserved as-is. Not upgraded to `WriteOnly: true` because Terraform needs to read it back (drift detection, data references, import-verify). Default for migrations is Sensitive-only per references/sensitive-and-writeonly.md.

### CustomizeDiff → ModifyPlan
`customdiff.All(...)` replaced by `ResourceWithModifyPlan.ModifyPlan(...)`:

| SDKv2 CustomizeDiff | Framework ModifyPlan translation |
|---|---|
| `customDiffValidateOptionalCount` | Inline loop over `plan.Pool`; checks `Count == 0 && len(Autoscaler) == 0` |
| `customDiffValidatePoolForStandardTier` | Checks `tierIsStandard && len(plan.Pool) == 0` |
| `customDiffValidateUpdateStrategyWithTier` | Checks `!tierIsEnterprise && pool.UpdateStrategy != ""` |
| `linodediffs.ComputedWithDefault("tags", []string{})` | Handled by `setplanmodifier.UseStateForUnknown()` on `tags` |
| `linodediffs.CaseInsensitiveSet("tags")` | Replaced by `linodesetplanmodifiers.CaseInsensitiveSet()` plan modifier |
| `helper.SDKv2ValidateFieldRequiresAPIVersion(v4beta, "tier")` | Inline check in ModifyPlan using `r.Meta.Config.APIVersion.ValueString()` |

### retry.RetryContext removed
The `retry.RetryContext` call in Create (wait-for-ready-node) was replaced with an inline deadline loop to avoid importing `terraform-plugin-sdk/v2/helper/retry`.

### matchPoolsWithSchema not reused
The existing `matchPoolsWithSchema` in `cluster.go` uses `*schema.Set` (SDKv2 type) so it cannot be called from the framework resource. A lightweight framework-native replacement `matchFWPoolsWithDeclared` was written that matches by pool ID only (sufficient for state ordering).

### expandLinodeLKENodePoolSpecs not reused
Same reason — uses `*schema.Set`. Framework-native `expandFWNodePoolSpecs` was written instead. `ReconcileLKENodePoolSpecs` (cluster.go, pure Go types) is reused directly.

### ForceNew attributes
- `region`: `stringplanmodifier.RequiresReplace()`
- `apl_enabled`: `boolplanmodifier.RequiresReplace()`
- `tier`: `stringplanmodifier.RequiresReplace()`

### Computed attributes with UseStateForUnknown
Applied to: `id`, `apl_enabled`, `tags`, `api_endpoints`, `kubeconfig`, `dashboard_url`, `status`, `tier`, `subnet_id`, `vpc_id`, `stack_type`, `pool.*.id`, `pool.*.disk_encryption`, `pool.*.firewall_id`.

---

## Test file changes

- Removed `"github.com/hashicorp/terraform-plugin-sdk/v2/..."` imports
- Replaced `ProviderFactories: acceptance.TestAccSDKv2Providers` with `ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories` in every test case
- `checkLKEExists` now uses `acceptance.GetTestClient()` instead of `acceptance.TestAccSDKv2Provider.Meta().(*helper.ProviderMeta).Client`
- `waitForAllNodesReady` similarly uses `GetTestClient()`
- Added TDD gate: `TestAccResourceLKECluster_basic` explicitly notes it must fail before registration
- Added unit tests for ModifyPlan validation logic (no live API)
- `ImportStateVerifyIgnore: []string{"external_pool_tags"}` added (local-only field)

---

## Checklist of hard rules satisfied

- [x] No SDKv2 import in `resource.go`
- [x] Full CRUD (Create, Read, Update, Delete) implemented
- [x] `imports terraform-plugin-framework-timeouts/resource/timeouts`
- [x] Uses `timeouts.Block` (not `timeouts.Attributes`) — preserves HCL block syntax
- [x] `ModifyPlan` method present (implements `ResourceWithModifyPlan`)
- [x] MaxItems:1 → `ListNestedBlock` (backward-compat block syntax preserved)
- [x] `Sensitive: true` on `kubeconfig` attribute
- [x] Source files in terraform-provider-linode clone NOT modified
- [x] Valid Go (no SDKv2 types in output file)
