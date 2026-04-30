# Migration summary: terraform-provider-openstack/v3 → plugin-framework

**Date:** 2026-04-30
**SDKv2 version:** v2.38.1
**Scope:** Full provider (no mux — single-release-cycle path)

---

## Scale

| Metric | Count |
|---|---|
| Go source files (excl. tests/vendor) | 236 |
| Resources | 109 |
| Data sources | 64 |
| Files needing manual review | ~127 |
| Estimated complexity-weeks (rough) | 16–24 |

---

## Key findings

**This is a large, complex migration.** The provider has 173 resources + data sources and 236 non-test source files. The mechanical work (schema conversion, CRUD rewrites) is large but tractable. The high-risk patterns are:

1. **`compute_instance_v2` is the hardest single file** — 33 ForceNew, 5 nested blocks, `CustomizeDiff`, `StateFunc`, and `DiffSuppressFunc` all present simultaneously. Allocate at least 2–3 days for this resource alone.

2. **71 nested `Elem: &schema.Resource{}` blocks** require individual block-vs-nested-attribute decisions. Default recommendation for all mature resources: keep as `ListNestedBlock` (backward-compatible). Only reconsider if this release is also bumping a major version.

3. **101 resources have importers**, all using `ImportStatePassthroughContext` (87) or custom `StateContext` functions (14). The 14 custom ones need the importer function read and reproduced as `ResourceWithImportState.ImportState`.

4. **Only one `StateUpgraders`** (`objectstorage_container_v1`, V0->V1). This is isolated and well-contained — but the single-step semantics in the framework differ from SDKv2 chaining. Read `references/state-upgrade.md` before touching it.

5. **3 `CustomizeDiff` resources** become `resource.ResourceWithModifyPlan`. This is a meaningful API change but well-documented in the framework.

6. **4 `StateFunc` usages** (JSON normalization, CIDR normalization) become custom types implementing `basetypes.StringValuable` or PlanModifiers.

7. **8 `DiffSuppressFunc` usages** — each requires intent analysis before choosing the framework equivalent (PlanModifier, custom type, or validator).

8. **68 resources have `Timeouts`** — all require the `terraform-plugin-framework-timeouts` package. This is mechanical but pervasive.

---

## Recommended approach

1. **Do not start editing until the baseline is green.** Run `go test ./...` first; fix any pre-existing failures so you have a clean bisect signal.

2. **Migrate in service-area batches**, not alphabetically. Establish the pattern on a simple service (e.g., firewall or workflow), then work up to networking and compute.

3. **Use Tier ordering**: Tier 1 (simple ForceNew-only) -> Tier 2 (Timeouts + nested blocks) -> Tier 3 (CustomizeDiff/StateFunc) -> Tier 4 (state upgrader).

4. **TDD gate is non-negotiable** (workflow step 7): update tests first, run them red, then migrate. This is especially critical for the complex Tier 3 resources where test quality determines whether the migration is verifiable.

5. **Run `verify_tests.sh --migrated-files <file>` after every resource** — the `--migrated-files` flag enables the negative gate (confirms the migrated file no longer imports SDKv2).

---

## Outputs

| File | Description |
|---|---|
| `audit_report.md` | Full semgrep-driven audit: per-file complexity table, pattern inventory by service area, special-case pattern analysis |
| `migration_checklist.md` | Phased migration checklist: pre-flight, 12 HashiCorp steps, per-resource task lists for all 109 resources and 64 data sources |
| `summary.md` | This file |

---

## Before starting: confirm with the team

- [ ] Scope confirmed: whole provider in one release? Or service-area batches across releases?
- [ ] Protocol version confirmed: v6 (default, recommended)
- [ ] OpenStack test environment available for `TF_ACC=1` runs?
- [ ] semgrep installed? (`brew install semgrep` or `pip install semgrep`)
- [ ] Team has read `audit_report.md` "Files requiring manual review" section?
