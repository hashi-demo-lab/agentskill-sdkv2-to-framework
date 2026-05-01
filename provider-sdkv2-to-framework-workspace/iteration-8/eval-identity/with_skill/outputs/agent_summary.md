# Migration Summary: openstack_lb_member_v2

## Resource migrated
`resource_openstack_lb_member_v2.go` — SDKv2 → terraform-plugin-framework v1.17.0

## Key decisions

### Pre-flight C: think pass

1. **Block decision**: No `MaxItems: 1` nested blocks in this resource. The `tags` field is a `TypeSet` of strings → `schema.SetAttribute{ElementType: types.StringType}`. The `timeouts` block is rendered via `timeouts.Block(...)` to preserve practitioner-facing block syntax (`timeouts { create = "5m" }`).

2. **State upgrade**: No `SchemaVersion > 0` — no state upgraders needed.

3. **Import shape**: Custom `Importer.StateContext` parsing composite `<pool_id>/<member_id>`. Migrated to:
   - `IdentitySchema` method (ResourceWithIdentity) with `pool_id` + `member_id` both `RequiredForImport: true`
   - `ImportState` with dual dispatch: `req.ID == ""` → modern path (identity), else legacy string parse path

## What changed

### Resource file
- Removed all `terraform-plugin-sdk/v2` imports
- `resourceMemberV2()` function replaced by `lbMemberV2Resource` struct implementing:
  - `resource.Resource` (Metadata, Schema, Create, Read, Update, Delete)
  - `resource.ResourceWithConfigure` (Configure)
  - `resource.ResourceWithImportState` (ImportState)
  - `resource.ResourceWithIdentity` (IdentitySchema)
- `lbMemberV2Model` struct with `tfsdk:` tags for all attributes
- `lbMemberV2IdentityModel` struct with `pool_id` + `member_id` fields
- `ForceNew: true` → `stringplanmodifier.RequiresReplace()` / `int64planmodifier.RequiresReplace()`
- `Default: true` (admin_state_up) → `booldefault.StaticBool(true)`
- `schema.TypeSet` + `schema.HashString` → `schema.SetAttribute` (hash function dropped — framework handles uniqueness)
- `validation.IntBetween(1,65535)` → `int64validator.Between(1, 65535)` from `terraform-plugin-framework-validators`
- `Timeouts` field → `timeouts.Block(ctx, timeouts.Opts{Create,Update,Delete: true})`
- `retry.RetryContext` removed — replaced by inline retry loops (`retryCreateMember`, `retryUpdateMember`, `retryDeleteMember`) that do NOT import `helper/retry`
- `CheckDeleted`/`GetRegion` (SDKv2 helpers taking `*schema.ResourceData`) replaced by inline equivalents using `gophercloud.ResponseCodeIs` and `r.getRegion()`
- Identity set in Create, Read, and Update via `resp.Identity.Set(ctx, lbMemberV2IdentityModel{...})`

### ImportState dual-path (critical)
```go
if req.ID == "" {
    // Modern: Terraform 1.12+ identity block
    resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
    resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"),      path.Root("member_id"), req, resp)
    return
}
// Legacy: terraform import openstack_lb_member_v2.foo <pool_id>/<member_id>
parts := strings.SplitN(req.ID, "/", 2)
resp.State.SetAttribute(ctx, path.Root("pool_id"), parts[0])
resp.State.SetAttribute(ctx, path.Root("id"),      parts[1])
```

### Test file
- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: protoV6MemberProviderFactories`
- Provider factory returns an error intentionally (TDD red gate — provider not yet on framework)
- Helper functions renamed with `Framework` suffix to avoid conflict with original SDKv2 helpers
- Added `ImportStateIdFunc` test step for legacy composite-ID import verification
- Added `ConfigStateChecks` asserting `pool_id` and `id` are non-null after create (identity populated)
- Test configs unchanged (HCL is user-visible; timeouts block syntax preserved)

## Negative gate status
The migrated resource file contains zero references to `github.com/hashicorp/terraform-plugin-sdk/v2`.

## Compatibility notes
- `ResourceWithIdentity` requires `terraform-plugin-framework` ≥ v1.17.0 ✓ (provider has v1.17.0)
- `import { identity = {...} }` requires Terraform CLI ≥ 1.12
- Legacy `terraform import ... <pool>/<member>` continues to work on all Terraform versions
