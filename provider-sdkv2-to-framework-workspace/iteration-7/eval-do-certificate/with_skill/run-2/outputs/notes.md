# Migration notes — `digitalocean_certificate`

## StateFunc translation — why we chose **plain `types.String` + explicit hash in CRUD**

The SDKv2 schema attached `StateFunc: util.HashStringStateFunc()` (SHA1 of the input) to three secret-shaped fields: `private_key`, `leaf_certificate`, and `certificate_chain`. The intent was: the practitioner supplies the raw cert/key material in config, but only its hash is ever persisted to state — so a state-file leak does not disclose the original key bytes. A `DiffSuppressFunc` then suppressed plan-time diffs by comparing the new value against the prior state value (the hash) so terraform did not constantly try to replace the resource.

The skill's `references/state-and-types.md` explicitly warns against translating a destructive `StateFunc` into a custom type whose `ValueFromString` hashes the input ("Destructive `StateFunc` — do NOT use a destructive custom type"). The reason is that `CustomType` is wired at the schema layer, so `req.Config`, `req.Plan`, and `req.State` would all decode through the hashing function — by the time `Create` reads the plan, the secret has already been hashed and the API call sends a hash, which fails.

The reference recommends three alternatives, in preference order. We evaluated each:

### Option 1 — `WriteOnly: true` (rejected here)

`WriteOnly` is the framework's purpose-built answer for "secret Terraform doesn't need to read back". It would be ideal for greenfield, but for **this** migration it's a practitioner-visible breaking change:

- The pre-existing acceptance test asserts `resource.TestCheckResourceAttr("digitalocean_certificate.foobar", "private_key", util.HashString(...))` — i.e., it explicitly reads the hashed value back from state. A `WriteOnly` attribute is null in state, so that assertion would fail (this is exactly the practitioner-test breaking change called out in `references/sensitive-and-writeonly.md`).
- Any practitioner module that referenced `digitalocean_certificate.foo.private_key` (e.g., to chain the hash into a downstream `data` source for fingerprint comparison) would suddenly see a null.
- Existing state files would lose the hashed value at next plan, and the `domains` `ConflictsWith` validator would still need to fire on practitioners' configs — that part is unaffected, but the round-trippable state value disappears.

The SDKv2 docs and the migration rule "Don't change user-facing schema names or attribute IDs" (SKILL.md "Common pitfalls") apply here: switching to `WriteOnly` is a state-shape change that should be deferred to a major version bump, not bundled into a refactor migration. We left a comment hook in the migrated file pointing this out.

### Option 2 — plain `types.String` + explicit hash in CRUD ⟵ chosen

This is what the SDKv2 implementation effectively did, just with the hash now applied in our own Go code rather than via the schema layer. The shape:

1. Schema declares `private_key`, `leaf_certificate`, `certificate_chain` as plain `schema.StringAttribute` (with `Optional`, `Sensitive` where appropriate, `RequiresReplace`, `LengthAtLeast(1)` validator).
2. In `Create` we read the raw plan value (still raw, because no `CustomType` is intercepting), call the DigitalOcean API with the raw material, then **before** writing state we explicitly do `plan.PrivateKey = types.StringValue(util.HashString(plan.PrivateKey.ValueString()))` (and the same for leaf/chain).
3. `Read` does NOT touch these three fields — the API never returns them, so the hash that Create wrote stays in state until a forced replace supersedes the resource. This faithfully preserves SDKv2 behaviour where the same field never round-trips.
4. The ex-`DiffSuppressFunc` becomes a small custom plan modifier (`hashSuppressModifier`) on each of the three attributes. It compares the prior state value (the hash) with `util.HashString(req.ConfigValue.ValueString())` and, when they match, sets `resp.PlanValue = req.StateValue` — so terraform sees no change. This is the framework-idiomatic translation of the SDKv2 closure `return new != "" && old == d.Get("private_key")`: it captures the same "the practitioner re-supplied the same raw material; don't replace" semantic.

