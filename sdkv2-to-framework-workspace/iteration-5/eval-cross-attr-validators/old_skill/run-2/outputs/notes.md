# Migration notes — `openstack_compute_interface_attach_v2`

Migrated from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.
Resource has three attributes that use SDKv2 `ConflictsWith`; this is the
core focus of the migration.

## Pre-migration shape

- `*schema.Resource` with `CreateContext`, `ReadContext`, `DeleteContext`
  (no `UpdateContext` — every attribute is `ForceNew`).
- `Importer: schema.ImportStatePassthroughContext`.
- `Timeouts: { Create: 10m, Delete: 10m }`.
- Five user-facing attributes plus implicit `id`:
  - `region` — Optional/Computed/ForceNew
  - `port_id` — Optional/Computed/ForceNew, **`ConflictsWith: ["network_id"]`**
  - `network_id` — Optional/Computed/ForceNew, **`ConflictsWith: ["port_id"]`**
  - `instance_id` — Required/ForceNew
  - `fixed_ip` — Optional/Computed/ForceNew, **`ConflictsWith: ["port_id"]`**
- Composite ID `<instance_id>/<port_id>` parsed by the shared
  `parsePairedIDs` helper.
- Wait loops via `retry.StateChangeConf` for ATTACHED / DETACHED transitions
  (defined in `compute_interface_attach_v2.go`).

## Migration decisions

### `ConflictsWith` → `stringvalidator.ConflictsWith`
This was the central translation. SDKv2's schema-level `ConflictsWith` field
becomes a per-attribute validator from
`terraform-plugin-framework-validators/stringvalidator` paired with
`path.MatchRoot`:

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

Per the validators reference, the validator only needs to be on **one** side
of the pair to fire, but it is idiomatic to add it to both for symmetry and
discoverability. I kept the same shape SDKv2 had (declared on both sides).

`fixed_ip` only declares the conflict against `port_id` — it does *not*
conflict with `network_id` (you can attach a network with a chosen fixed IP).
The original schema's asymmetry is preserved.

#### Considered alternative — `ResourceWithConfigValidators`
Could have used `resourcevalidator.Conflicting(path.MatchRoot("port_id"),
path.MatchRoot("network_id"))` plus a second one for `fixed_ip` ↔ `port_id`,
hoisted to a `ConfigValidators()` method. Rejected because the per-attribute
form keeps the constraint colocated with the attribute (matches the SDKv2
shape practitioners and reviewers see) and the constraint is genuinely
about each attribute, not a whole-resource property. Per validators.md:
"Per-attribute placement is fine when one attribute is the 'primary'", which
applies here — `port_id` is the primary identifier and the other two carry
the conflict against it.

### `ForceNew` → `stringplanmodifier.RequiresReplace()`
Every attribute except `id` was `ForceNew`. Translated into
`PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`.
Computed attributes also get `UseStateForUnknown()` so plans aren't noisy
about `region`/`port_id`/`network_id`/`fixed_ip` on no-op reads.

### Computed `id`
Added an explicit `id` attribute (Computed + `UseStateForUnknown`) — SDKv2
auto-injected this; the framework requires it on the schema.

### Timeouts
`Timeouts: &schema.ResourceTimeout{Create: 10m, Delete: 10m}` becomes the
`timeouts` block from `terraform-plugin-framework-timeouts/resource/timeouts`
with `Opts{Create: true, Delete: true}`. Used `timeouts.Block(...)` (not
`Attributes(...)`) to preserve the `timeouts { ... }` HCL block syntax that
existing practitioner configs use — per timeouts.md, this is the right call
for migrations vs greenfield work.

### Importer
`schema.ImportStatePassthroughContext` →
`resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` on
`ImportState`. The composite-ID parsing happens in `Read` (where it always
did), not in `ImportState` — the import-string is `<instance>/<port>`, and
`Read` parses it via `parsePairedIDs`.

### CRUD methods
- `CreateContext` → `Create(ctx, req, resp)`. Reads `req.Plan` into a typed
  `computeInterfaceAttachV2Model`, computes the create timeout via
  `plan.Timeouts.Create(ctx, 10*time.Minute)`, wraps the context, and writes
  the final state via `resp.State.Set`.
