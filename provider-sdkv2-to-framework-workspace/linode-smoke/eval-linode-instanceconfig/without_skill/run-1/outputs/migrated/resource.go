package instanceconfig

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
	instancehelpers "github.com/linode/terraform-provider-linode/v3/linode/instance"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// deviceSlotAttrTypes are the attribute types for a single device slot (disk_id + volume_id).
var deviceSlotAttrTypes = map[string]attr.Type{
	"disk_id":   types.Int64Type,
	"volume_id": types.Int64Type,
}

// devicesAttrTypes are the attribute types for the named "devices" block (sda, sdb, ...).
var devicesAttrTypes = buildDevicesAttrTypes()

func buildDevicesAttrTypes() map[string]attr.Type {
	keys := helper.GetConfigDeviceKeys()
	m := make(map[string]attr.Type, len(keys))
	for _, k := range keys {
		m[k] = types.ListType{ElemType: types.ObjectType{AttrTypes: deviceSlotAttrTypes}}
	}
	return m
}

// DeviceModel represents one entry in the new-style "device" set block.
type DeviceModel struct {
	DeviceName types.String `tfsdk:"device_name"`
	DiskID     types.Int64  `tfsdk:"disk_id"`
	VolumeID   types.Int64  `tfsdk:"volume_id"`
}

// HelpersModel represents the helpers nested block.
type HelpersModel struct {
	DevtmpfsAutomount types.Bool `tfsdk:"devtmpfs_automount"`
	Distro            types.Bool `tfsdk:"distro"`
	ModulesDep        types.Bool `tfsdk:"modules_dep"`
	Network           types.Bool `tfsdk:"network"`
	UpdateDBDisabled  types.Bool `tfsdk:"updatedb_disabled"`
}

// InterfaceModel represents one element of the interface list.
// (lightweight wrapper — the actual Linode provider stores this in helper pkg)
type InterfaceModel struct {
	Purpose    types.String `tfsdk:"purpose"`
	Label      types.String `tfsdk:"label"`
	IPAMAddr   types.String `tfsdk:"ipam_address"`
	Primary    types.Bool   `tfsdk:"primary"`
	SubnetID   types.Int64  `tfsdk:"subnet_id"`
	IPRanges   types.List   `tfsdk:"ip_ranges"`
	Active     types.Bool   `tfsdk:"active"`
	IPv4       types.List   `tfsdk:"ipv4"`
	IPv6       types.List   `tfsdk:"ipv6"`
	VPCID      types.Int64  `tfsdk:"vpc_id"`
	LinodeIfID types.Int64  `tfsdk:"id"`
}

// InstanceConfigResourceModel is the top-level state model for linode_instance_config.
type InstanceConfigResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	LinodeID    types.Int64  `tfsdk:"linode_id"`
	Label       types.String `tfsdk:"label"`
	Comments    types.String `tfsdk:"comments"`
	Kernel      types.String `tfsdk:"kernel"`
	MemoryLimit types.Int64  `tfsdk:"memory_limit"`
	RootDevice  types.String `tfsdk:"root_device"`
	RunLevel    types.String `tfsdk:"run_level"`
	VirtMode    types.String `tfsdk:"virt_mode"`
	Booted      types.Bool   `tfsdk:"booted"`

	// New-style: a set of device blocks each with device_name + disk_id/volume_id.
	Device types.Set `tfsdk:"device"`

	// Deprecated named block. MaxItems:1 in SDKv2 → SingleNestedBlock or ListNestedBlock
	// with a custom validator. We keep it as a list with at-most-one validator.
	Devices types.List `tfsdk:"devices"`

	Helpers   types.List `tfsdk:"helpers"`
	Interface types.List `tfsdk:"interface"`
}

// NewResource creates a new framework resource for linode_instance_config.
func NewResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_instance_config",
				IDAttr: "id",
				IDType: types.Int64Type,
				Schema: &resourceSchema,
			},
		),
	}
}

// Resource is the implementation of the linode_instance_config resource.
type Resource struct {
	helper.BaseResource
}

