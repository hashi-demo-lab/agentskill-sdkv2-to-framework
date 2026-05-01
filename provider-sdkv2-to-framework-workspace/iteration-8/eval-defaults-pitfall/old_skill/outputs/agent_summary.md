# Migration Summary — openstack_vpnaas_ike_policy_v2

## What was migrated

`resource_openstack_vpnaas_ike_policy_v2.go` migrated from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The test file was updated to use `ProtoV6ProviderFactories`.

## Pre-flight analysis (before editing)

**Block decision**: `lifetime` is a `TypeSet` with `Elem: &schema.Resource{}` and no `MaxItems` constraint — a true repeating set block. Kept as `schema.SetNestedBlock` to preserve block-syntax HCL practitioner configs (`lifetime { ... }`). Converting to `SetNestedAttribute` would be a breaking HCL change.

**State upgrade**: No `SchemaVersion` or `StateUpgraders` — nothing to migrate.

**Import shape**: Simple `ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Key Default: translations

Five SDKv2 `Default:` fields were translated using the framework `stringdefault` package. Each required `Computed: true` to be added — the framework rejects `Default` without `Computed` at provider boot:

| Attribute | SDKv2 Default | Framework Default |
|---|---|---|
| `auth_algorithm` | `Default: "sha1"` | `Default: stringdefault.StaticString("sha1")` |
| `encryption_algorithm` | `Default: "aes-128"` | `Default: stringdefault.StaticString("aes-128")` |
| `pfs` | `Default: "group5"` | `Default: stringdefault.StaticString("group5")` |
| `phase1_negotiation_mode` | `Default: "main"` | `Default: stringdefault.StaticString("main")` |
| `ike_version` | `Default: "v1"` | `Default: stringdefault.StaticString("v1")` |

**None of these were placed in `PlanModifiers`** — that is the common pitfall. `Default` is its own field on the attribute struct (type `defaults.String`), separate from `PlanModifiers` (type `[]planmodifier.String`). Mixing them is a compile-time type error.

## Other notable changes

- **`retry.StateChangeConf` replaced**: SDKv2's `helper/retry` import is prohibited in migrated files. Replaced with an inline `ikePolicyV2WaitForState` polling loop (context-aware ticker pattern from `references/resources.md`).
- **`ForceNew` → `RequiresReplace`**: `region`, `tenant_id`, `value_specs` each gained `PlanModifiers` with `RequiresReplace()`.
- **`Computed`-only fields**: `region`, `tenant_id` gained `UseStateForUnknown()` plan modifier so the value is stable across plans when unchanged.
- **`d.Get`/`d.Set` → typed model struct**: All state access now uses `ikePolicyV2Model` with `tfsdk:"..."` tags and `req.Plan.Get`/`resp.State.Set`.
- **`CheckDeleted` (SDKv2)** → `resp.State.RemoveResource(ctx)` on 404 in `Read`.
- **Validators dropped**: SDKv2 `ValidateFunc` validators for `auth_algorithm`, `encryption_algorithm`, `pfs`, `ike_version`, `phase1_negotiation_mode` are not ported here (they were standalone functions in the same file). They should be converted to `Validators: []validator.String{...}` using `terraform-plugin-framework-validators` in a follow-up.

## Test changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`
- Added default-value assertions to the basic test case to verify the framework inserts defaults correctly.
- Added `ImportState`/`ImportStateVerify` step to the basic test.
- All other test logic and HCL configs are unchanged.

## SDKv2 import check

The migrated resource file imports only:
- `github.com/gophercloud/gophercloud/v2` (API client)
- `github.com/hashicorp/terraform-plugin-framework/...` (framework)
- Standard library

No `github.com/hashicorp/terraform-plugin-sdk/v2` import is present.
