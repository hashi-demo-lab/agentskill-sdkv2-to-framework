package user

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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

// ResourceModel is the Terraform state model for linode_user.
type ResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Email             types.String `tfsdk:"email"`
	Username          types.String `tfsdk:"username"`
	Restricted        types.Bool   `tfsdk:"restricted"`
	UserType          types.String `tfsdk:"user_type"`
	SSHKeys           types.List   `tfsdk:"ssh_keys"`
	TFAEnabled        types.Bool   `tfsdk:"tfa_enabled"`
	GlobalGrants      types.Object `tfsdk:"global_grants"`
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

// globalGrantsModel is used to decode global_grants from types.Object.
type globalGrantsModel struct {
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

// entityGrantModel is used to decode entity grant set elements.
type entityGrantModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Permissions types.String `tfsdk:"permissions"`
}

// resourceGrantEntityObjectType defines the element type for entity grant sets in state.
var resourceGrantEntityObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"id":          types.Int64Type,
		"permissions": types.StringType,
	},
}

// resourceGrantsGlobalObjectType defines the type of the global_grants block in state.
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

var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed: true,
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
		"domain_grant": schema.SetNestedAttribute{
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
		},
		"firewall_grant": schema.SetNestedAttribute{
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
		},
		"image_grant": schema.SetNestedAttribute{
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
		},
		"linode_grant": schema.SetNestedAttribute{
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
		},
		"longview_grant": schema.SetNestedAttribute{
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
		},
		"nodebalancer_grant": schema.SetNestedAttribute{
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
		},
		"stackscript_grant": schema.SetNestedAttribute{
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
		},
		"volume_grant": schema.SetNestedAttribute{
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
		},
		"vpc_grant": schema.SetNestedAttribute{
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
		},
	},
	Blocks: map[string]schema.Block{
		// global_grants uses SingleNestedBlock to preserve the block HCL syntax
		// (`global_grants { ... }`) that existing practitioners use.
		"global_grants": schema.SingleNestedBlock{
			Description: "A structure containing the Account-level grants a User has.",
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
}

func NewResource() resource.Resource {
	return &Resource{
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

	plan.ID = types.StringValue(user.Username)
	ctx = tflog.SetField(ctx, "username", user.Username)

	if resourceModelHasGrantsConfigured(plan) {
		if err := resourceUpdateUserGrants(ctx, plan, client); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to set user grants (%s)", user.Username),
				err.Error(),
			)
			return
		}
	}

	if err := resourceReadIntoModel(ctx, client, &plan); err != nil {
		if linodego.IsNotFound(err) {
			resp.Diagnostics.AddWarning(
				"User not found after create",
				fmt.Sprintf("Removing linode_user %q from state: %s", plan.ID.ValueString(), err.Error()),
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read user after create", err.Error())
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

	var data ResourceModel
	client := r.Meta.Client

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if helper.FrameworkAttemptRemoveResourceForEmptyID(ctx, data.ID, resp) {
		return
	}

	ctx = tflog.SetField(ctx, "username", data.ID.ValueString())

	if err := resourceReadIntoModel(ctx, client, &data); err != nil {
		if linodego.IsNotFound(err) {
			resp.Diagnostics.AddWarning(
				"User not found",
				fmt.Sprintf("Removing linode_user %q from state: %s", data.ID.ValueString(), err.Error()),
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read user", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_user")

	var plan, state ResourceModel
	client := r.Meta.Client

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
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
			fmt.Sprintf("Failed to update user (%s)", id),
			err.Error(),
		)
		return
	}

	// After a username change, the resource ID changes too.
	plan.ID = types.StringValue(username)

	if resourceGrantsChanged(plan, state) {
		if err := resourceUpdateUserGrants(ctx, plan, client); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to update user grants (%s)", username),
				err.Error(),
			)
			return
		}
	}

	if err := resourceReadIntoModel(ctx, client, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to read user after update", err.Error())
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

	var data ResourceModel
	client := r.Meta.Client

	// Delete reads from State, not Plan.
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	username := data.ID.ValueString()
	ctx = tflog.SetField(ctx, "username", username)

	tflog.Debug(ctx, "client.DeleteUser(...)")
	if err := client.DeleteUser(ctx, username); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to delete user (%s)", username),
			err.Error(),
		)
	}
}

