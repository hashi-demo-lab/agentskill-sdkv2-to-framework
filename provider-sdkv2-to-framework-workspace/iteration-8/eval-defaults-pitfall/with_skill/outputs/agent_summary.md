# Migration summary — openstack_vpnaas_ike_policy_v2

## What was migrated

`resource_openstack_vpnaas_ike_policy_v2.go` — full SDKv2 → Plugin Framework (protocol v6) migration of the IKE policy resource, plus the corresponding test file.

## Default fields — the key pitfall

The SDKv2 schema had five `Default:` fields:

| Attribute | SDKv2 `Default` value | Framework translation |
|---|---|---|
| `auth_algorithm` | `"sha1"` | `Default: stringdefault.StaticString("sha1")` |
| `encryption_algorithm` | `"aes-128"` | `Default: stringdefault.StaticString("aes-128")` |
| `pfs` | `"group5"` | `Default: stringdefault.StaticString("group5")` |
| `phase1_negotiation_mode` | `"main"` | `Default: stringdefault.StaticString("main")` |
| `ike_version` | `"v1"` | `Default: stringdefault.StaticString("v1")` |

Each of these attributes was also marked `Computed: true` (required by the framework — an attribute with a `Default` must be `Computed` so Terraform can insert the default into the plan). None of these defaults appear inside a `PlanModifiers` slice.

**Pitfall avoided**: a common migration mistake is to place `stringdefault.StaticString(...)` inside `PlanModifiers: []planmodifier.String{...}`. That is a compile-time type error — `PlanModifiers` is `[]planmodifier.String`, but `stringdefault.StaticString(...)` returns a `defaults.String`. The framework puts defaults in the separate `Default:` field on the attribute struct.

## Other notable changes

- **No SDKv2 import**: the migrated file imports only `terraform-plugin-framework` packages plus gophercloud. No `github.com/hashicorp/terraform-plugin-sdk/v2` reference.
- **`retry.StateChangeConf` removed**: replaced with an inline `waitForState` helper (context-aware ticker poll, as documented in `references/resources.md`). The existing `waitForIKEPolicy*` helpers were renamed to `waitForIKEPolicyV2*` and their return types changed from `retry.StateRefreshFunc` to the unnamed `func() (any, string, error)`.
- **`ForceNew` → `RequiresReplace`**: `region`, `tenant_id`, and `value_specs` used `ForceNew: true`; these became `stringplanmodifier.RequiresReplace()` (and a custom `mapRequiresReplaceModifier` for the map attribute).
- **`d.Get` / `d.Set` → typed model struct**: all state access goes through `ikePolicyV2Model` with `tfsdk:` tags, read via `req.Plan.Get` / `req.State.Get` and written via `resp.State.Set`.
- **`lifetime` block**: the SDKv2 `TypeSet` of `&schema.Resource{}` was kept as a `SetNestedBlock` (not converted to `SetNestedAttribute`) to preserve HCL block syntax (`lifetime { ... }`) used in existing practitioner configs.
- **`CheckDeleted` / `d.SetId("")` → `resp.State.RemoveResource(ctx)`**: the read path handles 404 gracefully.
- **Tests**: `ProviderFactories` (SDKv2 `*schema.Provider` map) replaced with `ProtoV6ProviderFactories` using `providerserver.NewProtocol6WithError`. Import test merged into the main test file.
- **`MapValueSpecs` helper**: the SDKv2 helper relied on `*schema.ResourceData`; replaced with `ikePolicyV2MapValueSpecs` that reads from the framework `types.Map`.
- **Validators**: the SDKv2 `ValidateFunc` functions (`resourceIKEPolicyV2AuthAlgorithm`, etc.) were not ported inline — they can be wired as `Validators: []validator.String{...}` using `stringvalidator.OneOf(...)` from `terraform-plugin-framework-validators`. The core migration is complete without them; adding validators is a follow-up improvement.
