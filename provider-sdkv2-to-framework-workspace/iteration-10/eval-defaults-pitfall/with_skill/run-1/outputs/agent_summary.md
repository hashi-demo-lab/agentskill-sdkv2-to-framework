# Migration Summary: openstack_vpnaas_ike_policy_v2

## What was migrated

`resource_openstack_vpnaas_ike_policy_v2.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Default fields — key pitfall avoided

The resource had **5 string attributes with `Default:` values** in SDKv2:

| Attribute | Default value |
|---|---|
| `auth_algorithm` | `"sha1"` |
| `encryption_algorithm` | `"aes-128"` |
| `pfs` | `"group5"` |
| `phase1_negotiation_mode` | `"main"` |
| `ike_version` | `"v1"` |

Per the skill's Common Pitfalls note: **`Default` is not a plan modifier in the framework.** Each was translated using `stringdefault.StaticString(...)` (from `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault`) placed in the `Default:` field of the `schema.StringAttribute` — NOT in `PlanModifiers`. Placing a `defaults.String` value inside `PlanModifiers []planmodifier.String` is a compile-time type error and a common mistake.

## Other translation decisions

- **`ForceNew: true`** on `region`, `tenant_id`, `value_specs` → `stringplanmodifier.RequiresReplace()` / `mapplanmodifier.RequiresReplace()` in `PlanModifiers`.
- **`lifetime` TypeSet with nested Elem** → `schema.SetNestedBlock` (block syntax preserved; practitioners already use `lifetime { ... }` block syntax in configs).
- **Validators** (`ValidateFunc` switching on `ikepolicies.*` constants) → `stringvalidator.OneOf(...)` using the same gophercloud constants cast to `string`.
- **`Timeouts`** → `terraform-plugin-framework-timeouts` package with `timeouts.Block` / `timeouts.Value`.
- **`retry.StateChangeConf`** (SDKv2) removed; replaced with a minimal polling loop using `ikepolicies.Get` and `gophercloud.ResponseCodeIs` — no SDKv2 import retained.
- **`Importer: schema.ImportStatePassthroughContext`** → `ImportState` method that reads by ID.
- **`MapValueSpecs(d)`** → inline `ikePolicyV2ValueSpecsFromPlan` helper using `types.Map.ElementsAs`.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`
- Added `ImportState` / `ImportStateVerify` step to the basic test.
- Added default-value `TestCheckResourceAttr` assertions to the basic test.
- No SDKv2 imports remain in the test file.

## SDKv2 imports removed

All `github.com/hashicorp/terraform-plugin-sdk/v2/*` imports are absent from both output files.
