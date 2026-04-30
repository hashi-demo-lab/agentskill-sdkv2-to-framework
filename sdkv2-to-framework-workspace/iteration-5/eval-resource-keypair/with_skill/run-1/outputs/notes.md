# notes — openstack_compute_keypair_v2 SDKv2 → terraform-plugin-framework

## Scope

Single-resource migration:

- `resource_openstack_compute_keypair_v2.go` (resource)
- `resource_openstack_compute_keypair_v2_test.go` (test file)

Per the task brief, no other files in the provider repo were touched. The
provider type itself, helpers (`Config`, `Provider`, `MapValueSpecs`,
`GetRegion`, `CheckDeleted`, `ComputeV2Client`), the data-source variant of
this resource, and the rest of the test scaffolding remain SDKv2.

## Pre-flight observations

Schema audit of the source file:

| Attribute     | SDKv2                             | Framework                                             | Notes                                       |
|---------------|-----------------------------------|-------------------------------------------------------|---------------------------------------------|
| `id`          | implicit                          | `StringAttribute{Computed, UseStateForUnknown}`       | declared explicitly to wire UseStateForUnknown |
| `region`      | `TypeString, Optional, Computed, ForceNew` | `StringAttribute{Optional, Computed}` + `RequiresReplaceIfConfigured`, `UseStateForUnknown` | computed-fallback to provider Region |
| `name`        | `TypeString, Required, ForceNew`  | `StringAttribute{Required}` + `RequiresReplace`       | id-bearing attribute                        |
| `public_key`  | `TypeString, Optional, Computed, ForceNew` | `StringAttribute{Optional, Computed}` + `RequiresReplaceIfConfigured`, `UseStateForUnknown` | server-derived when omitted |
| `value_specs` | `TypeMap, Optional, ForceNew`     | `MapAttribute{Optional, ElementType: types.StringType}` + `mapplanmodifier.RequiresReplace()` | unchanged shape |
| `private_key` | `TypeString, Computed, Sensitive` | `StringAttribute{Computed, Sensitive, UseStateForUnknown}` | only set on create  |
| `fingerprint` | `TypeString, Computed`            | `StringAttribute{Computed, UseStateForUnknown}`       | server-derived              |
| `user_id`     | `TypeString, Optional, Computed, ForceNew` | `StringAttribute{Optional, Computed}` + `RequiresReplaceIfConfigured`, `UseStateForUnknown` | controls microversion bump |

### Block decision

No `MaxItems: 1 + nested Elem` patterns; nothing to convert to a single nested
attribute.

### State upgrade

`SchemaVersion` is unset (defaults to 0) and there are no upgraders — nothing
to port.

### Import shape

Importer is `schema.ImportStatePassthroughContext` — straight passthrough on
`id`. Migrated to `resource.ImportStatePassthroughID(ctx, path.Root("id"),
req, resp)` per `references/import.md`.

## Mechanical conversions applied

- `*schema.Resource` builder → resource type `computeKeypairV2Resource` with
  `resource.Resource`, `ResourceWithConfigure`, `ResourceWithImportState`
  interface assertions.
- `CreateContext`/`ReadContext`/`DeleteContext` (`d *schema.ResourceData`,
  `meta any`) → typed `Create`/`Read`/`Delete` methods on the resource
  receiver, with `req.Plan.Get` / `resp.State.Set` against
  `computeKeypairV2Model`.
- Added a no-op `Update` method (the framework's `resource.Resource` interface
  requires it; every configurable attribute is `RequiresReplace` so the body
  just persists the plan to keep the framework happy if it is ever called).
- `diag.Errorf(...)` / `diag.FromErr(...)` → `resp.Diagnostics.AddError(...)`
  with summary + detail.
- `d.Get("foo").(string)` → typed access on the model with explicit
  `IsNull/IsUnknown` guards before `ValueString()` (per
  `references/state-and-types.md`).