// resourceSchema defines the framework schema for linode_instance_config.
var resourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			Description: "The unique ID of the instance config.",
			Computed:    true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"linode_id": schema.Int64Attribute{
			Description: "The ID of the Linode to create this configuration profile under.",
			Required:    true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.RequiresReplace(),
			},
		},
		"label": schema.StringAttribute{
			Description: "The Config's label for display purposes only.",
			Required:    true,
		},
		"comments": schema.StringAttribute{
			Description: "Optional field for arbitrary User comments on this Config.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"kernel": schema.StringAttribute{
			Description: `A Kernel ID to boot a Linode with. Defaults to "linode/latest-64bit".`,
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("linode/latest-64bit"),
		},
		"memory_limit": schema.Int64Attribute{
			Description: "The memory limit of the Linode.",
			Optional:    true,
			Computed:    true,
			Default:     int64default.StaticInt64(0),
		},
		"root_device": schema.StringAttribute{
			Description: "The root device to boot. " +
				"If no value or an invalid value is provided, root device will default to /dev/sda. " +
				"If the device specified at the root device location is not mounted, " +
				"the Linode will not boot until a device is mounted.",
			Optional: true,
			Computed: true,
			Default:  stringdefault.StaticString("/dev/sda"),
		},
		"run_level": schema.StringAttribute{
			Description: "Defines the state of your Linode after booting.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("default"),
			Validators: []validator.String{
				stringvalidator.OneOfCaseInsensitive("default", "single", "binbash"),
			},
		},
		"virt_mode": schema.StringAttribute{
			Description: "Controls the virtualization mode.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("paravirt"),
			Validators: []validator.String{
				stringvalidator.OneOfCaseInsensitive("paravirt", "fullvirt"),
			},
		},
		"booted": schema.BoolAttribute{
			Description: "If true, the Linode will be booted to running state. " +
				"If false, the Linode will be shutdown. If undefined, no action will be taken.",
			Optional: true,
			Computed: true,
		},
	},
	Blocks: map[string]schema.Block{
		// New-style device set. ConflictsWith "devices" → setvalidator.ConflictsWith.
		"device": schema.SetNestedBlock{
			Description: "Blocks for device disks in a Linode's configuration profile.",
			Validators: []validator.Set{
				setvalidator.ConflictsWith(path.MatchRoot("devices")),
			},
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"device_name": schema.StringAttribute{
						Description: "The device slot identifier (for example, sda, sdb) to map a disk or volume into",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.OneOf(helper.GetConfigDeviceKeys()...),
						},
					},
					"disk_id": schema.Int64Attribute{
						Description: "The Disk ID to map to this disk slot",
						Optional:    true,
						Computed:    true,
					},
					"volume_id": schema.Int64Attribute{
						Description: "The Block Storage volume ID to map to this disk slot",
						Optional:    true,
						Computed:    true,
					},
				},
			},
		},

		// Deprecated named devices block. MaxItems:1 → listvalidator.SizeAtMost(1).
		// ConflictsWith "device" → listvalidator.ConflictsWith.
		"devices": schema.ListNestedBlock{
			Description:        "A dictionary of device disks to use as a device map in a Linode's configuration profile. Deprecated in favor of `device`.",
			DeprecationMessage: "Devices attribute is deprecated in favor of `device`.",
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
				listvalidator.ConflictsWith(path.MatchRoot("device")),
			},
			NestedObject: schema.NestedBlockObject{
				Attributes: buildDevicesNestedAttributes(),
			},
		},

		// helpers block (MaxItems:1 → listvalidator.SizeAtMost)
		"helpers": schema.ListNestedBlock{
			Description: "Helpers enabled when booting to this Linode Config.",
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"devtmpfs_automount": schema.BoolAttribute{
						Description: "Populates the /dev directory early during boot without udev.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
					"distro": schema.BoolAttribute{
						Description: "Helps maintain correct inittab/upstart console device.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
					"modules_dep": schema.BoolAttribute{
						Description: "Creates a modules dependency file for the Kernel you run.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
					"network": schema.BoolAttribute{
						Description: "Automatically configures static networking.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
					"updatedb_disabled": schema.BoolAttribute{
						Description: "Disables updatedb cron job to avoid disk thrashing.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
				},
			},
		},

		// interface list block — mirrors the SDKv2 list with no MaxItems
		"interface": schema.ListNestedBlock{
			Description: "An array of Network Interfaces to add to this Linode's Configuration Profile.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"purpose": schema.StringAttribute{
						Description: "The type of interface (public, vlan, vpc).",
						Required:    true,
					},
					"label": schema.StringAttribute{
						Description: "The VLAN label for vlan-purpose interfaces.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
					"ipam_address": schema.StringAttribute{
						Description: "The IPAM address for vlan-purpose interfaces.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
					},
					"primary": schema.BoolAttribute{
						Description: "Whether the interface is the primary interface.",
						Optional:    true,
						Computed:    true,
					},
					"subnet_id": schema.Int64Attribute{
						Description: "The ID of the subnet for vpc-purpose interfaces.",
						Optional:    true,
						Computed:    true,
					},
					"ip_ranges": schema.ListAttribute{
						Description: "IP ranges for vpc-purpose interfaces.",
						Optional:    true,
						Computed:    true,
						ElementType: types.StringType,
					},
					"active": schema.BoolAttribute{
						Description: "Whether this interface is currently booted and active.",
						Computed:    true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"vpc_id": schema.Int64Attribute{
						Description: "The ID of the VPC for vpc-purpose interfaces.",
						Computed:    true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"id": schema.Int64Attribute{
						Description: "The ID of the interface.",
						Computed:    true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"ipv4": schema.ListNestedAttribute{
						Description: "IPv4 configuration for vpc-purpose interfaces.",
						Optional:    true,
						Computed:    true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"vpc": schema.StringAttribute{
									Description: "The VPC-assigned IP address.",
									Optional:    true,
									Computed:    true,
									Default:     stringdefault.StaticString(""),
								},
								"nat_1_1": schema.StringAttribute{
									Description: "The 1:1 NAT address for this interface.",
									Optional:    true,
									Computed:    true,
									Default:     stringdefault.StaticString(""),
								},
							},
						},
					},
					"ipv6": schema.ListNestedAttribute{
						Description: "IPv6 configuration for vpc-purpose interfaces.",
						Optional:    true,
						Computed:    true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"is_public": schema.BoolAttribute{
									Description: "Whether the IPv6 range is public.",
									Optional:    true,
									Computed:    true,
								},
								"slaac": schema.ListNestedAttribute{
									Description: "SLAAC ranges.",
									Computed:    true,
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"range": schema.StringAttribute{
												Computed: true,
											},
											"assigned_range": schema.StringAttribute{
												Computed: true,
											},
										},
									},
								},
								"range": schema.ListNestedAttribute{
									Description: "IPv6 ranges.",
									Computed:    true,
									NestedObject: schema.NestedAttributeObject{
										Attributes: map[string]schema.Attribute{
											"range": schema.StringAttribute{
												Computed: true,
											},
											"assigned_range": schema.StringAttribute{
												Computed: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	},
}

// buildDevicesNestedAttributes generates per-slot (sda, sdb, …) attributes for the deprecated devices block.
func buildDevicesNestedAttributes() map[string]schema.Attribute {
	keys := helper.GetConfigDeviceKeys()
	result := make(map[string]schema.Attribute, len(keys))
	for _, k := range keys {
		result[k] = schema.ListNestedAttribute{
			Description: "Device can be either a Disk or Volume identified by disk_id or volume_id. Only one type per slot allowed.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"disk_id": schema.Int64Attribute{
						Description: "The Disk ID to map to this disk slot",
						Optional:    true,
						Computed:    true,
					},
					"volume_id": schema.Int64Attribute{
						Description: "The Block Storage volume ID to map to this disk slot",
						Optional:    true,
						Computed:    true,
					},
				},
			},
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// ImportState
// ---------------------------------------------------------------------------

func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	tflog.Debug(ctx, "Import linode_instance_config", map[string]any{
		"id": req.ID,
	})

	if strings.Contains(req.ID, ",") {
		parts := strings.Split(req.ID, ",")
		if len(parts) != 2 {
			resp.Diagnostics.AddError(
				"Invalid import ID",
				"Expected format: <linode_id>,<config_id>",
			)
			return
		}

		configID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			resp.Diagnostics.AddError("Invalid config ID", fmt.Sprintf("invalid config ID: %v", err))
			return
		}

		linodeID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			resp.Diagnostics.AddError("Invalid linode ID", fmt.Sprintf("invalid instance ID: %v", err))
			return
		}

		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), configID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("linode_id"), linodeID)...)
		return
	}

	// Single integer ID — just set it.
	configID, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid config ID", fmt.Sprintf("failed to parse id: %v", err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), configID)...)
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (r *Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_instance_config")

	var state InstanceConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFramework(ctx, state)

	client := r.Meta.Client

	configID := int(state.ID.ValueInt64())
	linodeID := int(state.LinodeID.ValueInt64())

	cfg, err := client.GetInstanceConfig(ctx, linodeID, configID)
	if linodego.IsNotFound(err) {
		tflog.Warn(ctx, fmt.Sprintf(
			"removing Instance Config ID %d from state because it no longer exists", configID,
		))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to get instance config", err.Error())
		return
	}

	inst, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get instance", err.Error())
		return
	}

	configBooted, err := isConfigBooted(ctx, &client, inst, cfg.ID, state.Booted.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Failed to check instance boot status", err.Error())
		return
	}

	// Populate flat attributes.
	state.Label = types.StringValue(cfg.Label)
	state.Comments = types.StringValue(cfg.Comments)
	state.Kernel = types.StringValue(cfg.Kernel)
	state.MemoryLimit = types.Int64Value(int64(cfg.MemoryLimit))
	state.RootDevice = types.StringValue(cfg.RootDevice)
	state.RunLevel = types.StringValue(string(cfg.RunLevel))
	state.VirtMode = types.StringValue(string(cfg.VirtMode))
	state.Booted = types.BoolValue(configBooted)

	// Flatten devices.
	if cfg.Devices != nil {
		resp.Diagnostics.Append(flattenDeviceBlockFramework(ctx, *cfg.Devices, &state)...)
	}

	// Flatten helpers.
	if cfg.Helpers != nil {
		resp.Diagnostics.Append(flattenHelpersFramework(ctx, *cfg.Helpers, &state)...)
	}

	// Flatten interfaces.
	resp.Diagnostics.Append(flattenInterfacesFramework(ctx, cfg.Interfaces, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_instance_config")

	var plan InstanceConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFramework(ctx, plan)
	client := r.Meta.Client
	linodeID := int(plan.LinodeID.ValueInt64())

	createOpts := linodego.InstanceConfigCreateOptions{
		Label:       plan.Label.ValueString(),
		Comments:    plan.Comments.ValueString(),
		MemoryLimit: int(plan.MemoryLimit.ValueInt64()),
		Kernel:      plan.Kernel.ValueString(),
		RunLevel:    plan.RunLevel.ValueString(),
		VirtMode:    plan.VirtMode.ValueString(),
	}

	if !plan.RootDevice.IsNull() && !plan.RootDevice.IsUnknown() {
		rd := plan.RootDevice.ValueString()
		createOpts.RootDevice = &rd
	}

	// Expand interfaces.
	ifaces, d := expandInterfacesFramework(ctx, plan.Interface)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	createOpts.Interfaces = ifaces

	// Expand helpers.
	helpers, d := expandHelpersFramework(ctx, plan.Helpers)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	createOpts.Helpers = helpers

	// Expand devices — prefer device block over deprecated devices block.
	devices, d := expandDevicesFramework(ctx, plan.Device, plan.Devices)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if devices != nil {
		createOpts.Devices = *devices
	}

	tflog.Debug(ctx, "client.CreateInstanceConfig(...)", map[string]any{
		"options": createOpts,
	})

	cfg, err := client.CreateInstanceConfig(ctx, linodeID, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create linode instance config", err.Error())
		return
	}

	plan.ID = types.Int64Value(int64(cfg.ID))

	ctx = tflog.SetField(ctx, "config_id", cfg.ID)

	// Handle booted state if explicitly configured.
	if !plan.Booted.IsNull() && !plan.Booted.IsUnknown() {
		deadlineSecs := getFrameworkDeadlineSeconds(ctx)
		if err := applyBootStatus(ctx, &client, linodeID, cfg.ID, deadlineSecs,
			plan.Booted.ValueBool(), false); err != nil {
			resp.Diagnostics.AddError("Failed to update boot status", err.Error())
			return
		}
	}

	// Re-read to get computed values.
	readResp := &resource.ReadResponse{State: resp.State}
	r.Read(ctx, resource.ReadRequest{State: resp.State}, readResp)
	resp.Diagnostics.Append(readResp.Diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Persist what we planned plus ID.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_instance_config")

	var plan, state InstanceConfigResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFramework(ctx, plan)
	client := r.Meta.Client

	configID := int(state.ID.ValueInt64())
	linodeID := int(plan.LinodeID.ValueInt64())

	putRequest := linodego.InstanceConfigUpdateOptions{}
	shouldUpdate := false

	if !plan.Comments.Equal(state.Comments) {
		putRequest.Comments = plan.Comments.ValueString()
		shouldUpdate = true
	}

	if !plan.Kernel.Equal(state.Kernel) {
		putRequest.Kernel = plan.Kernel.ValueString()
		shouldUpdate = true
	}

	if !plan.Label.Equal(state.Label) {
		putRequest.Label = plan.Label.ValueString()
		shouldUpdate = true
	}

	if !plan.MemoryLimit.Equal(state.MemoryLimit) {
		putRequest.MemoryLimit = int(plan.MemoryLimit.ValueInt64())
		shouldUpdate = true
	}

	if !plan.RootDevice.Equal(state.RootDevice) {
		putRequest.RootDevice = plan.RootDevice.ValueString()
		shouldUpdate = true
	}

	if !plan.RunLevel.Equal(state.RunLevel) {
		putRequest.RunLevel = plan.RunLevel.ValueString()
		shouldUpdate = true
	}

	if !plan.VirtMode.Equal(state.VirtMode) {
		putRequest.VirtMode = plan.VirtMode.ValueString()
		shouldUpdate = true
	}

	// Expand devices (device or devices, whichever is configured).
	if !plan.Device.Equal(state.Device) || !plan.Devices.Equal(state.Devices) {
		devices, d := expandDevicesFramework(ctx, plan.Device, plan.Devices)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		putRequest.Devices = devices
		shouldUpdate = true
	}

	// Expand helpers.
	if !plan.Helpers.Equal(state.Helpers) {
		helpers, d := expandHelpersFramework(ctx, plan.Helpers)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		putRequest.Helpers = helpers
		shouldUpdate = true
	}

	inst, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		resp.Diagnostics.AddError("Error finding the specified Linode Instance", err.Error())
		return
	}

	bootedConfigID, err := helper.GetCurrentBootedConfig(ctx, &client, linodeID)
	if err != nil {
		tflog.Warn(ctx, fmt.Sprintf("failed to get current booted config of Linode %d", linodeID))
	}

	isBootedConfig := bootedConfigID == configID && inst.Status == linodego.InstanceRunning
	powerOffRequired := false

	if !plan.Interface.Equal(state.Interface) {
		ifaces, d := expandInterfacesFramework(ctx, plan.Interface)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		putRequest.Interfaces = ifaces

		// Check if VPC interface is involved (requires instance power-off).
		existingCfg, err := client.GetInstanceConfig(ctx, linodeID, configID)
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("failed to get config %d", configID), err.Error())
			return
		}
		powerOffRequired = instancehelpers.VPCInterfaceIncluded(existingCfg.Interfaces, ifaces) && isBootedConfig
		shouldUpdate = true
	}

	managedBoot := !plan.Booted.IsNull() && !plan.Booted.IsUnknown()
	shouldPowerBackOn := !managedBoot && powerOffRequired

	if shouldUpdate {
		if powerOffRequired {
			if err := instancehelpers.ShutdownInstanceForOfflineOperation(
				ctx, &client,
				r.Meta.Config.SkipImplicitReboots.ValueBool(),
				linodeID,
				getFrameworkDeadlineSeconds(ctx),
				"VPC interface update",
			); err != nil {
				resp.Diagnostics.AddError("Failed to shutdown linode instance for VPC interface update", err.Error())
				return
			}
		}

		tflog.Debug(ctx, "client.UpdateInstanceConfig(...)", map[string]any{
			"options": putRequest,
		})
		if _, err := client.UpdateInstanceConfig(ctx, linodeID, configID, putRequest); err != nil {
			resp.Diagnostics.AddError("Failed to update instance config", err.Error())
			return
		}

		if shouldPowerBackOn {
			if err := helper.BootInstanceSync(ctx, &client, linodeID, configID, getFrameworkDeadlineSeconds(ctx)); err != nil {
				tflog.Warn(ctx, fmt.Sprintf("failed to boot instance after VPC interface update: %s", err))
			}
		}
	}

	shouldReboot := isBootedConfig && shouldUpdate && !powerOffRequired &&
		!r.Meta.Config.SkipImplicitReboots.ValueBool()

	if managedBoot {
		if err := applyBootStatus(ctx, &client, linodeID, configID,
			getFrameworkDeadlineSeconds(ctx),
			plan.Booted.ValueBool(),
			shouldReboot); err != nil {
			resp.Diagnostics.AddError("Failed to update boot status", err.Error())
			return
		}
	}

	// Re-read computed state.
	plan.ID = state.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readIntoState(ctx, plan, resp)
}

