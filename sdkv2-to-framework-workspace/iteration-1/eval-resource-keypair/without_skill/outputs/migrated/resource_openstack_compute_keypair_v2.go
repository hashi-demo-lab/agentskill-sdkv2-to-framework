package openstack

import (
	"context"
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

// NewComputeKeypairV2Resource is the factory function called by the provider.
func NewComputeKeypairV2Resource() resource.Resource {
	return &computeKeypairV2Resource{}
}

// computeKeypairV2Resource holds provider-level client configuration.
type computeKeypairV2Resource struct {
	config *Config
}

// computeKeypairV2Model is the typed Terraform state model for this resource.
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

// Metadata sets the Terraform resource type name.
func (r *computeKeypairV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compute_keypair_v2"
}

// Schema defines the resource schema.
// SDKv2 ForceNew attributes become RequiresReplace plan modifiers.
// Computed-only attributes use UseStateForUnknown to preserve values across refreshes.
func (r *computeKeypairV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			// Framework always requires an explicit "id" attribute.
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

			// private_key is only populated on Create; Sensitive keeps it out of logs.
			// UseStateForUnknown retains the value across refreshes so it is not lost.
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

// Configure receives the provider-level *Config and stores it for CRUD operations.
func (r *computeKeypairV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			"Expected *openstack.Config, got a different type. Please report this to the provider developers.",
		)

		return
	}

	r.config = config
}

// Create implements resource.Resource.
func (r *computeKeypairV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan computeKeypairV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := plan.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	userID := plan.UserID.ValueString()
	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	// Convert value_specs from types.Map to map[string]string.
	valueSpecs := make(map[string]string)
	if !plan.ValueSpecs.IsNull() && !plan.ValueSpecs.IsUnknown() {
		resp.Diagnostics.Append(plan.ValueSpecs.ElementsAs(ctx, &valueSpecs, false)...)
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
			"Unable to create openstack_compute_keypair_v2 "+name,
			err.Error(),
		)

		return
	}

	// The ID is the keypair name.
	plan.ID = types.StringValue(kp.Name)
	plan.Name = types.StringValue(kp.Name)
	plan.Region = types.StringValue(region)
	plan.PublicKey = types.StringValue(kp.PublicKey)
	plan.Fingerprint = types.StringValue(kp.Fingerprint)

	// Private key is only available in the Create response.
	plan.PrivateKey = types.StringValue(kp.PrivateKey)

	if userID != "" {
		plan.UserID = types.StringValue(kp.UserID)
	} else {
		plan.UserID = types.StringValue(kp.UserID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *computeKeypairV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state computeKeypairV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	userID := state.UserID.ValueString()
	if userID != "" {
		computeClient.Microversion = computeKeyPairV2UserIDMicroversion
	}

	log.Printf("[DEBUG] Microversion %s", computeClient.Microversion)

	kpopts := keypairs.GetOpts{UserID: userID}

	kp, err := keypairs.Get(ctx, computeClient, state.ID.ValueString(), kpopts).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			"Error retrieving openstack_compute_keypair_v2 "+state.ID.ValueString(),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved openstack_compute_keypair_v2 %s: %#v", state.ID.ValueString(), kp)

	state.Name = types.StringValue(kp.Name)
	state.PublicKey = types.StringValue(kp.PublicKey)
	state.Fingerprint = types.StringValue(kp.Fingerprint)
	state.Region = types.StringValue(region)

	if userID != "" {
		state.UserID = types.StringValue(kp.UserID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op because all attributes are RequiresReplace; the framework
// will destroy and recreate rather than calling Update. The method must exist
// to satisfy the resource.Resource interface.
func (r *computeKeypairV2Resource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

// Delete implements resource.Resource.
func (r *computeKeypairV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state computeKeypairV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

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

	kpopts := keypairs.DeleteOpts{UserID: userID}

	err = keypairs.Delete(ctx, computeClient, state.ID.ValueString(), kpopts).ExtractErr()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Already gone — not an error.
			return
		}

		resp.Diagnostics.AddError(
			"Error deleting openstack_compute_keypair_v2 "+state.ID.ValueString(),
			err.Error(),
		)
	}
}

// ImportState implements resource.ResourceWithImportState.
// The import ID is the keypair name (same as the resource ID).
func (r *computeKeypairV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
