# Refusal — Muxed / Multi-Release Migration Request

## Decision: Cannot Apply — Out of Scope

The sdkv2-to-framework skill **does not apply** to this request and cannot be used to assist with it.

## Why This Request Is Out of Scope

Your request explicitly asks to:

- Set up **terraform-plugin-mux** to run a muxed provider
- Ship resources on the framework **while keeping the rest on SDKv2** simultaneously
- Spread the migration across **two release cycles** (a multi-release, staged migration)

The skill's **Pre-flight 0 — Exit if mux** check fires immediately on all three signals:

1. **Keyword match**: the request contains `terraform-plugin-mux`, `staged`, `multi-release`, and `two release cycles` — every one of these is an explicit stop keyword.
2. **Semantic match**: even setting the keywords aside, the intent is unambiguous — resources served by SDKv2 and resources served by the framework simultaneously, coordinated across more than one provider release. That is a multi-release / phased migration by definition.

Per the skill's "Does NOT apply" section:

> **Multi-release / muxed / staged / phased migrations** (any `terraform-plugin-mux` workflow): out of scope. **If the user wants the migration spread across more than one provider release — regardless of vocabulary — this skill does not apply.** Refer them to HashiCorp's mux docs and exit. The audit and verification gates here assume a single-server tree; running them against a muxed provider produces false-greens on the SDKv2-routed half.

## What This Skill Covers Instead

This skill targets the **single-release-cycle workflow**: all resources move from SDKv2 to the framework in one release, with no interim muxed serving period. The full 12-step workflow (audit → plan → think pass → migrate → verify → release) applies only to that path.

## What You Should Do Instead

For a staged, multi-release migration using terraform-plugin-mux, defer to HashiCorp's official documentation:

- **terraform-plugin-mux documentation**: [https://developer.hashicorp.com/terraform/plugin/mux](https://developer.hashicorp.com/terraform/plugin/mux)
- The mux library itself: `github.com/hashicorp/terraform-plugin-mux`
- HashiCorp's guide on combining protocol version servers: [https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-5-providers](https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-5-providers)

These resources cover exactly the terraform-plugin-mux setup for a multi-release staged rollout that you are describing.

## What Was Not Done

- No audit of `<openstack-clone>` was performed.
- No migration plan, checklist, or migrated files were produced.
- The provider repository was not modified in any way.
- No `migrated/` output files were created.
