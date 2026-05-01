# Migration Summary — openstack_compute_interface_attach_v2

## Resource overview

Single-lifecycle resource (Create/Read/Delete — no Update, all attributes are ForceNew).
Composite ID: `{instance_id}/{port_id}`.

## Cross-attribute constraint translation (the eval focus)

SDKv2 used three `ConflictsWith` declarations:

| Attribute   | SDKv2 `ConflictsWith`      |
|-------------|---------------------------|
| `port_id`   | `["network_id"]`           |
| `network_id`| `["port_id"]`              |
| `fixed_ip`  | `["port_id"]`              |

Framework translation — per-attribute `Validators` with `stringvalidator.ConflictsWith(path.MatchRoot(...))`:

```go
"port_id": schema.StringAttribute{
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("network_id")),
    },
},
"network_id": schema.StringAttribute{
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},
"fixed_ip": schema.StringAttribute{
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},
```

The per-attribute approach was chosen over `resourcevalidator.Conflicting` because the constraints are asymmetric (`fixed_ip` conflicts with `port_id` only, not `network_id`), so symmetric `resourcevalidator.Conflicting` would over-constrain.

## Other migration decisions

- **`retry.StateChangeConf`**: Replaced with an inline `waitForComputeInterfaceAttachState` helper (the framework has no equivalent). The existing SDKv2 `computeInterfaceAttachV2AttachFunc` / `computeInterfaceAttachV2DetachFunc` were re-implemented as `*Framework` variants returning `func() (any, string, error)` instead of `retry.StateRefreshFunc`, eliminating the SDKv2 import entirely from the resource file.
- **Timeouts**: Translated via `github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts`. Added `timeouts` attribute with `Create` and `Delete` opts. Defaults preserved at 10 minutes each.
- **Import**: `ImportStatePassthroughContext` → `ResourceWithImportState.ImportState` using `parsePairedIDs` to split the composite `{instance_id}/{port_id}` ID into its components and seed state before `Read` runs.
- **`CheckDeleted`**: Replaced inline with `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` + `resp.State.RemoveResource(ctx)`.
- **`GetRegion`**: Replaced inline — read from `state.Region.ValueString()`, fall back to `r.config.Region`.
- **`ForceNew: true`**: All applicable attributes gained `stringplanmodifier.RequiresReplace()`. Computed-after-apply attributes also gained `stringplanmodifier.UseStateForUnknown()`.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` in all three test cases.
- Import test (`TestAccComputeV2InterfaceAttachImport_basic`) merged into the main test file (was a separate `import_openstack_compute_interface_attach_v2_test.go` in the SDKv2 tree).
- Helper functions (`testAccCheckComputeV2InterfaceAttachDestroy`, `testAccCheckComputeV2InterfaceAttachExists`, `testAccCheckComputeV2InterfaceAttachIP`) and config generators preserved unchanged — they use `terraform-plugin-testing` which is protocol-agnostic.

## Imports removed

- `github.com/hashicorp/terraform-plugin-sdk/v2/diag`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema`

## Assumptions / caveats

- `testAccProtoV6ProviderFactories` is assumed to exist in `provider_test.go` (standard pattern for framework-migrated providers). If it does not yet exist, a declaration such as `var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){ "openstack": providerserver.NewProtocol6WithError(New()()) }` must be added.
- The original `compute_interface_attach_v2.go` helper file still imports `helper/retry` (its `retry.StateRefreshFunc` return type); this file is NOT part of the migrated output and should be updated separately when the broader migration sweeps that file.
- No `ConflictsWith: []string{...}` literals remain in the migrated resource file.
