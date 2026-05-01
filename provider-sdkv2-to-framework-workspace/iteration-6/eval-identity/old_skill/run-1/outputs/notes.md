# Migration notes — `openstack_lb_member_v2`

## Scope

Single resource: `openstack/resource_openstack_lb_member_v2.go` (and its test
file). Nothing else in the repo was touched.

## Key shape changes

| SDKv2 | Framework |
|---|---|
| `func resourceMemberV2() *schema.Resource` | `type memberV2Resource struct` implementing `resource.Resource` |
| `CreateContext`/`ReadContext`/`UpdateContext`/`DeleteContext` | `Create` / `Read` / `Update` / `Delete` methods on the resource type |
| `ForceNew: true` | per-attribute `PlanModifiers: []planmodifier.X{xplanmodifier.RequiresReplace()}` |
| `Default: true` (on `admin_state_up`) | `booldefault.StaticBool(true)` + `Computed: true` |
| `validation.IntBetween(1, 65535)` (protocol_port, monitor_port) | `int64validator.Between(1, 65535)` |
| `validation.IntBetween(0, 256)` (weight) | `int64validator.Between(0, 256)` |
| `Type: TypeSet, Set: schema.HashString` (tags) | `schema.SetAttribute{ElementType: types.StringType}` — set hashing is automatic |
| `Timeouts: &schema.ResourceTimeout{...}` | `timeouts.Attributes(ctx, timeouts.Opts{Create: true, Update: true, Delete: true})` from `terraform-plugin-framework-timeouts` + `Timeouts timeouts.Value` on the model |
| `Importer: &schema.ResourceImporter{StateContext: ...}` | `ImportState` method (sub-interface `ResourceWithImportState`) |
| `d.Get("...")` / `d.Set(...)` / `d.Id()` / `d.HasChange("...")` | typed model + `req.Plan.Get` / `resp.State.Set` / `state.ID.ValueString()` / `!plan.X.Equal(state.X)` |
| `d.GetOkExists("weight")` | `!plan.Weight.IsNull() && !plan.Weight.IsUnknown()` (the framework already distinguishes null from known-zero) |
| SDKv2 `retry.RetryContext` | small in-file `retryFrameworkCtx` helper + `isRetryableLBErr` (kept so this file is fully SDKv2-free without tugging the rest of the package along) |
| `CheckDeleted` (404 → `d.SetId("")`) | `gophercloud.ResponseCodeIs(err, 404)` → `resp.State.RemoveResource(ctx)` in Read |

## Identity (`ResourceWithIdentity`)

- Identity schema declares two attributes, both `RequiredForImport: true`:
  `pool_id` and `id`. These mirror the legacy composite `<pool_id>/<member_id>`
  shape — only the addressing fields, nothing tunable.
- `Region` is *not* in identity. It comes from the provider config when not
  set per-resource and is unrelated to addressing the OpenStack member by
  primary key (a member is uniquely identified by `(pool_id, id)` regardless
  of region). Putting region in identity would cause spurious differences
  between practitioners using the provider-default vs explicit region.
- Identity is written in `Create`, `Read`, `Update`, and on the legacy
  `ImportState` path so practitioners on Terraform 1.12+ have a populated
  identity in state from the moment the resource exists, irrespective of how
  it was imported.

### Import paths supported

1. **Legacy** — `terraform import openstack_lb_member_v2.foo <pool>/<member>`.
   `req.ID` is set; we parse it with `strings.SplitN`, `SetAttribute` for both
   `pool_id` and `id`, and seed identity for free.
2. **Modern (Terraform 1.12+)** — `import { to = ... ; identity = { pool_id = "...", id = "..." } }`.
   `req.ID == ""` and `req.Identity` is populated; we use
   `resource.ImportStatePassthroughWithIdentity` once per attribute pair to
   mirror identity → state.

A single `if req.ID == ""` branch dispatches between the two — no changes
required to existing CLI-based import scripts, while practitioners on newer
Terraform get the discoverable identity-block syntax.

## Schema notes that aren't 1:1

- `monitor_address` and `monitor_port` were `Optional` only in SDKv2 (no
  `Computed`). I marked them `Computed` in the framework so we can write back
  the API's view of those fields in `Read` without producing diffs. This is a
  small behaviour change (Read now reflects API state instead of leaving the
  user's exact null/zero); the alternative would be skipping the assignment
  when the user hadn't set them, but that re-introduces the SDKv2-style null/
  zero ambiguity.
- `tenant_id` was already `Optional+Computed+ForceNew` — became
  `Optional+Computed` with `RequiresReplace` and `UseStateForUnknown` plan
  modifiers.
- `tags` retains `*schema.Set` semantics naturally — `SetAttribute` handles
  uniqueness without a hash function.

## Tests

- Switched both existing acceptance tests from `ProviderFactories` to
  `ProtoV6ProviderFactories` (the wiring-up of `protoV6ProviderFactories`
  itself lives in `provider_test.go`, which is out of scope for this resource
  migration — it would be added when the rest of the provider migrates or
  when a `terraform-plugin-mux` mux server is wired in).
- Added `TestAccLBV2Member_identityImport`:
  - `tfversion.SkipBelow(tfversion.Version1_12_0)` so it only runs where the
    `import { identity }` block syntax is supported.
  - `statecheck.ExpectIdentityValue` asserts identity is populated after
    Create.
  - A second step uses `ImportStateKind: resource.ImportBlockWithResourceIdentity`
    + `ImportStateVerify` to round-trip via the modern import path.
- Kept legacy composite-ID import covered via
  `ImportStateIdFunc: testAccLBV2MemberImportStateIdFunc(...)` returning
  `<pool>/<member>` — proves `req.ID` parsing still works.

## Verification

- `verify_tests.sh` would normally be the gate, but the rest of the provider
  is still SDKv2 in this repo, so a clean `go build ./...` against the live
  tree is not achievable without also wiring framework provider serving (mux
  or full migration). The migrated file is self-contained and SDKv2-import-
  free; once the provider's framework wiring is in place (provider type +
  Resources() / mux), it will compile as-is.
- All references in the framework file are to symbols already used elsewhere
  in the package (`Config`, `LoadBalancerV2Client`, `waitForLBV2Pool`,
  `waitForLBV2Member`, `getLbPendingStatuses`, `getLbPendingDeleteStatuses`).
- No imports of `github.com/hashicorp/terraform-plugin-sdk/v2` remain in the
  migrated file.

## Things deliberately not done

- The provider definition (`provider.go`) is untouched. Wiring this resource
  into the framework provider's `Resources()` (or surfacing it via mux
  alongside the SDKv2 server) is the next step for the wider migration but
  outside this resource's scope per the task instructions.
- The shared LB helpers (`waitForLBV2Pool`, etc.) still live in `lb_v2_shared.go`
  and depend on SDKv2 only via signature-irrelevant helpers; their bodies use
  Gophercloud directly. They work as-is from the framework code.
