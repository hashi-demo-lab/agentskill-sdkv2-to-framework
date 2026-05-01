package user

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
	_ resource.ResourceWithConfigure   = &UserResource{}
)

// resourceGlobalGrantsObjectType defines the object type for global_grants.
var resourceGlobalGrantsObjectType = types.ObjectType{
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

// resourceEntityGrantObjectType defines the object type for entity grant sets.
var resourceEntityGrantObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":          types.Int64Type,
		"permissions": types.StringType,
	},
}

// UserResourceModel is the state/plan model for the linode_user resource.
type UserResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Email            types.String `tfsdk:"email"`
	Username         types.String `tfsdk:"username"`
	Restricted       types.Bool   `tfsdk:"restricted"`
	UserType         types.String `tfsdk:"user_type"`
	SSHKeys          types.List   `tfsdk:"ssh_keys"`
	TFAEnabled       types.Bool   `tfsdk:"tfa_enabled"`
	GlobalGrants     types.List   `tfsdk:"global_grants"`
	DomainGrant      types.Set    `tfsdk:"domain_grant"`
	FirewallGrant    types.Set    `tfsdk:"firewall_grant"`
	ImageGrant       types.Set    `tfsdk:"image_grant"`
	LinodeGrant      types.Set    `tfsdk:"linode_grant"`
	LongviewGrant    types.Set    `tfsdk:"longview_grant"`
	NodebalancerGrant types.Set   `tfsdk:"nodebalancer_grant"`
	StackscriptGrant types.Set    `tfsdk:"stackscript_grant"`
	VolumeGrant      types.Set    `tfsdk:"volume_grant"`
	VPCGrant         types.Set    `tfsdk:"vpc_grant"`
}

// entityGrantSetAttr is the reusable schema for entity grant sets.
var entityGrantSetAttr = schema.SetNestedAttribute{
	Description: "A set containing all of the user's active grants for this resource type.",
	Optional:    true,
	Computed:    true,
	NestedObject: schema.NestedAttributeObject{
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:    true,
				Description: "The ID of the entity this grant applies to.",
			},
			"permissions": schema.StringAttribute{
				Required:    true,
				Description: "The level of access this User has to this entity. If null, this User has no access.",
			},
		},
	},
}

// frameworkResourceSchema is the framework schema for the linode_user resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:    true,
			Description: "The username of the user (used as the resource ID).",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"email": schema.StringAttribute{
			Required:    true,
			Description: "The email of the user.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"username": schema.StringAttribute{
			Required:    true,
			Description: "The username of the user.",
		},
		"restricted": schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Default:     booldefault.StaticBool(false),
			Description: "If true, the user must be explicitly granted access to platform actions and entities.",
		},
		"user_type": schema.StringAttribute{
			Computed:    true,
			Description: "The type of this user.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"ssh_keys": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "SSH keys to add to the user profile.",
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"tfa_enabled": schema.BoolAttribute{
			Computed:    true,
			Description: "If the User has Two Factor Authentication (TFA) enabled.",
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		},
		"global_grants": schema.ListNestedAttribute{
			Optional:    true,
			Computed:    true,
			Description: "A structure containing the Account-level grants a User has.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"account_access": schema.StringAttribute{
						Optional: true,
						Computed: true,
						Description: "The level of access this User has to Account-level actions, like billing " +
							"information. A restricted User will never be able to manage users.",
					},
					"add_domains": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add Domains.",
					},
					"add_databases": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add Databases.",
					},
					"add_firewalls": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add Firewalls.",
					},
					"add_images": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add Images.",
					},
					"add_linodes": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may create Linodes.",
					},
					"add_longview": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may create Longview clients and view the current plan.",
					},
					"add_nodebalancers": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add NodeBalancers.",
					},
					"add_stackscripts": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add StackScripts.",
					},
					"add_volumes": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add Volumes.",
					},
					"add_vpcs": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may add Virtual Private Clouds (VPCs).",
					},
					"cancel_account": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may cancel the entire Account.",
					},
					"longview_subscription": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "If true, this User may manage the Account's Longview subscription.",
					},
				},
			},
		},
		"domain_grant":       entityGrantSetAttr,
		"firewall_grant":     entityGrantSetAttr,
		"image_grant":        entityGrantSetAttr,
		"linode_grant":       entityGrantSetAttr,
		"longview_grant":     entityGrantSetAttr,
		"nodebalancer_grant": entityGrantSetAttr,
		"stackscript_grant":  entityGrantSetAttr,
		"volume_grant":       entityGrantSetAttr,
		"vpc_grant":          entityGrantSetAttr,
	},
}

