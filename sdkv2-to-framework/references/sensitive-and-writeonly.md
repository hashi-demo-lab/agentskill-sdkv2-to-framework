# Sensitive and write-only attributes

> **Which one do I want?** Ask: *does Terraform need to read this value back later?* (drift detection, cross-resource references, import-verify, etc.) If **yes** → `Sensitive: true` only; the value lives in state, redacted from output. If **no** (one-time creds, initial passwords, rotation seeds) → `Sensitive: true` AND `WriteOnly: true`; the value never persists. The default for migrations is **Sensitive only** — flipping to `WriteOnly` is a practitioner-visible breaking change (see "Sensitive → WriteOnly is a practitioner-test breaking change too" below). Don't preemptively upgrade.

## Quick summary
- `Sensitive: true` is unchanged in spelling and behaviour: the value is hidden in plans/logs but still stored in state.
- The framework adds **write-only** attributes (`WriteOnly: true`) — supplied by the practitioner but never persisted to state. Useful for credentials Terraform doesn't need to round-trip.
- Write-only requires `terraform-plugin-framework` **v1.14.0+** (technical preview through v1.19) and Terraform **1.11+**. Pin the framework as recent as your other constraints allow.
- **`WriteOnly` and `Computed` cannot coexist on the same attribute.** A write-only value isn't persisted; making it computed would need the framework to materialise a value in state, which contradicts write-only's whole point. The framework rejects this at provider boot via `ValidateImplementation`. If you need a "default-or-rotate" pattern, use a separate computed attribute that mirrors the rotation source rather than trying to mix the two on one attribute.
- **Nested `WriteOnly` cascades.** If a parent (`SingleNestedAttribute`, `ListNestedAttribute`, `MapNestedAttribute`) is `WriteOnly`, every child must also be `WriteOnly`, and none may be `Computed`. `SetNestedAttribute` does not support `WriteOnly` at all (the framework rejects sets containing write-only children). All violations are caught at provider boot via `ValidateImplementation`.
- Sensitive attributes still appear in `terraform show -json`, just redacted in human-readable output. Don't rely on `Sensitive` alone for security.
- When migrating, sweep for SDKv2 patterns where `Sensitive: true` was being used as a poor man's write-only (e.g., setting state to a hash placeholder); these may be candidates for the new write-only.

## Sensitive — unchanged

```go
"api_key": schema.StringAttribute{
    Required:  true,
    Sensitive: true,
}
```

Same effect as SDKv2: the value is redacted from plan output and logs (`<sensitive>`), but is still in state. Practitioners who run `terraform show -json` see the value as a sensitive-marked field; `terraform show` (text) redacts it.

## Sensitive does NOT mean encrypted

State is still plaintext on disk. If true secrecy matters, use a remote backend with encryption-at-rest, or a secrets manager fronting Terraform.

## Write-only — new in framework

Write-only attributes are supplied by config but never persisted. They're for inputs Terraform doesn't need to read back: short-lived credentials, one-time tokens, ephemeral seed values.

```go
"initial_password": schema.StringAttribute{
    Required:  true,
    Sensitive: true,
    WriteOnly: true,
}
```

In CRUD methods, the value is in `req.Config` (where the practitioner wrote it) but **not** in `req.State`. Don't try to read it from state in `Read` — it isn't there. Use it during `Create`/`Update` and let it be ephemeral.

```go
func (r *thingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var config thingModel
    resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
    // config.InitialPassword.ValueString() — usable here
    // ... call API ...
    var plan thingModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    // plan.InitialPassword is null — that's correct, it's write-only
    resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}
```

## Sensitive → WriteOnly is a *practitioner-test* breaking change too

Switching an existing `Sensitive: true` attribute to `WriteOnly: true` is not just a schema change — it breaks any acceptance test that asserts the value via `resource.TestCheckResourceAttr("myprov_thing.x", "secret_value", "...")`. With `WriteOnly`, that attribute is null in state, so the assertion fails.

This bites practitioners who have downstream tests against their own provider. The migration's "don't change user-facing schema names" rule extends here: switching to `WriteOnly` should be treated as a breaking change deferred to a major version bump unless you've audited every test that reads the attribute.

If you decide to defer the Sensitive→WriteOnly upgrade to a follow-up, write the deferral down — drop a comment in the migrated source citing the upgrade path, and add a CHANGELOG note. Otherwise the signal that "this could be tightened later" gets lost.

## Migration cue: SDKv2 hash-placeholder pattern

A common SDKv2 pattern for "I don't want to round-trip this":

```go
// SDKv2 — hash the secret into state so the diff is stable
"api_token": {
    Type:      schema.TypeString,
    Required:  true,
    Sensitive: true,
    StateFunc: func(v interface{}) string { return hashString(v.(string)) },
}
```

This was a workaround. In the framework, just use `WriteOnly: true` and drop the hashing entirely:

```go
"api_token": schema.StringAttribute{
    Required:  true,
    Sensitive: true,
    WriteOnly: true,
}
```

The practitioner provides the value in config; Terraform never persists it; subsequent plans won't show diffs because there's no state value to compare against.

## When NOT to use write-only

If you need to read the value back later (to detect drift, to refer to it in another resource via `data.myprov_x.attr`), do **not** use `WriteOnly`. The whole point is that it's not in state.

## Hard rules (all enforced at provider boot)

The framework's `ValidateImplementation` runs during `GetProviderSchema` and rejects violations of all four rules below before practitioners can configure the provider. Your `TestProvider` test (which boots the provider) will surface them.

1. **`WriteOnly` and `Computed` cannot coexist on the same attribute.** Rejected at boot.
2. **A top-level `WriteOnly` attribute must be `Required` or `Optional`** — not pure `Computed`. Rejected at boot.
3. **Nested `WriteOnly` cascade.** Under a `WriteOnly` `SingleNestedAttribute`, `ListNestedAttribute`, or `MapNestedAttribute`, every child must also be `WriteOnly` and none may be `Computed`. The framework's boot-time validators emit `InvalidWriteOnlyNestedAttributeDiag` / `InvalidComputedNestedAttributeWithWriteOnlyDiag`. **`SetNestedAttribute` has no `WriteOnly` field at all** — the framework rejects sets containing any `WriteOnly` child. If your secret lives inside a set-shaped attribute, restructure to a list or single nested.
4. **Pair `WriteOnly` with `ImportStateVerifyIgnore` in tests.** See "Test treatment" below.

A `TestProvider` test catches all four — no separate acceptance step is needed for nested-cascade verification.

## Test treatment

Sensitive values still appear in test step assertions; you can `resource.TestCheckResourceAttr` against them as normal.

For write-only, the value isn't in state — assertions on the attribute will return null. **Always** pair write-only attributes with `ImportStateVerifyIgnore` so import-verify steps don't fail trying to compare the absent value:

```go
resource.TestStep{
    ResourceName:            "myprov_thing.test",
    ImportState:             true,
    ImportStateVerify:       true,
    ImportStateVerifyIgnore: []string{"initial_password"},
}
```

This is not optional — without it, `ImportStateVerify` fails because the importer reads back a null where the practitioner wrote a value.