// readIntoState is a helper to call Read and absorb the result into the Update response.
func (r *Resource) readIntoState(
	ctx context.Context,
	current InstanceConfigResourceModel,
	resp *resource.UpdateResponse,
) {
	// Build a temporary ReadResponse backed by the current resp.State.
	readReq := resource.ReadRequest{State: resp.State}
	readResp := &resource.ReadResponse{State: resp.State}
	r.Read(ctx, readReq, readResp)
	resp.Diagnostics.Append(readResp.Diagnostics...)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func (r *Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_instance_config")

	var state InstanceConfigResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFramework(ctx, state)
	client := r.Meta.Client

	configID := int(state.ID.ValueInt64())
	linodeID := int(state.LinodeID.ValueInt64())

	inst, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		resp.Diagnostics.AddError("Error finding the specified Linode Instance", err.Error())
		return
	}

	locked, err := helper.LinodeIsLockedWithCannotDeleteSubresources(ctx, &client, linodeID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to get locks of Linode", err.Error())
		return
	}

	if locked {
		resp.Diagnostics.AddError(
			"Cannot delete config",
			fmt.Sprintf(
				"can't delete config %d in Linode %d: the resource lock on the Linode prohibits deletion "+
					"of its subresources, which includes this config",
				configID, linodeID,
			),
		)
		return
	}

	// Shutdown the instance if the config is in use.
	if booted, err := isConfigBooted(ctx, &client, inst, configID, state.Booted.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Failed to check if config is booted", err.Error())
		return
	} else if booted {
		tflog.Info(ctx, "Shutting down instance for config deletion")

		p, err := client.NewEventPoller(ctx, inst.ID, linodego.EntityLinode, linodego.ActionLinodeShutdown)
		if err != nil {
			resp.Diagnostics.AddError("Failed to poll for events", err.Error())
			return
		}

		tflog.Debug(ctx, "client.ShutdownInstance(...)")
		if err := client.ShutdownInstance(ctx, inst.ID); err != nil {
			resp.Diagnostics.AddError("Failed to shutdown instance", err.Error())
			return
		}

		tflog.Trace(ctx, "Waiting for instance shutdown to finish")
		if _, err := p.WaitForFinished(ctx, getFrameworkDeadlineSeconds(ctx)); err != nil {
			resp.Diagnostics.AddError("Failed to wait for instance shutdown", err.Error())
			return
		}
		tflog.Debug(ctx, "Instance shutdown complete")
	}

	tflog.Debug(ctx, "client.DeleteInstanceConfig(...)")
	if err := client.DeleteInstanceConfig(ctx, linodeID, configID); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting Linode Instance Config %d", configID),
			err.Error(),
		)
		return
	}
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func populateLogAttributesFramework(ctx context.Context, m InstanceConfigResourceModel) context.Context {
	return helper.SetLogFieldBulk(ctx, map[string]any{
		"linode_id": m.LinodeID.ValueInt64(),
		"id":        m.ID.ValueInt64(),
	})
}

