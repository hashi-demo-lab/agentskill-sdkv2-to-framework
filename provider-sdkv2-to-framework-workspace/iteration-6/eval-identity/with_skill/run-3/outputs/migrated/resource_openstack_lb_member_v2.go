package openstack

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
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
)

// Compile-time interface assertions.
var (
	_ resource.Resource                   = &memberV2Resource{}
	_ resource.ResourceWithConfigure      = &memberV2Resource{}
	_ resource.ResourceWithImportState    = &memberV2Resource{}
	_ resource.ResourceWithIdentity       = &memberV2Resource{}
)

// NewMemberV2Resource is the framework constructor.
func NewMemberV2Resource() resource.Resource {
	return &memberV2Resource{}
}

type memberV2Resource struct {
	config *Config
}

// memberV2Model is the typed model for the resource state/plan/config.
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

// memberV2IdentityModel is the typed model for the resource identity (composite ID).
type memberV2IdentityModel struct {
	Region   types.String `tfsdk:"region"`
	PoolID   types.String `tfsdk:"pool_id"`
	MemberID types.String `tfsdk:"id"`
}

func (r *memberV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_member_v2"
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
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
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

func (r *memberV2Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
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

func (r *memberV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

// regionFor returns the configured region (model value if present, else provider default).
func (r *memberV2Resource) regionFor(model memberV2Model) string {
	if !model.Region.IsNull() && !model.Region.IsUnknown() && model.Region.ValueString() != "" {
		return model.Region.ValueString()
	}

	return r.config.Region
}

// setIdentity writes the resource identity from the current model.
// `identity` may be nil for older Terraform CLI versions that do not request identity.
func setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, model memberV2Model) diag.Diagnostics {
	if identity == nil {
		return nil
	}

	id := memberV2IdentityModel{
		Region:   model.Region,
		PoolID:   model.PoolID,
		MemberID: model.ID,
	}

	return identity.Set(ctx, id)
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

	region := r.regionFor(plan)

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

	if !plan.SubnetID.IsNull() && !plan.SubnetID.IsUnknown() && plan.SubnetID.ValueString() != "" {
		createOpts.SubnetID = plan.SubnetID.ValueString()
	}

	// Frame "GetOkExists" with !IsNull (cf. references/state-and-types.md). The user
	// explicitly setting weight = 0 is meaningful and must be passed through.
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
		b := plan.Backup.ValueBool()
		createOpts.Backup = &b
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tagSlice []string

		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tagSlice, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		createOpts.Tags = tagSlice
	}

	log.Printf("[DEBUG] openstack_lb_member_v2 create options: %#v", createOpts)

	poolID := plan.PoolID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("pool_id=%s: %s", poolID, err),
		)

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to be ACTIVE", err.Error())

		return
	}

	var member *pools.Member

	err = retryOnConflict(ctx, createTimeout, func() error {
		var createErr error

		member, createErr = pools.CreateMember(ctx, lbClient, poolID, createOpts).Extract()

		return createErr
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating openstack_lb_member_v2", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be ACTIVE", err.Error())

		return
	}

	plan.ID = types.StringValue(member.ID)
	plan.Region = types.StringValue(region)

	// Re-read the new resource to populate computed fields cleanly.
	if err := r.readMemberInto(ctx, lbClient, region, poolID, member.ID, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading openstack_lb_member_v2 after create", err.Error())

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, plan)...)
}