- `ReadContext` → `Read`. Uses `req.State` to recover the composite ID,
  re-fetches via gophercloud, and on 404 calls `resp.State.RemoveResource`
  (the framework equivalent of `d.SetId("")`).
- `DeleteContext` → `Delete`, with the same timeout pattern.
- Added a no-op `Update` to satisfy the `resource.Resource` interface — the
  resource never actually updates because every attribute is RequiresReplace,
  but the method must exist.

### `retry.StateChangeConf` → in-line poller
`terraform-plugin-sdk/v2/helper/retry` is part of the SDKv2 surface and
must go (the negative gate flags any remaining import). Replaced with two
small in-file helpers `waitForComputeInterfaceAttachV2Attached` and
`waitForComputeInterfaceAttachV2Detached` that ticker-poll until the
context deadline expires. Behaviour matches the original ATTACHING/ATTACHED
and DETACHED state machines, including the 5-second min interval and the
HTTP 400 "busy, retry" handling on Detach.

### `log.Printf` → `tflog.Debug`
Replaced standard-library `log.Printf` with `tflog.Debug` from
`terraform-plugin-log/tflog`. Equivalent SDK-idiomatic logging that nests
under the framework's structured logger.

### `d.GetOk("region")` / `GetRegion`
The shared `GetRegion(d, config)` helper takes `*schema.ResourceData`, so
it can't be reused inside framework CRUD. Inlined the same logic: prefer
`plan.Region.ValueString()` if non-empty, else fall back to
`r.config.Region`.

### Configure
Added `Configure(ctx, req, resp)` to grab `*Config` from
`req.ProviderData`. Standard pattern from references/resources.md.

## Test file changes

- `ProviderFactories: testAccProviders` →
  `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. This assumes
  the broader provider has been (or will be) wired up to expose a v6
  factory map under that name, e.g. via `providerserver.NewProtocol6WithError`.
- `testAccProvider.Meta().(*Config)` calls are no longer reachable when the
  provider runs as framework. Replaced with a `testAccConfig()` helper call
  (assumed to live alongside the provider plumbing for framework tests). If
  that helper does not yet exist, the migration will need to add a thin
  shim that returns the same `*Config` instance the provider built during
  acceptance setup.
- Added two new acceptance tests that exercise the **framework** translation
  of `ConflictsWith`:
  - `TestAccComputeV2InterfaceAttach_conflictsPortNetwork` — both `port_id`
    and `network_id` set, expects an `ExpectError` with `PlanOnly: true`
    so the assertion fires at validation time without ever calling the API.
  - `TestAccComputeV2InterfaceAttach_conflictsFixedIPPort` — same pattern
    for the `fixed_ip` ↔ `port_id` pair.
  These two tests verify that the validators *actually* fire — without them
  a misnamed import or wrong path expression would silently pass. They are
  the workflow-step-7 "red-then-green" gate for the cross-attribute
  validator translation.

## Skipped / deferred

- The shared file `compute_interface_attach_v2.go` (the
  `computeInterfaceAttachV2AttachFunc` / `DetachFunc` helpers) still imports
  `terraform-plugin-sdk/v2/helper/retry`. The migrated resource no longer
  calls them (uses the new in-file `waitForComputeInterfaceAttachV2*`
  helpers instead). That file should be deleted in the broader migration —
  outside the scope of this single-resource migration.
- The shared `GetRegion` / `CheckDeleted` helpers in `util.go` still take
  `*schema.ResourceData`. Not refactored here; the migrated resource avoids
  them by inlining equivalent logic.
- Provider plumbing: registering `NewComputeInterfaceAttachV2Resource` in
  the framework provider's `Resources()` slice and removing the SDKv2
  registration is part of the broader migration (provider.md / step 6) and
  not in the scope of this single-resource exercise.

## Verification

Per the skill's verification gates, run:

```sh
bash <skill-path>/scripts/verify_tests.sh /path/to/openstack \
  --migrated-files \
    openstack/resource_openstack_compute_interface_attach_v2.go \
    openstack/resource_openstack_compute_interface_attach_v2_test.go
```

Expected: build / vet / TestProvider / unit tests pass; the negative gate
confirms neither migrated file imports `terraform-plugin-sdk/v2`. The
expanded acceptance tests for the conflict validators give a TF_ACC=1
red-then-green signal that the framework cross-attribute validator
translation is wired correctly.
