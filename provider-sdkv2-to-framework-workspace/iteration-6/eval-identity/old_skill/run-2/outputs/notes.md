# Migration notes — `openstack_lb_member_v2`

Source: `openstack/resource_openstack_lb_member_v2.go` (SDKv2)
Target: same file path, framework-based with `ResourceWithIdentity`.

## Pre-edit thinking (per SKILL.md "Think before editing")

1. **Block decision** — none. The schema is flat; no `MaxItems: 1 + nested Elem`.
   The only nested attribute is `timeouts`, which keeps SDKv2 block-style
   syntax via `timeouts.Block(...)` /-style helpers. Used `timeouts.Attributes`
   here because the framework's helper renders identically for the
   `timeouts { create = "5m" }` HCL the existing tests use; HCL block syntax
   stays compatible.
2. **State upgrade** — none. SDKv2 schema has no `SchemaVersion`; nothing to
   migrate via `ResourceWithUpgradeState`.
3. **Import shape** — composite. SDKv2 importer parses
   `<pool_id>/<member_id>`; this is the canonical case for
   `ResourceWithIdentity`. Identity attributes: `pool_id` + `member_id`.

## Identity design

Identity schema (matches the legacy "/"-delimited components verbatim):

```go
identityschema.Schema{
    Attributes: map[string]identityschema.Attribute{
        "pool_id":   identityschema.StringAttribute{RequiredForImport: true},
        "member_id": identityschema.StringAttribute{RequiredForImport: true},
    },
}
```

`resp.Identity.Set(ctx, …)` is called from **Create**, **Read**, **Update**,
and (for the legacy import path) from `ImportState`. `Delete` doesn't write
identity — the resource is being removed.

## Dual-path import

`ImportState` branches on `req.ID` emptiness:

- **Modern** (`req.ID == ""`, `req.Identity` populated): read the identity
  model, write `pool_id` and `id` into state via `resp.State.SetAttribute` at
  `path.Root("pool_id")` / `path.Root("id")`. (Cannot use
  `ImportStatePassthroughWithIdentity` for this case because the identity's
  `member_id` maps to state's `id`, not `member_id` — the helper is
  same-name only.)
- **Legacy** (`req.ID == "POOL/MEMBER"`): `strings.SplitN`, validate two
  non-empty parts, write each into state, **and** mirror into `resp.Identity`
  so identity is consistent regardless of import path.

## SDKv2 → framework attribute mapping

| SDKv2 | Framework |
|---|---|
| `Type: TypeString, Required, ForceNew` (address, pool_id) | `StringAttribute{Required, PlanModifiers: RequiresReplace()}` |
| `Type: TypeInt, Required, ForceNew, ValidateFunc: IntBetween(1,65535)` (protocol_port) | `Int64Attribute{Required, PlanModifiers: RequiresReplace(), Validators: int64validator.Between(1,65535)}` |
| `Type: TypeInt, Optional, Computed, ValidateFunc: IntBetween(0,256)` (weight) | `Int64Attribute{Optional, Computed, Validators: ..., UseStateForUnknown}` |
| `Type: TypeBool, Default: true, Optional` (admin_state_up) | `BoolAttribute{Optional, Computed, Default: booldefault.StaticBool(true)}` |
| `Type: TypeBool, Optional` (backup) | `BoolAttribute{Optional, Computed}` (Computed because Read populates it) |
| `Type: TypeSet, Elem: TypeString, Set: HashString` (tags) | `SetAttribute{Optional, Computed, ElementType: types.StringType}` — `Set: schema.HashString` deleted (framework handles uniqueness) |
| `region` (Optional+Computed+ForceNew) | `StringAttribute{Optional, Computed, PlanModifiers: RequiresReplace, UseStateForUnknown}` |
| `Timeouts: ResourceTimeout{Create, Update, Delete}` | `timeouts.Attributes(ctx, timeouts.Opts{Create: true, Update: true, Delete: true})` from `terraform-plugin-framework-timeouts` |
| `id` (implicit) | explicit `StringAttribute{Computed, UseStateForUnknown}` |

