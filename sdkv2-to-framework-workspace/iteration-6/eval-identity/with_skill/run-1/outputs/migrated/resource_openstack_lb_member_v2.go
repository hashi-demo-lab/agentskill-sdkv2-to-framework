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
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface checks.
var (
	_ resource.Resource                = &memberV2Resource{}
	_ resource.ResourceWithConfigure   = &memberV2Resource{}
	_ resource.ResourceWithImportState = &memberV2Resource{}
	_ resource.ResourceWithIdentity    = &memberV2Resource{}
)

// NewMemberV2Resource returns a framework resource for openstack_lb_member_v2.
func NewMemberV2Resource() resource.Resource {
	return &memberV2Resource{}
}

type memberV2Resource struct {
	config *Config
}

// memberV2Model is the typed state/plan/config model for openstack_lb_member_v2.
type memberV2Model struct {
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

// memberV2IdentityModel is the typed identity model. The composite key
// (pool_id, member_id) is what addresses this resource — exactly the same
// pieces the legacy "<pool>/<member>" import string carried.
type memberV2IdentityModel struct {
	PoolID types.String `tfsdk:"pool_id"`
	ID     types.String `tfsdk:"id"`
}

func (r *memberV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_member_v2"
}

func (r *memberV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *memberV2Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			// Preserve historical block-form `timeouts { ... }` syntax.
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// IdentitySchema declares the practitioner-addressable identity for this
// resource. openstack_lb_member_v2 is a child of a pool, so the identity is
// the (pool_id, id) tuple.
func (r *memberV2Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"pool_id": identityschema.StringAttribute{
				RequiredForImport: true,
			},
			"id": identityschema.StringAttribute{
				RequiredForImport: true,
			},
		},
	}
}

func (r *memberV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan memberV2Model
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

	region := r.regionForPlan(plan)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	adminStateUp := true
	if !plan.AdminStateUp.IsNull() && !plan.AdminStateUp.IsUnknown() {
		adminStateUp = plan.AdminStateUp.ValueBool()
	}

	createOpts := pools.CreateMemberOpts{
		Name:         plan.Name.ValueString(),
		ProjectID:    plan.TenantID.ValueString(),
		Address:      plan.Address.ValueString(),
		ProtocolPort: int(plan.ProtocolPort.ValueInt64()),
		AdminStateUp: &adminStateUp,
	}

	if !plan.SubnetID.IsNull() && plan.SubnetID.ValueString() != "" {
		createOpts.SubnetID = plan.SubnetID.ValueString()
	}

	// Set weight only if it's defined in the configuration. This preserves the
	// SDKv2 `getOkExists` semantics — a config-declared zero is meaningful, but
	// an absent attribute should leave the API default in place.
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		w := int(plan.Weight.ValueInt64())
		createOpts.Weight = &w
	}

	if !plan.MonitorAddress.IsNull() && plan.MonitorAddress.ValueString() != "" {
		createOpts.MonitorAddress = plan.MonitorAddress.ValueString()
	}

	if !plan.MonitorPort.IsNull() && !plan.MonitorPort.IsUnknown() && plan.MonitorPort.ValueInt64() != 0 {
		mp := int(plan.MonitorPort.ValueInt64())
		createOpts.MonitorPort = &mp
	}

	// Backup requires Octavia API version 2.1+; only set it if user-defined.
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

	log.Printf("[DEBUG] openstack_lb_member_v2 create options: %#v", createOpts)

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
		resp.Diagnostics.AddError("Error waiting for pool to become active", err.Error())

		return
	}

	log.Printf("[DEBUG] openstack_lb_member_v2: attempting to create member")

	member, err := createMemberWithRetry(ctx, lbClient, poolID, createOpts, createTimeout)
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

	// Read back the full resource so computed fields are populated. This
	// mirrors the SDKv2 `return resourceMemberV2Read(...)` tail-call.
	notFound, refreshErr := r.refreshState(ctx, lbClient, region, &plan, &resp.Diagnostics)
	if refreshErr || resp.Diagnostics.HasError() {
		return
	}

	if notFound {
		resp.Diagnostics.AddError(
			"Member disappeared after create",
			fmt.Sprintf("member %s was just created but cannot be retrieved", member.ID),
		)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Persist identity alongside state so practitioners on Terraform 1.12+
	// can reference it via `import { identity = {...} }`.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, memberV2IdentityModel{
		PoolID: plan.PoolID,
		ID:     plan.ID,
	})...)
}

