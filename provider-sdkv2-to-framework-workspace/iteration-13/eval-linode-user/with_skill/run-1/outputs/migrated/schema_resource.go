package user

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// frameworkResourceSchema mirrors the SDKv2 schema in
// schema_resource.go, translated to terraform-plugin-framework.
//
// Block-vs-attribute decision:
//
//   - "global_grants" was a TypeList of *schema.Resource with MaxItems: 1.
//     The existing acceptance tests reference "global_grants.0.account_access"
//     and friends, which means user HCL is using block syntax
//     ("global_grants { ... }") and state is shaped as a list. To preserve
//     practitioner HCL and existing state paths we keep it as
//     ListNestedBlock with listvalidator.SizeAtMost(1) (rather than
//     converting to SingleNestedAttribute, which would be a breaking
//     HCL change).
//
//   - "*_grant" were TypeSet of *schema.Resource with no MaxItems. They are
//     true repeating blocks in HCL, so they map to SetNestedBlock.
//
// Default translation:
//
//   - Every SDKv2 "Default: false" on a TypeBool becomes
//     booldefault.StaticBool(false) on the BoolAttribute (and the attribute
//     must be Optional+Computed for a Default to apply). Defaults belong in
//     the per-type "defaults" package — never inside PlanModifiers.
//
//   - "ForceNew: true" on email becomes
//     stringplanmodifier.RequiresReplace() in PlanModifiers (NOT a
//     RequiresReplace field — that does not exist on framework attributes).
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The unique ID of this Linode User. Equal to the username.",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
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
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"ssh_keys": schema.ListAttribute{
			Description: "SSH keys to add to the user profile.",
			Computed:    true,
			ElementType: types.StringType,
		},
		"tfa_enabled": schema.BoolAttribute{
			Description: "If the User has Two Factor Authentication (TFA) enabled.",
			Computed:    true,
		},
	},
	Blocks: map[string]schema.Block{
		"global_grants": schema.ListNestedBlock{
			Description: "A structure containing the Account-level grants a User has.",
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
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
						Description: "If true, this User may manage the Account’s Longview subscription.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
				},
			},
		},
		"domain_grant":       linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"firewall_grant":     linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"image_grant":        linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"linode_grant":       linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"longview_grant":     linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"nodebalancer_grant": linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"stackscript_grant":  linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"volume_grant":       linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
		"vpc_grant":          linodeUserGrantsEntityBlock("A set containing all of the user's active grants."),
	},
}

// linodeUserGrantsEntityBlock returns the per-entity grants block. SDKv2
// modeled these as TypeSet of *schema.Resource (no MaxItems), so they remain
// repeating blocks — SetNestedBlock — to keep practitioner HCL syntax
// ("linode_grant { id = 1 permissions = "read_write" }") unchanged.
func linodeUserGrantsEntityBlock(description string) schema.Block {
	return schema.SetNestedBlock{
		Description: description,
		NestedObject: schema.NestedBlockObject{
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
