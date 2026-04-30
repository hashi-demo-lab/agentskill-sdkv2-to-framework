# Migration Notes: openstack_blockstorage_availability_zones_v3

## What was migrated

`openstack/data_source_openstack_blockstorage_availability_zones_v3.go` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

The corresponding test file was updated per workflow step 7 (TDD gate).

---

## Key decisions

### 1. DataSource interface
The SDKv2 `*schema.Resource` returned by `dataSourceBlockStorageAvailabilityZonesV3()` is
replaced with a struct `blockStorageAvailabilityZonesV3DataSource` implementing
`datasource.DataSource` and `datasource.DataSourceWithConfigure`.

A `NewBlockStorageAvailabilityZonesV3DataSource()` constructor is provided for
registration in the provider's `DataSources()` list.

### 2. `state` attribute — no Default in datasource schema
The framework's `datasource/schema.StringAttribute` has no `Default` field (unlike
`resource/schema.StringAttribute` which can use the `defaults` package). The SDKv2
`Default: "available"` is therefore emulated in the `Read` method: if `model.State`
is null/unknown/empty at read time, `stateFilter` is set to `"available"` and written
back into state. This preserves the user-facing behaviour.

### 3. `ValidateFunc: validation.StringInSlice(…, true)` → `stringvalidator.OneOfCaseInsensitive`
The case-insensitive flag (`true`) maps to
`github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator.OneOfCaseInsensitive`.

### 4. `hashcode.Strings` retained for ID
The computed `id` is still derived from `hashcode.Strings(zones)` to preserve state
compatibility with any existing state files.

### 5. Typed model struct
`blockStorageAvailabilityZonesV3Model` uses `tfsdk:"..."` tags for all four
attributes: `id`, `region`, `state`, `names`.
`names` is `types.List` with `ElementType: types.StringType` (previously
`schema.TypeList` with `Elem: &schema.Schema{Type: schema.TypeString}`).
`types.ListValueFrom` is used to construct the list from `[]string`.

### 6. Region handling
`GetRegion(d, config)` (which reads from `*schema.ResourceData`) is replaced with
an equivalent inline check on `model.Region`, falling back to `d.config.Region`.

---

## Compile check (copy to /tmp)

The repo was copied to `/tmp/terraform-provider-openstack-migrate-check` and
framework dependencies were added via `go get`:

```
github.com/hashicorp/terraform-plugin-framework v1.14.1
github.com/hashicorp/terraform-plugin-framework-validators v0.16.0
```

Version v1.14.1 was chosen because it requires `terraform-plugin-go v0.26.0`,
which is compatible with `terraform-plugin-sdk/v2 v2.38.1` in the same module.
Using framework v1.19.0 (the latest at time of writing) caused a compile error in
the SDKv2 gRPC shim due to a newer `terraform-plugin-go` introducing a
`GenerateResourceConfig` method that SDKv2's `GRPCProviderServer` doesn't implement.

### Build result

```
$ go build ./openstack/...
# github.com/terraform-provider-openstack/terraform-provider-openstack/v3/openstack
openstack/provider.go:334:58: undefined: dataSourceBlockStorageAvailabilityZonesV3
```

**The sole build error is in `provider.go`**, which still references the old
`dataSourceBlockStorageAvailabilityZonesV3()` factory. This is expected: the task
scope is this data source only; `provider.go` registration is deliberately not
modified. The migrated file itself has no compile errors.

`gofmt -l` on the migrated file produces no output (clean formatting).

---

## Test update (workflow step 7 — TDD gate)

`ProviderFactories: testAccProviders` (SDKv2 map) is replaced with:

```go
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
```

`testAccProtoV6ProviderFactories` does not yet exist in the repo (the provider has
not been wired to the framework). Running the test at this point produces a compile
error on the undefined symbol — the expected "red" state per the TDD gate. Once the
provider is wired (`providerserver.NewProtocol6WithError`), the test will turn green.

---

## What is NOT changed

- `provider.go` registration (out of scope)
- Any other data source or resource
- The original source files in the openstack repo (read-only)
