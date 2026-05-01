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

var (
	_ resource.Resource                = &computeKeypairV2Resource{}
	_ resource.ResourceWithConfigure   = &computeKeypairV2Resource{}
	_ resource.ResourceWithImportState = &computeKeypairV2Resource{}
)

// NewComputeKeypairV2Resource is the resource constructor for the framework provider.
func NewComputeKeypairV2Resource() resource.Resource {
	return &computeKeypairV2Resource{}
}

type computeKeypairV2Resource struct {
	config *Config
}

// computeKeypairV2Model is the Terraform state model for openstack_compute_keypair_v2.
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

func (r *computeKeypairV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	// Build value_specs map from the plan.
	valueSpecs := map[string]string{}
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
			fmt.Sprintf("Unable to create openstack_compute_keypair_v2 %s", name),
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(kp.Name)
	plan.Region = types.StringValue(region)
	plan.Name = types.StringValue(kp.Name)
	plan.PublicKey = types.StringValue(kp.PublicKey)
	plan.Fingerprint = types.StringValue(kp.Fingerprint)
	plan.UserID = types.StringValue(userID)
	// Private key is only available in the Create response.
	plan.PrivateKey = types.StringValue(kp.PrivateKey)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

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

	kpopts := keypairs.GetOpts{
		UserID: userID,
	}

	kp, err := keypairs.Get(ctx, computeClient, state.ID.ValueString(), kpopts).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error retrieving openstack_compute_keypair_v2 %s", state.ID.ValueString()),
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

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is not implemented because all attributes are ForceNew (RequiresReplace).
// Terraform will always destroy and recreate instead of calling Update.
func (r *computeKeypairV2Resource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

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

	kpopts := keypairs.DeleteOpts{
		UserID: userID,
	}

	err = keypairs.Delete(ctx, computeClient, state.ID.ValueString(), kpopts).ExtractErr()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_compute_keypair_v2 %s", state.ID.ValueString()),
			err.Error(),
		)

		return
	}
}

func (r *computeKeypairV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
