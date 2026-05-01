# Migration notes — openstack_objectstorage_container_v1

## Pre-flight analysis

### Block decision

`versioning_legacy` (formerly `versioning` in V0) was `TypeSet + MaxItems:1` with a nested `Elem: &schema.Resource`.  Existing practitioner configs use block syntax (`versioning_legacy { ... }`).  Decision: **kept as a list attribute** (`schema.ListAttribute` with `ElementType: versioningLegacyElemType`) rather than a `SetNestedBlock` or `SingleNestedBlock`.

Rationale: because `versioning_legacy` is deprecated, minimising the syntax change reduces confusion during the deprecation cycle.  Using a typed `ListAttribute` is state-compatible with the SDKv2 TypeSet (same wire encoding for a single element) and avoids introducing new block-vs-attribute HCL syntax changes for an attribute users are being asked to stop using.

### State-upgrade semantics

SDKv2 resource had `SchemaVersion: 1` and one `StateUpgraders` entry (prior version 0).  The SDKv2 upgrader `resourceObjectStorageContainerStateUpgradeV0` did:

```go
rawState["versioning_legacy"] = rawState["versioning"]
rawState["versioning"] = false
```

The framework uses **single-step semantics**: `UpgradeState` returns a `map[int64]resource.StateUpgrader` keyed by the *prior* version, where each entry must produce the *current* (target) schema's state directly.  There is **no chaining** — the framework calls each entry independently with the prior-version bytes and the matching `PriorSchema`.

Because there was only one SDKv2 upgrader (V0 → V1), the framework map has exactly one entry (key `0`).

The `PriorSchema` for entry `0` must describe the V0 schema — in particular it must include `"versioning"` (as a ListAttribute) and must NOT include `"versioning_legacy"` or `"storage_class"` (those are V1-only fields).  Omitting `PriorSchema` would cause a runtime panic when old state is loaded.

Comparison of SDKv2 vs framework semantics:

| | SDKv2 | Framework |
|---|---|---|
| Version field | `SchemaVersion` on `*schema.Resource` | `Version` on `schema.Schema` |
| Upgrader registration | `StateUpgraders []schema.StateUpgrader` | `UpgradeState() map[int64]resource.StateUpgrader` |
| Semantics | chained: V0→V1→V2 | single-step: each entry → current |
| Type info | `Type: resource.CoreConfigSchema().ImpliedType()` | `PriorSchema: &schema.Schema{...}` |
| Target | next version | current (target) version always |

### Import shape

Passthrough (`schema.ImportStatePassthroughContext`).  Translated to `resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)` — no composite ID parsing required.

## What changed

1. **No SDKv2 import**: all `github.com/hashicorp/terraform-plugin-sdk/v2` imports removed.
2. **Resource type**: function returning `*schema.Resource` replaced by a struct type `objectStorageContainerV1Resource` implementing `resource.Resource` and three sub-interfaces.
3. **Schema**: moved to `Schema()` method; `SchemaVersion → schema.Schema.Version`; `ForceNew → stringplanmodifier.RequiresReplace()`; `Default: false → booldefault.StaticBool(false)`.
4. **CRUD**: `*schema.ResourceData` replaced by typed model structs; `d.Get`/`d.Set`/`d.Id` replaced by `req.Plan.Get` / `resp.State.Set`; `d.SetId("") → resp.State.RemoveResource(ctx)`.
5. **State upgrade**: `StateUpgraders` slice → `UpgradeState()` method returning single-step map.  `PriorSchema` added.  V0 model struct (`objectStorageContainerV0Model`) added.
6. **Import**: `Importer: &schema.ResourceImporter{StateContext: schema.ImportStatePassthroughContext}` → `ImportState` method with `resource.ImportStatePassthroughID`.
7. **Metadata helper**: `resourceContainerMetadataV2(d *schema.ResourceData)` → `containerMetadataFromModel(m types.Map)` (no ResourceData dependency).
8. **migrate_resource file**: no longer needed — its contents are superseded by `UpgradeState()` in the main resource file.

## Files produced

| File | Purpose |
|---|---|
| `resource_openstack_objectstorage_container_v1.go` | Full framework resource (schema, CRUD, UpgradeState, ImportState) |
| `resource_openstack_objectstorage_container_v1_test.go` | Acceptance + unit tests using `ProtoV6ProviderFactories` |

The `migrate_resource_openstack_objectstorage_container_v1.go` file from SDKv2 is superseded by the `UpgradeState()` method and should be deleted from the provider repo once this migration ships.