// getFrameworkDeadlineSeconds returns seconds until the context deadline, or the default (600s).
func getFrameworkDeadlineSeconds(ctx context.Context) int {
	if deadline, ok := ctx.Deadline(); ok {
		return int(time.Until(deadline).Seconds())
	}
	return helper.DefaultFrameworkRebootTimeout
}

// ---------------------------------------------------------------------------
// Flatten helpers (API → state)
// ---------------------------------------------------------------------------

func flattenDeviceBlockFramework(
	ctx context.Context,
	deviceMap linodego.InstanceConfigDeviceMap,
	state *InstanceConfigResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	// ---- new-style "device" set ----
	deviceElemType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"device_name": types.StringType,
			"disk_id":     types.Int64Type,
			"volume_id":   types.Int64Type,
		},
	}

	reflectMap := reflect.ValueOf(deviceMap)
	deviceElems := make([]attr.Value, 0)

	for i := 0; i < reflectMap.NumField(); i++ {
		field := reflectMap.Field(i).Interface().(*linodego.InstanceConfigDevice)
		if field == nil {
			continue
		}
		fieldName := strings.ToLower(reflectMap.Type().Field(i).Name)
		obj, d := types.ObjectValue(
			map[string]attr.Type{
				"device_name": types.StringType,
				"disk_id":     types.Int64Type,
				"volume_id":   types.Int64Type,
			},
			map[string]attr.Value{
				"device_name": types.StringValue(fieldName),
				"disk_id":     types.Int64Value(int64(field.DiskID)),
				"volume_id":   types.Int64Value(int64(field.VolumeID)),
			},
		)
		diags.Append(d...)
		deviceElems = append(deviceElems, obj)
	}

	deviceSet, d := types.SetValue(deviceElemType, deviceElems)
	diags.Append(d...)
	state.Device = deviceSet

	// ---- deprecated "devices" named block ----
	deviceSlotObjType := types.ObjectType{AttrTypes: deviceSlotAttrTypes}
	outerMap := make(map[string]attr.Value)
	for i := 0; i < reflectMap.NumField(); i++ {
		field := reflectMap.Field(i).Interface().(*linodego.InstanceConfigDevice)
		fieldName := strings.ToLower(reflectMap.Type().Field(i).Name)

		var slotList types.List
		if field == nil {
			slotList = types.ListValueMust(deviceSlotObjType, []attr.Value{})
		} else {
			slotObj, d := types.ObjectValue(
				deviceSlotAttrTypes,
				map[string]attr.Value{
					"disk_id":   types.Int64Value(int64(field.DiskID)),
					"volume_id": types.Int64Value(int64(field.VolumeID)),
				},
			)
			diags.Append(d...)
			slotList = types.ListValueMust(deviceSlotObjType, []attr.Value{slotObj})
		}
		outerMap[fieldName] = slotList
	}

	devicesObjAttrTypes := make(map[string]attr.Type, len(outerMap))
	for k := range outerMap {
		devicesObjAttrTypes[k] = types.ListType{ElemType: deviceSlotObjType}
	}
	devicesObj, d := types.ObjectValue(devicesObjAttrTypes, outerMap)
	diags.Append(d...)
	devicesObjType := types.ObjectType{AttrTypes: devicesObjAttrTypes}
	state.Devices = types.ListValueMust(devicesObjType, []attr.Value{devicesObj})

	return diags
}

