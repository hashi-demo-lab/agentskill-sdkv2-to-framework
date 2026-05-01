package openstack

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// State upgrade — single-step semantics (V0 → current).
//
// Per references/state-upgrade.md, framework upgraders are NOT chained:
//   - The framework calls each entry in UpgradeState() *independently* with
//     the matching PriorSchema; there is no chain.
//   - The upgrader keyed at version 0 must produce the *current* (V1) state
//     directly, not an intermediate value.
//   - Because this resource only has one prior version (V0), there is exactly
//     one upgrader entry: 0 → current.
//
// SDKv2 had a single `resourceObjectStorageContainerStateUpgradeV0` that
// rewrote raw map state. The framework equivalent reads typed prior state via
// PriorSchema, transforms, and writes typed current state — also in a single
// step.

// objectStorageContainerV1ModelV0 mirrors the V0 schema shape exactly.
//
// In V0 the block was named `versioning` (a TypeSet of {type, location}); in
// V1 it was renamed to `versioning_legacy` and a new scalar `versioning` BOOL
// attribute was added. V0 also lacked `storage_class`.
type objectStorageContainerV1ModelV0 struct {
	ID               types.String `tfsdk:"id"`
	Region           types.String `tfsdk:"region"`
	Name             types.String `tfsdk:"name"`
	ContainerRead    types.String `tfsdk:"container_read"`
	ContainerSyncTo  types.String `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String `tfsdk:"container_sync_key"`
	ContainerWrite   types.String `tfsdk:"container_write"`
	ContentType      types.String `tfsdk:"content_type"`
	Versioning       types.Set    `tfsdk:"versioning"` // V0 block; renamed to versioning_legacy in V1.
	Metadata         types.Map    `tfsdk:"metadata"`
	ForceDestroy     types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy    types.String `tfsdk:"storage_policy"`
}

// priorSchemaV0 returns the V0 schema description so the framework can
// deserialise prior-version state into objectStorageContainerV1ModelV0.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
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
			// V0's `versioning` was a TypeSet block of {type, location}.
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

// upgradeStateFromV0 produces the *current* (V1) state directly. It does NOT
// chain into another upgrader — single-step semantics.
//
// V0 → V1 transformation:
//   - V0's `versioning` block → V1's `versioning_legacy` block (renamed)
//   - V1 introduces a new `versioning` BOOL (default false; matches the SDKv2
//     upgrader, which set `rawState["versioning"] = false`)
//   - V1 introduces a new `storage_class` attribute (no V0 analogue; null
//     until the next refresh picks it up from the API)
func upgradeStateFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior objectStorageContainerV1ModelV0

	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// V0's `versioning` set element shape ({type, location}) is identical to
	// V1's `versioning_legacy`, so we can carry it across verbatim.
	versioningLegacy := prior.Versioning
	if versioningLegacy.IsNull() || versioningLegacy.IsUnknown() {
		versioningLegacy = types.SetNull(versioningLegacyObjectType())
	}

	current := objectStorageContainerV1Model{
		ID:               prior.ID,
		Region:           prior.Region,
		Name:             prior.Name,
		ContainerRead:    prior.ContainerRead,
		ContainerSyncTo:  prior.ContainerSyncTo,
		ContainerSyncKey: prior.ContainerSyncKey,
		ContainerWrite:   prior.ContainerWrite,
		ContentType:      prior.ContentType,
		// New in V1: scalar `versioning` bool. The SDKv2 upgrader set this
		// explicitly to false; we preserve that default here.
		Versioning: types.BoolValue(false),
		// V0 block renamed to `versioning_legacy`.
		VersioningLegacy: versioningLegacy,
		Metadata:         prior.Metadata,
		ForceDestroy:     prior.ForceDestroy,
		StoragePolicy:    prior.StoragePolicy,
		// New in V1: `storage_class`. No V0 analogue; leave null so the next
		// refresh picks it up from the API.
		StorageClass: types.StringNull(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &current)...)
}
