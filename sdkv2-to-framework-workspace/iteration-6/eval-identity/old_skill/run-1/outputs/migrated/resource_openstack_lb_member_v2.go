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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions. A missing method becomes a build error.
var (
	_ resource.Resource                = &memberV2Resource{}
	_ resource.ResourceWithConfigure   = &memberV2Resource{}
	_ resource.ResourceWithImportState = &memberV2Resource{}
	_ resource.ResourceWithIdentity    = &memberV2Resource{}
)

// NewMemberV2Resource returns the framework-shaped openstack_lb_member_v2 resource.
//
// Wire this into your framework provider's Resources() method (or, while the
// rest of the provider is still SDKv2, expose it via terraform-plugin-mux).
func NewMemberV2Resource() resource.Resource {
	return &memberV2Resource{}
}

type memberV2Resource struct {
	config *Config
}

// memberV2Model is the typed model the framework reads/writes against state,
// plan, and config. Field names map to schema attributes via tfsdk tags.
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

// memberV2IdentityModel is the identity payload Terraform 1.12+ practitioners
// can use inside `import { identity = {...} }` blocks.
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
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
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
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},

			"tags": schema.SetAttribute{
				Optional:    true,
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

// IdentitySchema declares the framework identity for this resource. Practitioners
// on Terraform 1.12+ can write:
//
//	import {
//	  to = openstack_lb_member_v2.example
//	  identity = {
//	    pool_id = "..."
//	    id      = "..."
//	  }
//	}
//
// The legacy `terraform import openstack_lb_member_v2.example <pool>/<member>`
// form keeps working (see ImportState below).
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

	if !plan.SubnetID.IsNull() && !plan.SubnetID.IsUnknown() {
		createOpts.SubnetID = plan.SubnetID.ValueString()
	}

	// Only set weight if the user explicitly configured it. The framework
	// distinguishes "null" (not set) from "known zero" (explicitly 0), so
	// !IsNull() is the right test — equivalent to SDKv2's getOkExists.
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

	// `backup` is optional and only sent if the user set it (requires API 2.1+).
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
	err = retryFrameworkCtx(ctx, createTimeout, func() (bool, error) {
		var innerErr error
		member, innerErr = pools.CreateMember(ctx, lbClient, poolID, createOpts).Extract()

		return isRetryableLBErr(innerErr), innerErr
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

	// Re-read so computed fields (weight default, tenant_id, etc.) reflect what
	// the API actually stored.
	if !r.refreshFromAPI(ctx, lbClient, &plan, poolID, member.ID, region, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Mirror the addressing fields into identity so Terraform 1.12+ practitioners
	// see a populated identity after apply.
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
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Resource is gone — signal recreation on next plan.
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to read member %s", memberID),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved member %s: %#v", memberID, member)

	r.applyMember(ctx, &state, member, region, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

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
		if plan.Tags.IsNull() || plan.Tags.IsUnknown() {
			updateOpts.Tags = []string{}
		} else {
			var tags []string
			resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
			if resp.Diagnostics.HasError() {
				return
			}

			updateOpts.Tags = tags
		}
	}

	poolID := plan.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to retrieve parent pool %s", poolID),
			err.Error(),
		)

		return
	}

	currentMember, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
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

	if err := waitForLBV2Member(ctx, lbClient, parentPool, currentMember, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Updating member %s with options: %#v", memberID, updateOpts)

	err = retryFrameworkCtx(ctx, updateTimeout, func() (bool, error) {
		_, innerErr := pools.UpdateMember(ctx, lbClient, poolID, memberID, updateOpts).Extract()

		return isRetryableLBErr(innerErr), innerErr
	})
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to update member %s", memberID),
			err.Error(),
		)

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, currentMember, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE after update", err.Error())

		return
	}

	// Re-read to pick up any computed fields the API rewrote.
	if !r.refreshFromAPI(ctx, lbClient, &plan, poolID, memberID, region, &resp.Diagnostics) {
		return
	}

	plan.ID = types.StringValue(memberID)
	plan.Region = types.StringValue(region)

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
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to retrieve parent pool %s for the member", poolID),
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
		resp.Diagnostics.AddError("Error waiting for the members pool status", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to delete member %s", memberID)

	err = retryFrameworkCtx(ctx, deleteTimeout, func() (bool, error) {
		innerErr := pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()

		return isRetryableLBErr(innerErr), innerErr
	})
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting member %s", memberID),
			err.Error(),
		)

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "DELETED", getLbPendingDeleteStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be DELETED", err.Error())

		return
	}
}

