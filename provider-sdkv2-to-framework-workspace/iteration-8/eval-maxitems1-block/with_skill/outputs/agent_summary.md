# Agent Summary — openstack_lb_pool_v2 Schema Migration

## Task

Migrate the schema (not CRUD methods, not tests) of `openstack_lb_pool_v2` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. Make an explicit decision on the `MaxItems: 1` `persistence` block.

## Source file examined

`<openstack-clone>/openstack/resource_openstack_lb_pool_v2.go`

## Skill workflow steps applied

- **Pre-flight 0** (mux check): Not a muxed migration — request is single-resource, single-release. Proceeding.
- **Pre-flight C** (per-resource think pass): Written inline during analysis.
  - **Block decision**: `persistence` (TypeList, MaxItems:1, Elem &schema.Resource) → keep as `ListNestedBlock + listvalidator.SizeAtMost(1)` per Q1 of the `references/blocks.md` decision tree.
  - **State upgrade**: No `SchemaVersion`, no `StateUpgraders` — nothing to migrate.
  - **Import shape**: Custom importer (`resourcePoolV2Import`) that resolves `listener_id` or `loadbalancer_id` from the API. Not migrated in this schema-only task.

## Schema attributes migrated

| SDKv2 attribute | Framework equivalent | Notes |
|---|---|---|
| `region` | `StringAttribute{Optional, Computed}` | `RequiresReplace` + `UseStateForUnknown` |
| `tenant_id` | `StringAttribute{Optional, Computed}` | `RequiresReplace` + `UseStateForUnknown` |
| `name` | `StringAttribute{Optional}` | |
| `description` | `StringAttribute{Optional}` | |
| `protocol` | `StringAttribute{Required}` | `RequiresReplace`; `stringvalidator.OneOf` |
| `loadbalancer_id` | `StringAttribute{Optional}` | `RequiresReplace`; `stringvalidator.ExactlyOneOf` |
| `listener_id` | `StringAttribute{Optional}` | `RequiresReplace`; `stringvalidator.ExactlyOneOf` |
| `lb_method` | `StringAttribute{Required}` | `stringvalidator.OneOf` |
| `alpn_protocols` | `SetAttribute{ElementType: StringType, Optional, Computed}` | `setvalidator.ValueStringsAre` for element validation |
| `ca_tls_container_ref` | `StringAttribute{Optional}` | |
| `crl_container_ref` | `StringAttribute{Optional}` | |
| `tls_enabled` | `BoolAttribute{Optional}` | |
| `tls_ciphers` | `StringAttribute{Optional, Computed}` | Computed: API returns default when unset |
| `tls_container_ref` | `StringAttribute{Optional}` | |
| `tls_versions` | `SetAttribute{ElementType: StringType, Optional, Computed}` | `setvalidator.ValueStringsAre` for element validation |
| `admin_state_up` | `BoolAttribute{Optional, Computed}` | `booldefault.StaticBool(true)`; `UseStateForUnknown` |
| `tags` | `SetAttribute{ElementType: StringType, Optional}` | `Set: schema.HashString` dropped (framework handles internally) |
| `persistence` (MaxItems:1 block) | `ListNestedBlock + listvalidator.SizeAtMost(1)` | **See below** |

## MaxItems:1 decision — persistence block

**Chose `ListNestedBlock + listvalidator.SizeAtMost(1)`** (not `SingleNestedAttribute`).

Per the `references/blocks.md` decision tree:

- **Q1: Are practitioners using block syntax in production?** YES. `openstack_lb_pool_v2` is a long-lived resource; `persistence { ... }` is established practitioner HCL. Converting to `SingleNestedAttribute` changes the syntax to `persistence = { ... }`, a breaking change.
- **Q2: Major-version bump or greenfield?** NO. This is a same-major migration.
- **Result: Output A — keep as block.**

`ListNestedBlock` is chosen over `SingleNestedBlock` because existing SDKv2 state was stored under the list-indexed path `persistence.0.*`. Preserving the list shape avoids backward state compatibility issues without a state upgrader.

## Key pitfalls observed and addressed

- `Set: schema.HashString` on `tags` — dropped; framework handles set uniqueness internally.
- `Default: true` on `admin_state_up` — expressed as `booldefault.StaticBool(true)` (the `defaults` package), NOT in `PlanModifiers`. Attribute marked `Computed` as required by the framework when `Default` is set.
- `ForceNew` on five attributes — converted to `stringplanmodifier.RequiresReplace()` plan modifiers.
- `ExactlyOneOf` on `loadbalancer_id`/`listener_id` — expressed as per-attribute `stringvalidator.ExactlyOneOf`; a `ConfigValidators` alternative using `resourcevalidator.ExactlyOneOf` is documented as the cleaner option.
- `TypeSet` attributes with primitive elements (`alpn_protocols`, `tls_versions`, `tags`) — migrated to `SetAttribute{ElementType: types.StringType}`.

## Files produced

- `migrated_schema.go` — framework schema implementation, valid Go syntax, schema-only.
- `reasoning.md` — full MaxItems:1 decision justification citing `references/blocks.md`.
- `agent_summary.md` — this file.
