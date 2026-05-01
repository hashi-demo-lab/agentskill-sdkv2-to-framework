# Migration notes — openstack_vpnaas_ike_policy_v2

Single-resource migration from terraform-plugin-sdk/v2 to terraform-plugin-framework.

## Default fields — the focus of this task

SDKv2 attached `Default:` directly on the schema field. The framework moves this
to a *separate* `Default` field on the typed attribute, populated from a
per-type `defaults` package — **not** a plan modifier. Each defaulted attribute
must also be `Computed: true` so the framework can insert the default into the
plan.

| Attribute | SDKv2 `Default:` | Framework `Default:` | `Computed: true` |
|---|---|---|---|
| `auth_algorithm`           | `"sha1"`     | `stringdefault.StaticString("sha1")`     | yes |
| `encryption_algorithm`     | `"aes-128"`  | `stringdefault.StaticString("aes-128")`  | yes |
| `pfs`                      | `"group5"`   | `stringdefault.StaticString("group5")`   | yes |
| `phase1_negotiation_mode`  | `"main"`     | `stringdefault.StaticString("main")`     | yes |
| `ike_version`              | `"v1"`       | `stringdefault.StaticString("v1")`       | yes |

Import: `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault`.

`Default` is its *own* field on `schema.StringAttribute`, sibling to
`PlanModifiers`, `Validators`, `Optional`, `Computed`. Putting `stringdefault`
inside the `PlanModifiers` slice would be a compile error (different interface).

## Other translations applied

- **`ForceNew: true`** on `region`, `tenant_id`, `value_specs` →
  `stringplanmodifier.RequiresReplace()` for the string attrs and a hand-rolled
  `mapRequiresReplace` for the map attribute (the framework does not ship a
  built-in map RequiresReplace via the public package — defining a small
  in-file plan modifier is the standard pattern).
- **`Computed: true` (existing on region/tenant_id/lifetime)** preserved; for
  `region` and `tenant_id` we add `UseStateForUnknown` so plans don't show
  noisy `(known after apply)` for created-once values.
- **`ValidateFunc`** for the five enum-style attributes (`auth_algorithm`,
  `encryption_algorithm`, `pfs`, `phase1_negotiation_mode`, `ike_version`) →
  `stringvalidator.OneOf(...)` enumerating the same set the SDKv2 switch was
  matching.
- **`Importer`** → `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
- **`Timeouts: schema.ResourceTimeout{...}`** → `timeouts.Block(ctx, ...)` so
  practitioners can keep writing `timeouts { create = "10m" }` block syntax.
  Each CRUD method reads its timeout via `plan.Timeouts.Create(ctx, default)`
  and wraps the context.
- **CRUD signatures**: `CreateContext`/`ReadContext`/`UpdateContext`/
  `DeleteContext` (returning `diag.Diagnostics`) → framework `Create`/`Read`/
  `Update`/`Delete` methods on `*ikePolicyV2Resource` with typed
  `resource.{Create,Read,Update,Delete}{Request,Response}` params and
  `resp.Diagnostics`.
- **`d.HasChange`** → typed `!plan.X.Equal(state.X)` comparisons in `Update`.
- **`d.Set("foo", v)`** → assignment on the model struct + `resp.State.Set(ctx, &model)`.
- **`CheckDeleted` (404 → SetId(""))** → `if gophercloud.ResponseCodeIs(err, 404) { resp.State.RemoveResource(ctx); return }`
  inside `Read`.

## Block vs nested attribute decision — `lifetime`

SDKv2 had `lifetime` as `TypeSet` of nested `*schema.Resource`. Practitioner
configs already write block syntax (`lifetime { units = "..." value = ... }`)
in the existing test file, so per `references/blocks.md` the right choice for
this migration is to keep it as a block (`SetNestedBlock`). Converting to
`SetNestedAttribute` would have changed the HCL syntax users wrote — a
practitioner-visible breaking change.

Note: blocks cannot be `Computed`, so the SDKv2 `Computed: true` on the outer
`lifetime` is dropped — the inner attributes (`units`, `value`) are still
`Computed` so the API-returned values populate state. `units` carries
`UseStateForUnknown` so plans don't show drift between configured-empty and
API-returned values.

## Things deliberately not migrated

- The provider definition / `Provider()` function. Out of scope per the task
  ("Don't migrate anything else"). The migrated resource will only register
  once the provider type itself is migrated to `provider.Provider`; for the
  test file to compile under `ProtoV6ProviderFactories`, the harness must add
  a wrapper that exposes the framework provider — not in scope here.
- `import_openstack_vpnaas_ike_policy_v2_test.go` — separate file, untouched.
- The `IKEPolicyCreateOpts` type and the `ikepolicies.LifetimeCreateOpts`
  builders — reused from existing `types.go` and replicated as helpers that
  read from typed model structs instead of `*schema.Set`.

## Test file changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- Added `TestCheckResourceAttr` assertions for the five default values in
  `_basic` so the test specifically exercises the migrated default-handling
  (red-then-green expectation: this assertion fails on the SDKv2 baseline if
  the schema were broken).
- HCL configs unchanged — `lifetime { ... }` block syntax preserved (see
  block-decision note above).

## Risk notes

- The `mapRequiresReplace` plan modifier is hand-written. Check it against any
  existing in-tree map plan modifier the openstack provider may already define
  during the broader provider-level migration.
- `lifetime` is a `Computed: true` block in SDKv2. Blocks can't be
  `Computed` in the framework, so any usage that relied on the block being
  populated from API state without being declared in HCL still works (the
  inner attributes are computed) but the *outer* set is no longer
  Computed-marked. Acceptance tests should be re-run to confirm no spurious
  diffs.
- The harness var `testAccProtoV6ProviderFactories` is referenced but not
  defined in this file; once the provider type itself is migrated, that var
  needs to exist in the test harness (typically in `provider_test.go`).
