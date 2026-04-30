# Notes — `resource_openstack_vpnaas_ike_policy_v2` migration

## Scope

Migrated only the IKE policy resource and its test file from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`, per the user's
explicit "don't migrate anything else" directive. No provider-level wiring
changes, no edits to `types.go` (`IKEPolicyCreateOpts`), no edits to shared
helpers (`MapValueSpecs`, `GetRegion`, `CheckDeleted`).

## Defaults — the load-bearing translation

The SDKv2 schema had **five** `Default:` fields. Each translates to the
framework's separate `defaults` package (NOT a plan modifier — that's the
documented pitfall in `SKILL.md` and `references/plan-modifiers.md`):

| Attribute                  | SDKv2 default | Framework                              |
|----------------------------|---------------|----------------------------------------|
| `auth_algorithm`           | `"sha1"`      | `Default: stringdefault.StaticString("sha1")`     |
| `encryption_algorithm`     | `"aes-128"`   | `Default: stringdefault.StaticString("aes-128")`  |
| `pfs`                      | `"group5"`    | `Default: stringdefault.StaticString("group5")`   |
| `phase1_negotiation_mode`  | `"main"`      | `Default: stringdefault.StaticString("main")`     |
| `ike_version`              | `"v1"`        | `Default: stringdefault.StaticString("v1")`       |

Each of these attributes was `Optional: true` only in SDKv2. In the
framework, an attribute carrying a `Default` **must also be `Computed: true`**
or the framework panics during schema validation — so each gained
`Computed: true`. Practitioners can still set the value because the attribute
is also `Optional`. The framework then inserts the default into the plan
when the user omits it.

The default lives on the attribute's **own** `Default:` field, not inside
`PlanModifiers:`. Wiring `Default` into `PlanModifiers` is a compile error
that the older skill snapshot calls out specifically.

## ValidateFunc → `stringvalidator.OneOf`

Each `Default` attribute also had a custom `ValidateFunc` that switch-cased
over a fixed list of `ikepolicies.*` constants. These translate cleanly to
`stringvalidator.OneOf(...)` over the same constant set (cast to `string`),
which is the canonical mapping per `references/validators.md`. The custom
validators (`resourceIKEPolicyV2AuthAlgorithm`, etc.) are removed.

## Other schema decisions

- **`region`, `tenant_id`** — `ForceNew: true` becomes
  `stringplanmodifier.RequiresReplace()`. They are also `Computed:true`, so
  `UseStateForUnknown` is added to keep the plan stable.
- **`value_specs`** — `TypeMap` of strings, `ForceNew: true`. Becomes
  `MapAttribute{ElementType: types.StringType}` with
  `mapplanmodifier.RequiresReplace()`.
- **`lifetime`** — was `TypeSet` of `&schema.Resource` with no `MaxItems`.
  Per `references/blocks.md`, true repeating sets stay as `SetNestedBlock` to
  preserve practitioner HCL syntax (`lifetime { ... }`). Block presence is
  HCL-driven so it can't be `Computed`; the inner `units`/`value` attributes
  are `Optional + Computed` (with `UseStateForUnknown` on `units`) so the API
  values that come back populate the state without showing diffs.
- **`Timeouts`** — the SDKv2 `Timeouts:` field is gone. Migrated to the
  `terraform-plugin-framework-timeouts` package. Used `timeouts.Block(...)`
  (not `Attributes`) to preserve the `timeouts { create = "10m" }` block
  syntax practitioners already write. Each CRUD method reads its specific
  timeout via `plan.Timeouts.Create(ctx, 10*time.Minute)` etc., matching the
  SDKv2 `DefaultTimeout(10 * time.Minute)` defaults.

## CRUD changes (mechanical)

- `*schema.ResourceData` is gone. State/plan are read into a typed model
  struct (`ikePolicyV2ResourceModel`) via `req.Plan.Get(ctx, &m)` etc.
- `d.HasChange("foo")` is replaced by `!plan.Foo.Equal(state.Foo)`, comparing
  typed values from the plan vs. state.
- `diag.FromErr` / `diag.Errorf` → `resp.Diagnostics.AddError(summary, detail)`.
- `d.Set("foo", value)` → `m.Foo = types.StringValue(value)` plus a final
  `resp.State.Set(ctx, &m)`.
- After `Update` succeeds the resource is re-fetched so computed lifetime
  values reflect the API response (matching the SDKv2 path that ended in
  `resourceIKEPolicyV2Read`).
- `Importer.StateContext: ImportStatePassthroughContext` →
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.

## Test-file changes

- Switched `ProviderFactories: testAccProviders` →
  `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` (the framework
  conventional factory name; per the skill's `references/testing.md` step-7
  TDD gate).
- Added `TestCheckResourceAttr` assertions in
  `TestAccIKEPolicyVPNaaSV2_basic` for the five default values
  (`sha1`, `aes-128`, `group5`, `main`, `v1`). The pre-migration test only
  asserted name/description/tenant_id, which would not have caught a missing
  or wrong default. Adding these checks is the TDD evidence that the
  defaults round-trip.
- All other tests are kept as-is structurally; only the factory field name
  changes.

## Out-of-scope follow-ups (not done; user said don't migrate other things)

These would be needed for a full provider migration but are deliberately
left for later:

1. The provider itself still uses SDKv2 — `testAccProtoV6ProviderFactories`
   doesn't yet exist in the openstack provider, so the migrated test file
   will not compile against the current `main`. This is the expected
   "step-7-red" state from the skill workflow: the test file is updated
   *first*, fails red against an unmigrated provider, then the resource
   migration unblocks it.
2. `IKEPolicyCreateOpts` in `types.go` is still SDKv2-flavoured but is type
   compatible (it just embeds `ikepolicies.CreateOpts`); no change needed
   for this resource alone.
3. `getRegionFromPlan` shadows `GetRegion(d, config)`. A future cross-cutting
   refactor should provide a typed-model `GetRegion` helper that all
   migrated resources share.

## Pitfall checklist (from `SKILL.md` "Common pitfalls")

- [x] `Default` placed on the attribute's `Default:` field, not in
      `PlanModifiers`.
- [x] Each `Default` attribute is `Computed: true` (alongside `Optional: true`).
- [x] `ForceNew` translated to `*planmodifier.RequiresReplace()`, not to a
      non-existent `RequiresReplace: true` schema field.
- [x] No user-facing schema names changed; default values are byte-identical
      to the SDKv2 strings.
- [x] `Set: schema.HashString` etc. — no such hashers existed in this
      resource, nothing to drop.
