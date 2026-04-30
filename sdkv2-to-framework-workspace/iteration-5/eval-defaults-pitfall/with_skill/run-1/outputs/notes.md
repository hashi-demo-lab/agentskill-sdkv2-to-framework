# Migration notes — `openstack_vpnaas_ike_policy_v2`

Migrated `resource_openstack_vpnaas_ike_policy_v2.go` from `terraform-plugin-sdk/v2`
to `terraform-plugin-framework`, plus the corresponding test file.

## Per-attribute defaults

The framework treats `Default` as a separate field on the attribute struct using
the per-type `*default` packages from
`github.com/hashicorp/terraform-plugin-framework/resource/schema/...default`.
**It is NOT a plan modifier.** Every defaulted attribute is also `Computed: true`
(required so the framework can write the default into the plan).

| Attribute                 | SDKv2 `Default` | Framework default                       | `*default` package |
|---------------------------|-----------------|------------------------------------------|--------------------|
| `auth_algorithm`          | `"sha1"`        | `stringdefault.StaticString("sha1")`     | `resource/schema/stringdefault` |
| `encryption_algorithm`    | `"aes-128"`     | `stringdefault.StaticString("aes-128")`  | `resource/schema/stringdefault` |
| `pfs`                     | `"group5"`      | `stringdefault.StaticString("group5")`   | `resource/schema/stringdefault` |
| `phase1_negotiation_mode` | `"main"`        | `stringdefault.StaticString("main")`     | `resource/schema/stringdefault` |
| `ike_version`             | `"v1"`          | `stringdefault.StaticString("v1")`       | `resource/schema/stringdefault` |

All five are string attributes, so only `stringdefault` is needed. Each
attribute is declared `Optional: true, Computed: true` with the `Default` field
populated alongside `Validators` (see below).

```go
"auth_algorithm": schema.StringAttribute{
    Optional: true,
    Computed: true,
    Default:  stringdefault.StaticString("sha1"),
    Validators: []validator.String{
        stringvalidator.OneOf( /* ... */ ),
    },
},
```

If any of these were integers I would have reached for `int64default.StaticInt64(...)`,
booleans `booldefault.StaticBool(...)`, etc. — the same pattern but per-type
package.

## Other migration choices (FYI, not part of the brief)

- `lifetime` (SDKv2 `TypeSet` of `&schema.Resource`) was kept as a
  `SetNestedBlock` to preserve practitioner HCL syntax (`lifetime { ... }`).
  Switching to `SetNestedAttribute` would break user configs (`lifetime = [{...}]`).
- `ForceNew` on `region` and `tenant_id` → `stringplanmodifier.RequiresReplace()`.
- SDKv2 `Timeouts: &schema.ResourceTimeout{...}` → `timeouts.Block(ctx, timeouts.Opts{...})`
  from `terraform-plugin-framework-timeouts`, preserving the block syntax practitioners had.
- SDKv2 `ValidateFunc` (one-of style switches) → `stringvalidator.OneOf(...)` from
  `terraform-plugin-framework-validators/stringvalidator`.
- `d.Get` / `d.Set` removed in favour of a typed `ikePolicyV2Model` struct with
  `tfsdk:` tags; reads use `req.Plan.Get` / `req.State.Get`, writes use
  `resp.State.Set`.
- `ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
- The `retry.StateChangeConf` polling loops are reimplemented as plain
  context-aware loops (`waitForIKEPolicyActive` / `waitForIKEPolicyDeletedFramework`).

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- Added explicit `TestCheckResourceAttr` assertions for the five defaulted
  attributes in `TestAccIKEPolicyVPNaaSV2_basic` (`sha1`, `aes-128`, `group5`,
  `main`, `v1`) — verifying the defaults flow into state correctly.
- The `testAccProvider.Meta()` accessor doesn't exist in framework tests; the
  `testAccCheckIKEPolicyV2{Destroy,Exists}` helpers now call a `testAccConfig()`
  helper (assumed present in the framework-converted provider scaffolding) to
  obtain a `*Config`. If the surrounding harness uses a different accessor
  (e.g., `testAccProvider.(provider.Provider)`-cast to extract config), swap
  this single helper accordingly — none of the per-resource logic depends on
  the choice.
- Test `Config:` strings are unchanged — `lifetime { ... }` stays as a block
  because we used `SetNestedBlock`, not `SetNestedAttribute`.
