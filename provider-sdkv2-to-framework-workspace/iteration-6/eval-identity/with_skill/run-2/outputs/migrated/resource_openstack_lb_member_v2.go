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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                   = &lbMemberV2Resource{}
	_ resource.ResourceWithConfigure      = &lbMemberV2Resource{}
	_ resource.ResourceWithImportState    = &lbMemberV2Resource{}
	_ resource.ResourceWithIdentity       = &lbMemberV2Resource{}
)

// NewLBMemberV2Resource is the framework resource constructor.
func NewLBMemberV2Resource() resource.Resource {
	return &lbMemberV2Resource{}
}

type lbMemberV2Resource struct {
	config *Config
}

// lbMemberV2Model maps the resource state.
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

// lbMemberV2IdentityModel describes the resource identity (composite ID).
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
				PlanModifiers: []planmodifier.Bool{
					boolUseStateForUnknown(),
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
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setUseStateForUnknown(),
				},
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

// IdentitySchema declares the typed identity for this resource. The composite
// id is (pool_id, member_id); member_id holds the value previously written to
// d.SetId() in SDKv2 (i.e. the OpenStack member UUID).
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

	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromPlan(plan)

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

	// Set the weight only if it's defined in the configuration. The framework
	// distinguishes null (not set) from known zero, so an explicit weight = 0
	// is honoured.
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		w := int(plan.Weight.ValueInt64())
		createOpts.Weight = &w
	}

	if !plan.MonitorAddress.IsNull() && !plan.MonitorAddress.IsUnknown() && plan.MonitorAddress.ValueString() != "" {
		createOpts.MonitorAddress = plan.MonitorAddress.ValueString()
	}

	if !plan.MonitorPort.IsNull() && !plan.MonitorPort.IsUnknown() {
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

	log.Printf("[DEBUG] Create Options: %#v", createOpts)

	poolID := plan.PoolID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("Pool %s: %s", poolID, err),
		)

		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to create member")

	var member *pools.Member

	err = retryFuncFramework(ctx, createTimeout, func() error {
		var createErr error

		member, createErr = pools.CreateMember(ctx, lbClient, poolID, createOpts).Extract()

		return createErr
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

	if err := r.populateModelFromAPI(ctx, &plan, lbClient, poolID, member.ID, region, &resp.Diagnostics); err != nil {
		// Diagnostics already populated; bail.
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set identity alongside state.
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

	region := r.regionFromPlan(state)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	if err := r.populateModelFromAPI(ctx, &state, lbClient, poolID, memberID, region, &resp.Diagnostics); err != nil {
		if errors.Is(err, errLBMemberGone) {
			resp.State.RemoveResource(ctx)

			return
		}

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Keep identity in sync.
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

	region := r.regionFromPlan(state)

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

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool",
			fmt.Sprintf("Pool %s: %s", poolID, err),
		)

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to retrieve member",
			fmt.Sprintf("%s: %s", memberID, err),
		)

		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool to become ACTIVE", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Updating member %s with options: %#v", memberID, updateOpts)

	err = retryFuncFramework(ctx, updateTimeout, func() error {
		_, updateErr := pools.UpdateMember(ctx, lbClient, poolID, memberID, updateOpts).Extract()

		return updateErr
	})
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update member",
			fmt.Sprintf("%s: %s", memberID, err),
		)

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())

		return
	}

	// Refresh state.
	plan.ID = state.ID
	plan.Region = types.StringValue(region)

	if err := r.populateModelFromAPI(ctx, &plan, lbClient, poolID, memberID, region, &resp.Diagnostics); err != nil {
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

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromPlan(state)

	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		if isNotFoundError(err) {
			return
		}

		resp.Diagnostics.AddError(
			"Unable to retrieve parent pool for the member",
			fmt.Sprintf("%s: %s", poolID, err),
		)

		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if isNotFoundError(err) {
			return
		}

		resp.Diagnostics.AddError("Unable to retrieve member", err.Error())

		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		if isNotFoundError(err) {
			return
		}

		resp.Diagnostics.AddError("Error waiting for parent pool to become ACTIVE", err.Error())

		return
	}

	log.Printf("[DEBUG] Attempting to delete member %s", memberID)

	err = retryFuncFramework(ctx, deleteTimeout, func() error {
		return pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()
	})
	if err != nil {
		if isNotFoundError(err) {
			return
		}

		resp.Diagnostics.AddError("Error deleting member", err.Error())

		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "DELETED", getLbPendingDeleteStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to be DELETED", err.Error())

		return
	}
}

