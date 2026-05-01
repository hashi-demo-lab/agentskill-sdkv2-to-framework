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

// Compile-time interface checks.
var (
	_ resource.Resource                = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithConfigure   = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithImportState = &computeInterfaceAttachV2Resource{}
)

// NewComputeInterfaceAttachV2Resource is the constructor registered with the
// framework provider.
func NewComputeInterfaceAttachV2Resource() resource.Resource {
	return &computeInterfaceAttachV2Resource{}
}

type computeInterfaceAttachV2Resource struct {
	config *Config
}

// computeInterfaceAttachV2Model mirrors the resource schema in typed form.
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

			// port_id conflicts with both network_id and fixed_ip. The
			// constraint is asymmetric (port_id is the "primary" alternative),
			// so per-attribute stringvalidator.ConflictsWith reads more
			// naturally here than a schema-wide resourcevalidator.
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
			// Preserve the SDKv2 `timeouts { ... }` block syntax used by
			// existing practitioner configs.
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

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = config
}

func (r *computeInterfaceAttachV2Resource) regionFor(plan computeInterfaceAttachV2Model) string {
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
		return plan.Region.ValueString()
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

	region := r.regionFor(plan)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

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
			"Missing required attribute",
			"Must set one of network_id and port_id",
		)

		return
	}

	// For some odd reason the API takes an array of IPs, but you can only have
	// one element in the array.
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
		resp.Diagnostics.AddError("Error creating openstack_compute_interface_attach_v2", err.Error())

		return
	}

	if err := waitForComputeInterfaceAttach(ctx, computeClient, instanceID, attachment.PortID, createTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error creating openstack_compute_interface_attach_v2 %s", instanceID),
			err.Error(),
		)

		return
	}

	id := fmt.Sprintf("%s/%s", instanceID, attachment.PortID)

	log.Printf("[DEBUG] Created openstack_compute_interface_attach_v2 %s: %#v", id, attachment)

	plan.ID = types.StringValue(id)
	plan.Region = types.StringValue(region)
	plan.InstanceID = types.StringValue(instanceID)
	plan.PortID = types.StringValue(attachment.PortID)
	plan.NetworkID = types.StringValue(attachment.NetID)

	if len(attachment.FixedIPs) > 0 {
		plan.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
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

	region := r.regionFor(state)

	computeClient, err := r.config.ComputeV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	instanceID, attachmentID, err := parsePairedIDs(state.ID.ValueString(), "openstack_compute_interface_attach_v2")
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource ID", err.Error())

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

	state.InstanceID = types.StringValue(instanceID)
	state.PortID = types.StringValue(attachment.PortID)
	state.NetworkID = types.StringValue(attachment.NetID)
	state.Region = types.StringValue(region)

	if len(attachment.FixedIPs) > 0 {
		state.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is implemented as a no-op: every user-settable attribute is marked
// RequiresReplace, so the framework will never call Update with a real diff.
// The method exists to satisfy the resource.Resource interface.
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

	computeClient, err := r.config.ComputeV2Client(ctx, r.regionFor(state))
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack compute client", err.Error())

		return
	}

	instanceID, attachmentID, err := parsePairedIDs(state.ID.ValueString(), "openstack_compute_interface_attach_v2")
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource ID", err.Error())

		return
	}

	if err := waitForComputeInterfaceDetach(ctx, computeClient, instanceID, attachmentID, deleteTimeout); err != nil {
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

// waitForComputeInterfaceAttach polls until the interface reports ATTACHED, or
// the supplied timeout elapses. It replaces the SDKv2 retry.StateChangeConf
// usage with a context-driven loop appropriate for the framework.
func waitForComputeInterfaceAttach(ctx context.Context, client *gophercloud.ServiceClient, instanceID, attachmentID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		_, err := attachinterfaces.Get(ctx, client, instanceID, attachmentID).Extract()
		if err == nil {
			return nil
		}

		if !gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return err
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for openstack_compute_interface_attach_v2 %s/%s to become ATTACHED", instanceID, attachmentID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// waitForComputeInterfaceDetach polls until the interface reports DETACHED, or
// the supplied timeout elapses.
func waitForComputeInterfaceDetach(ctx context.Context, client *gophercloud.ServiceClient, instanceID, attachmentID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		log.Printf("[DEBUG] Attempting to detach openstack_compute_interface_attach_v2 %s from instance %s", attachmentID, instanceID)

		_, err := attachinterfaces.Get(ctx, client, instanceID, attachmentID).Extract()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return nil
			}

			return err
		}

		if err := attachinterfaces.Delete(ctx, client, instanceID, attachmentID).ExtractErr(); err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return nil
			}

			if gophercloud.ResponseCodeIs(err, http.StatusBadRequest) {
				// Treat as transient; retry.
			} else {
				return err
			}
		}

		log.Printf("[DEBUG] openstack_compute_interface_attach_v2 %s is still active.", attachmentID)

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for openstack_compute_interface_attach_v2 %s/%s to detach", instanceID, attachmentID)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}
