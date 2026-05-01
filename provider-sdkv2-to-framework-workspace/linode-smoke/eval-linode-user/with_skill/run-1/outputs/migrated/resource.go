package user

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// resourceUserGrantsEntityObjectType describes the schema object type for entity grants in the resource.
var resourceUserGrantsEntityObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":          types.Int64Type,
		"permissions": types.StringType,
	},
}

// resourceUserGrantsGlobalObjectType describes the schema object type for global grants in the resource.
var resourceUserGrantsGlobalObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"account_access":        types.StringType,
		"add_domains":           types.BoolType,
		"add_databases":         types.BoolType,
		"add_firewalls":         types.BoolType,
		"add_images":            types.BoolType,
		"add_linodes":           types.BoolType,
		"add_longview":          types.BoolType,
		"add_nodebalancers":     types.BoolType,
		"add_stackscripts":      types.BoolType,
		"add_volumes":           types.BoolType,
		"add_vpcs":              types.BoolType,
		"cancel_account":        types.BoolType,
		"longview_subscription": types.BoolType,
	},
}

// frameworkResourceSchema is the schema for the linode_user resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The username of the user (used as resource ID).",
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
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"tfa_enabled": schema.BoolAttribute{
			Description: "If the User has Two Factor Authentication (TFA) enabled.",
			Computed:    true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		},
		"domain_grant": schema.SetNestedAttribute{
			Description: "A set of domain grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"firewall_grant": schema.SetNestedAttribute{
			Description: "A set of firewall grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"image_grant": schema.SetNestedAttribute{
			Description: "A set of image grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"linode_grant": schema.SetNestedAttribute{
			Description: "A set of Linode grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"longview_grant": schema.SetNestedAttribute{
			Description: "A set of Longview grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"nodebalancer_grant": schema.SetNestedAttribute{
			Description: "A set of NodeBalancer grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"stackscript_grant": schema.SetNestedAttribute{
			Description: "A set of StackScript grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"volume_grant": schema.SetNestedAttribute{
			Description: "A set of Volume grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
		"vpc_grant": schema.SetNestedAttribute{
			Description: "A set of VPC grants for the user.",
			Optional:    true,
			Computed:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Description: "The ID of the entity this grant applies to.",
						Required:    true,
					},
					"permissions": schema.StringAttribute{
						Description: "The level of access this User has to this entity.",
						Required:    true,
					},
				},
			},
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
			},
		},
	},
	Blocks: map[string]schema.Block{
		// global_grants uses ListNestedBlock (was TypeList + MaxItems:1 in SDKv2).
		// Kept as a block to preserve practitioner HCL syntax (global_grants { ... }).
		"global_grants": schema.ListNestedBlock{
			Description: "A structure containing the Account-level grants a User has.",
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
						Description: "If true, this User may manage the Account's Longview subscription.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
				},
			},
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
		},
	},
}

// ResourceModel is the Terraform state model for the linode_user resource.
type ResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Email              types.String `tfsdk:"email"`
	Username           types.String `tfsdk:"username"`
	Restricted         types.Bool   `tfsdk:"restricted"`
	UserType           types.String `tfsdk:"user_type"`
	SSHKeys            types.List   `tfsdk:"ssh_keys"`
	TFAEnabled         types.Bool   `tfsdk:"tfa_enabled"`
	GlobalGrants       types.List   `tfsdk:"global_grants"`
	DomainGrant        types.Set    `tfsdk:"domain_grant"`
	FirewallGrant      types.Set    `tfsdk:"firewall_grant"`
	ImageGrant         types.Set    `tfsdk:"image_grant"`
	LinodeGrant        types.Set    `tfsdk:"linode_grant"`
	LongviewGrant      types.Set    `tfsdk:"longview_grant"`
	NodebalancerGrant  types.Set    `tfsdk:"nodebalancer_grant"`
	StackscriptGrant   types.Set    `tfsdk:"stackscript_grant"`
	VolumeGrant        types.Set    `tfsdk:"volume_grant"`
	VPCGrant           types.Set    `tfsdk:"vpc_grant"`
}

