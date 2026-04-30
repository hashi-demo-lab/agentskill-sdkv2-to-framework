# Resource import

## Quick summary
- SDKv2 `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` becomes the resource type implementing `resource.ResourceWithImportState`.
- The simplest case (ID is the primary identifier): use `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`.
- Custom `Importer.StateContext` parsing composite IDs (`region/resource_id`) becomes manual `path.Root` setting in `ImportState`.
- The import method runs *before* `Read`, so you can't fetch from the API yet; just parse the ID into state fields and let `Read` populate the rest.
- Multi-resource imports (one import call seeding multiple resources) are out of scope — the framework supports it but it's rare; refer to HashiCorp docs if you need it.

## Fetch for depth

This file is a navigational stub. The 5-bullet summary above captures the *decisions* the skill needs you to make. For full conversion tables, code examples, and authoritative type signatures, fetch one of the URLs below as needed:

- [Import migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating/import)
- [Import state helpers source](https://github.com/hashicorp/terraform-plugin-framework/blob/main/resource/import_state.go)

Default: read the relevant HashiCorp `migrating/*` page first; fall back to framework source for fields/methods/signatures the docs don't pin down.
