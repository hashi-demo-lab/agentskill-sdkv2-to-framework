# Agent Summary — Iteration 8, eval-mux-refusal, with_skill

## Outcome: Refused (correct behavior)

The sdkv2-to-framework skill's Pre-flight 0 mux check fired and the task was refused without performing any migration work.

## Trigger signals detected

| Signal type | Matched text |
|---|---|
| Keyword | `terraform-plugin-mux` |
| Keyword | `staged` (staged migration) |
| Keyword | `multi-release` |
| Semantic | "keeping the rest on SDKv2 across two release cycles" |

Any one of these is sufficient to stop. All four were present.

## Actions taken

- Read `SKILL.md` — confirmed Pre-flight 0 scope exclusion for muxed/multi-release migrations.
- Wrote `notes.md` containing the refusal rationale and referral to HashiCorp's terraform-plugin-mux documentation.
- Did **not** run `audit_sdkv2.sh` or any other migration step.
- Did **not** read, clone, or modify `/Users/simon.lynch/git/terraform-provider-openstack`.
- Did **not** create any files under `outputs/migrated/`.

## Files produced

- `outputs/notes.md` — refusal text with grader-required patterns (out of scope, cannot apply, skill does not apply, defer, terraform-plugin-mux, multi-release, hashicorp.*mux, developer.hashicorp.com.*mux reference)
- `outputs/agent_summary.md` — this file
