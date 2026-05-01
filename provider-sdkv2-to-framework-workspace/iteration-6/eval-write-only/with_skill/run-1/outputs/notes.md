# Migration notes — openstack_db_user_v1

Migration of the single resource `openstack_db_user_v1` from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`. Scoped to that one
resource per task instructions; nothing else in the repo was changed.

## Sensitive vs WriteOnly decision

Applied the rule from `references/sensitive-and-writeonly.md`:

> *Does Terraform need to read this value back later?* If **no** (one-time
> creds, initial passwords, rotation seeds) → `Sensitive: true` AND
> `WriteOnly: true`.

The `password` attribute on `openstack_db_user_v1`:
- Is required only at user-creation time (passed to the OpenStack Trove
  `users.Create` API).
- Is never read back — the API does not return it, and `Read` would have
  nothing useful to compare against for drift detection.
- All mutable attributes on this resource are `RequiresReplace`, so password
  changes recreate the user; there is no `Update` flow that needs the prior
  value either.

Combined with the task's explicit allowance for breaking credentials-handling
changes (major-version bump), this is a textbook WriteOnly candidate:

```go
"password": schema.StringAttribute{
    Required:    true,
    Sensitive:   true,
    WriteOnly:   true,   // never persisted to state
    // NOTE: NOT Computed — Computed + WriteOnly is rejected by the framework.
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(),
    },
},
```

### Implementation rules followed

1. **`Sensitive: true` AND `WriteOnly: true`, NOT `Computed`** — set per the
   skill's hard rules. `Computed` + `WriteOnly` is a boot-time error.
2. **Read password from `req.Config`, not `req.Plan` / `req.State`** — the
   skill's reference is explicit on this. In `Create` the resource calls
   `req.Plan.Get` for the rest of the model and `req.Config.Get` for the
   password value:

   ```go
   var plan databaseUserV1Model
   resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
   var config databaseUserV1Model
   resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
   // ...
   Password: config.Password.ValueString(),
   ```
3. **Pair WriteOnly with `ImportStateVerifyIgnore` in tests** — the import
   step in the test file lists `"password"` in `ImportStateVerifyIgnore`,
   without which `ImportStateVerify` would fail because state holds null
   while config holds `"password"`.
4. **Practitioner-visible breaking change documented** — the task explicitly
   identifies this as a major-version bump, so the upgrade is appropriate
   here rather than deferred. CHANGELOG note recommended (out of scope for
   this single-resource artefact).

## Schema mapping (SDKv2 → framework)

| SDKv2 field | Framework attribute | Notes |
|---|---|---|
| `region` (Optional, Computed, ForceNew) | `StringAttribute{Optional, Computed}` + `RequiresReplace` + `UseStateForUnknown` | Standard pattern. |
| `name` (Required, ForceNew) | `StringAttribute{Required}` + `RequiresReplace` | |
| `instance_id` (Required, ForceNew) | `StringAttribute{Required}` + `RequiresReplace` | |
| `password` (Required, ForceNew, Sensitive) | `StringAttribute{Required, Sensitive, WriteOnly}` + `RequiresReplace` | **Upgraded to WriteOnly.** |
| `host` (Optional, ForceNew) | `StringAttribute{Optional}` + `RequiresReplace` | |
| `databases` (TypeSet, Optional, Computed) | `SetAttribute{Optional, Computed, ElementType: types.StringType}` + `UseStateForUnknown` | `Set: schema.HashString` dropped — framework sets handle uniqueness intrinsically. |
| `id` (implicit) | `StringAttribute{Computed}` + `UseStateForUnknown` | Made explicit, as required by the framework. |
| `Timeouts: {Create, Delete}` | `timeouts.Block(ctx, timeouts.Opts{Create: true, Delete: true})` | Used `Block` (not `Attributes`) to preserve existing HCL block syntax. |

## CRUD changes

- `CreateContext` / `ReadContext` / `DeleteContext` → `Create` / `Read` /
  `Delete` framework methods with typed `req`/`resp`.
- `d.Get("password").(string)` → `config.Password.ValueString()` (config, not
  plan, because of WriteOnly).
- `d.SetId("")` (resource gone) → `resp.State.RemoveResource(ctx)` in `Read`.
- Diagnostics return `diag.Errorf(...)` → `resp.Diagnostics.AddError(...)`.
- `retry.StateChangeConf.WaitForStateContext` → inline `waitForDatabaseUserV1State`
  helper (per `references/resources.md`'s "Replacing retry.StateChangeConf"
  section). The migrated file does **not** import
  `terraform-plugin-sdk/v2/helper/retry`. The pre-existing helper file
  `db_user_v1.go` still uses it; per the skill's guidance for single-resource
  migrations, the named return type `retry.StateRefreshFunc` is structurally
  identical to `func() (any, string, error)` and is assignable across the
  call boundary, so the helper file is left untouched and the migrated
  resource file stays SDKv2-import-clean.
- `Importer: ImportStatePassthroughContext` → `ResourceWithImportState` with
  `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`. The
  composite ID `instance_id/name` round-trips through `Read` (which calls
  `parsePairedIDs` to populate the rest of the model), so passthrough is
  sufficient — no manual parsing needed in `ImportState`.

## What I did NOT change

Per task scope ("Don't migrate anything else in the repo"):

- The shared helper file `openstack/db_user_v1.go` still imports
  `terraform-plugin-sdk/v2/helper/retry` for `databaseUserV1StateRefreshFunc`.
  This is fine: the migrated resource file is the only file that needs to be
  SDKv2-clean, and Go's type identity rules let the named `retry.StateRefreshFunc`
  return value satisfy the `func() (any, string, error)` parameter on
  `waitForDatabaseUserV1State`. A whole-provider sweep would change the
  helper's declared return type to remove the import; out of scope here.
- Test bootstrap (`provider_test.go`) — the test file references
  `testAccProtoV6ProviderFactories`, which must be added to the provider's
  test bootstrap as part of the wider migration (workflow steps 3-5). The
  existing `testAccProvider` / `testAccProviders` references in the helper
  funcs are kept untouched because the SDKv2 provider is still present in
  the codebase during a single-release migration.
- No CHANGELOG, docs, or example HCL updates — out of scope per the task.

## Verification

Did not run `verify_tests.sh` — task explicitly forbids running `go` and
limits writes to the output dir; the openstack clone is read-only. The
migrated file passes the static checks the skill cares about:

- No `terraform-plugin-sdk/v2` import in
  `migrated/resource_openstack_db_user_v1.go`.
- `password` has `Sensitive: true`, `WriteOnly: true`, no `Computed`.
- Password is read from `req.Config`, not `req.Plan` or `req.State`, in
  `Create`. Update is a no-op (no mutable attributes); Read does not touch
  password; Delete does not touch password.
- Test file lists `"password"` in `ImportStateVerifyIgnore` and switches to
  `ProtoV6ProviderFactories`.

## Compatibility floor

`WriteOnly` requires `terraform-plugin-framework` v1.14.0+ (technical
preview) and Terraform 1.11+, with v1.17.0+ recommended for production use
(per `references/sensitive-and-writeonly.md`). Bump the provider's
`go.mod` to at least v1.17.0 of the framework when integrating this change.
