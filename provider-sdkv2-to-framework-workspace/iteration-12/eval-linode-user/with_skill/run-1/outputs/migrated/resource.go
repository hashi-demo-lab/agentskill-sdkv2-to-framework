package user

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// Ensure Resource implements the framework resource interfaces.
var _ resource.Resource = &Resource{}
var _ resource.ResourceWithImportState = &Resource{}

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

// ResourceModel holds the Terraform state for a linode_user resource.
type ResourceModel struct {
	Username           types.String `tfsdk:"username"`
	Email              types.String `tfsdk:"email"`
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

// resourceGrantsGlobalObjectType mirrors the global_grants block object type for the resource.
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

// resourceGrantsEntityObjectType mirrors the entity grant block object type for the resource.
var resourceGrantsEntityObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":          types.Int64Type,
		"permissions": types.StringType,
	},
}

var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"username": schema.StringAttribute{
			Description: "The username of the user.",
			Required:    true,
		},
		"email": schema.StringAttribute{
			Description: "The email of the user.",
			Required:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
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
		"domain_grant":       resourceGrantsEntitySetAttribute(),
		"firewall_grant":     resourceGrantsEntitySetAttribute(),
		"image_grant":        resourceGrantsEntitySetAttribute(),
		"linode_grant":       resourceGrantsEntitySetAttribute(),
		"longview_grant":     resourceGrantsEntitySetAttribute(),
		"nodebalancer_grant": resourceGrantsEntitySetAttribute(),
		"stackscript_grant":  resourceGrantsEntitySetAttribute(),
		"volume_grant":       resourceGrantsEntitySetAttribute(),
		"vpc_grant":          resourceGrantsEntitySetAttribute(),
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
						Description: "If true, this User may manage the Account's Longview subscription.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
				},
			},
		},
	},
}

