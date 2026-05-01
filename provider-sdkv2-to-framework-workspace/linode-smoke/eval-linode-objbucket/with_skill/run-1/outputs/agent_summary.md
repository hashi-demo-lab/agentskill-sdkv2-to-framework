# Migration Summary: linode_object_storage_bucket

## Block-vs-Attribute Decisions

### `cert` (SDKv2: `TypeList`, `MaxItems: 1`)

**Decision: `ListNestedBlock` + `listvalidator.SizeAtMost(1)`**

Rationale: `cert { ... }` is well-established in practitioner configs for the Linode provider (it appears in tmpl/cert.gotf and is documented). Switching to `SingleNestedAttribute` would change the HCL syntax from `cert { ... }` to `cert = { ... }`, which is a breaking change for existing configurations. Since there is no major-version bump in scope, the block form is preserved. `ListNestedBlock` with `SizeAtMost(1)` was chosen over `SingleNestedBlock` because the existing state path is `cert.0.certificate` (the list-shaped path), preserving backward state compatibility.

### `lifecycle_rule` (SDKv2: `TypeList`, no `MaxItems`)

**Decision: `ListNestedBlock`**

Rationale: This is a true repeating block (no `MaxItems`). Practitioners write `lifecycle_rule { ... } lifecycle_rule { ... }`. Converting to a nested attribute would change HCL syntax. `ListNestedBlock` is the correct framework equivalent.

### `expiration` and `noncurrent_version_expiration` (nested inside `lifecycle_rule`, `MaxItems: 1`)

**Decision: `ListNestedBlock` + `listvalidator.SizeAtMost(1)`**

Rationale: Same reasoning as `cert`. Both are nested inside `lifecycle_rule` and use block syntax in practitioner configs. State paths like `lifecycle_rule.0.expiration.0.date` depend on the list-shaped path, so `ListNestedBlock` + `SizeAtMost(1)` is used to preserve both HCL syntax and state path compatibility.

## Default Value Handling

### `acl` (SDKv2: `Default: "private"`)

**Decision: `Default: stringdefault.StaticString("private")` with `Computed: true`**

The framework separates `Default` from plan modifiers. `stringdefault.StaticString("private")` is placed in the `Default` field of `schema.StringAttribute`, NOT in `PlanModifiers`. The attribute is also marked `Computed: true` as required by the framework whenever a `Default` is set (the framework uses `Computed` to inject the default into the plan).

## Composite ID Import Parsing

### `Importer` (SDKv2: `schema.ImportStatePassthroughContext`)

**Decision: Manual parse in `ImportState` method writing to `path.Root("id")`**

Despite the SDKv2 resource using `ImportStatePassthroughContext`, the ID stored in state is always a composite `"<cluster_or_region>:<label>"` string (set by `Create`/`Read` via `fmt.Sprintf`). The framework `ImportState` implementation:

1. Validates that `req.ID` matches the `X:Y` pattern.
2. Writes the full composite string to `resp.State.SetAttribute(ctx, path.Root("id"), req.ID)`.
3. Lets `Read` decode the composite ID via `decodeBucketIDFromModel` (which splits on `:`) and populate all remaining attributes.

This approach mirrors the `DecodeBucketID` function already present in the package: if the ID is well-formed, split and use directly; if malformed, fall back to `cluster`/`region` + `label` attributes.

## Other Migration Notes

- **No SDKv2 imports** remain in the migrated `resource.go`. The `helpers.go` `populateLogAttributes` and `validateRegionIfPresent` (which use `*schema.ResourceData`) are not referenced; equivalent methods are implemented on `*BucketResource`.
- **`GetObjKeys`** (SDKv2-based, in `obj` package) is not reused. A local `getObjKeys` method on `BucketResource` replicates the three-tier key resolution (resource-level → provider-level → temp keys) using `helper.FrameworkProviderModel` fields directly, avoiding an import cycle with the `obj` package.
- **`Delete` reads from `req.State`**, not `req.Plan` (which is null on delete).
- **`cors_enabled`** uses `boolplanmodifier.UseStateForUnknown()` since it is `Computed` and stabilises after creation.
- **Test file** uses `ProtoV6ProviderFactories` and `acceptance.TestAccFrameworkProvider.Meta.Client` (framework pattern), removing all `TestAccSDKv2Provider.Meta().(*helper.ProviderMeta)` references. Import steps include `ImportStateVerifyIgnore: []string{"cert"}` since certificate/private-key fields are write-only (not readable from the API).
