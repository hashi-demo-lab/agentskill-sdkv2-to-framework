# Notes — `openstack_compute_volume_attach_v2` schema migration

## Scope

Schema only. CRUD methods, the import handler, the timeouts plumbing inside
CRUD methods, and tests are NOT migrated. The output is a single
`migrated_schema.go` plus this notes file and `reasoning.md`.

## Source resource summary

`<openstack-clone>/openstack/resource_openstack_compute_volume_attach_v2.go`

- SDKv2 resource: 7 top-level fields plus a single `vendor_options`
  `TypeSet` block (1 inner attribute: `ignore_volume_confirmation`).
- ID format: `<instance_id>/<attachment_id>` (parsed via
  `parsePairedIDs`). Out of scope for this schema-only step but flagged
  here for the eventual full-resource migration.
- Importer: `schema.ImportStatePassthroughContext` — straightforward to
  port to `resource.ImportStatePassthroughID` later.
- Timeouts: Create + Delete (each 10 min default). Schema-side migration
  uses `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})`.

## Key decision points

### 1. `vendor_options` as a true repeating block

The user explicitly directed: "true repeating block (MinItems > 0 or no
MaxItems): the migration should keep block syntax to preserve practitioner
HCL." The literal SDKv2 shape has `MaxItems: 1` (which would normally
suggest `SingleNestedAttribute`), but per the user directive we keep it as a
block. This is also covered by the "backward compat is sacred" branch in
`references/blocks.md`.

### 2. `TypeSet` → `SetNestedBlock`

Not `ListNestedBlock`. Set semantics are preserved; framework computes set
uniqueness internally; the SDKv2 `Set: hashFunc` field has no equivalent and
nothing needs to be carried over (no `Set:` field was present in the source
anyway, but the principle holds).

### 3. `MinItems: 1` + `MaxItems: 1` → `setvalidator.SizeAtLeast(1)` + `setvalidator.SizeAtMost(1)`

Block-level validators on `SetNestedBlock`. There is no `MinItems` /
`MaxItems` field on framework blocks; size constraints are validators.

### 4. `ForceNew` on the block → `setplanmodifier.RequiresReplace()`

Block-level plan modifier. Blocks can carry `Validators` and
`PlanModifiers`, but cannot be `Required` / `Optional` / `Computed`.

### 5. `Default: false` on `ignore_volume_confirmation`

Becomes `Default: booldefault.StaticBool(false)`. The attribute must be
`Computed: true` in addition to `Optional: true` (framework requirement for
attributes with defaults). `Default` is its own attribute field — NOT
something that goes inside `PlanModifiers`.

## Things deliberately not done

- Did not switch `vendor_options` to `SingleNestedAttribute` despite
  `MaxItems: 1` — user directive overrides the normal blocks.md guidance.
- Did not add new validators beyond what SDKv2 had.
- Did not rename any attribute / block — pure refactor from the
  practitioner's POV.
- Did not migrate CRUD methods, tests, or imports.
- Did not run `go build` / `go vet` against the openstack clone (the task
  rules forbid it; the clone is read-only).

## Compile-time caveats

The output file is structured as a stand-alone `package openstack` file
intended for review only — it imports framework packages that the
openstack clone does not yet depend on, so dropping it into the read-only
clone unmodified would not compile. That is expected for a schema-only
migration: the surrounding provider definition (resource registration,
provider's `Resources()` slice, model wiring inside CRUD) needs the rest of
the migration before this file becomes part of a buildable tree.

The `id` attribute is declared explicitly because the framework requires it
on resources (SDKv2 added it implicitly via `d.SetId`).

## What a follow-up step would do

1. Migrate the CRUD methods (Create, Read, Delete) into framework methods
   on `*computeVolumeAttachV2Resource`, reading from `req.Plan` /
   `req.State` into the `computeVolumeAttachV2Model` struct.
2. Implement `Metadata`, `Configure`, and `ImportState` on the resource
   type. The composite `<instance_id>/<attachment_id>` ID may benefit from
   `ResourceWithIdentity` (see `references/identity.md`).
3. Wire the timeouts: read `plan.Timeouts.Create(ctx, 10*time.Minute)` and
   `state.Timeouts.Delete(ctx, 10*time.Minute)` inside the CRUD methods.
4. Migrate the acceptance/unit tests to `ProtoV6ProviderFactories`.
5. Register the new resource type in the provider's `Resources()` slice
   and remove the SDKv2 registration.

## Files produced

- `migrated_schema.go` — the framework schema file.
- `reasoning.md` — full rationale for the decisions, including the
  block-vs-attribute call and the `TypeSet` → `SetNestedBlock` mapping.
- `notes.md` — this file.
