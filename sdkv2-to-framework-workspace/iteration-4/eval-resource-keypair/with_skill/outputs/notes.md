# Migration Notes — openstack_compute_keypair_v2

## Pre-migration analysis

### Block decision
No `MaxItems:1` nested blocks. `value_specs` is a `TypeMap` → `schema.MapAttribute{ElementType: types.StringType}`.

### State upgrade
No `SchemaVersion`. No `StateUpgraders`. Nothing to do.

### Import shape
Simple passthrough: `schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

---

## Key migration decisions

| SDKv2 pattern | Framework translation |
|---|---|
| `*schema.Resource` function | `computeKeypairV2Resource` struct implementing `resource.Resource` |
| `ForceNew: true` on all mutable attrs | `stringplanmodifier.RequiresReplace()` (or `mapplanmodifier.RequiresReplace()` for `value_specs`) |
| `Computed: true` on stable attrs | `UseStateForUnknown()` added to avoid `(known after apply)` noise on every plan |
| `Sensitive: true` on `private_key` | `Sensitive: true` unchanged — kept in state (not upgraded to `WriteOnly` since it's server-generated and practitioners may reference it downstream) |
| `d.Get("user_id").(string)` | `plan.UserID.ValueString()` via typed model struct |
| `d.SetId(kp.Name)` | `plan.ID = types.StringValue(kp.Name)` |
| `d.SetId("")` (in CheckDeleted) | `resp.State.RemoveResource(ctx)` |
| `GetRegion(d, config)` | Inlined: `plan.Region.ValueString()` with fallback to `r.config.Region` |
| `MapValueSpecs(d)` | Inlined: iterate `plan.ValueSpecs.Elements()` casting each to `types.String` |
| `CheckDeleted(d, err, msg)` | Inlined: `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` guard before `RemoveResource` |
| SDKv2 read-at-end-of-create pattern | Eliminated — all fields set directly from `keypairs.Create` response in `Create` method |
| `Importer: &schema.ResourceImporter{...}` | `ResourceWithImportState` interface + `ImportStatePassthroughID` |

### `private_key` import caveat
The Nova API only returns `private_key` in the Create response, never in subsequent Get calls. `UseStateForUnknown()` preserves the value across plans. Import cannot recover the private key, so the test file adds `ImportStateVerifyIgnore: []string{"private_key"}` to prevent import-verify failure.

### `Update` method
All attributes carry `RequiresReplace`, so the framework will never call `Update` — it will destroy and recreate instead. An empty `Update` body satisfies the `resource.Resource` interface requirement.

---

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: protoV6ProviderFactories`
- `protoV6ProviderFactories` references `NewFrameworkProvider()` — this constructor must be provided when the provider itself is migrated to the framework. Until then, the test file will not compile standalone.
- Added an `ImportState`/`ImportStateVerify` step to `TestAccComputeV2Keypair_basic` with `ImportStateVerifyIgnore: []string{"private_key"}`.
- `testAccCheckComputeV2KeypairDestroy` and `testAccCheckComputeV2KeypairExists` helpers still reference `testAccProvider.Meta().(*Config)` — these use gophercloud directly (not the Terraform protocol layer) so they remain valid.
- `CheckDestroy` and helper functions are unchanged in logic.

---

## Compile / test results

**Verification script** (`verify_tests.sh`) run against the openstack provider repo (which still uses SDKv2 throughout):

```
=== 1/6 go build ./...         PASS
=== 2/6 go vet ./...           PASS
=== 3/6 TestProvider           SKIPPED (no TestProvider in repo)
=== 4/6 Non-TestAcc unit tests PASS (1.412s)
=== 5/6 Negative gate          PASS — migrated file contains no terraform-plugin-sdk/v2 imports
=== 6/6 Acceptance tests       SKIPPED (--with-acc not set, TF_ACC unset)
ALL GATES PASSED
```

**Full compile of migrated file**: The migrated resource file cannot be compiled in isolation because:
1. `terraform-plugin-framework` is not yet in the provider's `go.mod` (only `terraform-plugin-go v0.29.0` is present as an indirect dep).
2. The file references `Config`, `ComputeKeyPairV2CreateOpts`, `computeKeyPairV2UserIDMicroversion`, and `BuildRequest` — all defined in the existing openstack package.

To make the file fully compile, the provider migration must also:
- `go get github.com/hashicorp/terraform-plugin-framework@v1.14.0` (or latest)
- Add `NewFrameworkProvider()` to the provider setup (step 4 of the migration workflow)
- Register `NewComputeKeypairV2Resource()` in the provider's `Resources()` method

**Acceptance tests**: Not run — no `TF_ACC` credentials available. The test file requires `NewFrameworkProvider()` and a fully migrated provider to execute.

---

## What is NOT migrated (out of scope)

Per task constraints, only `resource_openstack_compute_keypair_v2.go` was migrated. Not touched:
- `data_source_openstack_compute_keypair_v2.go`
- `compute_keypair_v2.go` (shared helpers — unchanged, no SDKv2 imports)
- Provider registration (`provider.go`)
- `go.mod`/`go.sum`
