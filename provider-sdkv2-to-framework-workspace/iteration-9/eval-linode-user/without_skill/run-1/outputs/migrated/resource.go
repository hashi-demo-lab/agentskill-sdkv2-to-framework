package user

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// NewResource returns a new framework Resource for linode_user.
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

// frameworkResourceSchema is the terraform-plugin-framework schema for the linode_user resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed: true,
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
			ElementType: types.StringType,
			Computed:    true,
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
	},
	Blocks: map[string]schema.Block{
		// global_grants was MaxItems:1 TypeList in SDK v2 -> SingleNestedBlock in framework.
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
		// Entity grant sets (TypeSet + nested resource in SDK v2 -> SetNestedBlock in framework).
		"domain_grant":       resourceGrantEntitySetBlock(),
		"firewall_grant":     resourceGrantEntitySetBlock(),
		"image_grant":        resourceGrantEntitySetBlock(),
		"linode_grant":       resourceGrantEntitySetBlock(),
		"longview_grant":     resourceGrantEntitySetBlock(),
		"nodebalancer_grant": resourceGrantEntitySetBlock(),
		"stackscript_grant":  resourceGrantEntitySetBlock(),
		"volume_grant":       resourceGrantEntitySetBlock(),
		"vpc_grant":          resourceGrantEntitySetBlock(),
	},
}

func resourceGrantEntitySetBlock() schema.SetNestedBlock {
	return schema.SetNestedBlock{
		Description: "A set containing all of the user's active grants.",
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

// ---- Model types ----

// GlobalGrantsModel maps to the global_grants single nested block.
type GlobalGrantsModel struct {
	AccountAccess        types.String `tfsdk:"account_access"`
	AddDomains           types.Bool   `tfsdk:"add_domains"`
	AddDatabases         types.Bool   `tfsdk:"add_databases"`
	AddFirewalls         types.Bool   `tfsdk:"add_firewalls"`
	AddImages            types.Bool   `tfsdk:"add_images"`
	AddLinodes           types.Bool   `tfsdk:"add_linodes"`
	AddLongview          types.Bool   `tfsdk:"add_longview"`
	AddNodeBalancers     types.Bool   `tfsdk:"add_nodebalancers"`
	AddStackScripts      types.Bool   `tfsdk:"add_stackscripts"`
	AddVolumes           types.Bool   `tfsdk:"add_volumes"`
	AddVPCs              types.Bool   `tfsdk:"add_vpcs"`
	CancelAccount        types.Bool   `tfsdk:"cancel_account"`
	LongviewSubscription types.Bool   `tfsdk:"longview_subscription"`
}

// GrantEntityModel maps to each entity grant set block element.
type GrantEntityModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Permissions types.String `tfsdk:"permissions"`
}

// ResourceModel is the full resource state/plan model.
type ResourceModel struct {
	ID                types.String       `tfsdk:"id"`
	Email             types.String       `tfsdk:"email"`
	Username          types.String       `tfsdk:"username"`
	Restricted        types.Bool         `tfsdk:"restricted"`
	UserType          types.String       `tfsdk:"user_type"`
	SSHKeys           types.List         `tfsdk:"ssh_keys"`
	TFAEnabled        types.Bool         `tfsdk:"tfa_enabled"`
	GlobalGrants      *GlobalGrantsModel `tfsdk:"global_grants"`
	DomainGrant       []GrantEntityModel `tfsdk:"domain_grant"`
	FirewallGrant     []GrantEntityModel `tfsdk:"firewall_grant"`
	ImageGrant        []GrantEntityModel `tfsdk:"image_grant"`
	LinodeGrant       []GrantEntityModel `tfsdk:"linode_grant"`
	LongviewGrant     []GrantEntityModel `tfsdk:"longview_grant"`
	NodebalancerGrant []GrantEntityModel `tfsdk:"nodebalancer_grant"`
	StackscriptGrant  []GrantEntityModel `tfsdk:"stackscript_grant"`
	VolumeGrant       []GrantEntityModel `tfsdk:"volume_grant"`
	VPCGrant          []GrantEntityModel `tfsdk:"vpc_grant"`
}

// ---- Resource implementation ----

// Resource is the framework resource struct.
type Resource struct {
	helper.BaseResource
}

func (r *Resource) Create(
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
		resp.Diagnostics.AddError("Failed to create user", err.Error())
		return
	}

	plan.ID = types.StringValue(user.Username)
	ctx = tflog.SetField(ctx, "username", user.Username)

	// Apply grants if the user configured any.
	if plan.GlobalGrants != nil || hasEntityGrants(&plan) {
		resp.Diagnostics.Append(updateUserGrantsFromModel(ctx, client, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Read back to populate all computed fields.
	diags, removed := readIntoModel(ctx, client, user.Username, &plan)
	resp.Diagnostics.Append(diags...)
	if removed {
		resp.State.RemoveResource(ctx)
		return
	}
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

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if helper.FrameworkAttemptRemoveResourceForEmptyID(ctx, state.ID, resp) {
		return
	}

	username := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "username", username)

	diags, removed := readIntoModel(ctx, r.Meta.Client, username, &state)
	resp.Diagnostics.Append(diags...)
	if removed {
		resp.State.RemoveResource(ctx)
		return
	}
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

	var plan, state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client
	id := state.ID.ValueString()
	username := plan.Username.ValueString()
	restricted := plan.Restricted.ValueBool()

	ctx = tflog.SetField(ctx, "username", id)

	updateOpts := linodego.UserUpdateOptions{
		Username:   username,
		Restricted: &restricted,
	}

	tflog.Debug(ctx, "client.UpdateUser(...)", map[string]any{
		"options": updateOpts,
	})
	if _, err := client.UpdateUser(ctx, id, updateOpts); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to update user (%s)", id),
			err.Error(),
		)
		return
	}

	plan.ID = types.StringValue(username)

	// Update grants if configured.
	if plan.GlobalGrants != nil || hasEntityGrants(&plan) {
		resp.Diagnostics.Append(updateUserGrantsFromModel(ctx, client, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Read back to populate computed fields.
	diags, _ := readIntoModel(ctx, client, username, &plan)
	resp.Diagnostics.Append(diags...)
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

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	username := state.ID.ValueString()
	ctx = tflog.SetField(ctx, "username", username)

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := r.Meta.Client.DeleteUser(ctx, username); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete user (%s)", username),
			err.Error(),
		)
	}
}

func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...,
	)
}

