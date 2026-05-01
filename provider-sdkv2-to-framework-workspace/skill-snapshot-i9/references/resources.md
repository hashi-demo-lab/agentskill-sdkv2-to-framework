# Resource migration (CRUD)

## Quick summary
- SDKv2 resources are functions returning `*schema.Resource`; framework resources are Go types implementing `resource.Resource` (and optional sub-interfaces for import, state upgrade, etc.).
- Required methods: `Metadata`, `Schema`, `Create`, `Read`, `Update`, `Delete`. `Configure` is optional but almost always implemented.
- `CreateContext`, `ReadContext`, `UpdateContext`, `DeleteContext` lose the `Context` suffix and gain typed `req`/`resp` parameters with `Plan`/`State`/`Config` accessors.
- All state access becomes typed: define a model struct with `tfsdk:"..."` tags, then `req.Plan.Get(ctx, &model)` / `resp.State.Set(ctx, model)`.
- Optional sub-interfaces add capabilities: `ResourceWithImportState`, `ResourceWithUpgradeState`, `ResourceWithConfigure`, `ResourceWithModifyPlan`, `ResourceWithValidateConfig`.

## Old shape (SDKv2)

```go
func resourceThing() *schema.Resource {
    return &schema.Resource{
        CreateContext: resourceThingCreate,
        ReadContext:   resourceThingRead,
        UpdateContext: resourceThingUpdate,
        DeleteContext: resourceThingDelete,
        Schema: map[string]*schema.Schema{ /* ... */ },
        Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext},
    }
}

func resourceThingCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
    client := m.(*Client)
    name := d.Get("name").(string)
    id, err := client.Create(name)
    if err != nil { return diag.FromErr(err) }
    d.SetId(id)
    return resourceThingRead(ctx, d, m)
}
```

## New shape (framework)

