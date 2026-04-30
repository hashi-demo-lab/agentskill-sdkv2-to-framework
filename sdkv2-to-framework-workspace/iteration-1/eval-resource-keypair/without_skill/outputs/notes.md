# Migration Notes: resource_openstack_compute_keypair_v2

## What was done

Migrated `resource_openstack_compute_keypair_v2.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`, and updated the corresponding test file.

### Resource migration (`resource_openstack_compute_keypair_v2.go`)

- **Removed** all imports from `github.com/hashicorp/terraform-plugin-sdk/v2`.
- **Added** imports from `github.com/hashicorp/terraform-plugin-framework/*`.
- Replaced the `*schema.Resource` factory function `resourceComputeKeypairV2()` with a framework struct `computeKeypairV2Resource` implementing `resource.Resource`, `resource.ResourceWithConfigure`, and `resource.ResourceWithImportState`.
- Added a `computeKeypairV2Model` struct with `tfsdk` tags for all attributes, including the mandatory `id` attribute (framework requires explicit `id`).
- **Schema translation**:
  - All `ForceNew: true` SDKv2 attributes → `RequiresReplace()` plan modifiers.
  - All `Computed: true` attributes → `UseStateForUnknown()` plan modifiers to preserve state across refreshes.
  - `Sensitive: true` on `private_key` preserved.
  - `value_specs` `TypeMap` → `schema.MapAttribute{ElementType: types.StringType}`.
- **CRUD methods**:
  - `Create`: reads plan, builds `ComputeKeyPairV2CreateOpts` (reusing existing helper from `compute_keypair_v2.go`), calls `keypairs.Create`, populates state including `public_key`, `fingerprint`, and `private_key` (available only on create).
  - `Read`: handles 404 via `resp.State.RemoveResource(ctx)` instead of SDKv2's `d.SetId("")`.
  - `Update`: no-op (all attributes are RequiresReplace).
  - `Delete`: handles 404 gracefully (already deleted).
  - `ImportState`: uses `resource.ImportStatePassthroughID`.
- Reused existing helpers: `ComputeKeyPairV2CreateOpts`, `computeKeyPairV2UserIDMicroversion` from `compute_keypair_v2.go`.

### Test migration (`resource_openstack_compute_keypair_v2_test.go`)

- Replaced `ProviderFactories: testAccProviders` with `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` (required for framework resources).
- Added an `ImportState` test step to `TestAccComputeV2Keypair_basic` (with `ImportStateVerifyIgnore: []string{"private_key"}` since the API doesn't return `private_key` on read).
- All acceptance test logic (`testAccCheckComputeV2KeypairDestroy`, `testAccCheckComputeV2KeypairExists`) and HCL configs are unchanged.

## Compile check

Tested using `/tmp/openstack-keypair-compile` (copy of the provider at `/Users/simon.lynch/git/terraform-provider-openstack`):

1. Replaced `openstack/resource_openstack_compute_keypair_v2.go` with the migrated version.
2. Added `github.com/hashicorp/terraform-plugin-framework@v1.14.1` via `go get`.
3. Removed the now-undefined `resourceComputeKeypairV2()` call from `provider.go` in the compile copy (only in `/tmp`, not in the source repo).
4. `go build ./...` — **passed** with no errors.

### Test compile note

The test file uses `testAccProtoV6ProviderFactories` which does not yet exist in the provider's `provider_test.go` (the provider is currently pure SDKv2, no mux server). Running `go vet ./openstack/...` with the test file reports `undefined: testAccProtoV6ProviderFactories`.

**This is expected**: in a full migration, `provider_test.go` would be updated to add `testAccProtoV6ProviderFactories` (using `terraform-plugin-mux` or `terraform-plugin-framework`'s `providerserver`). Since the task scope is limited to the keypair resource only, this infrastructure change is out of scope.

The resource implementation file itself compiles cleanly; only the test depends on the (not-yet-added) proto v6 provider factory.

## Framework version used

`github.com/hashicorp/terraform-plugin-framework v1.14.1` (latest available in the local module cache at migration time).
