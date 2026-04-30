package openstack

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface checks.
var (
	_ resource.Resource                = &computeKeypairV2Resource{}
	_ resource.ResourceWithConfigure   = &computeKeypairV2Resource{}
	_ resource.ResourceWithImportState = &computeKeypairV2Resource{}
)

// NewComputeKeypairV2Resource returns a new framework-based
// openstack_compute_keypair_v2 resource.
func NewComputeKeypairV2Resource() resource.Resource {
	return &computeKeypairV2Resource{}
}

type computeKeypairV2Resource struct {
	config *Config
}

// computeKeypairV2Model maps Terraform state for openstack_compute_keypair_v2.
type computeKeypairV2Model struct {
	ID          types.String `tfsdk:"id"`
	Region      types.String `tfsdk:"region"`
	Name        types.String `tfsdk:"name"`
	PublicKey   types.String `tfsdk:"public_key"`
	ValueSpecs  types.Map    `tfsdk:"value_specs"`
	PrivateKey  types.String `tfsdk:"private_key"`
	Fingerprint types.String `tfsdk:"fingerprint"`
	UserID      types.String `tfsdk:"user_id"`
}

func (r *computeKeypairV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compute_keypair_v2"
}

func (r *computeKeypairV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			// id is implicitly required by Terraform; declared here so we
			// can wire UseStateForUnknown.
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
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"public_key": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"value_specs": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},

			// computed-only
			"private_key": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"fingerprint": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"user_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *computeKeypairV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.config = config
}

// regionFor returns the configured region for the resource or falls back to
// the provider-level region — mirrors the SDKv2 GetRegion helper.
func (r *computeKeypairV2Resource) regionFor(region types.String) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	return r.config.Region
}

func (r *computeKeypairV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan computeKeypairV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(plan.Region)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	userID := ""
	if !plan.UserID.IsNull() && !plan.UserID.IsUnknown() {
		userID = plan.UserID.ValueString()
	}

	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	publicKey := ""
	if !plan.PublicKey.IsNull() && !plan.PublicKey.IsUnknown() {
		publicKey = plan.PublicKey.ValueString()
	}

	valueSpecs := make(map[string]string)
	if !plan.ValueSpecs.IsNull() && !plan.ValueSpecs.IsUnknown() {
		diags := plan.ValueSpecs.ElementsAs(ctx, &valueSpecs, false)
		resp.Diagnostics.Append(diags...)

		if resp.Diagnostics.HasError() {
			return
		}
	}

	name := plan.Name.ValueString()
	createOpts := ComputeKeyPairV2CreateOpts{
		keypairs.CreateOpts{
			Name:      name,
			PublicKey: publicKey,
			UserID:    userID,
		},
		valueSpecs,
	}

	log.Printf("[DEBUG] openstack_compute_keypair_v2 create options: %#v", createOpts)

	kp, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Unable to create openstack_compute_keypair_v2 %s", name),
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(kp.Name)
	plan.Name = types.StringValue(kp.Name)
	plan.PublicKey = types.StringValue(kp.PublicKey)
	plan.Fingerprint = types.StringValue(kp.Fingerprint)
	plan.PrivateKey = types.StringValue(kp.PrivateKey)
	plan.Region = types.StringValue(region)

	if userID != "" {
		// SDKv2 wrote back the user_id from the keypair response when the
		// user-id microversion was active.
		plan.UserID = types.StringValue(kp.UserID)
	} else if plan.UserID.IsUnknown() {
		// No user_id was supplied; populate the computed slot with null so
		// state stays consistent.
		plan.UserID = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *computeKeypairV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state computeKeypairV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(state.Region)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	userID := ""
	if !state.UserID.IsNull() && !state.UserID.IsUnknown() {
		userID = state.UserID.ValueString()
	}

	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	log.Printf("[DEBUG] Microversion %s", computeClient.Microversion)

	kpopts := keypairs.GetOpts{
		UserID: userID,
	}

	id := state.ID.ValueString()

	kp, err := keypairs.Get(ctx, computeClient, id, kpopts).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Resource is gone — clear it from state so Terraform plans a
			// recreate, mirroring CheckDeleted's d.SetId("") behaviour.
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error retrieving openstack_compute_keypair_v2 %s", id),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved openstack_compute_keypair_v2 %s: %#v", id, kp)

	state.Name = types.StringValue(kp.Name)
	state.PublicKey = types.StringValue(kp.PublicKey)
	state.Fingerprint = types.StringValue(kp.Fingerprint)
	state.Region = types.StringValue(region)

	if userID != "" {
		state.UserID = types.StringValue(kp.UserID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op for this resource — every configurable attribute is
// ForceNew (translated to RequiresReplace plan modifiers above). The framework
// still requires the method to satisfy resource.Resource.
func (r *computeKeypairV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan computeKeypairV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *computeKeypairV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state computeKeypairV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(state.Region)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	userID := ""
	if !state.UserID.IsNull() && !state.UserID.IsUnknown() {
		userID = state.UserID.ValueString()
	}

	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	log.Printf("[DEBUG] User ID %s", userID)
	log.Printf("[DEBUG] Microversion %s", computeClient.Microversion)

	kpopts := keypairs.DeleteOpts{
		UserID: userID,
	}

	id := state.ID.ValueString()

	if err := keypairs.Delete(ctx, computeClient, id, kpopts).ExtractErr(); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Already gone — treat as success.
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_compute_keypair_v2 %s", id),
			err.Error(),
		)

		return
	}
}

func (r *computeKeypairV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