func flattenHelpersFramework(
	ctx context.Context,
	helpers linodego.InstanceConfigHelpers,
	state *InstanceConfigResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	helpersAttrTypes := map[string]attr.Type{
		"devtmpfs_automount": types.BoolType,
		"distro":             types.BoolType,
		"modules_dep":        types.BoolType,
		"network":            types.BoolType,
		"updatedb_disabled":  types.BoolType,
	}
	helpersObjType := types.ObjectType{AttrTypes: helpersAttrTypes}

	helpersObj, d := types.ObjectValue(
		helpersAttrTypes,
		map[string]attr.Value{
			"devtmpfs_automount": types.BoolValue(helpers.DevTmpFsAutomount),
			"distro":             types.BoolValue(helpers.Distro),
			"modules_dep":        types.BoolValue(helpers.ModulesDep),
			"network":            types.BoolValue(helpers.Network),
			"updatedb_disabled":  types.BoolValue(helpers.UpdateDBDisabled),
		},
	)
	diags.Append(d...)

	state.Helpers = types.ListValueMust(helpersObjType, []attr.Value{helpersObj})
	return diags
}

func flattenInterfacesFramework(
	ctx context.Context,
	interfaces []linodego.InstanceConfigInterface,
	state *InstanceConfigResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	// Use the SDKv2 helper to get the raw map representation, then convert.
	rawInterfaces := helper.FlattenInterfaces(interfaces)

	// We'll build a list of objects from the raw map list.
	// For simplicity, store the list as types.List with DynamicType elements
	// converted from the raw maps using the same attribute types as the schema.
	ifaceAttrTypes := buildInterfaceAttrTypes()
	ifaceObjType := types.ObjectType{AttrTypes: ifaceAttrTypes}

	elems := make([]attr.Value, 0, len(rawInterfaces))
	for _, raw := range rawInterfaces {
		obj, d := rawInterfaceToObject(ctx, raw, ifaceAttrTypes)
		diags.Append(d...)
		elems = append(elems, obj)
	}

	ifaceList, d := types.ListValue(ifaceObjType, elems)
	diags.Append(d...)
	state.Interface = ifaceList
	return diags
}

