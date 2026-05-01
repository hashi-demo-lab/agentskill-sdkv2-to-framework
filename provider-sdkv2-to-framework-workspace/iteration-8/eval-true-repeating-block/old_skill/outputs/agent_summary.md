# Agent Summary: openstack_compute_volume_attach_v2 schema migration

## Task

Migrate the schema (not CRUD methods, not tests) of `openstack_compute_volume_attach_v2` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The resource has a `TypeSet` block (`vendor_options`) with `MinItems:1, MaxItems:1` — this is a true repeating block and must preserve block HCL syntax.

## Key decision: vendor_options → SetNestedBlock

`vendor_options` in SDKv2 is `TypeSet` + `Elem: &schema.Resource{}` — a nested block with set semantics. The migration converts it to `schema.SetNestedBlock` (not `SetNestedAttribute`) because:

- Practitioners write `vendor_options { ignore_volume_confirmation = false }` using block syntax.
- Converting to `SetNestedAttribute` would change HCL to `vendor_options = [{ ... }]` — a breaking change for all existing configurations.
- This provider is mature (not greenfield, not a major-version bump), so backward-compat is required.

`MinItems:1` and `MaxItems:1` are re-expressed as `setvalidator.SizeBetween(1, 1)` since the framework has no `MinItems`/`MaxItems` fields on blocks.

## Other schema changes

- All `ForceNew: true` primitives → `RequiresReplace()` plan modifier.
- `region` (Optional+Computed, ForceNew) → `RequiresReplace()` + `UseStateForUnknown()`.
- `device` (Optional+Computed, no ForceNew) → `StringAttribute{Optional: true, Computed: true}`.
- `vendor_options.ignore_volume_confirmation` `Default: false` → `booldefault.StaticBool(false)` with `Computed: true` (required by framework when Default is set).

## Outputs

- `migrated_schema.go` — valid Go, framework `Schema()` method. `vendor_options` is `SetNestedBlock` inside the `Blocks` map (not `Attributes`). All imports are explicit.
- `reasoning.md` — documents the TypeSet→SetNestedBlock mapping, HCL/practitioner justification, MinItems/MaxItems validator replacement, and Default/Computed handling.

## Items deferred to CRUD migration

- **ForceNew on vendor_options block**: blocks cannot carry `RequiresReplace`; this must be handled in `ModifyPlan` when CRUD methods are written.
- **Timeouts**: `terraform-plugin-framework-timeouts` helper requires coordination with CRUD methods; omitted from schema-only output.
- **Import**: `ImportStatePassthroughContext` → `ResourceWithImportState.ImportState`; out of scope.
