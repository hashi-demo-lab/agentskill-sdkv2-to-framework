# Migration Notes: openstack_blockstorage_availability_zones_v3

## Compile check

The openstack repo was copied to `/tmp/openstack-migration` for compile verification (constraint: do not modify the original repo).

`terraform-plugin-framework` and `terraform-plugin-framework-validators` were added via `go get`. The initial upgrade pulled `terraform-plugin-go` to v0.31.0 which broke the SDKv2 gRPC interface (`GRPCProviderServer` no longer implemented `tfprotov5.ProviderServer`). Fixed by pinning `terraform-plugin-go@v0.29.0`, which also downgraded the framework to v1.17.0 — still fully capable for this migration.

Final versions used:
- `terraform-plugin-framework v1.17.0`
- `terraform-plugin-framework-validators v0.19.0`
- `terraform-plugin-go v0.29.0`

Both `go build ./openstack/...` and `go vet ./openstack/...` pass cleanly.

## Schema decisions

### `state` attribute — Default handling

SDKv2 had `Default: "available"` on the `state` attribute. The framework `datasource/schema.StringAttribute` has no `Default` field (defaults are a resource-schema concept tied to plan modifiers). Instead, the `Read` method applies the default at runtime: if `state` is null/unknown/empty, `stateFilter` is set to `"available"`. The attribute is marked `Optional: true, Computed: true` so Terraform will write back the effective value ("available") to state even when the user omits it — preserving the same round-trip behaviour as the SDKv2 default.

### `names` attribute

`TypeList + Elem: &schema.Schema{TypeString}` → `schema.ListAttribute{ElementType: types.StringType}`. Order is preserved (zones are sorted before storage).

### `region` attribute

`GetRegion(d, config)` (which checked `d.GetOk("region")` then fell back to `config.Region`) is replaced by direct nil-checks on `cfg.Region` in the `Read` method, falling back to `d.config.Region`.

### ID

`d.SetId(hashcode.Strings(zones))` is preserved using the same `github.com/terraform-provider-openstack/utils/v2/hashcode` package, stored in the model's `ID` field.

### Validator

`validation.StringInSlice([]string{"available", "unavailable"}, true)` (case-insensitive) → `stringvalidator.OneOfCaseInsensitive("available", "unavailable")`.

## Test file

The test file switches `ProviderFactories: testAccProviders` to `ProtoV6ProviderFactories: protoV6ProviderFactories`. The `protoV6ProviderFactories` variable references `NewFrameworkProvider("test")()`, which is a placeholder for the framework provider constructor that would be created as part of a full provider migration (step 3 of the 12-step workflow). The test file also adds an assertion that `state` defaults to `"available"` and adds a second test case for the `unavailable` state filter.

The test file will not compile standalone until the full provider is migrated (specifically, `NewFrameworkProvider` must be defined). This is expected at this point in the workflow (step 7: tests are written to fail before the implementation is complete).

## What was NOT migrated

Per the task constraints, only this single data source file was migrated. The provider registration (`DataSourcesMap` in `provider.go`), `provider.go` itself, `util.go`, and all other resources/data sources remain SDKv2.
