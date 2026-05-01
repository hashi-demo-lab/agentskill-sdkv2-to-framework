# Migration Summary: linode_user resource

## Files Produced
- `migrated/resource.go` — full framework resource (schema + CRUD + helpers)
- `migrated/resource_test.go` — updated acceptance tests

## Key Migration Decisions

### MaxItems:1 block → SingleNestedBlock
`global_grants` was declared as `schema.TypeList` with `MaxItems: 1` in SDK v2.
In the framework this becomes `schema.SingleNestedBlock`, whose Go model field is a
pointer (`*GlobalGrantsModel`). A nil pointer means the block is absent; a non-nil
pointer means it is present.

### Default: false → booldefault.StaticBool(false)
Every `Default: false` bool field inside `global_grants` is reproduced with:
```go
schema.BoolAttribute{
    Optional: true,
    Computed: true,
    Default:  booldefault.StaticBool(false),
    ...
}
```
`Computed: true` is required alongside `Default` so the framework knows it may
differ from the plan value during reads.

### TypeSet entity blocks → SetNestedBlock
Each entity-grant field (`domain_grant`, `firewall_grant`, etc.) was a
`schema.TypeSet` with a nested `*schema.Resource`. In the framework these become
`schema.SetNestedBlock`. The Go model uses `[]GrantEntityModel` slices (framework
maps set blocks to slices automatically).

### ID handling
The resource ID is the username string. `ImportState` sets `path.Root("id")` to
`req.ID` and the `BaseResource.ImportState` fallback was overridden to use
`types.StringType`.

### Permissions entity filtering
The original SDK v2 resource filtered out `GrantedEntity` items where
`Permissions == ""` to avoid false diffs. This behaviour is preserved in
`flattenEntityGrantsToModel`.

### Test changes
- `global_grants.0.field` → `global_grants.field` (SingleNestedBlock uses no index)
- All other test checks are unchanged; `ProtoV6ProviderFactories` was already used

## What was NOT changed
- `flatten.go` (SDK v2 flatten helpers, still used by the datasource's SDK-era code)
- `framework_datasource*.go` / `framework_models.go` (datasource already migrated)
- `tmpl/` templates (no changes needed; the HCL already uses block syntax)
- Test `checkUserDestroy` function (still valid as-is)
