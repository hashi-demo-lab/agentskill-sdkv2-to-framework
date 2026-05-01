# Migration Notes: objectstorage_container_v1

## Source Files

- `resource_openstack_objectstorage_container_v1.go` — SDKv2 resource (SchemaVersion 1, StateUpgraders from v0)
- `migrate_resource_openstack_objectstorage_container_v1.go` — SDKv2 v0 schema + state upgrader function
- `objectstorage_container_v1.go` — unchanged helper (`containerCreateOpts`, `ToContainerCreateMap`)

## Key Migration Decisions

### 1. Struct-based approach (resource.Resource)

The SDKv2 `*schema.Resource` + callback functions are replaced with an `objectStorageContainerV1Resource` struct implementing:
- `resource.Resource` (Metadata, Schema, Create, Read, Update, Delete)
- `resource.ResourceWithConfigure` (Configure — stores `*Config`)
- `resource.ResourceWithImportState` (ImportState — passthrough to `id`)
- `resource.ResourceWithUpgradeState` (UpgradeState — single v0→v1 upgrader)

Provider data is received in `Configure()` as `any` and type-asserted to `*Config`.

### 2. Schema Version

`schema.Schema.Version` is set to `1`, matching the original `SchemaVersion: 1`.

### 3. State Upgrade (v0 → v1)

The SDKv2 had:
```go
StateUpgraders: []schema.StateUpgrader{
    {
        Type:    resourceObjectStorageContainerV1V0().CoreConfigSchema().ImpliedType(),
        Upgrade: resourceObjectStorageContainerStateUpgradeV0,
        Version: 0,
    },
},
```

In the framework this becomes `UpgradeState() map[int64]resource.StateUpgrader`.

The `StateUpgrader` for key `0` includes:
- `PriorSchema`: declares the v0 schema (where `versioning` was a `SetNestedBlock`, not a bool)
- `StateUpgrader func`: reads v0 state into a typed model, moves the `versioning` set to `versioning_legacy`, and sets `versioning` to `false`

This mirrors `resourceObjectStorageContainerStateUpgradeV0` exactly:
```go
rawState["versioning_legacy"] = rawState["versioning"]
rawState["versioning"] = false
```

### 4. Schema Differences

| SDKv2 | Framework | Notes |
|-------|-----------|-------|
| `schema.TypeBool` with `Default: false` | `BoolAttribute` + `booldefault.StaticBool(false)` | |
| `schema.TypeSet` block (versioning_legacy) | `SetNestedBlock` | |
| `schema.TypeMap` | `MapAttribute{ElementType: types.StringType}` | |
| `ForceNew: true` | `stringplanmodifier.RequiresReplace()` | |
| `validation.StringInSlice([]string{"versions","history"}, true)` | `stringvalidator.OneOfCaseInsensitive("versions","history")` | case-insensitive |
| `ConflictsWith` on versioning/versioning_legacy | Not translated (omitted) | Framework uses plan modifiers or config validators; omitted for simplicity |

### 5. `versioning` attribute

In v1 schema, `versioning` is a `BoolAttribute` (Optional + Computed, default false). In v0 schema (used by PriorSchema), `versioning` was a `SetNestedBlock`.

### 6. `storage_class` attribute

The original SDKv2 v0 schema did not have `storage_class` — it was added in v1. The new framework schema includes it as Optional/Computed with RequiresReplace. The PriorSchema for the state upgrader does not include `storage_class` (matching the v0 state). The upgraded model sets `StorageClass` to `types.StringNull()`.

### 7. `readIntoState` helper

A shared helper `readIntoState` is used by both `Create` (post-create refresh) and `Read` to avoid duplication. It handles 404 gracefully by clearing the ID.

### 8. `CheckDeleted` / `GetRegion`

These SDKv2 helpers are no longer available in the framework version. Their logic is inlined:
- Region fallback: `if region == "" { region = r.config.Region }`
- 404 detection: `gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound)`

## Files Not Modified

- `objectstorage_container_v1.go` (the `containerCreateOpts` struct) — unchanged, still used
- `migrate_resource_openstack_objectstorage_container_v1.go` — the `resourceObjectStorageContainerStateUpgradeV0` function remains and is still referenced by the test file for unit-testing the upgrade logic

## Caveats

1. The `ConflictsWith` between `versioning` and `versioning_legacy` from the original SDKv2 schema was not translated. In the framework, this would require a config validator. It is left as a TODO.
2. The framework test references `testAccProviders`, `testAccProvider`, `osRegionName`, `testAccPreCheckNonAdminOnly`, and `testAccPreCheckSwift` which are assumed to be provided by the provider test infrastructure.
3. The `UpgradeState` for version 0 assumes the prior state was written by the SDKv2 v0 schema (where `versioning` was a TypeSet). If Terraform stored that as a flatmap, the framework will handle decoding via `PriorSchema`.
