# Migration notes — `openstack_blockstorage_availability_zones_v3`

## Scope

Single data source migration only:
- `openstack/data_source_openstack_blockstorage_availability_zones_v3.go`
- `openstack/data_source_openstack_blockstorage_availability_zones_v3_test.go`

Per the task brief, no other files in `terraform-provider-openstack` were touched.

## Decisions

### 1. `id` is now declared explicitly

SDKv2 datasources get an implicit `id` attribute from `d.SetId(...)`. The
framework requires every state attribute to be declared on the schema, so I
added `"id": schema.StringAttribute{Computed: true}` to the schema and a
matching `ID types.String` field on the model. The user-facing schema name
(`id`) is unchanged, so no state-breaking change.

### 2. `state` attribute — `Default: "available"` and `StringInSlice(..., true)`

SDKv2:
```go
"state": {
    Type:         schema.TypeString,
    Default:      "available",
    Optional:     true,
    ValidateFunc: validation.StringInSlice([]string{"available", "unavailable"}, true),
},
```

Two changes:

- **Validator**: the second arg `true` to `StringInSlice` means
  case-insensitive match; the framework equivalent is
  `stringvalidator.OneOfCaseInsensitive("available", "unavailable")` (from
  `terraform-plugin-framework-validators`). Using plain `OneOf` would be a
  silent behaviour change.
- **Default**: data sources in the framework do **not** support attribute-
  level defaults (the `defaults` package only applies to resource attributes).
  I emulated the default in `Read()`: if the practitioner did not set
  `state`, it is treated as `"available"`, and the resolved value is written
  back to state. To allow the resolved value to land in state, the attribute
  is now both `Optional: true` and `Computed: true` (matching the existing
  `region` attribute pattern in the same data source).

### 3. `region` — same Optional+Computed pattern preserved

Already `Optional: true, Computed: true` in SDKv2. Kept identical. The
provider-level fallback is implemented in `Read()` rather than via
`GetRegion(d, config)` because the SDKv2 helper takes `*schema.ResourceData`,
which doesn't exist in the framework. The behaviour is equivalent.

### 4. `names` — `TypeList` of strings → `ListAttribute`

Straightforward `schema.ListAttribute{ElementType: types.StringType,
Computed: true}`. The order is sorted lexically (preserved from SDKv2).

### 5. `Configure` plumbing

Added the standard framework `Configure` method that pulls `*Config` out of
`req.ProviderData`. This presumes the framework version of the OpenStack
provider passes `*Config` through `provider.ConfigureResponse.DataSourceData`
(or both `DataSourceData` and `ResourceData`). When the provider itself is
migrated, that wiring needs to exist; until then this data source's
`Configure` will simply early-return because `req.ProviderData == nil`,
which would surface as a nil-pointer panic in `Read()`. See "Caveats" below.

### 6. `id` semantics — `hashcode.Strings(zones)`

Preserved exactly. `hashcode.Strings` returns a string already, so
`types.StringValue(hashcode.Strings(zones))` is a one-line swap.

## Test file changes

Switched `ProviderFactories: testAccProviders` to
`ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`. The rest of the
test (the regex check on `names.#`, the precheck functions, the HCL config)
is unchanged because none of it is SDKv2-specific.

This is the **TDD red gate (workflow step 7)**: on a still-SDKv2 tree,
`testAccProtoV6ProviderFactories` is not yet defined, so the test file fails
to compile. That compile failure is exactly the expected failure shape
quoted in `references/testing.md`. Once the provider is wired up to serve
this data source via the framework (single-release-cycle step 3), the
symbol will exist and the test will go green.

## Caveats / `go.mod` and provider-wiring recommendations

The task forbids modifying anything else in the repo, but a real migration
would need the following to land alongside the file changes above:

1. **`go.mod` adds:**
   ```
   require (
       github.com/hashicorp/terraform-plugin-framework v1.16.0
       github.com/hashicorp/terraform-plugin-framework-validators v0.18.0
   )
   ```
   (Use the latest stable releases compatible with the rest of the
   provider's modules; the audit checklist tracks framework-floor.)

2. **A framework provider scaffolding** that:
   - serves protocol v6 via `providerserver.NewProtocol6WithError`
   - in its `DataSources` method, returns
     `[]func() datasource.DataSource{NewBlockStorageAvailabilityZonesV3DataSource}`
   - in its `Configure` method, sets `resp.DataSourceData = cfg` (and
     `resp.ResourceData = cfg`) so this data source's `Configure` gets the
     `*Config` it needs.

3. **Test scaffolding** in `provider_test.go` that defines
   `testAccProtoV6ProviderFactories`:
   ```go
   var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
       "openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider("test")()),
   }
   ```
   plus the `tfprotov6` and `providerserver` imports.

4. **Mux** if the rest of the provider stays SDKv2 during a single
   in-tree migration cycle — but the skill's scope is the
   single-release-cycle workflow, not multi-release mux. If the user
   wants a mux-based migration the task should be re-scoped per
   `SKILL.md`'s "Does NOT apply" section.

5. **The original SDKv2 data source registration** (a line of the form
   `"openstack_blockstorage_availability_zones_v3": dataSourceBlockStorageAvailabilityZonesV3()`
   in the SDKv2 provider's `DataSourcesMap`) must be removed when the
   framework wiring lands — otherwise both implementations would race and
   Terraform Core would error on the duplicate registration.

## What I did NOT change

- No edits to `provider.go`, `util.go`, `provider_test.go`, or any other
  data source / resource file. The task brief explicitly forbade it.
- No `go.mod` edits (per rule 3 — the openstack clone is read-only).
- No build / vet / test commands run against the openstack clone (per
  rule 3).

## Verification I would run on a follow-up branch

Per `scripts/verify_tests.sh`:

```sh
bash <skill-path>/scripts/verify_tests.sh \
  /path/to/terraform-provider-openstack \
  --migrated-files \
    openstack/data_source_openstack_blockstorage_availability_zones_v3.go \
    openstack/data_source_openstack_blockstorage_availability_zones_v3_test.go
```

The negative gate (rule 7) requires neither file to import
`github.com/hashicorp/terraform-plugin-sdk/v2`. I confirmed neither
migrated file does.
