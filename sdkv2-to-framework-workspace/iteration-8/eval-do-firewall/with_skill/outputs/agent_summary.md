# Migration Summary — digitalocean_firewall

## What was migrated

`digitalocean/firewall/resource_firewall.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Key decisions

### CustomizeDiff → ResourceWithModifyPlan

The SDKv2 resource had a single inline `CustomizeDiff` function that validated:
1. At least one inbound or outbound rule must be present.
2. For inbound rules with non-icmp protocol, `port_range` is required.
3. For outbound rules with non-icmp protocol, `port_range` is required.

This was translated to a `ModifyPlan` method on `*firewallResource` implementing `resource.ResourceWithModifyPlan`. The compile-time assertion `var _ resource.ResourceWithModifyPlan = &firewallResource{}` is present. No `customdiff.` helper was used in the source; none remains.

### Schema shape

`inbound_rule` and `outbound_rule` were `TypeSet` of `&schema.Resource{...}` in SDKv2. They became `schema.SetNestedAttribute` in the framework. Practitioners use the same block syntax (`inbound_rule { ... }`) because `SetNestedAttribute` renders identically in HCL.

`droplet_ids` (TypeSet of TypeInt) → `schema.SetAttribute{ElementType: types.Int64Type}`.

`tags` (TypeSet of TypeString via `tag.TagsSchema()`) → `schema.SetAttribute{ElementType: types.StringType}`.

`pending_changes` (TypeList of nested Resource, Computed) → `schema.ListNestedAttribute{Computed: true}`.

`Set: schema.HashString` / `Set: util.HashStringIgnoreCase` helpers dropped — framework handles set uniqueness internally.

`ValidateFunc: validation.NoZeroValues` on `name` → `stringvalidator.LengthAtLeast(1)`.
`ValidateFunc: validation.StringInSlice(...)` on `protocol` → `stringvalidator.OneOf(...)`.

### CRUD

All four CRUD methods migrated to the typed framework signatures. `d.SetId("")` → `resp.State.RemoveResource(ctx)`. `d.Set(...)` / `d.Get(...)` replaced with typed model struct + `req.Plan.Get` / `resp.State.Set`. `diag.Errorf(...)` → `resp.Diagnostics.AddError(...)`.

`tag.FlattenTags` (which returns `*schema.Set`) was replaced with a local `stringSliceToSet` helper that produces `types.Set`. `tag.ExpandTags` (which takes `[]interface{}`) was replaced with direct `[]string` slice handling.

The SDKv2 `firewallRequest(d, client)` helper (package-level function operating on `*schema.ResourceData`) was replaced by `buildFirewallRequest(ctx, firewallModel)` (package-level function operating on the typed model).

### Import

`schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

### Compile-time assertions

```go
var (
    _ resource.Resource                = &firewallResource{}
    _ resource.ResourceWithConfigure   = &firewallResource{}
    _ resource.ResourceWithImportState = &firewallResource{}
    _ resource.ResourceWithModifyPlan  = &firewallResource{}
)
```

## Test file changes

- `ProviderFactories: acceptance.TestAccProviderFactories` (SDKv2 `*schema.Provider` factory) → `ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories` (framework `tfprotov6.ProviderServer` factory). **Note:** `acceptance.TestAccProtoV6ProviderFactories` must be added to the acceptance package as part of the broader provider migration.
- Import changed from `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` to `github.com/hashicorp/terraform-plugin-testing/helper/resource`.
- Import changed from `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` to `github.com/hashicorp/terraform-plugin-testing/terraform`.
- Check helpers (`testAccCheckDigitalOceanFirewallDestroy`, `testAccCheckDigitalOceanFirewallExists`) no longer use `acceptance.TestAccProvider.Meta()` (SDKv2 pattern). They construct a `*godo.Client` directly from `DIGITALOCEAN_TOKEN` via a local `godoClientFromEnv()` helper.
- HCL config functions are unchanged.

## Checklist status

- No `customdiff.` helper used (source had none; none emitted).
- No `CustomizeDiff:` field present in output.
- `var _ resource.ResourceWithModifyPlan = &firewallResource{}` assertion present.
- No `github.com/hashicorp/terraform-plugin-sdk/v2` import in migrated resource file.
- `ModifyPlan` method present and correctly validates the cross-attribute constraints.

## New go.mod dependencies required

```
github.com/hashicorp/terraform-plugin-framework         vX.Y.Z
github.com/hashicorp/terraform-plugin-framework-validators vX.Y.Z
github.com/hashicorp/terraform-plugin-testing           vX.Y.Z
github.com/hashicorp/terraform-plugin-go                vX.Y.Z  (transitive, for tfprotov6)
```

Run `go get github.com/hashicorp/terraform-plugin-framework` and `go get github.com/hashicorp/terraform-plugin-framework-validators` then `go mod tidy`.
