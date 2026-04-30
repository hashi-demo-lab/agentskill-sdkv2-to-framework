# Sensitive and write-only attributes

## Quick summary
- `Sensitive: true` is unchanged in spelling and behaviour: the value is hidden in plans/logs but still stored in state.
- The framework adds **write-only** attributes (`WriteOnly: true`) — supplied by the practitioner but never persisted to state. Useful for credentials Terraform doesn't need to round-trip.
- Write-only requires `terraform-plugin-framework` **v1.14.0+** (technical preview) and Terraform **1.11+**, with v1.17.0+ recommended for production use. The v1.14 preview ships the field; v1.17 stabilises behaviour.
- **`WriteOnly` and `Computed` should not be combined on the same attribute.** A write-only value isn't persisted; making it computed would need the framework to materialise a value in state, which contradicts write-only's whole point. Terraform CLI rejects configurations that use both at apply time. If you need a "default-or-rotate" pattern, use a separate computed attribute that mirrors the rotation source rather than trying to mix the two on one attribute.
- **Nested `WriteOnly` cascades.** If a parent (`SingleNestedAttribute`, `ListNestedAttribute`, etc.) is `WriteOnly`, every child attribute should also be `WriteOnly`, and none should be `Computed`. The framework's `ValidateImplementation` does not currently catch every violation at provider-boot time — so test this end-to-end. Run an acceptance step that exercises the attribute and confirm there's no apply-time error.
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

## Hard rules (most are caught at provider boot; nested cases need acceptance tests)

For top-level scalar attributes, the framework's `ValidateImplementation` enforces the constraints below at provider startup — your `TestProvider` test or any acceptance step will surface violations. For nested cases (a `WriteOnly` parent with non-`WriteOnly` or `Computed` children), Terraform CLI is the backstop at apply time, so test those with a real config-apply step.

1. **`WriteOnly` and `Computed` cannot coexist on the same attribute.** A write-only value isn't persisted; making it computed would need the framework to materialise a value in state, which contradicts write-only's whole point. The framework rejects this at boot for top-level scalars.
2. **A top-level `WriteOnly` attribute must be `Required` or `Optional`** — not pure `Computed`. The framework rejects this at boot.
3. **`WriteOnly` parent cascades to children.** Every nested attribute under a `WriteOnly` `SingleNestedAttribute` / `ListNestedAttribute` / `SetNestedAttribute` / `MapNestedAttribute` should itself be `WriteOnly`, and none should be `Computed`. The framework's boot-time validators cover top-level cases; nested-cascade enforcement falls back to Terraform CLI at apply time — write an acceptance step that exercises the nested attribute to catch this early.
4. **Pair `WriteOnly` with `ImportStateVerifyIgnore` in tests.** See "Test treatment" below.

The minimum end-to-end coverage: a `TestProvider` test (catches the top-level boot-time rejections) plus one acceptance step per write-only attribute that exercises the field with a config apply (catches the nested-cascade cases).

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