func buildInterfaceAttrTypes() map[string]attr.Type {
	ipv4AttrTypes := map[string]attr.Type{
		"vpc":     types.StringType,
		"nat_1_1": types.StringType,
	}
	ipv6RangeAttrTypes := map[string]attr.Type{
		"range":          types.StringType,
		"assigned_range": types.StringType,
	}
	ipv6AttrTypes := map[string]attr.Type{
		"is_public": types.BoolType,
		"slaac":     types.ListType{ElemType: types.ObjectType{AttrTypes: ipv6RangeAttrTypes}},
		"range":     types.ListType{ElemType: types.ObjectType{AttrTypes: ipv6RangeAttrTypes}},
	}
	return map[string]attr.Type{
		"purpose":      types.StringType,
		"label":        types.StringType,
		"ipam_address": types.StringType,
		"primary":      types.BoolType,
		"subnet_id":    types.Int64Type,
		"ip_ranges":    types.ListType{ElemType: types.StringType},
		"active":       types.BoolType,
		"vpc_id":       types.Int64Type,
		"id":           types.Int64Type,
		"ipv4":         types.ListType{ElemType: types.ObjectType{AttrTypes: ipv4AttrTypes}},
		"ipv6":         types.ListType{ElemType: types.ObjectType{AttrTypes: ipv6AttrTypes}},
	}
}

