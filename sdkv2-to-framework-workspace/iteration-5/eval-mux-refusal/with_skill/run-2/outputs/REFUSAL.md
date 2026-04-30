# Refusal: out-of-scope request (terraform-plugin-mux multi-release migration)

## Decision
**Refused.** This skill (`sdkv2-to-framework`) does not cover this request. No code, no audit, no checklist was produced against the openstack provider clone.

## Why this request is out of scope

The user's prompt explicitly asks for:

- *"using terraform-plugin-mux"*
- *"a staged, multi-release migration"*
- *"set up the muxed provider"*
- *"across two release cycles"*

Every one of those phrases is a refusal trigger called out verbatim in `SKILL.md`:

> **Before triggering, sanity-check the user's wording.** If the request mentions any of *mux*, *muxed*, *muxing*, *staged migration*, *two-release*, *multi-release*, or *terraform-plugin-mux* — **stop**. That's the multiplexed-migration path which is explicitly out of scope (see "Does NOT apply" below). Refer the user to HashiCorp's mux docs and exit instead of applying the single-release workflow.

And from the `Does NOT apply` list:

> - **`terraform-plugin-mux` multi-release migrations**: out of scope for this skill; the user wants the single-release path.

And from `What to never do`:

> - **Don't introduce `terraform-plugin-mux`.** *Because* muxing changes the migration shape entirely (incremental over many releases vs the single-release scope this skill targets); applying single-release patterns to a muxed provider creates an inconsistent half-state.

This skill targets HashiCorp's *single-release-cycle* migration workflow (12 steps, audit-first, TDD gate at step 7, layered verification). The mux workflow is a structurally different process: it ships a provider that serves both an SDKv2 server and a framework server via `tf6muxserver` (or `tf5muxserver`) for one or more releases, migrating resources incrementally and removing the mux server only after every resource has moved. Applying the single-release patterns this skill encodes (e.g., a single `main.go` swap to `providerserver.NewProtocol6WithError`, a one-shot resource sweep, a single round of test-suite migration) to a muxed provider produces an inconsistent half-state.

## What the user should do instead

The authoritative reference for the muxed/staged path is HashiCorp's own docs — this skill is not a substitute for them:

- **Mux overview & combining providers**: https://developer.hashicorp.com/terraform/plugin/mux
- **Combining SDKv2 + framework providers** (the specific recipe for this user's situation): https://developer.hashicorp.com/terraform/plugin/framework/migrating/mux
- **Migration workflow — multi-release-cycle option**: https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-over-multiple-release-cycles
- **`terraform-plugin-mux` Go module**: https://github.com/hashicorp/terraform-plugin-mux (`tf6muxserver` is the current default; `tf5muxserver` exists for protocol v5)

Per-resource conversion mechanics (schema, CRUD, validators, plan modifiers, state upgraders, import, identity, timeouts, sensitive/write-only, testing helpers) are essentially the same regardless of whether the migration is single-release or muxed — once the user is *inside* the framework half of a muxed provider, the same per-element references in this skill (`references/schema.md`, `references/resources.md`, etc.) describe what each individual resource's framework rewrite looks like. What this skill cannot do is set up the mux server, decide protocol version compatibility between the two halves, partition the provider's resource map across the two servers, or sequence the multi-release rollout. Those are precisely the things the user is asking for, and they belong to the HashiCorp mux docs.

## What was NOT done (and won't be, under this skill)

- No edits to `/Users/simon.lynch/git/terraform-provider-openstack` (read-only was honoured; the directory was not touched at all).
- `scripts/audit_sdkv2.sh` was **not** run. An audit would have been appropriate for a single-release migration; running it here would mislead the user into thinking the skill was engaging with their actual request.
- No `migration_checklist.md`, no `audit_report.md`, no migrated `*.go` files.
- No `main.go` swap to `providerserver.NewProtocol6WithError` — that's the single-release shape, and applying it to a muxed-provider plan would actively damage the user's setup.

## If the user wants the *single-release* path instead

If the user reconsiders and wants to do the whole openstack provider migration in one release (no mux, no staging), come back with that wording and this skill will run the standard pre-flight: audit script, checklist scaffold, scope confirmation, then the 12-step workflow. The trigger phrasing to use would be something like *"migrate my Terraform provider off SDKv2 to the plugin framework"* — without the mux/staged/multi-release qualifiers.
