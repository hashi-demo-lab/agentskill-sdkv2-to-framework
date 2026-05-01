# Migration Summary: digitalocean_vpc SDKv2 → terraform-plugin-framework

## Source file
`digitalocean/vpc/resource_vpc.go`

## Pre-flight checks

### Pre-flight 0 — Mux check
No mux, staged, or phased migration requested. Single-release path confirmed.

### Pre-flight A — Audit (manual, semgrep unavailable)
Patterns found in source:

| Pattern | Finding |
|---|---|
| `ForceNew` | `region`, `ip_range` — must become `RequiresReplace` plan modifiers |
| `Computed` attrs | `urn`, `default`, `created_at`, `ip_range`, `description` — need `UseStateForUnknown` |
| `Importer` | Simple passthrough (`ImportStatePassthroughContext`) — maps to `ImportStatePassthroughID` |
| `Timeouts` | Delete only (2m default) — maps to `terraform-plugin-framework-timeouts` |
| `retry.RetryContext` | Used in Delete — no framework equivalent; inlined as a context-aware ticker loop |
| `ValidateFunc: validation.NoZeroValues` | → `stringvalidator.LengthAtLeast(1)` |
| `ValidateFunc: validation.StringLenBetween(0,255)` | → `stringvalidator.LengthBetween(0, 255)` |
| `ValidateFunc: validation.IsCIDR` | → `stringvalidator.RegexMatches` (CIDR regex) |
| `SchemaVersion` | Not present — no state upgrader needed |

### Pre-flight C — Think pass
1. **Block decision**: No `MaxItems: 1` nested blocks — not applicable.
2. **State upgrade**: No `SchemaVersion` — not applicable.
3. **Import shape**: Simple passthrough importer — maps directly to `resource.ImportStatePassthroughID`.

## Changes made

### resource_vpc.go

- Replaced `func ResourceDigitalOceanVPC() *schema.Resource` with a struct type `vpcResource` implementing `resource.Resource`.
- Added `ResourceWithConfigure` to wire the `*godo.Client` from provider data.
- Added `ResourceWithImportState` with `ImportStatePassthroughID`.
- Schema migrated:
  - `name`: `StringAttribute{Required, Validators: LengthAtLeast(1)}`
  - `region`: `StringAttribute{Required, RequiresReplace, Validators: LengthAtLeast(1)}`
  - `description`: `StringAttribute{Optional, Computed, UseStateForUnknown, Validators: LengthBetween(0,255)}`
  - `ip_range`: `StringAttribute{Optional, Computed, UseStateForUnknown, RequiresReplace, Validators: RegexMatches(CIDR)}`
  - `urn`: `StringAttribute{Computed, UseStateForUnknown}`
  - `default`: `BoolAttribute{Computed}` (no UseStateForUnknown — the API may update this)
  - `created_at`: `StringAttribute{Computed, UseStateForUnknown}`
  - `timeouts`: `timeouts.Block(ctx, Opts{Delete: true})` in `Blocks` map
- Typed model struct `vpcResourceModel` with `tfsdk:` tags.
- CRUD methods ported:
  - `Create`: reads plan, calls API, writes state directly from API response (no separate Read call).
  - `Read`: reads state ID, fetches VPC, calls `resp.State.RemoveResource` on 404.
  - `Update`: reads plan+state, calls Update API only when name/description changed (preserving `default` from state as the API requires it), then re-reads from API.
  - `Delete`: reads `state.Timeouts.Delete(ctx, 2*time.Minute)` for the timeout, inlines a ticker-based retry loop replacing `retry.RetryContext` (retries on 403/409, fails immediately on other errors).
- Removed all `terraform-plugin-sdk/v2` imports.

### resource_vpc_test.go

- Merged `resource_vpc_test.go` and `import_vpc_test.go` into a single file.
- Swapped `ProviderFactories: acceptance.TestAccProviderFactories` → `ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories`.
- Swapped `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` → `github.com/hashicorp/terraform-plugin-testing/helper/resource`.
- Swapped `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` → `github.com/hashicorp/terraform-plugin-testing/terraform`.
- Import test (`TestAccDigitalOceanVPC_importBasic`) moved from separate file into `resource_vpc_test.go`.
- All HCL test configs unchanged (no user-visible schema names were modified).

## Outstanding action required (not in migrated output)

The `acceptance` package (`digitalocean/acceptance/acceptance.go`) needs a new export:

```go
var TestAccProtoV6ProviderFactories map[string]func() (tfprotov6.ProviderServer, error)

func init() {
    TestAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
        "digitalocean": providerserver.NewProtocol6WithError(/* new framework provider */),
    }
}
```

This requires the provider's `main.go` and provider type to be migrated first (steps 3–5 of the workflow). The resource migration (step 6+) is complete; the test file is written for the target state.

## Verification

The following would need to pass before marking step 9 green:

1. `go build ./...` — no SDKv2 imports remain in migrated files.
2. `go vet ./...` — no vet errors.
3. Non-acceptance unit tests.
4. Negative gate: no `terraform-plugin-sdk/v2` substring in `resource_vpc.go`.
5. (With `TF_ACC`) acceptance tests against a live DigitalOcean account.
