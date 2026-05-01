package openstack

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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

func NewMemberV2Resource() resource.Resource {
	return &memberV2Resource{}
}

type memberV2Resource struct {
	config *Config
}

// memberV2Model is the resource state/plan model.
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

// memberV2IdentityModel is the framework identity model. Matches the
// identityschema attributes one-for-one via tfsdk tags.
type memberV2IdentityModel struct {
	PoolID   types.String `tfsdk:"pool_id"`
	MemberID types.String `tfsdk:"member_id"`
}

func (r *memberV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_member_v2"
}

func (r *memberV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// req.ProviderData is nil on the early ValidateResourceConfig RPC; guard
	// against nil before type-asserting.
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected ProviderData type",
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
			},
			"pool_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"backup": schema.BoolAttribute{
				Optional: true,
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
			// Use Block form to preserve the existing `timeouts { ... }`
			// HCL syntax that practitioners' configs already use.
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// IdentitySchema declares the framework identity schema. Both segments of
// the composite ID (`pool_id` and `member_id`) are required for import so
// that practitioners on Terraform 1.12+ can write
//
//	import {
//	  to = openstack_lb_member_v2.foo
//	  identity = { pool_id = "...", member_id = "..." }
//	}
func (r *memberV2Resource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
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

// setIdentity writes the identity payload alongside state so practitioners
// see the typed identity at plan/apply time.
func (r *memberV2Resource) setIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, poolID, memberID string) diag.Diagnostics {
	if identity == nil {
		return nil
	}
	id := memberV2IdentityModel{
		PoolID:   types.StringValue(poolID),
		MemberID: types.StringValue(memberID),
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

	region := getRegionFromString(plan.Region, r.config)
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

	// Only set the weight when the practitioner provided it explicitly,
	// otherwise the API would default everyone to 0.
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		w := int(plan.Weight.ValueInt64())
		createOpts.Weight = &w
	}

	if !plan.MonitorAddress.IsNull() && plan.MonitorAddress.ValueString() != "" {
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
		resp.Diagnostics.AddError("Unable to retrieve parent pool", fmt.Sprintf("%s: %s", poolID, err))
		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), createTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool", err.Error())
		return
	}

	log.Printf("[DEBUG] Attempting to create member")

	var member *pools.Member
	err = retryFunc(ctx, createTimeout, func() error {
		var innerErr error
		member, innerErr = pools.CreateMember(ctx, lbClient, poolID, createOpts).Extract()
		return innerErr
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

	// Hydrate computed fields from the API response so that the round-trip
	// matches what Read would produce.
	r.flattenMember(member, &plan)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setIdentity(ctx, resp.Identity, poolID, member.ID)...)
}

func (r *memberV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state memberV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromString(state.Region, r.config)
	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if isResourceGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading member", err.Error())
		return
	}

	log.Printf("[DEBUG] Retrieved member %s: %#v", memberID, member)

	state.Region = types.StringValue(region)
	r.flattenMember(member, &state)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Refresh identity in case state was hand-imported via the legacy path.
	resp.Diagnostics.Append(r.setIdentity(ctx, resp.Identity, poolID, memberID)...)
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

	region := getRegionFromString(plan.Region, r.config)
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

	poolID := plan.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve parent pool", fmt.Sprintf("%s: %s", poolID, err))
		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Unable to retrieve member", fmt.Sprintf("%s: %s", memberID, err))
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

	err = retryFunc(ctx, updateTimeout, func() error {
		_, innerErr := pools.UpdateMember(ctx, lbClient, poolID, memberID, updateOpts).Extract()
		return innerErr
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update member", fmt.Sprintf("%s: %s", memberID, err))
		return
	}

	if err := waitForLBV2Member(ctx, lbClient, parentPool, member, "ACTIVE", getLbPendingStatuses(), updateTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for member to become ACTIVE", err.Error())
		return
	}

	// Re-read to reflect API state.
	updated, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error reading member after update", err.Error())
		return
	}

	plan.ID = types.StringValue(memberID)
	plan.Region = types.StringValue(region)
	r.flattenMember(updated, &plan)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.setIdentity(ctx, resp.Identity, poolID, memberID)...)
}