// ---- Helpers ----

func hasEntityGrants(plan *ResourceModel) bool {
	return len(plan.DomainGrant) > 0 ||
		len(plan.FirewallGrant) > 0 ||
		len(plan.ImageGrant) > 0 ||
		len(plan.LinodeGrant) > 0 ||
		len(plan.LongviewGrant) > 0 ||
		len(plan.NodebalancerGrant) > 0 ||
		len(plan.StackscriptGrant) > 0 ||
		len(plan.VolumeGrant) > 0 ||
		len(plan.VPCGrant) > 0
}

// readIntoModel fetches user data from the API and populates model.
// Returns (diagnostics, wasRemoved). wasRemoved is true when the resource no
// longer exists in the API and the caller should call RemoveResource.
func readIntoModel(
	ctx context.Context,
	client linodego.Client,
	username string,
	model *ResourceModel,
) (diag.Diagnostics, bool) {
	var diags diag.Diagnostics

	user, err := client.GetUser(ctx, username)
	if err != nil {
		if linodego.IsNotFound(err) {
			diags.AddWarning(
				"User Not Found",
				fmt.Sprintf("Removing Linode User %q from state because it no longer exists", username),
			)
			return diags, true
		}
		diags.AddError(
			fmt.Sprintf("Failed to get user (%s)", username),
			err.Error(),
		)
		return diags, false
	}

	model.ID = types.StringValue(username)
	model.Username = types.StringValue(user.Username)
	model.Email = types.StringValue(user.Email)
	model.Restricted = types.BoolValue(user.Restricted)
	model.UserType = types.StringValue(string(user.UserType))
	model.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, d := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	diags.Append(d...)
	if diags.HasError() {
		return diags, false
	}
	model.SSHKeys = sshKeys

	if user.Restricted {
		grants, err := client.GetUserGrants(ctx, username)
		if err != nil {
			diags.AddError(
				fmt.Sprintf("Failed to get user grants (%s)", username),
				err.Error(),
			)
			return diags, false
		}
		flattenGrantsIntoModel(grants, model)
	}

	return diags, false
}

func flattenGrantsIntoModel(grants *linodego.UserGrants, model *ResourceModel) {
	model.GlobalGrants = flattenGlobalGrantsToModel(&grants.Global)
	model.DomainGrant = flattenEntityGrantsToModel(grants.Domain)
	model.FirewallGrant = flattenEntityGrantsToModel(grants.Firewall)
	model.ImageGrant = flattenEntityGrantsToModel(grants.Image)
	model.LinodeGrant = flattenEntityGrantsToModel(grants.Linode)
	model.LongviewGrant = flattenEntityGrantsToModel(grants.Longview)
	model.NodebalancerGrant = flattenEntityGrantsToModel(grants.NodeBalancer)
	model.StackscriptGrant = flattenEntityGrantsToModel(grants.StackScript)
	model.VolumeGrant = flattenEntityGrantsToModel(grants.Volume)
	model.VPCGrant = flattenEntityGrantsToModel(grants.VPC)
}

