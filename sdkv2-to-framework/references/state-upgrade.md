# State upgraders (schema versions)

## Quick summary
- **Single-step semantics, not chained.** SDKv2 chained `StateUpgraders` V0→V1→V2; the framework's `UpgradeState` returns a map keyed by *prior* version, where each entry produces the *current* (target) state in one call.
- Set `Version` on the resource schema to the current version. Each `UpgradeState` entry must define a `PriorSchema` so the framework knows the shape it's upgrading from.
- The migration trap: an SDKv2 chain V0→V1→V2 becomes two framework upgraders — `0 → current` and `1 → current` — not three. Each must produce the target schema directly.
- For data extraction: read the prior state into a typed model that matches `PriorSchema`, transform, write a typed model that matches the *current* schema.
- Implement `resource.ResourceWithUpgradeState` and add `var _ resource.ResourceWithUpgradeState = &thingResource{}` so a missing method is a compile error.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [State upgrade migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/state-upgrade)
- [ResourceWithUpgradeState source](https://github.com/hashicorp/terraform-plugin-framework/blob/main/resource/state_upgrader.go)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