- `d.Set("name", kp.Name)` → field assignment on the model + `resp.State.Set`.
- `d.SetId(kp.Name)` → `plan.ID = types.StringValue(kp.Name)`.
- `CheckDeleted` (which mutates `*schema.ResourceData`) →
  `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` + `resp.State
  .RemoveResource(ctx)` in `Read`. In `Delete` a 404 is treated as already
  gone (no diagnostic).
- `MapValueSpecs(d)` (SDKv2-bound) → inline `plan.ValueSpecs.ElementsAs(ctx,
  &valueSpecs, false)` using the existing `ComputeKeyPairV2CreateOpts` /
  `keypairs.CreateOpts` types verbatim. This avoided introducing a parallel
  helper while the rest of the provider is still SDKv2.
- `ForceNew: true` → `RequiresReplace`/`RequiresReplaceIfConfigured` plan
  modifiers per `references/plan-modifiers.md`. Optional+Computed attributes
  use `RequiresReplaceIfConfigured` so server-derived values don't trigger a
  spurious replace when the practitioner omits the attribute.
- `UseStateForUnknown` added to every `Computed` attribute that doesn't change
  outside of a replace, to keep plans stable.
- Importer wired to `ImportStatePassthroughID` with `path.Root("id")`.

## Configure plumbing

The SDKv2 provider passed `*Config` via `meta.(*Config)`. The framework
resource consumes the same value through `req.ProviderData`. The provider
itself remains SDKv2 in this partial migration, so for the test/build to
actually run, the wider provider would need to:

1. Be served via `terraform-plugin-mux` (`tf6muxserver` / `tf5to6server`)
   alongside the framework resource.
2. Pass `*Config` to the framework provider's `Configure` so `ProviderData`
   is populated.

This skill's scope is the resource migration; the muxing/scaffolding work is
out of scope for the task as posed.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories:
  testAccProtoV6ProviderFactories` (per `references/testing.md`). The
  `testAccProtoV6ProviderFactories` symbol does not yet exist in the repo —
  it would be defined in `provider_test.go` once muxing is wired up; left as
  an explicit reference so reviewers can see the missing infrastructure.
- Added an `ImportState`/`ImportStateVerify` step to the `_basic` test, with
  `ImportStateVerifyIgnore: []string{"private_key"}` (private key is only
  emitted on create — re-importing would never recover it). This exercises
  the new `ImportState` method.
- `testAccCheckComputeV2KeypairDestroy` and `testAccCheckComputeV2KeypairExists`
  unchanged — both go through `keypairs.Get` directly, no SDKv2 plumbing.

## Verification

Per task rules I have **not** run `go build`/`go vet`/`go test`/`go mod tidy`
against the read-only openstack clone. The verification gate
(`scripts/verify_tests.sh`) is the next step in a real migration; in this
sandbox the migrated files live under the workspace `migrated/` directory
only.

Hand-checks performed:

- All `terraform-plugin-sdk/v2` imports removed from
  `resource_openstack_compute_keypair_v2.go`.
- Test file no longer imports `terraform-plugin-sdk/v2/...`.
- All `ForceNew: true` translations use the per-kind
  `*planmodifier.RequiresReplace*` helpers (none mistakenly placed on
  `Default`, none using a phantom `RequiresReplace: true` field).
- All `Computed` attributes carry `UseStateForUnknown` to avoid noisy
  `(known after apply)` plans.
- `private_key`'s `Sensitive: true` retained.

## Open follow-ups (not done — out of scope per the task)

- Mux scaffolding so the framework resource and the rest of the SDKv2
  provider can be served together (`provider_test.go` would gain the
  `testAccProtoV6ProviderFactories` symbol).
- Migrate the data-source variant (`data_source_openstack_compute_keypair_v2.go`)
  — it shares the same schema and would mirror this migration cleanly.
- Replace the inline `valueSpecs` extraction with a shared framework-friendly
  helper once a second framework resource appears (analogous to the SDKv2
  `MapValueSpecs`).
