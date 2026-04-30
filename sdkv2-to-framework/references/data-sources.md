# Data source migration

## Quick summary
- Data sources go from `*schema.Resource` (with the same shape as resources but only `ReadContext`) to a Go type implementing `datasource.DataSource`.
- Required methods: `Metadata`, `Schema`, `Read`. Optional: `Configure`, `ConfigValidators`, `ValidateConfig`.
- The schema package is `github.com/hashicorp/terraform-plugin-framework/datasource/schema` — do **not** import the resource package by mistake.
- Data sources have no `Create`/`Update`/`Delete`, no plan modifiers, and no state upgraders — strictly read-only.
- Like resources, all I/O is typed through a `tfsdk:"..."` model struct.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Data source migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/data-sources)
- [Ephemeral resources (modern alternative for credentials)](https://developer.hashicorp.com/terraform/plugin/framework/ephemeral-resources)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
