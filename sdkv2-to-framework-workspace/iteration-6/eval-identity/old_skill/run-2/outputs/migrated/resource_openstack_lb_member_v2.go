package openstack

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	fwdiag "github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &lbMemberV2Resource{}
	_ resource.ResourceWithConfigure   = &lbMemberV2Resource{}
	_ resource.ResourceWithImportState = &lbMemberV2Resource{}
	_ resource.ResourceWithIdentity    = &lbMemberV2Resource{}
)

// NewLBMemberV2Resource returns a framework-based openstack_lb_member_v2 resource.
func NewLBMemberV2Resource() resource.Resource {
	return &lbMemberV2Resource{}
}

type lbMemberV2Resource struct {
	config *Config
}

// lbMemberV2Model is the typed state model for the resource.
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

// lbMemberV2IdentityModel is the typed identity model. The composite identity
// is (pool_id, member_id) — exactly the two pieces the legacy
// "<pool_id>/<member_id>" import string carried.
type lbMemberV2IdentityModel struct {
	PoolID   types.String `tfsdk:"pool_id"`
	MemberID types.String `tfsdk:"member_id"`
}

func (r *lbMemberV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_member_v2"
}

func (r *lbMemberV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
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
					stringplanmodifier.RequiresReplace(),
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
					stringplanmodifier.RequiresReplace(),
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
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Int64{
					int64validator.Between(0, 256),
				},
			},
			"subnet_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// IdentitySchema declares the typed identity (pool_id + member_id). This is
// the framework's modern alternative to parsing "pool_id/member_id" in
// ImportState.
func (r *lbMemberV2Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"pool_id": identityschema.StringAttribute{
				RequiredForImport: true,
			},
			"member_id": identityschema.StringAttribute{
				RequiredForImport: true,
			},
		},
	}
}

func (r *lbMemberV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lbMemberV2Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(plan.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

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

	// Set the weight only when defined in the configuration. Mirrors SDKv2's
	// `getOkExists` behaviour — distinguishes "user explicitly set" from null.
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		w := int(plan.Weight.ValueInt64())
		createOpts.Weight = &w
	}

	if !plan.MonitorAddress.IsNull() && !plan.MonitorAddress.IsUnknown() && plan.MonitorAddress.ValueString() != "" {
		createOpts.MonitorAddress = plan.MonitorAddress.ValueString()
	}

	if !plan.MonitorPort.IsNull() && !plan.MonitorPort.IsUnknown() && plan.MonitorPort.ValueInt64() != 0 {
		mp := int(plan.MonitorPort.ValueInt64())
		createOpts.MonitorPort = &mp
	}

	// Backup requires API version 2.1+, so only send it when the user
	// explicitly set the field (mirrors the SDKv2 `GetOk` gate).
	if !plan.Backup.IsNull() && !plan.Backup.IsUnknown() && plan.Backup.ValueBool() {
		b := plan.Backup.ValueBool()
		createOpts.Backup = &b
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tagList []string

		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tagList, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		createOpts.Tags = tagList
	}

	log.Printf("[DEBUG] Create Options: %#v", createOpts)

	poolID := plan.PoolID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("pool %s: %s", poolID, err),
		)

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to create member")

	var member *pools.Member

	err = retryOpenStackOnTransient(ctx, createTimeout, func() error {
		var rerr error

		member, rerr = pools.CreateMember(ctx, lbClient, poolID, createOpts).Extract()

		return rerr
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating member", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())

		return
	}

	plan.ID = types.StringValue(member.ID)
	plan.PoolID = types.StringValue(poolID)
	plan.Region = types.StringValue(region)

	r.applyMember(ctx, &plan, member, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Mirror the composite identity alongside state.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   plan.PoolID,
		MemberID: plan.ID,
	})...)
}

func (r *lbMemberV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lbMemberV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(state.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if isOpenStackResourceGone(err) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Error retrieving member", err.Error())

		return
	}

	log.Printf("[DEBUG] Retrieved member %s: %#v", memberID, member)

	state.Region = types.StringValue(region)

	r.applyMember(ctx, &state, member, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   state.PoolID,
		MemberID: state.ID,
	})...)
}

func (r *lbMemberV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state lbMemberV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(plan.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	var updateOpts pools.UpdateMemberOpts

	if !plan.Name.Equal(state.Name) {
		n := plan.Name.ValueString()
		updateOpts.Name = &n
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
		var tagList []string

		if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
			resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tagList, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}

		if tagList == nil {
			tagList = []string{}
		}

		updateOpts.Tags = tagList
	}

	poolID := plan.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("pool %s: %s", poolID, err),
		)

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve member",
			fmt.Sprintf("member %s: %s", memberID, err),
		)

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Updating member %s with options: %#v", memberID, updateOpts)

	err = retryOpenStackOnTransient(ctx, updateTimeout, func() error {
		_, rerr := pools.UpdateMember(ctx, lbClient, poolID, memberID, updateOpts).Extract()

		return rerr
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update member",
			fmt.Sprintf("member %s: %s", memberID, err),
		)

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE after update", err.Error())

		return
	}

	updated, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error refreshing member after update", err.Error())

		return
	}

	plan.ID = types.StringValue(memberID)
	plan.PoolID = types.StringValue(poolID)
	plan.Region = types.StringValue(region)

	r.applyMember(ctx, &plan, updated, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   plan.PoolID,
		MemberID: plan.ID,
	})...)
}

