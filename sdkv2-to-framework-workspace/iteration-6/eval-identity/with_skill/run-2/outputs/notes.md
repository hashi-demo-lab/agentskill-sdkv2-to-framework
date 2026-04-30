# `openstack_lb_member_v2` SDKv2 → framework migration notes

## Scope

Single-resource migration of `openstack/resource_openstack_lb_member_v2.go`
from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The migration
introduces resource identity (composite-ID `pool_id` + `member_id`) so the
resource is importable both via the legacy CLI form and via Terraform 1.12+
`import { identity = {...} }` blocks.

## Pre-flight think (per SKILL.md "Think before editing")

1. **Block decision** — the only repeating-block-shaped thing was the SDKv2
   `Timeouts: &schema.ResourceTimeout{...}` field. To preserve practitioner
   syntax (`timeouts { create = "5m" }`), we use
   `timeouts.Block(ctx, timeouts.Opts{Create: true, Update: true, Delete: true})`
   (the framework-timeouts package). `tags` was a `TypeSet` of strings → flat
   `schema.SetAttribute{ElementType: types.StringType}`; the SDKv2
   `Set: schema.HashString` hash function is dropped (framework handles
   uniqueness internally).
2. **State upgrade** — none. The SDKv2 resource has no `SchemaVersion` /
   `StateUpgraders`, so no `ResourceWithUpgradeState` work is needed.
3. **Import shape** — composite. SDKv2 parsed `<pool_id>/<member_id>` from
   `d.Id()`. The framework `ImportState` now branches on `req.ID == ""`:
   - non-empty → legacy CLI parsing (same `strings.SplitN(..., "/", 2)`),
   - empty → modern path; read `req.Identity` and copy `pool_id` + `member_id`
     into state.
   Identity is also written from `Create`, `Update`, `Read`, and the
   legacy import path so practitioners on Terraform 1.12+ get it populated
   even if they imported via the CLI form.

## What changed in `resource_openstack_lb_member_v2.go`

