# Agent Summary — openstack_lb_pool_v2 schema migration

## Task

Migrate the schema (not CRUD, not tests) of `openstack_lb_pool_v2` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`, with specific focus
on the `MaxItems:1` block decision.

## Skill workflow followed

1. Read `SKILL.md` to ingest the workflow and the `MaxItems_1_decision` rule.
2. Read `references/blocks.md` for the full decision tree.
3. Read `references/schema.md`, `references/attributes.md`,
   `references/validators.md`, and `references/plan-modifiers.md` for
   mechanical attribute translations.
4. Read the source file
   `openstack/resource_openstack_lb_pool_v2.go` in full.
5. Applied the decision rule before editing, writing a 3-line pre-edit summary
   (block decision, state upgrade, import shape) mentally as instructed by
   SKILL.md "Think before editing".
6. Produced `migrated_schema.go`, `reasoning.md`, and this summary.

## Key findings

### MaxItems:1 — persistence block → ListNestedBlock (backward-compat)

The `persistence` block is a mature, documented feature used in production
OpenStack provider configurations with block syntax
(`persistence { type = "HTTP_COOKIE" ... }`). The provider is not undergoing a
major-version bump. Converting to `SingleNestedAttribute` would change the HCL
syntax (removing the block form), which is a breaking change for practitioners.

Decision: **ListNestedBlock + `listvalidator.SizeAtMost(1)`** (Output A from
SKILL.md / blocks.md backward-compat path). This also preserves the
`persistence.0.*` state path that existing state uses, avoiding the need for a
state-migration step.

`SingleNestedBlock` was considered but rejected: the existing CRUD code
serialises `persistence` as a list (`flattenLBPoolPersistenceV2` returns a
slice), so keeping the list-shaped state path avoids a state version bump.

### Other notable translations

- `ForceNew: true` on `region`, `tenant_id`, `protocol`, `loadbalancer_id`,
  `listener_id` → `stringplanmodifier.RequiresReplace()` in each attribute's
  `PlanModifiers` slice.
- `Default: true` on `admin_state_up` → `booldefault.StaticBool(true)` (the
  `defaults` package) plus `Computed: true` (required for framework defaults).
- `Set: schema.HashString` on `tags` → dropped; `SetAttribute` handles
  uniqueness natively.
- `TypeSet` of primitive strings (`alpn_protocols`, `tls_versions`, `tags`) →
  `SetAttribute{ElementType: types.StringType}`. Element `ValidateFunc` becomes
  `setvalidator.ValueStringsAre(stringvalidator.OneOf(...))`.
- `ExactlyOneOf` on `loadbalancer_id` / `listener_id` → per-attribute
  `stringvalidator.ExactlyOneOf(path.MatchRoot(...), path.MatchRoot(...))`.
- `Computed: true` on `tls_ciphers` / `alpn_protocols` / `tls_versions` →
  retained; added `UseStateForUnknown()` on `tls_ciphers` to suppress
  `(known after apply)` churn.

## Output files

- `migrated_schema.go` — valid Go, framework schema only, no SDKv2 imports.
- `reasoning.md` — full decision rationale with citations to `blocks.md` and
  `SKILL.md`.
- `agent_summary.md` — this file.

## Scope not covered (per task instructions)

- CRUD methods (`Create`, `Read`, `Update`, `Delete`, `Import`)
- Acceptance and unit tests
- Provider wiring (`Resources()`, `Metadata()`, mux or no-mux decision)
- State upgrader (none needed — `SchemaVersion` is 0 in the SDKv2 resource)
- Timeouts (would use `terraform-plugin-framework-timeouts` package)
