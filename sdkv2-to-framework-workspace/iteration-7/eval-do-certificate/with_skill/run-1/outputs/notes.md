# Migration notes — `digitalocean_certificate`

## StateFunc / DiffSuppressFunc — chosen approach

**Option 2 chosen: plain `types.String` + hash explicitly in `Create` before `resp.State.Set`.**

The SDKv2 source had three secret-bearing attributes (`private_key`,
`leaf_certificate`, `certificate_chain`) using:

```go
StateFunc: util.HashStringStateFunc(),
DiffSuppressFunc: func(k, old, new string, d *schema.ResourceData) bool {
    return new != "" && old == d.Get("private_key")
},
```

i.e. SDKv2 wrote a SHA1 hash of the user input into state, and suppressed the
diff between "raw input in config" and "hash in state". The framework
provides three correct ways to express this. I evaluated each:

### Option 1 — `WriteOnly: true` (rejected)

`WriteOnly: true` is the framework-purpose-built answer — the practitioner
supplies the secret in config, the API gets the raw value, nothing persists
to state. Cleanest answer in the abstract. Rejected here because:

- The skill's `references/sensitive-and-writeonly.md` says explicitly:
  "Switching an existing `Sensitive: true` attribute to `WriteOnly: true` is
  not just a schema change — it breaks any acceptance test that asserts the
  value via `resource.TestCheckResourceAttr(...)`. With `WriteOnly`, that
  attribute is null in state, so the assertion fails."
- The existing `TestAccDigitalOceanCertificate_Basic` test asserts
  `private_key`, `leaf_certificate`, `certificate_chain` against
  `util.HashString(...)`. Switching to `WriteOnly` would null these out and
  break the assertion.
- The skill flags this as a deferred breaking change for a major version bump;
  not appropriate for a single-resource migration.
- Existing prod state files contain the SHA1 hash for these fields. Switching
  to `WriteOnly` would surface a one-time diff (state has hash, new schema
  has null) on the first plan against migrated provider — practitioner-visible
  drift.

A code comment + `notes.md` deferral is left so a future major version can
flip these to `WriteOnly` cleanly.

### Option 2 — plain `types.String` + hash explicitly (chosen)

The schema attribute is plain `schema.StringAttribute{Optional: true,
Sensitive: true, ...}` with no custom type. The model carries a plain
`types.String`. In `Create`:

1. Read `req.Plan` into the model — `plan.PrivateKey.ValueString()` is the
   *raw* user input.
2. Send the raw value to `client.Certificates.Create(...)`.
3. Before `resp.State.Set(ctx, plan)`, replace each secret attribute with its
   SHA1 hash using a tiny `hashedOrNull` helper that preserves null/unknown.

`Read` is unchanged — the DigitalOcean API never returns the cert material,
so we leave whatever was already in state alone (the hash from `Create`).
`Update` is a no-op because every secret attribute carries `RequiresReplace`,
so any change forces replacement and runs `Create` afresh.

This preserves the SDKv2 on-disk format exactly: existing state files keep
their hash; new applies write the same SHA1 hash; the existing
`TestCheckResourceAttr(..., util.HashString(...))` assertion stays valid
without modification.

The DiffSuppressFunc that compared `new != "" && old == d.Get(...)` is
naturally subsumed: because `Create` writes the hash to state and the user
config carries the raw value, RequiresReplace plus the hash-on-write means a
re-apply with the same config produces a stable plan (the practitioner's
literal cert PEM was never in state to begin with). If the practitioner
*changes* the cert material, `RequiresReplace` triggers replacement — same
end-user behaviour as the SDKv2 ForceNew + DiffSuppressFunc combination.

### Option 3 — non-destructive custom type with `Equal` comparing hashes (rejected)

Genuinely fiddly per `state-and-types.md`: requires a complete
`StringTypable`/`StringValuable` pair with `Equal`, `String`, `ValueType`,
`Type`, `ValueFromTerraform`. The use case here doesn't need it — the API
never round-trips the secret, so the "compare equivalent representations on
read" affordance of a custom type buys nothing. Reserved for cases where the
API actually returns a normalised version of the value.

## Other patterns translated