// resourceReadIntoModel fetches the user (and grants if restricted) and populates data.
func resourceReadIntoModel(
	ctx context.Context,
	client linodego.Client,
	data *ResourceModel,
) error {
	username := data.ID.ValueString()

	user, err := client.GetUser(ctx, username)
	if err != nil {
		return err
	}

	data.Username = types.StringValue(user.Username)
	data.Email = types.StringValue(user.Email)
	data.Restricted = types.BoolValue(user.Restricted)
	data.UserType = types.StringValue(string(user.UserType))
	data.TFAEnabled = types.BoolValue(user.TFAEnabled)

	sshKeys, _ := types.ListValueFrom(ctx, types.StringType, user.SSHKeys)
	data.SSHKeys = sshKeys

	if user.Restricted {
		grants, err := client.GetUserGrants(ctx, username)
		if err != nil {
			return fmt.Errorf("failed to get user grants (%s): %w", username, err)
		}

		globalObj, err := resourceFlattenGlobalGrants(grants.Global)
		if err != nil {
			return err
		}
		data.GlobalGrants = globalObj

		data.DomainGrant = resourceFlattenEntityGrants(grants.Domain)
		data.FirewallGrant = resourceFlattenEntityGrants(grants.Firewall)
		data.ImageGrant = resourceFlattenEntityGrants(grants.Image)
		data.LinodeGrant = resourceFlattenEntityGrants(grants.Linode)
		data.LongviewGrant = resourceFlattenEntityGrants(grants.Longview)
		data.NodebalancerGrant = resourceFlattenEntityGrants(grants.NodeBalancer)
		data.StackscriptGrant = resourceFlattenEntityGrants(grants.StackScript)
		data.VolumeGrant = resourceFlattenEntityGrants(grants.Volume)
		data.VPCGrant = resourceFlattenEntityGrants(grants.VPC)
	}

	return nil
}

func resourceFlattenGlobalGrants(global linodego.GlobalUserGrants) (types.Object, error) {
	accountAccess := ""
	if global.AccountAccess != nil {
		accountAccess = string(*global.AccountAccess)
	}

	attrValues := map[string]attr.Value{
		"account_access":        types.StringValue(accountAccess),
		"add_domains":           types.BoolValue(global.AddDomains),
		"add_databases":         types.BoolValue(global.AddDatabases),
		"add_firewalls":         types.BoolValue(global.AddFirewalls),
		"add_images":            types.BoolValue(global.AddImages),
		"add_linodes":           types.BoolValue(global.AddLinodes),
		"add_longview":          types.BoolValue(global.AddLongview),
		"add_nodebalancers":     types.BoolValue(global.AddNodeBalancers),
		"add_stackscripts":      types.BoolValue(global.AddStackScripts),
		"add_volumes":           types.BoolValue(global.AddVolumes),
		"add_vpcs":              types.BoolValue(global.AddVPCs),
		"cancel_account":        types.BoolValue(global.CancelAccount),
		"longview_subscription": types.BoolValue(global.LongviewSubscription),
	}

	obj, diags := types.ObjectValue(resourceGrantsGlobalObjectType.AttrTypes, attrValues)
	if diags.HasError() {
		return types.ObjectNull(resourceGrantsGlobalObjectType.AttrTypes),
			fmt.Errorf("failed to build global_grants object: %s", diags[0].Detail())
	}
	return obj, nil
}

func resourceFlattenEntityGrants(entities []linodego.GrantedEntity) types.Set {
	elems := make([]attr.Value, 0, len(entities))

	for _, e := range entities {
		// Filter out entities without permissions — Linode creates empty placeholders
		// that would produce false diffs.
		if e.Permissions == "" {
			continue
		}
		obj, diags := types.ObjectValue(resourceGrantEntityObjectType.AttrTypes, map[string]attr.Value{
			"id":          types.Int64Value(int64(e.ID)),
			"permissions": types.StringValue(string(e.Permissions)),
		})
		if diags.HasError() {
			continue
		}
		elems = append(elems, obj)
	}

	result, diags := types.SetValue(resourceGrantEntityObjectType, elems)
	if diags.HasError() {
		return types.SetNull(resourceGrantEntityObjectType)
	}
	return result
}

