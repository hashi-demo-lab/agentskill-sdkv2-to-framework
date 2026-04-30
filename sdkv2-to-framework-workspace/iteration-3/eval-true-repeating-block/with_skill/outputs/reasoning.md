# Schema migration reasoning — openstack_compute_volume_attach_v2

## Source analysis

The SDKv2 schema has seven top-level keys:

| Field | SDKv2 type | ForceNew | Special |
|---|---|---|---|
| `region` | TypeString | yes | Optional+Computed |
| `instance_id` | TypeString | yes | Required |
| `volume_id` | TypeString | yes | Required |
| `device` | TypeString | no | Optional+Computed |
| `multiattach` | TypeBool | yes | Optional |
| `tag` | TypeString | yes | Optional |
| `vendor_options` | TypeSet of Resource | yes | MinItems:1, MaxItems:1 |

The `vendor_options` block contains a single nested field:

| Field | SDKv2 type | Default |
|---|---|---|
| `ignore_volume_confirmation` | TypeBool | false |

## Block-vs-attribute decision for `vendor_options`

The SDKv2 declaration is `TypeSet` + `Elem: &schema.Resource{...}` + `MinItems: 1` + `MaxItems: 1`.

Applying the skill decision tree from `blocks.md`:

```
Was this a TypeList/TypeSet of &schema.Resource{}? → Yes
├── MaxItems: 1 only (no MinItems) → SingleNestedAttribute candidate
└── MinItems > 0 (true repeating block) → keep as block
```

`MinItems: 1` makes this a **true repeating block** — even though `MaxItems: 1` means at most one instance can appear, the `MinItems: 1` constraint means the block is required to be present rather than optional. Practitioners write:

```hcl
vendor_options {
  ignore_volume_confirmation = true
}
```

Converting this to `SingleNestedAttribute` would change HCL syntax to `vendor_options = { ... }` — a breaking change for existing configurations. The correct migration is therefore `SetNestedBlock` (matching the SDKv2 `TypeSet`) with both `SizeAtLeast(1)` and `SizeAtMost(1)` validators to preserve the MinItems/MaxItems constraints.

## Per-attribute decisions

### `ForceNew: true` → `RequiresReplace` plan modifier

SDKv2's `ForceNew: true` is not a direct attribute in the framework. Each affected attribute receives:

```go
PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}
```

(or `boolplanmodifier.RequiresReplace()` for booleans). Affected fields: `region`, `instance_id`, `volume_id`, `multiattach`, `tag`.

### `region` — Optional+Computed+ForceNew

Gets both `RequiresReplace()` and `UseStateForUnknown()` plan modifiers, preserving the SDKv2 Computed behaviour (value is filled in from the provider if not set by the user).

### `device` — Optional+Computed, no ForceNew

No plan modifier for replacement. `UseStateForUnknown()` is not added because the read function actively refreshes this value from the API.

### `id` — Computed only

Standard framework pattern: declare an `id` attribute with `UseStateForUnknown()`.

### `Default: false` on `ignore_volume_confirmation`

In the framework, `Default` is not a plan modifier — it lives in the `defaults` package. The equivalent is:

```go
Default: booldefault.StaticBool(false),
```

The field is also marked `Computed: true` because a non-nil Default requires Computed to be set (the framework will write the default value into state).

### Timeouts

`schema.ResourceTimeout` migrates to the `terraform-plugin-framework-timeouts` package:

```go
"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
    Create: true, CreateDefault: 10 * time.Minute,
    Delete: true, DeleteDefault: 10 * time.Minute,
}),
```

### `vendor_options` ForceNew

The SDKv2 `vendor_options` block has `ForceNew: true`. In the framework, blocks cannot carry `RequiresReplace` directly. This plan modifier must be applied at the CRUD layer (in the `ModifyPlan` method or via `resource.RequiresReplace` on the block path). It is noted here as a CRUD-layer concern, outside the scope of this schema-only migration.

## What is NOT in this file

- CRUD methods (`Create`, `Read`, `Update`, `Delete`)
- `ImportState` implementation
- Provider registration
- Tests

These are explicitly out of scope per the task constraints.
