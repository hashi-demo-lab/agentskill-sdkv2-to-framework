# Migration notes — `openstack_compute_interface_attach_v2`

Single-resource SDKv2 → terraform-plugin-framework migration. Focus of this
eval is `ConflictsWith` → framework cross-attribute validators.

## ConflictsWith → framework validators (the load-bearing change)

### SDKv2 conflict graph (read straight off the original schema)

```go
"port_id":    {ConflictsWith: []string{"network_id"}},  // line 42
"network_id": {ConflictsWith: []string{"port_id"}},     // line 50
"fixed_ip":   {ConflictsWith: []string{"port_id"}},     // line 64
```

Three pairs:

| A | conflicts with | B |
|---|---|---|
| `port_id`  | ↔ | `network_id` |
| `port_id`  | ↔ | `fixed_ip`   |

The graph is **asymmetric**: `port_id` is the central exclusive alternative;
`network_id` and `fixed_ip` are the other side and **do not conflict with each
other** — `fixed_ip` is the IP to assign on the network identified by
`network_id` (this is exactly what `TestAccComputeV2InterfaceAttach_IP`
exercises with `network_id = ... + fixed_ip = "192.168.1.100"`).

### Pattern chosen — per-attribute `stringvalidator.ConflictsWith`

```go
"port_id": schema.StringAttribute{
    Validators: []validator.String{
        stringvalidator.ConflictsWith(
            path.MatchRoot("network_id"),
            path.MatchRoot("fixed_ip"),
        ),
    },
},
"network_id": schema.StringAttribute{
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},
"fixed_ip": schema.StringAttribute{
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},
```

### Why per-attribute, not `ResourceWithConfigValidators`

`references/validators.md` calls out the choice explicitly: schema-wide
`resourcevalidator.Conflicting` / `ExactlyOneOf` reads cleaner only when A, B,
C are **symmetric alternatives** — i.e. all of them mutually exclude each
other.

That is **not** the shape of this resource:

- `port_id` excludes `network_id` AND excludes `fixed_ip`.
- `network_id` and `fixed_ip` are **compatible** with each other (intentionally —
  see `TestAccComputeV2InterfaceAttach_IP`).

A single `resourcevalidator.Conflicting(port_id, network_id, fixed_ip)` would
**over-constrain** the resource. `resourcevalidator.ExactlyOneOf(...)` is
also wrong: two of the three may legitimately coexist.

So per-attribute placement preserves the asymmetric semantics exactly.
Per `validators.md` ("idiomatic to put it on both for clarity"), the
constraint is duplicated on both sides of each pair; the framework only needs
it on one side to be enforced.

### What was *not* turned into a validator (deliberately)

The original `Create` has a runtime check:

```go
if networkID == "" && portID == "" {
    return diag.Errorf("Must set one of network_id and port_id")
}
```

This is "at least one of `network_id` / `port_id` must be set". I considered
expressing it as `stringvalidator.AtLeastOneOf(path.MatchRoot("port_id"))` on
`network_id` (and vice versa), but kept the runtime check because:

1. The task brief specifically asks about `ConflictsWith`; there is no
   `AtLeastOneOf` constraint in the SDKv2 schema today.
2. Both attributes are `Optional + Computed`. With Computed attributes, an
   `AtLeastOneOf` validator can be a footgun if either value is unknown at
   plan time — a runtime check after attribute resolution is more
   conservative.
3. Identical pre/post-migration error message + timing keeps this a refactor.

Promoting it to `AtLeastOneOf` is reasonable as a follow-up, but it would be a
behaviour change (config-time vs apply-time validation), not a refactor.

## Other SDKv2 → framework conversions in this file