// NewUserResource creates a new UserResource.
func NewUserResource() resource.Resource {
	return &UserResource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_user",
				IDAttr: "id",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

// UserResource implements resource.Resource.
type UserResource struct {
	helper.BaseResource
}

// Create implements resource.Resource.
func (r *UserResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_user")

	client := r.Meta.Client

	var plan UserResourceModel
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

	// Set the ID immediately so import/state works if later steps fail.
	plan.ID = types.StringValue(user.Username)
	ctx = tflog.SetField(ctx, "username", user.Username)

	if modelHasGrantsConfigured(plan) {
		if diags := applyUserGrants(ctx, client, user.Username, plan.Restricted.ValueBool(), plan, &resp.Diagnostics); diags {
			return
		}
	}

	resp.Diagnostics.Append(readUserIntoModel(ctx, client, user.Username, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *UserResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_user")

	client := r.Meta.Client

	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	username := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "username", username)

	user, err := client.GetUser(ctx, username)
	if err != nil {
		if linodego.IsNotFound(err) {
			resp.Diagnostics.AddWarning(
				fmt.Sprintf("Removing User %s from State", username),
				"Removing the Linode User from state because it no longer exists",
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to get user (%s)", username), err.Error(),
		)
		return
	}

	state.Username = types.StringValue(user.Username)
	state.Email = types.StringValue(user.Email)
	state.Restricted = types.BoolValue(user.Restricted)
	state.UserType = types.StringValue(string(user.UserType))
	state.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, diags := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.SSHKeys = sshKeys

	if user.Restricted {
		grants, err := client.GetUserGrants(ctx, username)
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to get user grants (%s)", username), err.Error(),
			)
			return
		}

		resp.Diagnostics.Append(flattenGlobalGrantsToModel(ctx, grants.Global, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}

		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.Domain, &state.DomainGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.Firewall, &state.FirewallGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.Image, &state.ImageGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.Linode, &state.LinodeGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.Longview, &state.LongviewGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.NodeBalancer, &state.NodebalancerGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.StackScript, &state.StackscriptGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.VPC, &state.VPCGrant)...)
		resp.Diagnostics.Append(flattenEntityGrantsToModel(ctx, grants.Volume, &state.VolumeGrant)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		state.GlobalGrants = types.ListNull(resourceGlobalGrantsObjectType)
		state.DomainGrant = types.SetNull(resourceEntityGrantObjectType)
		state.FirewallGrant = types.SetNull(resourceEntityGrantObjectType)
		state.ImageGrant = types.SetNull(resourceEntityGrantObjectType)
		state.LinodeGrant = types.SetNull(resourceEntityGrantObjectType)
		state.LongviewGrant = types.SetNull(resourceEntityGrantObjectType)
		state.NodebalancerGrant = types.SetNull(resourceEntityGrantObjectType)
		state.StackscriptGrant = types.SetNull(resourceEntityGrantObjectType)
		state.VolumeGrant = types.SetNull(resourceEntityGrantObjectType)
		state.VPCGrant = types.SetNull(resourceEntityGrantObjectType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *UserResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_user")

	client := r.Meta.Client

	var plan, state UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "username", id)

	username := plan.Username.ValueString()
	restricted := plan.Restricted.ValueBool()

	updateOpts := linodego.UserUpdateOptions{
		Username:   username,
		Restricted: &restricted,
	}

	tflog.Debug(ctx, "client.UpdateUser(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUser(ctx, id, updateOpts); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to update user (%s)", id), err.Error(),
		)
		return
	}

	// Username may have changed — update ID.
	plan.ID = types.StringValue(username)

	if grantsHaveChanges(plan, state) {
		if diags := applyUserGrants(ctx, client, username, restricted, plan, &resp.Diagnostics); diags {
			return
		}
	}

	resp.Diagnostics.Append(readUserIntoModel(ctx, client, username, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *UserResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_user")

	client := r.Meta.Client

	var state UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	username := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "username", username)

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := client.DeleteUser(ctx, username); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete user (%s)", username), err.Error(),
		)
	}
}

