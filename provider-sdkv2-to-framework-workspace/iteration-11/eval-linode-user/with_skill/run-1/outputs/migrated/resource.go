package user

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
)

func NewResource() resource.Resource {
	return &UserResource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_user",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

type UserResource struct {
	helper.BaseResource
}

// ResourceModel is the flat model for the user resource, with grant blocks handled via nested types.
type ResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Username     types.String `tfsdk:"username"`
	Email        types.String `tfsdk:"email"`
	Restricted   types.Bool   `tfsdk:"restricted"`
	UserType     types.String `tfsdk:"user_type"`
	SSHKeys      types.List   `tfsdk:"ssh_keys"`
	TFAEnabled   types.Bool   `tfsdk:"tfa_enabled"`
	GlobalGrants types.Object `tfsdk:"global_grants"`

	DomainGrant       types.Set `tfsdk:"domain_grant"`
	FirewallGrant     types.Set `tfsdk:"firewall_grant"`
	ImageGrant        types.Set `tfsdk:"image_grant"`
	LinodeGrant       types.Set `tfsdk:"linode_grant"`
	LongviewGrant     types.Set `tfsdk:"longview_grant"`
	NodebalancerGrant types.Set `tfsdk:"nodebalancer_grant"`
	StackscriptGrant  types.Set `tfsdk:"stackscript_grant"`
	VolumeGrant       types.Set `tfsdk:"volume_grant"`
	VPCGrant          types.Set `tfsdk:"vpc_grant"`
}

// resourceGrantsGlobalAttrTypes defines the attribute types for the global_grants block.
var resourceGrantsGlobalAttrTypes = map[string]attr.Type{
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
}

// resourceGrantEntityAttrTypes defines the attribute types for entity grant sets.
var resourceGrantEntityAttrTypes = map[string]attr.Type{
	"id":          types.Int64Type,
	"permissions": types.StringType,
}

var resourceGrantEntityObjectType = types.ObjectType{AttrTypes: resourceGrantEntityAttrTypes}

