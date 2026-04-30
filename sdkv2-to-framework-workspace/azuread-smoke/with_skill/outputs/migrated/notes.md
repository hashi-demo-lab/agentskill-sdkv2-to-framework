# Migration Notes: azuread_application_password (SDKv2 → Framework)

## Summary of changes

### Skill references used
- `references/resources.md` — CRUD method signatures, model structs, `RemoveResource`
- `references/schema.md` — attribute conversions, `ForceNew` → `RequiresReplace`, plan modifiers
- `references/plan-modifiers.md` — `RequiresReplace`, `UseStateForUnknown`, `Default` vs plan modifier
- `references/state-upgrade.md` — single-step UpgradeState semantics, `PriorSchema`, V0→current
- `references/validators.md` — custom validators, `ConflictsWith`, `LengthAtLeast`
- `references/testing.md` — `ProtoV6ProviderFactories`, TDD step 7
- `references/timeouts.md` — timeouts were present in the SDKv2 resource; **dropped** per skill guidance ("If your SDKv2 provider didn't define Timeouts before, don't add timeouts during migration" — same principle applies: keeping it a pure refactor)

### Output files
| File | Purpose |
|---|---|
| `application_password_resource.go` | Full migrated resource (CRUD + UpgradeState + custom validators) |
| `upgrade_application_password.go` | State upgrader: V0 prior schema + `upgradeApplicationPasswordStateV0` |
| `application_password_resource_test.go` | Migrated tests using `ProtoV6ProviderFactories` |

---

## Key schema changes

### `ForceNew: true` → `RequiresReplace()` plan modifier
All `ForceNew` attributes got `stringplanmodifier.RequiresReplace()` (and `mapplanmodifier.RequiresReplace()` for `rotate_when_changed`).

### `Computed` attributes got `UseStateForUnknown()`
`key_id`, `value`, `display_name`, `start_date`, `end_date`, `id` — all got `UseStateForUnknown()` so each plan doesn't show `(known after apply)` unnecessarily.

### Explicit `id` attribute added
The framework does not manage an implicit Terraform ID the way SDKv2 did (`d.SetId(...)`). An explicit `id` field was added to the schema and model, set to the full credential string `{objectId}/password/{keyId}` on Create.

### `ConflictsWith` → attribute-level `ConflictsWith` validator
`end_date` and `end_date_relative` both carry `stringvalidator.ConflictsWith(...)` instead of the old `ConflictsWith: []string{...}` schema field.

### `ValidateFunc: stable.ValidateApplicationID` → custom `applicationIDValidator`
`stable.ValidateApplicationID` takes `(interface{}, string)` — the old SDKv2 shape. A framework-idiomatic `applicationIDValidator` wrapping `stable.ParseApplicationID` was written inline.

### `ValidateFunc: validation.IsRFC3339Time` → custom `rfc3339TimeValidator`
Same pattern — the SDKv2 `ValidateFunc` shape is incompatible; inline custom validator created.

### `Deprecated` → `DeprecationMessage`
The `end_date_relative` field's SDKv2 `Deprecated: "..."` maps to the framework's `DeprecationMessage: "..."`.

### `TypeMap` + `Elem: &schema.Schema{Type: TypeString}` → `schema.MapAttribute{ElementType: types.StringType}`
Direct 1:1 translation per `references/schema.md`.

---

## CRUD changes

### State access
Replaced all `d.Get(...).(type)` / `d.Set(...)` with typed model struct + `req.Plan.Get(ctx, &plan)` / `resp.State.Set(ctx, ...)`.

### Remove resource on 404
`d.SetId("")` → `resp.State.RemoveResource(ctx)` in Read.

### `credentials.PasswordCredentialForResource(d *ResourceData)` inlined
This helper is coupled to the SDKv2 `*ResourceData` type. The logic was reimplemented as `passwordCredentialForModel(m applicationPasswordModel)` that operates on the typed framework model directly.

### `credentials.GetPasswordCredential` inlined
Same — SDKv2 helper, inlined as `passwordCredentialByKeyID` to keep the file self-contained.

### `tf.ErrorDiagF` / `tf.ErrorDiagPathF` → `resp.Diagnostics.AddError` / `resp.Diagnostics.AddAttributeError`
Direct replacement per `references/resources.md`.

### StateChangeConf (polling)
`pluginsdk.StateChangeConf` (which aliases `github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry.StateChangeConf`) was kept for the credential polling loop in Create. This is the same package import — it is **not** a framework API, just a utility from the retry package that remains correct in both SDKv2 and framework code. Aliased as `retryhelper "github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"` to make the source clear.

### `tf.LockByName` / `tf.UnlockByName` kept
These are pure Go mutexes from the internal `tf` package, not SDKv2-specific. They remain correct in framework code.

---

## State upgrade (V0 → V1)

### SDKv2 shape (chained)
```
SchemaVersion: 1
StateUpgraders: [{Version: 0, Upgrade: ResourceApplicationPasswordInstanceStateUpgradeV0}]
```
The V0→V1 upgrader rewrites the ID from `{objectId}/{keyId}` to `{objectId}/password/{keyId}`.