func (r *memberV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Reads MUST come from req.State on Delete; req.Plan is null.
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

	region := getRegionFromString(state.Region, r.config)
	lbClient, err := r.config.LoadBalancerV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	poolID := state.PoolID.ValueString()
	memberID := state.ID.ValueString()

	parentPool, err := pools.Get(ctx, lbClient, poolID).Extract()
	if err != nil {
		if isResourceGone(err) {
			return
		}
		resp.Diagnostics.AddError("Unable to retrieve parent pool", fmt.Sprintf("%s: %s", poolID, err))
		return
	}

	member, err := pools.GetMember(ctx, lbClient, poolID, memberID).Extract()
	if err != nil {
		if isResourceGone(err) {
			return
		}
		resp.Diagnostics.AddError("Unable to retrieve member", err.Error())
		return
	}

	if err := waitForLBV2Pool(ctx, lbClient, parentPool, "ACTIVE", getLbPendingStatuses(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError("Error waiting for parent pool", err.Error())
		return
	}

	log.Printf("[DEBUG] Attempting to delete member %s", memberID)

	err = retryFunc(ctx, deleteTimeout, func() error {
		return pools.DeleteMember(ctx, lbClient, poolID, memberID).ExtractErr()
	})
	if err != nil {
		if isResourceGone(err) {
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

// ImportState supports BOTH import paths so the migration is invisible to
// existing CLI users while enabling the new identity-aware HCL flow:
//
//  1. Legacy CLI:  `terraform import openstack_lb_member_v2.foo <pool>/<member>`
//     — req.ID is non-empty, req.Identity is empty. Parse the slash-delimited
//     composite the same way the SDKv2 importer did.
//  2. Modern HCL (Terraform 1.12+):
//     import { to = ...  identity = { pool_id = "...", member_id = "..." } }
//     — req.ID is empty, req.Identity is populated.
//
// The two are mutually exclusive; branch on req.ID.
func (r *memberV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Modern path — read the typed identity directly. Using
	// ImportStatePassthroughWithIdentity keeps state and identity in lock-step
	// for each attribute pair.
	if req.ID == "" {
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("pool_id"), path.Root("pool_id"), req, resp)
		// member_id in identity → the resource's state ID attribute.
		resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("member_id"), req, resp)
		return
	}

	// Legacy path — parse `<pool_id>/<member_id>` exactly as the SDKv2
	// importer did so existing CLI muscle memory keeps working.
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
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
	if resp.Diagnostics.HasError() {
		return
	}

	// Mirror the parsed pieces into identity so the resource is identity-aware
	// even when imported via the legacy CLI form.
	resp.Diagnostics.Append(r.setIdentity(ctx, resp.Identity, poolID, memberID)...)
}

// flattenMember copies API fields onto the model. The `Region` and
// `Timeouts` fields are populated by the caller because they're not part of
// the API payload.
func (r *memberV2Resource) flattenMember(m *pools.Member, dst *memberV2Model) {
	dst.Name = types.StringValue(m.Name)
	dst.Weight = types.Int64Value(int64(m.Weight))
	dst.AdminStateUp = types.BoolValue(m.AdminStateUp)
	dst.TenantID = types.StringValue(m.ProjectID)
	dst.SubnetID = types.StringValue(m.SubnetID)
	dst.Address = types.StringValue(m.Address)
	dst.ProtocolPort = types.Int64Value(int64(m.ProtocolPort))
	dst.MonitorAddress = types.StringValue(m.MonitorAddress)
	dst.MonitorPort = types.Int64Value(int64(m.MonitorPort))
	dst.Backup = types.BoolValue(m.Backup)

	tags, _ := types.SetValueFrom(context.Background(), types.StringType, m.Tags)
	dst.Tags = tags
}
