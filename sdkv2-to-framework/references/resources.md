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
    _ resource.Resource              = &thingResource{}
    _ resource.ResourceWithConfigure = &thingResource{}
    _ resource.ResourceWithImportState = &thingResource{}
)

func NewThingResource() resource.Resource { return &thingResource{} }

type thingResource struct {
    client *Client
}

type thingModel struct {
    ID   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func (r *thingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
    resp.TypeName = req.ProviderTypeName + "_thing"
}

func (r *thingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
            "name": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
        },
    }
}

func (r *thingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
    if req.ProviderData == nil { return }
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
    if err != nil {
        resp.Diagnostics.AddError("create failed", err.Error())
        return
    }
    plan.ID = types.StringValue(id)
    resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *thingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state thingModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() { return }

    out, err := r.client.Get(state.ID.ValueString())
    if err != nil {
        if isNotFound(err) {
            resp.State.RemoveResource(ctx) // recreate on next plan
            return
        }
        resp.Diagnostics.AddError("read failed", err.Error())
        return
    }
    state.Name = types.StringValue(out.Name)
    resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *thingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
    var plan thingModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() { return }

    if err := r.client.Update(plan.ID.ValueString(), plan.Name.ValueString()); err != nil {
        resp.Diagnostics.AddError("update failed", err.Error())
        return
    }
    resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *thingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
    var state thingModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    if resp.Diagnostics.HasError() { return }
    if err := r.client.Delete(state.ID.ValueString()); err != nil {
        resp.Diagnostics.AddError("delete failed", err.Error())
    }
}

func (r *thingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
```

## CRUD method signature changes

| SDKv2 | Framework |
|---|---|
| `CreateContext func(ctx, d *schema.ResourceData, m interface{}) diag.Diagnostics` | `Create(ctx, req resource.CreateRequest, resp *resource.CreateResponse)` |
| `ReadContext func(ctx, d, m) diag.Diagnostics` | `Read(ctx, req resource.ReadRequest, resp *resource.ReadResponse)` |
| `UpdateContext func(ctx, d, m) diag.Diagnostics` | `Update(ctx, req resource.UpdateRequest, resp *resource.UpdateResponse)` |
| `DeleteContext func(ctx, d, m) diag.Diagnostics` | `Delete(ctx, req resource.DeleteRequest, resp *resource.DeleteResponse)` |

Returns become diagnostics on the response object instead of a return value.

## State / plan / config access

| SDKv2 | Framework |
|---|---|
| `d.Get("name").(string)` | `var m model; req.Plan.Get(ctx, &m); m.Name.ValueString()` |
| `d.Set("name", "foo")` | `m.Name = types.StringValue("foo"); resp.State.Set(ctx, m)` |
| `d.Id()` | `state.ID.ValueString()` (you keep an `ID` field on the model) |
| `d.SetId("...")` | `m.ID = types.StringValue("...")` then `resp.State.Set(ctx, m)` |
| `d.SetId("")` (drift cleanup) | `resp.State.RemoveResource(ctx)` |
| `d.GetChange("name")` | compare `req.State` and `req.Plan` (Update only) |
| `d.HasChange("name")` | `!plan.Name.Equal(state.Name)` |
| `d.IsNewResource()` | only relevant in Create — check whether `req.State.Raw.IsNull()` |

## Read drift handling

In SDKv2 you call `d.SetId("")` when the resource is gone to trigger recreation. In the framework, call `resp.State.RemoveResource(ctx)`. Doing nothing is wrong — Terraform will think the resource still exists.

## Diagnostics

Replace every `diag.FromErr(err)` and `diag.Errorf(...)` with calls on `resp.Diagnostics`:

```go
resp.Diagnostics.AddError("operation failed", err.Error())
resp.Diagnostics.AddWarning("deprecated field used", "the X field will be removed")
resp.Diagnostics.AddAttributeError(path.Root("name"), "name too long", "max 64 characters")
```

After calls that may add diagnostics, check `resp.Diagnostics.HasError()` and return early.

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
