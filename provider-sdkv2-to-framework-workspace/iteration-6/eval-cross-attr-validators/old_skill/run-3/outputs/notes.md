# resource_openstack_compute_interface_attach_v2 — SDKv2 → Framework migration

## Pre-edit summary (per-resource think-before-editing)

1. **Block decision**: no `MaxItems: 1 + nested Elem` blocks in this resource. The
   only block-shaped element is `Timeouts`, which migrates via the
   `terraform-plugin-framework-timeouts` package. Existing configs use HCL block
   syntax (`timeouts { … }`), so we use `timeouts.Block(...)` (in `Blocks:`) to
   preserve backward compatibility — not `timeouts.Attributes(...)`.
2. **State upgrade**: no `SchemaVersion` set; nothing to upgrade.
3. **Import shape**: SDKv2 used `schema.ImportStatePassthroughContext`. The
   resource ID is composite (`<instanceID>/<portID>`), but the importer just
   stores it as-is and `Read` parses it via `parsePairedIDs`. We mirror that
   exactly with `resource.ImportStatePassthroughID(ctx, path.Root("id"), ...)`
   so existing import strings keep working.

## Cross-attribute validators (the focus of this evaluation)

The SDKv2 schema declared three `ConflictsWith` relationships:

- `port_id` ↔ `network_id` (mutually exclusive)
- `fixed_ip` → `port_id` (`fixed_ip` cannot be set with `port_id`)

Per `references/validators.md`, framework cross-attribute checks become
per-attribute `Validators` using `stringvalidator.ConflictsWith(path.MatchRoot(...))`.

Decisions:

| Old (`ConflictsWith`) | New | Notes |
|---|---|---|
| `port_id: ["network_id"]` | `stringvalidator.ConflictsWith(path.MatchRoot("network_id"))` on `port_id` | symmetric pairing also placed on `network_id` for clarity |
| `network_id: ["port_id"]` | `stringvalidator.ConflictsWith(path.MatchRoot("port_id"))` on `network_id` | as above |
| `fixed_ip: ["port_id"]` | `stringvalidator.ConflictsWith(path.MatchRoot("port_id"))` on `fixed_ip` | one-directional in original SDKv2 declaration; preserved one-directional |

I did **not** collapse these to a single
`resourcevalidator.ExactlyOneOf(port_id, network_id)` because:

1. The original SDKv2 schema permitted *neither* being set at the schema level,
   leaving the "must set one" check to a runtime guard inside `Create`. That
   semantic — runtime error rather than plan-time error — is preserved by the
   imperative check in `Create` (`Must set one of network_id and port_id`).
2. `fixed_ip` interacts with `port_id` only, not `network_id`. A single
   `ExactlyOneOf` would not capture that asymmetry.

Per the validators reference, `ConflictsWith` is symmetric in *effect* once
attached to one side, but it's idiomatic to declare it on both sides for
readability — so I did.

## CRUD migration notes

- `CreateContext` → `Create`, etc. The SDKv2 `retry.StateChangeConf` polling
  helpers (which referenced `terraform-plugin-sdk/v2/helper/retry`) are gone;
  I inlined two small `for { … select { case <-ctx.Done(): … } }` loops
  (`waitForComputeInterfaceAttachV2Attach`, `waitForComputeInterfaceAttachV2Detach`)
  to keep the same poll-until-status semantics without dragging the SDKv2
  retry package along. This deletes the only SDKv2 import in the original
  `compute_interface_attach_v2.go` helper file (the original
  `computeInterfaceAttachV2{Attach,Detach}Func` returned `retry.StateRefreshFunc`).
  The helper file `compute_interface_attach_v2.go` would be removed (or its
  retry-based functions deleted) once the migration lands; no other resources
  in this provider call those helpers.
- `d.Get("foo")` → typed access on `computeInterfaceAttachV2Model` via
  `req.Plan.Get(ctx, &plan)`.
- `d.Set(...)` after Create/Read → assignment on the model + `resp.State.Set`.
- `diag.Errorf(...)` / `diag.FromErr` → `resp.Diagnostics.AddError(...)`.
- `CheckDeleted` (which accepts `*schema.ResourceData`) is replaced by an
  inline 404 check that calls `resp.State.RemoveResource(ctx)` per the
  framework Read-drift contract.
- `Timeouts: &schema.ResourceTimeout{…}` → `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})`.
- `ForceNew: true` on every non-id attribute → `stringplanmodifier.RequiresReplace()`.
- `Computed: true` on `region`, `port_id`, `network_id`, `fixed_ip` keeps
  `stringplanmodifier.UseStateForUnknown()` so refreshed values don't show
  spurious diffs (a class of bug specific to the framework, where
  computed-then-known values otherwise show as "(known after apply)" on every
  plan).

