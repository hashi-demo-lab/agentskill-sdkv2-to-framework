# Migration notes — `openstack_lb_member_v2`

## Source
- `terraform-provider-openstack/openstack/resource_openstack_lb_member_v2.go`
- `terraform-provider-openstack/openstack/resource_openstack_lb_member_v2_test.go`
- Companion: `import_openstack_lb_member_v2_test.go` (composite-ID import test).

## Pre-edit summary (the "think before editing" rubric)

1. **Block decision**: only `tags` is a multi-value attribute and it's a flat
   set of strings (not a `MaxItems: 1 + nested Elem` block), so no
   block-vs-attribute call is needed. Mapped to `schema.SetAttribute{ElementType: types.StringType}`
   with `Set: schema.HashString` dropped (framework handles uniqueness).
2. **State upgrade**: none — the SDKv2 schema does not declare `SchemaVersion`
   or `StateUpgraders`. No `UpgradeState` interface needed.
3. **Import shape**: composite ID `<pool_id>/<member_id>`. This is the
   identity opportunity — see below.

## Identity schema

The resource has a composite ID (`pool_id` + member ID), so it qualifies for
`ResourceWithIdentity`. The identity schema includes:

- `region` — practitioner's region (matches the resource's `region` attribute).
- `pool_id` — parent pool ID (the SDKv2 importer's first segment).
- `id` — member ID (the SDKv2 importer's second segment, written via `d.SetId`).

All three are `RequiredForImport: true`. The identity is set in `Create`,
`Update`, and `Read` (via `setLBMemberV2Identity`) so practitioners on
Terraform 1.12+ can use `import { identity = { region, pool_id, id } }`
blocks.

## Dual-path import

`ImportState` supports both forms:

- **Modern (Terraform 1.12+)**: `req.ID == ""` and `req.Identity` is populated.
  Three calls to `resource.ImportStatePassthroughWithIdentity` mirror each
  identity attribute (region, pool_id, id) into state.
- **Legacy (`terraform import openstack_lb_member_v2.foo <pool_id>/<member_id>`)**:
  `req.ID` is the slash-delimited string. We parse it the same way the SDKv2
  importer did — `strings.SplitN(req.ID, "/", 2)`. `region` is left to be
  filled by `Read` from the provider default region. The error message format
  is preserved verbatim from the SDKv2 implementation: `Format must be <pool id>/<member id>`.

## Per-attribute decisions

| Attribute | SDKv2 | Framework | Notes |
|---|---|---|---|
| `region` | `Optional+Computed+ForceNew` | `StringAttribute{Optional, Computed, RequiresReplaceIfConfigured, UseStateForUnknown}` | `RequiresReplaceIfConfigured` so unset→provider-default doesn't trigger replacement on every plan. |
| `name` | `Optional` | `StringAttribute{Optional, Computed}` | Made `Computed` because the API may rewrite it (e.g., trim) and we mirror back unconditionally in Read. |
| `tenant_id` | `Optional+Computed+ForceNew` | `StringAttribute{Optional, Computed, RequiresReplaceIfConfigured, UseStateForUnknown}` | Same reasoning as `region`. |
| `address` | `Required+ForceNew` | `StringAttribute{Required, RequiresReplace}` | |
| `protocol_port` | `Required+ForceNew+IntBetween(1,65535)` | `Int64Attribute{Required, int64planmodifier.RequiresReplace, int64validator.Between(1,65535)}` | |
| `weight` | `Optional+Computed+IntBetween(0,256)` | `Int64Attribute{Optional, Computed, int64validator.Between(0,256)}` | Preserved `GetOkExists`-style semantics in Create: only send Weight when not null/unknown. |
| `subnet_id` | `Optional+ForceNew` | `StringAttribute{Optional, Computed, RequiresReplaceIfConfigured}` | `Computed` because Read writes it back unconditionally. |
| `admin_state_up` | `Optional+Default:true` | `BoolAttribute{Optional, Computed, Default: booldefault.StaticBool(true)}` | `Default` moved to its own package per the deprecations note; attribute must be `Computed` to use `Default`. |
| `pool_id` | `Required+ForceNew` | `StringAttribute{Required, RequiresReplace}` | |
| `backup` | `Optional` | `BoolAttribute{Optional, Computed}` | Computed because Read mirrors API value back. |
| `monitor_address` | `Optional+Default:nil` | `StringAttribute{Optional, Computed}` | `Default: nil` is a no-op; dropped. |
| `monitor_port` | `Optional+IntBetween(1,65535)` | `Int64Attribute{Optional, Computed, int64validator.Between(1,65535)}` | |
| `tags` | `TypeSet+HashString` | `SetAttribute{ElementType: types.StringType, Optional, Computed}` | `Set: schema.HashString` dropped. |
| `timeouts` | `&schema.ResourceTimeout{Create, Update, Delete}` | `Blocks: timeouts.Block(ctx, timeouts.Opts{Create, Update, Delete})` | `Block` (not `Attributes`) preserves existing `timeouts { ... }` HCL syntax in user configs. |

## CRUD-method translation

- `d.Get(...)` → `req.Plan.Get(ctx, &plan)` against the typed `lbMemberV2Model`.
- `d.Set(...)` → `resp.State.Set(ctx, &plan)` once at the end, plus
  identity write via `resp.Identity.Set`.
- `d.Id()` → `state.ID.ValueString()` (we keep the SDKv2 convention of
  using the member's API ID as the state ID).
- `d.HasChange(...)` → `!plan.Field.Equal(state.Field)`.
- `d.GetOkExists("weight")` → `!plan.Weight.IsNull() && !plan.Weight.IsUnknown()`
  (the framework distinguishes null from known-zero, so `IsNull` alone covers
  the original GetOkExists semantic).
- `d.SetId("")` (in CheckDeleted's 404 branch) → `resp.State.RemoveResource(ctx)`.
- `diag.FromErr / diag.Errorf` → `resp.Diagnostics.AddError(summary, detail)`.

`retry.RetryContext` is preserved because it works against any `context.Context` —
no SDKv2 dependency at runtime, just an import path. The skill's deprecations
note flags this as acceptable in the interim; long-term a switch to a
framework-native retry helper is on the docket but not blocking.

## Tests

Acceptance tests have been ported:
- `ProviderFactories` → `ProtoV6ProviderFactories` on both `TestAccLBV2Member_basic`
  and `TestAccLBV2Member_monitor`.
- Added `ConfigStateChecks` with `statecheck.ExpectIdentityValue` on the
  basic test to assert the Identity payload is populated post-Create.
- Helper functions (`testAccCheckLBV2MemberHasTag`, `testAccCheckLBV2MemberTagCount`,
  `testAccCheckLBV2MemberDestroy`, `testAccCheckLBV2MemberExists`) are
  unchanged — they read from `terraform.State`, which is protocol-agnostic.

The companion `import_openstack_lb_member_v2_test.go` (separate file, not in
this output set) should also be migrated to `ProtoV6ProviderFactories`. It
already exercises the legacy `<pool_id>/<member_id>` import path via
`ImportStateIdFunc`, so no change to that side. A second test step using the
`statecheck.ExpectIdentity` assertion would round-trip the modern identity
path; left for the import-test migration.

## Things still required at provider level

The migrated resource needs to be registered with the framework's
`provider.Provider.Resources()` method. That registration is part of provider-
level migration (steps 4–5 in the workflow), not per-resource — so it's a
follow-up beyond this task's scope. The constructor exposed for that purpose
is `NewLBMemberV2Resource()`.

`testAccProtoV6ProviderFactories` referenced in the test file likewise needs
to be defined in the test helpers (`provider_test.go` or similar). It will
look like:

```go
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider()),
}
```

## Risk callouts

1. **`name` made Computed**: a deliberate change to support write-then-read
   semantics where the API may normalise. If the OpenStack API never rewrites
   `name`, this is a no-op. If it does, the previous SDKv2 implementation
   would have surfaced spurious diffs that the framework version now
   suppresses cleanly.
2. **Default `admin_state_up` semantics**: SDKv2 `Default: true` becomes
   `booldefault.StaticBool(true)` plus `Computed: true`. Framework requires
   `Computed` to apply a default — adding `Computed` to a previously Optional
   attribute is a (tiny) state-shape change but should be invisible to users.
3. **`subnet_id`/`monitor_address`/etc. now `Computed`**: needed because
   `Read` writes them back unconditionally. Without `Computed`, attributes
   the practitioner left unset would error if `Read` later populated them.
   Existing state should round-trip cleanly because the values being written
   match what the API returns.
4. **Identity `region`**: included as `RequiredForImport`. If a practitioner's
   identity-block import omits it, they'll get a validation error. Documented
   here so the import-docs update reflects this.
