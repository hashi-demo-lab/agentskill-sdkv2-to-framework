# Migration Summary: openstack_db_database_v1

## Source file
`openstack/resource_openstack_db_database_v1.go` — SDKv2, Create/Read/Delete, composite ID (`instanceID/dbName`), passthrough importer, `schema.ResourceTimeout` with Create and Delete defaults of 10 minutes.

## Pre-flight decisions

1. **Block decision**: No `MaxItems: 1` nested blocks in the schema — only flat primitive attributes. All three attributes (`region`, `name`, `instance_id`) have `ForceNew: true`, converted to `stringplanmodifier.RequiresReplace()`.
2. **State upgrade**: `SchemaVersion` not set — no upgraders needed.
3. **Import shape**: `schema.ImportStatePassthroughContext` with a composite `instanceID/dbName` ID. Translated to a custom `ImportState` method that splits on `/` and writes `id`, `instance_id`, and `name` into state before `Read` runs.

## Key changes

| SDKv2 | Framework |
|---|---|
| `*schema.Resource` function | `dbDatabaseV1Resource` struct implementing `resource.Resource` + sub-interfaces |
| `schema.ResourceTimeout` | `timeouts.Block(ctx, Opts{Create: true, Delete: true})` in `Blocks:` map (preserves HCL block syntax) |
| `timeouts.Value` field in model struct with `tfsdk:"timeouts"` | — |
| `d.Timeout(schema.TimeoutCreate)` | `plan.Timeouts.Create(ctx, 10*time.Minute)` |
| `d.Timeout(schema.TimeoutDelete)` | `state.Timeouts.Delete(ctx, 10*time.Minute)` |
| `retry.StateChangeConf.WaitForStateContext` | Inline `waitForState` helper (avoids importing `terraform-plugin-sdk/v2/helper/retry`) |
| `GetRegion(d, config)` (SDKv2 helper) | Inline: `plan.Region.ValueString()` with `r.config.Region` fallback |
| `CheckDeleted(d, err, msg)` | Inline: `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` check |
| `d.SetId("")` in Read | `resp.State.RemoveResource(ctx)` |
| `schema.ImportStatePassthroughContext` | `ImportState` method splitting composite ID into `id`/`instance_id`/`name` |

## `databaseDatabaseV1StateRefreshFunc` compatibility

The existing helper in `db_database_v1.go` returns `retry.StateRefreshFunc` (a named type for `func() (any, string, error)`). Go's type identity rules allow assigning it directly to the inline `waitForState`'s `refresh func() (any, string, error)` parameter — no change to the helper file is required, and the resource file itself has zero `terraform-plugin-sdk/v2` imports.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`
- Added `ImportStateVerify` step to the basic test
- All other test helper logic unchanged (uses `terraform-plugin-testing` which was already the test package)

## Outputs
- `migrated/resource_openstack_db_database_v1.go` — framework resource, no SDKv2 imports
- `migrated/resource_openstack_db_database_v1_test.go` — updated test with `ProtoV6ProviderFactories` and import verification step
