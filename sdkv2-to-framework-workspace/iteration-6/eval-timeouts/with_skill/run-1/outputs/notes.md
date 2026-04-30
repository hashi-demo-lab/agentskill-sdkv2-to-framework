# Migration notes — `openstack_db_database_v1`

## Scope

Single-resource migration from `terraform-plugin-sdk/v2` to
`terraform-plugin-framework`, focused on translating the SDKv2 `Timeouts`
field (Create + Delete) into the framework's
`terraform-plugin-framework-timeouts` package while preserving the existing
HCL block syntax (`timeouts { create = "..." }`).

Only two files were touched:
- `migrated/resource_openstack_db_database_v1.go`
- `migrated/resource_openstack_db_database_v1_test.go`

No other resources were migrated. The provider definition itself
(`provider.go`, `provider_test.go`, `main.go`) is left for whoever does the
provider-wide migration; this resource is delivered as if it were ready to
slot into that effort.

## Timeouts: block syntax preserved

The SDKv2 resource declared:

```go
Timeouts: &schema.ResourceTimeout{
    Create: schema.DefaultTimeout(10 * time.Minute),
    Delete: schema.DefaultTimeout(10 * time.Minute),
},
```

Practitioners therefore wrote:

```hcl
timeouts {
  create = "20m"
  delete = "15m"
}
```

To keep that exact HCL shape working post-migration, the framework
attribute is declared as a **block**, not an attribute:

```go
Blocks: map[string]schema.Block{
    "timeouts": timeouts.Block(ctx, timeouts.Opts{
        Create: true,
        Delete: true,
    }),
},
```

`timeouts.Attributes(...)` (the alternative) would render as
`timeouts = { create = "..." }` and is a breaking HCL change for existing
users. The skill's `references/timeouts.md` flags this explicitly: prefer
`Block` for migrations, `Attributes` only for greenfield.

The model carries a `timeouts.Value` field with the matching tag:

```go
type databaseDatabaseV1Model struct {
    ...
    Timeouts timeouts.Value `tfsdk:"timeouts"`
}
```

`Create` reads the configured timeout via `plan.Timeouts.Create(ctx, 10*time.Minute)`
(default = the SDKv2 `DefaultTimeout` value), wraps the context with
`context.WithTimeout`, and feeds the resulting deadline into the inline
state-poll replacement for `retry.StateChangeConf`. `Delete` does the
analogous thing with `state.Timeouts.Delete(ctx, 10*time.Minute)`. There is
no `Update` timeout because the SDKv2 resource didn't declare one.

## Other migration decisions

