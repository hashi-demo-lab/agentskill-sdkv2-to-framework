# Migration Summary: openstack_vpnaas_ike_policy_v2

## Overview

Migrated `resource_openstack_vpnaas_ike_policy_v2.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Key Changes

### Schema

- Replaced `*schema.Resource` / `schema.Schema` (SDK) with `resource.Schema` (framework).
- All top-level attributes converted to `schema.Attribute` types (`StringAttribute`, `MapAttribute`).
- The `lifetime` block, previously a `TypeSet` with embedded schema, is now a `schema.SetNestedBlock` with `NestedObject`.

### Default Values (the key pitfall)

The original resource had five `Default:` fields:
- `auth_algorithm` → `"sha1"`
- `encryption_algorithm` → `"aes-128"`
- `pfs` → `"group5"`
- `phase1_negotiation_mode` → `"main"`
- `ike_version` → `"v1"`

In terraform-plugin-framework, `Default:` on `StringAttribute` requires **both** `Optional: true` **and** `Computed: true`. Without `Computed: true`, the framework raises an error at startup because the provider would be promising a value that the config doesn't supply. Each of these five attributes was given `stringdefault.StaticString(...)` plus `Computed: true`.

### Resource Model

- Introduced `ikePolicyV2ResourceModel` struct with `tfsdk` tags for all attributes.
- Introduced `ikePolicyV2LifetimeModel` for the nested lifetime block.
- Defined `ikePolicyV2LifetimeAttrTypes` for constructing `types.Object` / `types.Set` values during reads.

### CRUD Methods

- Replaced `resourceIKEPolicyV2Create/Read/Update/Delete` functions (taking `*schema.ResourceData`) with methods on `*ikePolicyV2Resource` with framework `req`/`resp` signatures.
- Shared read logic extracted into `ikePolicyV2ReadInto` to avoid duplication between `Create` and `Read`.
- `d.Timeout(schema.TimeoutCreate)` replaced with a hardcoded `10 * time.Minute` (framework-timeouts extension could be used for a more complete implementation).
- `CheckDeleted` (SDK helper) replaced with direct `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` check.
- `GetRegion(d, config)` (SDK helper) replaced with package-local `ikePolicyV2GetRegion(region, config)`.
- `MapValueSpecs(d)` (SDK helper) replaced with `ikePolicyV2MapValueSpecs(ctx, data.ValueSpecs, ...)`.

### Import

- `schema.ImportStatePassthroughContext` replaced with `resource.ImportStatePassthroughID`.

### Validation

- SDK `ValidateFunc` callbacks (`resourceIKEPolicyV2AuthAlgorithm` etc.) are not wired into the framework schema in this migration. They remain in the original file and can be adapted to `validators.StringInSlice` from `terraform-plugin-framework-validators` in a follow-up.

## Test File Changes

- Removed `resource.TestCheckResourceAttrPtr` calls that relied on pointer fields (a pattern unavailable after removing `*schema.ResourceData`).
- Replaced those checks with `resource.TestCheckResourceAttr` checking the default values explicitly (e.g., `auth_algorithm = "sha1"`).
- Merged the import test (`TestAccVPNaaSV2IKEPolicy_importBasic`) into the single test file.
- All helper functions (`testAccCheckIKEPolicyV2Destroy`, `testAccCheckIKEPolicyV2Exists`) retained unchanged as they use gophercloud directly.