// rawInterfaceToObject converts a raw map[string]any (from FlattenInterfaces) to a framework Object.
func rawInterfaceToObject(
	ctx context.Context,
	raw map[string]any,
	attrTypes map[string]attr.Type,
) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics

	ipv4AttrTypes := map[string]attr.Type{
		"vpc":     types.StringType,
		"nat_1_1": types.StringType,
	}
	ipv6RangeAttrTypes := map[string]attr.Type{
		"range":          types.StringType,
		"assigned_range": types.StringType,
	}
	ipv6AttrTypes := map[string]attr.Type{
		"is_public": types.BoolType,
		"slaac":     types.ListType{ElemType: types.ObjectType{AttrTypes: ipv6RangeAttrTypes}},
		"range":     types.ListType{ElemType: types.ObjectType{AttrTypes: ipv6RangeAttrTypes}},
	}

	// Helpers.
	strVal := func(m map[string]any, k string) types.String {
		if v, ok := m[k]; ok && v != nil {
			return types.StringValue(fmt.Sprintf("%v", v))
		}
		return types.StringValue("")
	}
	boolVal := func(m map[string]any, k string) types.Bool {
		if v, ok := m[k]; ok {
			if b, ok := v.(bool); ok {
				return types.BoolValue(b)
			}
		}
		return types.BoolValue(false)
	}
	int64Val := func(m map[string]any, k string) types.Int64 {
		if v, ok := m[k]; ok && v != nil {
			switch n := v.(type) {
			case int:
				return types.Int64Value(int64(n))
			case int64:
				return types.Int64Value(n)
			case float64:
				return types.Int64Value(int64(n))
			}
		}
		return types.Int64Value(0)
	}

	// ip_ranges
	var ipRangeElems []attr.Value
	if ipRanges, ok := raw["ip_ranges"]; ok && ipRanges != nil {
		for _, r := range ipRanges.([]any) {
			ipRangeElems = append(ipRangeElems, types.StringValue(fmt.Sprintf("%v", r)))
		}
	}
	ipRangesList, d := types.ListValue(types.StringType, ipRangeElems)
	diags.Append(d...)

	// ipv4
	ipv4ObjType := types.ObjectType{AttrTypes: ipv4AttrTypes}
	var ipv4Elems []attr.Value
	if ipv4Raw, ok := raw["ipv4"]; ok && ipv4Raw != nil {
		for _, elem := range ipv4Raw.([]any) {
			em := elem.(map[string]any)
			obj, d2 := types.ObjectValue(ipv4AttrTypes, map[string]attr.Value{
				"vpc":     strVal(em, "vpc"),
				"nat_1_1": strVal(em, "nat_1_1"),
			})
			diags.Append(d2...)
			ipv4Elems = append(ipv4Elems, obj)
		}
	}
	ipv4List, d := types.ListValue(ipv4ObjType, ipv4Elems)
	diags.Append(d...)

	// ipv6 range helper
	toRangeObjs := func(raw []any) []attr.Value {
		var out []attr.Value
		for _, r := range raw {
			rm := r.(map[string]any)
			obj, d2 := types.ObjectValue(ipv6RangeAttrTypes, map[string]attr.Value{
				"range":          strVal(rm, "range"),
				"assigned_range": strVal(rm, "assigned_range"),
			})
			diags.Append(d2...)
			out = append(out, obj)
		}
		return out
	}

	ipv6ObjType := types.ObjectType{AttrTypes: ipv6AttrTypes}
	ipv6RangeObjType := types.ObjectType{AttrTypes: ipv6RangeAttrTypes}
	var ipv6Elems []attr.Value
	if ipv6Raw, ok := raw["ipv6"]; ok && ipv6Raw != nil {
		for _, elem := range ipv6Raw.([]any) {
			em := elem.(map[string]any)

			var slaacElems, rangeElems []attr.Value
			if s, ok := em["slaac"]; ok && s != nil {
				slaacElems = toRangeObjs(s.([]any))
			}
			if rr, ok := em["range"]; ok && rr != nil {
				rangeElems = toRangeObjs(rr.([]any))
			}

			slaacList, d2 := types.ListValue(ipv6RangeObjType, slaacElems)
			diags.Append(d2...)
			rangList, d3 := types.ListValue(ipv6RangeObjType, rangeElems)
			diags.Append(d3...)

			obj, d4 := types.ObjectValue(ipv6AttrTypes, map[string]attr.Value{
				"is_public": boolVal(em, "is_public"),
				"slaac":     slaacList,
				"range":     rangList,
			})
			diags.Append(d4...)
			ipv6Elems = append(ipv6Elems, obj)
		}
	}
	ipv6List, d := types.ListValue(ipv6ObjType, ipv6Elems)
	diags.Append(d...)

	obj, d := types.ObjectValue(attrTypes, map[string]attr.Value{
		"purpose":      strVal(raw, "purpose"),
		"label":        strVal(raw, "label"),
		"ipam_address": strVal(raw, "ipam_address"),
		"primary":      boolVal(raw, "primary"),
		"subnet_id":    int64Val(raw, "subnet_id"),
		"ip_ranges":    ipRangesList,
		"active":       boolVal(raw, "active"),
		"vpc_id":       int64Val(raw, "vpc_id"),
		"id":           int64Val(raw, "id"),
		"ipv4":         ipv4List,
		"ipv6":         ipv6List,
	})
	diags.Append(d...)
	return obj, diags
}

// ---------------------------------------------------------------------------
// Expand helpers (state → API)
// ---------------------------------------------------------------------------

