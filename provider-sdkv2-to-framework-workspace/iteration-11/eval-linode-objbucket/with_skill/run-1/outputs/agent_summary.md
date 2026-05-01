# Agent Summary: objbucket SDKv2 → Plugin Framework Migration

## Pre-flight Think Pass

### Block decisions
- `cert` (MaxItems:1, TypeList): kept as `ListNestedBlock + listvalidator.SizeAtMost(1)` — practitioners use block HCL syntax in production configs; no major-version bump planned.
- `lifecycle_rule` (no MaxItems, repeating TypeList): kept as `ListNestedBlock` — preserves `lifecycle_rule { ... }` HCL syntax.
- `expiration` (MaxItems:1, nested inside lifecycle_rule): kept as `ListNestedBlock + listvalidator.SizeAtMost(1)` — same reasoning as cert.
- `noncurrent_version_expiration` (MaxItems:1, nested inside lifecycle_rule): kept as `ListNestedBlock + listvalidator.SizeAtMost(1)`.

### State upgrade
No `SchemaVersion > 0` or `StateUpgraders` — no state upgrader needed.

### Import shape
Composite ID `cluster:label` (or `region:label`). SDKv2 used `schema.ImportStatePassthroughContext` which passed the full composite ID as the resource ID. The framework `ImportState` parses the ID into its constituent parts and writes them to state attributes, then lets `Read` populate the rest.

## Key decisions

- **`ExactlyOneOf` for cluster/region**: used `stringvalidator.ExactlyOneOf(path.MatchRoot("region"))` and vice versa, matching the `obj` package pattern.
- **`acl` Default**: uses `stringdefault.StaticString("private")` from `resource/schema/stringdefault`.
- **ForceNew → RequiresReplace**: `label`, `cluster`, `region`, `s3_endpoint`, `endpoint_type` all get `stringplanmodifier.RequiresReplace()`.
- **Sensitive fields**: `cert.certificate` and `cert.private_key` are marked `Sensitive: true` in the framework schema; `secret_key` is also sensitive. These cannot be imported back from the API, so `ImportStateVerifyIgnore` is set for `secret_key` and `access_key` in tests.
- **S3 key handling**: `obj.GetObjKeys` is SDKv2-tied (takes `*schema.ResourceData`). Implemented a local `getObjKeys` method on `BucketResource` that reads from model fields, falls back to provider config, then generates temp keys via `linodego` — matching the logic in `obj/framework_models.go:GetObjectStorageKeys`.
- **`DecodeBucketID`**: replaced with `decodeBucketIDFromModel` that works with typed framework model.
- **Not-found in Read**: sets `data.ID = types.StringNull()` as signal, then the `Read` handler calls `resp.State.RemoveResource(ctx)`.
- **`Delete` reads `req.State` not `req.Plan`**: `req.Plan` is null on Delete; correctly reads from `req.State`.

## Files produced
- `migrated/resource.go` — full framework resource (no SDKv2 imports); `BucketResource` implements `resource.Resource` and `resource.ResourceWithImportState`; uses framework schema with `ListNestedBlock + SizeAtMost(1)` for `cert`, `expiration`, `noncurrent_version_expiration`; `stringdefault.StaticString("private")` for `acl` default; composite ID import.
- `migrated/resource_test.go` — updated tests using `ProtoV6ProviderFactories` (already was using them in original); removed `terraform-plugin-sdk/v2/helper/schema` import; replaced `objbucket.DecodeBucketID` calls with inline string splitting; switched `TestAccSDKv2Provider.Meta().(*helper.ProviderMeta).Client` to `TestAccFrameworkProvider.Meta.Client`; added `ImportStateVerifyIgnore` for write-only fields.

## Negative gate
No `github.com/hashicorp/terraform-plugin-sdk/v2` references in either migrated file.
