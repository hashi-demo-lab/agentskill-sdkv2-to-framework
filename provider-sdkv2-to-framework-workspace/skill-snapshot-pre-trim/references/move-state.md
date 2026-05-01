# Cross-type and cross-provider state moves (`ResourceWithMoveState`)

## Quick summary
- `ResourceWithMoveState` (framework v1.6.0+) lets state from a *different* resource type — possibly even from a *different* provider — be carried into the new resource as part of an `apply`. Distinct from `UpgradeState` (which handles same-resource schema-version deltas).
- Implement by returning `[]StateMover`, each carrying a `SourceSchema *schema.Schema` (the prior shape, optional) and a `StateMover func(...)` (the transform). The framework tries them in order; the first one that doesn't return an empty response wins.
- `MoveStateRequest` carries `SourceProviderAddress`, `SourceTypeName`, `SourceSchemaVersion`, `SourceRawState`, and `SourceIdentity` — enough to route by provider + resource type before transforming.
- Use this when migrating renames a resource, splits one resource into two, or moves a resource from a sibling provider (e.g. consolidating a community provider into an official one).
- Do NOT use this for plain SDKv2 → framework version migration of the *same* resource — that's `UpgradeState` (`references/state-upgrade.md`).

## When you need it

Three scenarios in the wild:

1. **Rename during migration.** SDKv2 resource was named `myprov_widget`; the migrated framework resource is `myprov_widget_v2`. Practitioners with `myprov_widget` in state need a path forward without `terraform state mv` shenanigans.
2. **Resource split.** SDKv2 resource `myprov_account_with_quota` is being split into `myprov_account` and `myprov_quota`. Each new resource needs to claim part of the old state.
3. **Cross-provider consolidation.** A sibling community provider's resource is being absorbed into an official provider. Practitioners shouldn't have to re-create resources.

If none of these apply, you don't need `ResourceWithMoveState`.

## Old shape (SDKv2)

There isn't one. SDKv2 has no equivalent — the closest is `terraform state mv` run by the practitioner, which is a manual, error-prone step in their migration plan.

## New shape (framework)

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/resource"
    "github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

var _ resource.ResourceWithMoveState = &widgetV2Resource{}

type widgetV1Model struct {
    ID   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
    // ... whatever the old resource had
}

// SourceSchema is *schema.Schema (resource/schema package), NOT a separate type.
// It's the same type the resource itself uses for its current schema.
func sourceSchemaV1() *schema.Schema {
    return &schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   schema.StringAttribute{Computed: true},
            "name": schema.StringAttribute{Required: true},
            // ...
        },
    }
}

func (r *widgetV2Resource) MoveState(ctx context.Context) []resource.StateMover {
    return []resource.StateMover{
        {
            SourceSchema: sourceSchemaV1(),
            StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
                // Route by provider + type. The framework runs every mover in order;
                // return early if this isn't the source we expect.
                if req.SourceProviderAddress != "registry.terraform.io/example/myprov" {
                    return
                }
                if req.SourceTypeName != "myprov_widget" {
                    return
                }

                // SourceState is *tfsdk.State and is nil if SourceSchema wasn't set
                // or the framework couldn't deserialise the prior state with it.
                // Always nil-check before calling Get — otherwise this panics.
                if req.SourceState == nil {
                    return
                }

                var prior widgetV1Model
                resp.Diagnostics.Append(req.SourceState.Get(ctx, &prior)...)
                if resp.Diagnostics.HasError() {
                    return
                }

                // Transform into the current resource's shape.
                current := widgetV2Model{
                    ID:   prior.ID,
                    Name: prior.Name,
                    // map renamed/split fields
                }

                resp.Diagnostics.Append(resp.TargetState.Set(ctx, current)...)
            },
        },
    }
}
```

Practitioners then use a `moved {}` block (Terraform 1.8+) instead of `terraform state mv`:

```hcl
moved {
  from = myprov_widget.foo
  to   = myprov_widget_v2.foo
}
```

## Routing pattern (multiple movers)

Return one `StateMover` per source type you support. Each filters via `req.SourceProviderAddress` + `req.SourceTypeName`:

```go
return []resource.StateMover{
    {SourceSchema: sourceMyprovWidget(),    StateMover: moveFromMyprovWidget},
    {SourceSchema: sourceCommunityWidget(), StateMover: moveFromCommunityWidget},
}
```

Movers are tried in slice order; the first one that produces *any* response wins — that includes a mover that returns only error diagnostics (no state set). So when the source doesn't match, just `return` without writing state and without adding diagnostics — the framework then tries the next mover. Returning an error means "this mover matched but the state is invalid", which short-circuits the chain.

## When to combine with `UpgradeState`

These features are independent. A migration *can* use both:

- `MoveState` for the rename/split (handle the type-name change).
- `UpgradeState` for any schema-version deltas within the new type.

Order: Terraform applies `MoveState` first (changing the resource's address), then `UpgradeState` if the moved-in state is at a prior schema version.

## Identity moves

If the source resource has an identity schema (and the destination has one too), the request and response surfaces are *asymmetric*:

- `req.SourceIdentity` is `*tfprotov6.RawState` (raw, untyped — no `Get(ctx, &model)` method available, because the framework doesn't know the source identity's typed shape). To extract typed fields, call `req.SourceIdentity.Unmarshal(sourceIdentityType)` against the `tftypes.Type` describing the source identity. `req.SourceIdentitySchemaVersion int64` carries the source identity's schema version.
- `resp.TargetIdentity` IS a typed `*tfsdk.ResourceIdentity` you can `Set` against directly.

```go
// Extract source identity from raw protobuf state.
sourceIdentityType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
    "region": tftypes.String,
    "id":     tftypes.String,
}}
sourceIdentVal, err := req.SourceIdentity.Unmarshal(sourceIdentityType)
if err != nil {
    resp.Diagnostics.AddError("decoding source identity failed", err.Error())
    return
}
// Pull individual fields from sourceIdentVal (a tftypes.Value) using As(...).
// Then transform into the current identity model and write it typed:
currentIdent := widgetV2IdentityModel{ /* ... */ }
resp.Diagnostics.Append(resp.TargetIdentity.Set(ctx, currentIdent)...)
```

For most migrations, identity moves aren't needed — the source provider didn't have identity in the first place. Reach for this only when both the old and new resource have identity schemas.

See `references/identity.md` for the identity primitives.

## Don't surprise practitioners

`ResourceWithMoveState` is invisible to practitioners until they write a `moved {}` block. If you're using it as part of a migration, document the new `moved` blocks they need to add in the changelog. Practitioners *will* be confused by `terraform plan` output that says a resource is being created/destroyed unless they wrote the `moved` block first.

## Compatibility

| Feature | Min framework version | Min Terraform CLI |
|---|---|---|
| `ResourceWithMoveState` | v1.6.0 (Feb 2024) | 1.8 (for `moved` blocks across types) |
| `req.SourceIdentity` / `resp.TargetIdentity` | v1.15.0 | 1.12 |