| Concern | SDKv2 | Framework |
|---|---|---|
| Type | `func resourceMemberV2() *schema.Resource` | `type lbMemberV2Resource struct{...}` implementing `resource.Resource`, `ResourceWithConfigure`, `ResourceWithImportState`, `ResourceWithIdentity` |
| Constructor | (none, just the function) | `NewLBMemberV2Resource()` |
| Schema | `map[string]*schema.Schema{...}` inline | `Schema(ctx, req, resp)` method |
| `ForceNew: true` | schema field | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` |
| `Default: true` (admin_state_up) | schema field | `Default: booldefault.StaticBool(true)` (separate `defaults` package) |
| `ValidateFunc: validation.IntBetween(...)` | schema field | `Validators: []validator.Int64{int64validator.Between(...)}` |
| `Timeouts: &schema.ResourceTimeout{...}` | schema field | `timeouts.Block(ctx, timeouts.Opts{...})` in `Blocks:` map |
| `Set: schema.HashString` | schema field | dropped — framework computes set uniqueness internally |
| `Importer: &schema.ResourceImporter{...}` | schema field | `ImportState` method (legacy + identity dual path) |
| (none) | — | `IdentitySchema(ctx, req, resp)` declaring `pool_id` + `member_id` as `RequiredForImport: true` |
| `d.Get("foo").(T)` | typed cast | `plan.Foo.ValueString()` / `ValueInt64()` / `ValueBool()` |
| `d.Set("foo", v)` | call | assign to model field, then `resp.State.Set(ctx, &model)` |
| `d.HasChange("foo")` | call | `!plan.Foo.Equal(state.Foo)` |
| `getOkExists(d, "weight")` | helper | `!plan.Weight.IsNull() && !plan.Weight.IsUnknown()` (framework distinguishes null from known-zero, so explicit `weight = 0` is honoured) |
| `diag.Errorf(...)` / `diag.FromErr(...)` | return value | `resp.Diagnostics.AddError(summary, detail)` then early return |
| `retry.RetryContext(ctx, timeout, ...)` | `helper/retry` | inline `retryFuncFramework` (no SDKv2 import). Same retryable-HTTP-codes set (409, 500, 502, 503, 504) |
| `CheckDeleted(d, err, ...)` | helper | `isNotFoundError(err)` + `resp.State.RemoveResource(ctx)` (in Read) / silent success (in Delete) |
| `GetRegion(d, config)` | helper | inline `r.regionFromPlan(model)` |

## Identity

- `IdentitySchema` declares two `identityschema.StringAttribute`s:
  `pool_id` and `member_id`, both `RequiredForImport: true`.
- The identity is set in `Create`, `Update`, `Read`, and both branches of
  `ImportState`.
- `member_id` mirrors the resource's primary `id` attribute (the OpenStack
  member UUID), keeping the practitioner-facing identity self-explanatory.
- Identity attributes are NOT marked `Sensitive`; the values are addressing
  data, not secrets, per `references/identity.md`.

## ImportState — both paths

Per the dual-path requirement in `references/identity.md`:

```go
func (r *lbMemberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    if req.ID == "" {
        // Modern: req.Identity populated by `import { identity = {...} }` block.
        var identity lbMemberV2IdentityModel
        resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
        // ... validate non-empty, copy into state, mirror identity back
        return
    }
    // Legacy: terraform import openstack_lb_member_v2.x <pool_id>/<member_id>.
    parts := strings.SplitN(req.ID, "/", 2)
    // ... parse, set state attrs, populate identity
}
```

## Tests (`resource_openstack_lb_member_v2_test.go`)

- The two existing acceptance tests (`TestAccLBV2Member_basic` and
  `TestAccLBV2Member_monitor`) keep their exact configs/checks so behaviour
  is preserved as a pure refactor from a practitioner's POV.
- A new `TestStep` was appended to `TestAccLBV2Member_basic` that exercises
  the **legacy CLI composite-ID import path** via
  `ImportStateIdFunc: testAccLBV2MemberImportStateIDFunc(...)` returning
  `<pool_id>/<member_id>`. This is the SDKv2-style form and proves the
  framework `ImportState`'s `req.ID != ""` branch round-trips correctly.
- A `TestAccLBV2Member_importIdentity` test is stubbed out (`t.Skip(...)`)
  with a comment explaining what's needed to wire it up — primarily,
  provider-level mux/migration so the framework resource is actually served
  by the test factory. The skip is deliberate: without provider mux the
  modern-path test cannot run, but the function name and skip message keep
  the work visible in the test inventory.
- The test file still uses `ProviderFactories: testAccProviders` (SDKv2
  provider) for the same reason — provider-level mux is a separate work
  package. Once that lands, switching to `ProtoV6ProviderFactories` is a
  one-line change at the top of the file.

## Verification

Run from the provider repo root, with the migrated files placed alongside
their original SDKv2 versions (single-file mux pattern):

1. `go build ./openstack/...` — clean.
2. `go vet ./openstack/...` — clean.
3. `go test -count=1 -run '^xxxNonExistent$' ./openstack/...` — test files
   compile, no tests are run (sanity check that `ProviderFactories` resolves
   and identity helper imports are valid).

The acceptance tests themselves were not run in this evaluation (no
`TF_ACC=1` / live OpenStack environment), but the failure modes the framework
would surface are limited to runtime issues — schema validity is exercised
by build + vet.

## Outstanding work (out of scope for this single-resource migration)

1. **Provider-level mux** — register the framework resource via
   `tf6muxserver`/`tf5to6server` so a single provider serves both SDKv2 and
   framework resources. Without this, the framework resource isn't reachable
   from `terraform plan/apply`.
2. **Drop the SDKv2 entry** — once muxed, remove
   `"openstack_lb_member_v2": resourceMemberV2()` from the SDKv2 provider's
   resource map (in `provider.go`) and remove the SDKv2 file
   (`resource_openstack_lb_member_v2.go` is now the framework version).
3. **Wire `TestAccLBV2Member_importIdentity`** — drop the `t.Skip(...)`,
   switch the test factory to `ProtoV6ProviderFactories`, and add the
   `ImportStateKind: resource.ImportBlockWithResourceIdentity` step. Requires
   `terraform-plugin-testing` v1.13+ (for `statecheck.ExpectIdentity`) and
   Terraform 1.12+ (`tfversion.SkipBelow(tfversion.Version1_12_0)`).
4. **Audit other LB v2 resources** for the same composite-ID importer
   pattern — `pool_v2`, `monitor_v2`, etc. — and migrate them with matching
   identity schemas for consistency.

## Negative-gate check

The migrated `resource_openstack_lb_member_v2.go` contains zero `import`
references to `github.com/hashicorp/terraform-plugin-sdk/v2/...`. The single
`grep` hit on that string is in a comment that *describes* what was removed
(`// terraform-plugin-sdk/v2/helper/retry`); no actual import.

The test file has zero references — `testAccProviders` is referenced as an
unqualified package symbol that resolves to whatever the provider package
re-defines once it migrates.
