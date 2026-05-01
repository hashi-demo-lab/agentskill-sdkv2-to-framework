# Migration Summary: resource_certificate.go (SDKv2 → Framework)

## Patterns migrated

### SchemaVersion 1 with V0 upgrader → ResourceWithUpgradeState
- Implemented `resource.ResourceWithUpgradeState` with a single map entry keyed `0`.
- `PriorSchema` is defined via `priorSchemaV0()` matching the V0 schema shape (no `uuid` field; `id` = API UUID).
- The upgrader performs the V0→V1 transformation in one step: `id` becomes the cert name (stable across renewals), `uuid` receives the old API UUID.
- `MigrateCertificateStateV0toV1` is kept as a free function to preserve the existing unit test.

### StateFunc (hash) → hash on Create + hashSuppressPlanModifier
- SDKv2 `StateFunc: util.HashStringStateFunc()` on `private_key`, `leaf_certificate`, `certificate_chain` stored a SHA1 hash in state.
- Framework pattern: raw values flow through `Create`, are hashed explicitly (`util.HashString()`), and the hash is written to state via `resp.State.Set`.
- A custom `hashSuppressPlanModifier` suppresses diffs when the configured raw value hashes to the value already in state — matching the SDKv2 `DiffSuppressFunc` that handled old statefiles with fully-saved PEM values.
- No destructive `ValueFromString` custom type was used (avoided per skill guidance).

### DiffSuppressFunc on domains → domainsSuppressPlanModifier
- SDKv2 `DiffSuppressFunc` suppressed the `domains` diff when `type == "custom"`.
- Framework pattern: custom `domainsSuppressPlanModifier` reads the `type` attribute from `req.Config` and keeps the prior state value when `type == "custom"`.

### ConflictsWith → setvalidator.ConflictsWith
- `domains.ConflictsWith: []string{"private_key", "leaf_certificate", "certificate_chain"}` → `setvalidator.ConflictsWith(path.MatchRoot("private_key"), path.MatchRoot("leaf_certificate"), path.MatchRoot("certificate_chain"))`.

### ForceNew → RequiresReplace plan modifier
- All `ForceNew: true` fields converted to `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` (or `setplanmodifier.RequiresReplace()` for `domains`).

### Default → stringdefault package
- `Default: "custom"` → `Default: stringdefault.StaticString("custom")` with `Computed: true`.

### retry.StateChangeConf → inline waitForCertificateState
- Replaced `retry.StateChangeConf` with a local `waitForCertificateState` function using a context-aware ticker loop.
- No SDKv2 `helper/retry` import remains.

### Importer → ResourceWithImportState
- `schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Test file changes
- `ProviderFactories` → `ProtoV6ProviderFactories` (local `protoV6ProviderFactories` map referencing `acceptance.NewFrameworkProvider()`).
- Import changed from `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` to `github.com/hashicorp/terraform-plugin-testing/helper/resource`.
- Destroy/exists check functions replaced `TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()` with a direct `godo.Client` constructed from `DIGITALOCEAN_TOKEN` — necessary because the framework's protocol boundary does not expose provider meta to check functions.

## Notes
- `FindCertificateByName` (defined in `datasource_certificate.go`) is unchanged and still exported.
- The framework provider (`acceptance.NewFrameworkProvider()`) and `acceptance.TestAccProtoV6ProviderFactories` must be added to the acceptance package as part of the broader provider migration.
- The `util.HashString` and `util.HashStringStateFunc` functions remain in their current package; only `HashStringStateFunc` is no longer used (its logic is inlined into `hashSuppressPlanModifier`).
