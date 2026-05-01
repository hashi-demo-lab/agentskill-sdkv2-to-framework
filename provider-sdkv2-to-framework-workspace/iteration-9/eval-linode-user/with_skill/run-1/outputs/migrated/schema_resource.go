package user

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// frameworkResourceSchema is the Plugin Framework schema for linode_user.
//
// Block decision for global_grants:
//   - SDKv2: TypeList, MaxItems:1, nested Elem (*schema.Resource) → practitioners use
//     block syntax (global_grants { ... }) in existing configs.
//   - Decision: keep as SingleNestedBlock to avoid a breaking HCL syntax change.
//
// Entity grant sets (domain_grant, etc.):
//   - SDKv2: TypeSet, nested Elem (*schema.Resource) → no MaxItems:1 constraint.
//   - Decision: migrate to SetNestedAttribute (no block syntax required; id/permissions
//     are not user-visible block tokens, so attribute syntax is fine and preferred).
//
// Default: false fields:
//   - All boolean optional+computed fields with Default:false use booldefault.StaticBool(false)
//     from the resource/schema/booldefault sub-package.  Default is NOT placed in PlanModifiers.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"email": schema.StringAttribute{
			Description: "The email of the user.",
			Required:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"username": schema.StringAttribute{
			Description: "The username of the user.",
			Required:    true,
		},
		"restricted": schema.BoolAttribute{
			Description: "If true, the user must be explicitly granted access to platform actions and entities.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"user_type": schema.StringAttribute{
			Description: "The type of this user.",
			Computed:    true,
		},
		"ssh_keys": schema.ListAttribute{
			Description: "SSH keys to add to the user profile.",
			Computed:    true,
			ElementType: types.StringType,
		},
		"tfa_enabled": schema.BoolAttribute{
			Description: "If the User has Two Factor Authentication (TFA) enabled.",
			Computed:    true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		},

		// Entity grant sets — SetNestedAttribute (no HCL block syntax was used for these).
		"domain_grant":       resourceGrantEntitySetAttribute(),
		"firewall_grant":     resourceGrantEntitySetAttribute(),
		"image_grant":        resourceGrantEntitySetAttribute(),
		"linode_grant":       resourceGrantEntitySetAttribute(),
		"longview_grant":     resourceGrantEntitySetAttribute(),
		"nodebalancer_grant": resourceGrantEntitySetAttribute(),
		"stackscript_grant":  resourceGrantEntitySetAttribute(),
		"volume_grant":       resourceGrantEntitySetAttribute(),
		"vpc_grant":          resourceGrantEntitySetAttribute(),
	},

	// global_grants — kept as SingleNestedBlock because practitioners use block syntax.
	Blocks: map[string]schema.Block{
		"global_grants": schema.SingleNestedBlock{
			Description: "A structure containing the Account-level grants a User has.",
			Attributes:  resourceGrantsGlobalBlockAttributes(),
		},
	},
}

// resourceGrantEntitySetAttribute returns the SetNestedAttribute definition used
// for all per-entity grant sets.
func resourceGrantEntitySetAttribute() schema.SetNestedAttribute {
	return schema.SetNestedAttribute{
		Description: "A set containing all of the user's active grants.",
		Optional:    true,
		Computed:    true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.Int64Attribute{
					Description: "The ID of the entity this grant applies to.",
					Required:    true,
				},
				"permissions": schema.StringAttribute{
					Description: "The level of access this User has to this entity. If null, this User has no access.",
					Required:    true,
				},
			},
		},
	}
}

// resourceGrantsGlobalBlockAttributes returns the attribute map for the global_grants block.
// Every boolean field that had Default: false in SDKv2 uses booldefault.StaticBool(false).
func resourceGrantsGlobalBlockAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"account_access": schema.StringAttribute{
			Description: "The level of access this User has to Account-level actions, like billing information. " +
				"A restricted User will never be able to manage users.",
			Optional: true,
			Computed: true,
		},
		"add_domains": schema.BoolAttribute{
			Description: "If true, this User may add Domains.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_databases": schema.BoolAttribute{
			Description: "If true, this User may add Databases.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_firewalls": schema.BoolAttribute{
			Description: "If true, this User may add Firewalls.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_images": schema.BoolAttribute{
			Description: "If true, this User may add Images.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_linodes": schema.BoolAttribute{
			Description: "If true, this User may create Linodes.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_longview": schema.BoolAttribute{
			Description: "If true, this User may create Longview clients and view the current plan.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_nodebalancers": schema.BoolAttribute{
			Description: "If true, this User may add NodeBalancers.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_stackscripts": schema.BoolAttribute{
			Description: "If true, this User may add StackScripts.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_volumes": schema.BoolAttribute{
			Description: "If true, this User may add Volumes.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"add_vpcs": schema.BoolAttribute{
			Description: "If true, this User may add Virtual Private Clouds (VPCs).",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"cancel_account": schema.BoolAttribute{
			Description: "If true, this User may cancel the entire Account.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
		"longview_subscription": schema.BoolAttribute{
			Description: "If true, this User may manage the Account's Longview subscription.",
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
		},
	}
}
