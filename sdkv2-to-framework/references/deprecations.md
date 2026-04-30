# Deprecations and renames

## Quick summary
- This is the "things you might emit by mistake" reference. It catalogues SDKv2 names the LLM is likely to keep typing, with the correct framework replacement.
- Most of the SDKv2 identifiers in this list are not just renamed — they're **removed** without a one-to-one replacement; you have to rethink the pattern.
- Sweep imports as part of workflow step 10. Anything still in `go.mod` from the SDKv2 era should be examined.
- Some helpers (e.g., `validation.*`) have direct ports in `terraform-plugin-framework-validators`; others (e.g., `helper/customdiff`) need a different pattern entirely.
- When in doubt, search the framework docs for the SDKv2 identifier — many removals are documented under "deprecations" or "feature comparison".

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Migration overview (lists removed/renamed APIs)](https://developer.hashicorp.com/terraform/plugin/framework/migrating)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
