# Refusal — terraform-plugin-mux multi-release migration request

## Request summary

The user asked to set up a **muxed provider** using `terraform-plugin-mux` to ship a few resources on terraform-plugin-framework while keeping the rest on SDKv2 across **two release cycles** (a staged, multi-release migration strategy).

## Why this is out of scope

This request is explicitly out of scope for the `sdkv2-to-framework` skill. The skill's SKILL.md "Does NOT apply" section states:

> **`terraform-plugin-mux` multi-release migrations**: out of scope for this skill; the user wants the single-release path.

Additionally, the "When this skill applies" section includes a hard stop:

> Before triggering, sanity-check the user's wording. If the request mentions any of *mux*, *muxed*, *muxing*, *staged migration*, *two-release*, *multi-release*, or *terraform-plugin-mux* — **stop**. That's the multiplexed-migration path which is explicitly out of scope.

This request contains multiple trigger terms: **terraform-plugin-mux**, **muxed provider**, **staged**, and **multi-release migration**. The skill does not cover this scenario.

## What the skill does cover

The `sdkv2-to-framework` skill covers **single-release-cycle** migrations only — moving an entire provider (or a scoped set of resources) from SDKv2 to terraform-plugin-framework in one release, following HashiCorp's canonical 12-step workflow. It does not introduce `terraform-plugin-mux`, which changes the migration shape entirely (incremental over many releases vs. the single-release scope this skill targets).

## Where to get help

For the `terraform-plugin-mux` multi-release migration path, refer to HashiCorp's official documentation:

- **Mux overview and setup**: https://developer.hashicorp.com/terraform/plugin/mux
- **Combining protocol version 5 and 6 providers**: https://developer.hashicorp.com/terraform/plugin/mux/combining-protocol-version-5-providers
- **Migrating resources incrementally with mux**: https://developer.hashicorp.com/terraform/plugin/framework/migrating/mux
- **`terraform-plugin-mux` GitHub repository**: https://github.com/hashicorp/terraform-plugin-mux

The mux approach lets you serve some resources from the framework and others from SDKv2 within a single provider binary, enabling a staged migration across multiple release cycles — which is precisely what the user is asking for.

## Outcome

- No migration files were produced.
- The terraform-provider-openstack clone at `<openstack-clone>` was not modified.
- No `migrated/` directory was created.