func flattenGlobalGrantsToModel(global *linodego.GlobalUserGrants) *GlobalGrantsModel {
	m := &GlobalGrantsModel{}

	if global.AccountAccess != nil {
		m.AccountAccess = types.StringValue(string(*global.AccountAccess))
	} else {
		m.AccountAccess = types.StringValue("")
	}

	m.AddDomains = types.BoolValue(global.AddDomains)
	m.AddDatabases = types.BoolValue(global.AddDatabases)
	m.AddFirewalls = types.BoolValue(global.AddFirewalls)
	m.AddImages = types.BoolValue(global.AddImages)
	m.AddLinodes = types.BoolValue(global.AddLinodes)
	m.AddLongview = types.BoolValue(global.AddLongview)
	m.AddNodeBalancers = types.BoolValue(global.AddNodeBalancers)
	m.AddStackScripts = types.BoolValue(global.AddStackScripts)
	m.AddVolumes = types.BoolValue(global.AddVolumes)
	m.AddVPCs = types.BoolValue(global.AddVPCs)
	m.CancelAccount = types.BoolValue(global.CancelAccount)
	m.LongviewSubscription = types.BoolValue(global.LongviewSubscription)

	return m
}

// flattenEntityGrantsToModel converts API grant entities to model slice,
// filtering out entries without permissions (same logic as SDK v2 resource).
func flattenEntityGrantsToModel(entities []linodego.GrantedEntity) []GrantEntityModel {
	var result []GrantEntityModel
	for _, entity := range entities {
		if entity.Permissions == "" {
			continue
		}
		result = append(result, GrantEntityModel{
			ID:          types.Int64Value(int64(entity.ID)),
			Permissions: types.StringValue(string(entity.Permissions)),
		})
	}
	return result
}

func updateUserGrantsFromModel(
	ctx context.Context,
	client linodego.Client,
	model *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	if !model.Restricted.ValueBool() {
		diags.AddError(
			"Cannot Update Grants for Unrestricted User",
			"User must be restricted in order to update grants.",
		)
		return diags
	}

	updateOpts := linodego.UserGrantsUpdateOptions{}

	if model.GlobalGrants != nil {
		updateOpts.Global = expandGlobalGrantsFromModel(model.GlobalGrants)
	}

	updateOpts.Domain = expandEntityGrantsFromModel(model.DomainGrant)
	updateOpts.Firewall = expandEntityGrantsFromModel(model.FirewallGrant)
	updateOpts.Image = expandEntityGrantsFromModel(model.ImageGrant)
	updateOpts.Linode = expandEntityGrantsFromModel(model.LinodeGrant)
	updateOpts.Longview = expandEntityGrantsFromModel(model.LongviewGrant)
	updateOpts.NodeBalancer = expandEntityGrantsFromModel(model.NodebalancerGrant)
	updateOpts.StackScript = expandEntityGrantsFromModel(model.StackscriptGrant)
	updateOpts.VPC = expandEntityGrantsFromModel(model.VPCGrant)
	updateOpts.Volume = expandEntityGrantsFromModel(model.VolumeGrant)

	tflog.Debug(ctx, "client.UpdateUserGrants(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUserGrants(ctx, model.ID.ValueString(), updateOpts); err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to update user grants (%s)", model.ID.ValueString()),
			err.Error(),
		)
	}

	return diags
}

func expandGlobalGrantsFromModel(m *GlobalGrantsModel) linodego.GlobalUserGrants {
	result := linodego.GlobalUserGrants{}

	if accountAccess := m.AccountAccess.ValueString(); accountAccess != "" {
		level := linodego.GrantPermissionLevel(accountAccess)
		result.AccountAccess = &level
	}

	result.AddDomains = m.AddDomains.ValueBool()
	result.AddDatabases = m.AddDatabases.ValueBool()
	result.AddFirewalls = m.AddFirewalls.ValueBool()
	result.AddImages = m.AddImages.ValueBool()
	result.AddLinodes = m.AddLinodes.ValueBool()
	result.AddLongview = m.AddLongview.ValueBool()
	result.AddNodeBalancers = m.AddNodeBalancers.ValueBool()
	result.AddStackScripts = m.AddStackScripts.ValueBool()
	result.AddVolumes = m.AddVolumes.ValueBool()
	result.AddVPCs = m.AddVPCs.ValueBool()
	result.CancelAccount = m.CancelAccount.ValueBool()
	result.LongviewSubscription = m.LongviewSubscription.ValueBool()

	return result
}

func expandEntityGrantsFromModel(entities []GrantEntityModel) []linodego.EntityUserGrant {
	result := make([]linodego.EntityUserGrant, len(entities))
	for i, e := range entities {
		permissions := linodego.GrantPermissionLevel(e.Permissions.ValueString())
		result[i] = linodego.EntityUserGrant{
			ID:          int(e.ID.ValueInt64()),
			Permissions: &permissions,
		}
	}
	return result
}
