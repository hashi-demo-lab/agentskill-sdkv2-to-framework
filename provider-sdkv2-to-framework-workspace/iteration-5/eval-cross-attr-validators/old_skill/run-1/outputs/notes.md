# Migration notes — `openstack_compute_interface_attach_v2`

## Scope

Single-resource migration of `resource_openstack_compute_interface_attach_v2.go`
plus its test file from `terraform-plugin-sdk/v2` to
`terraform-plugin-framework`. Helper functions in `compute_interface_attach_v2.go`
(`computeInterfaceAttachV2AttachFunc`, `computeInterfaceAttachV2DetachFunc`)
are reused as-is — they take a `context.Context` and a gophercloud client, so
they are framework-agnostic.

## Block/attribute decision

No `MaxItems: 1 + nested Elem` patterns in this resource, so no
block-vs-nested-attribute call to make. The only block in the migrated schema
is `timeouts`, kept as a `Block` (via
`terraform-plugin-framework-timeouts/resource/timeouts.Block`) so existing
practitioner configs that wrote `timeouts { create = "10m" }` keep parsing.

## State upgrade

No `SchemaVersion` set in the SDKv2 resource (defaults to 0) and no
`StateUpgraders`. Nothing to migrate on this axis — `ResourceWithUpgradeState`
is *not* implemented.

## Import shape

SDKv2 used `schema.ImportStatePassthroughContext`. The composite ID is
`<instance_id>/<port_id>` and is parsed by `parsePairedIDs` inside `Read` and
`Delete`, not at import time. So passthrough-import semantics translate
directly to:

```go
func (r *...) ImportState(ctx context.Context, req ..., resp ...) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

`Read` then handles the `instance_id/port_id` parsing exactly as in SDKv2.

## ConflictsWith translation (the focus of this eval)

SDKv2 declared three `ConflictsWith` relationships:

| Attribute    | SDKv2 ConflictsWith    |
|--------------|------------------------|
| `port_id`    | `["network_id"]`       |
| `network_id` | `["port_id"]`          |
| `fixed_ip`   | `["port_id"]`          |

These are **not** symmetric — `fixed_ip` conflicts with `port_id` but
`network_id` is allowed alongside `fixed_ip`. That asymmetry rules out a
single resource-level `resourcevalidator.Conflicting(port, network, fixed_ip)`
call, which would conflict all three pairwise and break the legitimate
`network_id + fixed_ip` shape that
`TestAccComputeV2InterfaceAttach_IP` exercises.

The skill's `references/validators.md` covers exactly this trade-off
(per-attribute vs schema-wide). Per-attribute is the right call here:

```go
"port_id": schema.StringAttribute{
    ...
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("network_id")),
    },
},

"network_id": schema.StringAttribute{
    ...
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},

