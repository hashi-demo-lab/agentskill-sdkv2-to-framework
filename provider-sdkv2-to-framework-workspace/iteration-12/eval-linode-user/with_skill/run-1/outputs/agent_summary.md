# Migration Summary: linode_user resource

## Source files
- `linode/user/resource.go` — SDKv2 CRUD + grant helpers
- `linode/user/schema_resource.go` — SDKv2 schema with `Default: false` booleans and `MaxItems:1` block

## Key decisions

### MaxItems:1 block (`global_grants`)
Used `schema.ListNestedBlock` with `listvalidator.SizeAtMost(1)` (kept as block, not converted to `SingleNestedAttribute`) because practitioners are using block syntax `global_grants { ... }` in production configs. Switching to `SingleNestedAttribute` would be an HCL-breaking change.

### Default: false fields
All `Default: false` booleans in `global_grants` nested block translated via `booldefault.StaticBool(false)` in the schema definition, NOT inside `PlanModifiers`. The `restricted` top-level field similarly uses `booldefault.StaticBool(false)`.

### Entity grant sets (TypeSet of Resource)
Translated to `schema.SetNestedAttribute` with `NestedObject` containing `id` (Int64) and `permissions` (String). The `label` field present in the data source schema was intentionally omitted from the resource schema (matches original SDKv2 schema).

### Model design
- `ResourceModel` struct with `tfsdk:` tags matching schema attribute names exactly.
- Separate `resourceGrantsGlobalObjectType` and `resourceGrantsEntityObjectType` for type-safe object construction.
- `parseGrantsIntoModel` / `flattenResourceGrantEntities` helper functions for clean Read logic.
- `expandResourceGrantEntities` returns `[]linodego.EntityUserGrant` (no error return — diags pointer pattern used instead).

### Import
Custom `ImportState` implementation that sets username from `req.ID` then calls `readResourceInto` to populate full state (matching the passthrough + read pattern of the original).

### Test file
- `ProviderFactories` → `ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories` (already present in original; preserved)
- `checkUserDestroy` preserved as-is (uses `acceptance.TestAccSDKv2Provider` which is valid in a muxed provider setup)
- No SDKv2 imports in test file

## No SDKv2 imports
`resource.go` imports only `terraform-plugin-framework` packages + `linodego` + `helper`.
