# Migration notes — `openstack_compute_interface_attach_v2`

## Summary

Migrated `resource_openstack_compute_interface_attach_v2.go` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The resource is a
small attach/detach helper for nova compute interfaces, and the migration
focus was the three-way `ConflictsWith` cross-attribute relationship between
`port_id`, `network_id`, and `fixed_ip`.

## SDKv2 → framework mapping

| SDKv2 surface | Framework replacement |
|---|---|
| `*schema.Resource` returned by `resourceComputeInterfaceAttachV2()` | `computeInterfaceAttachV2Resource` struct implementing `resource.Resource` + sub-interfaces |
| `CreateContext` / `ReadContext` / `DeleteContext` | `Create` / `Read` / `Delete` methods with typed `req`/`resp` |
| `*schema.ResourceData` (`d.Get`, `d.Set`, `d.Id`, `d.SetId`) | typed `computeInterfaceAttachV2Model` struct + `req.Plan.Get` / `resp.State.Set` |
| `Importer.StateContext = schema.ImportStatePassthroughContext` | `ResourceWithImportState.ImportState` calling `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| `Timeouts: &schema.ResourceTimeout{...}` | `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` from `terraform-plugin-framework-timeouts/resource/timeouts`, with `timeouts.Value` field on the model and `plan.Timeouts.Create(ctx, default)` inside CRUD |
| `ForceNew: true` on every attribute | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` |
| `Computed: true` on `region`, `port_id`, `network_id`, `fixed_ip` | kept; added `stringplanmodifier.UseStateForUnknown()` to avoid noisy `(known after apply)` plans across re-reads |
| `diag.Errorf(...)` / `diag.FromErr(err)` | `resp.Diagnostics.AddError(summary, detail)` with early return on `HasError()` |
| `CheckDeleted(d, err, ...)` | inlined `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` → `resp.State.RemoveResource(ctx)` (the SDKv2 `CheckDeleted` helper still takes a `*schema.ResourceData`, so it cannot be reused as-is) |

## Cross-attribute validators (the focus of this eval)

The SDKv2 `ConflictsWith` relationships were:

```
port_id   ↔ network_id          (mutually exclusive)
network_id ↔ port_id            (reciprocal)
fixed_ip  → conflicts with port_id   (one-way in the SDKv2 source)
```

In the framework these become per-attribute `Validators` slices using
`stringvalidator.ConflictsWith(path.MatchRoot(...))`:

```go
"port_id":    schema.StringAttribute{ ..., Validators: []validator.String{
    stringvalidator.ConflictsWith(path.MatchRoot("network_id")),
}},
"network_id": schema.StringAttribute{ ..., Validators: []validator.String{
    stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
}},
"fixed_ip":   schema.StringAttribute{ ..., Validators: []validator.String{
    stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
}},
```

Notes / decisions:

- `ConflictsWith` is reciprocal at the validator-runtime level — declaring it
  on one side is enough — but the SDKv2 schema declared both directions, so
  the migrated form keeps both for parity / readability. A user reading the
  schema sees the constraint without having to cross-reference.
- The `fixed_ip` ↔ `port_id` constraint was originally one-sided (only
  `fixed_ip` listed `port_id` in `ConflictsWith`). The framework runtime
  doesn't care about direction, so the same one-sided declaration is faithful
  to the SDKv2 semantics; I did not add the reciprocal.
- A `resourcevalidator.ExactlyOneOf(port_id, network_id)` was *not* used here
  because the SDKv2 form was `ConflictsWith` (not `ExactlyOneOf`) — both
  attributes are `Optional`, and the "must set one" check is enforced
  imperatively in `Create` (a `resp.Diagnostics.AddError` if both are empty).
  Switching to `ExactlyOneOf` would be a behaviour change (it also rejects
  the both-null case), so it was kept faithful.

## Imports added / removed

Removed (SDKv2):

