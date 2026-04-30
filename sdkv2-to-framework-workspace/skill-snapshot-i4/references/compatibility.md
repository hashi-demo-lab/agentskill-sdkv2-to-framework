# Framework version-floor compatibility

## Quick summary
- Framework features arrived in distinct versions; if the provider must support a frozen Terraform CLI or a pinned framework dependency, you may have to skip newer features.
- The newest features (resource identity, write-only attributes, `UseNonNullStateForUnknown`) all require **framework v1.14+** (some v1.15+, some v1.17+) AND **Terraform 1.11+ or 1.12+**.
- Most migrations from SDKv2 land on framework v1.13+ comfortably; only "what's the minimum framework version we can pin" decisions need this table.
- Rule of thumb for new migrations in 2026: pin `terraform-plugin-framework v1.17+` to get the GA features, with Terraform 1.12+ as the practitioner-side floor. Note that **`WriteOnly` and `action.Action` are still technical preview as of v1.19** — pinning v1.17+ does not buy GA status for those two; check the framework's CHANGELOG before relying on them.
- If you must support older Terraform (< 1.5), skip identity, skip `import {}` blocks, skip write-only — fall back to manual `ImportState` parsing of `req.ID` (see `import.md`).

## Feature → minimum versions

| Feature | Framework version | Terraform CLI | Notes |
|---|---|---|---|
| `Int32Attribute` / `Float32Attribute` | v1.10.0 (July 2024) | any | use when the underlying API is genuinely 32-bit; otherwise default to `Int64Attribute` |
| `DynamicAttribute` / `types.Dynamic` | v1.7.0 (March 2024) | any | rare; only for deliberate untyped passthrough |
| `ResourceWithMoveState` (cross-type / cross-provider state moves) | v1.6.0 (Feb 2024) | 1.8+ (for `moved {}` blocks across types) | see `move-state.md` |
| `ResourceWithIdentity` + `identityschema` package | v1.15.0 (May 2025) | 1.12+ (for `identity = {...}` inside `import {}`) | the `import {}` block itself shipped in 1.5 |
| `ResourceWithUpgradeIdentity` | v1.15.0+ (interface present in source; specific introduction not changelogged — verify before pinning) | 1.12+ | rare; only when changing identity schema |
| `ImportStatePassthroughWithIdentity` | v1.15.0 | 1.12+ | one (state, identity) pair per call |
| `WriteOnly` attributes | v1.14.0+ (still **technical preview** through at least v1.19) | 1.11+ | feature is preview-only — no GA flip in the framework's CHANGELOG yet. Adopt only if you accept preview status. |
| `UseNonNullStateForUnknown` (nested-attribute null fix) | v1.17.0 | any | use on Computed children inside nested attributes; see `plan-modifiers.md` |
| `resp.Deferred` / `resource.Deferred` (cross-resource ordering) | v1.9.0 | 1.9+ | still experimental |
| `function.Function` (provider-defined functions, GA) | v1.8.0 | 1.8+ | framework-only; out of scope for this skill |
| `ephemeral.EphemeralResource` | v1.13.0 | 1.10+ | framework-only |
| `action.Action` (technical preview) | v1.16.0 | 1.14+ | framework-only |
| Protocol v6 (default for new framework providers) | any framework version | 0.15.4+ | choose at workflow step 3; see `protocol-versions.md` |
| Protocol v5 (only if backward-compat needed) | any framework version | 0.12+ | rare in 2026; declines features |

## terraform-plugin-testing version floor

The `terraform-plugin-testing` repo (separate from the framework) gates the test helpers you'll lean on:

| Test helper | terraform-plugin-testing version |
|---|---|
| `ProtoV6ProviderFactories` | v1.0.0+ |
| `plancheck` package (`PlanCheck`, `ExpectEmptyPlan`) | v1.2.0+ (March 2023) |
| `statecheck.ExpectKnownValue` and family | v1.7.0+ |
| `statecheck.ExpectIdentity` / `ExpectIdentityValue` | v1.13.0+ |

Pin `terraform-plugin-testing v1.14.0+` for new migrations in 2026 to get every helper.

## Companion modules

Each is independently versioned; check their own go.mod entries:

- `terraform-plugin-framework-validators` — validator ports (StringLenBetween → LengthBetween, etc.)
- `terraform-plugin-framework-timeouts` — replacement for SDKv2 `Timeouts: &schema.ResourceTimeout`
- `terraform-plugin-framework-jsontypes` — `jsontypes.Normalized`/`Exact` (one of the two most-common DiffSuppressFunc replacements)
- `terraform-plugin-framework-nettypes` — `cidrtypes`, `iptypes`, `hwtypes` (the other most-common DiffSuppressFunc replacement)

## Practitioner-side floors (what you tell users in the changelog)

When you cut a major version of your provider after migration, document:

- **Minimum Terraform CLI version**: at least 1.5 (for `import {}` blocks if you adopted identity), at least 1.12 (for `identity = {...}` payload), at least 1.11 (for write-only). Pick the highest your features require.
- **Minimum Go version for building the provider**: framework v1.10+ requires Go 1.21; v1.15+ requires Go 1.22; v1.17+ requires Go 1.23. Check the framework's `go.mod`.

## Where this matters in the workflow

- **Step 3 (serve via framework)**: protocol version + framework version pin. Check what your customers' Terraform CLI versions look like and pick the floor.
- **Step 6 (per-element conversion)**: when you reach for a feature flagged "v1.10+", confirm your `go.mod` requires that or higher. Don't silently ratchet the floor without changelogging.
- **Step 12 (release)**: write the new floors into the release notes. Practitioners on older Terraform need the heads-up.
