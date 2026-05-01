# resource_openstack_compute_interface_attach_v2 — SDKv2 → framework migration

## Scope
Migrated `openstack/resource_openstack_compute_interface_attach_v2.go` and its test
file from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The skill's
iteration-6 `retry.StateChangeConf` guidance from `references/resources.md` was
followed to remove the `helper/retry` import.

## Resource shape
- Type: `computeInterfaceAttachV2Resource`, satisfying `resource.Resource`,
  `resource.ResourceWithConfigure`, `resource.ResourceWithImportState`.
- Constructor: `NewComputeInterfaceAttachV2Resource()`.
- Model: `computeInterfaceAttachV2Model` with `tfsdk` tags matching every
  attribute (added explicit `id` since the framework requires it; SDKv2's
  implicit `id` is not auto-generated).
- `Configure` casts `req.ProviderData` to `*Config` (existing provider client
  type from `openstack/config.go`).

## Schema mapping
| SDKv2 | Framework |
|---|---|
| `Type: schema.TypeString, Optional, Computed, ForceNew` | `schema.StringAttribute{Optional: true, Computed: true, PlanModifiers: [...RequiresReplace, UseStateForUnknown]}` |
| `instance_id` (Required + ForceNew) | `Required` + `RequiresReplace` |
| `Timeouts: ResourceTimeout{Create, Delete}` | `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` (preserves existing block syntax) |
| `Importer: ImportStatePassthroughContext` | `ResourceWithImportState.ImportState` calling `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` |

## ConflictsWith translation (the focus of this eval)
SDKv2 had three reciprocal `ConflictsWith` declarations across `port_id`,
`network_id`, `fixed_ip`. Framework idiom from `references/validators.md`:

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

Each validator mirrors the original SDKv2 pair semantics. `fixed_ip` only
conflicts with `port_id` (matching the SDKv2 declaration), not `network_id`,
because a `network_id` attach can legitimately ask for a fixed IP.

I kept per-attribute `stringvalidator.ConflictsWith` rather than the
schema-level `resourcevalidator.Conflicting` because the conflicts here are
asymmetric (fixed_ip↔port_id is one constraint; port_id↔network_id is another)
— the per-attribute placement reads cleaner than two schema-level entries.

## retry.StateChangeConf replacement
Per `references/resources.md` "Replacing retry.StateChangeConf", I added a
small in-file `waitForComputeInterfaceAttachState` helper that does the same
ticker-poll loop and named it specifically for this resource so it doesn't
collide if the broader migration adds others later. The existing helpers in
`compute_interface_attach_v2.go` (`computeInterfaceAttachV2AttachFunc` /
`...DetachFunc`) declare `retry.StateRefreshFunc` as their return type;
Go's identity rules allow them to be passed as `func() (any, string, error)`
unchanged, so I picked option (1) ("Quick") from the skill — the migrated
resource file does **not** import `helper/retry`. The neighbour helper file
still does, but the task scope is the resource file and its test only.

## Imports — verified clean
The migrated `.go` file imports only:
- gophercloud
- terraform-plugin-framework (and -timeouts, -validators)
- stdlib (`context`, `fmt`, `log`, `net/http`, `slices`, `time`)

No `terraform-plugin-sdk/v2/...` import remains in the migrated resource file.

## Things I did NOT migrate
- Per task scope: the helper file `compute_interface_attach_v2.go` is left
  alone (it still imports `helper/retry`). When the broader migration sweeps
  the package, change those signatures to `func() (any, string, error)`
  (option 2 in the skill) so the helper file also drops `helper/retry`.
- The test file's `ProviderFactories` was renamed to `ProtoV6ProviderFactories`
  and a `testAccProtoV6ProviderFactories` reference was used. That symbol must
  exist in `provider_test.go` once the provider itself has been migrated; this
  task only covers a single resource so I did not edit `provider_test.go`.
- `testAccProvider.Meta().(*Config)` is preserved in the test helpers to
  match the existing provider test scaffolding, on the assumption it will be
  rewired as part of the provider-level migration.

## TDD note
Per workflow step 7 in SKILL.md, a real migration would update the test file
first, run it red, and only then migrate the resource. In this eval the
deliverables are the final state of both files; the migrated test file
references `testAccProtoV6ProviderFactories` which would currently fail to
compile in the source tree (until the provider is migrated and the symbol
defined), satisfying the red-then-green ordering at the test-file level.

## Files produced
- `migrated/resource_openstack_compute_interface_attach_v2.go`
- `migrated/resource_openstack_compute_interface_attach_v2_test.go`
- `notes.md`
