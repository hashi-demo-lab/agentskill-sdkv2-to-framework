package openstack

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var (
	_ resource.Resource                = &lbMemberV2Resource{}
	_ resource.ResourceWithConfigure   = &lbMemberV2Resource{}
	_ resource.ResourceWithImportState = &lbMemberV2Resource{}
	_ resource.ResourceWithIdentity    = &lbMemberV2Resource{}
)

// NewLBMemberV2Resource returns a new lb_member_v2 resource.
func NewLBMemberV2Resource() resource.Resource {
	return &lbMemberV2Resource{}
}

type lbMemberV2Resource struct {
	config *Config
}

// Resource model.
type lbMemberV2Model struct {
	ID             types.String   `tfsdk:"id"`
	Region         types.String   `tfsdk:"region"`
	Name           types.String   `tfsdk:"name"`
	TenantID       types.String   `tfsdk:"tenant_id"`
	Address        types.String   `tfsdk:"address"`
	ProtocolPort   types.Int64    `tfsdk:"protocol_port"`
	Weight         types.Int64    `tfsdk:"weight"`
	SubnetID       types.String   `tfsdk:"subnet_id"`
	AdminStateUp   types.Bool     `tfsdk:"admin_state_up"`
	PoolID         types.String   `tfsdk:"pool_id"`
	Backup         types.Bool     `tfsdk:"backup"`
	MonitorAddress types.String   `tfsdk:"monitor_address"`
	MonitorPort    types.Int64    `tfsdk:"monitor_port"`
	Tags           types.Set      `tfsdk:"tags"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

// Identity model — practitioner-facing addressing fields used by Terraform 1.12+
// `import { identity = { ... } }` blocks.
type lbMemberV2IdentityModel struct {
	Region types.String `tfsdk:"region"`
	PoolID types.String `tfsdk:"pool_id"`
	ID     types.String `tfsdk:"id"`
}

func (r *lbMemberV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_member_v2"
}

func (r *lbMemberV2Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"region": identityschema.StringAttribute{
				RequiredForImport: true,
			},
			"pool_id": identityschema.StringAttribute{
				RequiredForImport: true,
			},
			"id": identityschema.StringAttribute{
				RequiredForImport: true,
			},
		},
	}
}

func (r *lbMemberV2Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"tenant_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"address": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"protocol_port": schema.Int64Attribute{
				Required: true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},
			"weight": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Validators: []validator.Int64{
					int64validator.Between(0, 256),
				},
			},
			"subnet_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
				},
			},
			"admin_state_up": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"pool_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"backup": schema.BoolAttribute{
				Optional: true,
				Computed: true,
			},
			"monitor_address": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"monitor_port": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},
			"tags": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *lbMemberV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *lbMemberV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lbMemberV2Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	region := resolveRegion(plan.Region, r.config)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	adminStateUp := plan.AdminStateUp.ValueBool()

	createOpts := pools.CreateMemberOpts{
		Name:         plan.Name.ValueString(),
		ProjectID:    plan.TenantID.ValueString(),
		Address:      plan.Address.ValueString(),
		ProtocolPort: int(plan.ProtocolPort.ValueInt64()),
		AdminStateUp: &adminStateUp,
	}

	if !plan.SubnetID.IsNull() && !plan.SubnetID.IsUnknown() && plan.SubnetID.ValueString() != "" {
		createOpts.SubnetID = plan.SubnetID.ValueString()
	}

	// Set weight only if defined; the framework distinguishes null (unset) from
	// known-zero. This preserves the SDKv2 GetOkExists semantics that the
	// previous implementation relied on (don't send Weight=0 by default).
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		w := int(plan.Weight.ValueInt64())
		createOpts.Weight = &w
	}

	if !plan.MonitorAddress.IsNull() && !plan.MonitorAddress.IsUnknown() && plan.MonitorAddress.ValueString() != "" {
		createOpts.MonitorAddress = plan.MonitorAddress.ValueString()
	}

	if !plan.MonitorPort.IsNull() && !plan.MonitorPort.IsUnknown() && plan.MonitorPort.ValueInt64() > 0 {
		mp := int(plan.MonitorPort.ValueInt64())
		createOpts.MonitorPort = &mp
	}

	if !plan.Backup.IsNull() && !plan.Backup.IsUnknown() {
		b := plan.Backup.ValueBool()
		createOpts.Backup = &b
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string

		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)

		if resp.Diagnostics.HasError() {
			return
		}

		createOpts.Tags = tags
	}

	tflog.Debug(ctx, fmt.Sprintf("[DEBUG] Create Options: %#v", createOpts))

	poolID := plan.PoolID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve parent pool",
			fmt.Sprintf("pool %s: %s", poolID, err))

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become active", err.Error())

		return
	}

	tflog.Debug(ctx, "[DEBUG] Attempting to create member")

	var member *pools.Member

	err = retry.RetryContext(ctx, createTimeout, func() *retry.RetryError {
		var createErr error

		member, createErr = pools.CreateMember(ctx, lbClient, poolID, createOpts).Extract()
		if createErr != nil {
			return checkForRetryableError(createErr)
		}

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating member", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become active", err.Error())

		return
	}

	plan.ID = types.StringValue(member.ID)
	plan.Region = types.StringValue(region)

	r.populateModelFromMember(ctx, &plan, member, region, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(setLBMemberV2Identity(ctx, resp.Identity, plan)...)
}

func (r *lbMemberV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lbMemberV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := resolveRegion(state.Region, r.config)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()

	member, err := pools.GetMember(ctx, lbClient, poolID, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Error reading member", err.Error())

		return
	}

	tflog.Debug(ctx, fmt.Sprintf("[DEBUG] Retrieved member %s: %#v", state.ID.ValueString(), member))

	r.populateModelFromMember(ctx, &state, member, region, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(setLBMemberV2Identity(ctx, resp.Identity, state)...)
}

func (r *lbMemberV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state lbMemberV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	region := resolveRegion(state.Region, r.config)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	var updateOpts pools.UpdateMemberOpts

	if !plan.Name.Equal(state.Name) {
		name := plan.Name.ValueString()
		updateOpts.Name = &name
	}

	if !plan.Weight.Equal(state.Weight) {
		w := int(plan.Weight.ValueInt64())
		updateOpts.Weight = &w
	}

	if !plan.AdminStateUp.Equal(state.AdminStateUp) {
		asu := plan.AdminStateUp.ValueBool()
		updateOpts.AdminStateUp = &asu
	}

	if !plan.MonitorAddress.Equal(state.MonitorAddress) {
		ma := plan.MonitorAddress.ValueString()
		updateOpts.MonitorAddress = &ma
	}

	if !plan.MonitorPort.Equal(state.MonitorPort) {
		mp := int(plan.MonitorPort.ValueInt64())
		updateOpts.MonitorPort = &mp
	}

	if !plan.Backup.Equal(state.Backup) {
		b := plan.Backup.ValueBool()
		updateOpts.Backup = &b
	}

	if !plan.Tags.Equal(state.Tags) {
		var tags []string

		if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
			resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)

			if resp.Diagnostics.HasError() {
				return
			}
		}

		if tags == nil {
			tags = []string{}
		}

		updateOpts.Tags = tags
	}

	poolID := plan.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve parent pool",
			fmt.Sprintf("pool %s: %s", poolID, err))

		return
	}

	currentMember, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve member",
			fmt.Sprintf("%s: %s", memberID, err))

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become active", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, currentMember, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become active", err.Error())

		return
	}

	tflog.Debug(ctx, fmt.Sprintf("[DEBUG] Updating member %s with options: %#v", memberID, updateOpts))

	err = retry.RetryContext(ctx, updateTimeout, func() *retry.RetryError {
		_, updErr := pools.UpdateMember(ctx, lbClient, poolID, memberID, updateOpts).Extract()
		if updErr != nil {
			return checkForRetryableError(updErr)
		}

		return nil
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update member",
			fmt.Sprintf("%s: %s", memberID, err))

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, currentMember, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become active after update", err.Error())

		return
	}

	// Re-read to populate computed fields and capture any server-side rewrites.
	updated, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error reading updated member", err.Error())

		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Region = types.StringValue(region)

	r.populateModelFromMember(ctx, &plan, updated, region, &resp.Diagnostics)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(setLBMemberV2Identity(ctx, resp.Identity, plan)...)
}

func (r *lbMemberV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lbMemberV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	region := resolveRegion(state.Region, r.config)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Unable to retrieve parent pool",
			fmt.Sprintf("pool %s: %s", poolID, err))

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Unable to retrieve member",
			fmt.Sprintf("%s: %s", memberID, err))

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Error waiting for the members pool status", err.Error())

		return
	}

	tflog.Debug(ctx, fmt.Sprintf("[DEBUG] Attempting to delete member %s", memberID))

	err = retry.RetryContext(ctx, deleteTimeout, func() *retry.RetryError {
		delErr := pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()
		if delErr != nil {
			return checkForRetryableError(delErr)
		}

		return nil
	})
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Error deleting member", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "DELETED", getLbPendingDeleteStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be deleted", err.Error())

		return
	}
}

// ImportState supports two paths:
//
//  1. Modern: practitioner uses an `import { identity = { region, pool_id, id } }`
//     block (Terraform 1.12+). req.ID is empty and req.Identity is populated;
//     the framework's helper mirrors each identity attribute to state.
//  2. Legacy: `terraform import openstack_lb_member_v2.foo <pool_id>/<member_id>`.
//     req.ID is the slash-delimited string; we parse it into pool_id+id the
//     same way the SDKv2 importer did. region is left to be picked up from
//     the provider default during the subsequent Read.
func (r *lbMemberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("region"), path.Root("region"), req, resp)
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("id"), req, resp)

		return
	}

	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Format must be <pool id>/<member id>, got %q", req.ID),
		)

		return
	}

	poolID := parts[0]
	memberID := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), poolID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memberID)...)
}

// populateModelFromMember writes the API response into the model. It does not
// touch identity (set by callers via setLBMemberV2Identity), pool_id (caller-
// supplied, never returned by the API in the same shape), or timeouts.
func (r *lbMemberV2Resource) populateModelFromMember(ctx context.Context, m *lbMemberV2Model, member *pools.Member, region string, diagSink *diag.Diagnostics) {
	m.Name = types.StringValue(member.Name)
	m.Weight = types.Int64Value(int64(member.Weight))
	m.AdminStateUp = types.BoolValue(member.AdminStateUp)
	m.TenantID = types.StringValue(member.ProjectID)
	m.SubnetID = types.StringValue(member.SubnetID)
	m.Address = types.StringValue(member.Address)
	m.ProtocolPort = types.Int64Value(int64(member.ProtocolPort))
	m.Region = types.StringValue(region)
	m.MonitorAddress = types.StringValue(member.MonitorAddress)
	m.MonitorPort = types.Int64Value(int64(member.MonitorPort))
	m.Backup = types.BoolValue(member.Backup)

	if member.Tags == nil {
		m.Tags = types.SetNull(types.StringType)

		return
	}

	tagsValue, d := types.SetValueFrom(ctx, types.StringType, member.Tags)
	diagSink.Append(d...)
	m.Tags = tagsValue
}

// setLBMemberV2Identity writes the identity payload from the resolved model.
// Returns diagnostics from the underlying Identity.Set call (or nil if
// identity is unset on the request — e.g., older Terraform CLIs that don't
// know about resource identity).
func setLBMemberV2Identity(ctx context.Context, identity *tfsdk.ResourceIdentity, m lbMemberV2Model) diag.Diagnostics {
	if identity == nil {
		return nil
	}

	id := lbMemberV2IdentityModel{
		Region: m.Region,
		PoolID: m.PoolID,
		ID:     m.ID,
	}

	return identity.Set(ctx, id)
}

// resolveRegion mirrors the SDKv2 GetRegion helper: prefer the resource's
// "region" attribute when set, otherwise fall back to the provider's default
// region.
func resolveRegion(attr types.String, config *Config) string {
	if !attr.IsNull() && !attr.IsUnknown() && attr.ValueString() != "" {
		return attr.ValueString()
	}

	if config != nil {
		return config.Region
	}

	return ""
}
