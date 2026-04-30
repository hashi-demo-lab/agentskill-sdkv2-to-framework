# Refusal — terraform-plugin-mux multi-release migration is out of scope

## Decision

I'm not going to drive this task with the `sdkv2-to-framework` skill. Your request is
for a **muxed, multi-release, staged** migration — using `terraform-plugin-mux` to keep
SDKv2 and framework resources in the same provider binary across two release cycles.

That workflow is explicitly out of scope for this skill. The skill's `SKILL.md` calls it
out in four separate places:

- The frontmatter description: *"Does NOT cover: ... multi-release `terraform-plugin-mux`
  workflows"*.
- A pre-trigger guard: *"If the request mentions any of mux, muxed, muxing, staged
  migration, two-release, multi-release, or terraform-plugin-mux — **stop**."*
- A "Does NOT apply" list entry for `terraform-plugin-mux` multi-release migrations.
- A "What to never do" rule: *"Don't introduce terraform-plugin-mux"*, with the
  rationale that muxing changes the migration shape entirely (incremental over many
  releases vs the single-release scope this skill targets), and applying single-release
  patterns to a muxed provider creates an inconsistent half-state.

Your prompt hits four of those banned phrases verbatim (`terraform-plugin-mux`, `staged,
multi-release migration`, `muxed provider`, `two release cycles`), so this is a clean
refusal, not a judgement call.

## Why the skill draws this line

The skill is built for the **single-release-cycle** migration HashiCorp describes here:
https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-in-a-single-release-cycle

That workflow's invariants — one provider server, one protocol version, "remove all
SDKv2 references" as step 10, TDD gating per resource — actively conflict with what a
muxed provider needs. A muxed provider has *two* servers running concurrently, must keep
SDKv2 imports alive on purpose, has to coordinate non-overlapping resource registrations
across both servers, and plans the SDKv2 sunset as a separate release. Trying to use
this skill's checklists and verification gates would either lie to you (the negative
gate "no file imports SDKv2" makes no sense mid-mux) or push you toward an
inconsistent half-state.

## What you should do instead

Use HashiCorp's official mux documentation. The relevant entry points:

1. **Migration workflow overview** — the page that contrasts the single-release path
   (this skill) with the mux multi-release path (your request):
   https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow

2. **Combining provider servers with mux** (pick the protocol version your SDKv2
   provider currently serves; SDKv2 defaults to v5):
   - v6: https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-6-providers
   - v5: https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-5-providers

3. **Mux server API reference** — the actual `NewMuxServer` constructors you'll wire in
   `main.go`:
   - https://pkg.go.dev/github.com/hashicorp/terraform-plugin-mux/tf6muxserver
   - https://pkg.go.dev/github.com/hashicorp/terraform-plugin-mux/tf5muxserver

4. **Protocol version translation** — needed if the SDKv2 half stays on v5 while the
   framework half ships v6 (common in two-release plans):
   https://developer.hashicorp.com/terraform/plugin/mux/translating-protocol-version-5-to-6

The shape of the work HashiCorp's docs walk you through:

- Stand up a framework `provider.Provider` alongside your existing SDKv2 provider
  (initially with zero resources/data sources registered on the framework side).
- Switch `main.go` to serve a mux server that combines both — `tf6muxserver.NewMuxServer`
  (or v5) given the SDKv2 server (via `schema.Provider.GRPCProvider` plus the v5→v6
  upgrader if needed) and the framework server (via `providerserver.NewProtocol6`).
- Verify the muxed provider boots and serves the existing SDKv2 resources unchanged.
- *Then*, per release, move a batch of resources: register them in the framework
  provider, deregister them from the SDKv2 provider (mux disallows duplicate
  registration), and ship.
- Plan the final release: SDKv2 server and the mux wrapper come out, framework server
  becomes the only server.

## Where this skill *can* still help you

Inside each release, when you're doing the actual SDKv2 → framework rewrite for a chosen
batch of resources (schema conversion, CRUD method rewrites, validators, plan modifiers,
state upgraders, tests), that per-resource conversion work is exactly what this skill is
built for. Once you've decided which resources move in release N, come back and ask the
skill to migrate those specific resources — at that point the single-release-cycle
patterns apply correctly to the in-scope subset, and the bundled references
(`schema.md`, `resources.md`, `state-upgrade.md`, etc.) are directly useful.

What stays out of scope even then: the mux wiring itself, the release-cycle planning,
non-overlap bookkeeping between the two servers, the v5/v6 protocol translation, and
the SDKv2 sunset release. Those are the parts HashiCorp's mux docs own.
