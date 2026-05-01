# Migration Summary: linode_user resource

## Source files read

- `/Users/simon.lynch/git/terraform-provider-linode/linode/user/resource.go` (SDKv2 CRUD + helper funcs)
- `/Users/simon.lynch/git/terraform-provider-linode/linode/user/schema_resource.go` (SDKv2 schema)
- `/Users/simon.lynch/git/terraform-provider-linode/linode/user/flatten.go` (flatten helpers)
- `/Users/simon.lynch/git/terraform-provider-linode/linode/user/framework_models.go` (existing datasource models — reused patterns)
- `/Users/simon.lynch/git/terraform-provider-linode/linode/user/framework_datasource_schema.go` (object types, datasource schema)
- `/Users/simon.lynch/git/terraform-provider-linode/linode/firewall/framework_schema_resource.go` (style reference)
- `/Users/simon.lynch/git/terraform-provider-linode/linode/firewall/framework_resource.go` (style reference)

## Pre-flight C think pass

1. **Block decision (`global_grants`)**: The SDKv2 schema has `TypeList + MaxItems:1 + Elem: &schema.Resource{...}`. This is an existing production resource whose practitioners use block syntax (`global_grants { ... }`). Switching to `SingleNestedAttribute` would be a breaking HCL change. Decision: **`ListNestedBlock` + `listvalidator.SizeAtMost(1)`**. The entity grant sets (`domain_grant`, etc.) are `TypeSet + Elem: &schema.Resource{...}` — these become **`SetNestedAttribute`** (attribute not block, consistent with how the already-migrated datasource treats them, and there is no block-syntax compatibility concern for set-typed attrs).

2. **State upgrade**: `SchemaVersion` is not set (defaults to 0). No state upgraders needed.

3. **Import shape**: The SDKv2 importer uses `ImportStatePassthroughContext` with the username as ID. Migrated to `resource.ImportStatePassthroughID` on `path.Root("id")`.

## Default fields — booldefault, not PlanModifiers

All `Default: false` fields in the SDKv2 schema are translated using `booldefault.StaticBool(false)` on the `Default:` field of the schema attribute. No `Default` value is placed inside `PlanModifiers` (which would be a type error — `defaults.Bool` is not `planmodifier.Bool`).

Fields using `booldefault.StaticBool(false)`:
- `restricted` (top-level resource attribute)
- `add_domains`, `add_databases`, `add_firewalls`, `add_images`, `add_linodes`, `add_longview`, `add_nodebalancers`, `add_stackscripts`, `add_volumes`, `add_vpcs`, `cancel_account`, `longview_subscription` (all inside `global_grants` block)

## SDKv2 removal

- No `github.com/hashicorp/terraform-plugin-sdk/v2` import in the migrated `resource.go`.
- All `*schema.ResourceData` accesses replaced with typed model struct reads via `req.Plan.Get` / `req.State.Get`.
- `d.Set(...)` calls replaced with `resp.State.Set(ctx, &model)`.
- `diag.Errorf(...)` replaced with `resp.Diagnostics.AddError(...)`.
- `d.Id()` / `d.SetId()` replaced with `model.ID` (`types.String`).
- `d.HasChanges(...)` replaced with `resourceGrantsChanged()` comparing typed model fields.

## Delete reads from State

`Delete` reads from `req.State` (not `req.Plan`, which is null on Delete) — pitfall from SKILL.md observed and applied.

## Test file

The test file (`resource_test.go`) already used `ProtoV6ProviderFactories` in the original. The migrated version keeps all three test functions (`TestAccResourceUser_basic`, `TestAccResourceUser_updates`, `TestAccResourceUser_grants`) and adds an `ImportState`/`ImportStateVerify` step to `TestAccResourceUser_basic` to exercise the new `ImportState` method. The `checkUserDestroy` helper is preserved as-is.

## What was NOT changed

- No files in the linode provider repo were modified.
- The datasource (`framework_datasource.go`, `framework_datasource_schema.go`, `framework_models.go`) was not touched.
- The `flatten.go` SDKv2 helper file was not touched (it remains for any residual SDKv2 usage in the repo).
