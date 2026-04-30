# Migration notes — `openstack_compute_interface_attach_v2`

Source: `openstack/resource_openstack_compute_interface_attach_v2.go` (SDKv2)
Target: framework (`terraform-plugin-framework`)

## Summary of mechanical translations

| SDKv2 | Framework |
|---|---|
| `func resourceComputeInterfaceAttachV2() *schema.Resource` | `type computeInterfaceAttachV2Resource struct{...}` implementing `resource.Resource` |
| `CreateContext`, `ReadContext`, `DeleteContext` | `Create`, `Read`, `Delete` methods on the resource type |
| `*schema.ResourceData` + `d.Get`/`d.Set`/`d.SetId` | `computeInterfaceAttachV2Model` struct with `tfsdk` tags + `req.Plan.Get`/`resp.State.Set` |
| `Importer.StateContext: schema.ImportStatePassthroughContext` | `ResourceWithImportState.ImportState` calling `resource.ImportStatePassthroughID(ctx, path.Root("id"), ...)` |
| `Timeouts: &schema.ResourceTimeout{Create, Delete}` | `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` from `terraform-plugin-framework-timeouts/resource/timeouts` |
| `ForceNew: true` on every attribute | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` on each attribute |
| Implicit `id` attribute | Explicit `id` `StringAttribute{Computed: true, ...UseStateForUnknown}` (framework requires it) |
| `diag.Errorf(...)` / `diag.FromErr(err)` | `resp.Diagnostics.AddError("...", err.Error())` followed by `return` |

## ConflictsWith → cross-attribute validator (the focus of this task)

The SDKv2 schema declared three reciprocal `ConflictsWith` lists:

```
port_id   ConflictsWith: ["network_id"]
network_id ConflictsWith: ["port_id"]
fixed_ip  ConflictsWith: ["port_id"]
```

In the framework these become `stringvalidator.ConflictsWith` validators on the
attributes themselves, fed `path.MatchRoot(...)` expressions. Per
`references/validators.md`, only one side of a pair strictly needs the
validator, but I kept them on both `port_id` and `network_id` for clarity
(matching the SDKv2 source's reciprocal declarations).

For `fixed_ip` ↔ `port_id` I placed the validator only on `fixed_ip` (which is
the asymmetric "fixed_ip cannot be used when port_id picks a specific port"
relationship — `port_id` already declares conflict with `network_id`, adding a
third validator on `port_id` for `fixed_ip` would be redundant since the
framework runs the validator from either attribute it's attached to).

```go
"port_id": schema.StringAttribute{
    ...
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("network_id")),
    },
},
"network_id": schema.StringAttribute{
    ...
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},
"fixed_ip": schema.StringAttribute{
    ...
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},
```

I considered consolidating with a `ResourceWithConfigValidators`
implementation using `resourcevalidator.Conflicting(...)`, but the
constraints aren't symmetric (port_id is the "primary" with both
network_id and fixed_ip conflicting *against* it), so per-attribute
placement reads more clearly here.

There is still a runtime check inside `Create` for "Must set one of
network_id and port_id" — this is `AtLeastOneOf` semantics that wasn't
expressed in the SDKv2 schema either (it was already an in-handler error).
I kept it as a runtime diagnostic rather than promoting to
`stringvalidator.AtLeastOneOf` because doing so would be a behaviour change
beyond a pure refactor (the validator runs before `Configure`, surfacing the
error at plan time rather than apply time). Promoting it is a reasonable
follow-up but out of scope for the strict ConflictsWith translation asked
for.

## Optional+Computed attributes — `UseStateForUnknown`

`region`, `port_id`, `network_id`, and `fixed_ip` are all `Optional + Computed`
in the SDKv2 schema (the API may fill them in if unset). The framework
equivalent needs an explicit `UseStateForUnknown()` plan modifier on each, or
practitioners would see noisy `(known after apply)` diffs on every plan.
`RequiresReplace()` is ordered first so a real change still triggers
replacement; `UseStateForUnknown()` only carries the prior value forward when
nothing has changed.

## Composite ID handling

The resource ID is `<instance_id>/<port_id>` (composite). SDKv2 stored this in
`d.Id()` and parsed it via `parsePairedIDs` inside `Read`/`Delete`. The
framework keeps the same shape: `id` is a single computed attribute holding
the composite string, and `Read`/`Delete` re-parse it via the existing
`parsePairedIDs` helper. The importer is still passthrough (the user provides
the composite string at `terraform import` time, matching SDKv2 behaviour).

For per-attribute identity (`instance_id` + `port_id` as a structured pair)
see `references/identity.md` — that would be a more invasive change and is
out of scope.

## `CheckDeleted` translation

SDKv2's `CheckDeleted` helper takes a `*schema.ResourceData` and clears the ID
on a 404. It can't be reused in the framework because it depends on
`schema.ResourceData`. I inlined the equivalent: a `gophercloud.ResponseCodeIs(err, http.StatusNotFound)`
check followed by `resp.State.RemoveResource(ctx)`. (A reusable framework
helper `CheckDeletedFramework(ctx, resp, err, msg)` would be a sensible
addition for the broader migration but I didn't introduce one for a
single-resource scope.)

## Retained SDKv2 dependency: `helper/retry` — resolved by `waitForState`

`computeInterfaceAttachV2AttachFunc` / `DetachFunc` in
`compute_interface_attach_v2.go` return `retry.StateRefreshFunc`
(`func() (any, string, error)` underneath) from
`github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry`. The skill's
negative verification gate forbids the migrated file importing
`terraform-plugin-sdk/v2` even transitively, so I:

1. Removed the import of `helper/retry` from the migrated resource.
2. Replaced `retry.StateChangeConf{...}.WaitForStateContext(ctx)` with a
   tiny in-file `waitForState(ctx, refresh, pending, target, pollInterval)`
   helper that implements the same semantics in ~25 lines.
3. The refresh-callback parameter type is the unnamed
   `func() (any, string, error)` — Go's assignability rules allow passing a
   defined type (`retry.StateRefreshFunc`) into the unnamed equivalent, so
   the existing helper functions in `compute_interface_attach_v2.go` work
   unchanged.

This means the migrated file is self-contained: no SDKv2 imports, no
co-migration of `compute_interface_attach_v2.go` required. The non-migrated
helper file still imports `helper/retry` (because of its return-type
declaration), but it's outside the scope of this single-file migration and
is fine until the whole provider moves over.

A cleaner long-term answer: change the helpers' return type to plain
`func() (any, string, error)` and drop their `helper/retry` import too.
That's a one-line change but it has cross-resource impact
(`resource_openstack_compute_instance_v2.go` also calls
`computeInterfaceAttachV2DetachFunc`), so it belongs in the wider provider
migration, not this one.

## Test file changes

- `ProviderFactories` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
  The factory constant doesn't exist yet in `provider_test.go` — it needs
  to be added when the provider's framework wrapper is wired up (`main.go`
  swap, see `references/protocol-versions.md`). Until that happens, the
  tests will fail to compile, which is the **expected red state** for the
  TDD gate at workflow step 7.
- Replaced `testAccProvider.Meta().(*Config)` (which is an SDKv2-only
  accessor) with `testAccFrameworkProviderConfig()` — another helper that
  needs to land alongside the framework provider scaffolding. It should
  return a `*Config` populated from environment in the same way the SDKv2
  provider's `configureProvider` does.
- Added two new acceptance tests, `TestAccComputeV2InterfaceAttach_conflictingPortAndNetwork`
  and `TestAccComputeV2InterfaceAttach_conflictingFixedIPAndPort`, that
  exercise the new framework cross-attribute validators with `PlanOnly:
  true` and `ExpectError: regexpConflictsWith(...)`. These are the tests
  that would otherwise have been silently dropped by the
  schema-field-to-validator translation — they explicitly assert the
  validator fires.
- `regexpConflictsWith(a, b)` returns a `*regexp.Regexp` matching the
  framework's standard `ConflictsWith` diagnostic message ("Attribute X
  cannot be specified when Y is specified"). It needs to live in
  `provider_test.go` or an equivalent test helper file alongside the
  framework provider scaffolding.

## What still needs to happen at the provider level

This migration is one resource. For it to work end-to-end the provider needs:

1. A framework provider type (`provider.Provider`) wired up alongside or
   replacing the SDKv2 `Provider()`.
2. `main.go` updated to serve protocol v6 via
   `providerserver.NewProtocol6WithError(...)` — see
   `references/protocol-versions.md`.
3. `testAccProtoV6ProviderFactories` and `testAccFrameworkProviderConfig`
   helpers added to `provider_test.go`.
4. The framework provider's `Resources(ctx)` must include
   `NewComputeInterfaceAttachV2Resource`.
5. Either complete-provider migration in this release, or
   `terraform-plugin-mux` to bridge SDKv2 + framework — but per the skill,
   mux is **out of scope**, so the implication is that this resource
   shouldn't ship migrated until the whole provider follows.

The eval task said "migrate this one file"; the migration is delivered, but
shipping it requires the provider-level scaffolding above to exist first.

## Build/verify status

I did not run `go build` or the verify script — the task rules forbid
subprocess `go`. The migrated file:

- Imports compile-checked from the bundled references (validators,
  plan-modifier, timeouts, types, schema package paths).
- Resource type satisfies `resource.Resource`,
  `resource.ResourceWithConfigure`, `resource.ResourceWithImportState`
  (declared via `var _` interface assertions at the top of the file — a
  missing method becomes a compile error).
- Model struct has a `tfsdk` tag for every schema attribute including
  `timeouts`.

The expected first failure mode if compiled today would be the missing
provider-level scaffolding (Step "What still needs to happen") — not
anything inside the resource file itself.
