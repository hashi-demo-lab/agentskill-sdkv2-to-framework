# Migration Summary: openstack_vpnaas_ike_policy_v2

## What was done

Migrated `resource_openstack_vpnaas_ike_policy_v2.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. No SDKv2 imports remain in either output file.

## Key decisions

### Default fields (the pitfall addressed)

The source had 5 attributes with `Default:` values:
- `auth_algorithm` → default `"sha1"`
- `encryption_algorithm` → default `"aes-128"`
- `pfs` → default `"group5"`
- `phase1_negotiation_mode` → default `"main"`
- `ike_version` → default `"v1"`

Each was translated using `stringdefault.StaticString(...)` in the `Default:` field of the schema attribute. None were placed in `PlanModifiers` (the common pitfall: `Default` is typed `defaults.String`, not `planmodifier.String`; the compiler would catch the mix, but the pattern is wrong regardless).

Each defaulted attribute is also marked `Computed: true` (required when `Default` is set so the framework knows the value may be server-populated or filled from the default).

### lifetime block

`lifetime` is a `TypeSet` with `Elem: &schema.Resource{...}` (no `MaxItems`). The test configs use block syntax (`lifetime { ... }`), so it was kept as `SetNestedBlock` (preserves practitioner HCL).

### ForceNew → RequiresReplace

`region`, `tenant_id`, and `value_specs` had `ForceNew: true`; translated to `stringplanmodifier.RequiresReplace()` in `PlanModifiers`.

### retry.StateChangeConf replacement

`terraform-plugin-sdk/v2/helper/retry` is gone. Replaced with an inline `waitForIKEPolicyV2State` function (context-aware ticker poll) per the resources.md pattern. The three wait helpers (`waitForIKEPolicyCreationV2`, `waitForIKEPolicyUpdateV2`, `waitForIKEPolicyDeletionV2`) return `func() (any, string, error)` directly (no named `retry.StateRefreshFunc` type).

### Import

`Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

### value_specs

Translated from `TypeMap` to `schema.MapAttribute{ElementType: types.StringType}`. Map elements extracted manually in Create via `plan.ValueSpecs.Elements()`.

### Tests

Updated `ProviderFactories` to keep `testAccProviders` (the existing provider factory in this codebase; `testAccProtoV6ProviderFactories` would be created once the provider server itself is migrated to framework). Added default-value assertions to the basic test. Merged the import test into the main test file. No SDKv2 imports.

## Files produced

- `migrated/resource_openstack_vpnaas_ike_policy_v2.go` — framework resource, zero SDKv2 imports
- `migrated/resource_openstack_vpnaas_ike_policy_v2_test.go` — updated test, zero SDKv2 imports
