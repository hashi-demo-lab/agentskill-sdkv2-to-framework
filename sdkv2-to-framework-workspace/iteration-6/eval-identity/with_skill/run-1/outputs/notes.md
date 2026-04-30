# `openstack_lb_member_v2` migration — notes

## Identity schema

The resource is naturally addressed by the `(pool_id, member_id)` tuple — that
is exactly the data the legacy SDKv2 importer encoded as the slash-delimited
import string `"<pool>/<member>"`. So the framework `IdentitySchema` declares
two attributes, both `RequiredForImport: true`:

```go
identityschema.Schema{
    Attributes: map[string]identityschema.Attribute{
        "pool_id": identityschema.StringAttribute{RequiredForImport: true},
        "id":      identityschema.StringAttribute{RequiredForImport: true},
    },
}
```

`Identity` is set in `Create`, `Update`, `Read`, and the legacy branch of
`ImportState` so it is always populated alongside state.

Identity attributes are NOT marked `Sensitive` — neither `pool_id` nor
the member's UUID is a secret; both are how a practitioner *addresses* the
resource (per the skill's identity reference, "the value is part of how the
resource is addressed, not a secret").

## Import — the two paths

`ImportState` MUST handle both:

| Path | Trigger | What's populated | Approach used |
|---|---|---|---|
| Modern (Terraform 1.12+) | `import { identity = { pool_id = "...", id = "..." } }` block | `req.Identity` populated, `req.ID == ""` | **`ImportStatePassthroughWithIdentity`**, called once per attribute |
| Legacy CLI / Terraform <1.12 | `terraform import openstack_lb_member_v2.foo <pool>/<member>` | `req.ID == "<pool>/<member>"`, `req.Identity` empty | **Manual `strings.SplitN` + `resp.State.SetAttribute`**, identical to the SDKv2 importer's behaviour, then `resp.Identity.Set` to mirror the parsed values into identity |

Dispatch is the canonical `req.ID == ""` branch documented in
`references/identity.md`. The two paths are mutually exclusive — you can
think of `req.ID == ""` as "the modern, identity-supplied call".

### Per-path approach justification

- **Modern path → `ImportStatePassthroughWithIdentity`.** The identity has
  exactly two attributes and both map 1:1 to state attributes of the same
  name (`pool_id` → `pool_id`, `id` → `id`). The helper handles a single
  attribute pair per call, so we call it twice. Using the helper rather than
  reading `req.Identity` manually keeps the code obviously correct (no
  `GetAttribute`/`SetAttribute` plumbing to get wrong) and signals intent —
  "these identity attributes pass straight through to state untouched".

- **Legacy path → manual `req.Identity` writes (after parsing `req.ID`).**
  The helper isn't applicable here because `req.Identity` is empty on the
  legacy CLI path; we have to *construct* the identity from the parsed
  string. We use `resp.State.SetAttribute` for the two state attributes
  (matching what `ImportStatePassthroughWithIdentity` would have done), and
  `resp.Identity.Set(...)` once for the whole identity model so a subsequent
  refresh sees a populated identity even though the practitioner came in via
  the CLI. This is the "manual req.Identity reads" shape — except inverted:
  manual *writes* into `resp.Identity` rather than reads from `req.Identity`,
  because on this path we are the source of identity.

## Other migration decisions

- **Timeouts**: kept as a block (`timeouts { create = ... }`) using
  `timeouts.Block(...)` because pre-existing practitioner configs in this
  repo's test fixtures already use block syntax. Switching to nested-attribute
  syntax would be a breaking HCL change.
- **`getOkExists("weight")`** preserved by branching on
  `plan.Weight.IsNull() && !plan.Weight.IsUnknown()` — a config-declared
  zero is still meaningful, but an absent attribute leaves the API default
  in place.
- **`Set: schema.HashString`** dropped — framework `SetAttribute` handles
  uniqueness internally.
- **`ForceNew: true`** translated to `RequiresReplace()` plan modifiers on
  `region`, `tenant_id`, `address`, `protocol_port`, `subnet_id`, `pool_id`.
- **`validation.IntBetween(...)`** translated to `int64validator.Between(...)`.
- **`retry.RetryContext(...)`** loops replaced with inline ticker-poll
  helpers (`createMemberWithRetry`, `updateMemberWithRetry`,
  `deleteMemberWithRetry`) that classify retryable errors via a local
  `isRetryableLBError` (mirroring the SDKv2 `checkForRetryableError` 409 /
  500 / 502 / 503 / 504 set). This avoids importing
  `helper/retry` from the migrated file (negative gate).
- **`GetRegion(d, config)`** replaced with the in-file `regionForPlan(model)`
  helper, since the SDKv2 helper signature takes `*schema.ResourceData`.
- **`CheckDeleted(d, err, ...)`** replaced with explicit
  `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` checks plus
  `resp.State.RemoveResource(ctx)` in `Read` — same effect, no SDKv2 type.

## Tests

`ProviderFactories` → `ProtoV6ProviderFactories` (the conventional framework
name; the bootstrap definition lives in `provider_test.go`, untouched per
"don't migrate anything else"). Three test functions:

1. `TestAccLBV2Member_basic` — existing assertions plus new
   `ConfigStateChecks` using `statecheck.ExpectIdentityValueMatchesState`
   to assert identity is populated for both `pool_id` and `id`.
2. `TestAccLBV2Member_monitor` — unchanged behavior, factory swap only.
3. `TestAccLBV2Member_importLegacyCompositeID` — the existing test from
   `import_openstack_lb_member_v2_test.go`, copied here so the migrated file
   carries its own legacy-path import test.
4. `TestAccLBV2Member_importByIdentity` — new test, exercises the modern
   identity-driven import using `ImportStateKind: ImportBlockWithResourceIdentity`.

## Out of scope (callouts for the surrounding migration)

- `provider.go` is still SDKv2; the framework resource expects
  `req.ProviderData` to be `*Config`. In a real single-release migration the
  provider would be served via `providerserver.NewProtocol6WithError` and
  `Configure` would surface `*Config` as `resp.ResourceData`. Until the
  provider is migrated, this file alone does not link cleanly; that's
  expected per the task scope.
- `import_openstack_lb_member_v2_test.go` and
  `data_source_openstack_lb_member_v2.go` are untouched per scope. The
  legacy import test there will still pass once the surrounding provider is
  framework-served, since the dispatch on `req.ID != ""` keeps that path
  alive.
- `testAccProtoV6ProviderFactories` is referenced but not defined here —
  defining it is part of the provider-level migration (HashiCorp step 3,
  serve via the framework).
