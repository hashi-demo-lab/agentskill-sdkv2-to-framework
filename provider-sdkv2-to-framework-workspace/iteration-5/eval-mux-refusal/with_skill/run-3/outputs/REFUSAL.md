# Refusal: terraform-plugin-mux multi-release migration is out of scope

## What you asked for

> "Migrate my Terraform provider at <openstack-clone> off SDKv2 using terraform-plugin-mux for a staged, multi-release migration — please help me set up the muxed provider so I can ship a few resources on the framework while keeping the rest on SDKv2 across two release cycles."

## Why I'm declining (with this skill)

The `sdkv2-to-framework` skill explicitly does **not** cover `terraform-plugin-mux` workflows. From `SKILL.md`:

- **"When this skill applies"** instructs: *"If the request mentions any of mux, muxed, muxing, staged migration, two-release, multi-release, or terraform-plugin-mux — stop. That's the multiplexed-migration path which is explicitly out of scope. Refer the user to HashiCorp's mux docs and exit instead of applying the single-release workflow."*
- **"Does NOT apply"** lists: *"`terraform-plugin-mux` multi-release migrations: out of scope for this skill; the user wants the single-release path."*
- **"What to never do"** lists: *"Don't introduce `terraform-plugin-mux`. Because muxing changes the migration shape entirely (incremental over many releases vs the single-release scope this skill targets); applying single-release patterns to a muxed provider creates an inconsistent half-state."*

Your request hits every refusal trigger: "terraform-plugin-mux", "staged, multi-release migration", "muxed provider", "two release cycles", "shipping a few resources on the framework while keeping the rest on SDKv2".

This skill drives a **single-release-cycle**, audit-first, TDD-gated workflow that migrates the entire provider in one cut. The artefacts it produces (audit report, per-resource checklist, single `main.go` swap to `providerserver.NewProtocol6WithError`, layered `verify_tests.sh` gate) are designed around that shape. Applying them piecewise to a muxed provider would produce an inconsistent half-state — the very failure mode the skill warns against.

## What you actually need

A different workflow. The authoritative starting points:

1. **HashiCorp's mux migration guide** — the canonical multi-release path. Search "terraform-plugin-mux mux migration" or start at:
   - https://developer.hashicorp.com/terraform/plugin/framework/migrating/mux
   - https://developer.hashicorp.com/terraform/plugin/mux

2. **`terraform-plugin-mux` repo** — `github.com/hashicorp/terraform-plugin-mux`, specifically the `tf5to6server` and `tf6muxserver` (or `tf5muxserver`) packages. The two combinators you'll wire up in `main.go`:
   - `tf5to6server.UpgradeServer` — wraps an SDKv2 (protocol v5) server so it can sit behind a v6 mux.
   - `tf6muxserver.NewMuxServer` — combines the upgraded SDKv2 server with your `providerserver.NewProtocol6` framework server.

3. **HashiCorp examples** — the `terraform-provider-tls` and `terraform-provider-random` providers both shipped through a mux phase; their `main.go` and provider-definition history is the cleanest reference for the wiring.

## Rough shape of what your migration will look like (so you can scope it)

This is informational, not a migration plan — I'm not authoring code under this skill.

- **Release N (mux entry)**: Keep all resources on SDKv2. Introduce a second, empty framework provider. Wire both into a v6 mux in `main.go`. No user-visible behaviour change. Ship.
- **Release N+1**: Move 1–N "early" resources from SDKv2 → framework, one at a time. Each resource is removed from the SDKv2 provider's `ResourcesMap` and added to the framework provider's `Resources()`. The mux handles routing. Ship.
- **Release N+2 (mux exit)**: Once the last SDKv2 resource is migrated, drop the SDKv2 provider, remove the mux, ship a plain framework provider.

Two cross-cutting constraints you'll need to plan for and which the mux docs cover in depth:

- **Resource names cannot appear in both servers simultaneously**; the mux will refuse to start. Migration is per-resource atomic.
- **Provider-level schema (the top-level `provider {}` block, `Configure`, shared meta)** lives in exactly one server. HashiCorp's recommendation is to keep it on SDKv2 until the very end, then port last. Splitting it mid-migration is the single most common source of mux pain.

## What I would do for you under a different skill

If a `sdkv2-to-framework-mux` skill existed, the right deliverables for *this* request would be:

1. Audit of the OpenStack provider (resource/data-source counts, state upgraders, custom importers, timeouts, `MaxItems: 1` block decisions) — this part *would* reuse `scripts/audit_sdkv2.sh` from this skill, since the inventory is workflow-agnostic.
2. A per-release migration plan: which resources ship in N+1, which slip to N+2, which provider-level `Configure` lives where each release.
3. The `main.go` mux wiring (the `tf5to6server.UpgradeServer` + `tf6muxserver.NewMuxServer` boilerplate).
4. A muxed-aware acceptance-test plan (which `ProtoV5ProviderFactories` vs `ProtoV6ProviderFactories` per-resource during the transition).

None of those should be produced under *this* skill, because doing so would violate its scope and the "never introduce terraform-plugin-mux" rule.

## How to proceed

- If you want the staged mux migration as described: use HashiCorp's mux guide directly, or invoke a skill purpose-built for that workflow.
- If you'd actually prefer the **single-release** migration (rip the band-aid in one go, no mux phase): re-ask without the mux/staged/multi-release language and this skill will run end-to-end on the OpenStack provider.

I have made **no edits** to `<openstack-clone>` and produced **no migrated `.go` files**, per the eval rules.