func (r *UserResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_user")

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client

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
		resp.Diagnostics.AddError("Failed to create Linode User", err.Error())
		return
	}

	plan.ID = types.StringValue(user.Username)

	ctx = tflog.SetField(ctx, "username", user.Username)

	if !plan.GlobalGrants.IsNull() && !plan.GlobalGrants.IsUnknown() ||
		!plan.DomainGrant.IsNull() && !plan.DomainGrant.IsUnknown() ||
		!plan.FirewallGrant.IsNull() && !plan.FirewallGrant.IsUnknown() ||
		!plan.ImageGrant.IsNull() && !plan.ImageGrant.IsUnknown() ||
		!plan.LinodeGrant.IsNull() && !plan.LinodeGrant.IsUnknown() ||
		!plan.LongviewGrant.IsNull() && !plan.LongviewGrant.IsUnknown() ||
		!plan.NodebalancerGrant.IsNull() && !plan.NodebalancerGrant.IsUnknown() ||
		!plan.StackscriptGrant.IsNull() && !plan.StackscriptGrant.IsUnknown() ||
		!plan.VolumeGrant.IsNull() && !plan.VolumeGrant.IsUnknown() ||
		!plan.VPCGrant.IsNull() && !plan.VPCGrant.IsUnknown() {
		resp.Diagnostics.Append(r.updateUserGrants(ctx, user.Username, plan.Restricted.ValueBool(), plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(r.readInto(ctx, user.Username, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", state.ID.ValueString())
	tflog.Debug(ctx, "Read linode_user")

	resp.Diagnostics.Append(r.readInto(ctx, state.ID.ValueString(), &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// readInto signals "not found" by nulling out the ID.
	if state.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UserResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan, state ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", state.ID.ValueString())
	tflog.Debug(ctx, "Update linode_user")

	client := r.Meta.Client
	id := state.ID.ValueString()
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
			fmt.Sprintf("Failed to update Linode User (%s)", id),
			err.Error(),
		)
		return
	}

	// ID may have changed if username was updated
	plan.ID = types.StringValue(username)

	if planGrantsChanged(plan, state) {
		resp.Diagnostics.Append(r.updateUserGrants(ctx, username, restricted, plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(r.readInto(ctx, username, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *UserResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", state.ID.ValueString())
	tflog.Debug(ctx, "Delete linode_user")

	client := r.Meta.Client
	username := state.ID.ValueString()

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := client.DeleteUser(ctx, username); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete Linode User (%s)", username),
			err.Error(),
		)
	}
}

func (r *UserResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	// Username is used as the ID
	var state ResourceModel
	state.ID = types.StringValue(req.ID)

	resp.Diagnostics.Append(r.readInto(ctx, req.ID, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// readInto fetches the user (and grants if restricted) and populates the model.
func (r *UserResource) readInto(ctx context.Context, username string, model *ResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	client := r.Meta.Client

	user, err := client.GetUser(ctx, username)
	if err != nil {
		if linodego.IsNotFound(err) {
			tflog.Warn(ctx, "Linode User not found; removing from state", map[string]any{
				"username": username,
			})
			// Signal removal by zeroing ID; caller must call resp.State.RemoveResource in Read.
			model.ID = types.StringNull()
			return diags
		}
		diags.AddError(
			fmt.Sprintf("Failed to get Linode User (%s)", username),
			err.Error(),
		)
		return diags
	}

	model.ID = types.StringValue(user.Username)
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
				fmt.Sprintf("Failed to get Linode User Grants (%s)", username),
				err.Error(),
			)
			return diags
		}

		diags.Append(flattenResourceGlobalGrants(ctx, grants.Global, model)...)
		if diags.HasError() {
			return diags
		}

		diags.Append(flattenResourceEntityGrants(ctx, grants.Domain, &model.DomainGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.Firewall, &model.FirewallGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.Image, &model.ImageGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.Linode, &model.LinodeGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.Longview, &model.LongviewGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.NodeBalancer, &model.NodebalancerGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.StackScript, &model.StackscriptGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.VPC, &model.VPCGrant)...)
		diags.Append(flattenResourceEntityGrants(ctx, grants.Volume, &model.VolumeGrant)...)
	} else {
		model.GlobalGrants = types.ObjectNull(resourceGrantsGlobalAttrTypes)
		model.DomainGrant = types.SetNull(resourceGrantEntityObjectType)
		model.FirewallGrant = types.SetNull(resourceGrantEntityObjectType)
		model.ImageGrant = types.SetNull(resourceGrantEntityObjectType)
		model.LinodeGrant = types.SetNull(resourceGrantEntityObjectType)
		model.LongviewGrant = types.SetNull(resourceGrantEntityObjectType)
		model.NodebalancerGrant = types.SetNull(resourceGrantEntityObjectType)
		model.StackscriptGrant = types.SetNull(resourceGrantEntityObjectType)
		model.VolumeGrant = types.SetNull(resourceGrantEntityObjectType)
		model.VPCGrant = types.SetNull(resourceGrantEntityObjectType)
	}

	return diags
}

// updateUserGrants calls the Linode API to update grants for a restricted user.
func (r *UserResource) updateUserGrants(
	ctx context.Context,
	username string,
	restricted bool,
	plan ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	if !restricted {
		diags.AddError(
			"Cannot set grants on unrestricted user",
			"User must be restricted in order to update grants.",
		)
		return diags
	}

	client := r.Meta.Client
	updateOpts := linodego.UserGrantsUpdateOptions{}

	// Global grants
	if !plan.GlobalGrants.IsNull() && !plan.GlobalGrants.IsUnknown() {
		globalGrants, d := expandResourceGlobalGrants(ctx, plan.GlobalGrants)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		updateOpts.Global = globalGrants
	}

	// Entity grants
	var d diag.Diagnostics

	updateOpts.Domain, d = expandResourceEntityGrants(ctx, plan.DomainGrant)
	diags.Append(d...)

	updateOpts.Firewall, d = expandResourceEntityGrants(ctx, plan.FirewallGrant)
	diags.Append(d...)

	updateOpts.Image, d = expandResourceEntityGrants(ctx, plan.ImageGrant)
	diags.Append(d...)

	updateOpts.Linode, d = expandResourceEntityGrants(ctx, plan.LinodeGrant)
	diags.Append(d...)

	updateOpts.Longview, d = expandResourceEntityGrants(ctx, plan.LongviewGrant)
	diags.Append(d...)

	updateOpts.NodeBalancer, d = expandResourceEntityGrants(ctx, plan.NodebalancerGrant)
	diags.Append(d...)

	updateOpts.StackScript, d = expandResourceEntityGrants(ctx, plan.StackscriptGrant)
	diags.Append(d...)

	updateOpts.VPC, d = expandResourceEntityGrants(ctx, plan.VPCGrant)
	diags.Append(d...)

	updateOpts.Volume, d = expandResourceEntityGrants(ctx, plan.VolumeGrant)
	diags.Append(d...)

	if diags.HasError() {
		return diags
	}

	tflog.Debug(ctx, "client.UpdateUserGrants(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUserGrants(ctx, username, updateOpts); err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to update Linode User Grants (%s)", username),
			err.Error(),
		)
	}

	return diags
}

// planGrantsChanged returns true if any grant field differs between plan and state.
func planGrantsChanged(plan, state ResourceModel) bool {
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

// flattenResourceGlobalGrants populates the global_grants object in the model.
func flattenResourceGlobalGrants(
	ctx context.Context,
	grants linodego.GlobalUserGrants,
	model *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	accountAccess := ""
	if grants.AccountAccess != nil {
		accountAccess = string(*grants.AccountAccess)
	}

	attrs := map[string]attr.Value{
		"account_access":        types.StringValue(accountAccess),
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

	obj, d := types.ObjectValue(resourceGrantsGlobalAttrTypes, attrs)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	model.GlobalGrants = obj
	return diags
}

// flattenResourceEntityGrants flattens entity grant list into a typed set, filtering empty-permission entries.
func flattenResourceEntityGrants(
	ctx context.Context,
	entities []linodego.GrantedEntity,
	target *types.Set,
) diag.Diagnostics {
	var diags diag.Diagnostics

	elems := make([]attr.Value, 0, len(entities))
	for _, entity := range entities {
		// Filter out entities with no permissions set to avoid false diffs.
		if entity.Permissions == "" {
			continue
		}

		obj, d := types.ObjectValue(resourceGrantEntityAttrTypes, map[string]attr.Value{
			"id":          types.Int64Value(int64(entity.ID)),
			"permissions": types.StringValue(string(entity.Permissions)),
		})
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		elems = append(elems, obj)
	}

	result, d := types.SetValue(resourceGrantEntityObjectType, elems)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	*target = result
	return diags
}

// expandResourceGlobalGrants converts the framework global_grants Object to linodego options.
func expandResourceGlobalGrants(
	ctx context.Context,
	obj types.Object,
) (linodego.GlobalUserGrants, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := linodego.GlobalUserGrants{}

	attrs := obj.Attributes()

	if v, ok := attrs["account_access"].(types.String); ok && !v.IsNull() && v.ValueString() != "" {
		perm := linodego.GrantPermissionLevel(v.ValueString())
		result.AccountAccess = &perm
	}

	if v, ok := attrs["add_domains"].(types.Bool); ok {
		result.AddDomains = v.ValueBool()
	}
	if v, ok := attrs["add_databases"].(types.Bool); ok {
		result.AddDatabases = v.ValueBool()
	}
	if v, ok := attrs["add_firewalls"].(types.Bool); ok {
		result.AddFirewalls = v.ValueBool()
	}
	if v, ok := attrs["add_images"].(types.Bool); ok {
		result.AddImages = v.ValueBool()
	}
	if v, ok := attrs["add_linodes"].(types.Bool); ok {
		result.AddLinodes = v.ValueBool()
	}
	if v, ok := attrs["add_longview"].(types.Bool); ok {
		result.AddLongview = v.ValueBool()
	}
	if v, ok := attrs["add_nodebalancers"].(types.Bool); ok {
		result.AddNodeBalancers = v.ValueBool()
	}
	if v, ok := attrs["add_stackscripts"].(types.Bool); ok {
		result.AddStackScripts = v.ValueBool()
	}
	if v, ok := attrs["add_volumes"].(types.Bool); ok {
		result.AddVolumes = v.ValueBool()
	}
	if v, ok := attrs["add_vpcs"].(types.Bool); ok {
		result.AddVPCs = v.ValueBool()
	}
	if v, ok := attrs["cancel_account"].(types.Bool); ok {
		result.CancelAccount = v.ValueBool()
	}
	if v, ok := attrs["longview_subscription"].(types.Bool); ok {
		result.LongviewSubscription = v.ValueBool()
	}

	return result, diags
}

// expandResourceEntityGrants converts a framework Set of entity grants to linodego options.
func expandResourceEntityGrants(
	ctx context.Context,
	set types.Set,
) ([]linodego.EntityUserGrant, diag.Diagnostics) {
	var diags diag.Diagnostics

	if set.IsNull() || set.IsUnknown() {
		return nil, diags
	}

	elems := set.Elements()
	result := make([]linodego.EntityUserGrant, 0, len(elems))

	for _, elem := range elems {
		obj, ok := elem.(types.Object)
		if !ok {
			continue
		}
		attrs := obj.Attributes()

		var id int64
		if v, ok := attrs["id"].(types.Int64); ok {
			id = v.ValueInt64()
		}

		var permissions string
		if v, ok := attrs["permissions"].(types.String); ok {
			permissions = v.ValueString()
		}

		perm := linodego.GrantPermissionLevel(permissions)
		result = append(result, linodego.EntityUserGrant{
			ID:          int(id),
			Permissions: &perm,
		})
	}

	return result, diags
}

// frameworkResourceSchema is the Plugin Framework schema for the linode_user resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:    true,
			Description: "The unique username of this user (used as resource ID).",
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
		},
		"ssh_keys": schema.ListAttribute{
			Computed:    true,
			ElementType: types.StringType,
			Description: "SSH keys to add to the user profile.",
		},
		"tfa_enabled": schema.BoolAttribute{
			Computed:    true,
			Description: "If the User has Two Factor Authentication (TFA) enabled.",
		},
	},
	Blocks: map[string]schema.Block{
		"global_grants": schema.SingleNestedBlock{
			Description: "A structure containing the Account-level grants a User has.",
			Attributes: map[string]schema.Attribute{
				"account_access": schema.StringAttribute{
					Optional: true,
					Description: "The level of access this User has to Account-level actions, like billing information. " +
						"A restricted User will never be able to manage users.",
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
		"domain_grant":       resourceGrantEntitySetBlock("domain"),
		"firewall_grant":     resourceGrantEntitySetBlock("firewall"),
		"image_grant":        resourceGrantEntitySetBlock("image"),
		"linode_grant":       resourceGrantEntitySetBlock("linode"),
		"longview_grant":     resourceGrantEntitySetBlock("longview"),
		"nodebalancer_grant": resourceGrantEntitySetBlock("nodebalancer"),
		"stackscript_grant":  resourceGrantEntitySetBlock("stackscript"),
		"volume_grant":       resourceGrantEntitySetBlock("volume"),
		"vpc_grant":          resourceGrantEntitySetBlock("vpc"),
	},
}

// resourceGrantEntitySetBlock returns a SetNestedBlock for entity grants.
func resourceGrantEntitySetBlock(entityName string) schema.SetNestedBlock {
	return schema.SetNestedBlock{
		Description: fmt.Sprintf("A set of %s grants for this user.", entityName),
		NestedObject: schema.NestedBlockObject{
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
}
