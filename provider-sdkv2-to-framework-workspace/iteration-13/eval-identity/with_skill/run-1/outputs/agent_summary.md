# Migration summary — `openstack_lb_member_v2`

## Scope

Single resource migration from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`, with
identity schema added so practitioners on Terraform 1.12+ can use
`import { identity = {...} }` blocks.

## Key decisions

- **Block-vs-attribute (timeouts)**: kept `timeouts.Block(...)` to preserve the existing
  `timeouts { create = "5m" }` HCL syntax practitioners already use. Switching to
  attribute syntax would be a breaking HCL change.
- **No `MaxItems: 1` block** in the source schema, so the block-vs-attribute decision
  applies only to the timeouts ergonomics.
- **No state upgrader** required (SDKv2 schema had no `SchemaVersion`).
- **Composite-ID importer** translated to dual-path `ImportState`:
  - Legacy CLI: `terraform import openstack_lb_member_v2.foo <pool>/<member>` →
    `req.ID` parse via `strings.SplitN`.
  - Modern HCL (Terraform 1.12+): `import { identity = { pool_id = "...", member_id = "..." } }` →
    branch on `req.ID == ""` and use `ImportStatePassthroughWithIdentity` per attribute pair.
- **Identity schema**: two `RequiredForImport: true` string attributes (`pool_id`,
  `member_id`). Identity is set in `Create`, `Update`, `Read`, and on the legacy import
  path so the resource is identity-aware regardless of how it was imported.
- **`ForceNew: true`** translated to `stringplanmodifier.RequiresReplace()` /
  `int64planmodifier.RequiresReplace()` (NOT `RequiresReplace: true`).
- **`Default: true` on `admin_state_up`** translated to `booldefault.StaticBool(true)`
  with `Computed: true` (framework requires `Computed` for `Default`).
- **Computed `id` / `region` / `tenant_id` / `weight`** all carry `UseStateForUnknown` to
  avoid `(known after apply)` noise in dependent resources.
- **`d.GetOk` / `getOkExists`** replaced with `IsNull()` / `IsUnknown()` checks against
  the typed plan model.
- **`Set: schema.HashString`** dropped — framework `SetAttribute` handles uniqueness
  internally (per skill pitfall list).
- **Test file**: `ProviderFactories: testAccProviders` flipped to
  `ProtoV6ProviderFactories: testAccProtoV6ProviderFactories` exactly per the skill's
  TDD-red gate guidance. If the symbol doesn't yet exist at provider scope it'll fail
  to compile — that's the desired step-7 red signal.

## Files

- `migrated/resource_openstack_lb_member_v2.go`
- `migrated/resource_openstack_lb_member_v2_test.go`

## Caveats

- Helper symbols that already exist in the package (`Config`, `LoadBalancerV2Client`,
  `waitForLBV2Pool`, `waitForLBV2Member`, `getLbPendingStatuses`,
  `getLbPendingDeleteStatuses`, `osRegionName`, `testAccProvider`, `testAccPreCheck*`,
  `expandToStringSlice`) are referenced as-is; this migration assumes the broader
  provider migration provides framework-friendly equivalents (`getRegionFromString`,
  `retryFunc`, `isResourceGone`, `testAccProtoV6ProviderFactories`). These shims are
  expected to be wired during the provider-level migration (workflow steps 3–5)
  before this resource compiles cleanly.
