# Migration Summary: linode_domain resource (SDKv2 → terraform-plugin-framework)

## Source files analysed

- `linode/domain/resource.go` – SDKv2 CRUD + CustomizeDiff
- `linode/domain/schema_resource.go` – SDKv2 schema with DiffSuppressFunc
- `linode/domain/resource_test.go` – acceptance tests
- `linode/helper/domain.go` – `DomainSecondsDiffSuppressor` implementation
- `linode/domain/framework_datasource.go` / `framework_models.go` / `framework_schema_datasource.go` – existing framework data-source (reused model patterns)
- `linode/helper/framework_resource_base.go` – `BaseResource` / `BaseResourceConfig` helper
- `linode/helper/setplanmodifiers/caseinsensitiveset.go` – `CaseInsensitiveSet` plan modifier
- `linode/helper/framework_schema.go` – `EmptySetDefault` helper

---

## Translation decisions

### 1. DiffSuppressFunc → custom `Int64PlanModifier`

`helper.DomainSecondsDiffSuppressor()` rounded the *declared* value to the nearest accepted Linode domain-seconds bucket and compared it with the *provisioned* (state) value.

**Framework equivalent:** a custom `planmodifier.Int64` (`DomainSecondsPlanModifier`) implemented in `resource.go`.

Logic:
```
if roundDomainSeconds(planned) == state { resp.PlanValue = stateValue }
```

Applied to `ttl_sec`, `retry_sec`, `expire_sec`, `refresh_sec`.  
All four fields are `Optional+Computed` with `Default: int64default.StaticInt64(0)`.

### 2. CustomizeDiff → schema defaults + plan modifiers

The SDKv2 resource used `customdiff.All(...)` with two functions:

| SDKv2 customdiff | Framework equivalent |
|---|---|
| `linodediffs.ComputedWithDefault("tags", []string{})` | `Default: helper.EmptySetDefault(types.StringType)` on the `tags` schema attribute |
| `linodediffs.CaseInsensitiveSet("tags")` | `PlanModifiers: []planmodifier.Set{linodesetplanmodifiers.CaseInsensitiveSet()}` on `tags` |

A `ModifyPlan` method is present on `DomainResource` but is a no-op; both behaviours are encoded declaratively in the schema, which is idiomatic in the framework.

### 3. ConflictsWith

The SDKv2 schema_resource.go does **not** contain explicit `ConflictsWith` annotations.  
The task description references them in context of the data-source (`id` conflicts with `domain`), which already exists in `framework_schema_datasource.go` and was not changed.  
No `ConflictsWith` wiring was needed in the resource schema.

### 4. Resource model

Used `types.Set` (not `[]types.String`) for `master_ips`, `axfr_ips`, and `tags` to match the schema `SetAttribute` type and to be consistent with the `Default: EmptySetDefault(...)` pattern used elsewhere in this provider (firewall, volume, nodebalancer).

### 5. Import

`BaseResource.ImportState` already handles int64 IDs via `IDType: types.Int64Type`; no custom `ImportState` override is needed.

### 6. Test file changes

- Removed `helper.ProviderMeta` reference in `checkDomainExists` / `checkDestroy`; replaced with `acceptance.GetTestClient()` so tests do not depend on the SDKv2 provider singleton.
- Changed `resource.TestCheckNoResourceAttr(resName, "master_ips")` → `resource.TestCheckResourceAttr(resName, "master_ips.#", "0")` to match the framework's behaviour where `Computed+Default` sets always appear in state (empty set, not absent).
- `ProtoV6ProviderFactories` already used throughout – no change needed.
- Build tag and package declaration unchanged.

---

## Files produced

| Path | Description |
|---|---|
| `migrated/resource.go` | Full framework resource (schema, model, CRUD, plan modifier) |
| `migrated/resource_test.go` | Updated acceptance tests |
