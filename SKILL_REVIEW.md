# SKILL.md review — `provider-sdkv2-to-framework`

**Target**: `<repo>/provider-sdkv2-to-framework/SKILL.md` (234 lines, 19.5 KB at time of review; pre-`provider-` prefix the directory was named `sdkv2-to-framework`)
**Reviewers**: 3 concurrent Opus agents — structural/authoring, content effectiveness, triggering surface
**Method**: critique against Anthropic's canonical skill-authoring guidance, cited inline below.

---

## Authoritative sources cited

Each finding references one or more of these. Full quotations of the rules appear in **Appendix A** at the end.

| Tag | Source | URL / path |
|---|---|---|
| **[BP]** | Anthropic — *Skill authoring best practices* | https://platform.claude.com/docs/en/agents-and-tools/agent-skills/best-practices |
| **[OV]** | Anthropic — *Skills overview* (frontmatter limits) | https://platform.claude.com/docs/en/agents-and-tools/agent-skills/overview#skill-structure |
| **[SC]** | `skill-creator` — Anthropic's bundled skill-authoring skill | `~/.claude/plugins/cache/claude-plugins-official/skill-creator/unknown/skills/skill-creator/SKILL.md` |

Where I cite a specific rule, I use shorthand like **[BP §"Writing effective descriptions"]** or **[SC L67]**.

---

## Executive summary

The skill is **structurally strong** and follows several Anthropic best practices well: progressive disclosure with bundled `references/*.md` files **[BP §"Progressive disclosure patterns"]**, scripts in `scripts/` **[BP §"Provide utility scripts"]**, the `verify_tests.sh` validator-loop pattern **[BP §"Implement feedback loops"]**, and *because*-clauses on every "What to never do" rule **[BP §"Default assumption: Claude is already very smart" + SC §"Writing Style"]**.

Two **P0** risks dominate, both about the description / triggering surface — the highest-leverage area in any skill **[BP §"Writing effective descriptions": "The description is critical for skill selection"]**:

1. Negative triggers (SDK v1, mux) trail the description; Anthropic's router weights front-loaded signal more strongly than tail clauses, so SDK v1 prompts and "phased mux migration" prompts are likely to false-fire.
2. The mid-body mux bail-out (lines 30–31) is prose, not a hard gate — once the skill has triggered, a model deep in execution may treat the bare word "stop" as advisory.

The biggest structural lever is **reducing duplication** between SKILL.md and `references/blocks.md` — Anthropic's progressive-disclosure rule is "SKILL.md serves as an overview that points Claude to detailed materials as needed, like a table of contents" **[BP §"Progressive disclosure patterns"]**. The 49-line `MaxItems_1` worked example currently lives in both places, which violates the pattern.

