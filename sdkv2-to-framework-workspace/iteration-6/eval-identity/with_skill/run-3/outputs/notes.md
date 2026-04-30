# Migration notes — `openstack_lb_member_v2`

## Pre-edit summary (per skill `Think before editing` rule)

1. **Block decision**: no `MaxItems: 1 + Elem` blocks in this resource — only flat
   primitives plus a `TypeSet` of strings (`tags`). Mapped `tags` to a
   `schema.SetAttribute` with `ElementType: types.StringType`. Dropped the
   SDKv2 `Set: schema.HashString` hash function (the framework handles set
   uniqueness internally — see SKILL.md "common pitfalls").
2. **State upgrade**: no `SchemaVersion > 0`, no `StateUpgraders` — nothing to
   port. (`SchemaVersion` defaults to `0`; not present in the source file.)
3. **Import shape**: SDKv2 `Importer.StateContext` parsed a composite
   `<pool_id>/<member_id>` ID (no region in the import string). The migration
   adds an *identity* schema covering `region`, `pool_id`, and `id` (member id),
   and keeps the legacy CLI string-parse path for Terraform <1.12.

## Resource conversion summary

| SDKv2 element | Framework replacement |
|---|---|
| `func resourceMemberV2() *schema.Resource` | `memberV2Resource` type implementing `resource.Resource` + `ResourceWithConfigure` + `ResourceWithImportState` + `ResourceWithIdentity` |
| `CreateContext`/`ReadContext`/`UpdateContext`/`DeleteContext` | `Create`/`Read`/`Update`/`Delete` methods with typed `req`/`resp` |
| `*schema.ResourceData` + `d.Get`/`d.Set`/`d.Id`/`d.HasChange` | `memberV2Model` struct with `tfsdk` tags + `Plan.Get`/`State.Set` + `Equal()` for change detection |
| `Importer: &schema.ResourceImporter{StateContext: resourceMemberV2Import}` | `ImportState` method dispatching on `req.ID == ""` |
| `Timeouts: &schema.ResourceTimeout{...}` | `timeouts.Attributes(ctx, timeouts.Opts{Create:true,Update:true,Delete:true})` from `terraform-plugin-framework-timeouts/resource/timeouts` |
| `validation.IntBetween(...)` | `int64validator.Between(...)` |
| `ForceNew: true` | `stringplanmodifier.RequiresReplace()` / `int64planmodifier.RequiresReplace()` |
| `Default: true` (bool) | `booldefault.StaticBool(true)` |
| `Set: schema.HashString` on a `TypeSet` of strings | dropped (framework handles uniqueness) |
| `getOkExists(d, "weight")` (custom helper) | `!plan.Weight.IsNull() && !plan.Weight.IsUnknown()` — the framework natively distinguishes null from explicit-zero, so this is the canonical replacement (see `references/state-and-types.md`) |
| `retry.RetryContext` + `checkForRetryableError` | inline `retryOnConflict` helper that mirrors the same retryable-status-code set; avoids importing `terraform-plugin-sdk/v2/helper/retry` (negative gate) |
| `CheckDeleted(d, err, msg)` | inline `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` + `resp.State.RemoveResource(ctx)` (in `Read`) or silent return (in `Delete`) |
| `GetRegion(d, config)` | `r.regionFor(model)` method on the resource (model field if set, else provider default) |

## Identity schema

```go
identityschema.Schema{
    Attributes: map[string]identityschema.Attribute{
        "region":  identityschema.StringAttribute{RequiredForImport: true},
        "pool_id": identityschema.StringAttribute{RequiredForImport: true},
        "id":      identityschema.StringAttribute{RequiredForImport: true},
    },
}
```

`region` is included because the resource is region-scoped at the API level —
practitioners running multi-region OpenStack need it as part of resource
addressing. None of these are sensitive (per `references/identity.md`).

`Identity` is written via `setIdentity(...)` after every successful state write
in `Create`, `Read`, and `Update`. `setIdentity` is a no-op when
`resp.Identity == nil` (older Terraform CLI versions that didn't request it).

## ImportState — dual-path

- **Modern (Terraform 1.12+, `import { identity = {...} }`)**: dispatched when
  `req.ID == ""`. Three calls to `resource.ImportStatePassthroughWithIdentity`
  (one per identity attribute: `region`, `pool_id`, `id`).
- **Legacy (`terraform import openstack_lb_member_v2.foo <pool_id>/<member_id>`)**:
  dispatched when `req.ID != ""`. Splits on `/`, validates two non-empty
  segments, writes them with `resp.State.SetAttribute`. Returns a clear
  diagnostic if the format is wrong (matches the SDKv2 error wording).

The legacy path matches the SDKv2 importer exactly — region is *not* in the
legacy ID string, so a legacy import yields a state with `region` populated
later by `Read` (using the provider default) once the API fetch completes.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- Added `ConfigStateChecks` with `statecheck.ExpectIdentityValue(...)` to assert
  the new identity is populated post-create.
- Added an `ImportState`/`ImportStateVerify` step that exercises the legacy
  composite-ID path via `ImportStateIdFunc` (mirrors what real practitioners on
  pre-1.12 Terraform send today). `timeouts` is in `ImportStateVerifyIgnore`
  because it's config-only and not returned by `Read`.
- All `testAccCheck*` helpers (Exists, Destroy, HasTag, TagCount) are unchanged
  — they read from `terraform.State`, which is protocol-agnostic, so no
  rewrites were needed.

The test file references `testAccProtoV6ProviderFactories` — that variable is
expected to exist in `provider_test.go` once the provider-level migration
(workflow steps 3-5) is done. For the single-resource scope of this task it's
declared as a forward dependency rather than added here (the provider migration
is owned by a different checklist row).

## What I did NOT change

- `pools.CreateMember`/`UpdateMember`/`DeleteMember` API call sites — same
  gophercloud client, identical signatures.
- `waitForLBV2Pool` / `waitForLBV2Member` helpers (in `lb_v2_shared.go`,
  unchanged).
- The HCL config strings used by the tests — unchanged so the test
  configurations exercise exactly the same surface as before.
- `tenant_id` semantics — kept as a state-tracked computed-or-optional field;
  the API populates `ProjectID` after create.

## Verification

Per `references/workflow.md` step 9, run after wiring the resource into
the framework provider's `Resources()` list:

```sh
bash <skill-path>/scripts/verify_tests.sh <provider-repo> \
  --migrated-files openstack/resource_openstack_lb_member_v2.go \
                   openstack/resource_openstack_lb_member_v2_test.go
```

The negative gate (no `terraform-plugin-sdk/v2` imports in the migrated files)
should pass — none of the imports in the migrated `.go` reference the SDKv2
module path. The test file imports only `terraform-plugin-testing` (which is
correct — it's the framework-compatible testing module).
