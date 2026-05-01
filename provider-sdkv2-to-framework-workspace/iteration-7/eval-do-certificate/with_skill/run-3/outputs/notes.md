# `digitalocean_certificate` — SDKv2 → Plugin Framework migration notes

## Approach summary

The resource has four migration hot-spots. Each is handled with the framework
idiom from the bundled references rather than a 1:1 SDKv2 port:

| SDKv2 element | Framework treatment | Reference |
|---|---|---|
| `StateFunc: util.HashStringStateFunc()` on `private_key` / `leaf_certificate` / `certificate_chain` | Plain `types.String` attributes; raw value flows from `req.Config` to the API call in `Create`; SHA-1 hash written to state via `resp.State.Set` (option **2** from `state-and-types.md`'s "Destructive `StateFunc`" guidance — explicitly NOT a custom-type-with-hashing-`ValueFromString`) | `references/state-and-types.md` |
| `DiffSuppressFunc` on the same three attributes (carry old hash forward when new config hashes to it) | Custom `hashSuppressPlanModifier` that runs **before** `RequiresReplace()` — when `HashString(req.ConfigValue) == req.StateValue`, it sets `resp.PlanValue = req.StateValue` so `RequiresReplace` sees no change. | `references/plan-modifiers.md` ("DiffSuppressFunc — not directly portable") |
| `ConflictsWith` on `domains` | `setvalidator.ConflictsWith(path.MatchRoot(...))` (variadic, no `path.Expressions{}` wrapper) | `references/validators.md` |
| `DiffSuppressFunc` on `domains` (suppress when `type == "custom"`) | Custom `customTypeDomainsSuppressPlanModifier` that pulls state forward when planned `type` is `"custom"` | `references/plan-modifiers.md` |
| `SchemaVersion: 1` + chained-style upgrader V0→V1 | `resource.ResourceWithUpgradeState` with one map entry at key `0`; typed `certificateModelV0` matching `priorSchemaV0()`; transformation produces the **current** model directly (not a V1 intermediate). The free function `MigrateCertificateStateV0toV1` is preserved unchanged so the existing raw-map unit test still compiles. | `references/state-upgrade.md` |
| `Importer: ImportStatePassthroughContext` | `resource.ResourceWithImportState.ImportState` calls `resource.ImportStatePassthroughID(ctx, path.Root("id"), ...)` | `references/import.md` |
| `helper/retry.StateChangeConf.WaitForStateContext` | Inline `waitForCertificateState` polling helper — no framework equivalent and the SDKv2 helper must not be re-imported (negative gate). | `references/resources.md` |

## Why NOT a destructive custom type

`references/state-and-types.md` calls this out as a load-bearing trap:

> A common SDKv2 pattern is `StateFunc: hashString` on a secret attribute … It's tempting to translate this directly to a custom type whose `ValueFromString` hashes the input. **Don't.** `CustomType` is wired at the schema level, so `req.Config`, `req.Plan`, and `req.State` all decode through `ValueFromString` — there is no `req.Plan` value with the unhashed original.

In other words: a custom type that hashes in `ValueFromString` would also hash
the value coming out of `req.Config`, so by the time `Create` reads the secret
the API request would carry the SHA-1 hash and silently fail upstream.

Three correct patterns are listed in the reference. We pick **option 2**
(plain `types.String` + hash in CRUD) over options 1 (`WriteOnly`) and 3
(non-destructive custom type) because:

- **Option 1 (WriteOnly)** would make `private_key`/`leaf_certificate`/
  `certificate_chain` null in state. Existing acceptance tests assert against
  the hashed value (`util.HashString(fmt.Sprintf("%s\n", material))`), so
  flipping to `WriteOnly` is a practitioner-test breaking change called out
  explicitly in `references/sensitive-and-writeonly.md` ("Sensitive →
  WriteOnly is a *practitioner-test* breaking change too"). The migration
  rule is to keep schema names and observable values stable; deferring the
  WriteOnly upgrade to a major-version bump is the documented default.
- **Option 3 (non-destructive custom type)** would work but is "genuinely
  fiddly" per the reference — and unnecessary here because option 2 is
  straightforward.

## Plan modifier ordering (load-bearing)

`hashSuppressPlanModifier` MUST appear before `stringplanmodifier.RequiresReplace()`
in the `PlanModifiers` slice. From `references/plan-modifiers.md`:

> Plan modifiers on an attribute run in the order they appear in the slice.

If `RequiresReplace` ran first, it would compare the raw plan value
(`-----BEGIN PRIVATE KEY-----\n…`) against the stored hash and unconditionally
trigger replacement on every plan. Putting the suppressor first lets it
collapse `plan := state` first; `RequiresReplace` then sees no change.

## State upgrader — single-step semantics

`references/state-upgrade.md` is explicit that an SDKv2 `StateUpgraders` chain
becomes one framework upgrader **per prior version**, and each upgrader
produces the **current** state directly — not an intermediate-version state.

Here there's only one prior version (V0), so the framework upgrader at key
`0` produces the current (V1) state directly. The transformation
(`id = name`, `uuid = old id`) is identical to the raw-map version retained
in `MigrateCertificateStateV0toV1`.

The unit test (`TestResourceExampleInstanceStateUpgradeV0`) still drives the
free-function form because that test exercises raw-map semantics rather than
typed framework state — keeping the helper avoids a test rewrite. The typed
upgrader is the production path; the helper documents the underlying
transformation for the test.

## ConflictsWith — set validator placement

`domains` ConflictsWith covers `private_key`, `leaf_certificate`, and
`certificate_chain`. Per `references/validators.md`, `ConflictsWith` is
variadic and only needs to live on one of the conflicting attributes. We
keep it on `domains` only (matching the SDKv2 layout).

## Things I deliberately did NOT do

1. **Did not switch to `WriteOnly`** for the cert material. See the rationale
   above — it's a practitioner-test breaking change. A follow-up major
   version bump should consider it.
2. **Did not delete `MigrateCertificateStateV0toV1`.** It's the surface that
   the existing unit test calls. The framework upgrader is added alongside,
   not in place of.
3. **Did not add a `Timeouts` block.** SDKv2 used `d.Timeout(schema.TimeoutCreate)`
   which falls back to the `helper/schema` default (20m); the migrated code
   uses an inline 5-minute deadline for `waitForCertificateState`. If users
   want a longer timeout, the framework's `terraform-plugin-framework-timeouts`
   package is the documented path (`references/timeouts.md`) — left as a
   follow-up rather than scope-creeping this migration.
4. **Did not change schema names.** `id`, `uuid`, `name`, `private_key`,
   `leaf_certificate`, `certificate_chain`, `domains`, `type`, `state`,
   `not_after`, `sha1_fingerprint` — all preserved verbatim.
5. **Did not import `terraform-plugin-sdk/v2`** in the migrated source. The
   negative gate from `verify_tests.sh` requires this; switched to
   `terraform-plugin-testing/helper/resource` and
   `terraform-plugin-testing/terraform` in the test file.

## Out-of-file dependencies

This migration assumes the parent provider is wired to expose:

- `acceptance.TestAccProtoV6ProviderFactories` — the new framework factory.
  Not present in `acceptance/acceptance.go` today (only the SDKv2
  `TestAccProviderFactories` is). The test file references the v6 factory
  per the framework migration step 3 default; the eval scope is one
  resource, so adding the factory to the acceptance package is part of the
  broader provider migration, not this resource file.
- `NewCertificateResource()` — the resource constructor must be wired into
  the framework provider's `Resources()` slice (provider-level work covered
  by `references/provider.md`, again outside this resource's scope).
- `acceptance.GenerateTestCertMaterial(t)` — already used by the SDKv2 test;
  unchanged.