func (r *memberV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionForPlan(state)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	notFound, refreshErr := r.refreshState(ctx, lbClient, region, &state, &resp.Diagnostics)
	if refreshErr || resp.Diagnostics.HasError() {
		return
	}

	if notFound {
		resp.State.RemoveResource(ctx)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Keep identity in sync on every Read so it's populated even after import.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, memberV2IdentityModel{
		PoolID: state.PoolID,
		ID:     state.ID,
	})...)
}

func (r *memberV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state memberV2Model
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

	region := r.regionForPlan(plan)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	var updateOpts pools.UpdateMemberOpts

	if !plan.Name.Equal(state.Name) {
		v := plan.Name.ValueString()
		updateOpts.Name = &v
	}

	if !plan.Weight.Equal(state.Weight) {
		v := int(plan.Weight.ValueInt64())
		updateOpts.Weight = &v
	}

	if !plan.AdminStateUp.Equal(state.AdminStateUp) {
		v := plan.AdminStateUp.ValueBool()
		updateOpts.AdminStateUp = &v
	}

	if !plan.MonitorAddress.Equal(state.MonitorAddress) {
		v := plan.MonitorAddress.ValueString()
		updateOpts.MonitorAddress = &v
	}

	if !plan.MonitorPort.Equal(state.MonitorPort) {
		v := int(plan.MonitorPort.ValueInt64())
		updateOpts.MonitorPort = &v
	}

	if !plan.Backup.Equal(state.Backup) {
		v := plan.Backup.ValueBool()
		updateOpts.Backup = &v
	}

	if !plan.Tags.Equal(state.Tags) {
		if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
			var tags []string
			resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
			if resp.Diagnostics.HasError() {
				return
			}

			updateOpts.Tags = tags
		} else {
			updateOpts.Tags = []string{}
		}
	}

	poolID := plan.PoolID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("pool %s: %s", poolID, err),
		)

		return
	}

	memberID := state.ID.ValueString()

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve member",
			fmt.Sprintf("member %s: %s", memberID, err),
		)

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for pool to become active", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become active", err.Error())

		return
	}

	log.Printf("[DEBUG] openstack_lb_member_v2: updating member %s with options: %#v", memberID, updateOpts)

	if err := updateMemberWithRetry(ctx, lbClient, poolID, memberID, updateOpts, updateTimeout); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update member",
			fmt.Sprintf("member %s: %s", memberID, err),
		)

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become active", err.Error())

		return
	}

	// Carry the prior ID into the plan model before refreshing, since plan.ID
	// will be unknown for an Update.
	plan.ID = state.ID

	notFound, refreshErr := r.refreshState(ctx, lbClient, region, &plan, &resp.Diagnostics)
	if refreshErr || resp.Diagnostics.HasError() {
		return
	}

	if notFound {
		resp.Diagnostics.AddError(
			"Member disappeared during update",
			fmt.Sprintf("member %s no longer exists", memberID),
		)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Identity.Set(ctx, memberV2IdentityModel{
		PoolID: plan.PoolID,
		ID:     plan.ID,
	})...)
}

func (r *memberV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state memberV2Model
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

	region := r.regionForPlan(state)

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
			// Parent pool already gone; member must be too.
			return
		}

		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("pool %s: %s", poolID, err),
		)

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			"Unable to retrieve member",
			fmt.Sprintf("member %s: %s", memberID, err),
		)

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Error waiting for pool status before delete", err.Error())

		return
	}

	log.Printf("[DEBUG] openstack_lb_member_v2: attempting to delete member %s", memberID)

	if err := deleteMemberWithRetry(ctx, lbClient, poolID, memberID, deleteTimeout); err != nil {
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

// ImportState handles BOTH supported import paths:
//
//  1. Modern (Terraform 1.12+):
//
//     import {
//       to = openstack_lb_member_v2.foo
//       identity = { pool_id = "...", id = "..." }
//     }
//
//     `req.ID` is empty and `req.Identity` is populated. We dispatch to
//     `ImportStatePassthroughWithIdentity` once per identity attribute
//     (the helper handles a single attribute pair per call).
//
//  2. Legacy CLI: `terraform import openstack_lb_member_v2.foo <pool>/<member>`.
//     `req.ID` is the slash-delimited string and `req.Identity` is empty. We
//     parse it the same way the SDKv2 importer did and then mirror the parsed
//     values into identity so subsequent plans/refreshes have it populated.
//
// The two branches are mutually exclusive: branching on `req.ID == ""` is the
// canonical dispatch (see references/identity.md).
func (r *memberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		// Modern identity-driven import. Two attributes => two helper calls.
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("id"), req, resp)

		return
	}

	// Legacy "<pool_id>/<member_id>" import string.
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Invalid format specified for Member. Format must be <pool id>/<member id>, got %q", req.ID),
		)

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Identity.Set(ctx, memberV2IdentityModel{
		PoolID: types.StringValue(parts[0]),
		ID:     types.StringValue(parts[1]),
	})...)
}

