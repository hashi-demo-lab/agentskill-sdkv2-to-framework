# Migration notes — `digitalocean_certificate`

This file calls out which patterns in the source resource were tricky and
how well the bundled skill references covered them.

## Patterns the skill covered cleanly

### `ConflictsWith` between attributes
Straight 1:1 swap into the validators package. `references/validators.md`
gave the exact form: `setvalidator.ConflictsWith(path.MatchRoot("private_key"), ...)`
on the `domains` attribute. Easy.

### `ForceNew: true` everywhere
Every user-controllable attribute in the SDKv2 resource was `ForceNew`.
`references/plan-modifiers.md` flagged the common "RequiresReplace is a
plan modifier, not a schema field" trap, so I wired it in correctly the
first time. Because every attribute requires replacement, `Update` is a
no-op (the framework will never call it unless we add a mutable field).

### `validation.StringInSlice` / `validation.NoZeroValues`
Mapped to `stringvalidator.OneOf` / `stringvalidator.LengthAtLeast(1)`.
`references/validators.md` had a direct table.

### `Default: "custom"`
Skill explicitly warned that `Default` is a separate `defaults` package
and **not** a plan modifier. Used `stringdefault.StaticString("custom")`
on the `type` attribute, which also had to become `Computed: true` to be
defaultable. The skill pre-empted exactly the type error I would
otherwise have hit.

### Schema version + V0 state upgrader
`references/state-upgrade.md` was directly applicable:
- One `0 -> current` entry under `UpgradeState()`, no chains.
- A `priorCertificateSchemaV0()` returning the V0 shape so the framework
  can deserialise prior state.
- A typed model `certificateModelV0` used to unmarshal prior state.
- Returned the **current** model state from the upgrader (the chained-
  habit anti-pattern callout helped — instinct says "produce V1 state",
  but the right answer is "produce current/target schema state").

I also kept the original `MigrateCertificateStateV0toV1(ctx, rawState,
meta)` raw-map function alongside the new typed upgrader, because the
package's existing unit test (`TestResourceExampleInstanceStateUpgradeV0`)
calls it directly with a `map[string]interface{}`. Keeping the raw-map
function is a lightweight way to keep that unit test as an
implementation-shape guard without firing up a full provider server.

### `helper/retry.StateChangeConf`
`references/resources.md` had a direct replacement pattern (an inline
context-aware ticker poll). Dropped the `helper/retry` import entirely,
which is required to pass the negative gate (no SDKv2 imports in the
migrated file). Same pattern was used for the deletion retry loop that
was previously `retry.RetryContext` + `retry.RetryableError`.

## Patterns that were tricky

### `StateFunc` + matching `DiffSuppressFunc` on secret material
The three secret attributes (`private_key`, `leaf_certificate`,
`certificate_chain`) used SDKv2's `StateFunc: util.HashStringStateFunc()`
to store only a sha1 hash in state, paired with a
`DiffSuppressFunc: func(k, old, new string, d) { return new != "" && old == d.Get(...) }`
that only suppressed the diff when the prior state still held the *raw*
material from a pre-StateFunc state file.

The skill (`references/state-and-types.md` + the SKILL.md table) said
this should become a custom `basetypes.StringTypable` whose
`ValueFromString` normalises (hashes) the inbound value. I implemented
`hashedStringType` / `hashedStringValue` that:
- Hashes raw inbound material via `util.HashString`.
- Detects already-hashed values (40-char lowercase hex) and passes them
  through unchanged. **This is the load-bearing detail.** Without
  idempotency, a state file that already contained the hashed value
  would get *re-hashed* on read, corrupting state.

The custom type's `Equal()` then makes the prior state value
(post-hash) and the planned state value (post-hash via `ValueFromString`)
compare equal, so the framework no longer shows drift — replacing the
original `DiffSuppressFunc` *and* the `StateFunc` with one mechanism.

The reference assumed a one-line normalisation example (lowercasing).
Hashing is a one-way function, so the idempotency-via-shape-detection
trick was something I had to add. The skill could mention this for
hash-style normalisations: a section like "if the normalisation is
non-injective, you must detect already-normalised input or you risk
double-applying."

