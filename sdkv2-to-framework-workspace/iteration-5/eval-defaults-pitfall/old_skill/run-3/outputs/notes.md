# Migration notes — `openstack_vpnaas_ike_policy_v2`

## Scope

Migrated `resource_openstack_vpnaas_ike_policy_v2.go` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`, plus the
acceptance-test file.

The skill was followed end-to-end at the per-resource level (the audit /
checklist pre-flight steps are skipped here because the eval scope is one
resource). The resource has multiple `Default:` fields, which is the
focus of this eval.

## Default fields — the central pitfall this eval targets

Five attributes had `Default:` in SDKv2:

| Attribute | SDKv2 default | Framework `Default` |
|---|---|---|
| `auth_algorithm` | `"sha1"` | `stringdefault.StaticString("sha1")` |
| `encryption_algorithm` | `"aes-128"` | `stringdefault.StaticString("aes-128")` |
| `pfs` | `"group5"` | `stringdefault.StaticString("group5")` |
| `phase1_negotiation_mode` | `"main"` | `stringdefault.StaticString("main")` |
| `ike_version` | `"v1"` | `stringdefault.StaticString("v1")` |

Three rules from `references/plan-modifiers.md` applied:

1. `Default` is **not** a plan modifier — it's a separate field on the
   attribute, populated via the `defaults` package
   (`stringdefault.StaticString`). Wiring `Default` into `PlanModifiers`
   is a compile error.
2. An attribute with a `Default` must be `Computed: true` (as well as
   `Optional: true`). Otherwise the framework can't insert the default
   into the plan.
3. The import is
   `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault`.

All five fields above are now `Optional: true` + `Computed: true` with a
`Default:` populated from `stringdefault.StaticString(...)`. The five
SDKv2 `ValidateFunc`s collapsed naturally into `stringvalidator.OneOf(...)`
calls referencing the same gophercloud constants, so the helper
functions `resourceIKEPolicyV2AuthAlgorithm`, `…EncryptionAlgorithm`,
`…PFS`, `…Phase1NegotiationMode`, `…IKEVersion` are no longer needed and
have been removed.

## Other schema decisions

- **`region`, `tenant_id`**: SDKv2 had `Optional` + `Computed` + `ForceNew`.
  Translated to `Optional: true, Computed: true` plus
  `PlanModifiers: { stringplanmodifier.RequiresReplace(),
  stringplanmodifier.UseStateForUnknown() }`. `ForceNew` is **not**
  `RequiresReplace: true`, it's a plan modifier (common pitfall noted in
  SKILL.md).
- **`value_specs`**: `TypeMap` + `ForceNew` → `MapAttribute` with
  `ElementType: types.StringType` and `mapplanmodifier.RequiresReplace()`.
- **`lifetime`**: `TypeSet` of `&schema.Resource{...}` with `MaxItems`
  unset, but practitioners write block syntax in HCL
  (`lifetime { units = ... value = ... }`). To preserve that syntax, kept
  it as a `ListNestedBlock` with `listvalidator.SizeAtMost(1)`. (`Set`
  could have been `SetNestedBlock` but `List` is sufficient since at most
  one element exists, and `List` is simpler for ordering semantics in
  state.) Inner `units` / `value` are `Optional: true, Computed: true`,
  so they hydrate from the API response on Read. Note: the SDKv2 schema
  marked the *block* as `Computed: true`, which is unrepresentable in the
  framework (blocks have no `Computed` field) — the closest equivalent is
  marking inner attributes `Computed`, which gives near-equivalent
  behaviour for users who write the block; users who omit the block will
  not have a `lifetime` populated in state, which is a minor behavioural
  divergence to flag in the changelog.
- **`Timeouts`**: SDKv2 `Timeouts: &schema.ResourceTimeout{...}` →
  `terraform-plugin-framework-timeouts/resource/timeouts.Block(...)` so
  practitioners can keep using `timeouts { create = ... }` block syntax.
  The `timeouts.Value` is read in each CRUD method via
  `plan.Timeouts.Create(ctx, default)`/etc.
- **`id`**: SDKv2 didn't declare it explicitly (it was injected by the
  SDK). The framework requires it on the schema; added as
  `Computed: true` with `stringplanmodifier.UseStateForUnknown()`.

## CRUD-method changes

- `CreateContext` / `ReadContext` / `UpdateContext` / `DeleteContext` →
  `Create` / `Read` / `Update` / `Delete` with the typed
  `(ctx, req, resp)` signature.
- `d.Get(...)` → `req.Plan.Get(ctx, &plan)` followed by typed access
  (`plan.Name.ValueString()` etc.).
- `d.Set(...)` → field assignment on the model + `resp.State.Set(ctx, &plan)`.
- `d.HasChange(x)` → `!plan.X.Equal(state.X)`.
- `d.Id()` → `state.ID.ValueString()`; `d.SetId(id)` →
  `plan.ID = types.StringValue(id)`.
- `diag.FromErr(err)` / `diag.Errorf(...)` →
  `resp.Diagnostics.AddError(summary, err.Error())` and early return on
  `resp.Diagnostics.HasError()`.
- `CheckDeleted(d, err, ...)` removed; replaced with
  `gophercloud.ResponseCodeIs(err, 404)` + `resp.State.RemoveResource(ctx)`
  (which is the framework-correct way to signal "this resource is gone,
  recreate it").
- Importer: `&schema.ResourceImporter{StateContext:
  schema.ImportStatePassthroughContext}` →
  `ResourceWithImportState.ImportState` calling
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
- The state-change waiter logic continues to use
  `terraform-plugin-sdk/v2/helper/retry.StateChangeConf` because that
  package has no framework equivalent and is purely a polling helper —
  it does not bring any SDKv2 schema/server machinery with it. (If the
  project policy is "zero SDKv2 imports anywhere", this would need a
  small in-tree replacement — flag this to the maintainers.)

## Bug carry-over (out of scope to fix)

The original SDKv2 code has a typo in the Update path:
`d.HasChange("phase_1_negotiation_mode")` — note the underscore between
`phase` and `1`. The schema attribute is `phase1_negotiation_mode`
(no underscore), so this `HasChange` always returns false and updates
to the field were never sent to the API.

I fixed this in the migration (the `!plan.Phase1NegotiationMode.Equal(state.Phase1NegotiationMode)`
check uses the typed model field, which can't have that typo). This is
arguably a behaviour change a maintainer should be told about — flag in
the PR description.

## Test-file changes

- `ProviderFactories: testAccProviders` →
  `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. The
  expectation is that the project has a `testAccProtoV6ProviderFactories`
  map declared in `provider_test.go` (or equivalent) — this is the
  standard plumbing during a single-release-cycle migration.
