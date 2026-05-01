# Migration Summary — openstack_db_user_v1

## Source file
`openstack/resource_openstack_db_user_v1.go` (SDKv2)

## What was done

### Resource file
- Removed all `terraform-plugin-sdk/v2` imports.
- Implemented the framework `resource.Resource` interface with `Metadata`, `Schema`, `Configure`, `Create`, `Read`, `Update`, `Delete`.
- Added `ResourceWithConfigure` and `ResourceWithImportState` sub-interfaces.
- **`password` attribute**: `Required: true, Sensitive: true, WriteOnly: true` — NOT Computed. Write-only values are not persisted to state.
- **CRUD reads `password` from `req.Config`** (not `req.Plan` or `req.State`), as required for write-only attributes.
- `ForceNew: true` on all attributes → `stringplanmodifier.RequiresReplace()` plan modifiers.
- `region` is `Optional + Computed` with both `RequiresReplace` and `UseStateForUnknown`.
- `databases` migrated from `TypeSet` with `schema.HashString` to `schema.SetAttribute{ElementType: types.StringType}` — hash function dropped (framework handles uniqueness internally).
- `Timeouts` field migrated to `terraform-plugin-framework-timeouts` with block syntax (preserving HCL compatibility).
- `retry.StateChangeConf` replaced with an inline `waitForDBUserState` poll loop — no SDKv2 retry import.
- `GetRegion(d, config)` replaced with direct region resolution from plan/state model with `config.Region` fallback.
- `CheckDeleted` (which requires `*schema.ResourceData`) replaced with inline `gophercloud.ResponseCodeIs(err, 404)` check in Delete.
- `ImportState` added: parses composite ID `instance_id/name` into state fields; `Read` populates the rest.
- `Update` is a no-op stub (all attributes have `RequiresReplace`; Terraform will never call Update).

### Test file
- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- Added an import test step with `ImportStateVerifyIgnore: []string{"password"}` — required because `password` is write-only and absent from state; without this ignore, `ImportStateVerify` would fail comparing null to the configured value.

## Key write-only rule applied
Per `references/sensitive-and-writeonly.md`: write-only values live in `req.Config` (where the practitioner wrote them) but are never in `req.Plan` or `req.State`. `Create` reads the full config model to get `password`; `Read` never attempts to read or write back the password field (it remains null in state).

## No breaking schema names
All attribute names are identical to the SDKv2 version. The only user-visible change is `password` no longer appearing in state (it was already `Sensitive`). This migration is declared as part of a major version bump where breaking changes are acceptable.