// GlobalGrantsModel is the nested model for the global_grants block.
type GlobalGrantsModel struct {
	AccountAccess       types.String `tfsdk:"account_access"`
	AddDomains          types.Bool   `tfsdk:"add_domains"`
	AddDatabases        types.Bool   `tfsdk:"add_databases"`
	AddFirewalls        types.Bool   `tfsdk:"add_firewalls"`
	AddImages           types.Bool   `tfsdk:"add_images"`
	AddLinodes          types.Bool   `tfsdk:"add_linodes"`
	AddLongview         types.Bool   `tfsdk:"add_longview"`
	AddNodeBalancers    types.Bool   `tfsdk:"add_nodebalancers"`
	AddStackScripts     types.Bool   `tfsdk:"add_stackscripts"`
	AddVolumes          types.Bool   `tfsdk:"add_volumes"`
	AddVPCs             types.Bool   `tfsdk:"add_vpcs"`
	CancelAccount       types.Bool   `tfsdk:"cancel_account"`
	LongviewSubscription types.Bool  `tfsdk:"longview_subscription"`
}

// EntityGrantModel is the nested model for entity-level grant sets.
type EntityGrantModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Permissions types.String `tfsdk:"permissions"`
}

// NewResource returns a new framework resource for linode_user.
func NewResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_user",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

// Resource is the framework resource implementation.
type Resource struct {
	helper.BaseResource
}

// Ensure Resource satisfies required interfaces.
var _ resource.Resource = &Resource{}
var _ resource.ResourceWithImportState = &Resource{}

func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_user")

	client := r.Meta.Client

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createOpts := linodego.UserCreateOptions{
		Email:      plan.Email.ValueString(),
		Username:   plan.Username.ValueString(),
		Restricted: plan.Restricted.ValueBool(),
	}

	tflog.Debug(ctx, "client.CreateUser(...)", map[string]any{
		"options": createOpts,
	})

	user, err := client.CreateUser(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create user", err.Error())
		return
	}

	// Set the ID immediately so it appears in state if subsequent calls fail.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), user.Username)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(user.Username)
	ctx = tflog.SetField(ctx, "username", user.Username)

	if resourceModelHasGrantsConfigured(&plan) {
		if err := resourceUpdateUserGrants(ctx, client, &plan, &resp.Diagnostics); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to set user grants (%s)", user.Username),
				err.Error(),
			)
			return
		}
	}

	resp.Diagnostics.Append(resourceReadUser(ctx, client, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_user")

	client := r.Meta.Client

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", state.ID.ValueString())

	resp.Diagnostics.Append(resourceReadUser(ctx, client, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_user")

	client := r.Meta.Client

	var state, plan ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", state.ID.ValueString())

	username := plan.Username.ValueString()
	restricted := plan.Restricted.ValueBool()

	updateOpts := linodego.UserUpdateOptions{
		Username:   username,
		Restricted: &restricted,
	}

	tflog.Debug(ctx, "client.UpdateUser(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUser(ctx, state.ID.ValueString(), updateOpts); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to update user (%s)", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	// Username may have changed — update the ID.
	plan.ID = types.StringValue(username)

	if resourceGrantsChanged(&state, &plan) {
		if err := resourceUpdateUserGrants(ctx, client, &plan, &resp.Diagnostics); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to update user grants (%s)", username),
				err.Error(),
			)
			return
		}
	}

	resp.Diagnostics.Append(resourceReadUser(ctx, client, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_user")

	client := r.Meta.Client

	// Delete reads from State, not Plan (req.Plan is null on Delete).
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", state.ID.ValueString())

	username := state.ID.ValueString()

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := client.DeleteUser(ctx, username); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete user (%s)", username),
			err.Error(),
		)
		return
	}
}

// resourceReadUser fetches the user (and optionally grants) from the API and
// populates the model in-place.
func resourceReadUser(
	ctx context.Context,
	client *linodego.Client,
	model *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	username := model.ID.ValueString()

	user, err := client.GetUser(ctx, username)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing Linode User %q from state because it no longer exists", username)
			// Caller must handle removal; we simply don't populate.
			return diags
		}
		diags.AddError(
			fmt.Sprintf("Failed to get user (%s)", username),
			err.Error(),
		)
		return diags
	}

	model.Username = types.StringValue(user.Username)
	model.Email = types.StringValue(user.Email)
	model.Restricted = types.BoolValue(user.Restricted)
	model.UserType = types.StringValue(string(user.UserType))
	model.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, d := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	model.SSHKeys = sshKeys

	if user.Restricted {
		grants, err := client.GetUserGrants(ctx, username)
		if err != nil {
			diags.AddError(
				fmt.Sprintf("Failed to get user grants (%s)", username),
				err.Error(),
			)
			return diags
		}

		diags.Append(resourceFlattenGlobalGrants(ctx, grants.Global, model)...)
		if diags.HasError() {
			return diags
		}

		diags.Append(resourceFlattenEntityGrants(ctx, grants.Domain, &model.DomainGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.Firewall, &model.FirewallGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.Image, &model.ImageGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.Linode, &model.LinodeGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.Longview, &model.LongviewGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.NodeBalancer, &model.NodebalancerGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.StackScript, &model.StackscriptGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.Volume, &model.VolumeGrant)...)
		diags.Append(resourceFlattenEntityGrants(ctx, grants.VPC, &model.VPCGrant)...)
	}

	return diags
}

