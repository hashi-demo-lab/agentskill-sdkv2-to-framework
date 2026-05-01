# Migration notes — `digitalocean_database_logsink_rsyslog`

Source: `<digitalocean-clone>/digitalocean/database/resource_database_logsink_rsyslog.go`

This resource exhibits two SDKv2 patterns that the skill calls out as needing
careful, non-mechanical translation:

1. A **composite-ID importer** (`cluster_id,logsink_id`).
2. A chained `customdiff.All(...)` **CustomizeDiff**.

Both are covered below alongside the supporting refactors.

---

## 1. CustomizeDiff → `ResourceWithModifyPlan.ModifyPlan`

**Original (SDKv2):**

```go
CustomizeDiff: customdiff.All(
    customdiff.ForceNewIfChange("name", func(_ context.Context, old, new, meta interface{}) bool {
        return old.(string) != new.(string)
    }),
    func(ctx context.Context, diff *schema.ResourceDiff, v interface{}) error {
        return validateLogsinkCustomDiff(diff, "rsyslog")
    },
),
```

**Translation strategy** (per `references/plan-modifiers.md`):

`customdiff.All(legA, legB)` has no framework equivalent helper — both legs
fold into a single `ModifyPlan` method. Each leg keeps the same order so
short-circuit semantics are preserved.

| Leg | Original purpose | Framework home |
|---|---|---|
| `customdiff.ForceNewIfChange("name", ...)` | Force replacement when `name` changes | Already `ForceNew: true` on the `name` attribute, so this is **redundant** in the framework — translates to `stringplanmodifier.RequiresReplace()` on the `name` attribute. No `ModifyPlan` body needed for this leg. |
| `validateLogsinkCustomDiff(diff, "rsyslog")` | Cross-attribute validation (format/logline pairing, TLS gating, mTLS pairing) | Lives in `ModifyPlan`. The body reads the typed plan into a `databaseLogsinkRsyslogModel` and calls a renamed helper `validateLogsinkPlanRsyslog(*model)`. |

**Implementation details:**

- The destroy case is short-circuited at the top via `req.Plan.Raw.IsNull()`
  — no plan to validate when the resource is being removed.
- Each cross-field check now guards against `IsUnknown()` / `IsNull()` on the
  typed values. SDKv2's `diff.Get("...")` returned the zero-typed value for
  unknowns, but the framework distinguishes null/unknown/known explicitly and
  the validator must not panic on unknowns from interpolated configs.
- The interface assertion `var _ resource.ResourceWithModifyPlan = ...` makes
  a missing `ModifyPlan` a compile error.

---

## 2. Composite-ID importer → `ResourceWithImportState` + `ResourceWithIdentity`

**Original (SDKv2):**

```go
Importer: &schema.ResourceImporter{
    StateContext: resourceDigitalOceanDatabaseLogsinkRsyslogImport,
},
// where the body parses the "cluster_id,logsink_id" composite string.
```

**Translation strategy** (per `references/identity.md` and `references/import.md`):

This is the canonical case the skill flags for `ResourceWithIdentity`: a
composite ID with two clearly-typed components. Both import paths are
implemented:

### Modern path — `ResourceWithIdentity`

`IdentitySchema` exposes `cluster_id` and `logsink_id` as identity attributes
(`RequiredForImport: true`). Practitioners on Terraform 1.12+ get:

```hcl
import {
  to = digitalocean_database_logsink_rsyslog.foo
  identity = {
    cluster_id = "deadbeef-..."
    logsink_id = "01234567-..."
  }
}
```

`Identity` is set in `Create`, `Update`, and `Read`, mirroring the cluster_id
and logsink_id from state.

### Legacy CLI path — kept

The `cluster_id,logsink_id` form remains supported for `terraform import` on
older CLIs:

```sh
terraform import digitalocean_database_logsink_rsyslog.foo deadbeef-...,01234567-...
```

`ImportState` branches on `req.ID == ""`:

- empty → modern (read identity, write state attributes + `id`)
- non-empty → legacy (split on `,`, write state attributes + `id`, mirror onto identity)