| SDKv2 | Framework |
|---|---|
| `func resourceComputeInterfaceAttachV2() *schema.Resource` | `type computeInterfaceAttachV2Resource struct{ config *Config }` + `NewComputeInterfaceAttachV2Resource()` constructor + `resource.Resource` / `ResourceWithConfigure` / `ResourceWithImportState` interface impls |
| `CreateContext` / `ReadContext` / `DeleteContext` | `Create` / `Read` / `Delete` with typed `(req, resp)` and `resp.Diagnostics`; added a no-op `Update` to satisfy `resource.Resource` |
| `ForceNew: true` (region, port_id, network_id, instance_id, fixed_ip) | `stringplanmodifier.RequiresReplace()` |
| `Optional + Computed` (region, port_id, network_id, fixed_ip) | added `stringplanmodifier.UseStateForUnknown()` so values stay stable across plans |
| Implicit `id` | explicit `schema.StringAttribute{Computed: true, PlanModifiers: …UseStateForUnknown}` (framework requires every attribute declared) |
| `Timeouts: &schema.ResourceTimeout{Create, Delete}` | `terraform-plugin-framework-timeouts` `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` — **Block** form preserves the existing `timeouts { ... }` HCL syntax used by current practitioner configs |
| `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` | `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| `retry.StateChangeConf` (Pending/Target ATTACHING/ATTACHED, ""/DETACHED) | inline `waitForComputeInterfaceAttach` / `waitForComputeInterfaceDetach` driven by `context.WithTimeout` — this resource file no longer imports `terraform-plugin-sdk/v2/helper/retry`, satisfying the negative gate |
| `d.Get` / `d.Set` / `d.Id` / `d.SetId` | typed `req.Plan.Get(ctx, &plan)` and `resp.State.Set(ctx, &plan)` against `computeInterfaceAttachV2Model` |
| `d.GetOk("port_id")` etc. | `if !plan.PortID.IsNull() && !plan.PortID.IsUnknown() { … }` |
| `diag.Errorf` / `diag.FromErr` | `resp.Diagnostics.AddError(summary, detail)` |
| `CheckDeleted(d, err, msg)` | inline 404 handling: `if gophercloud.ResponseCodeIs(err, http.StatusNotFound) { resp.State.RemoveResource(ctx); return }` (the helper takes `*schema.ResourceData`, which doesn't exist in the framework world) |
| `GetRegion(d, config)` | private `regionFor(model)` method on the resource — same precedence (resource-level value if set, else `config.Region`) |

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. Comma-aligned formatting tweak.
- Helpers (`testAccCheckComputeV2InterfaceAttachExists`, `testAccCheckComputeV2InterfaceAttachIP`, `testAccCheckComputeV2InterfaceAttachDestroy`) and HCL fixtures untouched: they only inspect public schema (`rs.Primary.ID`, `rs.Primary.Attributes`), which is preserved by the migration.
- `testAccProvider.Meta().(*Config)` is kept inside the destroy/exists helpers. While the rest of the provider is still SDKv2, that handle remains valid; once the rest of the provider migrates, the helpers should be reworked to take a `*Config` parameter rather than reaching back through the SDKv2 test-provider singleton.

## TDD ordering (workflow step 7)

The task instructs migrating both the resource file and its test file in one
shot, so strict step-7 red-then-green isn't directly observable here. The
test-side change (the factory swap) would, in isolation, fail to compile/link
until:

1. `testAccProtoV6ProviderFactories` is declared in the package's test
   scaffolding (out of scope per the task).
2. `NewComputeInterfaceAttachV2Resource` is registered in the framework
   provider's `Resources()` slice (also out of scope).

That dependency chain is what step 7 is meant to surface; it is preserved
here.

## Remaining caveats for the integrator

- This file no longer imports any `terraform-plugin-sdk/v2/...` symbol — the
  negative gate in `verify_tests.sh` will pass for it. The companion file
  `compute_interface_attach_v2.go` (the `Attach`/`Detach` `retry.StateRefreshFunc`
  helpers) is unchanged here because the migrated resource no longer calls it —
  delete those helpers when their last SDKv2 caller (`resource_openstack_compute_instance_v2.go`)
  also migrates, or convert their return type to `func() (any, string, error)` and reuse them.
- The provider-level wiring (registering the resource on the framework
  provider, building a muxed server if SDKv2 resources still live alongside
  this one) is out of scope per the task brief. Without it, the test file
  won't compile in tree — that's the expected step-7 red signal.
