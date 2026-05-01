# Migration notes: openstack_vpnaas_ike_policy_v2

## Scope

Partial migration of one resource — `openstack_vpnaas_ike_policy_v2` — from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`. This eval is
specifically scoped to the **`Default:` pitfall**: the SDKv2 schema has five
attributes with `Default: "..."` values that need careful framework-side
handling.

## Defaults — the central pitfall

Five fields had SDKv2 defaults:

| Attribute | SDKv2 Default | Framework |
|---|---|---|
| `auth_algorithm` | `"sha1"` | `Default: stringdefault.StaticString("sha1")` |
| `encryption_algorithm` | `"aes-128"` | `Default: stringdefault.StaticString("aes-128")` |
| `pfs` | `"group5"` | `Default: stringdefault.StaticString("group5")` |
| `phase1_negotiation_mode` | `"main"` | `Default: stringdefault.StaticString("main")` |
| `ike_version` | `"v1"` | `Default: stringdefault.StaticString("v1")` |

Two trapdoors that the SKILL.md "common pitfalls" section flags and that we
explicitly avoided:

1. **`Default` is NOT a plan modifier.** Wiring it into `PlanModifiers: []`
   would be a compile error. In the framework, `Default` is its own field on
   the attribute struct; it is populated via the dedicated `defaults` package
   (here, `terraform-plugin-framework/resource/schema/stringdefault`).
2. **An attribute with a `Default` must be `Computed: true`.** Otherwise the
   framework cannot inject the default value into the plan when the user omits
   the attribute. We therefore added `Computed: true` to every attribute that
   gained a `Default`. (The user-facing semantics — Optional, with the value
   defaulting in absentia — are preserved.)

## Other migration choices

### Validators
The five `ValidateFunc` closures (auth algorithm, encryption algorithm, pfs,
ike version, phase1 negotiation mode) became typed
`validator.String` implementations on per-attribute `Validators: []validator.String{...}`
slices. Behaviour is unchanged — same allow-lists, same error messages.

### `lifetime` block
SDKv2 shape: `TypeSet` + `Elem: &schema.Resource{}` + `Computed + Optional`,
no `MaxItems`. Per the skill's `references/blocks.md` "true repeating blocks
→ keep as block" rule and to preserve practitioner HCL (`lifetime { ... }`),
we kept it as a `schema.SetNestedBlock`. Inner `units` / `value` attributes
keep the `Optional + Computed` semantics so the API-server-generated default
flows back through `Read` cleanly.

Note: blocks themselves cannot be `Computed`; the original SDKv2 had
`Computed: true` on the outer set, but in practice the only Computed semantics
needed are on the inner attributes (server-populated unit/value). The block
syntax preservation outweighs the lost outer `Computed` annotation.

### `ForceNew` → `RequiresReplace`
- `region` → `stringplanmodifier.RequiresReplace()`
- `tenant_id` → `stringplanmodifier.RequiresReplace()`
- `value_specs` → `mapplanmodifier.RequiresReplace()`

### Computed-with-prior-state stability
Added `stringplanmodifier.UseStateForUnknown()` on `id`, `region`,
`tenant_id`, `name`, `description` so the framework re-uses prior state
values for computed attributes, suppressing spurious `(known after apply)`
plans across no-op updates.

### Importer
SDKv2 `ImportStatePassthroughContext` → `resource.ImportStatePassthroughID`
on `path.Root("id")` via `ResourceWithImportState`.

### Timeouts
Old `schema.ResourceTimeout{Create, Update, Delete: 10m}` → `timeouts.Block`
from `terraform-plugin-framework-timeouts/resource/timeouts`, preserving the
existing `timeouts { ... }` block HCL syntax. Defaults of 10 minutes are
applied per-CRUD-method when the practitioner does not set one.

### CRUD bodies
Translated per the skill's `references/resources.md` and `state-and-types.md`:
typed model struct, `req.Plan.Get` / `resp.State.Set` instead of `d.Get` /
`d.Set`, `resp.Diagnostics.AddError` instead of `diag.Errorf`. The wait/retry
helpers (`waitForIKEPolicyCreation`, `waitForIKEPolicyDeletion`,
`waitForIKEPolicyUpdate`) are unchanged — they only touch the gophercloud
client, not the schema.

## Test file changes

Two changes to the test file:
1. `ProviderFactories` → `ProtoV6ProviderFactories` per the skill's
   `references/testing.md`. The factory map type changes accordingly to
   `map[string]func() (tfprotov6.ProviderServer, error)`.
2. Added explicit `TestCheckResourceAttr` lines in
   `TestAccIKEPolicyVPNaaSV2_basic` for each of the five `Default` values —
   asserting that omitting the attribute in HCL still produces the expected
   default in state. This is the migration-specific assertion that validates
   the `Default:` migration end-to-end (it would catch a regression where
   the framework `Default:` field was forgotten or misspelled).

The acceptance-test config HCL is unchanged — `lifetime { ... }` block
syntax is preserved by the `SetNestedBlock` choice above.

## Caveats — partial migration only

The repository's other ~250 resources are still SDKv2. Two consequences:

1. **`testAccProtoV6ProviderFactoriesIKE` is a forward reference** to a
   factory that lives in a sibling file (`provider_framework.go`) added
   alongside the framework provider scaffolding (workflow step 3). That file
   is out of scope for this single-resource migration; once it exists, the
   factory wires in `NewIKEPolicyV2Resource()` so the test can address the
   migrated resource via protocol v6.
2. **Provider registration**: `provider.go` line 495 still references
   `resourceIKEPolicyV2()` (the SDKv2 constructor). When this resource is
   actually cut over to the framework, that line needs to be removed and
   `NewIKEPolicyV2Resource` registered on the framework provider's
   `Resources()` method instead. (Doing both would double-register.)

Per SKILL.md "What to never do", we deliberately did NOT introduce
`terraform-plugin-mux`. The provider-level scaffolding to serve framework
resources alongside SDKv2 resources via a single binary is part of step 3
of the workflow (`references/protocol-versions.md`) and not implemented as
part of this single-resource migration eval.

## Verification gates not run

Per the eval rules, we did NOT run `go build`, `go vet`, `go test`, or
`go mod tidy` against the openstack clone (the clone is read-only). The
files were authored by hand against the bundled framework references; the
intended verification path is `scripts/verify_tests.sh` after the
provider-level scaffolding is in place.
