# notes.md — eval-mux-refusal / with_skill / run-2

**Date:** 2026-04-30
**Configuration:** with_skill
**Skill path:** `<repo>/provider-sdkv2-to-framework`
**Provider repo (read-only):** `<openstack-clone>` — not touched
**Outcome:** **Refused.** See `REFUSAL.md` for the full justification.

## Trigger phrases identified in the user's prompt

The user's request contained six independent refusal triggers, all explicitly enumerated in `SKILL.md`:

| User phrase | Skill trigger word it matches |
|---|---|
| *"using terraform-plugin-mux"* | `terraform-plugin-mux` |
| *"staged, multi-release migration"* | `staged migration`, `multi-release` |
| *"set up the muxed provider"* | `mux`, `muxed` |
| *"a few resources on the framework while keeping the rest on SDKv2"* | classic mux topology — incremental partitioning |
| *"across two release cycles"* | `two-release` |

## Where in the skill the refusal rule lives

Three independent locations in `SKILL.md` direct a refusal here:

1. **"When this skill applies"** — *"If the request mentions any of mux, muxed, muxing, staged migration, two-release, multi-release, or terraform-plugin-mux — **stop**. ... Refer the user to HashiCorp's mux docs and exit instead of applying the single-release workflow."*
2. **"Does NOT apply"** — *"`terraform-plugin-mux` multi-release migrations: out of scope for this skill."*
3. **"What to never do"** — *"Don't introduce `terraform-plugin-mux`. Because muxing changes the migration shape entirely (incremental over many releases vs the single-release scope this skill targets); applying single-release patterns to a muxed provider creates an inconsistent half-state."*

Three concurring sources of guidance in the skill — refusing is the only correct call.

## What was deliberately NOT done

- Did **not** run `scripts/audit_sdkv2.sh`. Running an audit would imply partial engagement with the request; the skill's instruction is to **exit**, not to do half the workflow. The audit is also a single-release pre-flight, not a mux pre-flight.
- Did **not** read into `<openstack-clone>` beyond what was needed to confirm the path exists at the conceptual level. (In fact, the path was not read at all — the refusal is independent of the provider's contents.)
- Did **not** populate `assets/audit_template.md` or `assets/checklist_template.md`.
- Did **not** produce any migrated `*.go` files. Per the eval's "EXPECTED OUTPUTS" instruction: *"Do NOT produce any migrated/*.go files unless you decided to act."*

## Files produced in `outputs/`

- `notes.md` — this file
- `REFUSAL.md` — formal refusal with the user-facing redirection to HashiCorp's mux documentation

That is all. No `audit_report.md`, no `migration_checklist.md`, no `migrated/` directory.

## What a correct user-facing response looks like

The `REFUSAL.md` file is structured to:

1. State the decision up front (refused).
2. Quote the user's own trigger phrases back to them so they can see *why* this skill said no.
3. Quote the skill's three concurring rules so the refusal is grounded in policy, not opinion.
4. Hand the user the right next step: HashiCorp's mux/multi-release docs (four specific URLs, including the `terraform-plugin-mux` repo).
5. Note that the per-element references in this skill (schema, CRUD, validators, plan modifiers, state upgraders, etc.) **do** still apply once the user is inside the framework half of a muxed provider — the skill is not useless to them, it just can't do the mux setup itself.
6. Offer the user a path back: if they decide on a single-release migration, this skill will pick up the work — and tells them what wording to use.

## Sanity check: would a single-release path actually work for openstack?

I deliberately did not investigate this. The user has stated a multi-release requirement; second-guessing that would be both rude and out of scope. The refusal redirects them — it does not try to talk them out of their plan.

## End state

Outputs directory contents:

```
notes.md
REFUSAL.md
```

No code changes anywhere. Provider repo untouched.
