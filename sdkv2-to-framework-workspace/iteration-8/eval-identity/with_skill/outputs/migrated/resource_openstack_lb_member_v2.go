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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure interface compliance at compile time.
var (
	_ resource.Resource                = &lbMemberV2Resource{}
	_ resource.ResourceWithConfigure   = &lbMemberV2Resource{}
	_ resource.ResourceWithImportState = &lbMemberV2Resource{}
	_ resource.ResourceWithIdentity    = &lbMemberV2Resource{}
)

// NewLbMemberV2Resource returns a new instance of the resource.
func NewLbMemberV2Resource() resource.Resource {
	return &lbMemberV2Resource{}
}

// lbMemberV2Resource is the framework resource struct.
type lbMemberV2Resource struct {
	config *Config
}

// lbMemberV2Model is the Terraform state model for the resource.
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
	Tags           []string       `tfsdk:"tags"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

// lbMemberV2IdentityModel is the identity model for composite-ID import.
type lbMemberV2IdentityModel struct {
	PoolID   types.String `tfsdk:"pool_id"`
	MemberID types.String `tfsdk:"member_id"`
}

// Metadata sets the resource type name.
func (r *lbMemberV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_member_v2"
}

// Schema defines the resource schema.
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
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"monitor_address": schema.StringAttribute{
				Optional: true,
			},
			"monitor_port": schema.Int64Attribute{
				Optional: true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
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

// IdentitySchema defines the identity schema for composite-ID import (Terraform 1.12+).
// Two attributes are RequiredForImport so practitioners can write:
//
//	import {
//	  to       = openstack_lb_member_v2.foo
//	  identity = { pool_id = "…", member_id = "…" }
//	}
func (r *lbMemberV2Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"pool_id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "The ID of the pool this member belongs to.",
			},
			"member_id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "The ID of the member.",
			},
		},
	}
}

// Configure stores the provider-level config.
func (r *lbMemberV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = config
}

// getRegion returns the region from the model or falls back to the provider config.
func (r *lbMemberV2Resource) getRegion(region types.String) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	return r.config.Region
}

// isRetryableLBError returns true for transient HTTP status codes.
func isRetryableLBError(err error) bool {
	var e gophercloud.ErrUnexpectedResponseCode
	if !errors.As(err, &e) {
		return false
	}

	switch e.Actual {
	case http.StatusConflict,           // 409
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	}

	return false
}

// retryCreateMember wraps pools.CreateMember with a retry loop for transient
// errors. The SDKv2 helper/retry package is NOT used in this framework file.
func retryCreateMember(ctx context.Context, lbClient *gophercloud.ServiceClient, poolID string, opts pools.CreateMemberOpts, timeout time.Duration) (*pools.Member, error) {
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
			return nil, fmt.Errorf("timeout waiting for member creation: %w", err)
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// retryUpdateMember wraps pools.UpdateMember with the same retry logic.
func retryUpdateMember(ctx context.Context, lbClient *gophercloud.ServiceClient, poolID, memberID string, opts pools.UpdateMemberOpts, timeout time.Duration) error {
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
			return fmt.Errorf("timeout waiting for member update: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// retryDeleteMember wraps pools.DeleteMember with the same retry logic.
func retryDeleteMember(ctx context.Context, lbClient *gophercloud.ServiceClient, poolID, memberID string, timeout time.Duration) error {
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
			return fmt.Errorf("timeout waiting for member deletion: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// populateStateFromMember copies API-returned fields into the model.
func populateStateFromMember(state *lbMemberV2Model, member *pools.Member, region string) {
	state.Name = types.StringValue(member.Name)
	state.Weight = types.Int64Value(int64(member.Weight))
	state.AdminStateUp = types.BoolValue(member.AdminStateUp)
	state.TenantID = types.StringValue(member.ProjectID)
	state.SubnetID = types.StringValue(member.SubnetID)
	state.Address = types.StringValue(member.Address)
	state.ProtocolPort = types.Int64Value(int64(member.ProtocolPort))
	state.Region = types.StringValue(region)
	state.MonitorAddress = types.StringValue(member.MonitorAddress)
	state.MonitorPort = types.Int64Value(int64(member.MonitorPort))
	state.Backup = types.BoolValue(member.Backup)
	state.Tags = member.Tags
}

// Create implements resource.Resource.
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

	region := r.getRegion(plan.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack LB client", err.Error())

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

	if !plan.SubnetID.IsNull() && !plan.SubnetID.IsUnknown() {
		createOpts.SubnetID = plan.SubnetID.ValueString()
	}

	// Only set weight if explicitly configured (not null), to avoid creating
	// members with a default weight of 0 when the attribute is omitted.
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		w := int(plan.Weight.ValueInt64())
		createOpts.Weight = &w
	}

	if !plan.MonitorAddress.IsNull() && !plan.MonitorAddress.IsUnknown() {
		createOpts.MonitorAddress = plan.MonitorAddress.ValueString()
	}

	if !plan.MonitorPort.IsNull() && !plan.MonitorPort.IsUnknown() {
		mp := int(plan.MonitorPort.ValueInt64())
		createOpts.MonitorPort = &mp
	}

	// Only set backup if explicitly configured (requires API version 2.1+).
	if !plan.Backup.IsNull() && !plan.Backup.IsUnknown() {
		bk := plan.Backup.ValueBool()
		createOpts.Backup = &bk
	}

	if len(plan.Tags) > 0 {
		createOpts.Tags = plan.Tags
	}

	log.Printf("[DEBUG] Create Options: %#v", createOpts)

	poolID := plan.PoolID.ValueString()

	// Get a clean copy of the parent pool and wait for it to be ACTIVE.
	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("Unable to retrieve parent pool %s: %s", poolID, err),
		)

		return
	}

	if err = waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to create member")

	member, err := retryCreateMember(ctx, lbClient, poolID, createOpts, createTimeout)
	if err != nil {
		resp.Diagnostics.AddError("Error creating member", err.Error())

		return
	}

	// Wait for member to become active.
	if err = waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())

		return
	}

	plan.ID = types.StringValue(member.ID)
	populateStateFromMember(&plan, member, region)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	// Set identity for Terraform 1.12+ import blocks.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   types.StringValue(poolID),
		MemberID: types.StringValue(member.ID),
	})...)
}

// Read implements resource.Resource.
func (r *lbMemberV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lbMemberV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.getRegion(state.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack LB client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			"Error reading member",
			fmt.Sprintf("Error reading member %s: %s", memberID, err),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved member %s: %#v", memberID, member)

	populateStateFromMember(&state, member, region)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	// Refresh identity on every read so it stays current.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   state.PoolID,
		MemberID: types.StringValue(memberID),
	})...)
}

// Update implements resource.Resource.
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

	region := r.getRegion(state.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack LB client", err.Error())

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
		bk := plan.Backup.ValueBool()
		updateOpts.Backup = &bk
	}

	// Always send tags: an empty/nil slice means "clear all tags".
	if plan.Tags != nil {
		updateOpts.Tags = plan.Tags
	} else {
		updateOpts.Tags = []string{}
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	// Get a clean copy of the parent pool.
	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("Unable to retrieve parent pool %s: %s", poolID, err),
		)

		return
	}

	// Get a clean copy of the member.
	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve member",
			fmt.Sprintf("Unable to retrieve member %s: %s", memberID, err),
		)

		return
	}

	// Wait for parent pool to become active before continuing.
	if err = waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become ACTIVE", err.Error())

		return
	}

	// Wait for the member to become active before continuing.
	if err = waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Updating member %s with options: %#v", memberID, updateOpts)

	if err = retryUpdateMember(ctx, lbClient, poolID, memberID, updateOpts, updateTimeout); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update member",
			fmt.Sprintf("Unable to update member %s: %s", memberID, err),
		)

		return
	}

	// Wait for the member to become active again after the update.
	if err = waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE after update", err.Error())

		return
	}

	// Re-read the updated member from the API to populate state.
	updatedMember, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading updated member",
			fmt.Sprintf("Error reading updated member %s: %s", memberID, err),
		)

		return
	}

	populateStateFromMember(&state, updatedMember, region)
	state.ID = types.StringValue(memberID)
	state.PoolID = types.StringValue(poolID)
	// Preserve the timeouts from the plan.
	state.Timeouts = plan.Timeouts

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

	// Refresh identity.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   types.StringValue(poolID),
		MemberID: types.StringValue(memberID),
	})...)
}

// Delete implements resource.Resource.
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

	region := r.getRegion(state.Region)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack LB client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	// Get a clean copy of the parent pool.
	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("Unable to retrieve parent pool (%s) for the member: %s", poolID, err),
		)

		return
	}

	// Get a clean copy of the member.
	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Already deleted.
			return
		}

		resp.Diagnostics.AddError(
			"Unable to retrieve member",
			fmt.Sprintf("Unable to retrieve member %s: %s", memberID, err),
		)

		return
	}

	// Wait for parent pool to become active before continuing.
	if err = waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Error waiting for parent pool status before delete", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to delete member %s", memberID)

	if err = retryDeleteMember(ctx, lbClient, poolID, memberID, deleteTimeout); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			"Error deleting member",
			fmt.Sprintf("Error deleting member %s: %s", memberID, err),
		)

		return
	}

	// Wait for the member to become DELETED.
	if err = waitForLBV2Member(ctx, lbClient, parentPool, member, "DELETED", getLbPendingDeleteStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be DELETED", err.Error())
	}
}

// ImportState implements resource.ResourceWithImportState.
//
// Handles both import paths:
//
//  1. Legacy CLI: `terraform import openstack_lb_member_v2.foo <pool_id>/<member_id>`
//     In this case req.ID is the composite string; req.Identity is empty.
//
//  2. Modern HCL (Terraform 1.12+):
//     import { to = openstack_lb_member_v2.foo; identity = { pool_id = "…", member_id = "…" } }
//     In this case req.ID is empty; req.Identity carries the typed attributes.
func (r *lbMemberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		// Modern path — Terraform 1.12+ passes identity attributes.
		// Copy each identity attribute into the corresponding state attribute.
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("member_id"), req, resp)

		return
	}

	// Legacy path — composite string: <pool_id>/<member_id>.
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID format",
			fmt.Sprintf(
				"Expected '<pool_id>/<member_id>', got %q. Format must be <pool id>/<member id>",
				req.ID,
			),
		)

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
