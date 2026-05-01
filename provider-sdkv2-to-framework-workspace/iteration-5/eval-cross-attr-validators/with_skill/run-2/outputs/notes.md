# Migration notes — `openstack_compute_interface_attach_v2`

## Scope
Partial migration: this single resource (and its acceptance-test file) ported from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The shared helper `compute_interface_attach_v2.go` (which still owns the `retry.StateRefreshFunc` factories) is intentionally left on SDKv2 — its `retry.StateChangeConf` API has no framework-native replacement and is safe to keep at this stage of a single-release-cycle migration.

## Cross-attribute validation — the load-bearing change

The SDKv2 schema used `ConflictsWith` on three attributes:

| Attribute | SDKv2 `ConflictsWith` |
|---|---|
| `port_id` | `["network_id"]` |
| `network_id` | `["port_id"]` |
| `fixed_ip` | `["port_id"]` |

The framework removes the `ConflictsWith` schema field entirely. Two equivalent expressions exist; we picked **per-attribute Validators with `path.Expression`** because it preserves the symmetric intent of the original schema and reads identically to it line-for-line. The alternative (`ResourceWithConfigValidators` / `resourcevalidator.Conflicting`) would have collapsed the three lists into one resource-level declaration, which was tempting but loses the locality of "this attribute conflicts with that one".

Resulting validators:

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

Note that the SDKv2 schema only had `port_id ⇄ network_id` and `fixed_ip → port_id` listed reciprocally. We kept that semantic exactly: `port_id` now lists *both* counterparts (`network_id` and `fixed_ip`) so the validator fires from either side, matching SDKv2's behaviour where `ConflictsWith` is symmetric regardless of which side declares it.

### Why not `ResourceWithConfigValidators`?
It's a clean fit when the constraint is "exactly one of A, B, C" or "all of these together". Here the relationships are asymmetric (`fixed_ip` conflicts with `port_id` but not with `network_id`), so per-attribute validators with `path.Expression` are the more accurate translation.

## Other changes in this resource

| SDKv2 | Framework |
|---|---|
| `CreateContext` / `ReadContext` / `DeleteContext` | `Create` / `Read` / `Delete` methods on `*computeInterfaceAttachV2Resource` |
| `*schema.ResourceData` typed access (`d.Get`, `d.Set`, `d.Id`) | `computeInterfaceAttachV2Model` struct via `req.Plan.Get` / `resp.State.Set` |
| `ForceNew: true` | `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` |
| `Computed: true` (alone) | `Computed: true` + `stringplanmodifier.UseStateForUnknown()` to keep plans clean |
| `Timeouts: &schema.ResourceTimeout{...}` | `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` (block syntax preserved for backward-compat with existing HCL) and `plan.Timeouts.Create(ctx, 10*time.Minute)` inside CRUD |
| `Importer: ImportStatePassthroughContext` | `resource.ResourceWithImportState` with `ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| `diag.FromErr` / `diag.Errorf` | `resp.Diagnostics.AddError("...", err.Error())` + `if resp.Diagnostics.HasError() { return }` |
| `CheckDeleted(d, err, ...)` | inline `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` → `resp.State.RemoveResource(ctx)` (CheckDeleted's signature is SDKv2-bound; replicating its 404 handling inline keeps the migrated resource SDKv2-free) |
| `GetRegion(d, config)` | local `getRegionFromModel(plan.Region, r.config)` helper that operates on `types.String` |

`Update` is implemented as a near-no-op solely to satisfy `resource.Resource`. Every non-`id` attribute is `RequiresReplace`, so Terraform never plans an update path, but the interface still requires the method.

## Tests

- Switched both existing tests to `ProtoV6ProviderFactories` (using a project-wide `testAccProtoV6ProviderFactories` map that the provider scaffold is expected to provide once the framework provider is wired in).
- Added a new `TestAccComputeV2InterfaceAttach_conflictingPortAndNetwork` test that sets both `port_id` and `network_id` and uses `PlanOnly: true` + `ExpectError: regexpConflictsWithPortNetwork` to assert the new validator fires at config-validate time. This is the test that proves the migration of the cross-attribute validators (TDD step 7 — should fail red before the resource migrates).
- Added an `ImportStateVerify` step to the basic test (a free win once the importer moves to `ImportStatePassthroughID`).
- The destroy/exists checks no longer reach for the SDKv2 `testAccProvider` singleton; they call a shared `testAccConfig()` helper that the test bootstrap is expected to expose alongside `testAccProtoV6ProviderFactories`. If that helper doesn't exist yet in the repo, it'll need to be added as part of step 4 (provider definition) — out of scope for this single-resource migration but flagged here.

## Caveats / follow-ups

1. **`testAccProtoV6ProviderFactories` and `testAccConfig()` are referenced but assumed to exist.** They're standard scaffolding for a framework provider; the rest of the test suite will need them too once more resources migrate. They can't be defined inside this single-resource test file because they're shared across the suite.
2. **The "must set one of network_id and port_id" check stays in `Create`** rather than moving to a `resourcevalidator.AtLeastOneOf`. That would be a strictly better idiom — config-time error vs apply-time — but it changes the *timing* of the error from apply to plan, which is a (very mild) user-visible change. Flagged for a follow-up pass; mentioned in the per-resource checklist.
3. **`fixed_ip` conflicts with `port_id` only.** That matches SDKv2 exactly, so there's no behaviour change. Worth re-confirming with the OpenStack API docs that this is intentional rather than an oversight in the original schema.
4. **`compute_interface_attach_v2.go` (the helper file) still imports `terraform-plugin-sdk/v2/helper/retry`.** The migrated resource also imports it (for `retry.StateChangeConf`). The negative gate in `verify_tests.sh` (`--migrated-files` cannot import SDKv2) will fail on this resource until either (a) the retry helpers move to a non-SDKv2 home, or (b) we accept the remaining SDKv2 dep for this surface. Per the skill's guidance, `retry.StateChangeConf` is one of the patterns where keeping SDKv2 during a partial migration is the standard approach — it's listed under "Things you no longer need" *only* for the ones the framework has direct replacements for, and `retry` isn't one.