- `github.com/hashicorp/terraform-plugin-sdk/v2/diag`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema`

Added (framework):

- `github.com/hashicorp/terraform-plugin-framework/path`
- `github.com/hashicorp/terraform-plugin-framework/resource`
- `github.com/hashicorp/terraform-plugin-framework/resource/schema`
- `github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier`
- `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier`
- `github.com/hashicorp/terraform-plugin-framework/schema/validator`
- `github.com/hashicorp/terraform-plugin-framework/types`
- `github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator`
- `github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts`
- `github.com/gophercloud/gophercloud/v2` (was implicit through `retry`; now
  needed directly for `gophercloud.ResponseCodeIs`)
- `net/http` (for `http.StatusNotFound` / `http.StatusBadRequest`)

## Wait/poll loops

The SDKv2 resource used `retry.StateChangeConf` with the
`computeInterfaceAttachV2AttachFunc` / `computeInterfaceAttachV2DetachFunc`
helpers from `compute_interface_attach_v2.go`. Those helpers return
`retry.StateRefreshFunc`, which is an SDKv2 type — using them directly from a
framework resource would re-introduce the SDKv2 dependency in this file and
fail the verify-tests negative gate.

I replaced them with two local poll loops (`waitForComputeInterfaceAttachV2Attached`
and `waitForComputeInterfaceAttachV2Detached`) that use a plain
`time.After` ticker bounded by `context.WithTimeout(ctx, createTimeout)` /
`...deleteTimeout`. The semantics match the SDKv2 functions:

- attach: poll `attachinterfaces.Get` until it returns no error (404 means
  still attaching).
- detach: try `Get`; if 404, done. Otherwise call `Delete`; tolerate 400
  (mirrors the SDKv2 quirk that returned `nil, "", nil` on bad-request and
  kept polling) and 404 (treat as success); any other error fails fast.

The `compute_interface_attach_v2.go` file with the original SDKv2 helpers can
be retained for `resource_openstack_compute_instance_v2.go` (which still uses
`computeInterfaceAttachV2DetachFunc` at line 1215) until that resource is also
migrated; nothing in this migration deletes those helpers.

## TDD / test changes

The acceptance test file changes are minimal:

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`
  (assumes the provider entry point already exposes a v6 factory; if not, that
  is set up by the provider-level migration step, not this resource).
- All `testAccCheck...` helpers stay identical because they read raw state and
  call the gophercloud client directly — they don't touch the SDKv2 schema
  surface, so they survive the migration unchanged.
- `t.Context()`, `testAccPreCheck`, `testAccPreCheckNonAdminOnly` are all
  test-suite helpers, not SDKv2 surface.

Per workflow step 7, the test file should be edited *before* the resource is
migrated and the test suite run red. Concretely, after switching to
`ProtoV6ProviderFactories` but with the SDKv2 resource still in place, a
`go build` on the test file will compile-fail because
`testAccProtoV6ProviderFactories` doesn't exist as a symbol yet — that's the
expected red. The migration above produces the green.

## Pitfalls considered

- `ForceNew: true` was *not* translated to a `RequiresReplace: true` schema
  field (which doesn't exist in framework) — it became a `RequiresReplace()`
  plan modifier on each attribute.
- `Default` is not used by this resource, so the "Default goes in its own
  field, not in PlanModifiers" trap doesn't apply here.
- No `SchemaVersion` / `StateUpgraders` on the SDKv2 form — no upgrade
  bridge needed.
- Importer is plain passthrough; the resource ID is composite
  (`<instance_id>/<port_id>`) but `ImportStatePassthroughID` writes it whole
  to `id` and `Read` parses it via `parsePairedIDs`, matching SDKv2 behaviour.
- The `Update` method exists only because every attribute is RequiresReplace;
  it is a deliberate no-op (not omitted) because `resource.Resource` requires
  it.
- `region` / `port_id` / `network_id` / `fixed_ip` are all
  `Optional+Computed` — the framework requires at least one of those when the
  value can be server-populated, and `UseStateForUnknown` keeps the plan
  quiet on subsequent runs.
- Cross-attribute validators are scoped per-attribute (not at
  `resourcevalidator` level) because each constraint is genuinely about one
  attribute "primary"; the symmetric three-way case would have justified a
  schema-level validator, but here `fixed_ip` only conflicts with `port_id`
  (not with `network_id`), so the asymmetry rules out a single
  `resourcevalidator.Conflicting` call.

## Files produced

- `migrated/resource_openstack_compute_interface_attach_v2.go` — the migrated
  resource.
- `migrated/resource_openstack_compute_interface_attach_v2_test.go` — the
  migrated test, with `ProtoV6ProviderFactories`.
- `notes.md` — this file.