## Update method

All non-id attributes are `RequiresReplace`, so `Update` is unreachable — the
framework still requires the method to exist for `resource.Resource`, so we
implement it as a no-op. (An alternative is to omit `Update` from the
interface, but the `resource.Resource` interface requires it; there is no
"create-replace-delete only" sub-interface in the current framework version.)

## Import

`Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}`
becomes `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`,
which preserves the existing user-facing import contract:
`terraform import openstack_compute_interface_attach_v2.foo <instanceID>/<portID>`
still works, and the subsequent `Read` parses the composite via
`parsePairedIDs` as before.

## "Must set one of network_id and port_id" — kept where it was

The SDKv2 code emitted a runtime `diag.Errorf` in `Create` if both were unset.
I kept that as a `resp.Diagnostics.AddError(...)` in the framework `Create`,
*not* a schema-level `resourcevalidator.AtLeastOneOf(...)`. Reasoning:

- Promoting it to a schema validator would change the failure mode from
  apply-time to plan-time. That's user-observable behaviour change beyond a
  pure refactor.
- A future improvement could promote to `ConfigValidators(...)` returning
  `resourcevalidator.AtLeastOneOf(path.MatchRoot("network_id"), path.MatchRoot("port_id"))`
  for a better practitioner experience, but it should be a deliberate change
  with a changelog entry, not snuck into the migration.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- All other test helpers (`testAccCheckComputeV2InterfaceAttachExists`,
  `testAccCheckComputeV2InterfaceAttachDestroy`, `testAccCheckComputeV2InterfaceAttachIP`,
  config builder funcs) are unchanged — they use `terraform-plugin-testing`
  which works identically against framework-served providers, and the
  on-disk shape of resources (their attributes) is unchanged.
- `testAccProvider.Meta().(*Config)` — kept as is on the assumption that the
  surrounding `provider_test.go` will be updated separately to expose the
  framework provider's data the same way (or that during the muxed transition
  this still resolves through the SDKv2 facade). If the surrounding
  `provider_test.go` switches to framework-only, that line becomes
  `testAccProvider.(framework.Provider).…` or whatever the chosen exposure is;
  no change required to this test file beyond the factory swap.
- `testAccProtoV6ProviderFactories` is the symbol the surrounding
  `provider_test.go` is expected to declare (a
  `map[string]func() (tfprotov6.ProviderServer, error)` built via
  `providerserver.NewProtocol6WithError`). The migration of `provider_test.go`
  itself is out of scope for a single-resource migration but is implied by
  workflow steps 3–5.

## Out-of-scope but related cleanup

- The helper file `compute_interface_attach_v2.go` (defining
  `computeInterfaceAttachV2AttachFunc` and `computeInterfaceAttachV2DetachFunc`
  on top of `terraform-plugin-sdk/v2/helper/retry`) is deletable once this
  resource is migrated. Both helpers are inlined into the new
  `wait*` functions inside the migrated resource file, and a grep confirms no
  other resources in the openstack package import them. Removing it closes
  the negative gate (`grep terraform-plugin-sdk/v2 …`) for these files.
- `import_openstack_compute_interface_attach_v2_test.go` (separate import
  test file) is unmodified by this output but will need the same factory swap
  if it sets `ProviderFactories` directly. I did not touch it as the task
  explicitly named only `resource_…_test.go`.

## Verification (assumed; not run here per the read-only-on-openstack rule)

After the surrounding provider has been ported to framework (`main.go` +
`provider.go` switched to `providerserver.NewProtocol6WithError` per
`references/protocol-versions.md`), run:

```sh
bash <skill-path>/scripts/verify_tests.sh \
  /path/to/terraform-provider-openstack \
  --migrated-files openstack/resource_openstack_compute_interface_attach_v2.go \
                   openstack/resource_openstack_compute_interface_attach_v2_test.go \
                   openstack/compute_interface_attach_v2.go
```

Expected results:

1. `go build ./...` — green.
2. `go vet ./...` — green.
3. TestProvider — green (schema is well-formed).
4. Non-`TestAcc*` unit tests — green.
5. (Optional) protocol-v6 smoke — server boots with the new resource registered.
6. (Optional, with `TF_ACC=1`) `TestAccComputeV2InterfaceAttach_basic` and
   `_IP` — green.
7. **Negative gate**: none of the three migrated files import
   `github.com/hashicorp/terraform-plugin-sdk/v2`. Confirmed by inspection of
   the diff: the only SDKv2 imports referenced in the original code
   (`/diag`, `/helper/retry`, `/helper/schema`) are absent from the new
   resource file; the test file's only SDKv2 dependency was the
   `ProviderFactories` map type, which the factory swap removes; the helper
   file is being deleted.