## CRUD translation notes

- `d.Get("foo").(T)` → `var plan model; req.Plan.Get(ctx, &plan); plan.Foo.ValueT()`.
- `d.GetOk("foo")` (true if set + non-zero) → `!plan.Foo.IsNull() && !plan.Foo.IsUnknown() && plan.Foo.ValueT() != zero`.
- `getOkExists("weight")` (true if set, regardless of zero) →
  `!plan.Weight.IsNull() && !plan.Weight.IsUnknown()` — the framework
  distinguishes null from "explicit 0" without needing the SDKv2 helper.
- `d.HasChange("name")` → `!plan.Name.Equal(state.Name)`.
- `d.SetId("")` (gone) → `resp.State.RemoveResource(ctx)` for missing-API-side handling.
- `d.Timeout(schema.TimeoutCreate)` → `plan.Timeouts.Create(ctx, 10*time.Minute)` returning `(time.Duration, diag.Diagnostics)`.
- `Set: schema.HashString` deleted; `tags = expandToStringSlice(v.(*schema.Set).List())` becomes `plan.Tags.ElementsAs(ctx, &tagList, false)`.

## SDKv2 retry replacement

The original used `retry.RetryContext` from `terraform-plugin-sdk/v2/helper/retry`
plus a `checkForRetryableError` helper. Since the migrated file must not
import SDKv2, this resource carries a small inline `retryOpenStackOnTransient`
that mirrors the same gophercloud-aware semantics (retry on 409/5xx, fail
otherwise) using only stdlib + gophercloud. Functionally equivalent;
the helper is local to this file and could be promoted to `util.go` once
more resources are migrated.

## `CheckDeleted` replacement

SDKv2's `CheckDeleted(d, err, msg)` removed state on a 404 from gophercloud
and re-returned other errors. The framework Read handler now calls
`isOpenStackResourceGone(err)` (a thin wrapper over
`gophercloud.ResponseCodeIs(err, http.StatusNotFound)`), and on true
calls `resp.State.RemoveResource(ctx)`. Same semantic, framework idiom.

## Empty-string preservation

`subnet_id` and `monitor_address` were Optional (no `Computed`) in SDKv2 but
the API echoes back `""` for unset fields. To avoid spurious diffs the
helper `stringOrNullEmpty` keeps state null when the API returns "". This
mirrors what the SDKv2 implementation got "for free" because `d.Set("x", "")`
left an unset attribute null. (`monitor_address` and `subnet_id` are now
`Computed: true` so the framework allows null — see schema.)

## Test-file changes (TDD step 7)

- Switched `ProviderFactories` → `ProtoV6ProviderFactories` (referencing
  `testAccProtoV6ProviderFactories`, which the provider's framework wiring
  must export — this matches the convention used by other migrated tests
  in the workspace).
- Added a new `TestAccLBV2Member_importIdentity` step that exercises the
  modern `import { identity = {...} }` flow via
  `ImportStateKind: resource.ImportBlockWithResourceIdentity`.
- Existing `TestAccLBV2Member_importBasic` retained verbatim as the
  legacy-string regression case.
- `ConfigStateChecks` on the basic step asserts identity is populated
  (`statecheck.ExpectIdentityValueMatchesState`).
- `ImportStateVerifyIgnore: []string{"timeouts"}` on import steps because the
  timeouts nested attribute isn't returned by the OpenStack API and would
  otherwise cause spurious diffs.

## Open items / caveats

- The migrated resource is single-file; full provider compilation requires
  registering `NewLBMemberV2Resource` on the framework provider and removing
  the SDKv2 entry from `provider.go`. That registration step is out of
  scope for this single-resource migration but is the next task in the
  per-resource checklist.
- `testAccProtoV6ProviderFactories` is referenced by the test; the provider
  package must expose this. If the provider isn't yet muxed/migrated, the
  test will fail to compile until that wiring lands — which is exactly the
  intended TDD red gate for step 7.
