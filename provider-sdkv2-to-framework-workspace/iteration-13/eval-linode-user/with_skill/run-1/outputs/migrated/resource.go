package user

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// In SDKv2 the grant-bearing attribute names lived in a top-level
// resourceLinodeUserGrantFields slice consumed by d.HasChanges(...). The
// framework analogue is grantsChanged() below, which compares the typed
// fields directly — there is no dynamic-string path equivalent.

// linodeUserGrantsGlobalObjectType describes the per-element object shape of
// the global_grants list block, used when reading state back into a
// types.List value.
var resourceLinodeUserGrantsGlobalObjectType = types.ObjectType{
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

// resourceLinodeUserGrantsEntityObjectType describes the per-element shape of
// the domain_grant / firewall_grant / ... set blocks. Note this differs from
// the data-source schema, which also exposes a "label" — the resource schema
// historically did not.
var resourceLinodeUserGrantsEntityObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":          types.Int64Type,
		"permissions": types.StringType,
	},
}

// ResourceModel is the in-Go representation of the framework schema. The
// `tfsdk:` tags must match attribute names exactly — a typo silently turns
// the field into a no-op.
type ResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Email             types.String `tfsdk:"email"`
	Username          types.String `tfsdk:"username"`
	Restricted        types.Bool   `tfsdk:"restricted"`
	UserType          types.String `tfsdk:"user_type"`
	SSHKeys           types.List   `tfsdk:"ssh_keys"`
	TFAEnabled        types.Bool   `tfsdk:"tfa_enabled"`
	GlobalGrants      types.List   `tfsdk:"global_grants"`
	DomainGrant       types.Set    `tfsdk:"domain_grant"`
	FirewallGrant     types.Set    `tfsdk:"firewall_grant"`
	ImageGrant        types.Set    `tfsdk:"image_grant"`
	LinodeGrant       types.Set    `tfsdk:"linode_grant"`
	LongviewGrant     types.Set    `tfsdk:"longview_grant"`
	NodebalancerGrant types.Set    `tfsdk:"nodebalancer_grant"`
	StackscriptGrant  types.Set    `tfsdk:"stackscript_grant"`
	VolumeGrant       types.Set    `tfsdk:"volume_grant"`
	VPCGrant          types.Set    `tfsdk:"vpc_grant"`
}

// NewResource conforms to the project-wide resource constructor convention:
// every framework resource embeds helper.BaseResource and supplies its name
// + schema via BaseResourceConfig. Metadata, Schema, Configure, and
// ImportState are inherited from the base. ID type is StringType because
// linode_user IDs are usernames, not numeric.
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

type Resource struct {
	helper.BaseResource
}

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create "+r.Config.Name)

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
		resp.Diagnostics.AddError(
			"Failed to create user",
			err.Error(),
		)
		return
	}

	// linode_user.ID is the username.
	plan.ID = types.StringValue(user.Username)

	ctx = populateLogAttributes(ctx, plan)

	if userHasGrantsConfigured(plan) {
		if err := r.updateUserGrants(ctx, plan); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to set user grants (%s)", user.Username),
				err.Error(),
			)
			return
		}
	}

	// Refresh remote state. We pass through the freshly-built plan so the
	// post-read model carries the correct ID.
	r.refresh(ctx, &plan, &resp.Diagnostics)
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
	tflog.Debug(ctx, "Read "+r.Config.Name)

	var data ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributes(ctx, data)

	r.refresh(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// refresh signals "gone from upstream" by emitting an empty ID; mirror
	// the SDKv2 d.SetId("") flow by removing the resource from state.
	if data.ID.ValueString() == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update "+r.Config.Name)

	var plan, state ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributes(ctx, state)

	client := r.Meta.Client

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

	// linode_user uses the username as ID; if the username changed, the ID
	// must follow.
	plan.ID = types.StringValue(username)

	if grantsChanged(plan, state) {
		if err := r.updateUserGrants(ctx, plan); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to update user grants (%s)", username),
				err.Error(),
			)
			return
		}
	}

	r.refresh(ctx, &plan, &resp.Diagnostics)
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
	tflog.Debug(ctx, "Delete "+r.Config.Name)

	// Pitfall: req.Plan is null on Delete; read from req.State.
	var data ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributes(ctx, data)

	client := r.Meta.Client

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := client.DeleteUser(ctx, data.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete user (%s)", data.ID.ValueString()),
			err.Error(),
		)
		return
	}
}

