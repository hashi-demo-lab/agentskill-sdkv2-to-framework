# Migration Summary: linode_user resource (SDKv2 → terraform-plugin-framework)

## Files produced

| Output path | Description |
|---|---|
| `migrated/resource.go` | Full framework resource implementation (no SDKv2 imports) |
| `migrated/resource_test.go` | Acceptance test file updated for framework provider factories |

## Key decisions

### global_grants: ListNestedAttribute (not SingleNestedAttribute)

The SDKv2 schema used `TypeList` with `MaxItems: 1`. In terraform-plugin-framework the canonical
translation of a `MaxItems:1` *list* block is `schema.ListNestedAttribute` (keeping a list of
exactly one element), **not** `SingleNestedAttribute`. This preserves the `global_grants.0.*`
address used in the existing test checks (e.g.
`resource.TestCheckResourceAttr(testUserResName, "global_grants.0.add_domains", "true")`).
Using `SingleNestedAttribute` would change the address to `global_grants.add_domains`, breaking
all existing test assertions and HCL configs. The list approach also matches how the existing
framework datasource for this resource models the same field
(`framework_datasource_schema.go` line 68–72, `framework_models.go` line 22 `GlobalGrants types.List`).

### Default: false → booldefault.StaticBool(false)

All SDKv2 `Default: false` bool fields in `global_grants` are mapped to:

```go
schema.BoolAttribute{
    Optional: true,
    Computed: true,
    Default:  booldefault.StaticBool(false),
    ...
}
```

`Computed: true` is required alongside `Default` so that Terraform knows the provider will supply
a value when the user omits the attribute. Without `Computed: true`, setting `Default` raises a
framework validation error.

### restricted: booldefault.StaticBool(false)

The top-level `restricted` field had `Default: false` in SDKv2 and is likewise translated to
`Optional + Computed + Default: booldefault.StaticBool(false)`.

### Entity grant sets: SetNestedAttribute

The SDKv2 `TypeSet` fields (`domain_grant`, `firewall_grant`, etc.) are translated to
`schema.SetNestedAttribute`. Each element has `id` (Int64, Required) and `permissions`
(String, Required). The SDKv2 resource's `flatten.go` filtered out entities whose `Permissions`
is empty; the same filtering is applied in `flattenEntityGrantsToModel`.

### ForceNew on email → RequiresReplace plan modifier

SDKv2 `ForceNew: true` on the `email` field is translated to
`stringplanmodifier.RequiresReplace()`.

### Resource ID

The resource uses the username as its string ID (matching SDKv2 `d.SetId(user.Username)`).
`BaseResource.ImportState` handles import passthrough because `IDAttr: "id"` and
`IDType: types.StringType` are set in the `BaseResourceConfig`.

### No schema.go changes

`schema_resource.go` was left untouched. The new framework schema is self-contained inside
`resource.go`. The object-type variables (`resourceGlobalGrantsObjectType`,
`resourceEntityGrantObjectType`) live in `resource.go`; the datasource equivalents
(`linodeUserGrantsGlobalObjectType`, `linodeUserGrantsEntityObjectType`) already exist in
`framework_datasource_schema.go` and were not duplicated.

## Test file changes

The test file is functionally identical to the original `resource_test.go` except:
- Build tags and package declaration are preserved (`//go:build integration || user`).
- `ProtoV6ProviderFactories` is used throughout (was already the case in the original).
- `checkUserDestroy` uses `TestAccSDKv2Provider` (retained as-is from the original).

No template files (`tmpl/*.gotf`) were modified.
