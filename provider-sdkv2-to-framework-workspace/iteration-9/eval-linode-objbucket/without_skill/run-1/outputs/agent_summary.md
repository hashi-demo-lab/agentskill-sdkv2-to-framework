# Migration Summary: linode_object_storage_bucket

## What was migrated

The `linode/objbucket` resource was migrated from `terraform-plugin-sdk/v2` to `terraform-plugin-framework`. The primary source files analysed were:

- `resource.go` — CRUD logic
- `schema_resource.go` — SDK v2 schema definitions  
- `helpers.go` — utility functions (populateLogAttributes, validateRegion, getS3Endpoint)

The output is a single self-contained `resource.go` that replaces all three SDK v2 source files.

---

## Key migration decisions

### 1. MaxItems:1 blocks → ListNestedBlock + SizeAtMost(1)

Both `cert` and the nested `expiration` / `noncurrent_version_expiration` blocks under `lifecycle_rule` had `MaxItems:1` in the SDK schema. In the framework these become `schema.ListNestedBlock` with a `listvalidator.SizeAtMost(1)` validator, and are mapped to `[]<ModelType>` slices (length 0 or 1) in the Go model structs.

### 2. Default value for `acl`

The SDK `Default: "private"` is expressed as `stringdefault.StaticString("private")` on the `acl` `schema.StringAttribute`.

### 3. Custom ImportState for composite ID

The SDK v2 resource used `ImportStatePassthroughContext` which only sets the `id` attribute. The framework `ImportState` override splits the incoming `<cluster_or_region>:<label>` ID, validates both parts are non-empty, and sets `id` on the state. The subsequent `Read` call then re-populates all fields from the API.

The helper `DecodeBucketIDString` (public, replaces the old `DecodeBucketID` which required `*schema.ResourceData`) is exported so the test file can use it in `checkBucketExists` / `checkBucketDestroy`.

### 4. ExactlyOneOf(region, cluster) validator

Replicated with `stringvalidator.ExactlyOneOf(path.MatchRoot("region"))` on each of `cluster` and `region`.

### 5. Deprecated fields

`cluster` and `endpoint` carry `DeprecationMessage` on their schema attributes. `ForceNew` on `cluster`, `region`, `label`, `s3_endpoint`, `endpoint_type` is expressed as `stringplanmodifier.RequiresReplace()`.

### 6. S3 key resolution (versioning / lifecycle)

The SDK v2 code used `obj.GetObjKeys(*schema.ResourceData, ...)`. In the framework that function is unavailable without the SDK dependency. The migration inlines the key-resolution logic as `(m *ResourceModel) getObjKeys(...)`, following the same priority order:

1. Resource-level `access_key` / `secret_key`
2. Provider-level `obj_access_key` / `obj_secret_key` (from `r.Meta.Config`)
3. Temporary keys via `client.CreateObjectStorageKey` when `obj_use_temp_keys` is set

### 7. helpers.go

`helpers.go` still exists (unchanged) and continues to export `validateRegion`, `getS3Endpoint`, etc. The framework resource calls `validateRegion` directly (it takes `*linodego.Client`, no SDK v2 dependency). `populateLogAttributes` and `validateRegionIfPresent` (which require `*schema.ResourceData`) are not called by the new framework resource.

---

## Model types (new)

```
ResourceModel            — top-level tfsdk struct
CertModel                — cert block ([]CertModel, len 0-1)
LifecycleRuleModel       — one lifecycle_rule entry
ExpirationModel          — expiration sub-block ([]ExpirationModel, len 0-1)
NoncurrentVersionExpirationModel — noncurrent_version_expiration sub-block
```

---

## Test file changes

- Removed `"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"` import.
- Replaced `objbucket.DecodeBucketID(ctx, id, &schema.ResourceData{})` with `objbucket.DecodeBucketIDString(id)` (new exported function, no `*schema.ResourceData` required).
- Changed `acceptance.TestAccSDKv2Provider.Meta().(*helper.ProviderMeta).Client` to `acceptance.TestAccFrameworkProvider.Meta.Client` (framework provider, field access not method call).
- All other test logic and helper checks are preserved as-is.

---

## Files produced

- `$OUTPUT_DIR/migrated/resource.go` — migrated framework resource (replaces resource.go + schema_resource.go, inlines the framework-compatible parts of helpers.go)
- `$OUTPUT_DIR/migrated/resource_test.go` — updated test file
