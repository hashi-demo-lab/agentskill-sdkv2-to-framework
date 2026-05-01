# Migration Summary: digitalocean_database_user

## Sensitive-vs-WriteOnly Decision

The `password` attribute is `Sensitive: true` + `Computed: true`. **WriteOnly was NOT applied.**

Decision rationale (from `references/sensitive-and-writeonly.md`):
- Ask: *does Terraform need to read this value back later?* **Yes.**
- The DigitalOcean API returns the password in the `CreateUser` response.
- Terraform stores it in state so downstream resources can reference `digitalocean_database_user.x.password`.
- `WriteOnly: true` is mutually exclusive with `Computed: true` (the framework rejects the combination at boot via `ValidateImplementation`).
- Even setting aside the hard rule, WriteOnly would break cross-resource references and ImportStateVerify assertions.
- `Sensitive: true` alone is correct: the value is redacted from plan output/logs but still stored in state.

The same reasoning applies to `access_cert` and `access_key` (also `Sensitive: true` + `Computed: true`, API-returned values for Kafka users).

## Schema Decisions

| Attribute | SDKv2 | Framework | Notes |
|---|---|---|---|
| `password` | `Computed: true, Sensitive: true` | `Computed: true, Sensitive: true` | Correct. NOT WriteOnly. |
| `access_cert` | `Computed: true, Sensitive: true` | `Computed: true, Sensitive: true` | Same pattern as password. |
| `access_key` | `Computed: true, Sensitive: true` | `Computed: true, Sensitive: true` | Same pattern as password. |
| `name` | `ForceNew: true` | `RequiresReplace()` plan modifier | Standard conversion. |
| `cluster_id` | `ForceNew: true` | `RequiresReplace()` plan modifier | Standard conversion. |
| `role` | `Computed: true` | `Computed: true` + `UseStateForUnknown` | Prevents spurious unknown in plans. |
| `mysql_auth_plugin` | `Optional + DiffSuppressFunc` | `Optional + Computed + UseStateForUnknown` + `ModifyPlan` | See below. |
| `settings` | `TypeList, Elem: &schema.Resource` (no MaxItems) | `ListNestedBlock` | Kept as block to preserve HCL syntax. |
| `settings.acl` | `TypeList, Elem: &schema.Resource` | `ListNestedBlock` inside settings | Kept as block. |
| `settings.opensearch_acl` | `TypeList, Elem: &schema.Resource` | `ListNestedBlock` inside settings | Kept as block. |

## DiffSuppressFunc Migration

The SDKv2 resource had a `DiffSuppressFunc` on `mysql_auth_plugin`:
```go
func(k, old, new string, d *schema.ResourceData) bool {
    return old == godo.SQLAuthPluginCachingSHA2 && new == ""
}
```

This is migrated to two mechanisms:
1. `mysql_auth_plugin` is `Optional + Computed` so omitting it from config doesn't show a diff (framework fills from state).
2. A `ModifyPlan` method carries the state value forward explicitly when state has `caching_sha2_password` and the plan is null/unknown/empty.

## Import

The SDKv2 importer parsed a comma-separated `clusterID,name` format (not the standard passthrough). This is preserved in the framework `ImportState` method using `resp.State.SetAttribute` to set `id`, `cluster_id`, and `name` individually, then `Read` populates the rest.

No `ImportStateVerifyIgnore` is needed — `password` is Sensitive (not WriteOnly) and is present in state, so import-verify works normally.

## Test Changes

- `ProviderFactories` → `ProtoV6ProviderFactories` (pointing to `acceptance.TestAccProtoV6ProviderFactories`)
- Import: `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` → `github.com/hashicorp/terraform-plugin-testing/helper/resource`
- Import: `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` → `github.com/hashicorp/terraform-plugin-testing/terraform`
- All `resource.TestCheckResourceAttrSet("...", "password")` assertions are preserved unchanged — password is in state (Sensitive, not WriteOnly), so the assertion works.
- Variable `config` renamed to `cfg` in `TestAccDigitalOceanDatabaseUser_MongoDBMultiUser` to avoid shadowing the `config` package import.

## SDKv2 References Removed

No imports of `github.com/hashicorp/terraform-plugin-sdk/v2` remain in either output file.