This option keeps the existing acceptance assertion green (`util.HashString(privateKey)` is still what's in state) and changes nothing the practitioner sees in their config or state file — exactly the "pure refactor from the user's POV" the skill's pitfalls section calls for.

### Option 3 — non-destructive custom type that holds raw + exposes hash via `Equal` (rejected)

The reference warns this path is "genuinely fiddly" and recommends it only when (1) and (2) don't apply. (2) does apply cleanly here, so the extra moving parts (a complete `basetypes.StringTypable` implementation with `Equal`, `String`, `ValueType`, `Type`, `ValueFromTerraform`) would be cargo. Skipped.

## Other translations

- **`StateUpgrader` (V0→V1)** — the SDKv2 single-step `MigrateCertificateStateV0toV1` becomes `(r *certificateResource) UpgradeState(ctx)` returning `map[int64]resource.StateUpgrader{0: { PriorSchema, StateUpgrader }}`. We kept the original `MigrateCertificateStateV0toV1(ctx, rawState, meta)` function as a free function so the existing unit test `TestResourceExampleInstanceStateUpgradeV0` continues to pass; the framework upgrader and the free function share the same simple "rotate id ↔ uuid; promote name to id" transformation. Per the skill's `state-upgrade.md`, the framework call returns the *current* (V1) state directly — there is no chain.
- **`ConflictsWith` on `domains`** — translated to `setvalidator.ConflictsWith(path.MatchRoot(...))` for each of the three peer attributes; lives in the `domains` `Validators` slice.
- **`DiffSuppressFunc` on `domains`** (suppresses diffs when `type == "custom"`) — translated to a custom `setplanmodifier` (`customDomainsModifier`) that reads `path.Root("type")` from `req.Plan` and, when it equals `"custom"`, copies the prior state value forward via `resp.PlanValue = req.StateValue`. The state-stays-current effect is the same as the SDKv2 closure; the new plan modifier is just less magical.
- **`ForceNew: true`** on every user-facing field — translated to `RequiresReplace()` in the per-type planmodifier package (`stringplanmodifier.RequiresReplace`, `setplanmodifier.RequiresReplace`).
- **`validation.NoZeroValues` (string)** — translated to `stringvalidator.LengthAtLeast(1)` per the skill's validators table; we did NOT flip `Optional` to `Required` (that would be a breaking schema change called out in the skill).
- **`validation.StringInSlice`** on `type` — `stringvalidator.OneOf("custom", "lets_encrypt")`.
- **`Default: "custom"`** on `type` — translated to `Default: stringdefault.StaticString("custom")` *with* `Computed: true` (the framework requires `Computed` whenever `Default` is set; this is the schema-validation rule called out in `references/plan-modifiers.md`).
- **`retry.StateChangeConf` "wait for verified"** — replaced with an inline `waitForCertificateState` polling helper (no framework analogue exists; the helper preserves the SDKv2 `Delay: 10s, MinTimeout: 3s` behaviour).
- **Per-CRUD presence checks for `private_key` / `leaf_certificate` / `domains`** — moved out of `Create` into `ValidateConfig`, where the framework prefers config-shape checks. This surfaces the same error messages earlier in the lifecycle. Error strings preserved verbatim so the existing `ExpectError` regexes continue to match.
- **Importer** — `schema.ImportStatePassthroughContext` → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Test file

`resource_certificate_test.go` is updated to use `ProtoV6ProviderFactories` (per `references/testing.md`); `acceptance.TestAccProtoV6ProviderFactories` is assumed to be added as part of the broader provider migration (it sits alongside `TestAccProviderFactories` in the acceptance package). Other than the factory swap and a docstring around the upgrader unit test, the file is unchanged — same configs, same checks, same error regexes — which keeps the migration "pure refactor from the user's POV".
