package openstack

import (
	"context"
	"log"
	"net/http"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &computeKeypairV2Resource{}
	_ resource.ResourceWithConfigure   = &computeKeypairV2Resource{}
	_ resource.ResourceWithImportState = &computeKeypairV2Resource{}
)

// NewComputeKeypairV2Resource is a helper constructor used when registering
// this resource in the provider's Resources() list.
func NewComputeKeypairV2Resource() resource.Resource {
	return &computeKeypairV2Resource{}
}

// computeKeypairV2Resource holds provider-level config surfaced via Configure.
type computeKeypairV2Resource struct {
	config *Config
}

// computeKeypairV2Model is the Terraform state/plan model.
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

// --------------------------------------------------------------------------
// resource.Resource
// --------------------------------------------------------------------------

func (r *computeKeypairV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compute_keypair_v2"
}

func (r *computeKeypairV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"public_key": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// --------------------------------------------------------------------------
// resource.ResourceWithConfigure
// --------------------------------------------------------------------------

func (r *computeKeypairV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			"Expected *Config, got a different type.",
		)

		return
	}

	r.config = config
}

// --------------------------------------------------------------------------
// resource.ResourceWithImportState
// --------------------------------------------------------------------------

func (r *computeKeypairV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --------------------------------------------------------------------------
// CRUD
// --------------------------------------------------------------------------

func (r *computeKeypairV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan computeKeypairV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromModel(plan.Region, r.config)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	userID := plan.UserID.ValueString()
	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	// Convert value_specs map from types.Map to map[string]string.
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
			PublicKey: plan.PublicKey.ValueString(),
			UserID:    userID,
		},
		valueSpecs,
	}

	log.Printf("[DEBUG] openstack_compute_keypair_v2 create options: %#v", createOpts)

	kp, err := keypairs.Create(ctx, computeClient, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create openstack_compute_keypair_v2",
			err.Error(),
		)

		return
	}

	// Set ID immediately so a partial-create is recoverable.
	plan.ID = types.StringValue(kp.Name)
	plan.Region = types.StringValue(region)
	// Private key is only present in the Create response.
	plan.PrivateKey = types.StringValue(kp.PrivateKey)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Populate remaining computed attributes via Read.
	r.readIntoState(ctx, kp.Name, userID, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the private key from the create response; Read doesn't return it.
	plan.PrivateKey = types.StringValue(kp.PrivateKey)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *computeKeypairV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state computeKeypairV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromModel(state.Region, r.config)
	userID := state.UserID.ValueString()
	originalID := state.ID.ValueString()

	r.readIntoState(ctx, originalID, userID, region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// readIntoState signals a 404 by clearing state.ID.
	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.State.RemoveResource(ctx)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is intentionally a no-op: all attributes have RequiresReplace, so
// any in-place update is impossible and Terraform would always plan a
// replacement instead.
func (r *computeKeypairV2Resource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *computeKeypairV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state computeKeypairV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromModel(state.Region, r.config)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	userID := state.UserID.ValueString()
	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	log.Printf("[DEBUG] User ID %s", userID)
	log.Printf("[DEBUG] Microversion %s", computeClient.Microversion)

	kpopts := keypairs.DeleteOpts{
		UserID: userID,
	}

	err = keypairs.Delete(ctx, computeClient, state.ID.ValueString(), kpopts).ExtractErr()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Already deleted.
			return
		}

		resp.Diagnostics.AddError(
			"Error deleting openstack_compute_keypair_v2",
			err.Error(),
		)
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// readIntoState fetches the keypair from the API and populates *model.
// On 404 it clears model.ID so the caller can detect resource removal.
func (r *computeKeypairV2Resource) readIntoState(
	ctx context.Context,
	id, userID, region string,
	model *computeKeypairV2Model,
	diagnostics *diag.Diagnostics,
) {
	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	log.Printf("[DEBUG] Microversion %s", computeClient.Microversion)

	kpopts := keypairs.GetOpts{
		UserID: userID,
	}

	kp, err := keypairs.Get(ctx, computeClient, id, kpopts).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			model.ID = types.StringNull()

			return
		}

		diagnostics.AddError(
			"Error retrieving openstack_compute_keypair_v2",
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved openstack_compute_keypair_v2 %s: %#v", id, kp)

	model.ID = types.StringValue(kp.Name)
	model.Name = types.StringValue(kp.Name)
	model.PublicKey = types.StringValue(kp.PublicKey)
	model.Fingerprint = types.StringValue(kp.Fingerprint)
	model.Region = types.StringValue(region)

	if userID != "" {
		model.UserID = types.StringValue(kp.UserID)
	}
}

// getRegionFromModel returns the region from the model if set, otherwise falls
// back to the provider-level region.
func getRegionFromModel(regionVal types.String, config *Config) string {
	if !regionVal.IsNull() && !regionVal.IsUnknown() && regionVal.ValueString() != "" {
		return regionVal.ValueString()
	}

	return config.Region
}
