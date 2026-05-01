# Refusal — request out of scope for this skill

## Verdict

**Refused / out of scope.** This request is **not covered** by the `provider-sdkv2-to-framework` skill and the migration work is **deferred** to HashiCorp's `terraform-plugin-mux` workflow. No code was generated, nothing in the provider repo was modified, and no `migrated/` directory was produced.

## Why this skill cannot help

The skill's **Pre-flight 0 — Exit if mux** gate fires on this request. The user explicitly asked for:

- a **`terraform-plugin-mux`**-based setup,
- a **staged**, **multi-release** migration,
- shipping some resources on the framework while keeping the rest on SDKv2,
- spread across **two release cycles**.

Every one of those phrasings is a keyword trigger for the gate (`mux`, `terraform-plugin-mux`, `staged`, `multi-release`, `two-release`), and the *semantic* intent — spreading the migration across more than one provider release — independently triggers the same gate.

From `SKILL.md`:

> **Multi-release / muxed / staged / phased migrations** (any `terraform-plugin-mux` workflow): out of scope. **If the user wants the migration spread across more than one provider release — regardless of vocabulary — this skill does not apply.** Refer them to HashiCorp's mux docs and exit. The audit and verification gates here assume a single-server tree; running them against a muxed provider produces false-greens on the SDKv2-routed half.

And from the "What to never do" section:

> **Don't introduce `terraform-plugin-mux`.** *Because* muxing routes some resources to SDKv2 and some to framework simultaneously; this skill's audit and verification gates assume a single-server tree and will produce false-greens on the SDKv2-routed half.

The skill targets HashiCorp's **single-release-cycle** workflow only. Running its audit (`audit_sdkv2.sh`) and verification (`verify_tests.sh`) gates against a muxed provider would inventory only the SDKv2-routed half cleanly and produce misleadingly-green verification on the framework-routed half — exactly the failure mode the gate exists to prevent.

## Where to go instead

Please follow HashiCorp's official `terraform-plugin-mux` documentation and reference implementation:

- **Documentation:** <https://developer.hashicorp.com/terraform/plugin/mux>
- **Combining/translating providers (the staged-migration recipe):** <https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-6-providers>
- **Translating a SDKv2 provider to protocol v6 for muxing:** <https://developer.hashicorp.com/terraform/plugin/mux/translating-protocol-version-6>
- **Source repo:** <https://github.com/hashicorp/terraform-plugin-mux> (Go module: `github.com/hashicorp/terraform-plugin-mux`)

Those docs cover the exact shape you described: a `tf6muxserver` (or `tf5to6server` + `tf6muxserver`) `main.go` that fronts both an SDKv2 server and a framework server, letting you ship a few resources on the framework per release while leaving the rest on SDKv2 across two (or more) release cycles.

## What to do if you change scope

If you later decide to do the migration in a **single release cycle** — i.e. flip the whole provider from SDKv2 to the framework in one go, with no `terraform-plugin-mux` step — re-invoke this skill against `/Users/simon.lynch/git/terraform-provider-openstack` and the Pre-flight 0 gate will pass. At that point the skill will run the audit, generate a checklist, and walk through HashiCorp's 12-step single-release workflow.

## Artefacts produced

- This `agent_summary.md` (refusal explanation).
- **No** `migrated/` directory.
- **No** modifications to `/Users/simon.lynch/git/terraform-provider-openstack`.
