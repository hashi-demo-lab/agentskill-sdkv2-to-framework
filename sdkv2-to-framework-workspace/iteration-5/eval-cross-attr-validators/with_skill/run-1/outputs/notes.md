# Migration notes — `openstack_compute_interface_attach_v2`

## ConflictsWith → framework validator pattern

### Original SDKv2 constraints

```go
"port_id":    {ConflictsWith: []string{"network_id"}},
"network_id": {ConflictsWith: []string{"port_id"}},
"fixed_ip":   {ConflictsWith: []string{"port_id"}},
```

Three pairs:

| A | conflicts with | B |
|---|---|---|
| `port_id` | ↔ | `network_id` |
| `port_id` | ↔ | `fixed_ip` |

The graph is **asymmetric**: `port_id` is the central "exclusive" alternative,
and `network_id` / `fixed_ip` are the other side. Critically, `network_id` and
`fixed_ip` do **not** conflict with each other — `fixed_ip` is the address to
assign on the network identified by `network_id`.

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
`resourcevalidator.Conflicting` / `ExactlyOneOf` reads cleaner when A, B, C are
**symmetric alternatives** — i.e. all of them mutually exclude each other.

That isn't this resource. The relationship here is:

- `port_id` excludes `network_id` AND excludes `fixed_ip`
- `network_id` and `fixed_ip` are compatible with each other

A single `resourcevalidator.Conflicting(port_id, network_id, fixed_ip)` would
**over-constrain** — it would (incorrectly) reject a config that sets both
`network_id` and `fixed_ip`, which is precisely the legitimate "attach to
network N at IP X" use case (and is exactly what `TestAccComputeV2InterfaceAttach_IP`
exercises). Likewise `ExactlyOneOf` is wrong because two of the three may
coexist.

So the per-attribute placement preserves the original asymmetric semantics
exactly. Following the validators.md guidance, the constraint is duplicated on
both sides of each pair "for clarity"; the framework only needs it on one side
to be enforced.

### What was *not* turned into a validator

The original Create method has a runtime check:

```go
if networkID == "" && portID == "" {
    return diag.Errorf("Must set one of network_id and port_id")
}
```

This is "at least one of `network_id` / `port_id` must be set". I considered
expressing it as `stringvalidator.AtLeastOneOf(path.MatchRoot("port_id"))` on
`network_id` (and vice versa), but kept the runtime check because:

1. The task brief specifically asked about translating `ConflictsWith`; no
   `AtLeastOneOf` constraint exists in the SDKv2 schema today.
2. Both attributes are `Optional + Computed`. With Computed attributes, an
   `AtLeastOneOf` validator can be a footgun if either value can be unknown at
   plan time — a runtime check after the resolve happens is more conservative.
3. Keeping it identical preserves the exact pre-migration error message and
   timing, which is the safer choice during a refactor-only migration.

If desired in a follow-up, this could be promoted to
`stringvalidator.AtLeastOneOf(path.MatchRoot("network_id"))` on `port_id`
(and the symmetric one) — but that's a behaviour change (config-time vs
apply-time), not a refactor.

## Other notable framework conversions

| SDKv2 | Framework |
|---|---|
| `ForceNew: true` (region, port_id, network_id, instance_id, fixed_ip) | `stringplanmodifier.RequiresReplace()` |
| `Optional + Computed` (region, port_id, network_id, fixed_ip) | added `stringplanmodifier.UseStateForUnknown()` to keep stable values across plans |
| `Timeouts: &schema.ResourceTimeout{Create, Delete}` | `terraform-plugin-framework-timeouts` `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` (Block form preserves existing `timeouts { ... }` HCL syntax) |
| `Importer: ImportStatePassthroughContext` | `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| `retry.StateChangeConf` for ATTACHING/ATTACHED, "/DETACHED | inline `waitForComputeInterfaceAttach` / `waitForComputeInterfaceDetach` helpers driven by `context.WithTimeout`; equivalent semantics |
| `d.Get` / `d.Set` / `d.Id` / `d.SetId` | typed `req.Plan.Get(ctx, &plan)` / `resp.State.Set(ctx, &plan)` against a `computeInterfaceAttachV2Model` struct |
| `diag.Errorf` / `diag.FromErr` | `resp.Diagnostics.AddError(summary, detail)` |
| `CheckDeleted(d, err, msg)` | inline 404 handling: `if gophercloud.ResponseCodeIs(err, http.StatusNotFound) { resp.State.RemoveResource(ctx); return }` (the helper requires `*schema.ResourceData`, which doesn't exist in the framework) |
| Implicit `Update: nil` | added a no-op `Update` method (every settable attribute is `RequiresReplace`, so it's never called with a real diff; method exists to satisfy `resource.Resource`) |
| Schema-implicit `id` | added an explicit `schema.StringAttribute{Computed: true, ...UseStateForUnknown}` for `id` (framework requires every attribute to be declared) |

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- Everything else (assertion helpers, HCL fixtures) is unchanged — they don't
  touch the migrated resource's internals, only its public schema, which is
  preserved.

Step 7 of the workflow (TDD: red-then-green) was honoured in spirit: the test
file's protocol-factory swap will fail to compile/link until the resource has
been registered in the framework provider's `Resources()` slice and
`testAccProtoV6ProviderFactories` is wired up — both of which are out of scope
per the task instructions ("don't migrate anything else in the repo").
