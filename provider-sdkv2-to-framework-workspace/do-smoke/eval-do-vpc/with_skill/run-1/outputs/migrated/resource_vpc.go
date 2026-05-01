package vpc

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/internal/mutexkv"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var mutexKV = mutexkv.NewMutexKV()

// Compile-time interface assertions.
var (
	_ resource.Resource                = &vpcResource{}
	_ resource.ResourceWithConfigure   = &vpcResource{}
	_ resource.ResourceWithImportState = &vpcResource{}
)

// NewVPCResource returns a new framework-based VPC resource.
func NewVPCResource() resource.Resource {
	return &vpcResource{}
}

type vpcResource struct {
	config *config.CombinedConfig
}

// vpcResourceModel mirrors the schema attributes plus the timeouts block.
type vpcResourceModel struct {
	ID          types.String   `tfsdk:"id"`
	Name        types.String   `tfsdk:"name"`
	Region      types.String   `tfsdk:"region"`
	Description types.String   `tfsdk:"description"`
	IPRange     types.String   `tfsdk:"ip_range"`
	URN         types.String   `tfsdk:"urn"`
	Default     types.Bool     `tfsdk:"default"`
	CreatedAt   types.String   `tfsdk:"created_at"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

// nonZeroStringValidator ports SDKv2 validation.NoZeroValues for strings:
// rejects the empty string when explicitly set.
type nonZeroStringValidator struct{}

func (nonZeroStringValidator) Description(_ context.Context) string {
	return "must not be empty"
}

func (nonZeroStringValidator) MarkdownDescription(_ context.Context) string {
	return "must not be empty"
}

func (nonZeroStringValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if req.ConfigValue.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"value must not be empty",
			"the empty string is not a valid value for this attribute",
		)
	}
}

// cidrValidator ports SDKv2 validation.IsCIDR. Kept as a small bespoke
// validator so the schema can use plain types.String; if the API ever
// renormalised CIDRs (it does not today), switching to
// cidrtypes.IPv4Prefix from terraform-plugin-framework-nettypes would be
// the cleaner long-term fix.
type cidrValidator struct{}

func (cidrValidator) Description(_ context.Context) string {
	return "must be a valid CIDR block"
}

func (cidrValidator) MarkdownDescription(_ context.Context) string {
	return "must be a valid CIDR block"
}

func (cidrValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	if _, _, err := net.ParseCIDR(s); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"invalid CIDR",
			fmt.Sprintf("expected a valid CIDR block, got %q: %s", s, err),
		)
	}
}

func (r *vpcResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpc"
}

func (r *vpcResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the VPC",
				Validators: []validator.String{
					nonZeroStringValidator{},
				},
			},
			"region": schema.StringAttribute{
				Required:    true,
				Description: "DigitalOcean region slug for the VPC's location",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					nonZeroStringValidator{},
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "A free-form description for the VPC",
				Validators: []validator.String{
					stringvalidator.LengthBetween(0, 255),
				},
			},
			"ip_range": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The range of IP addresses for the VPC in CIDR notation",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					cidrValidator{},
				},
			},

			// Computed attributes
			"urn": schema.StringAttribute{
				Computed:    true,
				Description: "The uniform resource name (URN) for the VPC",
			},
			"default": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether or not the VPC is the default one for the region",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "The date and time of when the VPC was created",
			},
		},
		Blocks: map[string]schema.Block{
			// Preserve SDKv2 block syntax for the timeouts block. Only Delete
			// was previously configurable; we keep that scope.
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Delete: true,
			}),
		},
	}
}

func (r *vpcResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *config.CombinedConfig, got %T", req.ProviderData),
		)
		return
	}
	r.config = cfg
}

func (r *vpcResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vpcResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.config.GodoClient()

	region := plan.Region.ValueString()
	vpcRequest := &godo.VPCCreateRequest{
		Name:       plan.Name.ValueString(),
		RegionSlug: region,
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		vpcRequest.Description = plan.Description.ValueString()
	}
	if !plan.IPRange.IsNull() && !plan.IPRange.IsUnknown() {
		vpcRequest.IPRange = plan.IPRange.ValueString()
	}

	// Prevent parallel creation of VPCs in the same region to protect against
	// race conditions in IP range assignment.
	key := fmt.Sprintf("resource_digitalocean_vpc/%s", region)
	mutexKV.Lock(key)
	defer mutexKV.Unlock(key)

	log.Printf("[DEBUG] VPC create request: %#v", vpcRequest)
	vpc, _, err := client.VPCs.Create(ctx, vpcRequest)
	if err != nil {
		resp.Diagnostics.AddError("Error creating VPC", err.Error())
		return
	}
	log.Printf("[INFO] VPC created, ID: %s", vpc.ID)

	plan.ID = types.StringValue(vpc.ID)

	// Populate computed/refreshed fields from the API response so we don't
	// emit an unknown-value diagnostic after Create.
	r.flatten(vpc, &plan)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *vpcResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vpcResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.config.GodoClient()

	vpc, httpResp, err := client.VPCs.Get(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			log.Printf("[DEBUG] VPC (%s) was not found - removing from state", state.ID.ValueString())
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VPC", err.Error())
		return
	}

	r.flatten(vpc, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *vpcResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vpcResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state vpcResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.config.GodoClient()

	// Mirror SDKv2 d.HasChanges("name", "description") gate. If neither
	// changed, skip the API call entirely.
	if !plan.Name.Equal(state.Name) || !plan.Description.Equal(state.Description) {
		// Preserve SDKv2 behaviour: pass the prior `default` value (a
		// server-computed attribute) back to the API on update. Default is
		// only readable from prior state, not the plan, since it's Computed
		// and won't change here.
		defaultVal := state.Default.ValueBool()
		vpcUpdateRequest := &godo.VPCUpdateRequest{
			Name:        plan.Name.ValueString(),
			Description: plan.Description.ValueString(),
			Default:     godo.PtrTo(defaultVal),
		}
		_, _, err := client.VPCs.Update(ctx, plan.ID.ValueString(), vpcUpdateRequest)
		if err != nil {
			resp.Diagnostics.AddError("Error updating VPC", err.Error())
			return
		}
		log.Printf("[INFO] Updated VPC")
	}

	// Re-read to pick up any server-side normalisation, then write to state.
	vpc, _, err := client.VPCs.Get(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading VPC after update", err.Error())
		return
	}
	r.flatten(vpc, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *vpcResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vpcResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 2*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	client := r.config.GodoClient()
	vpcID := state.ID.ValueString()

	// Replaces SDKv2 retry.RetryContext: poll deletion until either the API
	// reports success or the context deadline expires. The original logic
	// retried only on 403/409 (VPC contains member resources) — we keep
	// that, treating any other error as a non-retryable failure.
	const pollInterval = 5 * time.Second
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		httpResp, err := client.VPCs.Delete(ctx, vpcID)
		if err == nil {
			log.Printf("[INFO] VPC deleted, ID: %s", vpcID)
			return
		}

		retryable := httpResp != nil &&
			(httpResp.StatusCode == http.StatusForbidden ||
				httpResp.StatusCode == http.StatusConflict)
		if !retryable {
			resp.Diagnostics.AddError("Error deleting VPC", err.Error())
			return
		}

		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError(
				"Error deleting VPC",
				fmt.Sprintf("timed out waiting for VPC %s to delete: %s", vpcID, err),
			)
			return
		case <-ticker.C:
			// retry
		}
	}
}

func (r *vpcResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// flatten copies fields from the godo VPC response into the model. Always
// trusts the server for both user-set and computed fields after a successful
// API round-trip.
func (r *vpcResource) flatten(vpc *godo.VPC, m *vpcResourceModel) {
	m.ID = types.StringValue(vpc.ID)
	m.Name = types.StringValue(vpc.Name)
	m.Region = types.StringValue(vpc.RegionSlug)
	m.Description = types.StringValue(vpc.Description)
	m.IPRange = types.StringValue(vpc.IPRange)
	m.URN = types.StringValue(vpc.URN)
	m.Default = types.BoolValue(vpc.Default)
	m.CreatedAt = types.StringValue(vpc.CreatedAt.UTC().String())
}
