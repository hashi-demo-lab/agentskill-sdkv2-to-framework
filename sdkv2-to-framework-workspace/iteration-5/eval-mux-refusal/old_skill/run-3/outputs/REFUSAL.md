# Refusal: terraform-plugin-mux multi-release migration is out of scope

## Decision

I am declining to drive this task with the `sdkv2-to-framework` skill.

## Why

Per `SKILL.md` (skill-snapshot-i4), this skill targets **HashiCorp's
single-release-cycle migration workflow only**. The user's request explicitly
asks for the opposite shape:

- "off SDKv2 using **terraform-plugin-mux**"
- "**staged, multi-release migration**"
- "ship a few resources on the framework while keeping the rest on SDKv2"
- "across **two release cycles**"

The skill's "Before triggering, sanity-check the user's wording" guard lists
exactly these triggers — *mux*, *muxed*, *staged migration*, *two-release*,
*multi-release*, *terraform-plugin-mux* — and instructs me to **stop** rather
than apply the single-release workflow. The skill's "Does NOT apply" section
re-confirms: `terraform-plugin-mux` multi-release migrations are explicitly
not covered.

The "What to never do" section reinforces the same boundary:

> **Don't introduce `terraform-plugin-mux`.** *Because* muxing changes the
> migration shape entirely (incremental over many releases vs the
> single-release scope this skill targets); applying single-release patterns
> to a muxed provider creates an inconsistent half-state.

Applying the 12-step single-release workflow, the audit script, the TDD gate,
and `verify_tests.sh`'s "no remaining SDKv2 imports" negative gate would all
be wrong for a muxed provider that is *intentionally* keeping SDKv2 imports
across release cycles. The bundled references assume the SDKv2 code is being
deleted, not preserved alongside framework code behind a mux server.

## Where to go instead

HashiCorp publishes the canonical guide for the muxed, multi-release path:

- **Combining and translating providers** (terraform-plugin-mux):
  https://developer.hashicorp.com/terraform/plugin/mux
- **Mux a framework provider with an SDKv2 provider** (the `tf5muxserver` /
  `tf6muxserver` patterns, including protocol-version matching):
  https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-5-providers
  and
  https://developer.hashicorp.com/terraform/plugin/mux/translating-protocol-version-5-to-6
- **Migration overview — multi-release option**:
  https://developer.hashicorp.com/terraform/plugin/framework/migrating

The high-level shape the user is asking about (and which they should follow
from those docs, not from this skill) is:

1. Add `terraform-plugin-framework` and `terraform-plugin-mux` as
   dependencies alongside the existing SDKv2 provider.
2. Build a new framework `provider.Provider` whose `Resources()` /
   `DataSources()` lists are initially empty.
3. In `main.go`, wrap both providers with `tf6muxserver.NewMuxServer` (or
   `tf5muxserver` if staying on protocol v5 for now; mux requires both halves
   to speak the same protocol version, so `tf5to6server.UpgradeServer` may be
   needed on the SDKv2 side first).
4. Per release cycle, move a small batch of resources/data sources from the
   SDKv2 provider's `ResourcesMap` into the framework provider's
   `Resources()`. Mux ensures Terraform sees a single merged provider.
5. When the SDKv2 side is empty, delete it and drop the mux layer.

The key constraints not covered by *this* skill — and which the user must
get right from HashiCorp's mux docs — are:

- **No duplicate type names.** A given resource/data-source type name can
  only be served by one of the two providers at any time; mux will refuse to
  start otherwise.
- **Protocol-version alignment.** Both halves must serve the same protocol.
  Picking v6 from day one (via `tf5to6server` on the SDKv2 half) keeps
  options open for v6-only framework features (e.g. nested attributes that
  v5 cannot represent).
- **Schema parity during the transition.** A resource that lives on the
  framework side in release N must produce *byte-identical* state to its
  SDKv2 form in release N-1, or practitioners will see drift on upgrade.
  Per-resource state-upgrader work (the framework's single-step upgraders)
  may still be needed even though the *provider* is muxed rather than
  cut-over.
- **Test factory wiring.** Acceptance tests for migrated resources move to
  `ProtoV6ProviderFactories` pointing at the *muxed* server, not the
  framework provider in isolation — otherwise the tests don't exercise what
  practitioners actually run.

These are the points where mux migrations most commonly go wrong. They are
genuinely different from the single-release failure modes this skill is
tuned for, which is why the right answer is to send the user to the
upstream docs rather than improvise a partial application of this skill.

## What I did NOT do

- Did not run `scripts/audit_sdkv2.sh` against the openstack provider.
- Did not produce `audit_report.md`, `migration_checklist.md`, or any
  per-resource plan.
- Did not edit any file under `/Users/simon.lynch/git/terraform-provider-openstack`.
- Did not run `go build`, `go vet`, `go test`, or `go mod tidy` (per the
  task's read-only constraint, and because they would not be meaningful
  without a migration in progress).
