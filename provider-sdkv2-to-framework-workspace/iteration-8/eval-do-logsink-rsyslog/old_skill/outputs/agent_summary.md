# Migration Summary: resource_database_logsink_rsyslog

## What was migrated

Migrated `resource_database_logsink_rsyslog.go` from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The test file was updated to use `terraform-plugin-testing` and `ProtoV6ProviderFactories`.

## Pre-migration analysis

**Block decision**: No `MaxItems:1` nested `Elem: &schema.Resource` patterns — all attributes are flat primitives. No block conversion needed.

**State upgrade**: No `SchemaVersion > 0` or `StateUpgraders`. No state upgrade required.

**Import shape**: Composite-ID importer (`cluster_id,logsink_id` comma-separated). Migrated to `ImportState` method that calls `resp.State.SetAttribute` for `id`, `cluster_id`, and `logsink_id` so `Read` can fetch the resource. The original `splitLogsinkID` helper was renamed to `splitLogsinkIDFW` to avoid symbol collision if both files briefly coexist.

**CustomizeDiff**: `customdiff.All(...)` had two legs:
1. `customdiff.ForceNewIfChange("name", ...)` — **dropped** because `name` already carries `stringplanmodifier.RequiresReplace()` in the schema, making this leg redundant.
2. `validateLogsinkCustomDiff(diff, "rsyslog")` — **migrated to `ModifyPlan`**: validates that `logline` is set when `format == "custom"`, that `tls` is true when any cert field is set, and that `client_cert`/`client_key` are both set or both unset for mTLS.

## Key decisions

| Concern | Decision |
|---|---|
| `ForceNew: true` on `cluster_id`, `name` | `stringplanmodifier.RequiresReplace()` in schema |
| `Default: false` on `tls` | `booldefault.StaticBool(false)` + `Computed: true` |
| `Default: "rfc5424"` on `format` | `stringdefault.StaticString("rfc5424")` + `Computed: true` |
| `validateLogsinkPort` (1–65535) | `int64validator.Between(1, 65535)` |
| `validateRsyslogFormat` (enum) | `stringvalidator.OneOf("rfc5424", "rfc3164", "custom")` |
| `validation.NoZeroValues` on strings | `stringvalidator.LengthAtLeast(1)` |
| `CustomizeDiff` cross-field checks | `ModifyPlan` method (short-circuits on destroy via `req.Plan.Raw.IsNull()`) |
| Sensitive `ca_cert`, `client_key` | Carried forward from prior state when API does not echo them back |
| `id` (computed composite) | `UseStateForUnknown()` plan modifier |
| `logsink_id` (computed API sink ID) | `UseStateForUnknown()` plan modifier |

## SDKv2 references eliminated

- `github.com/hashicorp/terraform-plugin-sdk/v2/diag`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/customdiff`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema`
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation`
- Test file: `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` → `github.com/hashicorp/terraform-plugin-testing/helper/resource`
- Test file: `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` → `github.com/hashicorp/terraform-plugin-testing/terraform`

## New framework interfaces implemented

- `resource.Resource` (Metadata, Schema, Create, Read, Update, Delete)
- `resource.ResourceWithConfigure` (Configure — wires `*godo.Client`)
- `resource.ResourceWithImportState` (ImportState — composite ID parser)
- `resource.ResourceWithModifyPlan` (ModifyPlan — replaces CustomizeDiff)

## Test changes

- `ProviderFactories: acceptance.TestAccProviderFactories` → `ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories`
- `resource "github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"` → `"github.com/hashicorp/terraform-plugin-testing/helper/resource"`
- `terraform "github.com/hashicorp/terraform-plugin-sdk/v2/terraform"` → `"github.com/hashicorp/terraform-plugin-testing/terraform"`
- `testAccCheckDigitalOceanDatabaseLogsinkExists` and `testAccCheckDigitalOceanDatabaseLogsinkAttributes` were inlined/renamed to rsyslog-specific helpers to avoid cross-file dependencies with the still-SDKv2 opensearch test file
- `testAccCheckDigitalOceanDatabaseLogsinkDestroy` scoped to `digitalocean_database_logsink_rsyslog` only
- Added `TestAccDigitalOceanDatabaseLogsinkRsyslog_Import` step exercising `ImportState: true` + `ImportStateVerify: true` with `ImportStateIdFunc` using the composite ID

## Files produced

- `migrated/resource_database_logsink_rsyslog.go` — zero SDKv2 imports; implements ImportState and ModifyPlan; no CustomizeDiff field; valid Go
- `migrated/resource_database_logsink_rsyslog_test.go` — uses terraform-plugin-testing; ProtoV6ProviderFactories
