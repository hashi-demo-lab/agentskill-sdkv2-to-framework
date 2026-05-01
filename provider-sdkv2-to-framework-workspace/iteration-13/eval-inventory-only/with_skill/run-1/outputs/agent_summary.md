# Agent summary

Inventory-only run for terraform-provider-openstack (SDKv2 v2.38.1 → terraform-plugin-framework). No changes to the clone.

**Pre-flight 0:** keyword + semantic mux check passed (user prompt has none of `mux/staged/phased/multi-release`, asks for a single inventory pass; `go.mod` has no `terraform-plugin-mux`).

**Pre-flight A:** `audit_sdkv2.sh` ran clean (semgrep installed). Output saved verbatim to `audit_report.md`. The audit's per-file complexity table was capped (`--max-files 10`) but the manual-review section remains complete because every flagged file is real audit signal — the skill explicitly forbids hand-trimming. The provider is large (236 production Go files, 174 resource/data-source constructors), so the report exceeds the 25KB advisory cap; this is genuine signal, not over-firing.

**Headline findings:**
- 110 resources + 63 data sources registered (`provider.go`).
- **Step-2 data-consistency cleanups (do in SDKv2 first):** 88 `Default`-without-`Computed`, 5 `ForceNew`+`Computed` (3 containerinfra resources), 3 malformed `MaxItems:1`-without-`Elem`, 577 Optional+Computed-without-`UseStateForUnknown`. The provider has `Default` without `Computed` in `provider.go` itself.
- **Single state upgrader:** `openstack_objectstorage_container_v1` (`SchemaVersion: 1`).
- **Largest mechanical work:** 121 `retry.StateChangeConf` + 33 `retry.RetryContext` calls — no framework equivalent; need a shared waiter helper.
- **34 EnvDefaultFunc** provider-config calls to port to framework defaults.
- **Highest-risk file:** `resource_openstack_compute_instance_v2.go` (audit score 199.75 — combines MaxItems:1 + custom Importer + Timeouts + CustomizeDiff + StateFunc + DiffSuppressFunc + customdiff + retry.StateChangeConf). Migrate this last in Wave 4.
- **11 `MaxItems: 1`** sites — default decision is "keep as block" per skill rule; revisit on a major-version bump.
- **Test-side prerequisite:** `openstack/provider_test.go` is the single shared infra file (526 `ProviderFactories`, `resource.Test`, `PreCheck` references all flow through it). Wire `testAccProtoV6ProviderFactories` here *before* per-resource test rewrites.

**Recommended scope decision for the team meeting:** whole-provider, single-release, protocol v6, with explicit Wave 1–4 ordering documented in the checklist (data sources → simple resources → state-change resources → top-tier complexity).

Outputs:
- `audit_report.md` — verbatim audit output
- `migration_checklist.md` — populated from `assets/checklist_template.md`, with all 12 HashiCorp steps verbatim, full per-resource and per-data-source tables, and per-resource think-pass guidance for Tier-A files
- `agent_summary.md` — this file