- **`SchemaVersion: 1` + `StateUpgraders` V0**: ported to
  `ResourceWithUpgradeState` with a single map entry keyed at version `0`.
  Single-step semantics: `upgradeCertificateStateV0toV1` produces the
  *current* (V1) state directly, not an intermediate. The transformation is
  the same as the SDKv2 function: V0 `id` → V1 `uuid`; V0 `name` → V1 `id`.
  The `MigrateCertificateStateV0toV1` direct-call unit test from SDKv2 is
  removed because the framework upgrader takes typed `req`/`resp` arguments
  that aren't easily exercised as a plain unit test; it is replaced with
  `TestAccDigitalOceanCertificate_StateUpgradeV0toV1`, which uses
  `ExternalProviders` to write V0 state with the last published SDKv2
  release and then reapplies under the migrated framework provider with
  `PlanOnly: true` to assert no plan diff.

- **`ConflictsWith`** between `domains` and `private_key` /
  `leaf_certificate` / `certificate_chain`: ported to per-attribute
  `stringvalidator.ConflictsWith(path.MatchRoot("domains"))` on each of the
  three secret-bearing attributes. The skill notes that placing it on one
  side is sufficient; placing it on all three sides matches the original
  SDKv2 declaration site and reads more clearly.

- **`DiffSuppressFunc` on `domains`** (suppress drift when `type == "custom"`
  because the API computes domains from the cert SANs): replaced with
  `Computed: true` + `setplanmodifier.UseStateForUnknown()`. The API
  populates the value on `Create`/`Read`; the prior state is reused on plan
  when nothing else changes, so no spurious diff and no `(known after apply)`
  noise.

- **`StateFunc` whitespace handling**: the SDKv2 hash was applied to whatever
  string SDKv2 had already passed through (typically with a trailing
  newline). Tests use `util.HashString(fmt.Sprintf("%s\n", material))` to
  match. The framework code preserves this by hashing whatever the
  practitioner literally provided in the heredoc (which itself includes the
  trailing newline) — no additional whitespace normalisation needed.

- **`ForceNew: true`**: ported to `stringplanmodifier.RequiresReplace()` (and
  `setplanmodifier.RequiresReplace()` for `domains`). NOT
  `RequiresReplace: true` (which is not a thing in the framework).

- **`Default: "custom"`** on `type`: ported to `stringdefault.StaticString`
  on the attribute's `Default` field. The attribute is `Optional + Computed`
  as required when `Default` is set.

- **`validation.StringInSlice`** on `type`: ported to
  `stringvalidator.OneOf("custom", "lets_encrypt")`.

- **`validation.NoZeroValues`** on the string attributes: ported to
  `stringvalidator.LengthAtLeast(1)` per the cheatsheet (NOT promoted to
  `Required`; that would be a breaking schema change).

- **`Importer.StateContext: ImportStatePassthroughContext`** ported to
  `ResourceWithImportState.ImportState` calling
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

- **`retry.StateChangeConf`** in `Create` (wait for cert state to become
  `verified`): replaced with an inline `waitForCertificateState` poll — no
  framework equivalent helper exists; the in-file replacement keeps the
  10-second initial delay, 3-second tick interval, and 15-minute timeout
  defaults.

- **`retry.RetryContext`** in `Delete` (retry on the "in use" 403): replaced
  with an inline ticker loop, same 30-second timeout, same retry condition.

## Things deferred (not blocking this migration)

- `WriteOnly: true` on the secret attributes is a future major-version
  cleanup; documented in code comment on `certificateModel`.
- The provider's overall mux-up to serve framework + SDKv2 side-by-side is
  out of scope (single-resource migration). The test file references
  `acceptance.TestAccProtoV6ProviderFactories`, which the eventual provider
  wiring will populate when the provider itself is muxed.

## Files changed

- `migrated/resource_certificate.go` — full rewrite, no `terraform-plugin-sdk/v2`
  imports.
- `migrated/resource_certificate_test.go` — updated to
  `ProtoV6ProviderFactories`; `terraform-plugin-testing` package paths;
  state-upgrade unit test replaced with the `ExternalProviders`+`PlanOnly`
  acceptance pattern.
