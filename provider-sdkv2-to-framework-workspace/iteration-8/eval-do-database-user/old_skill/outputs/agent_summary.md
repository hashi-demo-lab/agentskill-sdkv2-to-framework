# Migration summary — digitalocean_database_user

## Skill version
i7 baseline (skill-snapshot-i7)

## What was migrated
`digitalocean/database/resource_database_user.go` and its test file from terraform-plugin-sdk/v2 to terraform-plugin-framework.

## Key decisions

### password — Sensitive: true, Computed: true, WriteOnly: false
The SDKv2 resource had `password` as `Computed: true, Sensitive: true`. The API returns the password on `CreateUser` (and on `GetUser` for most engines except MongoDB which omits it post-create). Terraform must store the value in state to support:
- drift detection across subsequent plans
- cross-resource references (`var.password = digitalocean_database_user.x.password`)
- `terraform show` (redacted but present)
- import round-trip verification

WriteOnly was therefore **not** applied. The `sensitive-and-writeonly.md` reference confirms: WriteOnly is only correct when Terraform does not need to read the value back. `UseStateForUnknown` was added so the password does not show as `(known after apply)` on every plan.

### mysql_auth_plugin — DiffSuppressFunc replaced with Default
The SDKv2 `DiffSuppressFunc` suppressed diffs when old=`caching_sha2_password` and new=`""`. This is semantically equivalent to a default: if the practitioner omits the field, the API applies `caching_sha2_password`. Migrated to `Optional: true, Computed: true, Default: stringdefault.StaticString(godo.SQLAuthPluginCachingSHA2)`.

### settings block — kept as ListNestedBlock
`settings` is a true repeating block (no `MaxItems: 1`) with nested `acl` and `opensearch_acl` sub-blocks. Practitioner configs use block syntax (`settings { acl { ... } }`). Converting to a nested attribute would break HCL syntax for existing users. Both outer and inner levels stay as `ListNestedBlock`.

### Import — composite-ID parsing
The SDKv2 importer parsed `cluster_id,name` from a comma-separated string. Migrated to `ImportState` method using `resp.State.SetAttribute` for `id`, `cluster_id`, and `name`. The `Read` method then populates the rest.

### access_cert / access_key
Both are `Computed: true, Sensitive: true` (Kafka-only API fields). Not WriteOnly because they need to round-trip through state for drift detection.

## Schema changes from SDKv2
- All `ForceNew: true` → `PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}`
- All `ValidateFunc: validation.NoZeroValues` → `Validators: []validator.String{stringvalidator.LengthAtLeast(1)}`
- `ValidateFunc: validation.StringInSlice(...)` → `Validators: []validator.String{stringvalidator.OneOf(...)}`
- Computed fields gained `UseStateForUnknown` plan modifiers

## Test file changes
- `github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource` → `github.com/hashicorp/terraform-plugin-testing/helper/resource`
- `github.com/hashicorp/terraform-plugin-sdk/v2/terraform` → `github.com/hashicorp/terraform-plugin-testing/terraform`
- `ProviderFactories: acceptance.TestAccProviderFactories` → `ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories`
- Local variable `config` renamed to `cfg` in `TestAccDigitalOceanDatabaseUser_MongoDBMultiUser` to avoid shadowing the imported `config` package

## What was NOT changed
- Resource type name (`digitalocean_database_user`) — unchanged
- All attribute names — unchanged (state-breaking if changed)
- `makeDatabaseUserID` format — unchanged (import ID compatibility)
- HCL syntax for `settings`, `acl`, `opensearch_acl` blocks — unchanged (backward-compat)
- `normalizePermission` / `normalizeOpenSearchPermission` helpers — unchanged logic

## SDKv2 references remaining
None in the migrated files. The original provider repo still imports SDKv2 in other files; those are out of scope for this resource-level migration.

## Verification needed
- `go build ./...` — should compile once `acceptance.TestAccProtoV6ProviderFactories` is wired in the acceptance package
- `go vet ./...`
- Register `NewDatabaseUserResource()` in the provider's `Resources()` method
- Remove the old `ResourceDigitalOceanDatabaseUser()` registration