// resourceFlattenGlobalGrants converts linodego.GlobalUserGrants into the
// ResourceModel's GlobalGrants list (a list containing at most one object).
func resourceFlattenGlobalGrants(
	ctx context.Context,
	grants linodego.GlobalUserGrants,
	model *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	accountAccess := ""
	if grants.AccountAccess != nil {
		accountAccess = string(*grants.AccountAccess)
	}

	obj := GlobalGrantsModel{
		AccountAccess:        types.StringValue(accountAccess),
		AddDomains:           types.BoolValue(grants.AddDomains),
		AddDatabases:         types.BoolValue(grants.AddDatabases),
		AddFirewalls:         types.BoolValue(grants.AddFirewalls),
		AddImages:            types.BoolValue(grants.AddImages),
		AddLinodes:           types.BoolValue(grants.AddLinodes),
		AddLongview:          types.BoolValue(grants.AddLongview),
		AddNodeBalancers:     types.BoolValue(grants.AddNodeBalancers),
		AddStackScripts:      types.BoolValue(grants.AddStackScripts),
		AddVolumes:           types.BoolValue(grants.AddVolumes),
		AddVPCs:              types.BoolValue(grants.AddVPCs),
		CancelAccount:        types.BoolValue(grants.CancelAccount),
		LongviewSubscription: types.BoolValue(grants.LongviewSubscription),
	}

	listVal, d := types.ListValueFrom(ctx, resourceUserGrantsGlobalObjectType, []GlobalGrantsModel{obj})
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	model.GlobalGrants = listVal

	return diags
}

// resourceFlattenEntityGrants converts a slice of GrantedEntity into a
// types.Set of EntityGrantModel. Entities without permissions are excluded.
func resourceFlattenEntityGrants(
	ctx context.Context,
	entities []linodego.GrantedEntity,
	target *types.Set,
) diag.Diagnostics {
	models := make([]EntityGrantModel, 0, len(entities))
	for _, e := range entities {
		if e.Permissions == "" {
			continue
		}
		models = append(models, EntityGrantModel{
			ID:          types.Int64Value(int64(e.ID)),
			Permissions: types.StringValue(string(e.Permissions)),
		})
	}

	setVal, diags := types.SetValueFrom(ctx, resourceUserGrantsEntityObjectType, models)
	if diags.HasError() {
		return diags
	}
	*target = setVal
	return diags
}

// resourceUpdateUserGrants sends user grant updates to the Linode API.
func resourceUpdateUserGrants(
	ctx context.Context,
	client *linodego.Client,
	plan *ResourceModel,
	diagnostics *diag.Diagnostics,
) error {
	username := plan.ID.ValueString()

	if !plan.Restricted.ValueBool() {
		return fmt.Errorf("user must be restricted in order to update grants")
	}

	updateOpts := linodego.UserGrantsUpdateOptions{}

	// Expand global_grants block (list of at most 1).
	var globalList []GlobalGrantsModel
	diagnostics.Append(plan.GlobalGrants.ElementsAs(ctx, &globalList, false)...)
	if diagnostics.HasError() {
		return fmt.Errorf("failed to expand global_grants")
	}
	if len(globalList) > 0 {
		updateOpts.Global = expandResourceGrantsGlobal(globalList[0])
	}

	updateOpts.Domain = expandResourceGrantEntities(ctx, plan.DomainGrant, diagnostics)
	updateOpts.Firewall = expandResourceGrantEntities(ctx, plan.FirewallGrant, diagnostics)
	updateOpts.Image = expandResourceGrantEntities(ctx, plan.ImageGrant, diagnostics)
	updateOpts.Linode = expandResourceGrantEntities(ctx, plan.LinodeGrant, diagnostics)
	updateOpts.Longview = expandResourceGrantEntities(ctx, plan.LongviewGrant, diagnostics)
	updateOpts.NodeBalancer = expandResourceGrantEntities(ctx, plan.NodebalancerGrant, diagnostics)
	updateOpts.StackScript = expandResourceGrantEntities(ctx, plan.StackscriptGrant, diagnostics)
	updateOpts.VPC = expandResourceGrantEntities(ctx, plan.VPCGrant, diagnostics)
	updateOpts.Volume = expandResourceGrantEntities(ctx, plan.VolumeGrant, diagnostics)

	if diagnostics.HasError() {
		return fmt.Errorf("failed to expand entity grants")
	}

	tflog.Debug(ctx, "client.UpdateUserGrants(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUserGrants(ctx, username, updateOpts); err != nil {
		return err
	}

	return nil
}