func (r *lbMemberV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lbMemberV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(state.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		if isOpenStackResourceGone(err) {
			return
		}

		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool for deletion",
			fmt.Sprintf("pool %s: %s", poolID, err),
		)

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if isOpenStackResourceGone(err) {
			return
		}

		resp.Diagnostics.AddError("Unable to retrieve member for deletion", err.Error())

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		if isOpenStackResourceGone(err) {
			return
		}

		resp.Diagnostics.AddError("Error waiting for the members pool status", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to delete member %s", memberID)

	err = retryOpenStackOnTransient(ctx, deleteTimeout, func() error {
		return pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()
	})
	if err != nil {
		if isOpenStackResourceGone(err) {
			return
		}

		resp.Diagnostics.AddError("Error deleting member", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "DELETED", getLbPendingDeleteStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become DELETED", err.Error())

		return
	}
}

// ImportState supports both the legacy composite-ID string ("pool_id/member_id")
// and the modern `import { identity = {...} }` form.
//
//   - Legacy:  `terraform import openstack_lb_member_v2.foo POOLID/MEMBERID`
//     `req.ID == "POOLID/MEMBERID"`, `req.Identity` is empty.
//   - Modern:  `import { to = openstack_lb_member_v2.foo, identity = { pool_id = ..., member_id = ... } }`
//     `req.ID == ""`, `req.Identity` is populated.
func (r *lbMemberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Modern path — identity-driven import (Terraform 1.12+).
	if req.ID == "" {
		var ident lbMemberV2IdentityModel

		resp.Diagnostics.Append(req.Identity.Get(ctx, &ident)...)
		if resp.Diagnostics.HasError() {
			return
		}

		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), ident.PoolID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ident.MemberID)...)

		return
	}

	// Legacy path — string-parse "pool_id/member_id".
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format <pool_id>/<member_id>, got %q", req.ID),
		)

		return
	}

	poolID := parts[0]
	memberID := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), poolID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memberID)...)

	// Mirror the parsed values into identity so subsequent operations can rely
	// on it as the source of truth.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   types.StringValue(poolID),
		MemberID: types.StringValue(memberID),
	})...)
}

// applyMember populates the model from a freshly-fetched pools.Member.
func (r *lbMemberV2Resource) applyMember(ctx context.Context, m *lbMemberV2Model, member *pools.Member, diags *fwdiag.Diagnostics) {
	m.Name = types.StringValue(member.Name)
	m.Weight = types.Int64Value(int64(member.Weight))
	m.AdminStateUp = types.BoolValue(member.AdminStateUp)
	m.TenantID = types.StringValue(member.ProjectID)
	m.SubnetID = stringOrNullEmpty(member.SubnetID)
	m.Address = types.StringValue(member.Address)
	m.ProtocolPort = types.Int64Value(int64(member.ProtocolPort))
	m.MonitorAddress = types.StringValue(member.MonitorAddress)
	m.MonitorPort = types.Int64Value(int64(member.MonitorPort))
	m.Backup = types.BoolValue(member.Backup)

	tags, tdiags := types.SetValueFrom(ctx, types.StringType, member.Tags)
	diags.Append(tdiags...)

	if !diags.HasError() {
		m.Tags = tags
	}
}

func (r *lbMemberV2Resource) regionFor(planRegion types.String) string {
	if !planRegion.IsNull() && !planRegion.IsUnknown() && planRegion.ValueString() != "" {
		return planRegion.ValueString()
	}

	if r.config != nil {
		return r.config.Region
	}

	return ""
}

// isOpenStackResourceGone is the framework-side equivalent of util.go's
// CheckDeleted: a 404 from the OpenStack API means the resource has been
// removed out-of-band and the caller should drop it from state instead of
// surfacing an error.
func isOpenStackResourceGone(err error) bool {
	return gophercloud.ResponseCodeIs(err, http.StatusNotFound)
}

// stringOrNullEmpty preserves SDKv2 semantics: if the API returns an empty
// string for an optional field that the user didn't set, keep state null
// rather than coercing to "".
func stringOrNullEmpty(s string) types.String {
	if s == "" {
		return types.StringNull()
	}

	return types.StringValue(s)
}

// retryOpenStackOnTransient is the framework-side replacement for SDKv2's
// `retry.RetryContext(ctx, timeout, fn)` paired with the package-level
// `checkForRetryableError`. It retries with a short backoff when the
// OpenStack API returns a transient HTTP status (409, 5xx) and bails
// immediately otherwise.
func retryOpenStackOnTransient(ctx context.Context, timeout time.Duration, op func() error) error {
	deadline := time.Now().Add(timeout)
	backoff := 500 * time.Millisecond

	for {
		err := op()
		if err == nil {
			return nil
		}

		if !isTransientOpenStackError(err) {
			return err
		}

		if time.Now().After(deadline) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		if backoff < 8*time.Second {
			backoff *= 2
		}
	}
}

func isTransientOpenStackError(err error) bool {
	var e gophercloud.ErrUnexpectedResponseCode
	if !errors.As(err, &e) {
		return false
	}

	switch e.Actual {
	case http.StatusConflict, // 409
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	}

	return false
}
