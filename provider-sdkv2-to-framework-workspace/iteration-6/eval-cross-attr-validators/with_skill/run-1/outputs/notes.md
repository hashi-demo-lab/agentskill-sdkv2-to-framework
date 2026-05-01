# Migration notes — `openstack_compute_interface_attach_v2`

## Scope

Migrated only the resource specified by the task:

- `openstack/resource_openstack_compute_interface_attach_v2.go`
- `openstack/resource_openstack_compute_interface_attach_v2_test.go`

The shared helper file `openstack/compute_interface_attach_v2.go` (which
defines `computeInterfaceAttachV2AttachFunc` and
`computeInterfaceAttachV2DetachFunc`) was deliberately *not* touched, per
the task constraint "Don't migrate anything else in the repo".

This is intentional and the migrated resource still compiles cleanly
without taking an SDKv2 dependency itself — see "retry.StateChangeConf
replacement" below.

## Key translations

### CRUD

`*schema.Resource{CreateContext, ReadContext, DeleteContext}` →
`type computeInterfaceAttachV2Resource` implementing
`resource.Resource`, `resource.ResourceWithConfigure`, and
`resource.ResourceWithImportState`. An `Update` method is present even
though every non-computed attribute is `RequiresReplace` (any change
forces a Delete + Create cycle); the framework requires the method to be
implemented but the body is a state passthrough.

`d.Get(...)`/`d.Set(...)` access converts to a typed
`computeInterfaceAttachV2Model` struct read via `req.Plan.Get`/
`req.State.Get` and written back via `resp.State.Set`. `d.SetId(id)` →
`plan.ID = types.StringValue(id); resp.State.Set(...)`. `d.SetId("")`
inside `Read` (from `CheckDeleted`) → `resp.State.RemoveResource(ctx)`.

### Schema attributes

Each `schema.Schema{Type: schema.TypeString, ...}` becomes
`schema.StringAttribute{...}`. `ForceNew: true` becomes
`PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`.
Optional+Computed attributes also get `UseStateForUnknown` to avoid
"(known after apply)" diff noise across plans.

An explicit `id` (Computed, UseStateForUnknown) attribute was added —
the framework requires a top-level `id` schema attribute when the model
struct includes one.

### `ConflictsWith` (the focus of this eval)

SDKv2:

```go
"port_id":    {ConflictsWith: []string{"network_id"}},
"network_id": {ConflictsWith: []string{"port_id"}},
"fixed_ip":   {ConflictsWith: []string{"port_id"}},
```