func resourceExpandEntityGrants(ctx context.Context, s types.Set) []linodego.EntityUserGrant {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}

	var items []entityGrantModel
	s.ElementsAs(ctx, &items, false)

	result := make([]linodego.EntityUserGrant, len(items))
	for i, item := range items {
		perm := linodego.GrantPermissionLevel(item.Permissions.ValueString())
		result[i] = linodego.EntityUserGrant{
			ID:          int(item.ID.ValueInt64()),
			Permissions: &perm,
		}
	}
	return result
}

func resourceExpandGlobalGrants(ctx context.Context, obj types.Object) linodego.GlobalUserGrants {
	if obj.IsNull() || obj.IsUnknown() {
		return linodego.GlobalUserGrants{}
	}

	var m globalGrantsModel
	obj.As(ctx, &m, types.ObjectAsOptions{})

	result := linodego.GlobalUserGrants{
		AddDomains:           m.AddDomains.ValueBool(),
		AddDatabases:         m.AddDatabases.ValueBool(),
		AddFirewalls:         m.AddFirewalls.ValueBool(),
		AddImages:            m.AddImages.ValueBool(),
		AddLinodes:           m.AddLinodes.ValueBool(),
		AddLongview:          m.AddLongview.ValueBool(),
		AddNodeBalancers:     m.AddNodeBalancers.ValueBool(),
		AddStackScripts:      m.AddStackScripts.ValueBool(),
		AddVolumes:           m.AddVolumes.ValueBool(),
		AddVPCs:              m.AddVPCs.ValueBool(),
		CancelAccount:        m.CancelAccount.ValueBool(),
		LongviewSubscription: m.LongviewSubscription.ValueBool(),
	}

	if !m.AccountAccess.IsNull() && !m.AccountAccess.IsUnknown() && m.AccountAccess.ValueString() != "" {
		aa := linodego.GrantPermissionLevel(m.AccountAccess.ValueString())
		result.AccountAccess = &aa
	}

	return result
}

func resourceUpdateUserGrants(
	ctx context.Context,
	plan ResourceModel,
	client linodego.Client,
) error {
	username := plan.ID.ValueString()

	// TODO: Implement this validation at plan-time.
	if !plan.Restricted.ValueBool() {
		return fmt.Errorf("user must be restricted in order to update grants")
	}

	updateOpts := linodego.UserGrantsUpdateOptions{
		Global:       resourceExpandGlobalGrants(ctx, plan.GlobalGrants),
		Domain:       resourceExpandEntityGrants(ctx, plan.DomainGrant),
		Firewall:     resourceExpandEntityGrants(ctx, plan.FirewallGrant),
		Image:        resourceExpandEntityGrants(ctx, plan.ImageGrant),
		Linode:       resourceExpandEntityGrants(ctx, plan.LinodeGrant),
		Longview:     resourceExpandEntityGrants(ctx, plan.LongviewGrant),
		NodeBalancer: resourceExpandEntityGrants(ctx, plan.NodebalancerGrant),
		StackScript:  resourceExpandEntityGrants(ctx, plan.StackscriptGrant),
		VPC:          resourceExpandEntityGrants(ctx, plan.VPCGrant),
		Volume:       resourceExpandEntityGrants(ctx, plan.VolumeGrant),
	}

	tflog.Debug(ctx, "client.UpdateUserGrants(...)", map[string]any{
		"options": updateOpts,
	})

	if _, err := client.UpdateUserGrants(ctx, username, updateOpts); err != nil {
		return err
	}
	return nil
}

func resourceModelHasGrantsConfigured(plan ResourceModel) bool {
	if !plan.GlobalGrants.IsNull() && !plan.GlobalGrants.IsUnknown() {
		return true
	}
	for _, s := range []types.Set{
		plan.DomainGrant, plan.FirewallGrant, plan.ImageGrant, plan.LinodeGrant,
		plan.LongviewGrant, plan.NodebalancerGrant, plan.StackscriptGrant,
		plan.VolumeGrant, plan.VPCGrant,
	} {
		if !s.IsNull() && !s.IsUnknown() {
			return true
		}
	}
	return false
}

func resourceGrantsChanged(plan, state ResourceModel) bool {
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
