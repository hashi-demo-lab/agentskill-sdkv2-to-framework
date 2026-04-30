# Migration notes — `openstack_compute_interface_attach_v2`

## Scope

Migrated `openstack/resource_openstack_compute_interface_attach_v2.go` (and its `_test.go`)
from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The rest of the
provider remains on SDKv2; this is a partial migration of one resource.

## ConflictsWith → framework idiom

The SDKv2 schema declared three `ConflictsWith` relationships:

| Attribute    | Conflicts with         |
|--------------|------------------------|
| `port_id`    | `network_id`           |
| `network_id` | `port_id`              |
| `fixed_ip`   | `port_id`              |

In the framework these move out of the schema's flat fields and onto the
attribute's `Validators` slice, using `stringvalidator.ConflictsWith` plus
`path.MatchRoot(...)`:

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

Notes on the placement:

- `stringvalidator.ConflictsWith` is symmetric — declaring it once is
  sufficient for the conflict to fire from either side. The SDKv2 schema
  declared the relation on both `port_id` and `network_id`; we kept the same
  symmetric declaration for clarity (per `references/validators.md`: "it's
  idiomatic to put it on both for clarity").
- A `resourcevalidator.Conflicting(...)` schema-level alternative was
  considered but rejected: `port_id` is the "primary" attribute of two
  different conflict pairs (`port_id`↔`network_id` and `port_id`↔`fixed_ip`),
  so per-attribute placement reads more naturally than two separate
  resource-level validators.

The runtime "must set one of `network_id` or `port_id`" check from the SDKv2
`Create` body was preserved as a `Create`-time diagnostic. It could be
expressed declaratively via
`resourcevalidator.AtLeastOneOf(path.MatchRoot("network_id"), path.MatchRoot("port_id"))`
(implementing `ResourceWithConfigValidators`), but doing so would change
behaviour subtly: validators run before `Configure`, and the existing error
message is created in `Create`. Left as-is to keep the migration a pure
refactor — flagging this as a possible follow-up improvement.

## Other field-by-field translations

| SDKv2                                     | Framework                                                                                          |
|-------------------------------------------|----------------------------------------------------------------------------------------------------|
| `Type: schema.TypeString`                 | `schema.StringAttribute{}`                                                                         |
| `ForceNew: true`                          | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`                       |
| `Computed: true` (on `Optional+Computed`) | `Computed: true` plus `stringplanmodifier.UseStateForUnknown()` to keep stable values across plans |
| `Timeouts.Create/Delete`                  | `timeouts.Attributes(ctx, timeouts.Opts{Create: true, Delete: true})` + `timeouts.Value` field     |
| `Importer.StateContext: ImportStatePassthroughContext` | `ResourceWithImportState.ImportState` calling `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |

A new `id` attribute was added to the schema (the framework requires it as a
typed attribute; SDKv2 derived it implicitly from `d.Id()`).

## CRUD-method translations

- `CreateContext` / `ReadContext` / `DeleteContext` → `Create` / `Read` /
  `Delete` methods on the resource type.
- `d.Get("instance_id").(string)` → `plan.InstanceID.ValueString()` after
  `req.Plan.Get(ctx, &plan)`.
- `d.GetOk("port_id")` (truthy iff non-zero) → `!plan.PortID.IsNull() && !plan.PortID.IsUnknown()` (and a non-empty check where it matters, e.g. `fixed_ip`).
- `d.SetId(id)` → `plan.ID = types.StringValue(id); resp.State.Set(ctx, &plan)`.
- `d.Timeout(schema.TimeoutCreate)` → `plan.Timeouts.Create(ctx, 10*time.Minute)`.
- `diag.Errorf(...)` / `diag.FromErr(err)` → `resp.Diagnostics.AddError(summary, err.Error())` followed by an early return.
- 404 handling in `Read`: SDKv2 `CheckDeleted` (which calls `d.SetId("")`) →
  `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` check followed by
  `resp.State.RemoveResource(ctx)`. The `CheckDeleted` helper takes a
  `*schema.ResourceData`, so we can't reuse it directly from a framework
  resource; the equivalent inline check is small.
- An `Update` method is implemented as a passthrough. Every user-visible
  attribute is `RequiresReplace`, so the framework will replace rather than
  update — but `resource.Resource` requires the method to exist.

## Region helper

The SDKv2 `GetRegion(d, config)` helper takes a `*schema.ResourceData`, so we
can't call it from a framework resource. A small `regionFromPlan` method on
the resource replicates the same logic against the typed model (use the
attribute if set, otherwise fall back to `r.config.Region`).

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- `testAccProvider.Meta().(*Config)` (which only works for the SDKv2 provider)
  → `testAccFrameworkConfig()`, a small helper expected to live alongside the
  framework provider factories.
- The HCL bodies are unchanged; ConflictsWith semantics produce the same
  HCL-time errors, so the existing test cases exercise the same surface.

Both `testAccProtoV6ProviderFactories` and `testAccFrameworkConfig` are
referenced but not defined in this file — they belong in the provider-level
test scaffolding (e.g., `provider_test.go`) and require the rest of the
provider to be served via the framework (or via `terraform-plugin-mux`)
before these tests can actually run.

## Residual SDKv2 import — `helper/retry`

The migrated resource still imports
`github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry` because the
`computeInterfaceAttachV2AttachFunc` / `computeInterfaceAttachV2DetachFunc`
helpers in `openstack/compute_interface_attach_v2.go` return
`retry.StateRefreshFunc` and the `Create`/`Delete` paths use
`retry.StateChangeConf{...}.WaitForStateContext(ctx)`.

`helper/retry` has no direct framework replacement — it's a generic polling
loop that doesn't depend on `*schema.ResourceData`, so it works fine as-is
from a framework resource. Migrating away from it would mean rewriting the
poll loop (e.g., `wait.PollUntilContextTimeout` from `k8s.io/apimachinery`,
or a hand-rolled `time.Ticker` loop) and updating the helper file, which is
a separate, larger refactor across many resources in this provider.

This means the skill's negative verification gate ("the migrated file may
not import `terraform-plugin-sdk/v2`") will be tripped by the `helper/retry`
import. Flagging this explicitly so it isn't mistaken for an oversight; it
is a known residual that requires the helper-file refactor.

## Out of scope / follow-ups

This is a single-resource migration; the rest of the provider is still on
SDKv2. Before this resource is testable end-to-end, the provider needs one
of:

1. Full provider migration to the framework, and registration of
   `NewComputeInterfaceAttachV2Resource` in the framework provider's
   `Resources` slice.
2. Or, `terraform-plugin-mux` to expose this single framework resource
   alongside the SDKv2 ones — explicitly out of scope per the skill's "don't
   introduce mux" guidance, so option 1 is preferred.

The skill's TDD step 7 (red-then-green) is therefore not fully exercisable
inside this single-file migration; the test file is migrated to the framework
idiom, but actually running it red→green requires the provider to serve the
framework resource.

## Files

- `migrated/resource_openstack_compute_interface_attach_v2.go` — full migrated resource.
- `migrated/resource_openstack_compute_interface_attach_v2_test.go` — full migrated tests.