// ImportState handles BOTH the legacy CLI form
//
//	terraform import openstack_lb_member_v2.x <pool_id>/<member_id>
//
// (req.ID populated, req.Identity empty) AND the modern Terraform 1.12+ form:
//
//	import { to = openstack_lb_member_v2.x identity = { pool_id = "..", member_id = ".." } }
//
// (req.ID empty, req.Identity populated).
func (r *lbMemberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Modern path: identity supplied via the Terraform 1.12+ import block.
	if req.ID == "" {
		var identity lbMemberV2IdentityModel

		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)

		if resp.Diagnostics.HasError() {
			return
		}

		if identity.PoolID.ValueString() == "" || identity.MemberID.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Invalid import identity",
				"Both 'pool_id' and 'member_id' must be supplied in the identity block.",
			)

			return
		}

		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), identity.PoolID.ValueString())...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), identity.MemberID.ValueString())...)

		// Mirror identity back so the post-import state has it populated.
		resp.Diagnostics.Append(resp.Identity.Set(ctx, identity)...)

		return
	}

	// Legacy path: composite ID string parsed the same way as the SDKv2 importer.
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Format must be <pool_id>/<member_id>, got %q", req.ID),
		)

		return
	}

	poolID := parts[0]
	memberID := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pool_id"), poolID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), memberID)...)

	// Populate identity from the parsed legacy ID so downstream code can rely on it.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, lbMemberV2IdentityModel{
		PoolID:   types.StringValue(poolID),
		MemberID: types.StringValue(memberID),
	})...)
}

// ----- helpers -----

// regionFromPlan returns the configured region falling back to the provider
// default when not set. Mirrors the SDKv2 GetRegion(d, config) helper.
func (r *lbMemberV2Resource) regionFromPlan(m lbMemberV2Model) string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}

	if r.config != nil {
		return r.config.Region
	}

	return ""
}

// populateModelFromAPI fetches the latest member from OpenStack and writes the
// resulting fields back onto m. Returns a non-nil error only when the resource
// vanished (404); other API errors are recorded on diags and signalled by
// HasError().
func (r *lbMemberV2Resource) populateModelFromAPI(ctx context.Context, m *lbMemberV2Model, lbClient lbV2Client, poolID, memberID, region string, diags *diag.Diagnostics) error {
	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if isNotFoundError(err) {
			// Caller treats nil-error+removed semantics; signal via the sentinel.
			return errLBMemberGone
		}

		diags.AddError(
			"Error reading member",
			fmt.Sprintf("%s: %s", memberID, err),
		)

		return err
	}

	log.Printf("[DEBUG] Retrieved member %s: %#v", memberID, member)

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

	tagsValue, tagDiags := types.SetValueFrom(ctx, types.StringType, member.Tags)
	diags.Append(tagDiags...)

	if tagDiags.HasError() {
		return errors.New("failed to convert tags")
	}

	m.Tags = tagsValue

	return nil
}

// errLBMemberGone is a sentinel signalling a 404 to the caller, which then
// removes the resource from state.
var errLBMemberGone = errors.New("lb member gone (404)")

// retryFuncFramework runs fn with retries for transient errors (HTTP 409, 5xx)
// until it succeeds, ctx is cancelled, or the timeout deadline expires. This
// replaces the SDKv2 retry.RetryContext / checkForRetryableError combination
// that lived in the original resource and avoids a continued dependency on
// terraform-plugin-sdk/v2/helper/retry from migrated files.
func retryFuncFramework(ctx context.Context, timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)

	const baseDelay = 2 * time.Second

	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if !isRetryableHTTPError(err) {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s waiting for retryable operation: %w", timeout, err)
		}

		delay := baseDelay
		if attempt > 4 {
			delay = 10 * time.Second
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func isRetryableHTTPError(err error) bool {
	var unexpected gophercloud.ErrUnexpectedResponseCode
	if !errors.As(err, &unexpected) {
		return false
	}

	switch unexpected.Actual {
	case http.StatusConflict,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}

	return false
}

func isNotFoundError(err error) bool {
	return gophercloud.ResponseCodeIs(err, http.StatusNotFound)
}

// lbV2Client is the minimal surface populateModelFromAPI needs — declared as a
// type alias so the helper signature doesn't pull gophercloud into every test
// helper. In practice this resolves to *gophercloud.ServiceClient.
type lbV2Client = *gophercloud.ServiceClient

// boolUseStateForUnknown is a tiny inline plan modifier — the framework's
// boolplanmodifier package only exposes RequiresReplace/RequiresReplaceIf
// historically; UseStateForUnknown was added in v1.4 (April 2024) but some
// downstream forks of this repo pin older versions, so we use a small local
// equivalent for safety.
func boolUseStateForUnknown() planmodifier.Bool {
	return useStateForUnknownBoolModifier{}
}

type useStateForUnknownBoolModifier struct{}

func (m useStateForUnknownBoolModifier) Description(_ context.Context) string {
	return "Once set, the value of this attribute in state will not change."
}

func (m useStateForUnknownBoolModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useStateForUnknownBoolModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if req.StateValue.IsNull() {
		return
	}

	if req.PlanValue.IsUnknown() {
		resp.PlanValue = req.StateValue
	}
}

// setUseStateForUnknown — same rationale, for set attributes (tags).
func setUseStateForUnknown() planmodifier.Set {
	return useStateForUnknownSetModifier{}
}

type useStateForUnknownSetModifier struct{}

func (m useStateForUnknownSetModifier) Description(_ context.Context) string {
	return "Once set, the value of this attribute in state will not change."
}

func (m useStateForUnknownSetModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useStateForUnknownSetModifier) PlanModifySet(_ context.Context, req planmodifier.SetRequest, resp *planmodifier.SetResponse) {
	if req.StateValue.IsNull() {
		return
	}

	if req.PlanValue.IsUnknown() {
		resp.PlanValue = req.StateValue
	}
}