```go
var (
    _ resource.Resource                = &thingResource{}
    _ resource.ResourceWithConfigure   = &thingResource{}
    _ resource.ResourceWithImportState = &thingResource{}
)

func NewThingResource() resource.Resource { return &thingResource{} }

type thingResource struct{ client *Client }

type thingModel struct {
    ID   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func (r *thingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_thing"
}

func (r *thingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{Attributes: map[string]schema.Attribute{
        "id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
        "name": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
    }}
}

func (r *thingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil { return } // called before provider Configure on early RPCs
    client, ok := req.ProviderData.(*Client)
    if !ok {
        resp.Diagnostics.AddError("unexpected provider data", fmt.Sprintf("%T", req.ProviderData))
        return
    }
    r.client = client
}

func (r *thingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan thingModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() { return }
    id, err := r.client.Create(plan.Name.ValueString())
    if err != nil { resp.Diagnostics.AddError("create failed", err.Error()); return }
    plan.ID = types.StringValue(id)
    resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read, Update follow the same Plan/State.Get → API call → State.Set shape.
// Read additionally calls resp.State.RemoveResource(ctx) on 404 (drift). Delete reads from req.State (Plan is null on Delete).

func (r *thingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

## CRUD method signatures, state access, diagnostics

These three are demonstrated by the worked example above. Three things to internalise:

- **Signatures**: `CreateContext`/`ReadContext`/`UpdateContext`/`DeleteContext` lose the `Context` suffix and gain typed `(ctx, req, resp)` parameters. Returns become diagnostics on `resp.Diagnostics`, not a return value.
- **State access**: every `d.Get`/`d.Set`/`d.Id`/`d.HasChange` operation translates to typed access on a model struct — the canonical conversion table is in `state-and-types.md`. Read that file before writing CRUD bodies.
- **Diagnostics**: replace `diag.FromErr(err)` / `diag.Errorf(...)` with `resp.Diagnostics.AddError("op", err.Error())` (or `AddWarning` / `AddAttributeError`). After any call that may add diagnostics, check `resp.Diagnostics.HasError()` and return early.

## Computed-but-API-meaningful fields on update

A pattern SDKv2 handled implicitly that the framework makes you think about: an attribute that's `Computed` (the API determines its value) but whose value is also *required by the API on every update call*. SDKv2's `*ResourceData` returned the prior-state value via `d.Get(...)` because state was the only readable source. The framework's behaviour depends on the plan modifier:

- **With `UseStateForUnknown` plan modifier** (idiomatic for stable-after-create computed fields): `req.Plan` carries the prior-state value forward, so `plan.Foo.ValueString()` works directly. No special handling needed. See `references/plan-modifiers.md`.
- **Without `UseStateForUnknown`**: `req.Plan` has the value as `unknown` (Terraform hasn't recomputed it yet); `plan.Foo.ValueString()` returns the empty string. Source from `req.State` instead.

The wrong thing is to call `plan.Foo.ValueString()` on an unknown value and pass empty/zero to the API. The right thing depends on which case you're in:

```go
func (r *thingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    var plan, state thingModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() { return }

    // `default` is Computed without UseStateForUnknown — Plan has it unknown,
    // State has the last-known value.
    apiPayload := updateRequest{
        Name:    plan.Name.ValueString(),    // user-changeable
        Default: state.Default.ValueBool(),  // server-determined; carry forward from state
    }
    // ... call API, then write resp.State from the API *response* (the API may
    // have changed the value as a side effect of the update).
}
```

A third pattern: when the API recomputes the field on every update (e.g., a `last_modified` timestamp the server always overwrites), neither plan nor prior state is right — read state, send it for the round trip, then overwrite the model with the API response value before writing back.

The shape applies whenever an attribute is `Computed` *and* the API requires it as input on update (immutable server-derived fields, defaults the API tracks, virtual provisioning IDs the practitioner never sees, etc.).

## Read drift handling

In SDKv2 you call `d.SetId("")` when the resource is gone to trigger recreation. In the framework, call `resp.State.RemoveResource(ctx)`. Doing nothing is wrong — Terraform will think the resource still exists.

## Optional sub-interfaces

Add by satisfying additional interfaces — declare with `var _ resource.ResourceWith... = &thingResource{}` so a missing method is a compile error:

| Interface | Method | When to add |
|---|---|---|
| `ResourceWithImportState` | `ImportState` | resource has any import support — see `import.md` |
| `ResourceWithUpgradeState` | `UpgradeState` | resource has `SchemaVersion > 0` — see `state-upgrade.md` |
| `ResourceWithConfigure` | `Configure` | resource needs the provider's client — almost always |
| `ResourceWithModifyPlan` | `ModifyPlan` | replaces SDKv2 `CustomizeDiff` |
| `ResourceWithValidateConfig` | `ValidateConfig` | cross-attribute validation that's not expressible per-attribute |
| `ResourceWithIdentity` | `IdentitySchema` | composite-ID resources (region+id, project+zone+name) — see `identity.md` |
| `ResourceWithMoveState` | `MoveState` | resource is being renamed/split, or moved from another provider — see `move-state.md` |
| `ResourceWithUpgradeIdentity` | `UpgradeIdentity` | identity-schema versioning (rare; only needed if you change the identity schema) — see `identity.md` |
| `ResourceWithConfigValidators` | `ConfigValidators` | declarative cross-attribute validators (e.g., `resourcevalidator.ExactlyOneOf`) — see `validators.md` |

### Cross-resource ordering: `resp.Deferred`

Framework v1.9+ added `resp.Deferred = &resource.Deferred{Reason: ...}` for the case where a resource can't be created/read yet because another resource hasn't propagated (e.g., IAM permission propagation delays). Niche and still marked experimental — only reach for it when retry-on-next-plan is the right semantic. Most providers won't need this.

## Replacing `retry.StateChangeConf` (no framework equivalent)

SDKv2 providers commonly use `helper/retry.StateChangeConf.WaitForStateContext` for "wait until the API reports state X" loops (resource ready, attachment detached, etc.). The framework has no equivalent helper — but the pattern itself is just a context-aware ticker poll, which you can write inline in 20 lines. Do NOT keep importing `terraform-plugin-sdk/v2/helper/retry` from migrated files; it fails the negative gate, and the in-file replacement is straightforward.

```go
// Replaces retry.StateChangeConf{Pending: ..., Target: ..., Refresh: f}.WaitForStateContext(ctx).
// Returns the final value (so the caller can extract API response details) or an error.
func waitForState(
    ctx context.Context,
    refresh func() (any, string, error),
    pending, target []string,
    pollInterval, timeout time.Duration,
) (any, error) {
    deadline := time.Now().Add(timeout)
    ticker := time.NewTicker(pollInterval)
    defer ticker.Stop()

    for {
        v, state, err := refresh()
        if err != nil {
            return v, err
        }
        if slices.Contains(target, state) {
            return v, nil
        }
        if !slices.Contains(pending, state) {
            return v, fmt.Errorf("unexpected state %q (pending=%v, target=%v)", state, pending, target)
        }
        if time.Now().After(deadline) {
            return v, fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, state)
        }
        select {
        case <-ctx.Done():
            return v, ctx.Err()
        case <-ticker.C:
        }
    }
}
```

Call sites change shape only slightly — the SDKv2 `Refresh: func() (interface{}, string, error)` is identical to the inline `refresh func() (any, string, error)` (Go's type identity rules make `retry.StateRefreshFunc` assignable here, so existing helpers compile unchanged if they use the same signature).

If the existing SDKv2 sibling helpers (e.g., `resourceFooAttachFunc`, `resourceFooDetachFunc`) return `retry.StateRefreshFunc`, you have two options:
1. **Quick**: keep their signature; Go's identity rules accept the named type wherever an unnamed `func() (any, string, error)` is expected.
2. **Clean**: change their declared return type to `func() (any, string, error)` so neither the resource file nor the helper file imports `helper/retry`. Preferred when migrating multiple resources in the same package.

Pick (1) when you're migrating a single resource and the helpers serve only that resource; (2) when the helpers are shared and the broader migration will sweep them.

## Things you no longer need

- `Set: schema.HashString` (or any `SchemaSetFunc`) — gone, framework handles uniqueness.
- `Exists` — gone; signal absence from `Read` via `RemoveResource`.
- `MigrateState` (the V0/V1 SDKv1 mechanism) — long obsolete.
- `Timeouts` field — see `timeouts.md` for the framework's package-based replacement.

## When the right answer is *not* a resource

Some SDKv2 resources are awkward fits because Terraform's resource lifecycle (Create/Read/Update/Delete with state persistence) doesn't match what the resource is actually doing. The framework adds three other primitives for these cases — they're framework-only (no SDKv2 to migrate from), but if your migration surfaces an SDKv2 resource that fits one of these shapes, signal that to the user:

- **Pure transform / value derivation** — an SDKv2 resource that takes inputs and produces a derived output without lifecycle, or a `data` source that does the same: maps cleanly onto a `function.Function` (provider-defined function, GA in framework v1.8). [Function docs](https://developer.hashicorp.com/terraform/plugin/framework/functions).
- **Short-lived credentials / one-time tokens** — anything fetched from the API that should not be persisted to state: maps onto an `ephemeral.EphemeralResource` (`Open`/`Renew`/`Close` lifecycle, never written to state). See also `data-sources.md`.
- **Imperative side effects without state lifecycle** — operations that don't fit the CRUD model at all (e.g., "trigger this rebuild" / "rotate that key"): maps onto an `action.Action` (technical preview in framework v1.16+).

Don't bloat this skill into covering function/ephemeral/action implementations — they're separate framework features. Just signal the decision so a migrator doesn't mechanically port a misshapen resource. Refer the user to HashiCorp's framework docs for the chosen primitive.
