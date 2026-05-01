# Migration Summary — openstack_compute_interface_attach_v2

## Source file
`openstack/resource_openstack_compute_interface_attach_v2.go`

## Key conversions performed

### ConflictsWith → per-attribute validators
All three `ConflictsWith` annotations were converted to `stringvalidator.ConflictsWith(path.MatchRoot(...))` placed in the `Validators` field of each attribute:

- `port_id`: conflicts with `network_id`
- `network_id`: conflicts with `port_id`
- `fixed_ip`: conflicts with `port_id`

Import: `"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"` and `"github.com/hashicorp/terraform-plugin-framework/path"`.

### ForceNew → RequiresReplace plan modifiers
All five `ForceNew: true` fields became `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`. Computed+Optional fields additionally got `stringplanmodifier.UseStateForUnknown()`.

### Resource shape
SDKv2 function `resourceComputeInterfaceAttachV2() *schema.Resource` → framework type `computeInterfaceAttachV2Resource` implementing `resource.Resource`, `ResourceWithConfigure`, and `ResourceWithImportState`.

### State access
All `d.Get`/`d.Set`/`d.SetId` calls replaced with typed model struct `computeInterfaceAttachV2Model` using `tfsdk:"..."` tags.

### retry.StateChangeConf → inline waitForState
The SDKv2 `retry.StateChangeConf` pattern was replaced with a local `waitForState` helper that mirrors the semantics without importing `terraform-plugin-sdk/v2`. The existing `computeInterfaceAttachV2AttachFunc` and `computeInterfaceAttachV2DetachFunc` helpers (which return `retry.StateRefreshFunc`) remain unchanged — Go's assignability rules allow `retry.StateRefreshFunc` (an alias for `func() (interface{}, string, error)`) to be passed where `func() (any, string, error)` is expected.

### Timeouts
`schema.ResourceTimeout{Create, Delete}` → `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` (block syntax preserved for backward-compat). Model field `Timeouts timeouts.Value` added. Create/Delete methods read the timeout via `plan.Timeouts.Create(ctx, 10*time.Minute)` / `state.Timeouts.Delete(ctx, 10*time.Minute)`.

### Import
`schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

### Read drift handling
`CheckDeleted` (SDKv2-dependent) replaced with direct `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` check followed by `resp.State.RemoveResource(ctx)`.

### Region handling
`GetRegion(d, config)` (SDKv2-dependent) replaced with inline: read `plan.Region.ValueString()` and fall back to `r.config.Region` when empty.

## Test file changes
- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: protoV6ProviderFactoriesComputeInterfaceAttach`
- Added `ImportState`/`ImportStateVerify` steps with `ImportStateVerifyIgnore: []string{"timeouts"}`
- Provider factory references `NewFrameworkProvider("test")()` (replace with the actual framework provider constructor name used by this repo)

## Imports removed from resource file
- `github.com/hashicorp/terraform-plugin-sdk/v2/diag`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema`

## Imports added to resource file
- `github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts`
- `github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator`
- `github.com/hashicorp/terraform-plugin-framework/path`
- `github.com/hashicorp/terraform-plugin-framework/resource`
- `github.com/hashicorp/terraform-plugin-framework/resource/schema`
- `github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier`
- `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier`
- `github.com/hashicorp/terraform-plugin-framework/schema/validator`
- `github.com/hashicorp/terraform-plugin-framework/types`
- `github.com/gophercloud/gophercloud/v2` (for `ResponseCodeIs`)
- `net/http` (for `http.StatusNotFound`)
- `slices` (for `waitForState`)

## Notes / follow-ups
1. The `NewFrameworkProvider` constructor name in the test file must be updated to match the actual framework provider factory function in this repo once the provider itself is migrated.
2. The `waitForState` helper is defined inline in this file. If other resources in the package are migrated, consider extracting it to a shared utility file to avoid duplicate declarations.
3. The `computeInterfaceAttachV2AttachFunc` / `computeInterfaceAttachV2DetachFunc` helpers in `compute_interface_attach_v2.go` still import `terraform-plugin-sdk/v2/helper/retry`. They should be migrated (return type changed to `func() (any, string, error)`) when the full package migration sweeps that file.
4. The `testAccProvider.Meta()` pattern in the destroy/exists check helpers still relies on the SDKv2 provider being available for test setup. This is expected in a partial migration; the check helpers can remain SDKv2-based until the provider itself is fully migrated.
