package openstack

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
)

// Schema returns the framework schema for openstack_compute_volume_attach_v2.
//
// Migration notes:
//   - vendor_options was TypeSet with MinItems:1, MaxItems:1 in SDKv2.
//     It is kept as a SetNestedBlock to preserve practitioner block HCL syntax.
//     MinItems:1 is expressed via a setvalidator.SizeBetween(1,1) validator on
//     the block (the framework cannot mark blocks Required; validators are the
//     idiomatic substitute).
//   - ignore_volume_confirmation had Default:false; in the framework a Default
//     requires Computed:true plus booldefault.StaticBool(false).
//   - All ForceNew:true fields become RequiresReplace() plan modifiers.
//   - device (Computed+Optional) gets UseStateForUnknown() so it does not show
//     "(known after apply)" on plans where the value is stable.
//   - region (Computed+Optional+ForceNew) gets both UseStateForUnknown() and
//     RequiresReplace().
func (r *computeVolumeAttachV2Resource) Schema(
	ctx context.Context,
	req resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},

			"instance_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"volume_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"device": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"multiattach": schema.BoolAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},

			"tag": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},

		Blocks: map[string]schema.Block{
			// vendor_options is kept as a SetNestedBlock to preserve the block
			// HCL syntax practitioners already use:
			//
			//   vendor_options {
			//     ignore_volume_confirmation = true
			//   }
			//
			// SDKv2 had MinItems:1, MaxItems:1 on a TypeSet.  The framework
			// cannot mark blocks Required, so the MinItems/MaxItems constraints
			// are expressed with a setvalidator.SizeBetween(1,1) validator.
			// ForceNew on the parent set becomes RequiresReplace() on the block.
			"vendor_options": schema.SetNestedBlock{
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
				Validators: []validator.Set{
					setvalidator.SizeBetween(1, 1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						// Default:false in SDKv2 requires Computed:true here so
						// the framework can insert the default into the plan.
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
