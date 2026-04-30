package openstack

// upgrade_objectstorage_container_v1.go implements resource.ResourceWithUpgradeState
// for objectStorageContainerV1Resource.
//
// Schema history
// --------------
// SchemaVersion 0  (SDKv2 provider releases prior to the versioning refactor)
//   - "versioning" was a TypeSet block with sub-fields "type" and "location".
//   - "versioning_legacy" did not exist.
//   - "storage_class" did not exist.
//
// SchemaVersion 1  (current — SDKv2 post-refactor and this framework port)
//   - "versioning" became a plain bool (default false).
//   - The old "versioning" block was renamed to "versioning_legacy".
//   - "storage_class" was added (Optional + Computed).
//
// SDKv2 upgrader chain: single step V0 → V1.
// Framework: one upgrader keyed at 0, producing V1 (current) state directly.
// No chaining is needed — the framework uses single-step semantics.

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
// state. Each upgrader must produce state matching the *current* schema
// (Version 1), not an intermediate version.
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Upgrade schema version 0 → current (1).
		0: {
			PriorSchema:   containerSchemaV0(),
			StateUpgrader: upgradeContainerStateV0toV1,
		},
	}
}

// ---------------------------------------------------------------------------
// V0 prior schema
// ---------------------------------------------------------------------------

// containerSchemaV0 returns the framework schema that mirrors the SDKv2 schema
// at version 0. The framework uses this to deserialise the prior state — every
// attribute name and type must match exactly what was written by the SDKv2
// provider at that schema version.
//
// V0 differences from V1 (current):
//   - "versioning" is a SetNestedBlock with "type" and "location" sub-attrs.
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
			// In V0, "versioning" was the legacy block (TypeSet, MaxItems:1).
			// Represent it as a SetNestedBlock so the framework can deserialise
			// it correctly from the stored state.
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
// V0 model (must exactly match containerSchemaV0)
// ---------------------------------------------------------------------------

// containerV0Model is the typed model for V0 prior state.
// The tfsdk tags must exactly match containerSchemaV0 attribute names and
// types — the framework deserialises through the prior schema.
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
// Upgrader: V0 → V1 (current)
// ---------------------------------------------------------------------------

// upgradeContainerStateV0toV1 transforms a V0 state into current (V1) state
// in a single step.
//
// The SDKv2 upgrader (resourceObjectStorageContainerStateUpgradeV0) performed:
//
//	rawState["versioning_legacy"] = rawState["versioning"]
//	rawState["versioning"]        = false
//
// This function does the same transformation using typed model structs so the
// framework can validate the result against the current schema.
//
// Note: the framework's UpgradeStateRequest does not carry provider meta, so
// no API calls are made here. The new "storage_class" field (absent in V0) is
// initialised as null; the next Read cycle will populate it from the API.
func upgradeContainerStateV0toV1(
	ctx context.Context,
	req resource.UpgradeStateRequest,
	resp *resource.UpgradeStateResponse,
) {
	// 1. Read prior (V0) state into the V0 model.
	var prior containerV0Model
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 2. Build the current (V1) model from V0 data.
	//
	//    Key transformations:
	//      prior.Versioning ([]versioningLegacyModel block) → current.VersioningLegacy
	//      current.Versioning (bool)                        → false
	//      current.StorageClass                             → null  (field new in V1)
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
		// Read populates the real value from the API.
		StorageClass: types.StringNull(),
	}

	// 3. Write the upgraded state.
	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
