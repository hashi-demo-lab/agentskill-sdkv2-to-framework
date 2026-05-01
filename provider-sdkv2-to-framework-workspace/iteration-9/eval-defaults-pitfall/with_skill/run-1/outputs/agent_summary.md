# Migration summary: openstack_vpnaas_ike_policy_v2

## What was migrated

Resource `openstack_vpnaas_ike_policy_v2` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework` v1.17.0.

## Default fields — correct translation

The source had five `Default:` fields on string attributes:

| Attribute | SDKv2 `Default:` | Framework translation |
|---|---|---|
| `auth_algorithm` | `"sha1"` | `Default: stringdefault.StaticString("sha1")` |
| `encryption_algorithm` | `"aes-128"` | `Default: stringdefault.StaticString("aes-128")` |
| `pfs` | `"group5"` | `Default: stringdefault.StaticString("group5")` |
| `phase1_negotiation_mode` | `"main"` | `Default: stringdefault.StaticString("main")` |
| `ike_version` | `"v1"` | `Default: stringdefault.StaticString("v1")` |

Each was placed in the `Default:` field on `schema.StringAttribute` (imported from `resource/schema/stringdefault`), **NOT** inside `PlanModifiers`. All five attributes are marked `Optional: true, Computed: true` as required by the framework (the `Computed` flag lets the framework insert the default into the plan).

## Key decisions

- **`lifetime` block**: kept as `schema.SetNestedBlock` (practitioners use block syntax in configs — switching to a nested attribute would be HCL-breaking). The `[]ikePolicyV2LifetimeModel` slice type in the model handles at-most-one semantics naturally.
- **`retry.StateChangeConf` removed**: replaced with an inline `waitForState` poll loop (no SDKv2 helper import in the migrated file).
- **`value_specs`**: translated to `schema.MapAttribute{ElementType: types.StringType}` with `mapplanmodifier.RequiresReplace()` (mirrors `ForceNew: true` from SDKv2).
- **`ForceNew` attributes** (`region`, `tenant_id`, `value_specs`): each gets `stringplanmodifier.RequiresReplace()` / `mapplanmodifier.RequiresReplace()`.
- **Computed-only fields with `UseStateForUnknown`**: `id`, `region`, `tenant_id` all get `UseStateForUnknown()` to suppress spurious `(known after apply)` noise.
- **`Delete` reads from `req.State`**, not `req.Plan` (plan is null on delete).
- **Import**: `resource.ImportStatePassthroughID` on `path.Root("id")`.

## SDKv2 imports removed

The migrated file has zero imports from `github.com/hashicorp/terraform-plugin-sdk/v2`.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`
- Added default-value assertions in the basic test (`auth_algorithm`, `encryption_algorithm`, `pfs`, `phase1_negotiation_mode`, `ike_version`) to explicitly verify defaults are applied via the `defaults` package.
- `testAccProtoV6ProviderFactories` must be wired in `provider_test.go` as part of the broader provider migration (not in scope for this single-resource output).

## Files produced

- `migrated/resource_openstack_vpnaas_ike_policy_v2.go` — framework resource, no SDKv2 import
- `migrated/resource_openstack_vpnaas_ike_policy_v2_test.go` — updated to `ProtoV6ProviderFactories`
