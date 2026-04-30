# Migration notes — openstack_compute_keypair_v2

## Pre-flight decisions

**Block decision**: No `MaxItems:1` nested `Elem` structures in this resource. All attributes are scalar or map primitives. No blocks needed.

**State upgrade**: `SchemaVersion` is zero (not set), no `StateUpgraders`. Nothing to migrate.

**Import shape**: `schema.ImportStatePassthroughContext` — simple passthrough. Migrated to `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

---

## Schema changes

| SDKv2 field | Framework equivalent |
|---|---|
| `schema.TypeString` Required/Optional/Computed | `schema.StringAttribute{Required/Optional/Computed: true}` |
| `schema.TypeMap` Optional | `schema.MapAttribute{Optional: true, ElementType: types.StringType}` |
| `ForceNew: true` | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` |
| `Computed: true` (stable after create) | Added `stringplanmodifier.UseStateForUnknown()` |
| `Sensitive: true` | `Sensitive: true` (unchanged) |
| Implicit `id` field | Explicit `id` schema attribute (`Computed`, `UseStateForUnknown`) |

## CRUD changes

- `resourceComputeKeypairV2Create/Read/Delete(ctx, *schema.ResourceData, any)` → methods on `*computeKeypairV2Resource` with typed `req`/`resp`.
- State access via typed `computeKeypairV2Model` struct with `tfsdk:""` tags; `req.Plan.Get`/`resp.State.Set` replace all `d.Get`/`d.Set` calls.
- `d.SetId(kp.Name)` → `state.ID = types.StringValue(kp.Name)`.
- `diag.Errorf` / `diag.FromErr` → `resp.Diagnostics.AddError`.
- `CheckDeleted(d, err, ...)` (sets `d.SetId("")`) → `resp.State.RemoveResource(ctx)` on HTTP 404.
- `GetRegion(d, config)` (SDKv2 helper) → inline: use `plan.Region.ValueString()` falling back to `r.config.Region`.
- `MapValueSpecs(d)` (SDKv2 helper) → inline iteration over `plan.ValueSpecs.Elements()`.
- `Update` method added as a no-op (required by the interface; all attributes have `RequiresReplace`).

## Test changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` (both test cases).
- `testAccProvider.Meta().(*Config)` → `testAccFrameworkProvider.Meta().(*Config)` in helper funcs `testAccCheckComputeV2KeypairDestroy` and `testAccCheckComputeV2KeypairExists`.
  - **Note**: `testAccFrameworkProvider` and `testAccProtoV6ProviderFactories` must be wired up in `provider_test.go` when this resource is registered in the framework provider. The exact variable names should match whatever the provider test setup uses.

## Registration (not in scope but noted)

The function `NewComputeKeypairV2Resource()` must be added to the provider's `Resources()` slice, and the old `resourceComputeKeypairV2()` SDKv2 registration must be removed, as part of the broader provider migration.

## SDKv2 imports eliminated

- `github.com/hashicorp/terraform-plugin-sdk/v2/diag`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema`