func (r *memberV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(state)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	if err := r.readMemberInto(ctx, lbClient, region, poolID, memberID, &state); err != nil {
		// If the resource is gone, drop it from state so Terraform recreates it.
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Error retrieving openstack_lb_member_v2", err.Error())

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, state)...)
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

	region := r.regionFor(plan)

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
		if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
			var tagSlice []string

			resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tagSlice, false)...)
			if resp.Diagnostics.HasError() {
				return
			}

			updateOpts.Tags = tagSlice
		} else {
			updateOpts.Tags = []string{}
		}
	}

	poolID := plan.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("pool_id=%s: %s", poolID, err),
		)

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve member", fmt.Sprintf("%s: %s", memberID, err))

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to be ACTIVE", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Updating openstack_lb_member_v2 %s with options: %#v", memberID, updateOpts)

	err = retryOnConflict(ctx, updateTimeout, func() error {
		_, updateErr := pools.UpdateMember(ctx, lbClient, poolID, memberID, updateOpts).Extract()

		return updateErr
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update openstack_lb_member_v2", fmt.Sprintf("%s: %s", memberID, err))

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be ACTIVE", err.Error())

		return
	}

	// Re-read to refresh computed fields after the update.
	plan.ID = state.ID
	plan.Region = types.StringValue(region)

	if err := r.readMemberInto(ctx, lbClient, region, poolID, memberID, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading openstack_lb_member_v2 after update", err.Error())

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(setIdentity(ctx, resp.Identity, plan)...)
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

	region := r.regionFor(state)

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
			// Pool already gone; member is implicitly gone too.
			return
		}

		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool for member",
			fmt.Sprintf("pool_id=%s: %s", poolID, err),
		)

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Unable to retrieve member", err.Error())

		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Error waiting for parent pool to be ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to delete openstack_lb_member_v2 %s", memberID)

	err = retryOnConflict(ctx, deleteTimeout, func() error {
		return pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()
	})
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError("Error deleting openstack_lb_member_v2", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "DELETED", getLbPendingDeleteStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member deletion", err.Error())

		return
	}
}

// ImportState handles both legacy `pool_id/member_id` and modern identity-based imports.
func (r *memberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Modern path — Terraform 1.12+ with `import { identity = {...} }`.
	if req.ID == "" {
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("region"), path.Root("region"), req, resp)
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("id"), req, resp)

		return
	}

	// Legacy path — `terraform import openstack_lb_member_v2.foo <pool_id>/<member_id>`.
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format <pool id>/<member id>, got %q", req.ID),
		)

		return
	}

	poolID := parts[0]
	memberID := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), poolID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memberID)...)
}

// readMemberInto fetches a member from the API and writes its fields into model.
func (r *memberV2Resource) readMemberInto(ctx context.Context, lbClient *gophercloud.ServiceClient, region, poolID, memberID string, model *memberV2Model) error {
	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] Retrieved openstack_lb_member_v2 %s: %#v", memberID, member)

	model.ID = types.StringValue(member.ID)
	model.Region = types.StringValue(region)
	model.Name = types.StringValue(member.Name)
	model.Weight = types.Int64Value(int64(member.Weight))
	model.AdminStateUp = types.BoolValue(member.AdminStateUp)
	model.TenantID = types.StringValue(member.ProjectID)
	model.SubnetID = types.StringValue(member.SubnetID)
	model.Address = types.StringValue(member.Address)
	model.ProtocolPort = types.Int64Value(int64(member.ProtocolPort))
	model.MonitorAddress = types.StringValue(member.MonitorAddress)
	model.MonitorPort = types.Int64Value(int64(member.MonitorPort))
	model.Backup = types.BoolValue(member.Backup)
	model.PoolID = types.StringValue(poolID)

	tagsValue, diags := types.SetValueFrom(ctx, types.StringType, member.Tags)
	if diags.HasError() {
		return fmt.Errorf("failed to convert tags to set: %v", diags)
	}

	model.Tags = tagsValue

	return nil
}

// retryOnConflict re-runs op while the API returns retryable response codes,
// up to the given timeout. Replaces SDKv2 retry.RetryContext for this resource.
// The set of retryable codes mirrors checkForRetryableError in util.go.
func retryOnConflict(ctx context.Context, timeout time.Duration, op func() error) error {
	retryable := []int{
		http.StatusConflict,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	deadline := time.Now().Add(timeout)
	backoff := 2 * time.Second

	for {
		err := op()
		if err == nil {
			return nil
		}

		var unexpected gophercloud.ErrUnexpectedResponseCode
		if !errors.As(err, &unexpected) || !slices.Contains(retryable, unexpected.Actual) {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("retry deadline exceeded: %w", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		// Exponential-ish backoff, capped.
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}
