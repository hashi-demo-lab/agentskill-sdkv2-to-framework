# Resource identity (`ResourceWithIdentity`)

## Quick summary
- **Resource identity** is the framework's first-class answer to composite-ID resources (region+id, project+region+name, etc.). Shipped in `terraform-plugin-framework` v1.15.0 (May 2025).
- A resource defines an *identity schema* alongside its main schema. The identity carries the practitioner-facing addressing data (region, account, project) separately from configuration.
- Implement `resource.ResourceWithIdentity` (and optionally `ResourceWithUpgradeIdentity` for identity-schema versioning).
- Every CRUD request/response now has an `Identity` field. `ImportStatePassthroughWithIdentity` is a helper that mirrors a *single* identity attribute into a *single* state attribute — useful for the simple "ID is the only addressing field" case. For multi-segment composite IDs you implement `ImportState` manually using `req.Identity.GetAttribute` / `resp.State.SetAttribute` per attribute.
- **When to migrate to identity rather than parse a composite ID by hand**: when practitioners are on Terraform 1.12+ and want to write `import { identity = { ... } }` blocks, identity is the framework-idiomatic answer. **Keep the legacy `req.ID` string-parse path in your `ImportState` too** — practitioners on Terraform <1.12 and CLI-driven `terraform import` still need the legacy form.

## Why identity exists

SDKv2 forced everything into a single opaque `ID` string. Practitioners wrote `terraform import myprov_thing.foo us-east-1/abc123` and the provider parsed the slash-delimited string in `Importer.StateContext`. Two problems:
- The format isn't discoverable — practitioners had to read the docs.
- Importing dozens of resources requires generating import IDs manually.

The framework's identity feature exposes a *typed* identity schema. The `import {}` block itself shipped in Terraform 1.5 (June 2023); the `identity = { ... }` payload inside an `import {}` block is the Terraform 1.12+ addition that makes this useful:

```hcl
import {
  to = myprov_thing.foo
  identity = {
    region = "us-east-1"
    id     = "abc123"
  }
}
```

Terraform queries the provider's identity schema, validates the user-supplied identity, and passes it through. No string parsing.

## Old shape (SDKv2 composite-ID importer)

```go
&schema.Resource{
    Schema: map[string]*schema.Schema{
        "region": {Type: schema.TypeString, Required: true, ForceNew: true},
        "id":     {Type: schema.TypeString, Computed: true},
    },
    Importer: &schema.ResourceImporter{
        StateContext: func(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
            parts := strings.SplitN(d.Id(), "/", 2)
            if len(parts) != 2 { return nil, fmt.Errorf("expected region/id, got %q", d.Id()) }
            d.Set("region", parts[0])
            d.SetId(parts[1])
            return []*schema.ResourceData{d}, nil
        },
    },
}
```

## New shape (framework, identity-aware)

> **The two import paths.** A migrated `ImportState` MUST handle both. `req.ID` is non-empty for the legacy `terraform import myprov_thing.foo us-east-1/abc123` form (Terraform <1.12 or anyone using the CLI). `req.Identity` is populated for the modern `import { identity = {...} }` block (Terraform 1.12+). The two are mutually exclusive — branch on `req.ID == ""` to dispatch. If you only handle one, you break either the CLI flow or the new HCL flow. Identity attribute types (`identityschema.StringAttribute`, etc.) have no sensitivity controls at all — the value is part of how the resource is *addressed*, not a secret. Don't put secret addressing data in the identity schema.

