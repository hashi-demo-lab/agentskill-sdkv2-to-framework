# Notes — eval-mux-refusal / with_skill / run-3

## Decision

**Refused.** REFUSAL.md emitted. No migration work performed, no `migrated/*.go` files produced, no edits to the OpenStack clone.

## Reasoning trace

1. Read `<repo>/provider-sdkv2-to-framework/SKILL.md` first, per eval rules.
2. The user's prompt contains every refusal trigger the skill enumerates:

   | Trigger phrase in SKILL.md | Match in user prompt |
   |---|---|
   | "mux" | "using terraform-plugin-mux" |
   | "muxed" | "set up the muxed provider" |
   | "staged migration" | "staged, multi-release migration" |
   | "two-release" | "across two release cycles" |
   | "multi-release" | "multi-release migration" |
   | "terraform-plugin-mux" | "using terraform-plugin-mux" |

3. Three independent sections of SKILL.md mandate refusal:
   - **"When this skill applies"** — *"stop. That's the multiplexed-migration path which is explicitly out of scope. Refer the user to HashiCorp's mux docs and exit instead of applying the single-release workflow."*
   - **"Does NOT apply"** — lists `terraform-plugin-mux` multi-release migrations explicitly.
   - **"What to never do"** — *"Don't introduce `terraform-plugin-mux`."* with explicit *because* clause about half-state risk.

4. No need to inspect the OpenStack repo, run `audit_sdkv2.sh`, or read any reference files — the refusal is a property of the *request shape*, not the codebase. Reading further would burn tokens and risk producing the very mux scaffolding the skill forbids.

## Artefacts

- `REFUSAL.md` — explanation, citation of the three SKILL.md sections, pointers to HashiCorp's mux docs and the `terraform-plugin-mux` repo packages (`tf5to6server.UpgradeServer`, `tf6muxserver.NewMuxServer`), rough shape of a 3-release mux plan (informational only, not a migration plan), and an off-ramp if the user actually wants the single-release path instead.
- `notes.md` — this file.

## Read-only verification

- No commands run against `<openstack-clone>`.
- No `go build`, `go vet`, `go test`, `go mod tidy` invoked (per eval rules).
- All outputs confined to this run's `outputs/` directory.

## Observations on the skill itself (for the eval harness)

The skill's refusal triggers are well-targeted: a single read of SKILL.md is sufficient to make this decision confidently without further tool calls. The "When this skill applies" sanity-check paragraph is the load-bearing instruction — it appears *before* the workflow steps, so an agent following SKILL.md top-to-bottom hits the refusal gate before being tempted to start an audit. The triple redundancy (sanity-check + "Does NOT apply" list + "never introduce mux" rule) is appropriate insurance for a high-cost wrong answer.
