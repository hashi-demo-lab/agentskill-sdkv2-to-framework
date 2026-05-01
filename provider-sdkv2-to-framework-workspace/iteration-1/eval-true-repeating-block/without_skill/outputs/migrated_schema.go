package openstack

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// Compile-time check that the type satisfies the resource.ResourceWithConfigure
// and resource.ResourceWithImportState interfaces (not shown here; only the
// schema is migrated per the task scope).

// Timeouts used in Create / Delete operations (kept for reference; in the
// framework these are usually declared via the timeouts framework helper or
// stored as constants).
const (
	computeVolumeAttachV2CreateTimeout = 10 * time.Minute
	computeVolumeAttachV2DeleteTimeout = 10 * time.Minute
)

// resourceComputeVolumeAttachV2Schema returns the terraform-plugin-framework
// schema that is equivalent to the terraform-plugin-sdk/v2 schema defined in
// resourceComputeVolumeAttachV2().
//
// Key migration decisions
// -----------------------
// 1. vendor_options was TypeSet / MinItems:1 / MaxItems:1 in the SDK.
//    Because MinItems > 0 this qualifies as a "true repeating block" — it must
//    remain a *block* (schema.SetNestedBlock) rather than being flattened into
//    a nested attribute.  Keeping block syntax preserves the practitioner HCL:
//
//      vendor_options {
//        ignore_volume_confirmation = true
//      }
//
//    Converting it to a nested attribute would require practitioners to rewrite
//    their configuration to use the object/set-of-objects attribute syntax,
//    which is a breaking change.
//
// 2. ForceNew on SDK attributes maps to planmodifier.RequiresReplace() in the
//    framework.
//
// 3. schema.TypeBool Default:false maps to booldefault.StaticBool(false).
//
// 4. Computed+Optional is expressed with both Computed:true and Optional:true
//    on the framework attribute.
func resourceComputeVolumeAttachV2Schema(_ context.Context) schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
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

			// id is managed automatically by the framework; no explicit
			// declaration is needed, but it is kept here as documentation.
			// "id": schema.StringAttribute{Computed: true},
		},

		// vendor_options is declared as a SetNestedBlock (not a nested
		// attribute) because the original SDK definition has MinItems:1.
		// A MinItems > 0 block is a "true repeating block": practitioners write
		// HCL block syntax for it, and changing to an attribute would be a
		// breaking change to their configurations.
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

// Schema implements resource.Resource.
func (r *computeVolumeAttachV2Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = resourceComputeVolumeAttachV2Schema(ctx)
}
