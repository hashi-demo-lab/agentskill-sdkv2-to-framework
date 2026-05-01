# Agent Summary — openstack_vpnaas_ike_policy_v2 migration

## Files produced

- `migrated/resource_openstack_vpnaas_ike_policy_v2.go`
- `migrated/resource_openstack_vpnaas_ike_policy_v2_test.go`

## Notes on the migration

### `Default:` translations (the focus of this eval)

All five `Default:` SDKv2 fields use the framework's per-type `*default` package — none are placed inside `PlanModifiers`:

| Attribute | Framework `Default` |
|---|---|
| `auth_algorithm` | `stringdefault.StaticString("sha1")` |
| `encryption_algorithm` | `stringdefault.StaticString("aes-128")` |
| `pfs` | `stringdefault.StaticString("group5")` |
| `phase1_negotiation_mode` | `stringdefault.StaticString("main")` |
| `ike_version` | `stringdefault.StaticString("v1")` |

Each attribute that has a `Default` is also `Optional: true, Computed: true` as required by the framework.

### Other notable conversions

- `ValidateFunc` (5 of them) → `Validators: []validator.String{stringvalidator.OneOf(...)}` using the SDK constants stringified.
- `ForceNew: true` on `region`, `tenant_id`, `value_specs` → `stringplanmodifier.RequiresReplace()` / `mapplanmodifier.RequiresReplace()` (NOT `RequiresReplace: true`).
- `Optional+Computed` on `region`, `tenant_id` → added `UseStateForUnknown` plan modifier.
- `Computed` `id` → `UseStateForUnknown`.
- SDKv2 `Timeouts:` → `timeouts.Block(ctx, timeouts.Opts{Create:true, Update:true, Delete:true})` to preserve block syntax in HCL; default 10m wired in each CRUD method via `plan.Timeouts.Create(ctx, 10*time.Minute)`.
- `lifetime` (`TypeSet` of `&schema.Resource{}`, no `MaxItems`, with `Computed+Optional` parent + `Computed+Optional` children) → `SetNestedBlock` to preserve `lifetime { ... }` HCL; child fields kept `Optional+Computed`.
- SDKv2 `retry.StateChangeConf.WaitForStateContext` → in-file `waitForIKEPolicyState` ticker poll (no more `helper/retry` import — closes negative gate).
- `Importer: schema.ImportStatePassthroughContext` → `ImportState` method using `resource.ImportStatePassthroughID(ctx, path.Root("id"), ...)`.
- `d.Get(...)` / `d.Set(...)` swept to typed model `ikePolicyV2Model` with `tfsdk:` tags.
- Read drift handling: `d.SetId("")` replaced with `resp.State.RemoveResource(ctx)` on 404.
- Configure: guards `req.ProviderData == nil` per the skill's pitfall.
- Update: each `d.HasChange("foo")` becomes `!plan.Foo.Equal(state.Foo)` against the typed plan + state models.

### Test file — TDD RED gate (skill pitfall)

Per the skill's pitfall #14: the `ProviderFactories: testAccProviders` field has been flipped to `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` even though the symbol does not yet exist at provider scope. This is the intended step-7 TDD RED signal — verified locally:

```
openstack/resource_openstack_vpnaas_ike_policy_v2_test.go:25:29: undefined: testAccProtoV6ProviderFactories
```

The compile failure forces the migrator to wire `testAccProtoV6ProviderFactories` at provider scope before step 9 — leaving the SDKv2 `ProviderFactories:` field would silently keep the test on the SDKv2 plumbing.

### Verification

- `gofmt -l` clean on the migrated file.
- Compiled in-place against the openstack package: only the expected `undefined: resourceIKEPolicyV2` (provider.go wiring) and `undefined: testAccProtoV6ProviderFactories` (intended RED signal) remain. No other compile errors in the migrated file or test.
- The migrated file no longer imports `terraform-plugin-sdk/v2` (or `helper/retry`) — closes the negative gate.

### Out of scope (per user's "Don't migrate anything else")

- `provider.go` is not modified; the SDKv2 `resourceIKEPolicyV2` reference there continues to fail to resolve, as expected.
- `provider_test.go` is not modified; `testAccProtoV6ProviderFactories` is intentionally left undefined to surface the TDD RED.