After the P0+P1 fixes the file should land at ~170 lines (well under Anthropic's 500-line target **[BP §"Token budgets"]**) with sharper triggering and better adherence to the documented patterns.

---

## P0 — blocks correct triggering or causes wrong behavior

### P0-1 — Description ordering causes false-positive triggers
- **Location**: line 3 (frontmatter `description`)
- **Anthropic rule**: **[BP §"Writing effective descriptions": "Be specific and include key terms. Include both what the Skill does and specific triggers/contexts for when to use it."]** and **[SC L67: "Note: currently Claude has a tendency to 'undertrigger' skills … make the skill descriptions a little bit 'pushy'"]**. The router uses *only* the description to select among 100+ skills.
- **Issue**: Positive triggers ("migrate", "port", "rewrite", "upgrade", "convert", "framework") lead the description; negatives ("Does NOT cover SDK v1 … `terraform-plugin-mux`") trail. Prompts like "I have a SDK v1 provider, port to framework" or "set up terraform-plugin-mux for a staged migration" are likely to false-fire because the negative clause is buried.
- **Fix**: Front-load the source-SDK constraint. Lead with: *"Use only when the source SDK is `terraform-plugin-sdk/v2` (not v1, not plugin-go-only). Not for muxed or multi-release migrations."* Then state capability and trigger phrases. Rebalances signal weight per **[BP]**.

### P0-2 — Mux bail-out is mid-body prose, not a gate
- **Location**: lines 30–31
- **Anthropic rule**: **[BP §"Use workflows for complex tasks": "Break complex operations into clear, sequential steps … provide a checklist that Claude can copy into its response and check off as it progresses."]** Bail-out checks should be steps with clear pass/fail criteria, not narrative prose.
- **Issue**: The mux check is a paragraph asking the model to "**stop**" if it sees certain words. Once the skill has fired, a model in deep-context invocation may treat this as advisory. Vocabulary-implicit prompts ("phased rollout across two releases") bypass the keyword list entirely.
- **Fix**: Promote to "Pre-flight 0 — exit if mux", framed as a workflow step with (a) a concrete check `grep -E 'mux|muxed|two-release|staged|phased' <user-prompt>` *and* (b) a phrase-agnostic semantic test: *"If the user wants the migration spread across more than one provider release, this skill does not apply, regardless of vocabulary."* Hard-exit before Pre-flight A.

### P0-3 — Step 7 TDD gate has fuzzy acceptance criteria
- **Location**: lines 83–89 (`<workflow_step number="7">`)
- **Anthropic rule**: **[BP §"Set appropriate degrees of freedom": "Low freedom (specific scripts, few or no parameters) — Use when: Operations are fragile and error-prone, Consistency is critical, A specific sequence must be followed."]** A TDD gate is exactly this — fragile, consistency-critical. The skill rightly uses low freedom here, but the *acceptance criteria* leak high-freedom ambiguity.
- **Issue**: "Quote the failing output verbatim" is actionable, but the acceptance signatures are under-specified. A test renamed to `ProtoV6ProviderFactories` against an unmigrated resource often compiles and fails at *runtime* (`schema for resource X not found`), not at compile time as line 87 implies. A model could see a runtime failure and conclude the gate hasn't been satisfied. The "(or the unit-test name if no acceptance tests)" parenthetical doesn't tell the model what to do when no test exists at all.
- **Fix**: Enumerate acceptable failure shapes (compile error on removed SDKv2 type, `protocol version mismatch`, `schema for resource X not found`, schema-shape assertion mismatch) and one explicit unacceptable shape ("test passed unchanged"). Add a sub-step: if no test exists, write a minimal one before proceeding — never skip the gate.

---

## P1 — degrades quality at scale

### P1-1 — `MaxItems_1` worked example duplicates `references/blocks.md`
- **Location**: lines 124–172 (49 lines, ~20% of file)
- **Anthropic rule**: **[BP §"Progressive disclosure patterns": "SKILL.md serves as an overview that points Claude to detailed materials as needed, like a table of contents in an onboarding guide. Keep SKILL.md body under 500 lines."]** And **[BP §"Concise is key": "Once Claude loads SKILL.md, every token competes with conversation history and other context."]**
- **Issue**: The full decision rule + both code outputs live inline AND in `references/blocks.md` (per the line-171 pointer). Pure duplication that both crowds SKILL.md and forces the model to reconcile two copies if they drift.
- **Fix**: Keep a 6–8 line stub in SKILL.md — the 3-question decision rule, no code blocks, with a pointer. Move the full worked example (both code samples, the rationale paragraph) into `references/blocks.md`. Saves ~40 lines / ~2.5 KB.

### P1-2 — "When this skill applies" appears after Prerequisites
- **Location**: line 12 (Prerequisites) precedes line 23 ("When this skill applies")
- **Anthropic rule**: **[BP §"Observe how Claude navigates Skills": "Pay attention to how Claude actually uses them in practice. Watch for: Unexpected exploration paths — does Claude read files in an order you didn't anticipate?"]** A model scanning top-down should hit applicability gates before scaffolding instructions.
- **Issue**: The skill tells a reader to install semgrep before telling them whether the skill applies at all. Negative-trigger sections belong as close to skill activation as possible.
- **Fix**: Reorder: Title → "When this skill applies" + "Does NOT apply" → Prerequisites → Workflow.

### P1-3 — Pitfall list missing 3–4 high-leverage footguns
- **Location**: lines 207–219 (`<common_pitfalls>`)
- **Anthropic rule**: **[BP §"Examples pattern": "For Skills where output quality depends on seeing examples, provide input/output pairs."]** The pitfall section serves the same function — input/output pairs of "what you'll write wrong / what you should write".
- **Issue**: Missing patterns the migration skill will repeatedly encounter:
  - **Delete plan-null**: `req.Plan` is null on Delete; reading from it panics. Read from `req.State` instead.
  - **`tfsdk:"foo"` struct tag mismatch**: the #1 silent state-mapping bug — wrong/missing tag silently drops the field.
  - **`Description` vs `MarkdownDescription`**: setting both differently causes docs drift; the framework prefers one consistently per attribute.
  - **Identity vs `ImportStatePassthroughContext` precedence**: both are referenced (lines 115/116) but no rule on which wins when both are configured.
- **Fix**: Add 4 bullets following the existing pattern.

### P1-4 — Only one inline worked example; state upgraders deserve a second
- **Location**: lines 124–172 is the only worked example
- **Anthropic rule**: **[BP §"Examples pattern": "Examples help Claude understand the desired style and level of detail more clearly than descriptions alone."]**
- **Issue**: State upgraders are the second-most-judgment-heavy decision. Single-step composition (vs SDKv2's chained V0→V1→V2) is a pattern an LLM gets wrong by analogy unless it sees the correct shape inline. Currently only "see `references/state-upgrade.md`" — but per **[BP §"Avoid deeply nested references"]**, a single reference indirection is fine, but inline examples for the most-judgment-heavy decisions reliably beat reference reads.
- **Fix**: Add a second `<example name="state_upgrader_collapse">` showing a chained V0→V1→V2 SDKv2 sequence collapsing into per-version functions that each produce target-version state directly. Optionally a third for composite-ID `ImportState`.

### P1-5 — Reference table gaps
- **Location**: lines 100–122
- **Anthropic rule**: **[BP §"Domain-specific organization": "When a Skill supports multiple domains, organize content by domain to avoid loading irrelevant context."]** The reference table IS this pattern — but the entry points need to map cleanly to the decision surface.
- **Issue**: (a) No row for `Configure`/provider client plumbing — the `*Client`-via-`req.ProviderData` pattern is a common bug, currently buried in `provider.md`. (b) `deprecations.md` (line 121) is reactive ("you might emit by mistake") — a model won't load it pre-emptively. (c) `UseStateForUnknown` (the second-biggest plan-modifier footgun) isn't flagged inline the way `Default` is.
- **Fix**: Add a "Provider configuration & client plumbing" row → `provider.md`. Move `deprecations.md` from the table to a workflow rule ("consult before any new symbol"). Append `(and UseStateForUnknown for computed-after-apply attributes)` to the plan-modifiers row.

### P1-6 — Description has ~180 chars of repetitive trigger phrases
- **Location**: line 3
- **Anthropic rule**: **[OV: "description: Maximum 1024 characters"]** is a hard limit; the description is currently ~975 chars and will grow if more negatives are added per P0-1. **[BP §"Concise is key"]** also applies.
- **Issue**: Five near-synonym trigger phrases plus internal-detail clause ("Drives the canonical 12-step…TDD gating at step 7, layered verification") spend characters that could go to negatives. The router doesn't need to know step 7 is the TDD gate.
- **Fix**: Trim trigger phrases to 3 representative ones. Drop the workflow-internals clause. Reclaim ~200 chars; spend on stronger negatives (P0-1).

### P1-7 — Mux *because* clause is weak
- **Location**: line 227
- **Anthropic rule**: **[SC §"Improving the skill" L302: "Try hard to explain the why behind everything you're asking the model to do … if possible, reframe and explain the reasoning so that the model understands why the thing you're asking for is important."]** Other *because* clauses in the section earn their keep; this one doesn't.
- **Issue**: "muxing changes the migration shape entirely (incremental over many releases vs the single-release scope this skill targets)" restates the scope rather than naming the failure mode. The reader can't judge edge cases from "this skill targets a different scope."
- **Fix**: Strengthen to: *"muxing routes some resources to SDKv2 and some to framework simultaneously; this skill's audit and verification gates assume a single-server tree and will produce false-greens on the SDKv2-routed half."*

### P1-8 — Inconsistent script-path notation
- **Location**: lines 14, 21, 50, 191, 55
- **Anthropic rule**: **[BP §"Use consistent terminology": "Choose one term and use it throughout the Skill … Consistency helps Claude understand and follow instructions."]** Path notation is terminology in this sense.
- **Issue**: Mixes bare `audit_sdkv2.sh` with `<skill-path>/scripts/audit_sdkv2.sh`. Bare names are ambiguous — cwd-relative or skill-relative? An LLM that pastes the bare form into a real session may invoke the wrong path.
- **Fix**: Use `<skill-path>/scripts/<name>.sh` (and `<skill-path>/assets/...`) on first reference for each path; bare names acceptable in subsequent prose.

### P1-9 (new) — Description not "pushy" per Anthropic guidance
- **Location**: line 3
- **Anthropic rule**: **[SC L67: "Currently Claude has a tendency to 'undertrigger' skills — to not use them when they'd be useful. To combat this, please make the skill descriptions a little bit 'pushy'. So for instance, instead of 'How to build a simple fast dashboard…', you might write 'Make sure to use this skill whenever the user mentions dashboards, data visualization, internal metrics, or wants to display any kind of company data, even if they don't explicitly ask for a dashboard.'"]**
- **Issue**: The current description leads with "Use this skill whenever the user wants to migrate, port, or upgrade…" which is *almost* pushy but immediately defuses with a parenthetical "even if they don't use the word 'migrate'" — fine — and then drifts into capability description ("Drives the canonical 12-step single-release-cycle migration workflow…"). The router-relevant pushy framing is diluted.
- **Fix**: After P0-1 + P1-6 trims, the lead sentence has room for a stronger pushy clause like *"Use this skill whenever a user wants to move any Terraform resource, data source, or whole provider off `terraform-plugin-sdk/v2`, even if they say 'rewrite', 'port', 'convert', or describe the work without naming the SDKs explicitly."*

---

## P2 — polish

### P2-1 — Description claims 12-step but actual sequence is 14
- **Location**: line 3 vs lines 41–94
- **Anthropic rule**: **[BP §"Use consistent terminology"]**
- **Fix**: "12-step single-release-cycle workflow with two pre-flight steps (audit + plan)".

### P2-2 — Defensive / motivational prose
- **Location**: line 10 (mechanical-but-error-prone paragraph), line 43 (scaffolding-not-competing-scheme)
- **Anthropic rule**: **[BP §"Default assumption: Claude is already very smart": "Only add context Claude doesn't already have. Challenge each piece of information: 'Does Claude really need this explanation?' 'Can I assume Claude knows this?' 'Does this paragraph justify its token cost?'"]** Both passages fail this test — they justify the *design* of the skill rather than instructing the model.
- **Fix**: Cut line 10 to one sentence or delete; delete line 43 entirely — the workflow itself communicates the design.

### P2-3 — `inventory_artefact_shape` example is wordy
- **Location**: lines 61–71
- **Anthropic rule**: **[BP §"Concise is key"]** — token cost vs information density.
- **Fix**: Compress to a 4-line bulleted size-budget table. Move the rationale ("the audit script is over-firing…") to a footnote or drop.

### P2-4 — XML tag use is inconsistent
- **Location**: lines 83, 184, 207, 221, 61, 128
- **Anthropic rule**: **[BP §"Use consistent terminology"]** + **[OV: "description: Cannot contain XML tags"]** (the limit is on the description, but consistent tagging in the body still aids extraction).
- **Issue**: `<workflow_step>`, `<verification_gates>`, `<common_pitfalls>`, `<never_do>`, `<example>` are applied to some sections but not others of equal importance (the 12-step list, "Think before editing" gate, the reference index).
- **Fix**: Either wrap all gate-class sections or none. The `<example>` tag clearly adds extraction value — preserve it.

### P2-5 — "Think before editing" name drift
- **Location**: lines 174–182
- **Anthropic rule**: **[BP §"Use consistent terminology"]**
- **Fix**: Rename to "Pre-flight C — per-resource think pass" (or "Pre-edit summary") and move directly after Pre-flight B for voice/structure consistency.

### P2-6 — "Per-element conversion" heading hides the lookup table
- **Location**: line 98
- **Anthropic rule**: **[BP §"Structure longer reference files with table of contents": "For reference files longer than 100 lines, include a table of contents at the top. This ensures Claude can see the full scope of available information."]** SKILL.md is over 100 lines, and this table is effectively its TOC.
- **Fix**: Rename to `## Reference index — open on demand` so an LLM searching for "where do I find validators" hits it.

### P2-7 — "Resource identity" row bolded; nothing else is
- **Location**: line 116
- **Anthropic rule**: **[BP §"Use consistent terminology"]**
- **Fix**: De-bold. If load-bearing, add a one-line note above the table — don't drift formatting within a uniform table.

### P2-8 — Verification gate 3 (`TestProvider`) silently skipped if absent
- **Location**: lines 198 (gate 3 description)
- **Anthropic rule**: **[BP §"Solve, don't punt": "Handle error conditions rather than punting to Claude."]** Silent skips punt the gate-skip decision to the reader.
- **Fix**: Add: "If skipped, note in the per-resource checklist row that `InternalValidate` was not exercised; consider adding a minimal `TestProvider` test."

### P2-9 — Step 7 4-substep procedure could be pushed to `references/workflow.md`
- **Location**: lines 83–89
- **Anthropic rule**: **[BP §"Avoid deeply nested references": "Keep references one level deep from SKILL.md."]** Already one-level, so this is acceptable as-is — but **[BP §"Concise is key"]** suggests pushing procedure detail down once load-bearing framing stays inline.
- **Fix**: Optional. Defer until after P0/P1 work — reduces tokens but the inline procedure is also genuinely useful where it sits.

### P2-10 — Name not in preferred gerund form *(partial follow-up: provider-prefix added)*
- **Location**: line 2
- **Anthropic rule**: **[BP §"Naming conventions": "Consider using gerund form (verb + -ing) for Skill names, as this clearly describes the activity or capability the Skill provides. Good naming examples (gerund form): processing-pdfs, analyzing-spreadsheets, managing-databases."]** The doc explicitly lists `pdf-processing` (noun phrase) as an "Acceptable alternative", so the current name is *acceptable* — but not preferred.
- **Issue**: `sdkv2-to-framework` is a noun phrase. Per Anthropic's preference, gerund form would trigger and disambiguate better, especially against any peer skill named `framework-migration` (Spring/Vue/Django/etc.) — the "terraform" anchor is also missing.
- **Original fix proposal**: Optional rename to `migrating-terraform-sdkv2` or `migrating-sdkv2-to-framework`. Costs a directory rename and any path references — defer until other fixes are settled.
- **Follow-up taken**: skill renamed to `provider-sdkv2-to-framework` to scope it to the Terraform-provider domain (the originally-missing anchor) without committing to a gerund form. The gerund-form rename remains open as a future refinement if this skill ever competes with another `provider-*` migration skill.

### P2-11 (new) — No copy-able workflow checklist
- **Location**: lines 41–94 (12 + 2 pre-flight steps)
- **Anthropic rule**: **[BP §"Use workflows for complex tasks": "Break complex operations into clear, sequential steps. For particularly complex workflows, provide a checklist that Claude can copy into its response and check off as it progresses."]** The Anthropic doc shows a literal `Task Progress: - [ ] Step 1…` block as the recommended pattern.
- **Issue**: This skill's 14-step migration is exactly what Anthropic flags as warranting a checklist. The current narrative listing makes it easy for the model to skip a step or lose its place between resources.
- **Fix**: Add a copy-able progress block at the start of the Workflow section:

  ```
  Migration progress (copy and check off):
  - [ ] Pre-flight A: audit_sdkv2.sh complete
  - [ ] Pre-flight B: checklist populated, scope confirmed
  - [ ] Step 1: SDKv2 baseline tests green
  - [ ] Step 2: data-consistency review complete
  ...
  - [ ] Step 12: release
  ```

  This is the exact pattern Anthropic recommends for fragile, multi-step workflows.

---

## Recommended fix order

1. **P0-1 + P0-2 + P1-6 + P1-9** in one pass — all touch the description and mux gate. ~10 minutes, biggest reliability win, all rooted in **[BP §"Writing effective descriptions"]** + **[SC L67]**.
2. **P1-2** (reorder sections), **P2-2** (cut defensive prose), **P2-5** (rename "Think before editing"), **P2-6** (rename reference-table heading) — also one pass; structural pruning per **[BP §"Concise is key"]**.
3. **P1-1** (push `MaxItems_1` worked example into `references/blocks.md`) + **P1-4** (add state-upgrader worked example) + **P2-11** (copy-able checklist). Net ~30 lines saved, judgment coverage broadened, and brings the file in line with the **[BP §"Use workflows"]** pattern.
4. **P0-3** (step-7 acceptance signatures) + **P1-3** (4 missing pitfalls) + **P1-5** (reference table fixes) + **P1-7** (mux *because*) + **P1-8** (script paths). Content correctness pass.
5. **P2 remainder** — polish.

After steps 1–4 the file should be ~170 lines, more triggering-precise, and conform tightly to Anthropic's documented patterns.

---

## Appendix A — Anthropic rules cited verbatim

### From Anthropic best-practices doc **[BP]**

- **Concise is key**: *"The context window is a public good. … Once Claude loads SKILL.md, every token competes with conversation history and other context."*
- **Default assumption: Claude is already very smart**: *"Only add context Claude doesn't already have. Challenge each piece of information: 'Does Claude really need this explanation?' 'Can I assume Claude knows this?' 'Does this paragraph justify its token cost?'"*
- **Set appropriate degrees of freedom**: *"Match the level of specificity to the task's fragility and variability. … Low freedom (specific scripts, few or no parameters): Use when operations are fragile and error-prone, consistency is critical, a specific sequence must be followed."*
- **Naming conventions**: *"Consider using gerund form (verb + -ing) for Skill names. … Good: `processing-pdfs`, `analyzing-spreadsheets`. Acceptable alternatives: noun phrases like `pdf-processing`. Avoid: vague names, overly generic, reserved words."*
- **Writing effective descriptions** (warning box): *"Always write in third person. The description is injected into the system prompt, and inconsistent point-of-view can cause discovery problems."*
- **Writing effective descriptions**: *"Be specific and include key terms. Include both what the Skill does and specific triggers/contexts for when to use it. … The description is critical for skill selection: Claude uses it to choose the right Skill from potentially 100+ available Skills."*
- **Progressive disclosure patterns**: *"SKILL.md serves as an overview that points Claude to detailed materials as needed, like a table of contents in an onboarding guide. Keep SKILL.md body under 500 lines."*
- **Avoid deeply nested references**: *"Keep references one level deep from SKILL.md. All reference files should link directly from SKILL.md to ensure Claude reads complete files when needed."*
- **Structure longer reference files with table of contents**: *"For reference files longer than 100 lines, include a table of contents at the top."*
- **Use workflows for complex tasks**: *"Break complex operations into clear, sequential steps. For particularly complex workflows, provide a checklist that Claude can copy into its response and check off as it progresses."*
- **Implement feedback loops**: *"Common pattern: Run validator → fix errors → repeat. This pattern greatly improves output quality."*
- **Avoid time-sensitive information**: *"Don't include information that will become outdated. … Use 'old patterns' section to provide historical context without cluttering the main content."*
- **Use consistent terminology**: *"Choose one term and use it throughout the Skill … Consistency helps Claude understand and follow instructions."*
- **Avoid offering too many options**: *"Don't present multiple approaches unless necessary. Provide a default with an escape hatch."*
- **Examples pattern**: *"For Skills where output quality depends on seeing examples, provide input/output pairs. … Examples help Claude understand the desired style and level of detail more clearly than descriptions alone."*
- **Solve, don't punt**: *"Handle error conditions rather than punting to Claude."*
- **Token budgets**: *"Keep SKILL.md body under 500 lines for optimal performance."*
- **Avoid Windows-style paths**: *"Always use forward slashes in file paths."*

### From Anthropic skills overview **[OV]**

- **YAML frontmatter requirements**:
  - `name`: max 64 chars, lowercase letters / numbers / hyphens only, no XML tags, no reserved words ("anthropic", "claude").
  - `description`: max 1024 chars, non-empty, no XML tags.

### From `skill-creator` skill **[SC]**

- **L67 (description guidance)**: *"Note: currently Claude has a tendency to 'undertrigger' skills — to not use them when they'd be useful. To combat this, please make the skill descriptions a little bit 'pushy'. So for instance, instead of 'How to build a simple fast dashboard…', you might write 'Make sure to use this skill whenever the user mentions dashboards, data visualization, internal metrics, or wants to display any kind of company data, even if they don't explicitly ask for a dashboard.'"*
- **L139 (writing style)**: *"Try to explain to the model why things are important in lieu of heavy-handed musty MUSTs. Use theory of mind and try to make the skill general and not super-narrow to specific examples."*
- **L302 (improving — explain the why)**: *"Try hard to explain the why behind everything you're asking the model to do. … If you find yourself writing ALWAYS or NEVER in all caps, or using super rigid structures, that's a yellow flag — if possible, reframe and explain the reasoning so that the model understands why the thing you're asking for is important."*
- **L398 (how triggering works)**: *"Skills appear in Claude's `available_skills` list with their name + description, and Claude decides whether to consult a skill based on that description. … Complex, multi-step, or specialized queries reliably trigger skills when the description matches."*
