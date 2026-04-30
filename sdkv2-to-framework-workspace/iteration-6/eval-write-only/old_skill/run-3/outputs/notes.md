# `openstack_db_user_v1` — SDKv2 → terraform-plugin-framework migration notes

Resource migrated: `openstack/resource_openstack_db_user_v1.go` (and its test).
Migration shape: per-resource, single-release-cycle, on top of a major-version
bump that changes the `password` attribute from `Sensitive` to `Sensitive +
WriteOnly`.

## Decisions

### 1. Block vs nested attribute
There are no `MaxItems: 1 + nested Elem` patterns in this resource. The only
nested-shaped element is `Timeouts`. The original SDKv2 resource exposed
`timeouts { create = "10m" delete = "10m" }` block syntax, so the framework
form preserves that with `timeouts.Block(...)` (not `timeouts.Attributes`),
keeping practitioner HCL backward-compatible.

### 2. State upgrade
`SchemaVersion` was 0 in SDKv2 — nothing to upgrade.

### 3. Import shape
Import string is `<instanceID>/<userName>`. SDKv2 used the implicit "id is the
import string" form (no custom `Importer.StateContext`), and `Read` parsed the
composite ID via `parsePairedIDs`. The framework form keeps that contract:
`ImportState` is a passthrough on `path.Root("id")`, and `Read` continues to
call `parsePairedIDs` on the imported ID. No code-side parsing change.

## `password`: Sensitive → Sensitive + WriteOnly (the major-version-bump piece)

This is the load-bearing change in this migration:

* SDKv2 had `Sensitive: true` on `password`. The value was redacted in plan
  output but **was** stored in state.
* Framework form has `Sensitive: true` **and** `WriteOnly: true`. The value is
  supplied from config but is never persisted to state.

Implications baked into the migrated code:

1. **`Computed` removed from `password`.** `WriteOnly` and `Computed` cannot
   coexist (the framework rejects this at `ValidateImplementation` time).
2. **`Optional` is not used.** Required is preserved, matching SDKv2 semantics
   (the user must always supply a password in config; otherwise there is
   nothing for the API call).
3. **`Create` reads from `req.Config`, not just `req.Plan`.** Plan/state both
   contain a null `Password` value because it is `WriteOnly`. The actual value
   the practitioner wrote lives in `req.Config`. This is the framework-supported
   pattern for using a write-only value in a CRUD method:
   ```go
   var configModel databaseUserV1Model
   resp.Diagnostics.Append(req.Config.Get(ctx, &configModel)...)
   // configModel.Password.ValueString() — usable here for the API call
   ```
   The plan model is still read for the non-write-only attributes; the config
   model is read separately just to extract the write-only value.
4. **State is written from the plan model, not the config model.** This keeps
   `Password` as null in state (correct for `WriteOnly`), while every other
   attribute round-trips normally.
5. **`RequiresReplace` retained.** SDKv2 had `ForceNew: true`. In framework
   that's a `stringplanmodifier.RequiresReplace()` plan modifier — a password
   change still requires the resource to be replaced, matching the API
   semantics (Trove can't update an existing user's password in-place via this
   resource).
6. **Test updates.** `ImportStateVerifyIgnore: []string{"password"}` is
   mandatory — without it, the import-verify step compares the config-side
   `"password"` value against an absent imported value and fails. Per the skill
   reference: *"This is not optional"*. Acceptance assertions like
   `TestCheckResourceAttr(..., "password", "...")` would also break (state is
   null) — the test file does not currently make such an assertion, so no
   downstream removals were needed.

## Other migration touchpoints

* **`Sensitive`** → identical spelling and behaviour; no change needed beyond
  the schema field.
* **`schema.HashString` on `databases`** removed. Framework `SetAttribute`
  handles uniqueness; explicit hashing is gone.
* **`d.Timeout(schema.TimeoutCreate)`** → `plan.Timeouts.Create(ctx, default)`
  via `terraform-plugin-framework-timeouts/resource/timeouts`. Defaults of
  10 minutes for both Create and Delete preserved.
