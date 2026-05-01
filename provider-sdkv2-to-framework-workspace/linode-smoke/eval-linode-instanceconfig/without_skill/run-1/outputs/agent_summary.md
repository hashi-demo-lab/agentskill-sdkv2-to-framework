# Migration Summary: linode_instance_config (SDKv2 → Framework)

## Source files read
- `linode/instanceconfig/resource.go`
- `linode/instanceconfig/schema_resource.go`
- `linode/instanceconfig/helper.go`
- `linode/instanceconfig/resource_test.go`
- `linode/helper/framework_resource_base.go`
- `linode/helper/framework_provider_model.go`
- `linode/helper/framework_util.go`
- `linode/helper/resource_datasource_config.go`
- `linode/helper/instance.go` (partial)
- `linode/instance/helpers.go` (partial — VPCInterfaceIncluded, ShutdownInstanceForOfflineOperation, BootInstanceAfterOfflineOperation)
- `linode/image/framework_schema_resource.go` (reference for ConflictsWith pattern)

## Key decisions

### 1. Resource struct and constructor
- `Resource()` (returning `*schema.Resource`) replaced by `NewResource()` (returning `resource.Resource`).
- Composed via `helper.BaseResource` / `helper.NewBaseResource` to get `Configure`, `Metadata`, and `Schema` for free.
- State kept in `InstanceConfigResourceModel` (tfsdk tags).

### 2. ConflictsWith translation
SDKv2 `ConflictsWith: []string{"devices"}` on `device` and vice-versa becomes:
```go
// on the "device" SetNestedBlock:
Validators: []validator.Set{
    setvalidator.ConflictsWith(path.MatchRoot("devices")),
}

// on the "devices" ListNestedBlock:
Validators: []validator.List{
    listvalidator.SizeAtMost(1),              // replaces MaxItems:1
    listvalidator.ConflictsWith(path.MatchRoot("device")),
}
```
Imports: `github.com/hashicorp/terraform-plugin-framework-validators/setvalidator`,
`listvalidator`, `stringvalidator`.

### 3. MaxItems:1 blocks
The `devices` (deprecated named block) and `helpers` blocks had `MaxItems:1` in SDKv2.  
In the framework these become `schema.ListNestedBlock` with `listvalidator.SizeAtMost(1)`.

### 4. Deprecated block
`devices` block carries `DeprecationMessage` on the block itself (framework equivalent of the SDKv2 `Deprecated` field).

### 5. Import
`importResource` (SDKv2 stateful func) replaced by `ImportState` method.  
Supports both `<linodeID>,<configID>` and single-integer `<configID>` import IDs.

### 6. Deadline / timeout
SDKv2 `helper.GetDeadlineSeconds(ctx, d)` (needs `*schema.ResourceData`) replaced by a local helper `getFrameworkDeadlineSeconds(ctx)` that inspects `ctx.Deadline()` and falls back to `helper.DefaultFrameworkRebootTimeout` (600 s).

### 7. `BootInstanceAfterOfflineOperation`
The original signature requires `*helper.ProviderMeta` (SDKv2 meta).  
In the framework resource, `r.Meta` is `*helper.FrameworkProviderMeta`.  
We call `helper.BootInstanceSync` directly instead of the SDKv2 wrapper.

### 8. `ShutdownInstanceForOfflineOperation`
Signature already takes `skipImplicitReboots bool` and `*linodego.Client`, so it is callable from the framework resource; `r.Meta.Config.SkipImplicitReboots.ValueBool()` is used.

### 9. Read/flatten
`d.Set(...)` calls replaced by typed assignments to `InstanceConfigResourceModel` fields.  
`helper.FlattenInterfaces` (returns `[]map[string]any`) is still used as an intermediate step; the result is converted to `types.List` of `types.Object` via `rawInterfaceToObject`.

### 10. `populateLogAttributes`
Replaces the SDKv2 version (which needed `*schema.ResourceData`) with a framework-idiomatic version that reads from the model struct.

## Known limitations / follow-up items
1. **Interface expand/flatten** — `expandInterfacesFramework` maps the framework model back to `[]any` for `helper.ExpandConfigInterfaces`. A full framework-native interface expand would be cleaner but requires deeper changes to the `helper` package.
2. **`device` set ordering on import** — because `device` is a `SetNestedBlock`, `ImportStateVerify` may produce ordering diffs. `ImportStateVerifyIgnore: []string{"device"}` is retained in tests that use the deprecated `devices` block alongside `device`.
3. **`booted` null check** — the SDKv2 resource used `d.GetRawConfig().GetAttr("booted").IsNull()` to distinguish "not set" from `false`. The framework equivalent is `plan.Booted.IsNull()` which is checked before calling `applyBootStatus`.
4. **`SetConnInfo`** — the SDKv2 resource set SSH connection info (`type=ssh`, `host=<public IP>`). The framework has no direct equivalent for provisioner connection info; this call was dropped. If provisioners are required, a `resource.PrivateState` approach or upstream workaround is needed.