// readUserIntoModel fetches the latest user data from the API and populates the model.
func readUserIntoModel(
	ctx context.Context,
	client *linodego.Client,
	username string,
	model *UserResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	user, err := client.GetUser(ctx, username)
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to get user (%s)", username), err.Error(),
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
				fmt.Sprintf("Failed to get user grants (%s)", username), err.Error(),
			)
			return diags
		}

		diags.Append(flattenGlobalGrantsToModel(ctx, grants.Global, model)...)
		if diags.HasError() {
			return diags
		}

		diags.Append(flattenEntityGrantsToModel(ctx, grants.Domain, &model.DomainGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.Firewall, &model.FirewallGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.Image, &model.ImageGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.Linode, &model.LinodeGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.Longview, &model.LongviewGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.NodeBalancer, &model.NodebalancerGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.StackScript, &model.StackscriptGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.VPC, &model.VPCGrant)...)
		diags.Append(flattenEntityGrantsToModel(ctx, grants.Volume, &model.VolumeGrant)...)
	} else {
		model.GlobalGrants = types.ListNull(resourceGlobalGrantsObjectType)
		model.DomainGrant = types.SetNull(resourceEntityGrantObjectType)
		model.FirewallGrant = types.SetNull(resourceEntityGrantObjectType)
		model.ImageGrant = types.SetNull(resourceEntityGrantObjectType)
		model.LinodeGrant = types.SetNull(resourceEntityGrantObjectType)
		model.LongviewGrant = types.SetNull(resourceEntityGrantObjectType)
		model.NodebalancerGrant = types.SetNull(resourceEntityGrantObjectType)
		model.StackscriptGrant = types.SetNull(resourceEntityGrantObjectType)
		model.VolumeGrant = types.SetNull(resourceEntityGrantObjectType)
		model.VPCGrant = types.SetNull(resourceEntityGrantObjectType)
	}

	return diags
}