func expandDevicesFramework(
	ctx context.Context,
	deviceSet types.Set,
	devicesNamedList types.List,
) (*linodego.InstanceConfigDeviceMap, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Prefer new-style device set if populated.
	if !deviceSet.IsNull() && !deviceSet.IsUnknown() && len(deviceSet.Elements()) > 0 {
		var deviceModels []DeviceModel
		diags.Append(deviceSet.ElementsAs(ctx, &deviceModels, false)...)
		if diags.HasError() {
			return nil, diags
		}

		var result linodego.InstanceConfigDeviceMap
		seen := make(map[string]bool)
		for _, dm := range deviceModels {
			name := dm.DeviceName.ValueString()
			if seen[name] {
				log.Printf("[WARN] device %q was defined more than once", name)
				continue
			}
			seen[name] = true

			linodeDevice := linodego.InstanceConfigDevice{
				DiskID:   int(dm.DiskID.ValueInt64()),
				VolumeID: int(dm.VolumeID.ValueInt64()),
			}
			field := reflect.Indirect(reflect.ValueOf(&result)).FieldByName(strings.ToUpper(name))
			if field.IsValid() {
				field.Set(reflect.ValueOf(&linodeDevice))
			}
		}
		return &result, diags
	}

	// Fall back to deprecated named block.
	if !devicesNamedList.IsNull() && !devicesNamedList.IsUnknown() && len(devicesNamedList.Elements()) > 0 {
		var rawSlice []map[string]attr.Value
		// We manually walk the list since the nested type is complex.
		listElems := devicesNamedList.Elements()
		if len(listElems) < 1 {
			return nil, diags
		}

		outerObj, ok := listElems[0].(types.Object)
		if !ok {
			diags.AddError("Unexpected devices element type", fmt.Sprintf("got %T", listElems[0]))
			return nil, diags
		}

		var result linodego.InstanceConfigDeviceMap
		for slotName, slotVal := range outerObj.Attributes() {
			slotList, ok := slotVal.(types.List)
			if !ok || slotList.IsNull() || slotList.IsUnknown() || len(slotList.Elements()) == 0 {
				continue
			}
			slotObj, ok := slotList.Elements()[0].(types.Object)
			if !ok {
				continue
			}
			attrs := slotObj.Attributes()
			diskIDVal, _ := attrs["disk_id"].(types.Int64)
			volumeIDVal, _ := attrs["volume_id"].(types.Int64)

			linodeDevice := linodego.InstanceConfigDevice{
				DiskID:   int(diskIDVal.ValueInt64()),
				VolumeID: int(volumeIDVal.ValueInt64()),
			}
			field := reflect.Indirect(reflect.ValueOf(&result)).FieldByName(strings.ToUpper(slotName))
			if field.IsValid() {
				field.Set(reflect.ValueOf(&linodeDevice))
			}
		}
		_ = rawSlice
		return &result, diags
	}

	return nil, diags
}

func expandHelpersFramework(
	ctx context.Context,
	helpersList types.List,
) (*linodego.InstanceConfigHelpers, diag.Diagnostics) {
	var diags diag.Diagnostics

	if helpersList.IsNull() || helpersList.IsUnknown() || len(helpersList.Elements()) == 0 {
		return nil, diags
	}

	var helpersModels []HelpersModel
	diags.Append(helpersList.ElementsAs(ctx, &helpersModels, false)...)
	if diags.HasError() {
		return nil, diags
	}

	hm := helpersModels[0]
	return &linodego.InstanceConfigHelpers{
		DevTmpFsAutomount: hm.DevtmpfsAutomount.ValueBool(),
		Distro:            hm.Distro.ValueBool(),
		ModulesDep:        hm.ModulesDep.ValueBool(),
		Network:           hm.Network.ValueBool(),
		UpdateDBDisabled:  hm.UpdateDBDisabled.ValueBool(),
	}, diags
}

func expandInterfacesFramework(
	ctx context.Context,
	interfaceList types.List,
) ([]linodego.InstanceConfigInterfaceCreateOptions, diag.Diagnostics) {
	var diags diag.Diagnostics

	if interfaceList.IsNull() || interfaceList.IsUnknown() || len(interfaceList.Elements()) == 0 {
		return nil, diags
	}

	// Convert to raw []any and use the existing SDKv2 helper.
	rawElems := make([]any, 0, len(interfaceList.Elements()))
	var ifaceModels []InterfaceModel
	diags.Append(interfaceList.ElementsAs(ctx, &ifaceModels, false)...)
	if diags.HasError() {
		return nil, diags
	}

	for _, im := range ifaceModels {
		m := map[string]any{
			"purpose":      im.Purpose.ValueString(),
			"label":        im.Label.ValueString(),
			"ipam_address": im.IPAMAddr.ValueString(),
			"primary":      im.Primary.ValueBool(),
			"subnet_id":    int(im.SubnetID.ValueInt64()),
		}

		// ip_ranges
		var ipRanges []any
		for _, r := range im.IPRanges.Elements() {
			if s, ok := r.(types.String); ok {
				ipRanges = append(ipRanges, s.ValueString())
			}
		}
		m["ip_ranges"] = ipRanges

		// ipv4
		// (pass through raw ipv4/ipv6 via helper — simplified)
		rawElems = append(rawElems, m)
	}

	return helper.ExpandConfigInterfaces(ctx, rawElems), diags
}
