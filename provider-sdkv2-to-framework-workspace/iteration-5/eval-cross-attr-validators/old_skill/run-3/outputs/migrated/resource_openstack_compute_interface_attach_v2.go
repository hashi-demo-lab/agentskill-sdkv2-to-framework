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
)

var (
	_ resource.Resource                = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithConfigure   = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithImportState = &computeInterfaceAttachV2Resource{}
)

// NewComputeInterfaceAttachV2Resource returns a new framework resource for
// openstack_compute_interface_attach_v2.
func NewComputeInterfaceAttachV2Resource() resource.Resource {
	return &computeInterfaceAttachV2Resource{}
}

type computeInterfaceAttachV2Resource struct {
	config *Config
}

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

			"port_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("network_id")),
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
					stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
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
					stringvalidator.ConflictsWith(path.MatchRoot("port_id")),
				},
			},
		},
		Blocks: map[string]schema.Block{
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
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

// regionFromPlan mirrors the SDKv2 GetRegion helper for typed framework state:
// returns the configured region if set, else falls back to the provider config.
func (r *computeInterfaceAttachV2Resource) regionFromPlan(region types.String) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	return r.config.Region
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

	region := r.regionFromPlan(plan.Region)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack compute client",
			err.Error(),
		)

		return
	}

	instanceID := plan.InstanceID.ValueString()

	var portID string
	if !plan.PortID.IsNull() && !plan.PortID.IsUnknown() {
		portID = plan.PortID.ValueString()
	}

	var networkID string
	if !plan.NetworkID.IsNull() && !plan.NetworkID.IsUnknown() {
		networkID = plan.NetworkID.ValueString()
	}

	if networkID == "" && portID == "" {
		resp.Diagnostics.AddError(
			"Missing required argument",
			"Must set one of network_id and port_id",
		)

		return
	}

	// For some odd reason the API takes an array of IPs, but you can only
	// have one element in the array.
	var fixedIPs []attachinterfaces.FixedIP
	if !plan.FixedIP.IsNull() && !plan.FixedIP.IsUnknown() && plan.FixedIP.ValueString() != "" {
		fixedIPs = append(fixedIPs, attachinterfaces.FixedIP{IPAddress: plan.FixedIP.ValueString()})
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

	if err := waitForState(
		ctx,
		computeInterfaceAttachV2AttachFunc(ctx, computeClient, instanceID, attachment.PortID),
		[]string{"ATTACHING"},
		[]string{"ATTACHED"},
		5*time.Second,
	); err != nil {
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
	plan.InstanceID = types.StringValue(instanceID)

	if len(attachment.FixedIPs) > 0 {
		plan.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
	} else {
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

	region := r.regionFromPlan(state.Region)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack compute client",
			err.Error(),
		)

		return
	}

	id := state.ID.ValueString()

	instanceID, attachmentID, err := parsePairedIDs(id, "openstack_compute_interface_attach_v2")
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid resource ID",
			err.Error(),
		)

		return
	}

	attachment, err := attachinterfaces.Get(ctx, computeClient, instanceID, attachmentID).Extract()
	if err != nil {
		// Mirrors SDKv2 CheckDeleted: a 404 means the resource is gone, so
		// drop it from state and let the next plan recreate.
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error retrieving openstack_compute_interface_attach_v2 %s", id),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved openstack_compute_interface_attach_v2 %s: %#v", id, attachment)

	state.InstanceID = types.StringValue(instanceID)
	state.PortID = types.StringValue(attachment.PortID)
	state.NetworkID = types.StringValue(attachment.NetID)
	state.Region = types.StringValue(region)

	if len(attachment.FixedIPs) > 0 {
		state.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
	} else {
		state.FixedIP = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: every attribute uses RequiresReplace, so updates always
// replace. The framework still requires the method to exist on the resource.
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

	region := r.regionFromPlan(state.Region)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack compute client",
			err.Error(),
		)

		return
	}

	id := state.ID.ValueString()

	instanceID, attachmentID, err := parsePairedIDs(id, "openstack_compute_interface_attach_v2")
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid resource ID",
			err.Error(),
		)

		return
	}

	if err := waitForState(
		ctx,
		computeInterfaceAttachV2DetachFunc(ctx, computeClient, instanceID, attachmentID),
		[]string{""},
		[]string{"DETACHED"},
		5*time.Second,
	); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error detaching openstack_compute_interface_attach_v2 %s", id),
			err.Error(),
		)

		return
	}
}

// waitForState is a minimal in-resource port of the SDKv2
// retry.StateChangeConf.WaitForStateContext semantics, so this migrated file
// does not need to import terraform-plugin-sdk/v2/helper/retry. The refresh
// callback is the same shape as retry.StateRefreshFunc — it returns
// (currentObj, currentState, error) — so the existing
// computeInterfaceAttachV2AttachFunc / DetachFunc work unchanged.
//
// The deadline is taken from ctx (the caller is expected to apply
// context.WithTimeout); on cancellation/deadline this returns ctx.Err().
func waitForState(
	ctx context.Context,
	refresh func() (any, string, error),
	pending []string,
	target []string,
	pollInterval time.Duration,
) error {
	contains := func(haystack []string, needle string) bool {
		for _, s := range haystack {
			if s == needle {
				return true
			}
		}

		return false
	}

	for {
		_, state, err := refresh()
		if err != nil {
			return err
		}

		if contains(target, state) {
			return nil
		}

		if !contains(pending, state) {
			return fmt.Errorf("unexpected state %q (pending=%v, target=%v)", state, pending, target)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func (r *computeInterfaceAttachV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
