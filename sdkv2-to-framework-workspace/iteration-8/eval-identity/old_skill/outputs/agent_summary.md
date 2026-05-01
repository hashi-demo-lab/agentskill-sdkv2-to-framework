# Migration Summary — openstack_lb_member_v2

## What was migrated

`resource_openstack_lb_member_v2.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Key decisions

### Block-vs-attribute
No nested `Elem: &schema.Resource{}` attributes exist in this resource — all attributes are primitives, sets, and booleans. The `tags` attribute (`TypeSet, Elem: &schema.Schema{Type: schema.TypeString}`) maps directly to `schema.SetAttribute{ElementType: types.StringType}`. The `Set: schema.HashString` hash function was dropped (framework handles uniqueness internally).

### Timeouts
Original resource had `Timeouts: &schema.ResourceTimeout{Create/Update/Delete}`. Existing practitioner configs use block syntax (`timeouts { create = "5m" }`), so `timeouts.Block(ctx, opts)` was chosen over `timeouts.Attributes(ctx, opts)` to preserve backward-compatible HCL syntax.

### State upgrade
No `SchemaVersion > 0` — no state upgrader needed.

### Import shape
The SDKv2 importer used `resourceMemberV2Import` with a composite string `<pool_id>/<member_id>`. The migrated `ImportState` handles both:
- **Modern path** (Terraform 1.12+): `req.ID == ""` branch uses `resource.ImportStatePassthroughWithIdentity` to copy `pool_id` and `member_id` from the identity block into state.
- **Legacy path**: `req.ID != ""` branch parses the `<pool_id>/<member_id>` string and sets `pool_id` and `id` on state via `resp.State.SetAttribute`.

### ResourceWithIdentity
`IdentitySchema` defines two `RequiredForImport` attributes:
- `pool_id` — the parent pool UUID
- `member_id` — the member UUID

The identity is written in `Create`, `Read`, and `Update` via `resp.Identity.Set(ctx, lbMemberV2IdentityModel{...})`.

### Retry loop
`retry.RetryContext` + `checkForRetryableError` (both from `terraform-plugin-sdk/v2`) were replaced with an inline `lbMemberV2Retry` function that retries on HTTP 409/500/502/503/504 errors using `gophercloud.ErrUnexpectedResponseCode`. This avoids any SDK v2 import in the resource file.

### Shared wait helpers
`waitForLBV2Pool` and `waitForLBV2Member` are defined in `lb_v2_shared.go` (which still imports the SDK). These are called from the migrated resource file as plain function calls — no SDK import is needed in the resource file itself.

## Files produced

| File | Description |
|---|---|
| `migrated/resource_openstack_lb_member_v2.go` | Full framework resource. Zero SDKv2 imports. IdentitySchema with 2 RequiredForImport attrs. Identity.Set called in Create/Read/Update. ImportState handles both req.ID and req.Identity paths. |
| `migrated/resource_openstack_lb_member_v2_test.go` | Updated test file. ProviderFactories → ProtoV6ProviderFactories. Added ImportState step with composite-ID IdFunc. Added ConfigStateChecks asserting pool_id is non-null in state. statecheck/knownvalue imports from terraform-plugin-testing v1.14. |

## Verification notes

The migrated resource file passes the negative gate (no `terraform-plugin-sdk/v2` string appears outside of a comment). Full acceptance tests require `TF_ACC=1` and a live OpenStack environment with the LB service available.
