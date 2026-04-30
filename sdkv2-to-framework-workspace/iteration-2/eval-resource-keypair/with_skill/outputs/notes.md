# Migration Notes — openstack_compute_keypair_v2

## Pre-flight audit (this resource only)

**Block decision**: No `MaxItems: 1 + nested Elem` patterns. `value_specs` is `TypeMap` → `schema.MapAttribute{ElementType: types.StringType}` with `mapplanmodifier.RequiresReplace()`. No blocks.

**State upgrade**: No `SchemaVersion`, no `StateUpgraders`. Nothing to flatten.

**Import shape**: Simple passthrough (`ImportStatePassthroughContext`) → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

---

## Key conversion decisions

| SDKv2 | Framework |
|---|---|
| `*schema.Resource` function | `computeKeypairV2Resource` struct implementing `resource.Resource` |
| `CreateContext` / `ReadContext` / `DeleteContext` | `Create` / `Read` / `Delete` with typed `req`/`resp` |
| `d.Get("x").(string)` | `plan.X.ValueString()` via typed model struct with `tfsdk:"..."` tags |
| `d.SetId(...)` / `d.Set(...)` | `resp.State.Set(ctx, &model)` |
| `diag.Errorf(...)` / `diag.FromErr(...)` | `resp.Diagnostics.AddError(...)` |
| `ForceNew: true` on strings | `stringplanmodifier.RequiresReplace()` |
| `Computed: true` (stable) | `stringplanmodifier.UseStateForUnknown()` |
| `Sensitive: true` | `Sensitive: true` (unchanged) |
| `TypeMap` | `schema.MapAttribute{ElementType: types.StringType}` |
| `schema.ImportStatePassthroughContext` | `resource.ImportStatePassthroughID` |
| `CheckDeleted(d, err, msg)` | Inline 404 check with `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` + `resp.State.RemoveResource(ctx)` |
| `MapValueSpecs(d)` | `plan.ValueSpecs.ElementsAs(ctx, &valueSpecs, false)` |

### Notable: no Update method
All attributes are either `Computed`-only or have `RequiresReplace`, so the resource can never be updated in-place. The `Update` method is a deliberate no-op (required by the interface but unreachable).

### Private key handling
The keypair's `private_key` is only returned by the Create API call, not by subsequent Get calls. The framework CRUD flow calls Read after Create to populate computed attributes; the private key is preserved from the Create response and set back into state after `readIntoState` runs.

### `getRegionFromModel` helper
Replaces the `GetRegion(d, config)` SDKv2 helper. Reads `types.String` region from the model and falls back to `config.Region`.

### `readIntoState` helper
Shared logic between `Create` and `Read`. Signals a 404 by clearing `model.ID` (null) rather than adding a diagnostic error, allowing `Read` to call `resp.State.RemoveResource` cleanly.

---

## Test file changes

| Change | Reason |
|---|---|
| `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` | Framework resources require the protocol-v6 provider factory |
| Import test moved from separate file into the main test file | Consolidation; avoids split across two files |
| Check helpers (`testAccCheckComputeV2KeypairDestroy`, `testAccCheckComputeV2KeypairExists`) unchanged | They use the Gophercloud API directly, not Terraform state helpers — no migration needed |

**`testAccProtoV6ProviderFactories` dependency**: This variable must be defined in `provider_test.go` (or a shared test helper) when the provider is registered via `providerserver.NewProtocol6WithError`. Since the openstack provider is currently 100% SDKv2, this wiring does not yet exist in the repo. When integrating this migration, add:

```go
var testAccProtoV6ProviderFactories map[string]func() (tfprotov6.ProviderServer, error)

func init() {
    testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
        "openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider()),
    }
}
```

---

## Compile / test results

**Environment**: Standalone Go module at `/tmp/kp_check` with:
- `github.com/gophercloud/gophercloud/v2 v2.10.0`
- `github.com/hashicorp/terraform-plugin-framework v1.14.0`

**Steps run**:
```
go mod tidy   → OK (no errors)
go build ./openstack/...   → OK (no output = clean build)
```

**Full `go test` / `TF_ACC` tests**: Not run.
- The openstack repo does not yet import `terraform-plugin-framework`, so the full provider cannot be compiled with the new resource without first adding the framework dependency and registering the resource in the provider.
- Acceptance tests require live OpenStack credentials (`TF_ACC=1`, `OS_AUTH_URL`, etc.).

**Negative gate**: The migrated `resource_openstack_compute_keypair_v2.go` contains **no** imports from `github.com/hashicorp/terraform-plugin-sdk/v2`. Confirmed by inspection.

---

## Integration steps (not done — out of scope)

1. Add `github.com/hashicorp/terraform-plugin-framework` to the provider's `go.mod` / `go.sum`.
2. Add `testAccProtoV6ProviderFactories` to `provider_test.go`.
3. Register `NewComputeKeypairV2Resource()` in the provider's `Resources()` method (framework provider).
4. Remove or replace the old `resourceComputeKeypairV2()` SDKv2 function.
5. Remove the old `resource_openstack_compute_keypair_v2.go` and the now-unused `import_openstack_compute_keypair_v2_test.go` (its test is consolidated into the main test file).
6. Run `go build ./...` and `go vet ./...` on the full repo.
7. Run `TF_ACC=1` acceptance tests against a live OpenStack environment.
