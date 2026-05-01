# Migration Summary — digitalocean_firewall

## What was migrated

`digitalocean/firewall/resource_firewall.go` migrated from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`.

## Key decisions

### Block-vs-attribute: inbound_rule / outbound_rule
These are `TypeSet + Elem: &schema.Resource` in SDKv2. This is a mature resource with practitioner configs using block syntax (`inbound_rule { ... }`). Decision: keep as **blocks** — but because the framework's `SetNestedAttribute` maps to the same HCL block syntax, this is actually `schema.SetNestedAttribute` which practitioners still write as blocks. No breaking HCL change.

### CustomizeDiff → ModifyPlan
The SDKv2 `CustomizeDiff` performed two checks:
1. At least one inbound or outbound rule must exist.
2. `port_range` is required for `tcp`/`udp` protocol rules.

These are now implemented in `ModifyPlan` on the `firewallResource` type, satisfying `resource.ResourceWithModifyPlan`.

### Import
`ImportStatePassthroughContext` → `resource.ImportStatePassthroughID` on the `ImportState` method, satisfying `resource.ResourceWithImportState`.

### Tags
The `tag.TagsSchema()` / `tag.FlattenTags()` / `tag.ExpandTags()` helpers from the SDKv2-based tag package are not used in the migrated file. Tags are handled as `schema.SetAttribute{ElementType: types.StringType}` directly, with inline expand/flatten logic in `buildFirewallRequest` and `refreshState`.

### pending_changes
Mapped to `schema.ListNestedAttribute{Computed: true}` — preserves the read-only list-of-objects shape from SDKv2.

### droplet_ids / port IDs
SDKv2 used `schema.TypeInt` (Go `int`). Framework default is `Int64`. Mapped to `types.Int64Type` / `schema.Int64Attribute`. The godo API uses `[]int`, so casts are done inline in expand/flatten helpers.

## Interface assertions added

```go
var (
    _ resource.Resource                = &firewallResource{}
    _ resource.ResourceWithConfigure   = &firewallResource{}
    _ resource.ResourceWithImportState = &firewallResource{}
    _ resource.ResourceWithModifyPlan  = &firewallResource{}
)
```

## Test file changes

- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` → `github.com/hashicorp/terraform-plugin-testing/helper/resource`
- `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` → `github.com/hashicorp/terraform-plugin-testing/terraform`
- `ProviderFactories: acceptance.TestAccProviderFactories` → `ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories`
- Destroy/exists helpers updated to use `acceptance.TestAccGodoClient()` instead of `acceptance.TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()`

## Acceptance package changes required (not done in this task scope)

The following must be added to `digitalocean/acceptance/acceptance.go` as part of the broader provider migration:
1. `TestAccProtoV6ProviderFactories` — a `map[string]func() (tfprotov6.ProviderServer, error)` using `providerserver.NewProtocol6WithError`.
2. `TestAccGodoClient()` — returns a `*godo.Client` without depending on `*schema.Provider.Meta()`.

## SDKv2 removals

- No `github.com/hashicorp/terraform-plugin-sdk/v2` imports remain in `resource_firewall.go`.
- `customdiff` helpers: none were used; the entire `CustomizeDiff` func was inline.
- `schema.ResourceDiff`, `schema.Set`, `schema.HashString` — all removed.
- `diag.Errorf` / `diag.FromErr` — replaced with `resp.Diagnostics.AddError`.
- `d.SetId("")` / `d.SetId(id)` — replaced with `resp.State.RemoveResource(ctx)` and `types.StringValue(id)`.
- `firewallRequest(d *schema.ResourceData, client)` helper — replaced with `buildFirewallRequest(ctx, plan)` returning `(*godo.FirewallRequest, diag.Diagnostics)`.
- `firewallPendingChanges`, `flattenFirewallInboundRules`, `flattenFirewallOutboundRules`, `flattenFirewallDropletIds`, `flattenFirewallRuleStringSet`, `expandFirewallInboundRules`, `expandFirewallOutboundRules`, `expandFirewallDropletIds`, `expandFirewallRuleStringSet` from `firewalls.go` — replaced with inline framework-typed equivalents in `resource_firewall.go`. The `firewalls.go` file is not modified (it remains SDKv2-based and will need migration separately).

## Files produced

- `outputs/migrated/resource_firewall.go` — framework resource, no SDKv2, ModifyPlan method, var _ assertions
- `outputs/migrated/resource_firewall_test.go` — framework test file using ProtoV6ProviderFactories
- `outputs/agent_summary.md` — this file