// ImportState uses the legacy passthrough semantics from SDKv2: the user
// passes the username as the import ID, and we set it as the resource's id
// attribute. We override BaseResource.ImportState because the base assumes
// Int64Type IDs by default and parses with strconv; user IDs are strings.
func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// refresh reads the remote user (and its grants if restricted) and writes
// the result into data. data.ID is set to "" if the user no longer exists,
// to mirror the SDKv2 d.SetId("") "drop from state" idiom.
func (r *Resource) refresh(
	ctx context.Context,
	data *ResourceModel,
	diags *diag.Diagnostics,
) {
	client := r.Meta.Client
	username := data.ID.ValueString()

	user, err := client.GetUser(ctx, username)
	if err != nil {
		if linodego.IsNotFound(err) {
			tflog.Warn(
				ctx,
				"removing Linode User from state because it no longer exists",
				map[string]any{"username": username},
			)
			data.ID = types.StringValue("")
			return
		}
		diags.AddError(
			fmt.Sprintf("Failed to get user (%s)", username),
			err.Error(),
		)
		return
	}

	data.Username = types.StringValue(username)
	data.Email = types.StringValue(user.Email)
	data.Restricted = types.BoolValue(user.Restricted)
	data.UserType = types.StringValue(string(user.UserType))
	data.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, d := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	data.SSHKeys = sshKeys

	if user.Restricted {
		// Only fetch grants from upstream if the practitioner actually
		// configured at least one grants attribute. Blocks in the
		// framework cannot be Optional+Computed (the SDKv2 schema relied
		// on that), so we mimic "populate when set" by leaving null
		// blocks alone — preventing a "Provider produced inconsistent
		// result after apply" error when the plan said null but Read
		// would have written a value.
		if userHasGrantsConfigured(*data) {
			grants, err := client.GetUserGrants(ctx, username)
			if err != nil {
				diags.AddError(
					fmt.Sprintf("Failed to get user grants (%s)", username),
					err.Error(),
				)
				return
			}

			if !data.GlobalGrants.IsNull() {
				globalList, d := flattenResourceGlobalGrants(&grants.Global)
				diags.Append(d...)
				if diags.HasError() {
					return
				}
				data.GlobalGrants = globalList
			}

			entitySets := []struct {
				dst    *types.Set
				source []linodego.GrantedEntity
			}{
				{&data.DomainGrant, grants.Domain},
				{&data.FirewallGrant, grants.Firewall},
				{&data.ImageGrant, grants.Image},
				{&data.LinodeGrant, grants.Linode},
				{&data.LongviewGrant, grants.Longview},
				{&data.NodebalancerGrant, grants.NodeBalancer},
				{&data.StackscriptGrant, grants.StackScript},
				{&data.VolumeGrant, grants.Volume},
				{&data.VPCGrant, grants.VPC},
			}
			for _, e := range entitySets {
				if e.dst.IsNull() {
					continue
				}
				set, d := flattenResourceGrantEntities(e.source)
				diags.Append(d...)
				if diags.HasError() {
					return
				}
				*e.dst = set
			}
		}
	} else {
		// Non-restricted users have no grants surface; null out the
		// optional+computed grant attributes so the framework doesn't
		// surface "(known after apply)" or stale prior values.
		data.GlobalGrants = types.ListNull(resourceLinodeUserGrantsGlobalObjectType)
		data.DomainGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.FirewallGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.ImageGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.LinodeGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.LongviewGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.NodebalancerGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.StackscriptGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.VolumeGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
		data.VPCGrant = types.SetNull(resourceLinodeUserGrantsEntityObjectType)
	}
}

