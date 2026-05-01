# Migration Summary — openstack_compute_keypair_v2

## Scope
Single resource: `resource_openstack_compute_keypair_v2.go` and its test file.
No other files in the repository were modified.

## Pre-flight checks

**Pre-flight 0 (mux check):** Not a muxed migration — single-release scope confirmed.

**Pre-flight A (audit):** Resource has no `StateUpgraders`, no `ValidateFunc`, no `CustomizeDiff`, no `DiffSuppressFunc`, no `StateFunc`, no `Timeouts`, no `ConflictsWith`. Import is a simple passthrough (`schema.ImportStatePassthroughContext`). No `MaxItems: 1` nested blocks. Low complexity.

**Pre-flight B (plan):** Scope is this resource only. All attributes are `ForceNew` (no in-place updates possible) — the migrated resource implements an empty `Update` method.

**Pre-flight C (think pass):**
1. **Block decision:** No `MaxItems: 1 + nested Elem` attributes. `value_specs` is `TypeMap` → `schema.MapAttribute{ElementType: types.StringType}`.
2. **State upgrade:** `SchemaVersion` is 0 (default). No upgraders needed.
3. **Import shape:** Passthrough — `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Key translation decisions

| SDKv2 | Framework |
|---|---|
| `*schema.Resource` function | `computeKeypairV2Resource` struct implementing `resource.Resource` |
| `CreateContext` / `ReadContext` / `DeleteContext` | `Create` / `Read` / `Delete` on the struct |
| `d.Get("foo").(string)` | typed `computeKeypairV2Model` struct with `tfsdk:"..."` tags + `req.Plan.Get` / `req.State.Get` |
| `d.SetId(...)` / `d.Set(...)` | `resp.State.Set(ctx, model)` |
| `ForceNew: true` on region, name, public_key, value_specs, user_id | `stringplanmodifier.RequiresReplace()` / `mapplanmodifier.RequiresReplace()` in `PlanModifiers` |
| `Computed: true` on id, region, public_key, private_key, fingerprint, user_id | `stringplanmodifier.UseStateForUnknown()` added to stable computed attributes |
| `Sensitive: true` on private_key | `Sensitive: true` (same field) |
| `schema.ImportStatePassthroughContext` | `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |
| `CheckDeleted(d, err, ...)` (sets `d.SetId("")`) | `gophercloud.ResponseCodeIs(err, 404)` + `resp.State.RemoveResource(ctx)` |
| `MapValueSpecs(d)` | `plan.ValueSpecs.ElementsAs(ctx, &valueSpecs, false)` |
| `GetRegion(d, config)` | `plan.Region.ValueString()` with fallback to `r.config.Region` |
| `ProviderFactories: testAccProviders` | `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` |

## Pitfalls avoided

- **Delete reads from `req.State`**, not `req.Plan` (plan is null on delete).
- **`value_specs` map** handled via `ElementsAs` rather than the old `MapValueSpecs(d)` helper (which takes `*schema.ResourceData`).
- **`private_key`** carries `UseStateForUnknown` so it is not re-shown as `(known after apply)` on subsequent plans (the value is set once on create and never changes).
- **No SDKv2 imports** remain in the migrated resource file.

## Test changes

- `ProviderFactories` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` (requires `testAccProtoV6ProviderFactories` to be wired in `provider_test.go` as part of the full provider migration).
- Added `ImportState` / `ImportStateVerify` step to `TestAccComputeV2Keypair_basic` (was missing in original).
- `ImportStateVerifyIgnore: []string{"private_key", "value_specs"}` — `private_key` is only returned on Create; `value_specs` is write-only (not populated on Read).
- `testAccCheckComputeV2KeypairDestroy` and `testAccCheckComputeV2KeypairExists` helpers are unchanged (they use gophercloud directly, not the Terraform provider under test).

## Files produced

- `migrated/resource_openstack_compute_keypair_v2.go` — framework implementation, no SDKv2 import.
- `migrated/resource_openstack_compute_keypair_v2_test.go` — uses `ProtoV6ProviderFactories`.