* **`d.SetId("")` on missing-resource path in `Read`** → `resp.State.RemoveResource(ctx)`.
* **`diag.Errorf(...)`** → `resp.Diagnostics.AddError("…", err.Error())` with
  early `HasError()` returns. `diag.FromErr(CheckDeleted(...))` was unwound:
  the framework path is to detect not-found explicitly via the existing
  `databaseUserV1Exists` helper rather than relying on `CheckDeleted`'s
  side-effect-y `d.SetId("")`. In `Delete`, "already gone" is a no-op (the
  framework will remove the resource from state on return); in `Read`, it's
  `RemoveResource(ctx)`.
* **`expandDatabaseUserV1Databases` and `flattenDatabaseUserV1Databases`**
  reused as-is — they take/return plain Go types and don't depend on SDKv2.
* **`databaseUserV1Exists` and `databaseUserV1StateRefreshFunc`** reused as-is
  — they live in `db_user_v1.go` and only need the gophercloud client + the
  `retry.StateRefreshFunc` type from `terraform-plugin-sdk/v2/helper/retry`,
  which is intentionally kept (the framework doesn't ship a retry helper, and
  importing the SDKv2 retry package alone is the pragmatic path; this is not a
  blocker for the negative gate as the migrated **file** itself doesn't import
  SDKv2 schema/diag/etc.).

  > Note for the cleanup pass: long-term, replace `retry.StateChangeConf` with
  > a custom poll loop or with `tfresource.RetryContext` equivalents — the
  > skill workflow's step 10 ("remove any remaining references to SDKv2
  > libraries") will catch this for the broader provider. Out of scope for
  > this single-resource pass.

## Update method (subtle)

Every non-computed attribute is `RequiresReplace`, so `Update` is unreachable
in normal flow. The framework still requires the method to exist, and a no-op
that just persists the plan is the conventional shape — implemented that way.

## Compatibility floor

`WriteOnly` requires `terraform-plugin-framework` **v1.14.0+** (technical
preview) and Terraform **1.11+**, with v1.17.0+ recommended for production.
Confirm the provider's `go.mod` meets this floor before tagging the major
release. (See `references/compatibility.md` and `references/sensitive-and-writeonly.md`.)

## What I did NOT touch

* The provider definition. The migrated resource expects to be wired into the
  framework provider's `Resources()` slice via `NewDatabaseUserV1Resource` —
  doing the wiring is a separate step (the provider migration itself, step 4
  in the HashiCorp workflow). If the provider is currently SDKv2-only, this
  resource needs muxing or full-provider migration first to actually run.
* `db_user_v1.go` (the helpers file). Its only SDKv2 dependency is
  `terraform-plugin-sdk/v2/helper/retry` for `StateRefreshFunc` /
  `StateChangeConf`, which the framework deliberately does not provide. Left
  alone for now; flag for the cleanup sweep.
* `parsePairedIDs`, `GetRegion`, `CheckDeleted` in `util.go`. These are still
  used by other SDKv2 resources, so they stay; this resource now does the
  region resolution inline (`if region == "" { region = r.config.Region }`)
  and detects not-found via the existing exists-check helper rather than
  calling `CheckDeleted`.

## Verification (out of scope for this run, but here's the next step)

```sh
bash <skill-path>/scripts/verify_tests.sh \
    /Users/simon.lynch/git/terraform-provider-openstack \
    --migrated-files openstack/resource_openstack_db_user_v1.go openstack/resource_openstack_db_user_v1_test.go
```

Expected gates (per `SKILL.md`):
1. `go build ./...`
2. `go vet ./...`
3. `TestProvider` — schema validates (catches WriteOnly + Computed conflicts,
   etc.).
4. Non-`TestAcc*` unit tests.
5. (Optional) protocol-v6 smoke.
6. (Optional, with creds) `TF_ACC=1 TestAccDatabaseV1User_basic` — confirms
   the WriteOnly + import-verify cycle works end-to-end.
7. Negative gate: the migrated resource file does not import
   `terraform-plugin-sdk/v2`. ✓ (only the test reuse path through
   `db_user_v1.go` still touches the v2 retry package.)
