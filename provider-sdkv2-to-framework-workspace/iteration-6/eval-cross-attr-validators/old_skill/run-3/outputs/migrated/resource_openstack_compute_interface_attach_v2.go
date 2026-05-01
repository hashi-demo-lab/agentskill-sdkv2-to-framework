package openstack

import (
	"context"
	"errors"
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

// NewComputeInterfaceAttachV2Resource returns the framework-based
// implementation of the openstack_compute_interface_attach_v2 resource.
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
			fmt.Sprintf("expected *openstack.Config, got %T", req.ProviderData),
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

	region := plan.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

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

	if networkID == "" && portID == "" {
		resp.Diagnostics.AddError(
			"Invalid configuration for openstack_compute_interface_attach_v2",
			"Must set one of network_id and port_id",
		)

		return
	}

	// For some odd reason the API takes an array of IPs, but you can only have one element in the array.
	var fixedIPs []attachinterfaces.FixedIP

	if !plan.FixedIP.IsNull() && plan.FixedIP.ValueString() != "" {
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
			"Error attaching openstack_compute_interface_attach_v2",
			err.Error(),
		)

		return
	}

	if err := waitForComputeInterfaceAttachV2Attach(ctx, computeClient, instanceID, attachment.PortID, createTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error creating openstack_compute_interface_attach_v2 %s", instanceID),
			err.Error(),
		)

		return
	}

	// Use the instance ID and attachment ID as the resource ID.
	id := fmt.Sprintf("%s/%s", instanceID, attachment.PortID)

	log.Printf("[DEBUG] Created openstack_compute_interface_attach_v2 %s: %#v", id, attachment)

	// Re-fetch the latest attachment so we populate every computed attribute
	// from the server, not just what we sent.
	final, err := attachinterfaces.Get(ctx, computeClient, instanceID, attachment.PortID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading openstack_compute_interface_attach_v2 after create",
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(id)
	plan.Region = types.StringValue(region)
	plan.InstanceID = types.StringValue(instanceID)
	plan.PortID = types.StringValue(final.PortID)
	plan.NetworkID = types.StringValue(final.NetID)

	if len(final.FixedIPs) > 0 {
		plan.FixedIP = types.StringValue(final.FixedIPs[0].IPAddress)
	} else {
		plan.FixedIP = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *computeInterfaceAttachV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state computeInterfaceAttachV2Model

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

	state.Region = types.StringValue(region)
	state.InstanceID = types.StringValue(instanceID)
	state.PortID = types.StringValue(attachment.PortID)
	state.NetworkID = types.StringValue(attachment.NetID)

	if len(attachment.FixedIPs) > 0 {
		state.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
	} else {
		state.FixedIP = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *computeInterfaceAttachV2Resource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All attributes RequiresReplace, so Update is never called in practice.
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

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

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

	if err := waitForComputeInterfaceAttachV2Detach(ctx, computeClient, instanceID, attachmentID, deleteTimeout); err != nil {
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

// waitForComputeInterfaceAttachV2Attach polls the API until the interface
// attachment reaches the ATTACHED state or the supplied timeout elapses.
func waitForComputeInterfaceAttachV2Attach(ctx context.Context, computeClient *gophercloud.ServiceClient, instanceID, attachmentID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		attachment, err := attachinterfaces.Get(ctx, computeClient, instanceID, attachmentID).Extract()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				// Still pending; the attachment hasn't shown up yet.
			} else {
				return err
			}
		} else if attachment != nil {
			return nil
		}

		if time.Now().After(deadline) {
			return errors.New("timeout waiting for interface attachment to become ATTACHED")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// waitForComputeInterfaceAttachV2Detach issues the detach request and polls
// until the attachment is gone or the supplied timeout elapses.
func waitForComputeInterfaceAttachV2Detach(ctx context.Context, computeClient *gophercloud.ServiceClient, instanceID, attachmentID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		log.Printf("[DEBUG] Attempting to detach openstack_compute_interface_attach_v2 %s from instance %s",
			attachmentID, instanceID)

		_, err := attachinterfaces.Get(ctx, computeClient, instanceID, attachmentID).Extract()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return nil
			}

			return err
		}

		err = attachinterfaces.Delete(ctx, computeClient, instanceID, attachmentID).ExtractErr()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return nil
			}

			if !gophercloud.ResponseCodeIs(err, http.StatusBadRequest) {
				return err
			}
		}

		log.Printf("[DEBUG] openstack_compute_interface_attach_v2 %s is still active.", attachmentID)

		if time.Now().After(deadline) {
			return errors.New("timeout waiting for interface attachment to detach")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}
