# State upgraders (schema versions)

## Quick summary
- **Single-step semantics, not chained.** SDKv2 chained `StateUpgraders` V0→V1→V2; the framework's `UpgradeState` returns a map keyed by *prior* version, where each entry produces the *current* (target) state in one call.
- Set `Version` on the resource schema to the current version. Each `UpgradeState` entry must define a `PriorSchema` so the framework knows the shape it's upgrading from.
- The migration trap: an SDKv2 chain V0→V1→V2 becomes two framework upgraders — `0 → current` and `1 → current` — not three. Each must produce the target schema directly.
- For data extraction: read the prior state into a typed model that matches `PriorSchema`, transform, write a typed model that matches the *current* schema.
- Implement `resource.ResourceWithUpgradeState` and add `var _ resource.ResourceWithUpgradeState = &thingResource{}` so a missing method is a compile error.

## Why single-step matters

In SDKv2, chained upgraders worked because each one could assume the previous had already run. So V0→V1 didn't need to know about V2; V1→V2 didn't need to know about V0. The chain composed.

The framework chose a different design: each upgrader takes you all the way to the *current* version. The reason is that it's easier to reason about correctness when each upgrader is a complete transformation — you don't have to mentally compose four upgraders to know what V0 state ends up as. The cost is that adding a new schema version means revisiting *every* prior upgrader to teach it the new target.

## Old shape (SDKv2)

```go
&schema.Resource{
    SchemaVersion: 2,
    StateUpgraders: []schema.StateUpgrader{
        {Version: 0, Type: resourceThingV0().CoreConfigSchema().ImpliedType(), Upgrade: upgradeV0ToV1},
        {Version: 1, Type: resourceThingV1().CoreConfigSchema().ImpliedType(), Upgrade: upgradeV1ToV2},
    },
}

func upgradeV0ToV1(ctx context.Context, raw map[string]interface{}, m interface{}) (map[string]interface{}, error) {
    raw["new_field"] = "default"
    return raw, nil
}
```

The chain is V0 → V1 → V2.

## New shape (framework)

```go
var _ resource.ResourceWithUpgradeState = &thingResource{}

func (r *thingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
    resp.Schema = schema.Schema{
        Version: 2, // current
        Attributes: map[string]schema.Attribute{ /* current shape */ },
    }
}

func (r *thingResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            PriorSchema:   priorSchemaV0(),
            StateUpgrader: upgradeFromV0,
        },
        1: {
            PriorSchema:   priorSchemaV1(),
            StateUpgrader: upgradeFromV1,
        },
    }
}

func priorSchemaV0() *schema.Schema {
    return &schema.Schema{
        Attributes: map[string]schema.Attribute{
            "id":   schema.StringAttribute{Computed: true},
            "name": schema.StringAttribute{Required: true},
            // ... whatever V0 had
        },
    }
}

type thingModelV0 struct {
    ID   types.String `tfsdk:"id"`
    Name types.String `tfsdk:"name"`
}

func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
    var prior thingModelV0
    resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
    if resp.Diagnostics.HasError() { return }

    // Transform V0 → current.
    current := thingModel{
        ID:       prior.ID,
        Name:     prior.Name,
        NewField: types.StringValue("default"),
        // ... fill in everything the current schema requires.
    }

    resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
```

Note: the upgrader keyed at `0` produces the *current* state, not V1 state. Same for the upgrader keyed at `1`.

## Migrating an SDKv2 chain

If you had V0→V1 and V1→V2 in SDKv2:

1. Write `priorSchemaV0()` and `priorSchemaV1()` matching the SDKv2 shapes at those versions (you may need to dig the historical schema out of git history).
2. For `upgradeFromV0`, *compose* the SDKv2 V0→V1 transformation with V1→V2 to produce one direct upgrader.
3. For `upgradeFromV1`, port the SDKv2 V1→V2 transformation directly.
4. Keep V0 → current logic separate from V1 → current logic — don't try to call one from the other; the testability suffers.

## Common pitfalls

- **Bind the upgrader as a method, not a free function, when it needs the API client.** The framework's `UpgradeStateRequest` does *not* carry provider meta — there's no `req.ProviderData` field on this request. If your V0→current transformation needs to call the API (e.g., to look up a derived field that V0 didn't store), the upgrader function must close over `r.client` by being a method on the resource type, not a free function. Pattern: `func (r *thingResource) upgradeFromV0(ctx, req, resp)` then `StateUpgrader: r.upgradeFromV0` in the `UpgradeState()` map. This is the most-common upgrader bug and it's silent until the upgrader actually runs.
- **Forgetting `PriorSchema`**: the framework needs it to deserialise prior state. Without it, you get a runtime error when an old state is loaded.
- **Returning V1 from the V0 upgrader (chain habit)**: this leaves the state two versions behind. The next plan will fail because Terraform will think the schema version mismatch persists.
- **Not handling new required fields**: when migrating V0 → current, every field that's `Required` *now* must get a value, either from V0 data or a sensible default. If the field has no analogue in V0, document the migration assumption.
- **Reading prior state via `types.X` types that don't match `PriorSchema`**: the prior model's `tfsdk:` tags must exactly match `PriorSchema`'s attribute names and types — the framework deserialises through the prior schema, not the current one.

## Testing state upgrades

Acceptance tests can pin a state file from the prior version:

```go
resource.Test(t, resource.TestCase{
    // ProtoV6ProviderFactories goes on the TestCase, NOT inside individual TestSteps.
    // Steps without ExternalProviders use the TestCase-level factories.
    ProtoV6ProviderFactories: protoV6ProviderFactories,
    Steps: []resource.TestStep{
        {
            // Step 1: write V0 state using the published SDKv2 provider.
            // ExternalProviders overrides the TestCase-level factories for this step.
            ExternalProviders: map[string]resource.ExternalProvider{
                "myprov": {
                    VersionConstraint: "= 1.x.x", // your last SDKv2 release
                    Source:            "registry.terraform.io/.../myprov",
                },
            },
            Config: testAccConfigV0,
        },
        {
            // Step 2: migrated provider (TestCase-level factories), assert no plan diff.
            Config:   testAccConfigCurrent,
            PlanOnly: true,
        },
    },
})
```

The first step writes a V0 state with the published SDKv2 provider; the second step uses the migrated provider (inherited from the TestCase-level `ProtoV6ProviderFactories`) and asserts no plan diff. If there's a diff, the upgrader is incomplete.