func resourceGrantsEntitySetAttribute() schema.SetNestedAttribute {
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

// Create implements resource.Resource.
func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_user")

	var data ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client

	createOpts := linodego.UserCreateOptions{
		Email:      data.Email.ValueString(),
		Username:   data.Username.ValueString(),
		Restricted: data.Restricted.ValueBool(),
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

	if hasGrantsConfigured(&data) {
		resp.Diagnostics.Append(updateUserGrantsFramework(ctx, &data, client)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(readResourceInto(ctx, client, user.Username, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read implements resource.Resource.
func (r *Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var data ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", data.Username.ValueString())
	tflog.Debug(ctx, "Read linode_user")

	client := r.Meta.Client

	resp.Diagnostics.Append(readResourceInto(ctx, client, data.Username.ValueString(), &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update implements resource.Resource.
func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var data ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", state.Username.ValueString())
	tflog.Debug(ctx, "Update linode_user")

	client := r.Meta.Client

	id := state.Username.ValueString()
	username := data.Username.ValueString()
	restricted := data.Restricted.ValueBool()

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

	if !data.GlobalGrants.Equal(state.GlobalGrants) ||
		!data.DomainGrant.Equal(state.DomainGrant) ||
		!data.FirewallGrant.Equal(state.FirewallGrant) ||
		!data.ImageGrant.Equal(state.ImageGrant) ||
		!data.LinodeGrant.Equal(state.LinodeGrant) ||
		!data.LongviewGrant.Equal(state.LongviewGrant) ||
		!data.NodebalancerGrant.Equal(state.NodebalancerGrant) ||
		!data.StackscriptGrant.Equal(state.StackscriptGrant) ||
		!data.VolumeGrant.Equal(state.VolumeGrant) ||
		!data.VPCGrant.Equal(state.VPCGrant) {
		resp.Diagnostics.Append(updateUserGrantsFramework(ctx, &data, client)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(readResourceInto(ctx, client, username, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete implements resource.Resource.
func (r *Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var data ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "username", data.Username.ValueString())
	tflog.Debug(ctx, "Delete linode_user")

	client := r.Meta.Client
	username := data.Username.ValueString()

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := client.DeleteUser(ctx, username); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete user (%s)", username),
			err.Error(),
		)
		return
	}
}

// ImportState uses the username as the import ID.
func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	var data ResourceModel

	// Set the username from the import ID.
	data.Username = types.StringValue(req.ID)

	client := r.Meta.Client

	resp.Diagnostics.Append(readResourceInto(ctx, client, req.ID, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// readResourceInto populates data from the API.
func readResourceInto(
	ctx context.Context,
	client linodego.Client,
	username string,
	data *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	user, err := client.GetUser(ctx, username)
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to get user (%s)", username),
			err.Error(),
		)
		return diags
	}

	data.Username = types.StringValue(user.Username)
	data.Email = types.StringValue(user.Email)
	data.Restricted = types.BoolValue(user.Restricted)
	data.UserType = types.StringValue(string(user.UserType))
	data.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, d := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	data.SSHKeys = sshKeys

	if user.Restricted {
		grants, err := client.GetUserGrants(ctx, username)
		if err != nil {
			diags.AddError(
				fmt.Sprintf("Failed to get user grants (%s)", username),
				err.Error(),
			)
			return diags
		}

		diags.Append(parseGrantsIntoModel(ctx, grants, data)...)
	} else {
		// Not restricted: set null for grant fields.
		data.GlobalGrants = types.ListNull(resourceGrantsGlobalObjectType)
		data.DomainGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.FirewallGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.ImageGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.LinodeGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.LongviewGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.NodebalancerGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.StackscriptGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.VolumeGrant = types.SetNull(resourceGrantsEntityObjectType)
		data.VPCGrant = types.SetNull(resourceGrantsEntityObjectType)
	}

	return diags
}

// parseGrantsIntoModel maps linodego.UserGrants into the ResourceModel.
func parseGrantsIntoModel(
	ctx context.Context,
	grants *linodego.UserGrants,
	data *ResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	// Global grants (MaxItems:1 block → list with SizeAtMost(1))
	globalAttrs := map[string]attr.Value{
		"account_access":        types.StringValue(""),
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
	}

	globalObj, d := types.ObjectValue(resourceGrantsGlobalObjectType.AttrTypes, globalAttrs)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	globalList, d := types.ListValue(resourceGrantsGlobalObjectType, []attr.Value{globalObj})
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	data.GlobalGrants = globalList

	// Entity grants
	var d2 diag.Diagnostics

	data.DomainGrant, d2 = flattenResourceGrantEntities(ctx, grants.Domain)
	diags.Append(d2...)

	data.FirewallGrant, d2 = flattenResourceGrantEntities(ctx, grants.Firewall)
	diags.Append(d2...)

	data.ImageGrant, d2 = flattenResourceGrantEntities(ctx, grants.Image)
	diags.Append(d2...)

	data.LinodeGrant, d2 = flattenResourceGrantEntities(ctx, grants.Linode)
	diags.Append(d2...)

	data.LongviewGrant, d2 = flattenResourceGrantEntities(ctx, grants.Longview)
	diags.Append(d2...)

	data.NodebalancerGrant, d2 = flattenResourceGrantEntities(ctx, grants.NodeBalancer)
	diags.Append(d2...)

	data.StackscriptGrant, d2 = flattenResourceGrantEntities(ctx, grants.StackScript)
	diags.Append(d2...)

	data.VolumeGrant, d2 = flattenResourceGrantEntities(ctx, grants.Volume)
	diags.Append(d2...)

	data.VPCGrant, d2 = flattenResourceGrantEntities(ctx, grants.VPC)
	diags.Append(d2...)

	return diags
}

// flattenResourceGrantEntities converts a slice of GrantedEntity into a framework Set.
// Entities with empty permissions are filtered out to avoid false diffs.
func flattenResourceGrantEntities(
	ctx context.Context,
	entities []linodego.GrantedEntity,
) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	vals := make([]attr.Value, 0, len(entities))

	for _, entity := range entities {
		if entity.Permissions == "" {
			continue
		}

		obj, d := types.ObjectValue(resourceGrantsEntityObjectType.AttrTypes, map[string]attr.Value{
			"id":          types.Int64Value(int64(entity.ID)),
			"permissions": types.StringValue(string(entity.Permissions)),
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.SetNull(resourceGrantsEntityObjectType), diags
		}

		vals = append(vals, obj)
	}

	result, d := types.SetValue(resourceGrantsEntityObjectType, vals)
	diags.Append(d...)
	return result, diags
}

// hasGrantsConfigured returns true if any grant field is non-null and non-unknown.
func hasGrantsConfigured(data *ResourceModel) bool {
	if !data.GlobalGrants.IsNull() && !data.GlobalGrants.IsUnknown() && len(data.GlobalGrants.Elements()) > 0 {
		return true
	}
	grantSets := []types.Set{
		data.DomainGrant, data.FirewallGrant, data.ImageGrant, data.LinodeGrant,
		data.LongviewGrant, data.NodebalancerGrant, data.StackscriptGrant,
		data.VolumeGrant, data.VPCGrant,
	}
	for _, s := range grantSets {
		if !s.IsNull() && !s.IsUnknown() && len(s.Elements()) > 0 {
			return true
		}
	}
	return false
}

// updateUserGrantsFramework converts the model into a linodego update call.
func updateUserGrantsFramework(
	ctx context.Context,
	data *ResourceModel,
	client linodego.Client,
) diag.Diagnostics {
	var diags diag.Diagnostics

	username := data.Username.ValueString()

	// TODO: Implement this validation at plan-time
	if !data.Restricted.ValueBool() {
		diags.AddError(
			"User must be restricted",
			"user must be restricted in order to update grants",
		)
		return diags
	}

	updateOpts := linodego.UserGrantsUpdateOptions{}

	// Global grants
	if !data.GlobalGrants.IsNull() && !data.GlobalGrants.IsUnknown() {
		var globalList []struct {
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
			LongviewSubscription types.Bool   `tfsdk:"longview_subscription"`
		}
		diags.Append(data.GlobalGrants.ElementsAs(ctx, &globalList, false)...)
		if diags.HasError() {
			return diags
		}

		if len(globalList) > 0 {
			g := globalList[0]
			global := linodego.GlobalUserGrants{
				AddDomains:           g.AddDomains.ValueBool(),
				AddDatabases:         g.AddDatabases.ValueBool(),
				AddFirewalls:         g.AddFirewalls.ValueBool(),
				AddImages:            g.AddImages.ValueBool(),
				AddLinodes:           g.AddLinodes.ValueBool(),
				AddLongview:          g.AddLongview.ValueBool(),
				AddNodeBalancers:     g.AddNodeBalancers.ValueBool(),
				AddStackScripts:      g.AddStackScripts.ValueBool(),
				AddVolumes:           g.AddVolumes.ValueBool(),
				AddVPCs:              g.AddVPCs.ValueBool(),
				CancelAccount:        g.CancelAccount.ValueBool(),
				LongviewSubscription: g.LongviewSubscription.ValueBool(),
			}
			if v := g.AccountAccess.ValueString(); v != "" {
				level := linodego.GrantPermissionLevel(v)
				global.AccountAccess = &level
			}
			updateOpts.Global = global
		}
	}

	// Entity grants
	updateOpts.Domain = expandResourceGrantEntities(ctx, data.DomainGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.Firewall = expandResourceGrantEntities(ctx, data.FirewallGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.Image = expandResourceGrantEntities(ctx, data.ImageGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.Linode = expandResourceGrantEntities(ctx, data.LinodeGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.Longview = expandResourceGrantEntities(ctx, data.LongviewGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.NodeBalancer = expandResourceGrantEntities(ctx, data.NodebalancerGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.StackScript = expandResourceGrantEntities(ctx, data.StackscriptGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.Volume = expandResourceGrantEntities(ctx, data.VolumeGrant, &diags)
	if diags.HasError() {
		return diags
	}

	updateOpts.VPC = expandResourceGrantEntities(ctx, data.VPCGrant, &diags)
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

// expandResourceGrantEntities converts a framework Set to a slice of EntityUserGrant.
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

	var entities []entityModel
	diags.Append(set.ElementsAs(ctx, &entities, false)...)
	if diags.HasError() {
		return nil
	}

	result := make([]linodego.EntityUserGrant, len(entities))
	for i, e := range entities {
		perms := linodego.GrantPermissionLevel(e.Permissions.ValueString())
		result[i] = linodego.EntityUserGrant{
			ID:          int(e.ID.ValueInt64()),
			Permissions: &perms,
		}
	}

	return result
}
