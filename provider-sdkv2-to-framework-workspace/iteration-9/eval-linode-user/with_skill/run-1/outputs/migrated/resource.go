package user

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

func NewResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_user",
				IDAttr: "username",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

type Resource struct {
	helper.BaseResource
}

// ResourceModel holds the framework state for the linode_user resource.
type ResourceModel struct {
	Username           types.String `tfsdk:"username"`
	Email              types.String `tfsdk:"email"`
	Restricted         types.Bool   `tfsdk:"restricted"`
	UserType           types.String `tfsdk:"user_type"`
	SSHKeys            types.List   `tfsdk:"ssh_keys"`
	TFAEnabled         types.Bool   `tfsdk:"tfa_enabled"`
	GlobalGrants       types.Object `tfsdk:"global_grants"`
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

// resourceGrantsGlobalObjectType mirrors the single-nested block attributes.
var resourceGrantsGlobalObjectType = types.ObjectType{
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

// resourceGrantsEntityObjectType mirrors the set-nested attribute element type.
var resourceGrantsEntityObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":          types.Int64Type,
		"permissions": types.StringType,
	},
}

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_user")

	var plan ResourceModel
	client := r.Meta.Client

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

	ctx = tflog.SetField(ctx, "username", user.Username)

	// Read back the full state
	resp.Diagnostics.Append(r.readIntoModel(ctx, client, user.Username, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply grants if configured
	if resourceModelHasGrantsConfigured(&plan) {
		resp.Diagnostics.Append(r.updateUserGrants(ctx, client, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// Re-read to get computed grant values
		resp.Diagnostics.Append(r.readIntoModel(ctx, client, user.Username, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
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
	client := r.Meta.Client

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	username := state.Username.ValueString()
	ctx = tflog.SetField(ctx, "username", username)

	user, err := client.GetUser(ctx, username)
	if err != nil {
		if linodego.IsNotFound(err) {
			resp.Diagnostics.AddWarning(
				"User No Longer Exists",
				fmt.Sprintf("Removing Linode User %q from state because it no longer exists", username),
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to get user (%s)", username),
			err.Error(),
		)
		return
	}

	state.Username = types.StringValue(user.Username)
	state.Email = types.StringValue(user.Email)
	state.Restricted = types.BoolValue(user.Restricted)
	state.UserType = types.StringValue(string(user.UserType))
	state.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, d := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.SSHKeys = sshKeys

	if user.Restricted {
		grants, err := client.GetUserGrants(ctx, username)
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to get user grants (%s)", username),
				err.Error(),
			)
			return
		}
		resp.Diagnostics.Append(flattenResourceGrants(ctx, grants, &state)...)
	} else {
		state.GlobalGrants = types.ObjectNull(resourceGrantsGlobalObjectType.AttrTypes)
		state.DomainGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.FirewallGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.ImageGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.LinodeGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.LongviewGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.NodebalancerGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.StackscriptGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.VolumeGrant = types.SetNull(resourceGrantsEntityObjectType)
		state.VPCGrant = types.SetNull(resourceGrantsEntityObjectType)
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

	var state, plan ResourceModel
	client := r.Meta.Client

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldUsername := state.Username.ValueString()
	ctx = tflog.SetField(ctx, "username", oldUsername)

	restricted := plan.Restricted.ValueBool()
	updateOpts := linodego.UserUpdateOptions{
		Username:   plan.Username.ValueString(),
		Restricted: &restricted,
	}

	tflog.Debug(ctx, "client.UpdateUser(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUser(ctx, oldUsername, updateOpts); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to update user (%s)", oldUsername),
			err.Error(),
		)
		return
	}

	newUsername := plan.Username.ValueString()

	// Determine if grants changed by comparing state vs plan
	if grantsChanged(&state, &plan) {
		resp.Diagnostics.Append(r.updateUserGrants(ctx, client, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(r.readIntoModel(ctx, client, newUsername, &plan)...)
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
	client := r.Meta.Client

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	username := state.Username.ValueString()
	ctx = tflog.SetField(ctx, "username", username)

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := client.DeleteUser(ctx, username); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete user (%s)", username),
			err.Error(),
		)
	}
}

// readIntoModel fetches the user (and if restricted, the grants) and populates m.
func (r *Resource) readIntoModel(
	ctx context.Context,
	client *linodego.Client,
	username string,
	m *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	user, err := client.GetUser(ctx, username)
	if err != nil {
		if linodego.IsNotFound(err) {
			return diags
		}
		diags.AddError(
			fmt.Sprintf("Failed to get user (%s)", username),
			err.Error(),
		)
		return diags
	}

	m.Username = types.StringValue(user.Username)
	m.Email = types.StringValue(user.Email)
	m.Restricted = types.BoolValue(user.Restricted)
	m.UserType = types.StringValue(string(user.UserType))
	m.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, d := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	m.SSHKeys = sshKeys

	if user.Restricted {
		grants, err := client.GetUserGrants(ctx, username)
		if err != nil {
			diags.AddError(
				fmt.Sprintf("Failed to get user grants (%s)", username),
				err.Error(),
			)
			return diags
		}
		diags.Append(flattenResourceGrants(ctx, grants, m)...)
	} else {
		// Clear grants to null when user is not restricted
		m.GlobalGrants = types.ObjectNull(resourceGrantsGlobalObjectType.AttrTypes)
		m.DomainGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.FirewallGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.ImageGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.LinodeGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.LongviewGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.NodebalancerGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.StackscriptGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.VolumeGrant = types.SetNull(resourceGrantsEntityObjectType)
		m.VPCGrant = types.SetNull(resourceGrantsEntityObjectType)
	}

	return diags
}

// flattenResourceGrants populates grant fields in the model from API grants.
func flattenResourceGrants(
	ctx context.Context,
	grants *linodego.UserGrants,
	m *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	// Global grants → SingleNestedBlock represented as types.Object
	globalAttrs := map[string]attr.Value{
		"add_domains":           types.BoolValue(grants.Global.AddDomains),
		"add_databases":         types.BoolValue(grants.Global.AddDatabases),
		"add_firewalls":         types.BoolValue(grants.Global.AddFirewalls),
		"add_images":            types.BoolValue(grants.Global.AddImages),
		"add_linodes":           types.BoolValue(grants.Global.AddLinodes),
		"add_longview":          types.BoolValue(grants.Global.AddLongview),
		"add_nodebalancers":     types.BoolValue(grants.Global.AddNodeBalancers),
		"add_stackscripts":      types.BoolValue(grants.Global.AddStackScripts),
		"add_volumes":           types.BoolValue(grants.Global.AddVolumes),
		"add_vpcs":              types.BoolValue(grants.Global.AddVPCs),
		"cancel_account":        types.BoolValue(grants.Global.CancelAccount),
		"longview_subscription": types.BoolValue(grants.Global.LongviewSubscription),
	}
	if grants.Global.AccountAccess != nil {
		globalAttrs["account_access"] = types.StringValue(string(*grants.Global.AccountAccess))
	} else {
		globalAttrs["account_access"] = types.StringValue("")
	}

	globalObj, d := types.ObjectValue(resourceGrantsGlobalObjectType.AttrTypes, globalAttrs)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	m.GlobalGrants = globalObj

	// Entity grants
	var d2 diag.Diagnostics
	m.DomainGrant, d2 = flattenResourceGrantEntities(ctx, grants.Domain)
	diags.Append(d2...)
	m.FirewallGrant, d2 = flattenResourceGrantEntities(ctx, grants.Firewall)
	diags.Append(d2...)
	m.ImageGrant, d2 = flattenResourceGrantEntities(ctx, grants.Image)
	diags.Append(d2...)
	m.LinodeGrant, d2 = flattenResourceGrantEntities(ctx, grants.Linode)
	diags.Append(d2...)
	m.LongviewGrant, d2 = flattenResourceGrantEntities(ctx, grants.Longview)
	diags.Append(d2...)
	m.NodebalancerGrant, d2 = flattenResourceGrantEntities(ctx, grants.NodeBalancer)
	diags.Append(d2...)
	m.StackscriptGrant, d2 = flattenResourceGrantEntities(ctx, grants.StackScript)
	diags.Append(d2...)
	m.VolumeGrant, d2 = flattenResourceGrantEntities(ctx, grants.Volume)
	diags.Append(d2...)
	m.VPCGrant, d2 = flattenResourceGrantEntities(ctx, grants.VPC)
	diags.Append(d2...)

	return diags
}

// flattenResourceGrantEntities converts a slice of API GrantedEntity into a types.Set,
// filtering out entities with no permissions (as the SDKv2 flatten did).
func flattenResourceGrantEntities(
	ctx context.Context,
	entities []linodego.GrantedEntity,
) (basetypes.SetValue, diag.Diagnostics) {
	var objs []attr.Value

	for _, entity := range entities {
		if entity.Permissions == "" {
			continue
		}
		obj, d := types.ObjectValue(resourceGrantsEntityObjectType.AttrTypes, map[string]attr.Value{
			"id":          types.Int64Value(int64(entity.ID)),
			"permissions": types.StringValue(string(entity.Permissions)),
		})
		if d.HasError() {
			return types.SetNull(resourceGrantsEntityObjectType), d
		}
		objs = append(objs, obj)
	}

	return types.SetValueFrom(ctx, resourceGrantsEntityObjectType, objs)
}

// updateUserGrants applies grant updates from the model to the API.
func (r *Resource) updateUserGrants(
	ctx context.Context,
	client *linodego.Client,
	plan *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	username := plan.Username.ValueString()

	if !plan.Restricted.ValueBool() {
		diags.AddError(
			"Cannot update grants for unrestricted user",
			"User must be restricted in order to update grants.",
		)
		return diags
	}

	updateOpts := linodego.UserGrantsUpdateOptions{}

	// Global grants
	if !plan.GlobalGrants.IsNull() && !plan.GlobalGrants.IsUnknown() {
		var globalModel struct {
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
		diags.Append(plan.GlobalGrants.As(ctx, &globalModel, basetypes.ObjectAsOptions{})...)
		if diags.HasError() {
			return diags
		}

		global := linodego.GlobalUserGrants{}
		if !globalModel.AccountAccess.IsNull() && globalModel.AccountAccess.ValueString() != "" {
			aa := linodego.GrantPermissionLevel(globalModel.AccountAccess.ValueString())
			global.AccountAccess = &aa
		}
		global.AddDomains = globalModel.AddDomains.ValueBool()
		global.AddDatabases = globalModel.AddDatabases.ValueBool()
		global.AddFirewalls = globalModel.AddFirewalls.ValueBool()
		global.AddImages = globalModel.AddImages.ValueBool()
		global.AddLinodes = globalModel.AddLinodes.ValueBool()
		global.AddLongview = globalModel.AddLongview.ValueBool()
		global.AddNodeBalancers = globalModel.AddNodeBalancers.ValueBool()
		global.AddStackScripts = globalModel.AddStackScripts.ValueBool()
		global.AddVolumes = globalModel.AddVolumes.ValueBool()
		global.AddVPCs = globalModel.AddVPCs.ValueBool()
		global.CancelAccount = globalModel.CancelAccount.ValueBool()
		global.LongviewSubscription = globalModel.LongviewSubscription.ValueBool()
		updateOpts.Global = global
	}

	updateOpts.Domain = expandResourceGrantEntities(ctx, plan.DomainGrant, &diags)
	updateOpts.Firewall = expandResourceGrantEntities(ctx, plan.FirewallGrant, &diags)
	updateOpts.Image = expandResourceGrantEntities(ctx, plan.ImageGrant, &diags)
	updateOpts.Linode = expandResourceGrantEntities(ctx, plan.LinodeGrant, &diags)
	updateOpts.Longview = expandResourceGrantEntities(ctx, plan.LongviewGrant, &diags)
	updateOpts.NodeBalancer = expandResourceGrantEntities(ctx, plan.NodebalancerGrant, &diags)
	updateOpts.StackScript = expandResourceGrantEntities(ctx, plan.StackscriptGrant, &diags)
	updateOpts.VPC = expandResourceGrantEntities(ctx, plan.VPCGrant, &diags)
	updateOpts.Volume = expandResourceGrantEntities(ctx, plan.VolumeGrant, &diags)

	if diags.HasError() {
		return diags
	}

	tflog.Debug(ctx, "client.UpdateUserGrants(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUserGrants(ctx, username, updateOpts); err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to update user grants (%s)", username),
			err.Error(),
		)
	}

	return diags
}

// expandResourceGrantEntities converts a types.Set of entity objects into []linodego.EntityUserGrant.
func expandResourceGrantEntities(
	ctx context.Context,
	set types.Set,
	diags *diag.Diagnostics,
) []linodego.EntityUserGrant {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}

	type entityModel struct {
		ID          types.Int64  `tfsdk:"id"`
		Permissions types.String `tfsdk:"permissions"`
	}

	var elements []entityModel
	diags.Append(set.ElementsAs(ctx, &elements, false)...)

	result := make([]linodego.EntityUserGrant, len(elements))
	for i, e := range elements {
		perm := linodego.GrantPermissionLevel(e.Permissions.ValueString())
		result[i] = linodego.EntityUserGrant{
			ID:          int(e.ID.ValueInt64()),
			Permissions: &perm,
		}
	}
	return result
}

// resourceModelHasGrantsConfigured returns true if any grant field is non-null in the plan.
func resourceModelHasGrantsConfigured(m *ResourceModel) bool {
	if !m.GlobalGrants.IsNull() && !m.GlobalGrants.IsUnknown() {
		return true
	}
	sets := []types.Set{
		m.DomainGrant, m.FirewallGrant, m.ImageGrant, m.LinodeGrant,
		m.LongviewGrant, m.NodebalancerGrant, m.StackscriptGrant,
		m.VolumeGrant, m.VPCGrant,
	}
	for _, s := range sets {
		if !s.IsNull() && !s.IsUnknown() {
			return true
		}
	}
	return false
}

// grantsChanged compares state vs plan to detect grant attribute changes.
func grantsChanged(state, plan *ResourceModel) bool {
	if !state.GlobalGrants.Equal(plan.GlobalGrants) {
		return true
	}
	if !state.DomainGrant.Equal(plan.DomainGrant) {
		return true
	}
	if !state.FirewallGrant.Equal(plan.FirewallGrant) {
		return true
	}
	if !state.ImageGrant.Equal(plan.ImageGrant) {
		return true
	}
	if !state.LinodeGrant.Equal(plan.LinodeGrant) {
		return true
	}
	if !state.LongviewGrant.Equal(plan.LongviewGrant) {
		return true
	}
	if !state.NodebalancerGrant.Equal(plan.NodebalancerGrant) {
		return true
	}
	if !state.StackscriptGrant.Equal(plan.StackscriptGrant) {
		return true
	}
	if !state.VolumeGrant.Equal(plan.VolumeGrant) {
		return true
	}
	if !state.VPCGrant.Equal(plan.VPCGrant) {
		return true
	}
	return false
}
