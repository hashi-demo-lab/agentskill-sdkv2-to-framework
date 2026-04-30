package openstack

// This file implements resource.ResourceWithUpgradeState for
// objectStorageContainerV1Resource.
//
// History
// -------
// SchemaVersion 0  (SDKv2 <= v1.x)
//   "versioning" was a TypeSet block with sub-fields "type" and "location".
//   "storage_class" was absent.
//
// SchemaVersion 1  (SDKv2 >= v2.x, and this framework port)
//   "versioning" became a plain bool (default false).
//   The old "versioning" block was renamed to "versioning_legacy".
//   "storage_class" was added (Optional+Computed).
//
// The SDKv2 upgrader chain was a single step: V0 → V1.  The framework uses
// the same single-step semantics — one upgrader keyed at 0 that produces the
// current (V1) state directly.

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UpgradeState returns the map of prior-version upgraders.
// The framework calls the upgrader keyed at the version number stored in
// state.  Each upgrader must produce state that matches the *current* schema
// (Version 1), not an intermediate version.
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Upgrade from schema version 0 → current (1).
		0: {
			PriorSchema:   containerSchemaV0(),
			StateUpgrader: upgradeContainerStateV0toV1,
		},
	}
}

// ---------------------------------------------------------------------------
// V0 prior schema
// ---------------------------------------------------------------------------

// containerSchemaV0 returns the framework schema that mirrors the SDKv2
// schema at version 0.  The framework uses this to deserialise the prior
// state — every attribute name and type must match exactly what was written
// by the SDKv2 provider.
//
// V0 differences from V1:
//   - "versioning" is a SetNestedBlock with type/location sub-attrs (not a bool).
//   - "versioning_legacy" does not exist.
//   - "storage_class" does not exist.
func containerSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"container_read": schema.StringAttribute{
				Optional: true,
			},
			"container_sync_to": schema.StringAttribute{
				Optional: true,
			},
			"container_sync_key": schema.StringAttribute{
				Optional: true,
			},
			"container_write": schema.StringAttribute{
				Optional: true,
			},
			"content_type": schema.StringAttribute{
				Optional: true,
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
			"force_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
			},
			"storage_policy": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
		},
		Blocks: map[string]schema.Block{
			// In V0, "versioning" was the legacy block.  The SDKv2 type was
			// TypeSet with MaxItems:1.  We represent it as a SetNestedBlock
			// here so the framework can deserialise it correctly.
			"versioning": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
						},
						"location": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// V0 model (matches containerSchemaV0)
// ---------------------------------------------------------------------------

// containerV0Model is the typed model for V0 prior state.
type containerV0Model struct {
	ID               types.String          `tfsdk:"id"`
	Region           types.String          `tfsdk:"region"`
	Name             types.String          `tfsdk:"name"`
	ContainerRead    types.String          `tfsdk:"container_read"`
	ContainerSyncTo  types.String          `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String          `tfsdk:"container_sync_key"`
	ContainerWrite   types.String          `tfsdk:"container_write"`
	ContentType      types.String          `tfsdk:"content_type"`
	// "versioning" was the TypeSet block in V0.
	Versioning       []versioningLegacyModel `tfsdk:"versioning"`
	Metadata         types.Map               `tfsdk:"metadata"`
	ForceDestroy     types.Bool              `tfsdk:"force_destroy"`
	StoragePolicy    types.String            `tfsdk:"storage_policy"`
}

// ---------------------------------------------------------------------------
// Upgrader function: V0 → V1 (current)
// ---------------------------------------------------------------------------

// upgradeContainerStateV0toV1 transforms a V0 state into a V1 state in a
// single step.
//
// The SDKv2 upgrader (resourceObjectStorageContainerStateUpgradeV0) performed
// the following transformation on the raw map:
//
//	rawState["versioning_legacy"] = rawState["versioning"]
//	rawState["versioning"]        = false
//
// This function does the same thing using typed model structs so the
// framework can validate the result against the current schema.
func upgradeContainerStateV0toV1(
	ctx context.Context,
	req resource.UpgradeStateRequest,
	resp *resource.UpgradeStateResponse,
) {
	// 1. Read prior state into the V0 model.
	var prior containerV0Model
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 2. Build the current (V1) model from the V0 data.
	//
	//    Key changes:
	//    - prior.Versioning (block)  → current.VersioningLegacy
	//    - current.Versioning (bool) → false  (the prior schema had no bool
	//      versioning field; the SDKv2 upgrader set it to false)
	//    - current.StorageClass      → null/unknown (field did not exist in V0;
	//      the next Read cycle will populate it from the API)
	current := objectStorageContainerV1Model{
		ID:               prior.ID,
		Region:           prior.Region,
		Name:             prior.Name,
		ContainerRead:    prior.ContainerRead,
		ContainerSyncTo:  prior.ContainerSyncTo,
		ContainerSyncKey: prior.ContainerSyncKey,
		ContainerWrite:   prior.ContainerWrite,
		ContentType:      prior.ContentType,
		Versioning:       types.BoolValue(false),
		VersioningLegacy: prior.Versioning, // rename: versioning → versioning_legacy
		Metadata:         prior.Metadata,
		ForceDestroy:     prior.ForceDestroy,
		StoragePolicy:    prior.StoragePolicy,
		// storage_class did not exist in V0; initialise as null so the next
		// Read cycle fetches the real value from the API.
		StorageClass: types.StringNull(),
	}

	// 3. Write the upgraded state.
	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
