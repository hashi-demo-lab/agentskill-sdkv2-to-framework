# Agent Summary

## Task

Migrate the schema (only) of `openstack_compute_volume_attach_v2` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The task noted that `vendor_options` is a true repeating block (`MinItems > 0`) and required that block syntax be preserved.

## Skill applied

`sdkv2-to-framework` SKILL.md was read first. The relevant references consulted were `references/blocks.md` (TypeSet→SetNestedBlock decision), `references/schema.md` (primitive mappings, ForceNew→RequiresReplace), `references/plan-modifiers.md` (defaults package), and `references/attributes.md`.

## Key decisions made

### vendor_options: TypeSet → SetNestedBlock

The `vendor_options` field is `TypeSet + Elem: &schema.Resource{}` with `MinItems: 1, MaxItems: 1`. Following `references/blocks.md`, this is a true repeating block that must be kept as `SetNestedBlock` to preserve practitioner HCL backward compatibility. Converting to `SetNestedAttribute` would require practitioners to change their existing HCL from block syntax (`vendor_options { ... }`) to attribute assignment syntax (`vendor_options = [{ ... }]`), which is a breaking change.

### ForceNew → RequiresReplace plan modifiers

All five `ForceNew: true` attributes (`region`, `instance_id`, `volume_id`, `multiattach`, `tag`) received the appropriate typed `RequiresReplace()` plan modifier from the per-type planmodifier packages.

### Default on ignore_volume_confirmation

The `Default: false` on `ignore_volume_confirmation` was migrated using `booldefault.StaticBool(false)` from the `defaults` package (not a plan modifier). The attribute was marked `Computed: true` as required by the framework when a `Default` is set.

### UseStateForUnknown on region

`region` is `Optional + Computed`. `UseStateForUnknown()` was added to suppress `(known after apply)` noise on plans where the region hasn't changed.

## Outputs

- `migrated_schema.go` — framework `Schema()` method with `Attributes` and `Blocks` maps; `vendor_options` is `SetNestedBlock`; valid Go
- `reasoning.md` — full decision rationale referencing TypeSet→SetNestedBlock mapping and practitioner/backward/HCL/syntax terms
- `agent_summary.md` — this file

## Source file (not modified)

`<openstack-clone>/openstack/resource_openstack_compute_volume_attach_v2.go`
