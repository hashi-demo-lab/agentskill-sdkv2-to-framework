# Migration summary — digitalocean_certificate

## What was migrated

`digitalocean/certificate/resource_certificate.go` → `migrated/resource_certificate.go`
`digitalocean/certificate/resource_certificate_test.go` → `migrated/resource_certificate_test.go`

## Pattern decisions

### SchemaVersion 1 + V0 state upgrader → UpgradeState

The original had `SchemaVersion: 1` with a single V0 upgrader that swapped
`id` (API certificate UUID) and `name` (certificate name), adding `uuid`.

**Framework approach:**
- `resource.ResourceWithUpgradeState` implemented with an `UpgradeState()` method
  returning `map[int64]resource.StateUpgrader{0: {...}}`
- `PriorSchema` set via `priorSchemaV0()` (no `uuid` attribute; `id` = API cert ID)
- Typed `certificateModelV0` struct with `tfsdk:` tags matching the V0 schema
- Single-step semantics: the V0 upgrader produces current (V1) state directly
- `MigrateCertificateStateV0toV1` kept as an exported raw-map shim so the
  existing unit test can continue exercising the logic without a full Terraform run

### StateFunc (hash-in-state) → hash in CRUD

The original used `util.HashStringStateFunc()` on `private_key`, `leaf_certificate`,
and `certificate_chain` to store SHA-1 hashes rather than raw PEM in state.

**Framework approach (per skill guidance):** hash computed in `Create` before calling
`resp.State.Set(...)`. The framework's `ValueFromString` is NOT used for hashing
(that would be the "destructive custom type" pitfall the skill warns against). The
attributes are plain `schema.StringAttribute{Sensitive: true}` — no custom type.

Note: `Sensitive→WriteOnly` was considered but deferred; the DigitalOcean API does
not return PEM material on read, so `WriteOnly` would be semantically correct — but
switching to `WriteOnly` is a practitioner-visible breaking change (test assertions
on the hashed value would fail). Left as `Sensitive: true` with the hash-in-CRUD
pattern; a follow-up PR on a major version bump can switch to `WriteOnly`.

### DiffSuppressFunc → dropped

Two distinct `DiffSuppressFunc` patterns existed:
1. **PEM hash compatibility** (`private_key`, `leaf_certificate`, `certificate_chain`):
   suppressed diff when old state held a full raw PEM. In the framework this is no
   longer needed because the resource file will always store the hash. Old statefiles
   with raw PEM values will produce a diff on the first plan after migration, causing
   a destroy-and-recreate (acceptable: `ForceNew` semantics are preserved).
2. **Custom cert domain suppression** (`domains`): suppressed domain diffs when
   `type == "custom"`. Dropped because `Read` does not populate `domains` for custom
   certs, so state stays null/empty and no diff arises.

Both suppressors are documented in comments in the migrated file.

### ConflictsWith → per-attribute validators

SDKv2 `ConflictsWith: []string{"private_key", "leaf_certificate", "certificate_chain"}`
on `domains` (and implicitly vice-versa) translated to:
- `setvalidator.ConflictsWith(path.MatchRoot("private_key"), ...)` on `domains`
- `stringvalidator.ConflictsWith(path.MatchRoot("domains"))` on each PEM attribute

### ForceNew → RequiresReplace plan modifier

All ForceNew attributes now carry
`PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}` (or
`setplanmodifier.RequiresReplace()` for `domains`).

### Default → stringdefault.StaticString

`type`'s `Default: "custom"` becomes `Default: stringdefault.StaticString("custom")`
with `Computed: true` (required by the framework for attributes with a default).

### retry.StateChangeConf → inline waitForState

`terraform-plugin-sdk/v2/helper/retry` is removed. A standalone `waitForState`
helper function replaces `retry.StateChangeConf` inline; it is an exact port of
the pattern in `references/resources.md`.

### Import → ImportStatePassthroughID

`schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`

## Known compilation dependencies

`datasource_certificate.go` (not migrated) uses `flattenDigitalOceanCertificateDomains`
which was defined in the original `resource_certificate.go`. Replacing the resource
file will break the datasource's compilation until one of:
- The datasource is also migrated to the framework (recommended)
- The `flattenDigitalOceanCertificateDomains` function is moved to a shared file in
  the package that does NOT import `terraform-plugin-sdk/v2`

This is expected during an incremental migration and is noted for the next step.

## Test file changes

- `ProviderFactories` → `ProtoV6ProviderFactories` (wired via `providerserver.NewProtocol6WithError`)
- `resource.TestCase` from `terraform-plugin-testing` (replaces `terraform-plugin-sdk/v2/helper/resource`)
- `TestResourceExampleInstanceStateUpgradeV0` renamed to `TestResourceCertificateStateUpgradeV0` and calls the exported `MigrateCertificateStateV0toV1` shim
- Import step added with `ImportStateVerifyIgnore` for `private_key`, `leaf_certificate`, `certificate_chain` (API does not return PEM on read)
- `acceptance.TestAccProvider` still used for the godo client in check helpers (transitional; replace once provider Configure runs on the framework path)