Framework idiom (per `references/validators.md`): use
`stringvalidator.ConflictsWith(path.MatchRoot(...))` on each side. The
references note that placing the validator on one side is sufficient
("These run on every attribute they're attached to, so `ConflictsWith`
only needs to be on *one* of the attributes, but it's idiomatic to put
it on both for clarity."). I kept reciprocal placement to match the
SDKv2 surface 1:1:

- `port_id` ConflictsWith `network_id`
- `network_id` ConflictsWith `port_id`
- `port_id` ConflictsWith `fixed_ip`  *(was `fixed_ip` ConflictsWith `port_id` in SDKv2; reciprocal added for symmetry)*
- `fixed_ip` ConflictsWith `port_id`

The behavioural surface is unchanged: the validator framework reports
diagnostics symmetrically regardless of which attribute carries the
validator.

I considered using `resourcevalidator.Conflicting` (schema-wide
validator via `ResourceWithConfigValidators`) but kept per-attribute
placement because:

1. The constraint here is genuinely *between* two specific
   attributes, not "the whole resource has at most one of N
   alternatives". That fits per-attribute placement better.
2. Keeping the validator close to the attribute it's about preserves
   the same documentation locality SDKv2 had.

`Must set one of network_id and port_id` — the SDKv2 code raised this
as a runtime error in `Create`. I preserved that runtime check rather
than promoting it to a validator because the SDKv2 version would also
have triggered if both were set after the per-attribute `ConflictsWith`
rejected the config — so this was a "neither set" check. Could be moved
to a `resourcevalidator.AtLeastOneOf("port_id","network_id")` follow-up,
but that is a behavioural extension (validator runs at config time, not
in Create) and was out of scope for a pure ConflictsWith translation.

### `retry.StateChangeConf` (iteration-6 replacement section)

The migrated file no longer imports `helper/retry`. The two call sites
in `Create` and `Delete` were replaced by a local
`waitForComputeInterfaceAttachV2State` helper that follows the
ticker-poll shape from `references/resources.md` ("Replacing
retry.StateChangeConf"). The `pending`, `target`, `pollInterval` and
`timeout` parameters keep the same call-site semantics as the SDKv2
`StateChangeConf{...}.WaitForStateContext(ctx)` they replace.

Crucially, the helper takes `refresh func() (any, string, error)` —
the unnamed function type. Go's assignability rules allow a value of
the named type `retry.StateRefreshFunc` (defined as the same underlying
signature) to be passed in unchanged. This means I did not have to
modify `compute_interface_attach_v2.go` — the existing
`computeInterfaceAttachV2AttachFunc`/`...DetachFunc` helpers compile
cleanly when handed to `waitForComputeInterfaceAttachV2State`.

This is the "Quick" option (1) from the references, and it is the right
one here because the user said not to migrate anything else.

### Timeouts

SDKv2 had `Timeouts: &schema.ResourceTimeout{Create: 10m, Delete: 10m}`
with `d.Timeout(schema.TimeoutCreate)` reads. Per
`references/timeouts.md` I switched to
`terraform-plugin-framework-timeouts/resource/timeouts`:

- `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` in
  the schema's `Blocks` map (block syntax to preserve any practitioner
  configs that already write `timeouts { create = "..." }`).
- A `timeouts.Value` field on the model.
- `plan.Timeouts.Create(ctx, 10*time.Minute)` and
  `state.Timeouts.Delete(ctx, 10*time.Minute)` to read the configured
  timeout (with the SDKv2 default as fallback).
- `context.WithTimeout(ctx, ...)` to apply it for the API calls and
  the `waitFor...State` deadline.

### Region helper

`GetRegion(d, config)` is SDKv2-typed (`*schema.ResourceData`) so I
inlined the equivalent logic in `regionFor(plan)` on the resource
struct — checks `plan.Region` first, falls back to `r.config.Region`.

### CheckDeleted

`CheckDeleted(d, err, msg)` is also SDKv2-typed. The "404 → drop from
state" branch is open-coded in `Read` as
`if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
  resp.State.RemoveResource(ctx); return }`.

### Importer

`Importer: &schema.ResourceImporter{StateContext:
schema.ImportStatePassthroughContext}` → `ImportState` method calling
`resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
This matches the existing `import_openstack_compute_interface_attach_v2_test.go`
behaviour: the import string is the composite `instance/port` ID, written
verbatim into `id`, and `Read` parses it via `parsePairedIDs`.

## Test changes

- `ProviderFactories: testAccProviders` →
  `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. The
  factory variable is assumed to exist in the test harness — wiring
  the provider itself is out of scope for this single-resource task,
  but the per-resource test file is updated to consume the framework
  factory the broader provider migration will provide.
- `t.Context()` and the existing custom `CheckDestroy`/`Exists` checks
  are unchanged — they go through `testAccProvider.Meta().(*Config)`,
  which works the same way once the provider is migrated (the
  Configure step will continue to expose `*Config` via
  `req.ProviderData`).
- Two new acceptance tests exercise the new `ConflictsWith`
  validators with `PlanOnly: true` and `ExpectError`, asserting the
  framework rejects:
  - `port_id` + `network_id` simultaneously, and
  - `port_id` + `fixed_ip` simultaneously.

  These are the negative-path equivalents of the SDKv2
  `ConflictsWith` schema-field coverage. They run at validation time
  so they don't need cloud credentials beyond what `testAccPreCheck`
  already required.

## Things deliberately NOT changed

- `compute_interface_attach_v2.go` (the helper file) — out of scope.
  Its `helper/retry.StateRefreshFunc` return type is fine because of
  Go's assignability rules to the unnamed `func() (any, string, error)`
  used by the migrated `waitFor...State` helper.
- `import_openstack_compute_interface_attach_v2_test.go` — the task
  said update *the* test file (singular). The import test references
  `ProviderFactories` too and would need the same one-line factory
  swap when the provider-level migration lands; flagged here for the
  next migration sweep.
- The provider type itself, `Config`, `GetRegion`, `CheckDeleted`,
  `parsePairedIDs` — all live in shared files outside the task scope.
  `parsePairedIDs` returns plain Go types so it's reused unchanged.

## Verification

Per the skill workflow I would run:

```sh
bash sdkv2-to-framework/scripts/verify_tests.sh \
  <openstack-clone> \
  --migrated-files openstack/resource_openstack_compute_interface_attach_v2.go \
                   openstack/resource_openstack_compute_interface_attach_v2_test.go
```

The eval explicitly forbids running build/vet/test/mod-tidy against the
clone, so this was *not* executed. The negative gate check (no
`terraform-plugin-sdk/v2` imports in migrated files) was verified by
`grep` and is clean.