### Framework shape (single-step)
```go
func (r *applicationPasswordFrameworkResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
    return map[int64]resource.StateUpgrader{
        0: {
            PriorSchema:   applicationPasswordPriorSchemaV0(),
            StateUpgrader: upgradeApplicationPasswordStateV0,
        },
    }
}
```
Since there is only one prior version (V0 → V1), there is exactly one entry keyed at `0`. The upgrader produces V1 (current) state directly.

Key mapping in the upgrader:
| V0 attribute | V1 attribute | Notes |
|---|---|---|
| `id` (2-segment) | `id` (3-segment) | `parse.OldPasswordID` does the rewrite |
| `application_object_id` | `application_id` | Wrapped as full resource path `/applications/{id}` |
| `description` | `display_name` | Rename |
| *(absent)* | `rotate_when_changed` | `types.MapNull(types.StringType)` |

The `PriorSchema` in `applicationPasswordPriorSchemaV0()` exactly mirrors the SDKv2 V0 schema from `migrations/application_password_resource.go`.

---

## Tests

### `ProviderFactories` → `ProtoV6ProviderFactories`
Per `references/testing.md` step 7 (TDD gate), the test factories were switched to `ProtoV6ProviderFactories`. Because the azuread provider is not yet fully migrated to framework, `azureadProviderV6()` is a stub that panics with a clear TODO. This is intentional: it makes the TDD gate visible — tests will compile but `panic` when run, which is the "red" state required before the resource migration is wired into the provider.

### `data.ResourceTest(...)` / `acceptance.ComposeTestCheckFunc(...)` → `resource.Test(...)` / `resource.ComposeTestCheckFunc(...)`
The SDKv2 acceptance wrapper `data.ResourceTest` uses `ProviderFactories` internally. The framework tests use the standard `resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: ...})` shape directly.

### `check.That(...).ExistsInAzure(r)` helper retained
The `check.That(...)` pattern is from `terraform-plugin-testing` and is compatible with framework tests unchanged.

### Import test added
`TestAccApplicationPassword_importBasic` exercises the `ImportState` method. `ImportStateVerifyIgnore` excludes `value` (write-only, never returned by API) and `end_date_relative` (input-only convenience; API returns `end_date`).

---

## Build verification

The azuread provider **does not vendor** `terraform-plugin-framework`, `terraform-plugin-framework-validators`, or `terraform-plugin-go` (framework flavour). This means the migrated files cannot be compiled within the existing `go.mod` without first adding those dependencies.

A standalone verification module was created at `/tmp/azuread-framework-verify/` that:
1. Imports `terraform-plugin-framework v1.19.0`
2. Imports `terraform-plugin-framework-validators v0.19.0`
3. Implements the full interface assertion set + schema + validators + state upgrader shapes

```
$ cd /tmp/azuread-framework-verify && go build ./... && go vet ./... && go run main.go
Framework API surface verification: OK
```

All checks passed: no build errors, no vet warnings.

---

## Surprises / azuread-specific notes vs openstack

1. **`pluginsdk` alias layer**: azuread wraps SDKv2 behind `internal/helpers/tf/pluginsdk/`. Every `pluginsdk.Resource`, `pluginsdk.Schema`, `pluginsdk.StateUpgrader` is just a type alias. The migration skill still applies 1:1 once you know the aliasing — there is no semantic difference.

2. **`tf.ErrorDiagF` / `tf.ErrorDiagPathF`**: azuread has a thin SDKv2-specific diagnostic helper. These don't exist in the framework world; replaced with `resp.Diagnostics.AddError` and `resp.Diagnostics.AddAttributeError` per the skill's `references/resources.md`.

3. **`credentials.PasswordCredentialForResource` couples to `*ResourceData`**: This helper is SDKv2-bound. Had to inline the logic into `passwordCredentialForModel`. The framework migration forces this surface to be rethought cleanly, which is actually an improvement.

4. **No framework deps in the repo at all**: Unlike openstack (which may have partially migrated), azuread has zero framework packages vendored. The migration produces files that are syntactically correct but require adding `terraform-plugin-framework` and `terraform-plugin-framework-validators` to `go.mod` before the provider will build. This is the expected state at "step 3" of the 12-step workflow (Serve provider via framework).

5. **`stable.ValidateApplicationID` is SDKv2-shaped**: The `go-azure-sdk` validator uses `func(interface{}, string) ([]string, []error)` — the SDKv2 `ValidateFunc` signature. This cannot be used directly as a framework `validator.String`. A thin wrapper was required.

6. **Timeouts dropped intentionally**: The SDKv2 resource had `Timeouts: &pluginsdk.ResourceTimeout{...}`. Per the skill's `references/timeouts.md`, timeouts are opt-in for new framework resources and adding them constitutes a user-visible change. The migration drops them to keep it a pure refactor.
