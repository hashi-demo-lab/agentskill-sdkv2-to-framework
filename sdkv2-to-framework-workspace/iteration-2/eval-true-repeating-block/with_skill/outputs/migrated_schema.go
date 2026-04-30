package openstack

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// computeVolumeAttachV2Resource is the framework resource type.
type computeVolumeAttachV2Resource struct{}

// Schema returns the framework schema for openstack_compute_volume_attach_v2.
//
// Migration notes:
//   - All primitive attributes map 1:1 to their framework equivalents.
//   - ForceNew: true becomes stringplanmodifier.RequiresReplace() (or the bool equivalent).
//   - vendor_options was TypeSet with MinItems:1, MaxItems:1 in SDKv2. Because MinItems > 0
//     this is a "true repeating block" and is kept as schema.SetNestedBlock to preserve the
//     practitioner HCL block syntax. Converting to SingleNestedAttribute would change the
//     user-facing syntax and constitute a breaking change.
//   - The ignore_volume_confirmation Default:false inside vendor_options becomes
//     booldefault.StaticBool(false) — Default is NOT a plan modifier in the framework.
//   - Timeouts are handled via the terraform-plugin-framework-timeouts helper package.
func (r *computeVolumeAttachV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
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

			// Timeouts block — managed via terraform-plugin-framework-timeouts.
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				CreateDefault: 10 * time.Minute,
				Delete: true,
				DeleteDefault: 10 * time.Minute,
			}),
		},

		// vendor_options is kept as a SetNestedBlock rather than converted to
		// SingleNestedAttribute because the original schema sets MinItems:1, making
		// it a true repeating block. Keeping it as a block preserves the HCL block
		// syntax that practitioners already use:
		//
		//   vendor_options {
		//     ignore_volume_confirmation = true
		//   }
		//
		// Switching to SingleNestedAttribute would change that syntax and break
		// existing configurations without a major-version bump.
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
