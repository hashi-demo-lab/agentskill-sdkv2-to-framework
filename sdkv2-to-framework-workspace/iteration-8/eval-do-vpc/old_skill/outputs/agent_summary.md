# Migration Summary — digitalocean_vpc

## What was done

Migrated `resource_vpc.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework` and updated `resource_vpc_test.go` to use `ProtoV6ProviderFactories`.

## Resource analysis (pre-edit checklist)

1. **Block decision**: No `MaxItems: 1 + nested Elem` attributes. All attributes are primitives. `Timeouts` block preserved using `timeouts.Block(ctx, timeouts.Opts{Delete: true})` to maintain backward-compatible HCL block syntax.
2. **State upgrade**: `SchemaVersion` is 0 (not set). No state upgraders needed.
3. **Import shape**: Simple passthrough — `schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Key decisions

### Schema
- `name` / `region`: `validation.NoZeroValues` → `stringvalidator.LengthAtLeast(1)` (per validator mapping table).
- `region`: `ForceNew: true` → `stringplanmodifier.RequiresReplace()`.
- `ip_range`: `ForceNew: true` + `Computed: true` → `stringplanmodifier.RequiresReplaceIfConfigured()` + `stringplanmodifier.UseStateForUnknown()`. `validation.IsCIDR` → inline `cidrValidator` struct using `net.ParseCIDR` (preserves string storage type, avoids breaking state with a custom nettypes type).
- `description`: Added `Computed: true` and `UseStateForUnknown()` so API-returned empty description doesn't create drift.
- All Computed fields: `UseStateForUnknown()` applied so plan stays clean on unchanged resources.
- `id`: Explicit `schema.StringAttribute{Computed: true, PlanModifiers: [UseStateForUnknown]}`.

### CRUD
- `retry.RetryContext` (SDKv2) → inline ticker-based retry loop per `resources.md` guidance. No `terraform-plugin-sdk/v2/helper/retry` import retained.
- `d.Get`/`d.Set`/`d.SetId` → typed model struct (`vpcResourceModel`) with `tfsdk` tags. `req.Plan.Get` / `resp.State.Set`.
- `Update`: reads both plan and state; `Default` field (Computed, server-set) is sourced from state for the update API call.
- `Read`: `d.SetId("")` → `resp.State.RemoveResource(ctx)`.
- `Delete`: uses `state.Timeouts.Delete(ctx, 2*time.Minute)` default matching the original 2-minute timeout.

### Tests
- `ProviderFactories` → `ProtoV6ProviderFactories` with `providerserver.NewProtocol6WithError`.
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` → `github.com/hashicorp/terraform-plugin-testing/helper/resource`.
- `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` → `github.com/hashicorp/terraform-plugin-testing/terraform`.
- `import_vpc_test.go` content merged into `resource_vpc_test.go` (single file for resource tests, consistent with framework convention).
- `testAccCheckDigitalOceanVPCDestroy` / `testAccCheckDigitalOceanVPCExists`: updated to use `vpcTestClient()` helper; logic is identical to the original.
- `acceptance.TestAccProvider.Meta()` call retained in helpers — this still works because the acceptance `TestAccProvider` is configured during `PreCheck`.

## Outstanding manual step

`testAccProtoV6ProviderFactories` references the framework provider constructor (`nil` placeholder). Once the top-level provider has been migrated to `terraform-plugin-framework`, replace `nil` with `digitalocean.NewFrameworkProvider()` (or equivalent) so the factory can boot the server.

## Files produced

- `migrated/resource_vpc.go` — full framework resource (no SDKv2 imports)
- `migrated/resource_vpc_test.go` — updated test file (ProtoV6ProviderFactories, terraform-plugin-testing)
