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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
)

var mutexKV = mutexkv.NewMutexKV()

var (
	_ resource.Resource                = &vpcResource{}
	_ resource.ResourceWithConfigure   = &vpcResource{}
	_ resource.ResourceWithImportState = &vpcResource{}
)

func NewVPCResource() resource.Resource {
	return &vpcResource{}
}

type vpcResource struct {
	client *godo.Client
}

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
					stringvalidator.LengthAtLeast(1),
				},
			},
			"region": schema.StringAttribute{
				Required:    true,
				Description: "DigitalOcean region slug for the VPC's location",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A free-form description for the VPC",
				Validators: []validator.String{
					stringvalidator.LengthBetween(0, 255),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ip_range": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The range of IP addresses for the VPC in CIDR notation",
				Validators: []validator.String{
					cidrValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIfConfigured(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"urn": schema.StringAttribute{
				Computed:    true,
				Description: "The uniform resource name (URN) for the VPC",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether or not the VPC is the default one for the region",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "The date and time of when the VPC was created",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
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
	combined, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *config.CombinedConfig, got: %T", req.ProviderData),
		)
		return
	}
	r.client = combined.GodoClient()
}

func (r *vpcResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vpcResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

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

	// Prevent parallel creation of VPCs in the same region to protect
	// against race conditions in IP range assignment.
	key := fmt.Sprintf("resource_digitalocean_vpc/%s", region)
	mutexKV.Lock(key)
	defer mutexKV.Unlock(key)

	log.Printf("[DEBUG] VPC create request: %#v", vpcRequest)
	vpc, _, err := r.client.VPCs.Create(ctx, vpcRequest)
	if err != nil {
		resp.Diagnostics.AddError("Error creating VPC", err.Error())
		return
	}

	log.Printf("[INFO] VPC created, ID: %s", vpc.ID)

	plan.ID = types.StringValue(vpc.ID)
	plan.Name = types.StringValue(vpc.Name)
	plan.Region = types.StringValue(vpc.RegionSlug)
	plan.Description = types.StringValue(vpc.Description)
	plan.IPRange = types.StringValue(vpc.IPRange)
	plan.URN = types.StringValue(vpc.URN)
	plan.Default = types.BoolValue(vpc.Default)
	plan.CreatedAt = types.StringValue(vpc.CreatedAt.UTC().String())

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *vpcResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vpcResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	vpc, httpResp, err := r.client.VPCs.Get(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			log.Printf("[DEBUG] VPC (%s) was not found - removing from state", state.ID.ValueString())
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VPC", err.Error())
		return
	}

	state.ID = types.StringValue(vpc.ID)
	state.Name = types.StringValue(vpc.Name)
	state.Region = types.StringValue(vpc.RegionSlug)
	state.Description = types.StringValue(vpc.Description)
	state.IPRange = types.StringValue(vpc.IPRange)
	state.URN = types.StringValue(vpc.URN)
	state.Default = types.BoolValue(vpc.Default)
	state.CreatedAt = types.StringValue(vpc.CreatedAt.UTC().String())

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *vpcResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vpcResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.Name.Equal(state.Name) || !plan.Description.Equal(state.Description) {
		vpcUpdateRequest := &godo.VPCUpdateRequest{
			Name:        plan.Name.ValueString(),
			Description: plan.Description.ValueString(),
			Default:     godo.PtrTo(state.Default.ValueBool()),
		}
		_, _, err := r.client.VPCs.Update(ctx, state.ID.ValueString(), vpcUpdateRequest)
		if err != nil {
			resp.Diagnostics.AddError("Error updating VPC", err.Error())
			return
		}
		log.Printf("[INFO] Updated VPC")
	}

	// Refresh state by reading from API
	vpc, httpResp, err := r.client.VPCs.Get(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VPC after update", err.Error())
		return
	}

	plan.ID = types.StringValue(vpc.ID)
	plan.Name = types.StringValue(vpc.Name)
	plan.Region = types.StringValue(vpc.RegionSlug)
	plan.Description = types.StringValue(vpc.Description)
	plan.IPRange = types.StringValue(vpc.IPRange)
	plan.URN = types.StringValue(vpc.URN)
	plan.Default = types.BoolValue(vpc.Default)
	plan.CreatedAt = types.StringValue(vpc.CreatedAt.UTC().String())

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

	vpcID := state.ID.ValueString()

	deadline := time.Now().Add(deleteTimeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		httpResp, err := r.client.VPCs.Delete(ctx, vpcID)
		if err == nil {
			log.Printf("[INFO] VPC deleted, ID: %s", vpcID)
			return
		}

		// Retry if VPC still contains member resources to prevent race condition.
		if httpResp != nil && (httpResp.StatusCode == http.StatusForbidden || httpResp.StatusCode == http.StatusConflict) {
			if time.Now().After(deadline) {
				resp.Diagnostics.AddError("Error deleting VPC", fmt.Sprintf("timeout waiting for VPC %s to be deleted: %s", vpcID, err))
				return
			}
			select {
			case <-ctx.Done():
				resp.Diagnostics.AddError("Error deleting VPC", ctx.Err().Error())
				return
			case <-ticker.C:
				continue
			}
		}

		resp.Diagnostics.AddError("Error deleting VPC", err.Error())
		return
	}
}

func (r *vpcResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// cidrValidator is a custom string validator that checks whether the value is a valid CIDR notation.
type cidrValidator struct{}

func (v cidrValidator) Description(_ context.Context) string {
	return "must be a valid CIDR notation IP range (e.g., 10.0.0.0/8)"
}

func (v cidrValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v cidrValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	_, _, err := net.ParseCIDR(val)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid CIDR notation",
			fmt.Sprintf("%q is not a valid CIDR notation IP range: %s", val, err),
		)
	}
}