// updateUserGrants pushes the locally-configured grants from data up to the
// API. This is invoked from Create (when the user supplied any grants
// fields) and Update (when any grants attribute changed).
func (r *Resource) updateUserGrants(ctx context.Context, data ResourceModel) error {
	client := r.Meta.Client

	username := data.ID.ValueString()
	restricted := data.Restricted.ValueBool()

	// TODO: surface this through ConfigValidators / ValidateConfig at
	// plan-time rather than failing during apply.
	if !restricted {
		return fmt.Errorf("user must be restricted in order to update grants")
	}

	updateOpts := linodego.UserGrantsUpdateOptions{}

	if !data.GlobalGrants.IsNull() && !data.GlobalGrants.IsUnknown() {
		elems := data.GlobalGrants.Elements()
		if len(elems) > 0 {
			obj, ok := elems[0].(basetypes.ObjectValue)
			if ok {
				updateOpts.Global = expandResourceGlobalGrants(obj)
			}
		}
	}

	updateOpts.Domain = expandResourceGrantEntities(data.DomainGrant)
	updateOpts.Firewall = expandResourceGrantEntities(data.FirewallGrant)
	updateOpts.Image = expandResourceGrantEntities(data.ImageGrant)
	updateOpts.Linode = expandResourceGrantEntities(data.LinodeGrant)
	updateOpts.Longview = expandResourceGrantEntities(data.LongviewGrant)
	updateOpts.NodeBalancer = expandResourceGrantEntities(data.NodebalancerGrant)
	updateOpts.StackScript = expandResourceGrantEntities(data.StackscriptGrant)
	updateOpts.VPC = expandResourceGrantEntities(data.VPCGrant)
	updateOpts.Volume = expandResourceGrantEntities(data.VolumeGrant)

	tflog.Debug(ctx, "client.UpdateUserGrants(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUserGrants(ctx, username, updateOpts); err != nil {
		return err
	}

	return nil
}

// userHasGrantsConfigured returns true iff any of the grants attributes is
// non-null and non-unknown — the framework analogue of SDKv2's d.GetOk loop.
func userHasGrantsConfigured(data ResourceModel) bool {
	if !data.GlobalGrants.IsNull() && !data.GlobalGrants.IsUnknown() && len(data.GlobalGrants.Elements()) > 0 {
		return true
	}
	sets := []types.Set{
		data.DomainGrant, data.FirewallGrant, data.ImageGrant,
		data.LinodeGrant, data.LongviewGrant, data.NodebalancerGrant,
		data.StackscriptGrant, data.VolumeGrant, data.VPCGrant,
	}
	for _, s := range sets {
		if !s.IsNull() && !s.IsUnknown() && len(s.Elements()) > 0 {
			return true
		}
	}
	return false
}

// grantsChanged compares plan and prior state for any of the grant
// collections. Equivalent to SDKv2 d.HasChanges(resourceLinodeUserGrantFields...).
func grantsChanged(plan, state ResourceModel) bool {
	if !plan.GlobalGrants.Equal(state.GlobalGrants) {
		return true
	}
	pairs := []struct{ p, s types.Set }{
		{plan.DomainGrant, state.DomainGrant},
		{plan.FirewallGrant, state.FirewallGrant},
		{plan.ImageGrant, state.ImageGrant},
		{plan.LinodeGrant, state.LinodeGrant},
		{plan.LongviewGrant, state.LongviewGrant},
		{plan.NodebalancerGrant, state.NodebalancerGrant},
		{plan.StackscriptGrant, state.StackscriptGrant},
		{plan.VolumeGrant, state.VolumeGrant},
		{plan.VPCGrant, state.VPCGrant},
	}
	for _, p := range pairs {
		if !p.p.Equal(p.s) {
			return true
		}
	}
	return false
}

// flattenResourceGlobalGrants converts the API representation of the
// account-level grants object into a single-element types.List, matching
// the schema's "global_grants" ListNestedBlock with SizeAtMost(1).
func flattenResourceGlobalGrants(
	grants *linodego.GlobalUserGrants,
) (types.List, diag.Diagnostics) {
	attrs := map[string]attr.Value{
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
		attrs["account_access"] = types.StringValue(string(*grants.AccountAccess))
	} else {
		// Match prior behaviour: the SDKv2 flatten emitted "" for nil so
		// existing test expectations like "account_access" == "" continue
		// to hold.
		attrs["account_access"] = types.StringValue("")
	}

	obj, d := types.ObjectValue(resourceLinodeUserGrantsGlobalObjectType.AttrTypes, attrs)
	if d.HasError() {
		return types.ListNull(resourceLinodeUserGrantsGlobalObjectType), d
	}

	list, d2 := types.ListValue(resourceLinodeUserGrantsGlobalObjectType, []attr.Value{obj})
	d.Append(d2...)
	return list, d
}

