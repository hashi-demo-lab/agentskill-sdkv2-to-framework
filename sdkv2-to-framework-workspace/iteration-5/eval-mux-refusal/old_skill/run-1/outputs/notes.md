# Notes — mux-refusal eval (old_skill / iteration-4 snapshot)

## Decision

**Refused.** The user's request is explicitly out of scope for the
`sdkv2-to-framework` skill at this snapshot. A `REFUSAL.md` has been
produced in this same outputs directory.

## Trigger words present in the user prompt

The skill's SKILL.md (lines 31, 37, 223) instructs the agent to refuse
when the user's wording mentions any of: *mux*, *muxed*, *muxing*,
*staged migration*, *two-release*, *multi-release*, or
*terraform-plugin-mux*.

The verbatim user prompt contains all of the following matches:

- "terraform-plugin-mux" — direct match on the package name
- "staged, multi-release migration" — matches *staged migration* and
  *multi-release*
- "two release cycles" — matches *two-release*
- "muxed provider" — matches *muxed* / *muxing*

Per the skill, this is an unambiguous refusal case — not an edge.

## Authority for the refusal

Three places in `SKILL.md` of the `skill-snapshot-i4` snapshot back this:

1. **Line 31** (in "When this skill applies"):
   > Before triggering, sanity-check the user's wording. If the request
   > mentions any of *mux*, *muxed*, *muxing*, *staged migration*,
   > *two-release*, *multi-release*, or *terraform-plugin-mux* — stop.
   > That's the multiplexed-migration path which is explicitly out of
   > scope […] Refer the user to HashiCorp's mux docs and exit instead
   > of applying the single-release workflow.

2. **Line 37** (in "Does NOT apply"):
   > `terraform-plugin-mux` multi-release migrations: out of scope for
   > this skill; the user wants the single-release path.

3. **Line 223** (in "What to never do"):
   > Don't introduce `terraform-plugin-mux`. Because muxing changes the
   > migration shape entirely (incremental over many releases vs the
   > single-release scope this skill targets); applying single-release
   > patterns to a muxed provider creates an inconsistent half-state.

## What was NOT done (per the eval rules)

- Did **not** read or modify any files under
  `/Users/simon.lynch/git/terraform-provider-openstack`.
- Did **not** run the audit script, the verify script, `go build`,
  `go vet`, `go test`, or `go mod tidy`.
- Did **not** produce any `migrated/*.go` files, an audit report, a
  migration checklist, or a populated `assets/` template.
- Did **not** propose a muxed-provider scaffold, since the skill
  explicitly forbids introducing `terraform-plugin-mux`
  (SKILL.md line 223).

## Outputs in this directory

- `notes.md` — this file
- `REFUSAL.md` — the refusal artefact returned to the caller

## Pointer for the user

The user should consult HashiCorp's official multi-release mux
migration guide rather than this skill:

- https://developer.hashicorp.com/terraform/plugin/framework/migrating/mux
- https://developer.hashicorp.com/terraform/plugin/mux/translating-protocol-version-5-to-6
- https://developer.hashicorp.com/terraform/plugin/framework/migrating/workflow#migrate-across-multiple-releases-using-muxing

(URLs reproduced from memory of HashiCorp's published docs structure;
the user should verify the live URLs.)
