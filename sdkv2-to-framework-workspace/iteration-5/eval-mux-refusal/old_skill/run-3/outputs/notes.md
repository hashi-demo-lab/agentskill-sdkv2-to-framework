# Notes — eval-mux-refusal, old_skill, run-3

## Configuration
- Skill under test: `old_skill` = `skill-snapshot-i4`
- Skill path: `/Users/simon.lynch/git/agentskill-sdkv2-to-framework/sdkv2-to-framework-workspace/skill-snapshot-i4`
- Target repo (read-only): `/Users/simon.lynch/git/terraform-provider-openstack`
- Output dir: `/Users/simon.lynch/git/agentskill-sdkv2-to-framework/sdkv2-to-framework-workspace/iteration-5/eval-mux-refusal/old_skill/run-3/outputs`

## User prompt (verbatim)
> I want to migrate my Terraform provider at /Users/simon.lynch/git/terraform-provider-openstack
> off SDKv2 using terraform-plugin-mux for a staged, multi-release migration —
> please help me set up the muxed provider so I can ship a few resources on
> the framework while keeping the rest on SDKv2 across two release cycles.

## Trigger phrases detected in the prompt
The skill's `SKILL.md` lists explicit refuse-triggers under
"Before triggering, sanity-check the user's wording". Matches found:

| Skill trigger word | Found in user prompt? | Evidence |
|---|---|---|
| `mux` / `terraform-plugin-mux` | yes | "using terraform-plugin-mux" |
| `staged migration` | yes | "for a staged, multi-release migration" |
| `multi-release` | yes | "multi-release migration", "across two release cycles" |
| `two-release` | yes (paraphrased) | "across two release cycles" |
| `muxed` | yes | "set up the muxed provider" |

Five of the six listed trigger phrases match. The skill's instruction is
unambiguous: **stop**, refer the user to HashiCorp's mux docs, do not apply
the single-release workflow.

## Decision
Refuse. Wrote `REFUSAL.md` in the output directory with:
- Why this skill does not apply (verbatim quotes from `SKILL.md`'s
  "Before triggering" guard, "Does NOT apply" list, and "What to never do"
  rule on `terraform-plugin-mux`).
- Where the user should go instead (HashiCorp's mux docs, with the relevant
  URLs and a high-level outline of the muxed multi-release shape — kept at
  pointer-level, not as a substitute migration plan).
- The specific constraints unique to mux that this skill does not cover
  (no duplicate type names, protocol-version alignment via `tf5to6server`,
  schema parity for state continuity, mux-aware test factories).
- Explicit list of things I did **not** do (no audit run, no edits to the
  openstack repo, no `go build/vet/test/mod tidy`).

## Read-only / scope adherence
- Did not read any file under `/Users/simon.lynch/git/terraform-provider-openstack`.
- Did not invoke `scripts/audit_sdkv2.sh` (would have implied accepting the
  task).
- Did not run any Go toolchain command.
- All outputs are inside the configured `OUTPUT DIR` only.

## Files produced
- `REFUSAL.md` — the formal refusal, with rationale and pointers.
- `notes.md` — this file.