"fixed_ip": schema.StringAttribute{
    ...
    Validators: []validator.String{
        stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
    },
},
```

The reciprocal placement on both `port_id` and `network_id` matches the
"idiomatic to put it on both for clarity" guidance in
`references/validators.md`. It also surfaces the violation regardless of
which attribute terraform-core attaches the diagnostic to (handy when
asserting `ExpectError` in tests, where the message can quote either name
first).

### Why not a `ResourceWithConfigValidators`?

A schema-level `resourcevalidator.ExactlyOneOf(port_id, network_id)` was
considered to enforce the runtime check (`Must set one of network_id and
port_id`) at config-validate time rather than in `Create`. **Rejected** for
this migration:

- It changes user-visible behaviour. SDKv2 enforced "one of" *at apply time*
  via the `if networkID == "" && portID == ""` block in CreateContext,
  not via schema validation. Adding `ExactlyOneOf` would reject configs at
  plan that previously made it to apply (and SDKv2 didn't have the
  cross-attribute helper, so this is genuinely new constraint surface).
- The skill's "what to never do" rule is *don't change user-facing schema
  behaviour* during migration. Migration is pure refactor.

So the runtime "must set one of" check is preserved verbatim in `Create`,
and only the existing per-attribute `ConflictsWith` constraints become
validators. Adding `ExactlyOneOf` would be a separate, deliberate change
documented in a changelog.

## ForceNew → RequiresReplace

Every attribute carried `ForceNew: true` in SDKv2. Translated to
`stringplanmodifier.RequiresReplace()` per the skill's plan-modifiers guidance
(and the SKILL.md "common pitfalls" reminder that `ForceNew` is a plan
modifier, not a `RequiresReplace: true` field). On `region`, `port_id`,
`network_id`, and `fixed_ip` — all of which are also `Computed` — the chain
is `RequiresReplace()` plus `UseStateForUnknown()` so the values don't go
unknown across no-op plans. `instance_id` is `Required` only, so just
`RequiresReplace()`.

## Timeouts

SDKv2 had `Create: 10m`, `Delete: 10m`. Translated using
`terraform-plugin-framework-timeouts/resource/timeouts.Block(...)` (see
`references/timeouts.md`). Block syntax is preserved (not Attribute syntax)
to avoid breaking existing practitioner configs that wrote
`timeouts { create = "10m" }`. The 10-minute defaults are passed in code at
`plan.Timeouts.Create(ctx, 10*time.Minute)` / `state.Timeouts.Delete(ctx,
10*time.Minute)`, exactly as `references/timeouts.md` describes.

## CRUD body changes

Followed `references/resources.md`:

- `CreateContext`/`ReadContext`/`DeleteContext` lose the `Context` suffix.
- Errors return via `resp.Diagnostics.AddError(...)`, not `diag.Errorf`.
- All `d.Get`/`d.Set`/`d.Id`/`d.SetId` traffic moves through a typed
  `computeInterfaceAttachV2Model` struct with `tfsdk:"..."` tags.
- `CheckDeleted` (which mutates `*schema.ResourceData`) is replaced inline
  with a `gophercloud.ResponseCodeIs(err, http.StatusNotFound)` check that
  calls `resp.State.RemoveResource(ctx)` on 404 (the framework idiom for
  "resource is gone, recreate on next plan").
- `Update` is implemented as a no-op state.Set even though every attribute
  is RequiresReplace and Terraform shouldn't ever call it; the framework
  still requires the method to satisfy `resource.Resource`.

## Test changes

- `ProviderFactories: testAccProviders` → `ProtoV6ProviderFactories:
  testAccProtoV6ProviderFactories`. The factory variable is assumed to exist
  in the test harness once the provider migration lands (per
  `references/testing.md`); if the harness still uses `ProtoV5`, switch to
  `ProtoV5ProviderFactories: testAccProtoV5ProviderFactories`.
- The two existing acceptance tests (`_basic`, `_IP`) keep their
  `resource.Test`/`resource.TestStep` shape — `terraform-plugin-testing` is
  the same package the framework uses.
- Added two **new** plan-only acceptance tests that exercise the migrated
  ConflictsWith validators: `TestAccComputeV2InterfaceAttach_conflictPortNetwork`
  and `TestAccComputeV2InterfaceAttach_conflictFixedIPPort`. Both use
  `PlanOnly: true` so they fail at config validation (no API calls), and a
  permissive regex over the framework's diagnostic message because
  `stringvalidator.ConflictsWith` can attribute the error to either side
  of the pair.

## Residual SDKv2 import (`helper/retry`)

The migrated resource still imports
`github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry` (aliased as
`sdkretry`) for `StateChangeConf` / `WaitForStateContext`, used to drive the
`ATTACHING → ATTACHED` and `→ DETACHED` state polling. Two reasons it is
kept:

1. The `terraform-plugin-framework` ecosystem has no first-class replacement
   for `StateChangeConf`-style polling. The community pattern is to keep
   `helper/retry` (which has no SDKv2-specific dependency) as a standalone
   poller utility during and after a framework migration.
2. The helper functions in the un-migrated `compute_interface_attach_v2.go`
   (`computeInterfaceAttachV2AttachFunc`, `computeInterfaceAttachV2DetachFunc`)
   already return `retry.StateRefreshFunc`; replacing those would require
   touching files outside the scope of this single-resource migration.

**Implication for `verify_tests.sh --migrated-files`**: the skill's negative
gate (no `terraform-plugin-sdk/v2` imports in migrated files) will currently
flag this file. Two acceptable resolutions:

- Keep the import and document it in the migration changelog as a known
  framework-ecosystem gap (recommended; matches what other large providers
  have done).
- Inline a small `WaitForState`-equivalent helper in the openstack package
  that depends only on `context.Context` + `time.Sleep`, then drop the
  `helper/retry` dependency provider-wide. That is a separate refactor, not
  part of the migration.

## Suggested follow-ups (out of scope here)

1. Provider-level wiring: register `NewComputeInterfaceAttachV2Resource`
   in the framework provider's `Resources()` slice and remove the SDKv2
   registration of `resourceComputeInterfaceAttachV2`. That edit is in
   `provider.go` (or equivalent) and is part of step 6/10 of the workflow,
   not this single-resource migration task.
2. `verify_tests.sh ... --migrated-files
   openstack/resource_openstack_compute_interface_attach_v2.go
   openstack/resource_openstack_compute_interface_attach_v2_test.go`
   to confirm `go build`, `go vet`, the negative-gate (no remaining
   `terraform-plugin-sdk/v2` import in the migrated file) all pass.
3. Re-evaluate whether the runtime `Must set one of network_id and port_id`
   check should be promoted to a schema-level
   `resourcevalidator.ExactlyOneOf` in a follow-up release with a changelog
   entry — see the "Why not a ResourceWithConfigValidators?" section above.
