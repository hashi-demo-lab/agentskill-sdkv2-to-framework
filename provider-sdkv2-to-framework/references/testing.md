# Acceptance and unit testing

## Quick summary
- Use `terraform-plugin-testing` (current name; replaces SDKv2's `helper/resource`) — the same package the framework uses.
- `ProviderFactories` → `ProtoV6ProviderFactories` (or `ProtoV5ProviderFactories` for v5).
- `r.Test(t, resource.TestCase{...})` is unchanged in shape; the factories field is what differs.
- TDD ordering matters here (workflow step 7): change the test first, run it red, then migrate the resource. Tests written *after* the migration inherit the migrator's blind spots.
- `TestProvider` is your cheap, fast schema-validity check without `TF_ACC`. Pre-migration this is the SDKv2 `provider.InternalValidate()` call; post-migration there's no single framework equivalent — the closest is calling the provider factory and asserting it returns no error (the schema is exercised whenever a `ProviderServer` is constructed).

## The testing package

```go
import (
    "testing"

    "github.com/hashicorp/terraform-plugin-framework/providerserver"
    "github.com/hashicorp/terraform-plugin-go/tfprotov6"
    "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)
```

(Old SDKv2 path: `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` — sweep these out as part of step 10.)

## Provider factory

### Old

```go
var providerFactories = map[string]func() (*schema.Provider, error){
    "myprov": func() (*schema.Provider, error) { return New(), nil },
}
```

### New (protocol v6)

```go
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
    "myprov": providerserver.NewProtocol6WithError(New("test")()),
}
```

For protocol v5, use `NewProtocol5WithError` and `tfprotov5.ProviderServer` / `ProtoV5ProviderFactories`.

## Test cases

```go
func TestAccThing_basic(t *testing.T) {
    resource.Test(t, resource.TestCase{
        PreCheck:                 func() { testAccPreCheck(t) },
        ProtoV6ProviderFactories: protoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccThingBasicConfig("name1"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("myprov_thing.test", "name", "name1"),
                ),
            },
            {
                ResourceName:      "myprov_thing.test",
                ImportState:       true,
                ImportStateVerify: true,
            },
        },
    })
}
```

`r.Test` is the acceptance-test gate (requires `TF_ACC=1`); `r.UnitTest` runs without `TF_ACC` for tests that don't hit a real cloud.

## TestProvider — cheap schema validation

A common pattern: a single `TestProvider` test that exercises `provider.InternalValidate()` (or its framework equivalent). It catches a huge class of schema errors — invalid attribute combinations, missing types, conflicting `Optional`/`Required`/`Computed` — without any acceptance setup.

For the framework, the equivalent is to construct the provider's schema and verify it matches expectations. The `terraform-plugin-framework` test helpers don't have a single `InternalValidate` call, but the schema is exercised whenever you spin up a `ProviderServer`. A minimal `TestProvider`:

```go
func TestProvider(t *testing.T) {
    if _, err := protoV6ProviderFactories["myprov"](); err != nil {
        t.Fatalf("provider failed to start: %v", err)
    }
}
```

Calls the factory, which constructs the provider, which evaluates its schema. Any malformed schema panics or returns an error here.

## TDD ordering (workflow step 7)

Step 7 says to update tests *first* and run them red, then migrate. In practice:

1. Update the test file to use `ProtoV6ProviderFactories` and any framework-style assertions.
2. Run `go test -run TestAccThing_basic ./...` — it should fail because the resource is still SDKv2 (mismatched protocol, or missing factory).
3. Migrate the resource.
4. Run again — green.

The reason this matters: if you migrate the resource first and then write tests, the tests can pass on incorrect behaviour because you wrote them to match what you just shipped. Red-then-green proves the test exercises real change.

## Helpful test step fields

| Field | Use |
|---|---|
| `Config` | the HCL to apply for this step |
| `Check` | post-apply assertions; usually `resource.ComposeTestCheckFunc(...)` |
| `PlanOnly: true` | don't apply, just plan; useful for asserting no-diff |
| `ExpectNonEmptyPlan: true` | flip if a non-zero plan is the expected outcome |
| `ImportState: true` + `ImportStateVerify: true` | round-trip import and assert fidelity |
| `ImportStateIdFunc` | for composite IDs (see `import.md`) |
| `ImportStateVerifyIgnore` | exclude attributes from import-verify (write-only, server-rewritten) |
| `ConfigStateChecks` | framework-only step-level state assertions, type-aware |
| `ExternalProviders` | use a published version of another (or this) provider for this step — useful for testing state upgrades |

## ConfigPlanChecks — assert plan shape pre-apply

For migration-specific assertions, plan-time checks are more useful than state-time ones because they catch "the migrated resource produces a different *plan* than the SDKv2 baseline" before the apply happens. `plancheck.ExpectEmptyPlan()` is the most-used variant for migrations:

```go
import "github.com/hashicorp/terraform-plugin-testing/plancheck"

resource.TestStep{
    Config: testAccThingBasicConfig("name1"),
    ConfigPlanChecks: resource.ConfigPlanChecks{
        PreApply: []plancheck.PlanCheck{
            plancheck.ExpectEmptyPlan(),
        },
    },
}
```

Other common plan checks: `ExpectResourceAction(addr, action)`, `ExpectKnownValue(addr, jsonpath, expected)`, `ExpectUnknownValue(addr, jsonpath)`, `ExpectSensitiveValue(addr, jsonpath)`. For state-upgrade migrations, pair `ExpectEmptyPlan()` after the upgrade step to assert no drift.

## ConfigStateChecks (framework-only)

For typed state assertions that don't fit `TestCheckResourceAttr`'s string-based comparisons:

```go
import "github.com/hashicorp/terraform-plugin-testing/statecheck"

resource.TestStep{
    Config: testAccThingBasicConfig("name1"),
    ConfigStateChecks: []statecheck.StateCheck{
        statecheck.ExpectKnownValue("myprov_thing.test", tfjsonpath.New("created_at"), knownvalue.NotNull()),
    },
}
```

Useful for asserting nested attributes, specific value shapes, or "this is null but exists in state" cases that string comparisons can't express cleanly.

## Recovering test signal without TF_ACC

When `TF_ACC` is unset (no creds, no live cloud), the layered checks in `verify_tests.sh` give you:

1. `go build ./...` — compiles
2. `go vet ./...` — passes vet
3. `TestProvider` — provider constructs and serves
4. Non-`TestAcc*` unit tests — your own non-acceptance tests pass
5. (Optional) protocol-v6 smoke — server boots
6. Negative gate — modified files no longer import `terraform-plugin-sdk/v2`

This is *not* equivalent to a full acceptance test, but it catches schema errors, basic type errors, and "you deleted the SDKv2 import but a stale reference remains" — which is most of what's likely to break during a mechanical migration.

## terraform-plugin-testing version

`terraform-plugin-testing` has supported both protocols since v1.0.0. Pin a recent release for the newer assertions you'll likely use (`plancheck`, `statecheck`, identity-aware checks):

```
require github.com/hashicorp/terraform-plugin-testing v1.14.0
```

Earlier versions miss `ConfigStateChecks` and some `statecheck` helpers.
