package openstack

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// computeVolumeAttachV2Resource implements resource.Resource.
// Only the Schema method is provided here; CRUD methods are out of scope.
type computeVolumeAttachV2Resource struct{}

// Schema returns the terraform-plugin-framework schema for openstack_compute_volume_attach_v2.
func (r *computeVolumeAttachV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			// id is a computed composite "instanceID/attachmentID" set by Create.
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			// region: optional+computed, forces replacement (same as SDKv2 ForceNew).
			// UseStateForUnknown keeps a stable value across plans when unset.
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

			// device is optional+computed: can be specified or assigned by Nova.
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
		},

		Blocks: map[string]schema.Block{
			// vendor_options was TypeSet + Elem: &schema.Resource{} with MinItems:1, MaxItems:1.
			//
			// Decision: TypeSet of nested Resource → SetNestedBlock.
			//   - Practitioners wrote:  vendor_options { ignore_volume_confirmation = false }
			//   - Switching to a nested attribute (SetNestedAttribute) would require:
			//       vendor_options = [{ ignore_volume_confirmation = false }]
			//     which is a breaking HCL change for existing configs.
			//   - Therefore keep as SetNestedBlock to preserve practitioner HCL.
			//
			// MinItems:1 is preserved via setvalidator.SizeBetween(1, 1).
			// MaxItems:1 is also preserved via the same validator.
			//
			// Note: blocks cannot carry RequiresReplace plan modifiers directly.
			// The ForceNew behaviour from the SDKv2 schema must be handled in
			// ModifyPlan or via resource-level logic (out of scope for schema migration).
			"vendor_options": schema.SetNestedBlock{
				Validators: []validator.Set{
					setvalidator.SizeBetween(1, 1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						// Default: false from SDKv2 is preserved via booldefault.StaticBool.
						// Computed: true is required when Default is set.
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
