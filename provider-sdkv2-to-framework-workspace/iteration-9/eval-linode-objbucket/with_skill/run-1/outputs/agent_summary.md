# Migration Summary — linode_object_storage_bucket

## Skill workflow executed

Pre-flight 0: No mux references; single-release path confirmed.
Pre-flight A: Audit run (files reviewed directly).
Pre-flight B: Scope confirmed — resource.go + schema_resource.go + helpers.go.
Pre-flight C: Think-pass completed before editing.

---

## Per-resource think pass

**Block decision**

| SDKv2 schema | Decision | Framework type |
|---|---|---|
| `cert` (TypeList, MaxItems:1) | Block syntax in production configs; keep as block | `SingleNestedBlock` |
| `lifecycle_rule` (TypeList, no MaxItems) | Repeating block; keep as block | `ListNestedBlock` |
| `expiration` inside `lifecycle_rule` (TypeList, MaxItems:1) | Nested inside lifecycle block; keep for state-path compat | `ListNestedBlock + listvalidator.SizeAtMost(1)` |
| `noncurrent_version_expiration` (TypeList, MaxItems:1) | Same as above | `ListNestedBlock + listvalidator.SizeAtMost(1)` |

**State upgrade**: SchemaVersion is 0; no upgraders needed.

**Import shape**: SDKv2 used `ImportStatePassthroughContext` but actual ID is composite (`clusterOrRegion:label`). Framework `ImportState` parses the ID, validates structure, then writes it to `path.Root("id")`. Read uses `decodeBucketIDFromModel` to extract parts.

---

## Key changes

### resource.go

- Removed all `github.com/hashicorp/terraform-plugin-sdk/v2` imports.
- New struct: `BucketResource` implementing `resource.Resource`, `ResourceWithConfigure`, `ResourceWithImportState`.
- New typed model structs: `BucketResourceModel`, `BucketCertModel`, `LifecycleRuleModel`, `ExpirationModel`, `NoncurrentVersionExpirationModel`.
- `acl` default (`"private"`) migrated to `Default: stringdefault.StaticString("private")` with `Computed: true` — NOT a plan modifier.
- `ForceNew: true` attributes migrated to `stringplanmodifier.RequiresReplace()`.
- Computed stable attributes use `stringplanmodifier.UseStateForUnknown()`.
- `d.Get`/`d.Set`/`d.HasChange` replaced with typed model reads from `req.Plan`/`req.State`; diagnostics on `resp.Diagnostics`.
- `d.SetId("")` → `resp.State.RemoveResource(ctx)` in Read on 404.
- Delete reads from `req.State` (not `req.Plan`).
- `GetObjKeys` (SDKv2 `*schema.ResourceData` based) replaced by inline `resolveObjKeys` using `FrameworkProviderModel`.
- `S3ConnectionFromData` (SDKv2 based) replaced by inline `s3ConnectionFromModel`.
- `GetRegionOrCluster` (SDKv2 based) replaced by inline logic in `decodeBucketIDFromModel`.
- `DecodeBucketID` signature changed from `(ctx, id, *schema.ResourceData)` to `(ctx, id)`.
- Cert update logic fixed: `updateBucketCert` now receives old cert from prior state to correctly handle cert removal.
- `createTempKeys` / `cleanUpTempKeys` inlined (they are unexported in the `obj` package).

### resource_test.go

- Removed `github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema` import.
- `checkBucketExists` and `checkBucketDestroy` changed to use `acceptance.GetTestClient()` instead of `acceptance.TestAccSDKv2Provider.Meta().(*helper.ProviderMeta).Client`.
- `DecodeBucketID` calls updated to 2-arg signature (no `schema.ResourceData{}`).
- Import steps updated with `ImportStateVerifyIgnore` for `cert.0.private_key` and `cert.0.certificate` (sensitive, not read back from API).
- `TestAccResourceBucket_forceDelete` PreConfig now uses `acceptance.GetTestClient()`.

---

## Pitfalls avoided

- `Default: stringdefault.StaticString("private")` is a separate field, not placed inside `PlanModifiers`.
- `Delete` reads from `req.State`, not `req.Plan` (which is null on Delete).
- `tfsdk:"..."` struct tags match schema attribute names exactly.
- No SDKv2 imports remain in either output file.
- Cert deletion on removal handled explicitly via old/new cert comparison.
