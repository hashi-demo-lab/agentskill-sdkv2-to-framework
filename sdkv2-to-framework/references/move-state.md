# Cross-type and cross-provider state moves (`ResourceWithMoveState`)

## Quick summary
- `ResourceWithMoveState` (framework v1.6.0+) lets state from a *different* resource type — possibly even from a *different* provider — be carried into the new resource as part of an `apply`. Distinct from `UpgradeState` (which handles same-resource schema-version deltas).
- Implement by returning `[]StateMover`, each carrying a `SourceSchema *schema.Schema` (the prior shape, optional) and a `StateMover func(...)` (the transform). The framework tries them in order; the first one that doesn't return an empty response wins.
- `MoveStateRequest` carries `SourceProviderAddress`, `SourceTypeName`, `SourceSchemaVersion`, `SourceRawState`, and `SourceIdentity` — enough to route by provider + resource type before transforming.
- Use this when migrating renames a resource, splits one resource into two, or moves a resource from a sibling provider (e.g. consolidating a community provider into an official one).
- Do NOT use this for plain SDKv2 → framework version migration of the *same* resource — that's `UpgradeState` (`references/state-upgrade.md`).

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Move state across resource types](https://developer.hashicorp.com/terraform/plugin/framework/resources/state-move)
- [StateMover source](https://github.com/hashicorp/terraform-plugin-framework/blob/main/resource/state_mover.go)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
