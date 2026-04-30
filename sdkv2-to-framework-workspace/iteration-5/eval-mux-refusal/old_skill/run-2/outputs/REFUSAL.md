# Refusal — out of scope for this skill

## What you asked for

> Migrate `terraform-provider-openstack` off SDKv2 using `terraform-plugin-mux` for a staged, multi-release migration — set up the muxed provider so a few resources ship on the framework while the rest stay on SDKv2 across two release cycles.

## Why this skill is declining

The `sdkv2-to-framework` skill explicitly drives HashiCorp's **single-release-cycle** migration workflow. Your request hits multiple explicit out-of-scope triggers in the skill's own gating rules:

- **"mux" / "muxed" / "terraform-plugin-mux"** — the skill's `When this skill applies` section says: *"If the request mentions any of mux, muxed, muxing, staged migration, two-release, multi-release, or terraform-plugin-mux — stop."*
- **"staged, multi-release migration"** — same rule.
- **"two release cycles"** — same rule.
- **"ship a few resources on the framework while keeping the rest on SDKv2"** — that *is* a muxed provider by definition; the skill's `Does NOT apply` list calls out *"`terraform-plugin-mux` multi-release migrations: out of scope for this skill"*.

The reasoning (from the skill's `What to never do`): muxing changes the migration shape entirely — it's incremental across many releases rather than a single-release cutover. Applying single-release patterns (audit-once, full TDD gate at step 7, "remove all SDKv2 references" at step 10, the `verify_tests.sh` negative gate that forbids any `terraform-plugin-sdk/v2` import in migrated files) to a *muxed* provider produces an inconsistent half-state: those gates are designed to fail closed on a tree where SDKv2 is supposed to be gone, but in your design SDKv2 is intentionally still present for the resources you haven't moved yet.

## Where to go instead

HashiCorp publishes a dedicated workflow for exactly your case. Read these in order:

1. **Combining and translating SDKs / muxing overview** — https://developer.hashicorp.com/terraform/plugin/mux
   Explains why mux exists, the two muxers (`tf5muxserver`, `tf6muxserver`), and the protocol-version constraints (all muxed servers must speak the same protocol; downgrade from v6 → v5 with `tf5to6server` / upgrade with `tf6to5server` if needed).

2. **Translating protocol versions** — https://developer.hashicorp.com/terraform/plugin/mux/translating-protocol-version-5-to-6 and the v6→v5 counterpart.
   Relevant because `terraform-plugin-sdk/v2` serves protocol v5 by default and the framework defaults to v6; you'll pick one protocol for the muxed surface and translate the other.

3. **Combining framework and SDKv2 providers** — https://developer.hashicorp.com/terraform/plugin/framework/migrating/mux
   The canonical recipe for *exactly* what you described: keep the SDKv2 `*schema.Provider` for unmigrated resources, stand up a `provider.Provider` (framework) for migrated ones, mux them behind a single `main.go`, and move resources across over multiple releases. This is the doc you want.

4. **Per-resource migration mechanics inside the muxed provider** — once mux is wired, the *content* of each resource's port (schema, CRUD, validators, plan modifiers, state upgraders, import) is the same work this skill's references cover. If you (separately, after mux is in place) want to migrate one specific resource and ask without the mux framing, the per-element references (`schema.md`, `resources.md`, `state-upgrade.md`, etc.) apply to that resource in isolation.

## What I did NOT do

To stay honest about scope:

- I did **not** read or modify any file under `/Users/simon.lynch/git/terraform-provider-openstack`.
- I did **not** run `audit_sdkv2.sh`, `verify_tests.sh`, or any build/test/vet/mod command.
- I did **not** produce an audit report, a checklist, or any `main.go` scaffolding for a muxed provider.
- I did **not** infer which resources you'd ship first — that's a product/release decision the mux docs walk you through.

## If you actually want the single-release path instead

If on reflection you'd rather do the **whole-provider, single-release** migration this skill *does* cover — i.e., cut over all of `terraform-provider-openstack` from SDKv2 to the framework in one release, no mux — re-ask without the mux/staged/multi-release framing and I'll run the audit and produce the migration plan.