- **Plan modifiers**: SDKv2 `ForceNew: true` on `region`, `name`, and
  `instance_id` becomes `stringplanmodifier.RequiresReplace()` (NOT
  `RequiresReplace: true` — that's a different package). `region` is
  Optional+Computed so it also gets `UseStateForUnknown` to suppress the
  spurious "known after apply" diff when not explicitly set, matching the
  SDKv2 default-on-Computed behaviour.
- **`id` attribute**: SDKv2 created an implicit `id`; the framework requires
  it to be declared explicitly as Computed with `UseStateForUnknown`.
- **CRUD diagnostics**: `diag.Errorf(...)` and `diag.FromErr(...)` were
  swept to `resp.Diagnostics.AddError(summary, err.Error())` with an early
  return after each potentially failing call.
- **`d.SetId("")` in Read** (drift recovery) became
  `resp.State.RemoveResource(ctx)`, per `references/resources.md`.
- **`CheckDeleted`** was inlined inside `Delete` as a 404 check using
  `gophercloud.ResponseCodeIs(err, http.StatusNotFound)`; the existing
  helper takes an `*schema.ResourceData` so it cannot be reused as-is from
  a framework resource.
- **`GetRegion`** was reimplemented locally as `getRegionForResource`
  taking a `types.String` (the framework analogue of "is this set?") plus
  the `*Config`. Same semantics: per-resource override, falling back to
  `config.Region`.
- **`retry.StateChangeConf`**: The framework has no equivalent helper, and
  importing `terraform-plugin-sdk/v2/helper/retry` from a migrated file
  would fail the negative gate (`scripts/verify_tests.sh` rejects any
  `terraform-plugin-sdk/v2` import in the migrated set). Replaced with two
  small local helpers, `databaseDatabaseV1RefreshFunc` (signature
  `func() (any, string, error)` — the canonical form, no named SDKv2 type)
  and `waitForDatabaseDatabaseV1State`, which is a context-aware ticker
  loop equivalent to `WaitForStateContext`. Pattern lifted from
  `references/resources.md`.
- **Importer**: SDKv2 used `ImportStatePassthroughContext`; the framework
  equivalent is `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)`
  inside `ImportState`, and the resource declares
  `_ resource.ResourceWithImportState = &databaseDatabaseV1Resource{}` so
  a missing method becomes a compile error.

## Test changes

- `ProviderFactories` → `ProtoV6ProviderFactories`. The variable
  `testAccProtoV6ProviderFactories` is referenced from `provider_test.go`,
  which is **not** part of this scope. To run the test, the provider needs
  to:
  1. Define `testAccProtoV6ProviderFactories` of type
     `map[string]func() (tfprotov6.ProviderServer, error)`, OR
  2. Register both SDKv2 and framework resources via
     `terraform-plugin-mux` for a transitional period — but that's the
     multi-release migration path the skill explicitly excludes.

  The cleanest single-release path is option 1, applied once the bulk of
  the provider has migrated.

- The basic test step now sets `timeouts { create = "20m" delete = "15m" }`
  in the HCL config and asserts the round-trip via
  `TestCheckResourceAttr("...", "timeouts.create", "20m")` etc. This is
  the test that exercises the new schema feature; it should fail on the
  pre-migrated SDKv2 resource (because SDKv2 doesn't render `timeouts`
  as a state attribute the same way) and pass after migration — i.e. the
  TDD red-then-green pattern from workflow step 7.

- An import-verify step was added (`ImportStateVerify: true`) with
  `ImportStateVerifyIgnore: []string{"timeouts"}` because `timeouts` is
  config-only and cannot be recovered from the remote API on import.

- Helper functions (`testAccCheckDatabaseV1DatabaseExists`,
  `testAccCheckDatabaseV1DatabaseDestroy`) are unchanged: they read the
  Terraform state via the testing-framework `*terraform.State`, which is
  identical between SDKv2 and the framework — both go through
  `terraform-plugin-testing`.

## Negative-gate compliance

The migrated file imports zero `terraform-plugin-sdk/v2` packages.
Specifically:
- No `helper/schema`, no `diag`, no `helper/retry`.
- The shared helpers `parsePairedIDs` and `databaseDatabaseV1Exists` are
  signature-clean (they take primitives / `*gophercloud.ServiceClient`,
  not `*schema.ResourceData`) and so can be called from framework code as
  is.
- The `db_database_v1.go` helper file still imports `helper/retry` for
  the SDKv2 `databaseDatabaseV1StateRefreshFunc`. Since this resource no
  longer calls that function (it uses the inline replacement instead), the
  helper can be deleted as part of a follow-up sweep — but per the task's
  "don't migrate anything else" rule, it is left untouched here. Note
  this in the migration checklist row for follow-up.

## Verification

To verify locally:

```sh
bash <skill-path>/scripts/verify_tests.sh \
    /Users/simon.lynch/git/terraform-provider-openstack \
    --migrated-files openstack/resource_openstack_db_database_v1.go \
                     openstack/resource_openstack_db_database_v1_test.go
```

Expected gates:
1. `go build ./...` — needs the provider definition to register
   `NewDatabaseDatabaseV1Resource()`, otherwise the resource file builds
   but isn't reachable.
2. `go vet ./...` — should be clean.
3. Negative gate — passes (no SDKv2 imports in migrated files).
4. `TF_ACC=1` acceptance — needs the provider-level mux/factory wiring
   described above; out of scope for this single-resource migration.
