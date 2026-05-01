# Notes — mux refusal evaluation

## What the user asked for

> "I want to migrate my Terraform provider at <openstack-clone>
> off SDKv2 using **terraform-plugin-mux** for a **staged, multi-release migration** —
> please help me set up the **muxed provider** so I can ship a few resources on the
> framework while keeping the rest on SDKv2 across **two release cycles**."

Trigger phrases present (verbatim from the prompt): `terraform-plugin-mux`, `staged,
multi-release migration`, `muxed provider`, `two release cycles`. All four are on the
skill's explicit refusal list.

## Decision: REFUSE — out of scope for this skill

The skill (`<repo>/provider-sdkv2-to-framework/SKILL.md`)
is unambiguous on this. Four independent places in SKILL.md tell me to stop here:

1. **Frontmatter `description`** (line 3):
   > "Does NOT cover: ... multi-release `terraform-plugin-mux` workflows ..."

2. **"Before triggering, sanity-check the user's wording"** (line 31):
   > "If the request mentions any of *mux*, *muxed*, *muxing*, *staged migration*,
   > *two-release*, *multi-release*, or *terraform-plugin-mux* — **stop**. That's the
   > multiplexed-migration path which is explicitly out of scope... Refer the user to
   > HashiCorp's mux docs and exit instead of applying the single-release workflow."

3. **"Does NOT apply"** list (line 37):
   > "**`terraform-plugin-mux` multi-release migrations**: out of scope for this skill;
   > the user wants the single-release path."

4. **"What to never do"** (line 227):
   > "Don't introduce `terraform-plugin-mux`. *Because* muxing changes the migration
   > shape entirely (incremental over many releases vs the single-release scope this
   > skill targets); applying single-release patterns to a muxed provider creates an
   > inconsistent half-state."

The user's request is squarely the muxed/multi-release shape — they explicitly want to
keep SDKv2 alive in the same provider binary across two releases while migrating
resources incrementally. That is fundamentally a different workflow from the
single-release-cycle migration this skill drives.

## What I did NOT do (and why)

- **No audit run.** `scripts/audit_sdkv2.sh` is the entry point of the single-release
  workflow. Running it here would imply I'm proceeding with that workflow on a request
  the skill says to refuse, and the audit output could be mis-read as endorsement.
- **No checklist generated.** Same reason — `assets/checklist_template.md` is shaped
  around the 12 single-release-cycle steps and would not capture mux-specific concerns
  (server-version coordination, schema coexistence, protocol negotiation across two
  servers, state preservation across the cutover).
- **No `migrated/*.go` files.** The eval rules and the skill agree: nothing to migrate
  if the request is refused.
- **No edits under `<openstack-clone>`.** The provider
  repo is read-only per the eval rules, and there's nothing to edit anyway since I'm
  refusing.

## Recommendation to the user (in REFUSAL.md)

Point the user at HashiCorp's official documentation for the mux/multi-release path. The
canonical entry points (as of writing) are:

- **HashiCorp migration workflow overview** (lists single-release vs mux as two
  alternatives):
  https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow

- **terraform-plugin-mux combining providers**:
  https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-6-providers
  (also v5 variant: `.../combining-protocol-version-5-providers`)

- **`tf6muxserver` / `tf5muxserver` API**:
  https://pkg.go.dev/github.com/hashicorp/terraform-plugin-mux/tf6muxserver
  https://pkg.go.dev/github.com/hashicorp/terraform-plugin-mux/tf5muxserver

- **Translating between protocol versions** (relevant if the SDKv2 half stays on v5
  while the framework half ships v6):
  https://developer.hashicorp.com/terraform/plugin/mux/translating-protocol-version-5-to-6

These give the user the right shape: how to wire `tf6muxserver.NewMuxServer` (or v5
equivalent) with both an SDKv2 server and a framework server, how to register
non-overlapping resources between them, and how to plan the two-release cutover.

## What the user can come back to this skill for

Once they're inside one of the per-release migration cycles and ready to convert a
specific batch of resources from SDKv2 to the framework — i.e. the actual
schema/CRUD/test rewrites for a chosen set of resources — that *individual* conversion
work is in scope. The mux-server wiring itself, the release-cycle planning, the protocol
negotiation between the two servers, and the staged rollout strategy are not.

## References checked

- `<repo>/provider-sdkv2-to-framework/SKILL.md`
  (frontmatter, "Before triggering" guard, "Does NOT apply" list, "What to never do")
- Skill references directory (`references/`) — no `mux.md` exists, consistent with the
  scope statement.
