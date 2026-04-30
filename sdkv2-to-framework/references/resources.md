# Resource migration (CRUD)

## Quick summary
- SDKv2 resources are functions returning `*schema.Resource`; framework resources are Go types implementing `resource.Resource` (and optional sub-interfaces for import, state upgrade, etc.).
- Required methods: `Metadata`, `Schema`, `Create`, `Read`, `Update`, `Delete`. `Configure` is optional but almost always implemented.
- `CreateContext`, `ReadContext`, `UpdateContext`, `DeleteContext` lose the `Context` suffix and gain typed `req`/`resp` parameters with `Plan`/`State`/`Config` accessors.
- All state access becomes typed: define a model struct with `tfsdk:"..."` tags, then `req.Plan.Get(ctx, &model)` / `resp.State.Set(ctx, model)`.
- Optional sub-interfaces add capabilities: `ResourceWithImportState`, `ResourceWithUpgradeState`, `ResourceWithConfigure`, `ResourceWithModifyPlan`, `ResourceWithValidateConfig`.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Resource migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/resources)
- [resource package source](https://github.com/hashicorp/terraform-plugin-framework/tree/main/resource)
- [Function/ephemeral/action primitives (when not a resource)](https://developer.hashicorp.com/terraform/plugin/framework/functions)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
