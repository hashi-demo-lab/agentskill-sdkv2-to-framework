package openstack

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &computeVolumeAttachV2Resource{}

// computeVolumeAttachV2Resource defines the resource implementation.
type computeVolumeAttachV2Resource struct {
	config *Config
}

// vendorOptionsModel is the model for the vendor_options block.
type vendorOptionsModel struct {
	IgnoreVolumeConfirmation types.Bool `tfsdk:"ignore_volume_confirmation"`
}

// computeVolumeAttachV2Model is the model for the resource.
type computeVolumeAttachV2Model struct {
	ID            types.String         `tfsdk:"id"`
	Region        types.String         `tfsdk:"region"`
	InstanceID    types.String         `tfsdk:"instance_id"`
	VolumeID      types.String         `tfsdk:"volume_id"`
	Device        types.String         `tfsdk:"device"`
	Multiattach   types.Bool           `tfsdk:"multiattach"`
	Tag           types.String         `tfsdk:"tag"`
	VendorOptions []vendorOptionsModel `tfsdk:"vendor_options"`
}

// Schema returns the framework schema for openstack_compute_volume_attach_v2.
//
// vendor_options is a TypeSet with MinItems: 1 in SDKv2. It is kept as a
// SetNestedBlock (not converted to SetNestedAttribute) to preserve the block
// HCL syntax that practitioners already use in their configurations:
//
//	vendor_options {
//	  ignore_volume_confirmation = true
//	}
//
// Converting to a nested attribute would require practitioners to rewrite their
// HCL to use assignment syntax (vendor_options = { ... }), which is a
// backward-incompatible breaking change.
func (r *computeVolumeAttachV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []schema.PlanModifier{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"instance_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []schema.PlanModifier{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"volume_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []schema.PlanModifier{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"device": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"multiattach": schema.BoolAttribute{
				Optional: true,
				PlanModifiers: []schema.PlanModifier{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"tag": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []schema.PlanModifier{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},

		// vendor_options is kept as a SetNestedBlock rather than a
		// SetNestedAttribute to preserve backward-compatible block HCL syntax.
		// TypeSet with Elem: &schema.Resource{} in SDKv2 maps to SetNestedBlock
		// in the framework when practitioner HCL compatibility must be maintained.
		Blocks: map[string]schema.Block{
			"vendor_options": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"ignore_volume_confirmation": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
						},
					},
				},
			},
		},
	}
}
