# Refusal — staged `terraform-plugin-mux` migration is out of scope

Hi — I can't help with this particular request using the
`sdkv2-to-framework` skill, and I want to be upfront about why before
you spend more time on it.

## Why I'm refusing

Your prompt asks for a **muxed**, **staged**, **multi-release**
migration using `terraform-plugin-mux` across **two release cycles**.
Each one of those phrasings is an explicit out-of-scope marker for
this skill:

| Your wording                            | Skill marker (SKILL.md)                          |
|-----------------------------------------|--------------------------------------------------|
| `terraform-plugin-mux`                  | line 31, 37, 223 — explicit refusal              |
| "staged, multi-release migration"       | line 31 — *staged migration*, *multi-release*    |
| "two release cycles"                    | line 31 — *two-release*                          |
| "muxed provider"                        | line 31, 223 — *muxed* / *muxing*                |

The skill drives the **single-release-cycle** workflow from HashiCorp's
docs: audit → plan → 12 mechanical steps → done in one release. Mux
inverts that shape — it's incremental over many releases, with two
provider servers running side by side and resources moving across in
batches. Trying to graft single-release patterns onto a muxed provider
produces an inconsistent half-state (per `SKILL.md` line 223–224).

So this is a deliberate scope boundary, not a limitation I can talk
myself out of.

## Where to go instead

HashiCorp publishes the mux path as a first-class workflow. The
relevant entry points:

- **Multi-release migration with muxing** —
  HashiCorp's "Migrate Across Multiple Releases Using Muxing" page in
  the plugin-framework migration guide. This is the workflow you
  actually want.
- **`terraform-plugin-mux` package docs** — the
  `tf5to6server` / `tf6to5server` translation servers (you'll need
  these because SDKv2 speaks protocol v5 and most framework features
  assume v6) and the `tf5muxserver` / `tf6muxserver` multiplexers
  (which combine an SDKv2 server and a framework server into one).
- **Provider examples** — the AWS, Google, and Azure providers all
  shipped staged mux migrations and their `main.go` and
  `internal/provider/` directories are good public references for
  the wiring.

Search terms that resolve to the right HashiCorp pages:
*"terraform plugin framework migrating muxing"*, *"tf5to6server"*,
*"tf6muxserver example"*.

## What you'd want to think about before starting

(Not skill output — just orientation, since you asked.)

1. **Protocol versions.** SDKv2 = v5. Framework defaults are typically
   v6. You'll either downgrade the framework half to v5 with
   `tf6to5server` (simpler, but you lose v6-only features) or upgrade
   the SDKv2 half to v6 with `tf5to6server` (more capable, but every
   resource gets re-encoded on each request).
2. **Resource ownership boundary.** A given resource type lives on
   exactly one server. The mux server routes by type name. Pick the
   first batch to be **simple, well-tested, low-state-upgrade-risk**
   resources — the goal of release 1 is to prove the wiring, not to
   tackle the hardest resources.
3. **State compatibility.** Practitioners should see zero schema or
   ID changes when a resource crosses the mux boundary. That's a
   stricter constraint than a single-release migration, because the
   same `.tfstate` has to round-trip through the new server.
4. **Two-release split.** The conventional split is: release N
   introduces the mux harness with maybe 2–3 resources on the
   framework as a smoke test; release N+1 (or N+2) moves the bulk;
   a final release deletes SDKv2. Your "two release cycles" phrasing
   suggests you're collapsing that to N + N+1 — workable for a
   smaller provider, but tight for openstack-sized surface area.
   You'll want to enumerate the resource count before committing.

If, after reading HashiCorp's mux guide, you decide to do a
**single-release** migration of the openstack provider instead (i.e.
no mux, all resources on the framework in one release), come back —
that's exactly what this skill is built for, and openstack is a
reasonable candidate (large but mostly mechanical).

## What I did not produce

Per your instructions and the skill's rules:

- No files were read from `<openstack-clone>`.
- No `migrated/*.go` files were created.
- No audit report or migration checklist was generated — those would
  presume the single-release workflow, which doesn't apply here.

The only artefacts in this run are `notes.md` (audit trail) and this
`REFUSAL.md`.