// applyUserGrants updates user grants via the API.
// Returns true if diagnostics contain errors.
func applyUserGrants(
	ctx context.Context,
	client *linodego.Client,
	username string,
	restricted bool,
	model UserResourceModel,
	respDiags *diag.Diagnostics,
) bool {
	if !restricted {
		respDiags.AddError(
			"Cannot set grants on unrestricted user",
			"User must be restricted in order to update grants",
		)
		return true
	}

	updateOpts := linodego.UserGrantsUpdateOptions{}

	// Global grants
	if !model.GlobalGrants.IsNull() && !model.GlobalGrants.IsUnknown() {
		var globalList []globalGrantModel
		respDiags.Append(model.GlobalGrants.ElementsAs(ctx, &globalList, false)...)
		if respDiags.HasError() {
			return true
		}
		if len(globalList) > 0 {
			updateOpts.Global = expandGlobalGrants(globalList[0])
		}
	}

	updateOpts.Domain = expandEntityGrantSet(ctx, model.DomainGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.Firewall = expandEntityGrantSet(ctx, model.FirewallGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.Image = expandEntityGrantSet(ctx, model.ImageGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.Linode = expandEntityGrantSet(ctx, model.LinodeGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.Longview = expandEntityGrantSet(ctx, model.LongviewGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.NodeBalancer = expandEntityGrantSet(ctx, model.NodebalancerGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.StackScript = expandEntityGrantSet(ctx, model.StackscriptGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.VPC = expandEntityGrantSet(ctx, model.VPCGrant, respDiags)
	if respDiags.HasError() {
		return true
	}
	updateOpts.Volume = expandEntityGrantSet(ctx, model.VolumeGrant, respDiags)
	if respDiags.HasError() {
		return true
	}

	tflog.Debug(ctx, "client.UpdateUserGrants(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUserGrants(ctx, username, updateOpts); err != nil {
		respDiags.AddError(
			fmt.Sprintf("Failed to update user grants (%s)", username), err.Error(),
		)
		return true
	}

	return false
}

// globalGrantModel is a helper struct to decode the global_grants list element.
type globalGrantModel struct {
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

// entityGrantModel is a helper struct to decode entity grant set elements.
type entityGrantModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Permissions types.String `tfsdk:"permissions"`
}

// expandGlobalGrants converts a globalGrantModel to linodego.GlobalUserGrants.
func expandGlobalGrants(g globalGrantModel) linodego.GlobalUserGrants {
	result := linodego.GlobalUserGrants{}

	result.AccountAccess = nil
	if !g.AccountAccess.IsNull() && !g.AccountAccess.IsUnknown() {
		if v := g.AccountAccess.ValueString(); v != "" {
			level := linodego.GrantPermissionLevel(v)
			result.AccountAccess = &level
		}
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

// expandEntityGrantSet converts a types.Set of entity grants to a linodego slice.
func expandEntityGrantSet(
	ctx context.Context,
	set types.Set,
	diags *diag.Diagnostics,
) []linodego.EntityUserGrant {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	var models []entityGrantModel
	diags.Append(set.ElementsAs(ctx, &models, false)...)
	if diags.HasError() {
		return nil
	}

	result := make([]linodego.EntityUserGrant, len(models))
	for i, m := range models {
		permissions := linodego.GrantPermissionLevel(m.Permissions.ValueString())
		result[i] = linodego.EntityUserGrant{
			ID:          int(m.ID.ValueInt64()),
			Permissions: &permissions,
		}
	}

	return result
}

// flattenGlobalGrantsToModel populates the GlobalGrants field in the model.
func flattenGlobalGrantsToModel(
	_ context.Context,
	grants linodego.GlobalUserGrants,
	model *UserResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	attrValues := map[string]attr.Value{
		"add_domains":           types.BoolValue(grants.AddDomains),
		"add_databases":         types.BoolValue(grants.AddDatabases),
		"add_firewalls":         types.BoolValue(grants.AddFirewalls),
		"add_images":            types.BoolValue(grants.AddImages),
		"add_linodes":           types.BoolValue(grants.AddLinodes),
		"add_longview":          types.BoolValue(grants.AddLongview),
		"add_nodebalancers":     types.BoolValue(grants.AddNodeBalancers),
		"add_stackscripts":      types.BoolValue(grants.AddStackScripts),
		"add_volumes":           types.BoolValue(grants.AddVolumes),
		"add_vpcs":              types.BoolValue(grants.AddVPCs),
		"cancel_account":        types.BoolValue(grants.CancelAccount),
		"longview_subscription": types.BoolValue(grants.LongviewSubscription),
	}

	if grants.AccountAccess != nil {
		attrValues["account_access"] = types.StringValue(string(*grants.AccountAccess))
	} else {
		attrValues["account_access"] = types.StringValue("")
	}

	obj, d := types.ObjectValue(resourceGlobalGrantsObjectType.AttrTypes, attrValues)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	listVal, d := basetypes.NewListValue(resourceGlobalGrantsObjectType, []attr.Value{obj})
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	model.GlobalGrants = listVal
	return diags
}

// flattenEntityGrantsToModel converts a slice of linodego.GrantedEntity into a types.Set.
func flattenEntityGrantsToModel(
	_ context.Context,
	entities []linodego.GrantedEntity,
	target *types.Set,
) diag.Diagnostics {
	var diags diag.Diagnostics

	elems := make([]attr.Value, 0, len(entities))
	for _, entity := range entities {
		// Filter out entities without any permissions set (Linode returns empty placeholder entities).
		if entity.Permissions == "" {
			continue
		}

		obj, d := types.ObjectValue(
			resourceEntityGrantObjectType.AttrTypes,
			map[string]attr.Value{
				"id":          types.Int64Value(int64(entity.ID)),
				"permissions": types.StringValue(string(entity.Permissions)),
			},
		)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		elems = append(elems, obj)
	}

	setVal, d := basetypes.NewSetValue(resourceEntityGrantObjectType, elems)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	*target = setVal
	return diags
}

// modelHasGrantsConfigured returns true if any grant attribute is set in the plan.
func modelHasGrantsConfigured(model UserResourceModel) bool {
	if !model.GlobalGrants.IsNull() && !model.GlobalGrants.IsUnknown() {
		return true
	}
	sets := []types.Set{
		model.DomainGrant, model.FirewallGrant, model.ImageGrant, model.LinodeGrant,
		model.LongviewGrant, model.NodebalancerGrant, model.StackscriptGrant,
		model.VolumeGrant, model.VPCGrant,
	}
	for _, s := range sets {
		if !s.IsNull() && !s.IsUnknown() {
			return true
		}
	}
	return false
}

// grantsHaveChanges returns true if any grant-related field differs between plan and state.
func grantsHaveChanges(plan, state UserResourceModel) bool {
	if !plan.GlobalGrants.Equal(state.GlobalGrants) {
		return true
	}
	planSets := []types.Set{
		plan.DomainGrant, plan.FirewallGrant, plan.ImageGrant, plan.LinodeGrant,
		plan.LongviewGrant, plan.NodebalancerGrant, plan.StackscriptGrant,
		plan.VolumeGrant, plan.VPCGrant,
	}
	stateSets := []types.Set{
		state.DomainGrant, state.FirewallGrant, state.ImageGrant, state.LinodeGrant,
		state.LongviewGrant, state.NodebalancerGrant, state.StackscriptGrant,
		state.VolumeGrant, state.VPCGrant,
	}
	for i := range planSets {
		if !planSets[i].Equal(stateSets[i]) {
			return true
		}
	}
	return false
}