// expandResourceGrantsGlobal converts a GlobalGrantsModel into linodego options.
func expandResourceGrantsGlobal(g GlobalGrantsModel) linodego.GlobalUserGrants {
	result := linodego.GlobalUserGrants{}

	if v := g.AccountAccess.ValueString(); v != "" {
		level := linodego.GrantPermissionLevel(v)
		result.AccountAccess = &level
	}

	result.AddDomains = g.AddDomains.ValueBool()
	result.AddDatabases = g.AddDatabases.ValueBool()
	result.AddFirewalls = g.AddFirewalls.ValueBool()
	result.AddImages = g.AddImages.ValueBool()
	result.AddLinodes = g.AddLinodes.ValueBool()
	result.AddLongview = g.AddLongview.ValueBool()
	result.AddNodeBalancers = g.AddNodeBalancers.ValueBool()
	result.AddStackScripts = g.AddStackScripts.ValueBool()
	result.AddVolumes = g.AddVolumes.ValueBool()
	result.AddVPCs = g.AddVPCs.ValueBool()
	result.CancelAccount = g.CancelAccount.ValueBool()
	result.LongviewSubscription = g.LongviewSubscription.ValueBool()

	return result
}

// expandResourceGrantEntities converts a types.Set of EntityGrantModel into linodego options.
func expandResourceGrantEntities(
	ctx context.Context,
	s types.Set,
	diagnostics *diag.Diagnostics,
) []linodego.EntityUserGrant {
	var models []EntityGrantModel
	diagnostics.Append(s.ElementsAs(ctx, &models, false)...)
	if diagnostics.HasError() {
		return nil
	}

	result := make([]linodego.EntityUserGrant, len(models))
	for i, m := range models {
		perm := linodego.GrantPermissionLevel(m.Permissions.ValueString())
		result[i] = linodego.EntityUserGrant{
			ID:          int(m.ID.ValueInt64()),
			Permissions: &perm,
		}
	}
	return result
}

// resourceModelHasGrantsConfigured returns true if any grant field is non-null and non-unknown.
func resourceModelHasGrantsConfigured(plan *ResourceModel) bool {
	sets := []types.Set{
		plan.DomainGrant, plan.FirewallGrant, plan.ImageGrant, plan.LinodeGrant,
		plan.LongviewGrant, plan.NodebalancerGrant, plan.StackscriptGrant,
		plan.VolumeGrant, plan.VPCGrant,
	}
	for _, s := range sets {
		if !s.IsNull() && !s.IsUnknown() && len(s.Elements()) > 0 {
			return true
		}
	}
	if !plan.GlobalGrants.IsNull() && !plan.GlobalGrants.IsUnknown() && len(plan.GlobalGrants.Elements()) > 0 {
		return true
	}
	return false
}

// resourceGrantsChanged returns true if any grant-related field differs between state and plan.
func resourceGrantsChanged(state, plan *ResourceModel) bool {
	return !plan.GlobalGrants.Equal(state.GlobalGrants) ||
		!plan.DomainGrant.Equal(state.DomainGrant) ||
		!plan.FirewallGrant.Equal(state.FirewallGrant) ||
		!plan.ImageGrant.Equal(state.ImageGrant) ||
		!plan.LinodeGrant.Equal(state.LinodeGrant) ||
		!plan.LongviewGrant.Equal(state.LongviewGrant) ||
		!plan.NodebalancerGrant.Equal(state.NodebalancerGrant) ||
		!plan.StackscriptGrant.Equal(state.StackscriptGrant) ||
		!plan.VolumeGrant.Equal(state.VolumeGrant) ||
		!plan.VPCGrant.Equal(state.VPCGrant)
}
