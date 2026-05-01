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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
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

// NewLbMemberV2Resource is the factory used by the provider to register the resource.
func NewLbMemberV2Resource() resource.Resource {
	return &lbMemberV2Resource{}
}

// lbMemberV2Resource is the framework implementation of openstack_lb_member_v2.
type lbMemberV2Resource struct {
	config *Config
}

// lbMemberV2Model is the state/plan model for openstack_lb_member_v2.
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

// lbMemberV2IdentityModel is the identity model that enables Terraform 1.12+ import blocks.
type lbMemberV2IdentityModel struct {
	PoolID   types.String `tfsdk:"pool_id"`
	MemberID types.String `tfsdk:"member_id"`
}

// ---------------------------------------------------------------------------
// resource.Resource interface
// ---------------------------------------------------------------------------

func (r *lbMemberV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_member_v2"
}

// ---------------------------------------------------------------------------
// resource.ResourceWithConfigure
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// resource.ResourceWithIdentity
// ---------------------------------------------------------------------------

// IdentitySchema defines the identity attributes for Terraform 1.12+ identity-block imports.
// The composite key is pool_id + member_id, matching the legacy <pool_id>/<member_id> string.
func (r *lbMemberV2Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"pool_id": identityschema.StringAttribute{
				Description:       "The ID of the pool this member belongs to.",
				RequiredForImport: true,
			},
			"member_id": identityschema.StringAttribute{
				Description:       "The unique ID of the member.",
				RequiredForImport: true,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

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
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},

			"monitor_address": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"monitor_port": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},

			"tags": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},

		// Preserve block syntax (`timeouts { create = "5m" }`) so existing HCL
		// configurations continue to work after migration.
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

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

	region := lbMemberV2Region(plan.Region, r.config)

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

	if !plan.Backup.IsNull() && !plan.Backup.IsUnknown() {
		backup := plan.Backup.ValueBool()
		createOpts.Backup = &backup
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
			fmt.Sprintf("Unable to retrieve parent pool %s", poolID),
			err.Error(),
		)
		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become ACTIVE", err.Error())
		return
	}

	log.Printf("[DEBUG] Attempting to create member")

	var member *pools.Member

	err = lbMemberV2Retry(ctx, createTimeout, func() error {
		member, err = pools.CreateMember(ctx, lbClient, poolID, createOpts).Extract()
		return err
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
	plan.Region = types.StringValue(region)

	// Populate all computed fields from the API response.
	lbMemberV2ReadInto(ctx, lbClient, region, poolID, member.ID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

	// Write identity so Terraform 1.12+ can reconstruct this resource from identity alone.
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

	region := lbMemberV2Region(state.Region, r.config)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack LB client", err.Error())
		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	lbMemberV2ReadInto(ctx, lbClient, region, poolID, memberID, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// 404 — resource was deleted outside of Terraform.
	if state.ID.ValueString() == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)

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

	updateTimeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := lbMemberV2Region(state.Region, r.config)

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
		weight := int(plan.Weight.ValueInt64())
		updateOpts.Weight = &weight
	}

	if !plan.AdminStateUp.Equal(state.AdminStateUp) {
		asu := plan.AdminStateUp.ValueBool()
		updateOpts.AdminStateUp = &asu
	}

	if !plan.MonitorAddress.Equal(state.MonitorAddress) {
		monitorAddress := plan.MonitorAddress.ValueString()
		updateOpts.MonitorAddress = &monitorAddress
	}

	if !plan.MonitorPort.Equal(state.MonitorPort) {
		monitorPort := int(plan.MonitorPort.ValueInt64())
		updateOpts.MonitorPort = &monitorPort
	}

	if !plan.Backup.Equal(state.Backup) {
		backup := plan.Backup.ValueBool()
		updateOpts.Backup = &backup
	}

	if !plan.Tags.Equal(state.Tags) {
		if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
			var tagList []string
			resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tagList, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			updateOpts.Tags = tagList
		} else {
			updateOpts.Tags = []string{}
		}
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to retrieve parent pool %s", poolID),
			err.Error(),
		)
		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to retrieve member %s", memberID),
			err.Error(),
		)
		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become ACTIVE", err.Error())
		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE before update", err.Error())
		return
	}

	log.Printf("[DEBUG] Updating member %s with options: %#v", memberID, updateOpts)

	err = lbMemberV2Retry(ctx, updateTimeout, func() error {
		_, err = pools.UpdateMember(ctx, lbClient, poolID, memberID, updateOpts).Extract()
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to update member %s", memberID),
			err.Error(),
		)
		return
	}

	// Re-fetch to have a fresh member for the post-update wait.
	member, err = pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to retrieve member %s after update", memberID),
			err.Error(),
		)
		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE after update", err.Error())
		return
	}

	// Re-read from API to populate all computed fields.
	lbMemberV2ReadInto(ctx, lbClient, region, poolID, memberID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)

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

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := lbMemberV2Region(state.Region, r.config)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack LB client", err.Error())
		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to retrieve parent pool (%s) for the member", poolID),
			err.Error(),
		)
		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to retrieve member %s", memberID),
			err.Error(),
		)
		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error waiting for members pool to become ACTIVE before delete", err.Error())
		return
	}

	log.Printf("[DEBUG] Attempting to delete member %s", memberID)

	err = lbMemberV2Retry(ctx, deleteTimeout, func() error {
		return pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()
	})
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}
		resp.Diagnostics.AddError(fmt.Sprintf("Error deleting member %s", memberID), err.Error())
		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "DELETED", getLbPendingDeleteStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be DELETED", err.Error())
	}
}

