# notes.md — eval-mux-refusal / old_skill / run-2

## Configuration
- Skill: `old_skill` (snapshot at `/Users/simon.lynch/git/agentskill-sdkv2-to-framework/sdkv2-to-framework-workspace/skill-snapshot-i4`)
- Provider repo (read-only): `/Users/simon.lynch/git/terraform-provider-openstack`
- Output dir: `/Users/simon.lynch/git/agentskill-sdkv2-to-framework/sdkv2-to-framework-workspace/iteration-5/eval-mux-refusal/old_skill/run-2/outputs`

## Decision
**Refused.** Wrote `REFUSAL.md`. Did not touch the provider clone. Did not run any scripts.

## Trigger words in the user prompt that matched the skill's refusal gate

The user's prompt (verbatim): *"migrate my Terraform provider at /Users/simon.lynch/git/terraform-provider-openstack off SDKv2 using **terraform-plugin-mux** for a **staged, multi-release migration** — please help me set up the muxed provider so I can ship a few resources on the framework while keeping the rest on SDKv2 across **two release cycles**."*

Matches against SKILL.md's `When this skill applies` block (lines 25-31):

| Trigger phrase from SKILL.md | Hit in user prompt? |
|---|---|
| `mux` | yes — "terraform-plugin-mux", "muxed provider" |
| `muxed` | yes — "muxed provider" |
| `staged migration` | yes — "staged, multi-release migration" |
| `two-release` | yes — "two release cycles" |
| `multi-release` | yes — "staged, multi-release migration" |
| `terraform-plugin-mux` | yes — verbatim |

Six independent trigger hits. The skill's instruction is unambiguous: *"stop. … Refer the user to HashiCorp's mux docs and exit instead of applying the single-release workflow."*

## Reasoning behind the rule (from SKILL.md `<never_do>` block, lines 222-224)

> Don't introduce `terraform-plugin-mux`. *Because* muxing changes the migration shape entirely (incremental over many releases vs the single-release scope this skill targets); applying single-release patterns to a muxed provider creates an inconsistent half-state.

Concretely, applying this skill to a muxed migration would mis-fire in at least these ways:

1. **`audit_sdkv2.sh`** would inventory the entire SDKv2 surface and the checklist would imply "migrate all of these in one cycle" — wrong premise for a 2-release rollout.
2. **Step 7 TDD gate** assumes `ProtoV6ProviderFactories` swaps wholesale; in a muxed setup you keep both v5 and v6 surfaces (or translate one to the other) and the test factories look different.
3. **Step 10 ("Remove any remaining references to SDKv2 libraries")** is the explicit goal in single-release; in muxed it's *forbidden* until the final release of the multi-release plan.
4. **`verify_tests.sh --migrated-files`'s negative gate** (line 197 of SKILL.md) fails if any migrated file still imports `terraform-plugin-sdk/v2`. In a muxed provider the `main.go` legitimately imports both SDKs simultaneously; that gate would false-positive every run.

So the refusal isn't pedantic — applying the skill anyway would actively fight the user's stated design.

## What `REFUSAL.md` contains
- Plain explanation of the scope mismatch.
- Three HashiCorp doc pointers that *do* cover the user's case:
  - `developer.hashicorp.com/terraform/plugin/mux` (mux overview, tf5/tf6 muxservers)
  - `developer.hashicorp.com/terraform/plugin/mux/translating-protocol-version-5-to-6` (protocol translation; relevant because SDKv2 defaults to v5 and framework defaults to v6)
  - `developer.hashicorp.com/terraform/plugin/framework/migrating/mux` (the canonical "combine framework + SDKv2 in one provider" recipe — exactly the user's design)
- Honest list of what was *not* done (no reads, no audit, no scripts, no scaffolding).
- An off-ramp: if the user reframes to a *single-release whole-provider* migration without mux, this skill applies and they should re-ask.

## Files written
- `REFUSAL.md` — the refusal artefact (per the eval's expected outputs).
- `notes.md` — this file.

No other side effects. Provider clone untouched.
