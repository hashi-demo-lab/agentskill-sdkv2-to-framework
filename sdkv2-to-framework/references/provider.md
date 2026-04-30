# Provider definition migration

## Quick summary
- SDKv2 providers are a configured `*schema.Provider` value; framework providers are a Go type implementing the `provider.Provider` interface.
- Required methods on the provider type: `Metadata`, `Schema`, `Configure`, `Resources`, `DataSources`. Optional: `Functions`, `EphemeralResources`, etc.
- `ConfigureContextFunc` becomes the `Configure` method, with typed `req.Config.Get(...)` instead of `d.Get`.
- `ResourcesMap` / `DataSourcesMap` become `Resources` / `DataSources` returning slices of constructor functions.
- `main.go` switches from `plugin.Serve` to `providerserver.Serve` — see `protocol-versions.md`.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Provider migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/providers)
- [provider package source](https://github.com/hashicorp/terraform-plugin-framework/tree/main/provider)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
