# Migration Summary — openstack_compute_keypair_v2

## What was migrated

`resource_openstack_compute_keypair_v2.go` and its test file were migrated from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. No other files in the repo were modified.

## Resource analysis (pre-edit checklist)

1. **Block decision**: No `MaxItems: 1` nested Elem patterns. `value_specs` is a flat `TypeMap` → `schema.MapAttribute{ElementType: types.StringType}`. No block conversion needed.
2. **State upgrade**: `SchemaVersion` is 0 (not set). No state upgraders to handle.
3. **Import shape**: `schema.ImportStatePassthroughContext` — simple passthrough. Migrated to `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Key conversion decisions

| SDKv2 pattern | Framework equivalent | Notes |
|---|---|---|
| `*schema.Resource` function | `computeKeypairV2Resource` struct implementing `resource.Resource` | Plus `ResourceWithConfigure` and `ResourceWithImportState` |
| `schema.ResourceData` CRUD | Typed model struct `computeKeypairV2Model` with `tfsdk` tags | All `d.Get`/`d.Set` replaced with `req.Plan.Get`/`resp.State.Set` |
| `ForceNew: true` on region, name, public_key, value_specs, user_id | `stringplanmodifier.RequiresReplace()` / `mapplanmodifier.RequiresReplace()` | Per SKILL.md: ForceNew → RequiresReplace plan modifier |
| `Computed` on region, public_key, private_key, fingerprint, user_id | `stringplanmodifier.UseStateForUnknown()` added | Prevents spurious `(known after apply)` on stable fields |
| `d.SetId("")` on 404 in Read | `resp.State.RemoveResource(ctx)` | Framework idiom for drift detection |
| `diag.Errorf` / `diag.FromErr` | `resp.Diagnostics.AddError(...)` | |
| `schema.ImportStatePassthroughContext` | `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` | |
| `MapValueSpecs(d)` | Manual `plan.ValueSpecs.ElementsAs(ctx, &valueSpecs, false)` | `MapValueSpecs` takes `*schema.ResourceData`; replaced inline |
| `CheckDeleted(d, err, msg)` | Inline `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` check | `CheckDeleted` takes `*schema.ResourceData`; replaced inline |
| `GetRegion(d, config)` | `plan.Region.ValueString()` with fallback to `r.config.Region` | `GetRegion` takes `*schema.ResourceData`; replaced inline |

## `value_specs` attribute

The SDKv2 `TypeMap` with no explicit `Elem` defaults to `interface{}` values but is effectively string-valued in this codebase (confirmed by `MapValueSpecs` cast). Migrated to `schema.MapAttribute{ElementType: types.StringType}` which is correct for `map[string]string`.

## Update method

All attributes carry `RequiresReplace`, so Terraform never calls `Update` — it destroys and recreates. The `Update` method is present (required by the `resource.Resource` interface) but is a documented no-op.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: protoV6ProviderFactories`
- Added `protoV6ProviderFactories` var using `providerserver.NewProtocol6WithError(NewFrameworkProvider())`
- Consolidated the import test (`TestAccComputeV2Keypair_importBasic`) into the same file with `ImportStateVerifyIgnore: []string{"private_key", "user_id"}` (private_key not readable after create; user_id may differ)
- All other test logic (check functions, HCL configs) is unchanged

## SDKv2 import removed

The migrated resource file imports only:
- `github.com/hashicorp/terraform-plugin-framework/...` packages
- `github.com/gophercloud/gophercloud/v2/...` packages

No `github.com/hashicorp/terraform-plugin-sdk/v2` import remains.

## Caveats

- `NewFrameworkProvider()` in the test factory must be wired to the actual framework provider constructor once the full provider migration is complete. The test file uses this placeholder name — adjust to match the real constructor.
- The `testAccProvider.Meta().(*Config)` calls in the check/destroy helpers still reference the SDKv2 test provider variable. In a fully migrated provider, these would be replaced with a framework-native approach to obtain the provider's config. They are left as-is because the task scope is this resource only, not the full provider.
