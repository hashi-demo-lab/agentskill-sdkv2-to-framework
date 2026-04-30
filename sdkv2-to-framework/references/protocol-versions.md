# Protocol v5 vs v6

## Quick summary
- The framework supports both Terraform Plugin Protocol v5 and v6. SDKv2 defaults to v5; `terraform-plugin-framework` defaults to v6.
- **Default to v6 for single-release migrations.** It's the framework's native protocol and unlocks attribute features that v5 cannot represent natively.
- v5 vs v6 is decided in `main.go` (the `providerserver.Serve` call). Pick at workflow step 3.
- Choose v5 only if you need backward compatibility with Terraform <0.15.4. Modern Terraform (0.15.4+) speaks both, transparently.
- v6 requires a Terraform CLI version that supports it. The release notes for the major version bump should call this out so practitioners on ancient Terraform versions know to upgrade.

## The main.go swap

### Old (SDKv2)

```go
package main

import (
    "github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
    "github.com/example/myprov/internal/provider"
)

func main() {
    plugin.Serve(&plugin.ServeOpts{
        ProviderFunc: provider.New,
    })
}
```

### New (framework, protocol v6)

```go
package main

import (
    "context"
    "flag"
    "log"

    "github.com/hashicorp/terraform-plugin-framework/providerserver"
    "github.com/example/myprov/internal/provider"
)

var version = "dev"

func main() {
    var debug bool
    flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
    flag.Parse()

    err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
        Address:         "registry.terraform.io/example/myprov",
        Debug:           debug,
        ProtocolVersion: 6,
    })
    if err != nil {
        log.Fatal(err.Error())
    }
}
```

The `Address` is your provider's registry path. The `ProtocolVersion: 6` is explicit; if you omit it, the framework defaults to 6 anyway, but stating it documents the choice.

### main.go for protocol v5

If you must serve v5 (e.g., to support Terraform 0.12.x — rarely needed in 2025+):

```go
err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
    Address:         "registry.terraform.io/example/myprov",
    ProtocolVersion: 5,
})
```

Some features (write-only attributes, certain nested-attribute shapes) require protocol v6 and the framework will surface schema-validation errors at provider start if you serve them under v5. Don't pick v5 unless you have a hard backward-compat constraint.

## When v6 is required

- **Write-only attributes** (`WriteOnly: true`)
- **Some nested-attribute features** that don't have a v5 representation
- **Functions** (provider-defined functions) — v6 only
- **Ephemeral resources** — v6 only

If your migration touches any of these, you must use v6.

## When v5 might be the right call

Realistically, almost never. Terraform 0.15.4+ (released April 2021) supports v6. Practitioners pinned to older Terraform have other problems.

The one case: an enterprise environment where the org has frozen Terraform <0.15.4. Even then, the right answer is usually to escalate the freeze, not to twist the provider into v5 shape.

## Communicating the bump

The protocol switch is a *practitioner-visible* change in the sense that it requires a minimum Terraform CLI. Document in the changelog:

```
## v2.0.0 — Plugin Framework migration

This release ports the provider from terraform-plugin-sdk v2 to terraform-plugin-framework.
**Breaking change**: Terraform 0.15.4 or later is required. Configurations are unchanged.
```

User configurations should not need to change. If they do, that's a separate breaking change and it should be documented separately.

## Version negotiation in practice

Terraform asks the provider what protocol versions it supports; the highest-common version is used. In your test setup, factories are protocol-versioned:

```go
ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
    "myprov": providerserver.NewProtocol6WithError(provider.New("test")()),
},
```

Use `NewProtocol5WithError` for v5. Don't mix and match — pick one consistent factory per test.