```go
import (
    "github.com/hashicorp/terraform-plugin-framework/path"
    "github.com/hashicorp/terraform-plugin-framework/resource"
    "github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
    "github.com/hashicorp/terraform-plugin-framework/resource/schema"
    "github.com/hashicorp/terraform-plugin-framework/types"
)

var (
    _ resource.Resource                   = &thingResource{}
    _ resource.ResourceWithIdentity       = &thingResource{}
    _ resource.ResourceWithImportState    = &thingResource{}
)

type thingIdentityModel struct {
    Region types.String `tfsdk:"region"`
    ID     types.String `tfsdk:"id"`
}

func (r *thingResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
    resp.IdentitySchema = identityschema.Schema{
        Attributes: map[string]identityschema.Attribute{
            "region": identityschema.StringAttribute{RequiredForImport: true},
            "id":     identityschema.StringAttribute{RequiredForImport: true},
        },
    }
}

func (r *thingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
    var plan thingModel
    resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
    if resp.Diagnostics.HasError() { return }

    out, err := r.client.Create(plan.Region.ValueString(), plan.Name.ValueString())
    if err != nil { /* ... */ }

    plan.ID = types.StringValue(out.ID)
    resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

    // Set identity alongside state.
    identity := thingIdentityModel{
        Region: plan.Region,
        ID:     plan.ID,
    }
    resp.Diagnostics.Append(resp.Identity.Set(ctx, identity)...)
}

func (r *thingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
    var state thingModel
    resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
    // req.Identity is also available — useful when state may be missing fields after import.
    // ...
}

func (r *thingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
    // Two paths to support: legacy `terraform import myprov_thing.foo us-east-1/abc123`
    // (req.ID is set, req.Identity is empty) and modern `import { identity = {...} }`
    // (req.ID empty, req.Identity populated). Handle both.

    // Modern path — Terraform 1.12+ supplies identity. Use the helper to copy
    // each identity attribute into the corresponding state attribute. The helper
    // handles ONE attribute pair per call; call it for each piece of the identity.
    if req.ID == "" {
        resource.ImportStatePassthroughWithIdentity(ctx, path.Root("region"), path.Root("region"), req, resp)
        resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"),     path.Root("id"),     req, resp)
        return
    }

    // Legacy path — practitioner ran `terraform import` with a composite ID string.
    // Parse it the same way the SDKv2 importer did.
    parts := strings.SplitN(req.ID, "/", 2)
    if len(parts) != 2 {
        resp.Diagnostics.AddError("invalid import ID", fmt.Sprintf("expected 'region/id', got %q", req.ID))
        return
    }
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("region"), parts[0])...)
    resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"),     parts[1])...)
}
```

`ImportStatePassthroughWithIdentity` takes a single `(stateAttrPath, identityAttrPath)` pair per call. For multi-segment composite IDs, call it once per piece, or use `req.Identity.GetAttribute(...)` and `resp.State.SetAttribute(...)` directly for full control.

Practitioners can then use either form:

```hcl
# Modern — preferred
import {
  to = myprov_thing.foo
  identity = { region = "us-east-1", id = "abc123" }
}

# Legacy fallback — still works
terraform import myprov_thing.foo us-east-1/abc123
```

For the legacy fallback, you still implement classic `ImportState` parsing (see `references/import.md`); the framework dispatches to whichever form the practitioner uses.

## Identity schema versioning

Rare. If the identity schema itself changes (region added to a previously global resource, single-segment split into two), implement `ResourceWithUpgradeIdentity` — semantics mirror `ResourceWithUpgradeState` (single-step, see `references/state-upgrade.md`). Map keyed by prior version; each entry has a `PriorSchema` and an `IdentityUpgrader` that produces the *current* identity directly. Don't chain.

## When to skip identity

- **Single-segment ID resources** where the SDKv2 importer was just `ImportStatePassthroughContext` — identity adds ceremony with little gain. Use `ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` per `references/import.md`.
- **Pre-Terraform-1.12 deployments** — practitioners on older Terraform can't use `import {}` blocks with identity. Identity still works for legacy `terraform import` if you implement both, so this is rarely a blocker.

## Migration checklist additions

When migrating a resource with a composite-ID importer, add to the per-resource checklist:

- [ ] Identity schema defined (or explicit decision to skip identity documented)
- [ ] `Identity` field set in `Create`, `Update`, `Read`
- [ ] `ImportStatePassthroughWithIdentity` (or manual import + identity dual-write) in `ImportState`
- [ ] Acceptance test asserts identity is populated using `statecheck.ExpectIdentity(addr, expected)` or `statecheck.ExpectIdentityValue(addr, path, value)` from `terraform-plugin-testing/statecheck` (v1.13+).

## Compatibility

| Feature | Min framework version | Min Terraform CLI |
|---|---|---|
| `ResourceWithIdentity` | v1.15.0 | 1.12 (for `identity = {...}` inside `import {}` blocks; the `import {}` block itself works on 1.5+) |
| `ResourceWithUpgradeIdentity` | v1.15.0 | 1.12 |
| `ImportStatePassthroughWithIdentity` | v1.15.0 | 1.12 |

If your provider must support Terraform <1.12, document that identity is opt-in and ensure the legacy import path still works.
