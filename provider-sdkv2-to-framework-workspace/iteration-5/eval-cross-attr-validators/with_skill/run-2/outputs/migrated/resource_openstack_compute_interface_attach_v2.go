package openstack

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/attachinterfaces"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	// retry.StateChangeConf still lives in the SDKv2 helper package; it has
	// no framework-native replacement and is safe to keep importing during
	// (and after) a partial migration. The shared compute_interface_attach_v2.go
	// helper already depends on it.
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Compile-time interface checks.
var (
	_ resource.Resource                   = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithConfigure      = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithImportState    = &computeInterfaceAttachV2Resource{}
)

// NewComputeInterfaceAttachV2Resource is the resource constructor wired into
// the provider's Resources() slice.
func NewComputeInterfaceAttachV2Resource() resource.Resource {
	return &computeInterfaceAttachV2Resource{}
}

type computeInterfaceAttachV2Resource struct {
	config *Config
}

// computeInterfaceAttachV2Model is the typed plan/state model.
type computeInterfaceAttachV2Model struct {
	ID         types.String   `tfsdk:"id"`
	Region     types.String   `tfsdk:"region"`
	PortID     types.String   `tfsdk:"port_id"`
	NetworkID  types.String   `tfsdk:"network_id"`
	InstanceID types.String   `tfsdk:"instance_id"`
	FixedIP    types.String   `tfsdk:"fixed_ip"`
	Timeouts   timeouts.Value `tfsdk:"timeouts"`
}

func (r *computeInterfaceAttachV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compute_interface_attach_v2"
}

func (r *computeInterfaceAttachV2Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

			// port_id and network_id are mutually exclusive (SDKv2 ConflictsWith).
			// fixed_ip conflicts with port_id (you can only set a fixed IP when
			// supplying a network_id). Each constraint is expressed as a
			// per-attribute Validator with a path.Expression — the framework's
			// idiomatic replacement for SDKv2 ConflictsWith.
			"port_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(
						path.MatchRoot("network_id"),
						path.MatchRoot("fixed_ip"),
					),
				},
			},

			"network_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(
						path.MatchRoot("port_id"),
					),
				},
			},

			"instance_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"fixed_ip": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(
						path.MatchRoot("port_id"),
					),
				},
			},
		},
		Blocks: map[string]schema.Block{
			// Preserve the SDKv2 `timeouts { ... }` block syntax for
			// backward-compatibility with existing practitioner configs.
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *computeInterfaceAttachV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)
		return
	}

	r.config = cfg
}

func (r *computeInterfaceAttachV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan computeInterfaceAttachV2Model
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

	region := getRegionFromModel(plan.Region, r.config)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack compute client",
			err.Error(),
		)
		return
	}

	instanceID := plan.InstanceID.ValueString()
	portID := plan.PortID.ValueString()
	networkID := plan.NetworkID.ValueString()

	// The cross-attribute validators above guarantee port_id/network_id/fixed_ip
	// don't collide. We still need the "at least one of port_id or network_id"
	// check at runtime — if you wanted that to be a config-time error too, swap
	// in a resourcevalidator.AtLeastOneOf via ConfigValidators.
	if networkID == "" && portID == "" {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"Must set one of network_id and port_id",
		)
		return
	}

	// For some odd reason the API takes an array of IPs, but you can only have
	// one element in the array.
	var fixedIPs []attachinterfaces.FixedIP
	if v := plan.FixedIP.ValueString(); v != "" {
		fixedIPs = append(fixedIPs, attachinterfaces.FixedIP{IPAddress: v})
	}

	attachOpts := attachinterfaces.CreateOpts{
		PortID:    portID,
		NetworkID: networkID,
		FixedIPs:  fixedIPs,
	}

	log.Printf("[DEBUG] openstack_compute_interface_attach_v2 attach options: %#v", attachOpts)

	attachment, err := attachinterfaces.Create(ctx, computeClient, instanceID, attachOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_compute_interface_attach_v2",
			err.Error(),
		)
		return
	}

	stateConf := &retry.StateChangeConf{
		Pending:    []string{"ATTACHING"},
		Target:     []string{"ATTACHED"},
		Refresh:    computeInterfaceAttachV2AttachFunc(ctx, computeClient, instanceID, attachment.PortID),
		Timeout:    createTimeout,
		Delay:      0,
		MinTimeout: 5 * time.Second,
	}

	if _, err = stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error creating openstack_compute_interface_attach_v2 %s", instanceID),
			err.Error(),
		)
		return
	}

	// Use the instance ID and attachment ID as the resource ID.
	id := fmt.Sprintf("%s/%s", instanceID, attachment.PortID)
	log.Printf("[DEBUG] Created openstack_compute_interface_attach_v2 %s: %#v", id, attachment)

	plan.ID = types.StringValue(id)
	plan.Region = types.StringValue(region)
	plan.PortID = types.StringValue(attachment.PortID)
	plan.NetworkID = types.StringValue(attachment.NetID)
	if len(attachment.FixedIPs) > 0 {
		plan.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
	} else if plan.FixedIP.IsUnknown() {
		plan.FixedIP = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *computeInterfaceAttachV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state computeInterfaceAttachV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromModel(state.Region, r.config)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack compute client",
			err.Error(),
		)
		return
	}

	instanceID, attachmentID, err := parsePairedIDs(state.ID.ValueString(), "openstack_compute_interface_attach_v2")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing openstack_compute_interface_attach_v2 ID",
			err.Error(),
		)
		return
	}

	attachment, err := attachinterfaces.Get(ctx, computeClient, instanceID, attachmentID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error retrieving openstack_compute_interface_attach_v2 %s", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved openstack_compute_interface_attach_v2 %s: %#v", state.ID.ValueString(), attachment)

	if len(attachment.FixedIPs) > 0 {
		state.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
	} else {
		state.FixedIP = types.StringNull()
	}

	state.InstanceID = types.StringValue(instanceID)
	state.PortID = types.StringValue(attachment.PortID)
	state.NetworkID = types.StringValue(attachment.NetID)
	state.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is intentionally empty: every non-id attribute is ForceNew /
// RequiresReplace, so Terraform never plans an update — but the framework
// still requires the method to satisfy resource.Resource.
func (r *computeInterfaceAttachV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan computeInterfaceAttachV2Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *computeInterfaceAttachV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state computeInterfaceAttachV2Model
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

	region := getRegionFromModel(state.Region, r.config)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack compute client",
			err.Error(),
		)
		return
	}

	instanceID, attachmentID, err := parsePairedIDs(state.ID.ValueString(), "openstack_compute_interface_attach_v2")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing openstack_compute_interface_attach_v2 ID",
			err.Error(),
		)
		return
	}

	stateConf := &retry.StateChangeConf{
		Pending:    []string{""},
		Target:     []string{"DETACHED"},
		Refresh:    computeInterfaceAttachV2DetachFunc(ctx, computeClient, instanceID, attachmentID),
		Timeout:    deleteTimeout,
		Delay:      0,
		MinTimeout: 5 * time.Second,
	}

	if _, err = stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error detaching openstack_compute_interface_attach_v2 %s", state.ID.ValueString()),
			err.Error(),
		)
		return
	}
}

func (r *computeInterfaceAttachV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// getRegionFromModel mirrors the SDKv2 GetRegion helper but takes a
// types.String from the typed model rather than a *schema.ResourceData.
// If the resource explicitly set a region, use it; otherwise fall back to
// the provider-level region.
func getRegionFromModel(region types.String, cfg *Config) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}
	return cfg.Region
}
