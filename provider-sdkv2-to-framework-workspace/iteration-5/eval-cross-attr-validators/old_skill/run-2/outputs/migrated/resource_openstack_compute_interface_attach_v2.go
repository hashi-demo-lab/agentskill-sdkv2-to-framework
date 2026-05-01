package openstack

import (
	"context"
	"fmt"
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
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                   = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithConfigure      = &computeInterfaceAttachV2Resource{}
	_ resource.ResourceWithImportState    = &computeInterfaceAttachV2Resource{}
)

// NewComputeInterfaceAttachV2Resource is the framework constructor for the
// openstack_compute_interface_attach_v2 resource.
func NewComputeInterfaceAttachV2Resource() resource.Resource {
	return &computeInterfaceAttachV2Resource{}
}

type computeInterfaceAttachV2Resource struct {
	config *Config
}

// computeInterfaceAttachV2Model mirrors the resource schema. All fields are
// types.String — the SDKv2 schema only used TypeString attributes.
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
			"Unexpected provider data",
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
			"Invalid configuration",
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

	tflog.Debug(ctx, "openstack_compute_interface_attach_v2 attach options", map[string]any{
		"opts": fmt.Sprintf("%#v", attachOpts),
	})

	attachment, err := attachinterfaces.Create(ctx, computeClient, instanceID, attachOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_compute_interface_attach_v2",
			err.Error(),
		)
		return
	}

	if err := waitForComputeInterfaceAttachV2Attached(ctx, computeClient, instanceID, attachment.PortID); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error creating openstack_compute_interface_attach_v2 %s", instanceID),
			err.Error(),
		)
		return
	}

	// Refresh attachment so we capture the actual server-assigned values.
	final, err := attachinterfaces.Get(ctx, computeClient, instanceID, attachment.PortID).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error retrieving openstack_compute_interface_attach_v2 after create",
			err.Error(),
		)
		return
	}

	id := fmt.Sprintf("%s/%s", instanceID, final.PortID)

	tflog.Debug(ctx, "Created openstack_compute_interface_attach_v2", map[string]any{
		"id":         id,
		"attachment": fmt.Sprintf("%#v", final),
	})

	plan.ID = types.StringValue(id)
	plan.InstanceID = types.StringValue(instanceID)
	plan.PortID = types.StringValue(final.PortID)
	plan.NetworkID = types.StringValue(final.NetID)
	plan.Region = types.StringValue(region)

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
			"Invalid resource ID",
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

	tflog.Debug(ctx, "Retrieved openstack_compute_interface_attach_v2", map[string]any{
		"id":         state.ID.ValueString(),
		"attachment": fmt.Sprintf("%#v", attachment),
	})

	if len(attachment.FixedIPs) > 0 {
		state.FixedIP = types.StringValue(attachment.FixedIPs[0].IPAddress)
	} else {
		state.FixedIP = types.StringValue("")
	}

	state.InstanceID = types.StringValue(instanceID)
	state.PortID = types.StringValue(attachment.PortID)
	state.NetworkID = types.StringValue(attachment.NetID)
	state.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is intentionally a no-op: every user-facing attribute is ForceNew /
// RequiresReplace, so any change recreates the resource. The framework still
// requires the method to satisfy the resource.Resource interface.
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
			"Invalid resource ID",
			err.Error(),
		)
		return
	}

	if err := waitForComputeInterfaceAttachV2Detached(ctx, computeClient, instanceID, attachmentID); err != nil {
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

// waitForComputeInterfaceAttachV2Attached polls until the attachment reaches
// the ATTACHED state. Replaces the SDKv2 retry.StateChangeConf flow.
func waitForComputeInterfaceAttachV2Attached(ctx context.Context, computeClient *gophercloud.ServiceClient, instanceID, attachmentID string) error {
	const minTimeout = 5 * time.Second

	ticker := time.NewTicker(minTimeout)
	defer ticker.Stop()

	for {
		_, err := attachinterfaces.Get(ctx, computeClient, instanceID, attachmentID).Extract()
		if err == nil {
			return nil
		}
		if !gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for attachment %s on %s: %w", attachmentID, instanceID, ctx.Err())
		case <-ticker.C:
		}
	}
}

// waitForComputeInterfaceAttachV2Detached polls and issues delete calls until
// the attachment is gone. Replaces the SDKv2 retry.StateChangeConf flow.
func waitForComputeInterfaceAttachV2Detached(ctx context.Context, computeClient *gophercloud.ServiceClient, instanceID, attachmentID string) error {
	const minTimeout = 5 * time.Second

	ticker := time.NewTicker(minTimeout)
	defer ticker.Stop()

	for {
		tflog.Debug(ctx, "Attempting to detach openstack_compute_interface_attach_v2", map[string]any{
			"attachment_id": attachmentID,
			"instance_id":   instanceID,
		})

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
			if gophercloud.ResponseCodeIs(err, http.StatusBadRequest) {
				// Server is busy doing something else with the attachment;
				// fall through and retry.
			} else {
				return err
			}
		}

		tflog.Debug(ctx, "openstack_compute_interface_attach_v2 is still active", map[string]any{
			"attachment_id": attachmentID,
		})

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for detach of %s on %s: %w", attachmentID, instanceID, ctx.Err())
		case <-ticker.C:
		}
	}
}
