# Migration summary — linode_user

## Pre-flight check

- **Mux check**: No mux/staged migration — single-release path confirmed.
- **Scope**: `resource.go` + `schema_resource.go` (the SDKv2 resource); existing framework datasource files left untouched.
- **Existing framework datasource**: `framework_datasource.go`, `framework_datasource_schema.go`, `framework_models.go` were read for patterns and reused as reference — not modified.

## Key decisions

### MaxItems:1 block — `global_grants`

- SDKv2: `TypeList`, `MaxItems: 1`, `Elem: *schema.Resource`.
- Block-vs-attribute decision (SKILL.md rule 1): practitioners use `global_grants { ... }` block syntax in existing Terraform configs. Switching to `SingleNestedAttribute` would change `foo { }` → `foo = { }` — a breaking HCL change.
- **Decision: `schema.SingleNestedBlock`**. No `SizeAtMost(1)` validator needed for `SingleNestedBlock` (unlike `ListNestedBlock`) because the framework enforces single-instance semantics structurally.
- Model field: `types.Object` (correct mapping for `SingleNestedBlock`).

### Entity grant sets (`domain_grant`, `firewall_grant`, etc.)

- SDKv2: `TypeSet`, no `MaxItems`, `Elem: *schema.Resource`.
- These sets do not use block syntax in HCL (they use assignment syntax), so attribute migration is safe.
- **Decision: `schema.SetNestedAttribute`** with `NestedObject`.
- Model field: `types.Set`.

### Default: false fields

- SDKv2 `Default: false` → `booldefault.StaticBool(false)` from `resource/schema/booldefault`.
- Applied to: `restricted` (top-level) and all 12 boolean fields inside `global_grants`.
- `Default` is **not** placed in `PlanModifiers` — it is a sibling field on the attribute struct.

### Import

- SDKv2: `ImportStatePassthroughContext` (string passthrough on `username`).
- Framework: `helper.BaseResource.ImportState` handles string passthrough when `IDAttr: "username"` and `IDType: types.StringType` are set in `BaseResourceConfig`.

### Resource ID

- SDKv2 used `d.SetId(user.Username)` — the ID is the username string.
- Framework: `IDAttr: "username"`, `IDType: types.StringType` in `BaseResourceConfig`.

## What changed

| File | Change |
|---|---|
| `resource.go` | Full rewrite to framework `resource.Resource`; no SDKv2 imports; CRUD uses typed model; grants expand/flatten use typed structs |
| `schema_resource.go` | Full rewrite; `booldefault.StaticBool(false)` for every `Default: false`; `SingleNestedBlock` for `global_grants`; `SetNestedAttribute` for entity grants |
| `resource_test.go` | `checkUserDestroy` updated to use `acceptance.TestAccFrameworkProvider.Meta.Client` instead of `TestAccSDKv2Provider`; `global_grants.0.*` paths updated to `global_grants.*` (SingleNestedBlock has no index); import step added |

## Verification notes

- No SDKv2 import in any migrated file (`grep terraform-plugin-sdk/v2` finds nothing).
- `Default` appears only as a schema field, never inside `PlanModifiers`.
- `Delete` reads from `req.State`, not `req.Plan`.
- `tfsdk:` struct tags match schema attribute names exactly.
- `flattenResourceGrantEntities` preserves the SDKv2 behavior of filtering entities with empty `Permissions` to avoid false diffs.
