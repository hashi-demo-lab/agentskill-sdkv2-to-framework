# Migration notes — openstack_objectstorage_container_v1

## What was migrated

| Source file | Output file |
|---|---|
| `resource_openstack_objectstorage_container_v1.go` | `migrated/resource_openstack_objectstorage_container_v1.go` |
| `migrate_resource_openstack_objectstorage_container_v1.go` | `migrated/upgrade_objectstorage_container_v1.go` |
| `resource_openstack_objectstorage_container_v1_test.go` | `migrated/resource_openstack_objectstorage_container_v1_test.go` |
| `migrate_resource_openstack_objectstorage_container_v1_test.go` | (merged into the test file above) |

## Schema version history

- **V0** (old SDKv2 releases): `versioning` was a `TypeSet` block (`type`+`location`). No `storage_class`. No `versioning_legacy`.
- **V1** (current): `versioning` became a plain `bool` (default `false`). The old `versioning` block was renamed `versioning_legacy`. `storage_class` was added (Optional+Computed).

## State-upgrade translation

The SDKv2 `StateUpgraders` contained a single step V0→V1 (`resourceObjectStorageContainerStateUpgradeV0`). The raw-map transformation was:

```go
rawState["versioning_legacy"] = rawState["versioning"]
rawState["versioning"] = false
```

The framework `UpgradeState` method returns a `map[int64]resource.StateUpgrader` with a single entry keyed at `0`. Each entry produces the **current** schema state directly — no chaining. The typed upgrader (`upgradeContainerStateV0toV1`) does the same rename using model structs:

- `prior.Versioning` (block slice) → `current.VersioningLegacy`
- `current.Versioning` → `types.BoolValue(false)`
- `current.StorageClass` → `types.StringNull()` (field absent in V0; next Read cycle populates it)

`containerSchemaV0` mirrors the SDKv2 V0 schema exactly — attribute names and types must match what the old provider wrote to state; the framework uses it to deserialise prior state before passing it to the upgrader.

## Key design decisions

- **`versioning_legacy` kept as `SetNestedBlock`**: preserves backward-compatible HCL block syntax (`versioning_legacy { ... }`). Converting to `SingleNestedAttribute` would break existing practitioner configs.
- **`ConflictsWith` → `ResourceWithConfigValidators`**: the SDKv2 `ConflictsWith` on `versioning` / `versioning_legacy` is implemented as a `versioningConflictConfigValidator`, which also enforces the `MaxItems:1` constraint (blocks can't be `Required`/have `MaxItems` in the framework).
- **`ForceNew` → `RequiresReplace` plan modifiers**: on `region`, `storage_policy`, `storage_class`.
- **`Default: false` → `booldefault.StaticBool(false)`**: on `versioning` and `force_destroy`.
- **`d.SetId("")` → `resp.State.RemoveResource(ctx)`**: used in Read when container returns 404.
- **Upgrader is a free function, not a method**: the V0→V1 upgrader needs no API call (pure data transform), so it does not need access to `r.config`. This is safe and correct per `state-upgrade.md`.

## Compile / test results

**`gofmt -e` (syntax check)**: all three migrated files pass — no parse errors.

**Negative gate (no `terraform-plugin-sdk/v2` imports)**: CLEAN — all migrated files import only framework packages.

**Full compile**: not possible in isolation — the source provider (`terraform-provider-openstack`) does not yet include `terraform-plugin-framework` in its `go.mod`. To compile, add the following (or equivalent current versions) to the provider's `go.mod`:

```
require (
    github.com/hashicorp/terraform-plugin-framework           v1.14.0
    github.com/hashicorp/terraform-plugin-framework-validators v0.24.0
    github.com/hashicorp/terraform-plugin-go                  v0.29.0
)
```

**Acceptance tests**: require `TF_ACC=1` and a live OpenStack Swift endpoint. All acceptance test cases from the original file are preserved; `ProviderFactories` replaced with `ProtoV6ProviderFactories`.

**State-upgrade unit test** (`TestObjectStorageV1ContainerStateUpgradeV0`): directly exercises `upgradeContainerStateV0toV1` using `tfsdk.State` / `tftypes.Value` without `TF_ACC`. Asserts:
- `versioning` == `false`
- `versioning_legacy[0].type` == `"versions"`
- `versioning_legacy[0].location` == `"test"`
- `name` == `"test"`, `region` == `"RegionOne"` (preserved from V0)
- `storage_class` is null (new field, not in V0)
- `force_destroy` == `false`
