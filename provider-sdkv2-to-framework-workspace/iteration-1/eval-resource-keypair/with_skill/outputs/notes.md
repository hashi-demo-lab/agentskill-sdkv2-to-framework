# Migration notes: openstack_compute_keypair_v2

## What changed

### Resource type shape
- Old: `func resourceComputeKeypairV2() *schema.Resource` returning an SDKv2 `*schema.Resource` with `CreateContext`/`ReadContext`/`DeleteContext` function pointers.
- New: `computeKeypairV2Resource` struct type implementing `resource.Resource`, `resource.ResourceWithConfigure`, and `resource.ResourceWithImportState`. Factory function `NewComputeKeypairV2Resource() resource.Resource`.

### CRUD method signatures
| SDKv2 | Framework |
|---|---|
| `resourceComputeKeypairV2Create(ctx, *schema.ResourceData, any) diag.Diagnostics` | `(r *computeKeypairV2Resource) Create(ctx, resource.CreateRequest, *resource.CreateResponse)` |
| `resourceComputeKeypairV2Read(ctx, *schema.ResourceData, any) diag.Diagnostics` | `(r *computeKeypairV2Resource) Read(ctx, resource.ReadRequest, *resource.ReadResponse)` |
| No Update (ForceNew on all mutable attrs) | `Update` is a no-op stub required by the interface |
| `resourceComputeKeypairV2Delete(ctx, *schema.ResourceData, any) diag.Diagnostics` | `(r *computeKeypairV2Resource) Delete(ctx, resource.DeleteRequest, *resource.DeleteResponse)` |

Errors previously returned as `diag.Diagnostics` are now appended to `resp.Diagnostics`.

### State access
- `d.Get("field").(string)` → `var model computeKeypairV2Model; req.Plan.Get(ctx, &model); model.Field.ValueString()`
- `d.Set("field", val)` → `model.Field = types.StringValue(val); resp.State.Set(ctx, &model)`
- `d.SetId(id)` → `model.ID = types.StringValue(id)` then `resp.State.Set`
- `d.SetId("")` (SDKv2 drift signal) → `resp.State.RemoveResource(ctx)` in Read

### ForceNew → RequiresReplace
Every attribute that had `ForceNew: true` received a `PlanModifiers` slice with `stringplanmodifier.RequiresReplace()` (or `mapplanmodifier.RequiresReplace()` for `value_specs`). Affected attributes: `region`, `name`, `public_key`, `value_specs`, `user_id`.

### Computed attributes: UseStateForUnknown
All Computed-only or Optional+Computed attributes also received `stringplanmodifier.UseStateForUnknown()` to prevent noisy "(known after apply)" diffs on unchanged refreshes. Affected: `id`, `region`, `public_key`, `private_key`, `fingerprint`, `user_id`.

### private_key handling
`private_key` is only returned by the Create API call; the Read API does not return it. In SDKv2 this worked silently because `d.Set` was idempotent (nil would leave the old value). In the framework, `UseStateForUnknown` on the plan modifier preserves the value written during Create so it is not lost on subsequent refreshes. The attribute remains `Sensitive: true`.

### value_specs
`TypeMap` with `Optional+ForceNew` became `schema.MapAttribute{Optional: true, ElementType: types.StringType}` with `mapplanmodifier.RequiresReplace()`. The map is decoded into `map[string]string` via `plan.ValueSpecs.ElementsAs(ctx, &valueSpecs, false)`.

### Drift / not-found handling
- SDKv2: `CheckDeleted(d, err, msg)` calls `d.SetId("")` on 404 and returns `nil`.
- Framework: direct `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` check in Read and Delete, then `resp.State.RemoveResource(ctx)` in Read; silent return in Delete.

### Import handling
- SDKv2: `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}`.
- Framework: `ResourceWithImportState` interface with `ImportState` method calling `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`. Semantics identical: import ID is the keypair name.

### Provider-level configuration
`meta.(*Config)` is replaced by the `Configure` method on the resource type, which receives `req.ProviderData.(*Config)` and stores it as `r.config`. This wires through the Gophercloud compute client factory.

### Explicit "id" attribute
The framework requires an explicit `id` attribute in the schema (SDKv2 created it implicitly). Added as `Computed: true` with `UseStateForUnknown`.

### No state upgraders / no Timeouts
The original resource had no `SchemaVersion`, no `StateUpgraders`, and no `Timeouts` — nothing to migrate for those sub-features.

---

## Test file changes

### ProviderFactories
The test cases' `ProviderFactories` field is retained as `testAccProviders` (the SDKv2 global) with a `TODO` comment. This is intentional: since the task scope is limited to migrating this single resource file (not the provider itself), the framework provider server (`ProtoV6ProviderFactories`) cannot be wired until `provider.go` is also migrated. The comment documents the exact swap needed.

### New import step added
`TestAccComputeV2Keypair_basic` gained an `ImportState: true` test step with `ImportStateVerifyIgnore: []string{"private_key"}`. The `private_key` attribute is excluded because the OpenStack API does not return it after creation, so it cannot be verified on import.

### TestAccComputeV2Keypair_generatePrivate
Unchanged in logic; `ProviderFactories` swap noted by TODO comment.

### No SDKv2 imports in either migrated file
Negative gate confirmed: neither `resource_openstack_compute_keypair_v2.go` nor `resource_openstack_compute_keypair_v2_test.go` import `github.com/hashicorp/terraform-plugin-sdk/v2`.

---

## Compile / vet verification

A compile check was performed by:
1. Copying the openstack provider repo to `/tmp/tp-openstack-keypair-check/`.
2. Adding `github.com/hashicorp/terraform-plugin-framework` via `go get` (required upgrading `terraform-plugin-sdk/v2` to `v2.40.1` and `terraform-plugin-go` to `v0.31.0` for compatibility).
3. Replacing the keypair resource and test files with the migrated versions.
4. Adding a minimal shim stub `resourceComputeKeypairV2()` so that `provider.go` (which still references the old function) could compile — this shim would be removed when `provider.go` is updated to call `NewComputeKeypairV2Resource()`.
5. Running `go build ./openstack/...` → **passed (no errors)**.
6. Running `go vet ./openstack/...` → **passed (no warnings)**.

`TF_ACC` tests were not run (require live OpenStack credentials). The build+vet gates confirm the migrated code is syntactically and type-correct.

---

## Next steps (out of scope for this task)

- Update `provider.go` line 413: replace `resourceComputeKeypairV2()` with registration of `NewComputeKeypairV2Resource()` in the framework provider's `Resources()` method.
- Once `provider.go` is migrated, replace `ProviderFactories: testAccProviders` with `ProtoV6ProviderFactories: protoV6ProviderFactories` in the test cases.
- Remove the temporary shim if one was used during incremental migration.
