package openstack

// Framework migration of the schema for openstack_compute_volume_attach_v2.
//
// Scope: schema only. CRUD methods, timeouts wiring inside CRUD, and
// acceptance/unit tests are intentionally NOT migrated here.
//
// Key decisions (see reasoning.md for full rationale):
//   - vendor_options stays a block (SetNestedBlock) to preserve practitioner
//     HCL: `vendor_options { ignore_volume_confirmation = true }`.
//   - SDKv2 TypeSet → SetNestedBlock (NOT ListNestedBlock), so set-uniqueness
//     semantics are preserved. The framework computes set uniqueness
//     internally; the SDKv2 `Set: hashFunc` field has no equivalent and is
//     not needed.
//   - MinItems:1 / MaxItems:1 are expressed as block-level validators
//     (setvalidator.SizeAtLeast(1) and setvalidator.SizeAtMost(1)).
//   - SDKv2 ForceNew becomes a per-attribute or per-block PlanModifier
//     using *planmodifier.RequiresReplace() from the kind-specific package.
//   - SDKv2 Default:false on ignore_volume_confirmation becomes
//     booldefault.StaticBool(false); the attribute therefore must be
//     Computed in addition to Optional.
//   - Timeouts move out of the resource definition into a schema block
//     using terraform-plugin-framework-timeouts. Block (not Attributes)
//     form preserves practitioners' existing `timeouts { ... }` HCL.

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// computeVolumeAttachV2Resource is the framework resource type. CRUD methods
// are out of scope for this migration step but the type is declared so the
// Schema method has a receiver.
type computeVolumeAttachV2Resource struct {
	// meta carries the provider's configured clients, populated in
	// Configure. Left untyped here because Configure is out of scope.
}

// computeVolumeAttachV2Model is the typed state/plan model for the resource.
// It is included so the schema's tfsdk tags have a corresponding Go shape.
type computeVolumeAttachV2Model struct {
	ID            types.String   `tfsdk:"id"`
	Region        types.String   `tfsdk:"region"`
	InstanceID    types.String   `tfsdk:"instance_id"`
	VolumeID      types.String   `tfsdk:"volume_id"`
	Device        types.String   `tfsdk:"device"`
	Multiattach   types.Bool     `tfsdk:"multiattach"`
	Tag           types.String   `tfsdk:"tag"`
	VendorOptions types.Set      `tfsdk:"vendor_options"`
	Timeouts      timeouts.Value `tfsdk:"timeouts"`
}

// vendorOptionsModel mirrors the SetNestedBlock element shape.
type vendorOptionsModel struct {
	IgnoreVolumeConfirmation types.Bool `tfsdk:"ignore_volume_confirmation"`
}

// Schema implements resource.Resource for openstack_compute_volume_attach_v2.
func (r *computeVolumeAttachV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			// Implicit "id" attribute — the framework requires resources to
			// declare it explicitly. Carries UseStateForUnknown so plans
			// don't churn after the initial Create.
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

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

			// device: SDKv2 Computed + Optional, no ForceNew. The compute
			// API may assign a device path if the practitioner doesn't.
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
			// vendor_options stays as a block to preserve practitioner HCL:
			//
			//     vendor_options {
			//       ignore_volume_confirmation = true
			//     }
			//
			// SDKv2 used TypeSet, so the framework equivalent is
			// SetNestedBlock (NOT ListNestedBlock) — set semantics matter
			// for the comparison/uniqueness rules even with MaxItems:1.
			//
			// MinItems:1 / MaxItems:1 from SDKv2 are expressed as block
			// validators. ForceNew on the SDKv2 block becomes a
			// set-kind RequiresReplace plan modifier on the block.
			"vendor_options": schema.SetNestedBlock{
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.SizeAtMost(1),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"ignore_volume_confirmation": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							// Default requires Computed:true on the
							// attribute so the framework can populate
							// the plan when the practitioner omits it.
							Default: booldefault.StaticBool(false),
						},
					},
				},
			},

			// timeouts: SDKv2 had Create + Delete. Use timeouts.Block
			// (not timeouts.Attributes) so practitioners keep block HCL:
			//   timeouts {
			//     create = "10m"
			//     delete = "10m"
			//   }
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}
