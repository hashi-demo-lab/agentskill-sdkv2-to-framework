# Agent summary — terraform-provider-openstack SDKv2 → Framework inventory

## What was done

Pre-flight 0 confirmed: no mux imports, no staged/multi-release intent — single-release-cycle path applies.

Pre-flight A: ran `audit_sdkv2.sh` against `/Users/simon.lynch/git/terraform-provider-openstack`. The provider imports `github.com/hashicorp/terraform-plugin-sdk/v2 v2.38.1`.

Pre-flight B: populated `migration_checklist.md` from the audit. Scope is whole-provider (174 resource/data-source constructors across 236 production Go files and 300 test files). No code was changed.

## Key inventory numbers

| Metric | Count |
|---|---|
| Resource constructors to migrate | 174 |
| Production Go files audited | 236 |
| Test files audited | 300 |
| ForceNew occurrences | 637 |
| ValidateFunc/ValidateDiagFunc | 140 |
| ConflictsWith/ExactlyOneOf/etc. | 110 |
| retry.StateChangeConf usages | 121 |
| Custom Importer (total) | 101 |
| — of which trivial passthrough | 87 |
| — of which non-trivial | 14 |
| Timeouts fields | 68 |
| MaxItems:1 + nested Elem (block decision) | 11 |
| DiffSuppressFunc | 8 |
| CustomizeDiff | 3 |
| StateFunc | 4 |
| StateUpgraders/SchemaVersion | 1 |
| Sensitive attributes | 20 |

## Highest-risk files

1. `openstack/resource_openstack_compute_instance_v2.go` (complexity score 162.75) — hits 9 distinct judgment-rich patterns simultaneously: CustomizeDiff, StateFunc, DiffSuppressFunc, MaxItems:1, custom Importer, Timeouts, 13× retry.StateChangeConf, ConflictsWith, nested blocks. Migrate last; requires Pre-flight C think pass.
2. `openstack/resource_openstack_networking_port_v2.go` (score 61.75) — MaxItems:1, StateFunc, DiffSuppressFunc, JSON normalisation helper, composite importer.
3. `openstack/resource_openstack_blockstorage_volume_v3.go` (score 61.25) — 5× retry.StateChangeConf, Timeouts, nested blocks, ConflictsWith.

## Only state upgrader

`openstack/resource_openstack_objectstorage_container_v1.go` (SchemaVersion=1, 1 upgrader). Migration code lives in `openstack/migrate_resource_openstack_objectstorage_container_v1.go`. Framework upgrader must be single-step — do not chain.

## Recommended migration order

1. `openstack/provider.go` + `openstack/provider_test.go` (shared test infra and provider factory — must land first)
2. Simple resources with only `ImportStatePassthroughContext` and no judgment-rich patterns (bulk of the 87 trivial importers)
3. Resources with Timeouts + retry.StateChangeConf but no MaxItems:1 / StateFunc / CustomizeDiff
4. Resources with non-trivial importers and nested blocks
5. `openstack_objectstorage_container_v1` (state upgrader)
6. `openstack_compute_instance_v2` last

## Output files

- `audit_report.md` — verbatim script output (36 KB, well under 100 KB cap)
- `migration_checklist.md` — populated checklist; 61 resource sections + 64 data-source sections; all 12 HashiCorp steps included verbatim; step 7 includes red-test-before-code TDD gate language; step 2 includes data-consistency review note
- `agent_summary.md` — this file
