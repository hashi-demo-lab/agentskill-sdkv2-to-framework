# Migration Summary: openstack_vpnaas_ike_policy_v2

## What was migrated

`resource_openstack_vpnaas_ike_policy_v2.go` and its test file were migrated from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Key decisions

### Default values — the core pitfall

The SDKv2 resource had five attributes with `Default:` fields:
- `auth_algorithm` → `"sha1"`
- `encryption_algorithm` → `"aes-128"`
- `pfs` → `"group5"`
- `phase1_negotiation_mode` → `"main"`
- `ike_version` → `"v1"`

Per SKILL.md, these were translated using the `*default` package (`stringdefault.StaticString(...)`), NOT placed inside `PlanModifiers`. Placing a `defaults.String` value inside `[]planmodifier.String` would cause a compile-time type error and is the common pitfall explicitly called out in the skill.

Each attribute with a Default also needed `Computed: true` (required by the framework when `Default` is set).

### Lifetime block

The `lifetime` TypeSet with nested schema became a `schema.SetNestedBlock` (not a nested attribute), preserving the block syntax `lifetime { ... }` used in production configs.

### Retry / wait logic

The SDKv2 `helper/retry` package was replaced with a simple polling loop to eliminate any `terraform-plugin-sdk/v2` imports from the migrated file. The `waitForIKEPolicyDeletion/Creation/Update` helper functions (which returned `retry.StateRefreshFunc`) were replaced by `ikePolicyV2WaitForActive` using a deadline loop.

### Test file

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` in all four test cases and the import test (per the SKILL.md pitfall: always flip even if the symbol doesn't yet exist provider-wide, as the compile failure is the intended TDD-red signal).
- All SDKv2 imports removed.

## Files produced

- `migrated/resource_openstack_vpnaas_ike_policy_v2.go` — framework resource, no SDKv2 import, all Defaults via stringdefault package
- `migrated/resource_openstack_vpnaas_ike_policy_v2_test.go` — uses ProtoV6ProviderFactories, no ProviderFactories field, no SDKv2 import
