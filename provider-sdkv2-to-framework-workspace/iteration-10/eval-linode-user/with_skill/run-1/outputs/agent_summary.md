# Migration Summary: linode_user resource

## Scope

Migrated `linode/user/resource.go` and `linode/user/schema_resource.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The data source (`framework_datasource.go`) and models (`framework_models.go`) were already on the framework and were not modified. The source files in the Linode clone were not touched.

## Key decisions

### MaxItems:1 block (`global_grants`)

Decision: **`SingleNestedBlock`** (keeps block HCL syntax `global_grants { ... }`).

The existing test file accesses `global_grants.0.add_domains` and the templates use block syntax. Switching to `SingleNestedAttribute` would change the HCL from `global_grants { ... }` to `global_grants = { ... }`, which is a breaking change for existing practitioners. `SingleNestedBlock` preserves the block syntax while removing the need for the list-with-SizeAtMost(1) pattern.

Note: framework state paths for `SingleNestedBlock` attributes drop the `.0.` index — tests were updated from `global_grants.0.add_domains` to `global_grants.add_domains`.

### Default: false fields

All `Default: false` SDKv2 fields translated via `booldefault.StaticBool(false)` on the schema attribute directly — NOT inside `PlanModifiers` (which would be a type error).

### Entity grant sets (`domain_grant`, `firewall_grant`, etc.)

Translated from `TypeSet` of `*schema.Resource` to `schema.SetNestedAttribute` with `Optional: true, Computed: true`. Each nested object has only `id` (Int64) and `permissions` (String) — matching the SDKv2 resource schema (not the datasource schema, which has an extra `label` field).

### ID attribute

The user resource uses `username` as the Terraform ID (matching the SDKv2 `d.SetId(user.Username)`). The framework resource stores this in an explicit `id` attribute with `stringplanmodifier.UseStateForUnknown()`.

### `email` ForceNew

Translated to `stringplanmodifier.RequiresReplace()`.

### CRUD

- `Create`: reads from `req.Plan`, calls `CreateUser`, optionally `UpdateUserGrants` if grants are configured, then reads back via `resourceReadIntoModel`.
- `Read`: reads from `req.State` (not `req.Plan`), calls `GetUser` + conditionally `GetUserGrants`.
- `Update`: reads state ID for the API call (handles username renames), calls `UpdateUser`, conditionally `UpdateUserGrants`, then reads back.
- `Delete`: reads from `req.State` (not `req.Plan` — avoids panic on null plan).

### Import

Handled by `helper.BaseResource.ImportState` (passthrough by string ID), consistent with the original `schema.ImportStatePassthroughContext`.

## Files produced

- `migrated/resource.go` — full framework resource; zero SDKv2 imports; all `Default:` via `booldefault`; `global_grants` as `SingleNestedBlock`
- `migrated/resource_test.go` — updated test paths for `global_grants` (removed `.0.` index); `ProtoV6ProviderFactories` retained (was already present in original tests)
