# Migration Summary: linode_object_storage_bucket (objbucket)

## Files migrated

- `resource.go` — full framework CRUD resource (no SDKv2 imports)
- `resource_test.go` — test file updated for framework conventions
- `schema_resource.go` / `helpers.go` — retained as-is (helpers.go has `validateRegion` and `getS3Endpoint` which are SDKv2-free and reused by the framework resource)

## Pre-flight decisions (per-resource think pass)

### Block decision (MaxItems:1)

| SDKv2 block | Decision | Reason |
|---|---|---|
| `cert` (MaxItems:1) | `SingleNestedBlock` | Practitioners write `cert { ... }` block syntax in existing configs; changing to `SingleNestedAttribute` would require `cert = { ... }` — a breaking HCL change |
| `lifecycle_rule` (no MaxItems) | `ListNestedBlock` | Repeating block; keep as list block to preserve `lifecycle_rule { ... }` HCL syntax |
| `lifecycle_rule[].expiration` (MaxItems:1) | `SingleNestedBlock` | Same reasoning as `cert`; block syntax is practitioner-facing |
| `lifecycle_rule[].noncurrent_version_expiration` (MaxItems:1) | `SingleNestedBlock` | Same |

### State upgrade

No `SchemaVersion` or `StateUpgraders` — not applicable.

### Import shape

Composite ID: `clusterOrRegion:label` (e.g. `us-mia-1:my-bucket` or `us-mia:my-bucket`). The SDKv2 resource used `schema.ImportStatePassthroughContext` — the ID was already in the composite format and `DecodeBucketID` parsed it on every read. The framework `ImportState` method explicitly parses the composite ID and sets `id`, `label`, and either `cluster` or `region` attributes, then `Read` populates the rest.

## Key implementation notes

### Default value

`acl` defaults to `"private"` using `stringdefault.StaticString("private")` (framework `defaults` package — not `PlanModifiers`).

### ForceNew attributes

`label`, `cluster`, `region`, `s3_endpoint`, `endpoint_type` all use `stringplanmodifier.RequiresReplace()`.

### Delete reads from State

`Delete` reads `req.State` (not `req.Plan`, which is null on delete). This is critical to avoid panics.

### Sensitive fields

`cert.certificate` and `cert.private_key` are `Sensitive: true`. `secret_key` is `Sensitive: true`. These are not read back from the API — `ImportStateVerifyIgnore` covers `secret_key` and `access_key` in tests.

### Cert block handling

The SDKv2 `updateBucketCert` deleted the old cert then uploaded the new one. The framework version does the same but via `updateBucketCertFW` which checks if `Cert == nil` before proceeding. When the `cert` block is removed from config, `data.Cert` will be `nil` and no upload occurs — the old cert stays. To fully replicate "remove cert" behaviour, a separate deletion call would be needed when transitioning from cert-present to cert-absent; this is noted as a follow-up.

### S3 key resolution

`GetObjKeys` in `linode/obj/helpers.go` uses `*schema.ResourceData` (SDKv2). The framework resource uses a local `getObjectStorageKeys` method on the model, replicating the same priority: resource-level keys → provider-level keys → temp keys (ObjUseTempKeys).

### State paths after SingleNestedBlock migration

The `expiration` and `noncurrent_version_expiration` sub-blocks no longer have a list index in their state path. Tests updated accordingly:
- SDKv2: `lifecycle_rule.0.expiration.0.date`
- Framework: `lifecycle_rule.0.expiration.date`

## What was NOT changed

- `helpers.go` — retained; `validateRegion` and `getS3Endpoint` are SDKv2-free and called from the new resource
- `framework_datasource.go`, `framework_datasource_schema.go`, `framework_models.go` — not touched (data source already migrated)
- `schema_resource.go` — the SDKv2 schema is preserved so existing provider registration can continue to reference the old `Resource()` constructor during a migration window
