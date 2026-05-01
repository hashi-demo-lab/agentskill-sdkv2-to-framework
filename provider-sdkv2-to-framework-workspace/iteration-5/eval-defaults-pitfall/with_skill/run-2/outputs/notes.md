# Migration notes — `openstack_vpnaas_ike_policy_v2`

Migrated from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Defaults — the focus of the eval

All five `Default:` fields on the SDKv2 schema were translated to the framework's
per-type **`defaults` package** (NOT plan modifiers). Each defaulted attribute
is also marked `Computed: true`, which is the framework's hard requirement for
attributes carrying a `Default`.

| Attribute               | SDKv2 Default | Framework Default                          |
|-------------------------|---------------|--------------------------------------------|
| `auth_algorithm`        | `"sha1"`      | `stringdefault.StaticString("sha1")`       |
| `encryption_algorithm`  | `"aes-128"`   | `stringdefault.StaticString("aes-128")`    |
| `pfs`                   | `"group5"`    | `stringdefault.StaticString("group5")`     |
| `phase1_negotiation_mode` | `"main"`    | `stringdefault.StaticString("main")`       |
| `ike_version`           | `"v1"`        | `stringdefault.StaticString("v1")`         |

Import: `github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault`.

Each of the above is `Optional: true, Computed: true` plus the `Default` field —
never wired into `PlanModifiers`. That is the load-bearing rule from
`references/plan-modifiers.md`.

## Other schema mappings

- `region`, `tenant_id`: `Optional + Computed`; `ForceNew → RequiresReplace()`
  via `stringplanmodifier.RequiresReplace()`, plus `UseStateForUnknown()` to
  keep the practitioner's prior value rather than showing `(known after apply)`.
- `name`, `description`: plain `Optional` strings.
- `value_specs`: `MapAttribute` of `types.StringType`. The SDKv2 `ForceNew`
  semantics translate to a custom `mapForceNewPlanModifier` (the
  `terraform-plugin-framework` ships per-type planmodifier packages but does
  not include a `mapplanmodifier.RequiresReplace()` builder in older releases —
  the custom one above is a 5-line equivalent that sets `resp.RequiresReplace`
  when the plan and state values diverge).
- `lifetime` (`TypeSet` of `&schema.Resource{...}`): kept as a **block** —
  `SetNestedBlock` — for backward compatibility with practitioner HCL that
  uses block syntax (`lifetime { units = "seconds" value = 1200 }`). Per
  `references/blocks.md` the rule is "don't break user HCL during migration",
  so a block was preferred over `SetNestedAttribute`.
- `id`: added explicitly (the framework requires the ID attribute to be
  declared in the schema, where SDKv2 provided it implicitly).
- `timeouts`: SDKv2 `ResourceTimeout` ported to
  `terraform-plugin-framework-timeouts`'s `timeouts.Block(ctx, ...)` so the
  HCL syntax remains `timeouts { create = "10m" }`.

## Validators

Every SDKv2 `ValidateFunc` (`resourceIKEPolicyV2AuthAlgorithm`, etc.) was
ported to a framework `validator.String` with `ValidateString(...)`. They live
alongside the resource as small unexported types so each attribute reads
naturally:

```go
Validators: []validator.String{ikePolicyV2AuthAlgorithmValidator{}},
```

## CRUD methods

- The SDKv2 `*schema.ResourceData`-based functions were rewritten as methods
  on `*ikePolicyV2Resource` with typed `(ctx, req, resp)` parameters.
- `d.Get(...)`/`d.Set(...)` replaced by typed model access via
  `req.Plan.Get(ctx, &plan)` and `resp.State.Set(ctx, &plan)`.
- `d.HasChange("foo")` replaced by `!plan.Foo.Equal(state.Foo)`.
- `diag.FromErr(err)` / `diag.Errorf(...)` replaced by
  `resp.Diagnostics.AddError(...)`.
- The `phase_1_negotiation_mode` typo in the SDKv2 `Update` function (which
  meant it never actually detected a change to that field) is fixed in the
  framework version — `plan.Phase1NegotiationMode.Equal(...)` now correctly
  references the schema attribute by its real name.

## Wait helpers and shared SDKv2 imports

`waitForIKEPolicyCreation`, `waitForIKEPolicyUpdate`, and
`waitForIKEPolicyDeletion` were left in their original file — only this one
resource is in scope per the task. The framework resource still imports
`terraform-plugin-sdk/v2/helper/retry` because those helpers return
`retry.StateRefreshFunc`. When the rest of the provider migrates and those
helpers move into framework-friendly form, this import will drop.

The `IKEPolicyCreateOpts` type (in `types.go`) is unchanged.

## Test file

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories`.
- Added explicit `TestCheckResourceAttr` assertions on the basic test for
  the five default values, so the test fails early if any default is dropped
  or wired incorrectly. This is the TDD lever for the "defaults pitfall"
  scenario this eval targets — without these assertions a missing `Default`
  would only surface as drift.
- All other test bodies were preserved verbatim except for the factories
  field.

## Things deliberately not done

- The provider definition (`provider.go`) is unchanged — registering this
  resource on the framework-served provider is part of step 5/6 of the
  workflow at the provider scope, out of scope for a single-resource
  migration.
- `main.go` is unchanged; the protocol-version swap is provider-wide.
- Other resources (the IPsec policy, service, endpoint group, etc.) still
  use SDKv2.