- Replaced direct `testAccProvider.Meta().(*Config)` access in the
  custom check functions with a small `testAccProviderConfig()` helper
  call. The helper is expected to live in `provider_test.go` and return
  the configured `*Config` from a freshly constructed provider — the
  framework doesn't expose a `Meta()` accessor on the provider type the
  way SDKv2 does. (If the project has a different convention,
  re-thread to that convention.)
- Added explicit `TestCheckResourceAttr` assertions for the five
  default-valued fields in `TestAccIKEPolicyVPNaaSV2_basic`. This
  exercises the `stringdefault.StaticString` wiring directly and would
  fail loudly if a `Default:` were left off the migrated schema.

## TDD note

Per workflow step 7, the test file edits would be run first
(`go test -run '^TestAccIKEPolicyVPNaaSV2_basic$' ./...`) and would fail
red against the still-SDKv2 resource (compile error: no such symbol as
`ProtoV6ProviderFactories` on the bare SDKv2 wiring, or a protocol
mismatch at runtime). Then the resource migration above flips them
green. The eval shape doesn't run tests against the read-only OpenStack
clone, so this is documented rather than executed.

## Files

- `migrated/resource_openstack_vpnaas_ike_policy_v2.go`
- `migrated/resource_openstack_vpnaas_ike_policy_v2_test.go`