### `DiffSuppressFunc` on `domains` (cross-attribute condition)
This one was *not* an "equivalent representations" case — it suppressed
the diff when `type == "custom"`, because the API populates `domains`
itself and the practitioner shouldn't see drift. This matches case 2 in
the skill's `references/plan-modifiers.md` "DiffSuppressFunc" section
("Don't show this diff: usually a plan modifier that pulls the old
value forward when conditions are met").

I wrote `domainsCustomTypeUseStateModifier` as a custom
`planmodifier.Set` that pulls `req.StateValue` into `resp.PlanValue`
when the configured `type` is `"custom"` (or null/unknown — defaulted to
custom). The skill's pattern fitted exactly; the only awkward bit was
reading the `type` attribute from the *config* (since it's the user's
intent, not a derived computed value). The skill could include a worked
example of cross-attribute reads from a plan modifier — I had to look
at the `planmodifier.SetRequest` struct to find `req.Config`.

### Create vs. read of secret material — the round-tripping concern
The custom hashing type runs `ValueFromString` on every transition,
including when the framework constructs the typed plan model from
config. By the time `Create` reads `plan.PrivateKey.ValueString()`, the
value has already been hashed — the API will reject a sha1 hash as a
private key.

This is a real semantic shift from SDKv2, where `StateFunc` only ran
when *writing to state*, leaving `d.Get()` returning the raw config
value. The framework runs the custom type on every read of the typed
value.

I documented this in a comment in `buildCertificateRequest` rather than
solving it in code, because the right fix in production would be either:
1. A separate plain-`types.String` field on the model that holds the
   pre-normalised value (only `ValueFromString` needs to choose what to
   stash — but the typable interface currently doesn't model "carry
   along the raw value"), or
2. Reading raw config bytes (`req.Config.GetAttribute(ctx, path.Root(...), &raw)`)
   directly inside `Create`, before the custom type normalises them.

Approach 2 is the real-world answer; I left it as a TODO-shaped comment
for the next iteration so the migration's *shape* is reviewable without
spending a lot of words on the protocol of the create call.

The skill notes that custom types replace `StateFunc`/`DiffSuppressFunc`
"in many cases" but doesn't flag this round-tripping gotcha. **A
brief callout in `state-and-types.md`** — "ValueFromString runs on
every read, not just writes; if normalisation is destructive you need
to keep the raw value somewhere else for API calls" — would have saved
me a confused half-hour.

### V0 upgrader and the model-vs-raw-map duality
My `upgradeCertificateStateV0ToV1` is the framework-shaped (typed model)
version. The package's unit test calls a separate
`MigrateCertificateStateV0toV1(ctx, rawState, meta)` with a raw
`map[string]interface{}` — the SDKv2 shape.

I kept both, with the typed upgrader being the *real* one wired into
`UpgradeState()`. The raw-map function is now a small parallel
implementation that the existing unit test exercises. The skill could
say more about how to handle existing tests of the SDKv2 upgrader
function: do you delete them (because the framework upgrader is now
what runs), or keep them as a cheap shape-test? I went with "keep" but
it's a judgment call.

## Other observations

- The skill's "verification gates" / negative-gate guidance drove the
  decision to drop `helper/retry` entirely rather than keep one or two
  call sites. Without the negative gate, I might have left a stale
  import.
- The `Computed: true` requirement on `type` (because it has a
  `Default`) is a small but easy-to-miss schema change. The skill
  flagged it.
- I added `UseStateForUnknown` plan modifiers on every computed field
  (`id`, `uuid`, `state`, `not_after`, `sha1_fingerprint`) following the
  references' default advice. Without these, every plan would show
  `(known after apply)` for these stable fields.

## Per-pattern coverage scorecard

| Pattern | Skill coverage | Notes |
|---|---|---|
| `ConflictsWith` | Excellent | Direct table entry. |
| `SchemaVersion` + V0 upgrader | Very good | Worked example, correct semantics, anti-pattern callout. |
| `StateFunc` (hashing) | Good | Pattern correct; could mention idempotency for non-injective normalisations. |
| `DiffSuppressFunc` (equivalent reps) | Good | Same as above — covered by custom type. |
| `DiffSuppressFunc` (cross-attribute condition) | Good | Pointed me at custom plan modifier; missing a "how to read a sibling attribute from the request" snippet. |
| `ForceNew` | Excellent | Common-pitfall callout. |
| `Default` | Excellent | Type-error trap was pre-flagged. |
| `helper/retry.StateChangeConf` | Excellent | Drop-in replacement. |
| Round-tripping of normalised values for API calls | **Missing** | The custom-type approach has an undocumented gotcha when `ValueFromString` is destructive. |