The error message for malformed legacy input is preserved verbatim from the
SDKv2 implementation so existing tooling that scrapes import errors keeps
working.

### Why `ImportStatePassthroughWithIdentity` was *not* used

The skill notes that helper is a thin wrapper for "one identity attribute
mirrored to one state attribute". This resource has two identity attributes
plus a derived composite `id` in state, so manual `SetAttribute` calls are
clearer.

---

## Other SDKv2 → framework changes worth flagging

| SDKv2 idiom | Framework |
|---|---|
| `Default: false` (bool) | `Default: booldefault.StaticBool(false)` + `Computed: true` |
| `Default: "rfc5424"` (string) | `Default: stringdefault.StaticString("rfc5424")` + `Computed: true` |
| `validation.NoZeroValues` | `stringvalidator.LengthAtLeast(1)` (per the skill cheatsheet — "no zero values" on a string == non-empty) |
| `validateLogsinkPort` (custom 1..65535) | `int64validator.Between(1, 65535)` — the framework validator has the same wording so the test regex still matches |
| `validateRsyslogFormat` (custom one-of) | `stringvalidator.OneOf("rfc5424", "rfc3164", "custom")` |
| `ForceNew: true` | `stringplanmodifier.RequiresReplace()` |
| `d.SetId("")` on 404 | `resp.State.RemoveResource(ctx)` |
| `d.Get("x").(int)` | typed `m.Port.ValueInt64()` (note: `port` upgraded from `TypeInt` → `Int64Attribute`) |
| `d.Set("foo", v)` | typed assignment to the model + `resp.State.Set(ctx, &model)` |
| `Sensitive: true` | unchanged — same field on `schema.StringAttribute` |
| `Computed: true` for `id` | `Computed: true` + `stringplanmodifier.UseStateForUnknown()` to avoid noisy plans |

The `expand`/`flatten` helpers were renamed to `*FromModel` / `*IntoModel`
to take the typed model directly — this drops the dependency on
`*schema.ResourceData` from helper signatures.

---

## Test file changes

Per `references/testing.md`:

- `ProviderFactories: acceptance.TestAccProviderFactories` →
  `ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories`.
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` →
  `github.com/hashicorp/terraform-plugin-testing/helper/resource`.
- `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` →
  `github.com/hashicorp/terraform-plugin-testing/terraform`.
- Added an import-step round-trip that exercises the legacy composite-ID CLI
  path via `ImportStateIdFunc`. (A modern identity-based import test would
  use `ConfigStateChecks` with `statecheck.ExpectIdentityValue`; left for a
  follow-up because it requires `terraform-plugin-testing v1.13+`.)
- Two `ExpectError` regexes were loosened where framework wording differs
  from SDKv2 — flagged inline in the test file:
  - `port` validator: original `must be between 1 and 65535` → loosened to
    `(?i)between 1 and 65535` to tolerate framework's "Attribute port value"
    prefix.
  - `format` validator: original literal "must be one of: rfc5424, rfc3164,
    custom" → loosened to `(?s)rfc5424.*rfc3164.*custom`.

---

## Caveats / known limitations of this single-file migration

- **Provider isn't muxed.** The DigitalOcean provider is currently 100%
  SDKv2; a real merge would either land a `terraform-plugin-mux` setup
  alongside this file or migrate the whole provider in lockstep. The skill's
  scope explicitly excludes mux setup, so the migrated file is correct in
  isolation but cannot be wired up without provider-level work that is
  out of scope here.
- **`acceptance.TestAccProtoV6ProviderFactories`** does not exist in the
  current acceptance package — the test file references it as the symbol
  the provider-level migration will need to introduce. Until that helper
  exists, the test will not compile (this is the same shape recommended
  by `references/testing.md` for the post-migration end state).
- **Identity-aware test assertions** (`statecheck.ExpectIdentityValue`)
  were intentionally not added because they require `terraform-plugin-testing`
  ≥ 1.13. Confirm the version floor before adding them.
- **`MaxItems: 1`** decision — N/A. This resource has no nested blocks.
- **State upgrades** — N/A. No `SchemaVersion > 0` in the original.
