# linode_user migration — SDKv2 → terraform-plugin-framework

## Decisions

- **Single-release-cycle migration**: no mux, no staged rollout. Skill applies.
- **Block vs attribute** for `global_grants` (`MaxItems: 1`):
  - Existing acceptance tests reference state paths like
    `global_grants.0.account_access`, and `tmpl/grants.gotf` writes the
    block syntax `global_grants { ... }`. Practitioner HCL is using the
    block shape.
  - Per `references/blocks.md` Q1: keep as a **`ListNestedBlock` with
    `listvalidator.SizeAtMost(1)`**. Switching to `SingleNestedAttribute`
    would change `global_grants.0.X` → `global_grants.X` in state and
    require `global_grants = { ... }` in HCL — breaking change.
- **Per-entity grants** (`*_grant`): `TypeSet` of `*schema.Resource` with
  no `MaxItems` → true repeating blocks. Modelled as `SetNestedBlock` to
  preserve HCL syntax.
- **`Default: false`** on every bool: translated to
  `booldefault.StaticBool(false)` on Optional+Computed `BoolAttribute`s.
  Defaults are wired through the `Default:` field, NOT inside
  `PlanModifiers` (the typed-defaults / typed-planmodifier mismatch is a
  compile-time guardrail per the skill's pitfall list).
- **`ForceNew: true`** on `email`: translated to
  `stringplanmodifier.RequiresReplace()` in `PlanModifiers`. There is no
  `RequiresReplace` field on framework attributes.
- **ID type**: `linode_user` IDs are usernames, so `IDType: types.StringType`
  in the `BaseResourceConfig`, and `ImportState` uses
  `resource.ImportStatePassthroughID(path.Root("id"), ...)`.
- **`req.ProviderData == nil` guard**: handled by inheriting
  `helper.BaseResource.Configure`, which already implements the guard.
- **`req.State` not `req.Plan` in Delete**: explicit, with comment.

## Files emitted

- `migrated/resource.go` — Linode-style resource (`NewResource`, embeds
  `helper.BaseResource`; CRUD + ImportState; helper functions for
  flatten/expand of grants and global grants).
- `migrated/schema_resource.go` — schema using
  `terraform-plugin-framework/resource/schema` with `Attributes` and
  `Blocks`, all `Default:` values using `booldefault.StaticBool(false)`
  in the per-type defaults package.
- `migrated/resource_test.go` — unchanged from upstream's already-converted
  layout: `ProtoV6ProviderFactories` only, no SDKv2 `ProviderFactories`
  field.

## Defaults audit

Top-level `restricted` (Default: false) plus 12 nested-bool defaults
inside `global_grants` (`add_domains`, `add_databases`, `add_firewalls`,
`add_images`, `add_linodes`, `add_longview`, `add_nodebalancers`,
`add_stackscripts`, `add_volumes`, `add_vpcs`, `cancel_account`,
`longview_subscription`) — 13 booldefault.StaticBool(false) call-sites
total. Each is on an attribute marked `Optional: true, Computed: true`,
which is a hard prerequisite for `Default` to apply.

## Notable runtime behaviour

`refresh()` only fetches grants from upstream when the practitioner
configured at least one grants attribute. This is necessary because
blocks in the framework cannot be Optional+Computed at the block level
(SDKv2's schema relied on that), so we mimic "populate when set" by
leaving null blocks alone. Without this, restricted users with no grants
block in HCL would surface "Provider produced inconsistent result after
apply" because the plan said null but Read would have written a value.