// ImportState supports both the legacy composite-string form and the modern
// identity-block form.
//
//   - Legacy:  `terraform import openstack_lb_member_v2.foo <pool>/<member>`
//     Terraform sets req.ID to the slash-delimited string. Parse it.
//   - Modern:  `import { to = ... ; identity = { pool_id = ..., id = ... } }`
//     Terraform leaves req.ID empty and populates req.Identity. Pass each
//     identity attribute through into state.
//
// In both cases, Read runs after this method to populate the rest of the state.
func (r *memberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		// Modern path — Terraform 1.12+ supplied identity. Mirror identity
		// attributes into state. ImportStatePassthroughWithIdentity handles
		// one (state, identity) attribute pair per call.
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("id"), req, resp)

		return
	}

	// Legacy path — practitioner ran `terraform import` with a composite ID.
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Invalid format specified for Member. Format must be <pool id>/<member id>, got %q", req.ID),
		)

		return
	}

	poolID := parts[0]
	memberID := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), poolID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memberID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Also seed identity so practitioners on Terraform 1.12+ have a valid
	// identity in state immediately after a legacy CLI import.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, memberV2IdentityModel{
		PoolID: types.StringValue(poolID),
		ID:     types.StringValue(memberID),
	})...)
}

// regionFor returns the per-resource region if set, otherwise the provider's
// default region — the framework analogue of the SDKv2 GetRegion helper.
func (r *memberV2Resource) regionFor(region types.String) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	return r.config.Region
}

// applyMember copies API fields onto the model.
func (r *memberV2Resource) applyMember(ctx context.Context, m *memberV2Model, member *pools.Member, region string, diags *diag.Diagnostics) {
	m.ID = types.StringValue(member.ID)
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

	tags, d := types.SetValueFrom(ctx, types.StringType, member.Tags)
	diags.Append(d...)
	m.Tags = tags
}

// refreshFromAPI re-fetches the member and applies it to the model. Returns
// true on success; on error appends to diags and returns false.
func (r *memberV2Resource) refreshFromAPI(ctx context.Context, lbClient *gophercloud.ServiceClient, m *memberV2Model, poolID, memberID, region string, diags *diag.Diagnostics) bool {
	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Unable to read member %s after write", memberID),
			err.Error(),
		)

		return false
	}

	r.applyMember(ctx, m, member, region, diags)

	return !diags.HasError()
}

// retryFrameworkCtx is a small framework-side replacement for SDKv2's
// retry.RetryContext. It calls op until either the context expires, op
// returns (false, anyErr) (= non-retryable), or op returns (true, nil).
func retryFrameworkCtx(ctx context.Context, timeout time.Duration, op func() (retryable bool, err error)) error {
	deadline := time.Now().Add(timeout)

	var lastErr error

	for {
		retryable, err := op()
		if err == nil {
			return nil
		}

		lastErr = err

		if !retryable {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s, last error: %w", timeout, lastErr)
		}

		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), lastErr)
		case <-time.After(2 * time.Second):
		}
	}
}

// isRetryableLBErr mirrors checkForRetryableError's classification but in a
// shape independent of the SDKv2 retry types.
func isRetryableLBErr(err error) bool {
	if err == nil {
		return false
	}

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
