# Agent summary — terraform-provider-openstack SDKv2 → Framework inventory

## Files produced

- `audit_report.md` — verbatim output of `scripts/audit_sdkv2.sh` against `<openstack-clone>`
- `migration_checklist.md` — `assets/checklist_template.md` populated from the audit, with per-resource rows for all 109 resources and 64 data sources
- `agent_summary.md` — this file

## Workflow steps run

1. Read `SKILL.md` to confirm workflow and scope rules.
2. Confirmed SDKv2 dependency: `go.mod` imports `github.com/hashicorp/terraform-plugin-sdk/v2` v2.38.1.
3. Ran Pre-flight A (audit): `bash scripts/audit_sdkv2.sh <openstack-clone>` — completed successfully using semgrep AST-aware matching over 236 files.
4. Ran Pre-flight B (plan): populated `assets/checklist_template.md` from audit findings; per-resource rows cover all 109 resource files and 64 data source files.
5. No code in `<openstack-clone>` was modified.

## Scope confirmed

Whole provider — 109 resources, 64 data sources (173 files total). Scope to be confirmed with the team before any editing begins.

## Key findings

- **Highest-complexity file**: `resource_openstack_compute_instance_v2.go` (33 ForceNew, 5 nested Elem, 1 MaxItems:1, 1 CustomizeDiff, 1 StateFunc, 1 DiffSuppressFunc, 1 Importer, 1 Timeout) — recommend migrating this last.
- **Only StateUpgrader in the provider**: `resource_openstack_objectstorage_container_v1.go` (SchemaVersion + StateUpgraders) — read `references/state-upgrade.md` before touching this file.
- **101 custom Importers** across the provider — most will need composite-ID parsing review (see `references/import.md` and `references/identity.md`).
- **637 ForceNew occurrences** — each becomes `PlanModifiers: []planmodifier.<Type>{<type>planmodifier.RequiresReplace()}`.
- **88 Default values** — these go into the `defaults` package, NOT into `PlanModifiers` (common compile-time error).
- **Recommended migration order**: start with simple resources (no Importer, no Timeouts, no nested blocks) such as `resource_openstack_identity_role_v3`, `resource_openstack_networking_segment_v2`, and `resource_openstack_dns_zone_share_v2`; save `resource_openstack_compute_instance_v2` for last.
- **Protocol version**: default to v6 for this single-release migration.