// flattenResourceGrantEntities converts a slice of GrantedEntity into a
// types.Set matching one of the *_grant SetNestedBlock attributes. Entities
// with empty permissions are dropped, matching the original SDKv2 flatten
// (which filtered them to avoid spurious diffs).
func flattenResourceGrantEntities(
	entities []linodego.GrantedEntity,
) (types.Set, diag.Diagnostics) {
	values := make([]attr.Value, 0, len(entities))
	for _, e := range entities {
		if e.Permissions == "" {
			continue
		}
		obj, d := types.ObjectValue(
			resourceLinodeUserGrantsEntityObjectType.AttrTypes,
			map[string]attr.Value{
				"id":          types.Int64Value(int64(e.ID)),
				"permissions": types.StringValue(string(e.Permissions)),
			},
		)
		if d.HasError() {
			return types.SetNull(resourceLinodeUserGrantsEntityObjectType), d
		}
		values = append(values, obj)
	}
	set, d := types.SetValue(resourceLinodeUserGrantsEntityObjectType, values)
	return set, d
}

// expandResourceGlobalGrants converts the framework single-element list
// element into the API options struct.
func expandResourceGlobalGrants(obj basetypes.ObjectValue) linodego.GlobalUserGrants {
	out := linodego.GlobalUserGrants{}
	attrs := obj.Attributes()

	if v, ok := attrs["account_access"].(types.String); ok && !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		level := linodego.GrantPermissionLevel(v.ValueString())
		out.AccountAccess = &level
	}

	out.AddDomains = boolFromAttrs(attrs, "add_domains")
	out.AddDatabases = boolFromAttrs(attrs, "add_databases")
	out.AddFirewalls = boolFromAttrs(attrs, "add_firewalls")
	out.AddImages = boolFromAttrs(attrs, "add_images")
	out.AddLinodes = boolFromAttrs(attrs, "add_linodes")
	out.AddLongview = boolFromAttrs(attrs, "add_longview")
	out.AddNodeBalancers = boolFromAttrs(attrs, "add_nodebalancers")
	out.AddStackScripts = boolFromAttrs(attrs, "add_stackscripts")
	out.AddVolumes = boolFromAttrs(attrs, "add_volumes")
	out.AddVPCs = boolFromAttrs(attrs, "add_vpcs")
	out.CancelAccount = boolFromAttrs(attrs, "cancel_account")
	out.LongviewSubscription = boolFromAttrs(attrs, "longview_subscription")

	return out
}

// expandResourceGrantEntities converts a *_grant set into the
// linodego.EntityUserGrant slice the API expects.
func expandResourceGrantEntities(set types.Set) []linodego.EntityUserGrant {
	if set.IsNull() || set.IsUnknown() {
		return nil
	}
	elems := set.Elements()
	out := make([]linodego.EntityUserGrant, 0, len(elems))
	for _, e := range elems {
		obj, ok := e.(basetypes.ObjectValue)
		if !ok {
			continue
		}
		attrs := obj.Attributes()

		entity := linodego.EntityUserGrant{}
		if v, ok := attrs["id"].(types.Int64); ok {
			entity.ID = int(v.ValueInt64())
		}
		if v, ok := attrs["permissions"].(types.String); ok && !v.IsNull() {
			perm := linodego.GrantPermissionLevel(v.ValueString())
			entity.Permissions = &perm
		}
		out = append(out, entity)
	}
	return out
}

func boolFromAttrs(attrs map[string]attr.Value, key string) bool {
	if v, ok := attrs[key].(types.Bool); ok {
		return v.ValueBool()
	}
	return false
}

func populateLogAttributes(ctx context.Context, data ResourceModel) context.Context {
	return tflog.SetField(ctx, "username", data.ID.ValueString())
}
