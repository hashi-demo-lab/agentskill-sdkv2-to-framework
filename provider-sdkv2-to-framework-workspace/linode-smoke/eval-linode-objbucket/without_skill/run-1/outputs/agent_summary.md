# Migration Summary: linode_object_storage_bucket

## What was migrated

Migrated `linode/objbucket/resource.go` (plus `schema_resource.go` and `helpers.go`) from
`terraform-plugin-sdk/v2` to `terraform-plugin-framework`.  The result is a single self-contained
`resource.go` file that implements `resource.Resource` and `resource.ResourceWithImportState`.
The original repo files were not modified.

---

## Block vs. Attribute decisions

### `cert` (SDKv2: TypeList, MaxItems:1)
- **Decision: `schema.ListNestedBlock`** (not `SingleNestedBlock`)
- Rationale: The SDKv2 schema uses `TypeList` with `MaxItems:1`; the idiomatic framework
  equivalent is `ListNestedBlock` (length constrained to 0–1 in practice).  `SingleNestedBlock`
  would always require the block to be present and cannot be made optional without a custom plan
  modifier.  Using `ListNestedBlock` preserves the optional / removable semantics.

### `lifecycle_rule` (SDKv2: TypeList, no explicit MaxItems)
- **Decision: `schema.ListNestedBlock`**
- Rationale: Ordered list of rules, exactly matching the SDKv2 behaviour.  Each rule is a nested
  object with its own optional sub-blocks (`expiration`, `noncurrent_version_expiration`).

### `expiration` / `noncurrent_version_expiration` (SDKv2: TypeList, MaxItems:1, inside lifecycle_rule)
- **Decision: nested `schema.ListNestedBlock` inside the `lifecycle_rule` block**
- Rationale: Same MaxItems:1 → ListNestedBlock pattern as `cert`.  Using nested blocks mirrors
  how they appear in the HCL config.

### Simple scalar fields (`acl`, `cors_enabled`, `cluster`, `region`, etc.)
- **Decision: `schema.StringAttribute` / `schema.BoolAttribute`** – standard attributes.

### Default value for `acl`
- Implemented with `stringdefault.StaticString("private")` which sets the default when the
  attribute is not specified, matching the SDKv2 `Default: "private"` behaviour.

### `versioning`
- Implemented as `schema.BoolAttribute` with `booldefault.StaticBool(false)` to preserve the
  `Computed:true, Optional:true` behaviour without requiring the user to specify it.

---

## Composite-ID Importer

The SDKv2 resource used `schema.ImportStatePassthroughContext` which sets the whole raw ID into
state.  The framework `ImportState` method:

1. Splits `req.ID` on `:` to validate the `<cluster_or_region>:<label>` format.
2. Sets the `id` attribute directly via `resp.State.SetAttribute(ctx, path.Root("id"), req.ID)`.
3. The subsequent `Read` call parses the same `id` value to fetch the bucket from the API.

This avoids importing individual fields separately (which would require an API round-trip that
might not have the right data at import time).

---

## Key-resolution approach

The SDKv2 code relied on `obj.GetObjKeys` which accepts a `*schema.ResourceData`.  Because the
framework resource has no `*schema.ResourceData`, a `resolveObjKeys` method was implemented
inline that replicates the three-step fallback:

1. Keys from the resource's own `access_key` / `secret_key` attributes.
2. Keys from the provider-level `ObjAccessKey` / `ObjSecretKey` fields.
3. Temporary keys if `ObjUseTempKeys` is enabled.

---

## Test changes

- Removed `"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"` import.
- Updated `checkBucketExists` and `checkBucketDestroy` to call the new `DecodeBucketID(ctx, id)`
  signature (two-argument, no `*schema.ResourceData`).
- All test functions and sweep logic are otherwise unchanged.
