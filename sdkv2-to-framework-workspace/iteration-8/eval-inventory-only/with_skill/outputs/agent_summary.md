# Agent summary — terraform-provider-openstack SDKv2 → Framework inventory

## Files produced

| File | Description |
|------|-------------|
| `audit_report.md` | Verbatim output of `audit_sdkv2.sh` run against `/Users/simon.lynch/git/terraform-provider-openstack` |
| `migration_checklist.md` | Populated checklist covering all 86 resources and 38 data sources, with per-resource audit-flag rows and all 12 HashiCorp single-release-cycle steps |
| `agent_summary.md` | This file |

## Workflow steps run

- **Pre-flight 0 (mux check):** Passed — no `terraform-plugin-mux` import found in `go.mod` or `main.go`. Request did not mention staged/phased/muxed migration. Single-release-cycle workflow confirmed applicable.
- **Pre-flight A (audit):** `bash sdkv2-to-framework/scripts/audit_sdkv2.sh /Users/simon.lynch/git/terraform-provider-openstack` completed successfully using semgrep AST-aware matching. SDKv2 version confirmed as `v2.38.1`.
- **Pre-flight B (plan):** Checklist populated from `assets/checklist_template.md` with one section per resource/data source. No code was modified.

Pre-flight C (per-resource think pass) and steps 1–12 are deferred pending team scope confirmation.

## Scope confirmed

Whole provider inventory produced. **Scope not yet confirmed with team** — per Pre-flight B, the team must decide whether to migrate the full provider or a resource subset before step 1 begins.

## Key audit findings

- **236 files audited**, **86 resources**, **38 data sources** (some names shared between resource and datasource registries)
- **Highest-complexity resource:** `openstack_compute_instance_v2` — 33 ForceNew, MaxItems:1, custom Importer, Timeouts, CustomizeDiff, StateFunc, DiffSuppressFunc, 5 nested Elem blocks, ConflictsWith. Migrate last.
- **Only StateUpgrader in the provider:** `openstack_objectstorage_container_v1` (SchemaVersion present). Must collapse to single-step `ResourceWithUpgradeState` semantics; also read `migrate_resource_openstack_objectstorage_container_v1.go` before editing.
- **101 custom importers** — nearly every resource has one; all flagged for manual review to determine composite-ID vs passthrough.
- **68 resources with Timeouts** — all require migration to `terraform-plugin-framework-timeouts` package.
- **110 ConflictsWith/ExactlyOneOf/AtLeastOneOf/RequiredWith** occurrences — route to `terraform-plugin-framework-validators`.
- **11 MaxItems:1 + nested Elem** blocks — each requires the block-vs-nested-attribute decision before editing.
- **No `MigrateState` (legacy v1.x)** found — clean baseline for state upgrade path.
- **No mux** — straightforward single-server migration path.

## Recommended migration order

1. Migrate simple resources with only **CI** flag first (e.g., `openstack_networking_rbac_policy_v2`, `openstack_networking_segment_v2`, `openstack_workflow_cron_trigger_v2`) to validate the framework wiring and test harness.
2. Work through **TO-only** resources next.
3. Address **NE** (block decision) resources once the team has established a house rule on MaxItems:1 block vs attribute.
4. Tackle **CD/SF/DS** resources (CustomizeDiff, StateFunc, DiffSuppressFunc) after the simpler patterns are proven.
5. Migrate `openstack_objectstorage_container_v1` (StateUpgrader) as a dedicated PR for careful review.
6. Migrate `openstack_compute_instance_v2` last — most complex, highest risk.