// ---------------------------------------------------------------------------
// resource.ResourceWithImportState
// ---------------------------------------------------------------------------

// ImportState handles two import paths:
//
// Modern (Terraform 1.12+) — practitioner writes:
//
//	import {
//	  to = openstack_lb_member_v2.foo
//	  identity = { pool_id = "...", member_id = "..." }
//	}
//
// Legacy — practitioner runs:
//
//	terraform import openstack_lb_member_v2.foo <pool_id>/<member_id>
func (r *lbMemberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Modern path: req.ID is empty; identity attributes are populated by Terraform.
	if req.ID == "" {
		// Copy pool_id from the identity into state attribute "pool_id".
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
		// Copy member_id from the identity into state attribute "id".
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("member_id"), req, resp)
		return
	}

	// Legacy path: composite string "<pool_id>/<member_id>".
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID format",
			fmt.Sprintf("Expected '<pool_id>/<member_id>', got %q. Format must be <pool_id>/<member_id>", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// ---------------------------------------------------------------------------
// Package-private helpers
// ---------------------------------------------------------------------------

// lbMemberV2Region resolves the effective region from the state/plan value or the
// provider-level default.
func lbMemberV2Region(regionAttr types.String, config *Config) string {
	if !regionAttr.IsNull() && !regionAttr.IsUnknown() && regionAttr.ValueString() != "" {
		return regionAttr.ValueString()
	}

	return config.Region
}

// lbMemberV2ReadInto populates model fields from the API.
// On 404, it sets model.ID to the empty string so callers can call
// resp.State.RemoveResource(ctx).
func lbMemberV2ReadInto(
	ctx context.Context,
	lbClient *gophercloud.ServiceClient,
	region, poolID, memberID string,
	model *lbMemberV2Model,
	diagnostics *diag.Diagnostics,
) {
	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			model.ID = types.StringValue("")
			return
		}

		diagnostics.AddError(
			fmt.Sprintf("Error reading member %s", memberID),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved member %s: %#v", memberID, member)

	model.ID = types.StringValue(member.ID)
	model.Name = types.StringValue(member.Name)
	model.Weight = types.Int64Value(int64(member.Weight))
	model.AdminStateUp = types.BoolValue(member.AdminStateUp)
	model.TenantID = types.StringValue(member.ProjectID)
	model.SubnetID = types.StringValue(member.SubnetID)
	model.Address = types.StringValue(member.Address)
	model.ProtocolPort = types.Int64Value(int64(member.ProtocolPort))
	model.Region = types.StringValue(region)
	model.MonitorAddress = types.StringValue(member.MonitorAddress)
	model.MonitorPort = types.Int64Value(int64(member.MonitorPort))
	model.Backup = types.BoolValue(member.Backup)
	model.PoolID = types.StringValue(poolID)

	tagSet, tagDiags := types.SetValueFrom(ctx, types.StringType, member.Tags)
	diagnostics.Append(tagDiags...)

	if !diagnostics.HasError() {
		model.Tags = tagSet
	}
}

// lbMemberV2Retry retries fn on transient HTTP errors (409/500/502/503/504).
// It replaces the SDKv2 retry.RetryContext + checkForRetryableError pattern
// without importing terraform-plugin-sdk/v2.
func lbMemberV2Retry(ctx context.Context, timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)

	for {
		err := fn()
		if err == nil {
			return nil
		}

		var e gophercloud.ErrUnexpectedResponseCode

		if !errors.As(err, &e) {
			// Not an HTTP response error — non-retryable.
			return err
		}

		switch e.Actual {
		case http.StatusConflict,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			// transient — retry
		default:
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for transient error to clear: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}