// regionForPlan returns the region from the model (if set) or falls back to
// the provider-level region. Replaces SDKv2's GetRegion(d, config).
func (r *memberV2Resource) regionForPlan(m memberV2Model) string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}

	return r.config.Region
}

// refreshState fetches the member from the API and writes its fields back
// into the supplied model. Returns (notFound, hardError):
//   - notFound=true: the API returned 404 — caller should remove from state.
//   - hardError=true: a hard error was added to diags; caller should bail.
//
// Replaces the SDKv2 `Read` body + `CheckDeleted` helper.
func (r *memberV2Resource) refreshState(
	ctx context.Context,
	lbClient *gophercloud.ServiceClient,
	region string,
	m *memberV2Model,
	diags *diag.Diagnostics,
) (notFound bool, hardError bool) {
	poolID := m.PoolID.ValueString()
	memberID := m.ID.ValueString()

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return true, false
		}

		diags.AddError(
			"Error reading member",
			fmt.Sprintf("member %s: %s", memberID, err),
		)

		return false, true
	}

	log.Printf("[DEBUG] openstack_lb_member_v2: retrieved member %s: %#v", memberID, member)

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

	tags, tdiags := types.SetValueFrom(ctx, types.StringType, member.Tags)
	diags.Append(tdiags...)

	if diags.HasError() {
		return false, true
	}

	m.Tags = tags

	return false, false
}

// createMemberWithRetry replaces the SDKv2 helper/retry.RetryContext block
// around pools.CreateMember. The framework has no retry helper, so we ticker
// poll using the same retryable-error classification SDKv2 used.
func createMemberWithRetry(
	ctx context.Context,
	lbClient *gophercloud.ServiceClient,
	poolID string,
	opts pools.CreateMemberOpts,
	timeout time.Duration,
) (*pools.Member, error) {
	deadline := time.Now().Add(timeout)

	for {
		member, err := pools.CreateMember(ctx, lbClient, poolID, opts).Extract()
		if err == nil {
			return member, nil
		}

		if !isRetryableLBError(err) {
			return nil, err
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout creating member: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// updateMemberWithRetry mirrors createMemberWithRetry for UpdateMember.
func updateMemberWithRetry(
	ctx context.Context,
	lbClient *gophercloud.ServiceClient,
	poolID, memberID string,
	opts pools.UpdateMemberOpts,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)

	for {
		_, err := pools.UpdateMember(ctx, lbClient, poolID, memberID, opts).Extract()
		if err == nil {
			return nil
		}

		if !isRetryableLBError(err) {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout updating member: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// deleteMemberWithRetry mirrors createMemberWithRetry for DeleteMember.
func deleteMemberWithRetry(
	ctx context.Context,
	lbClient *gophercloud.ServiceClient,
	poolID, memberID string,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)

	for {
		err := pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()
		if err == nil {
			return nil
		}

		if !isRetryableLBError(err) {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout deleting member: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// isRetryableLBError matches the same error classes the SDKv2
// `checkForRetryableError` helper treated as retryable: 409, 500, 502, 503, 504.
func isRetryableLBError(err error) bool {
	var e gophercloud.ErrUnexpectedResponseCode
	if !errors.As(err, &e) {
		return false
	}

	switch e.Actual {
	case http.StatusConflict,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}

	return false
}
