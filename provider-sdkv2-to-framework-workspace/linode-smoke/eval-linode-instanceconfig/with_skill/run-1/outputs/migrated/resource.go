package instanceconfig

import (
	"context"
	"fmt"
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

// Ensure interface compliance.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithConfigure   = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// NewResource returns a new framework resource for linode_instance_config.
func NewResource() resource.Resource {
	return &Resource{}
}

// Resource implements resource.Resource for linode_instance_config.
type Resource struct {
	Meta *helper.FrameworkProviderMeta
}

// ---------------------------------------------------------------------------
// Model structs
// ---------------------------------------------------------------------------

type ResourceModel struct {
	ID          types.String `tfsdk:"id"`
	LinodeID    types.Int64  `tfsdk:"linode_id"`
	Label       types.String `tfsdk:"label"`
	Comments    types.String `tfsdk:"comments"`
	Kernel      types.String `tfsdk:"kernel"`
	MemoryLimit types.Int64  `tfsdk:"memory_limit"`
	RootDevice  types.String `tfsdk:"root_device"`
	RunLevel    types.String `tfsdk:"run_level"`
	VirtMode    types.String `tfsdk:"virt_mode"`
	Booted      types.Bool   `tfsdk:"booted"`

	// Blocks
	Device    types.Set  `tfsdk:"device"`    // SetNestedBlock  – new preferred style
	Devices   types.List `tfsdk:"devices"`   // ListNestedBlock MaxItems:1 – deprecated
	Helpers   types.List `tfsdk:"helpers"`   // ListNestedBlock (0-1)
	Interface types.List `tfsdk:"interface"` // ListNestedBlock (0-n)
}

type DeviceBlockModel struct {
	DeviceName types.String `tfsdk:"device_name"`
	DiskID     types.Int64  `tfsdk:"disk_id"`
	VolumeID   types.Int64  `tfsdk:"volume_id"`
}

type DeviceSlotModel struct {
	DiskID   types.Int64 `tfsdk:"disk_id"`
	VolumeID types.Int64 `tfsdk:"volume_id"`
}

// DevicesModel is intentionally not a fixed struct because the device-key set
// is large (64 entries, sda..sdbl) and dynamically generated. The `devices`
// block element is accessed as a types.Object with a dynamic attr-type map.

type HelpersModel struct {
	DevtmpfsAutomount types.Bool `tfsdk:"devtmpfs_automount"`
	Distro            types.Bool `tfsdk:"distro"`
	ModulesDep        types.Bool `tfsdk:"modules_dep"`
	Network           types.Bool `tfsdk:"network"`
	UpdatedbDisabled  types.Bool `tfsdk:"updatedb_disabled"`
}

// ---------------------------------------------------------------------------
// Metadata / Schema / Configure
// ---------------------------------------------------------------------------

func (r *Resource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_instance_config"
}

func (r *Resource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	r.Meta = helper.GetResourceMeta(req, resp)
}

// deviceSlotAttrTypes returns the attr.Type map for a single DeviceSlot nested block object.
func deviceSlotAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"disk_id":   types.Int64Type,
		"volume_id": types.Int64Type,
	}
}

// deviceSlotListType returns the types.ListType for a single-slot list.
func deviceSlotListType() types.ListType {
	return types.ListType{ElemType: types.ObjectType{AttrTypes: deviceSlotAttrTypes()}}
}

// deviceBlockAttrTypes returns the attr.Type map for the `device` set element.
func deviceBlockAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"device_name": types.StringType,
		"disk_id":     types.Int64Type,
		"volume_id":   types.Int64Type,
	}
}

// devicesAttrTypes returns the attr.Type map for the `devices` (named-map) block object.
func devicesAttrTypes() map[string]attr.Type {
	slotListType := deviceSlotListType()
	result := make(map[string]attr.Type)
	for _, key := range helper.GetConfigDeviceKeys() {
		result[key] = slotListType
	}
	return result
}

func helpersAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"devtmpfs_automount": types.BoolType,
		"distro":             types.BoolType,
		"modules_dep":        types.BoolType,
		"network":            types.BoolType,
		"updatedb_disabled":  types.BoolType,
	}
}

func (r *Resource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	deviceKeys := helper.GetConfigDeviceKeys()
	devicesNestedAttrs := make(map[string]schema.Attribute, len(deviceKeys))
	for _, key := range deviceKeys {
		devicesNestedAttrs[key] = schema.ListNestedAttribute{
			Optional:    true,
			Description: "Device can be either a Disk or Volume identified by disk_id or volume_id. Only one type per slot allowed.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"disk_id": schema.Int64Attribute{
						Optional:    true,
						Description: "The Disk ID to map to this disk slot",
					},
					"volume_id": schema.Int64Attribute{
						Optional:    true,
						Description: "The Block Storage volume ID to map to this disk slot",
					},
				},
			},
		}
	}

	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique ID of the Instance Config.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"linode_id": schema.Int64Attribute{
				Required:    true,
				Description: "The ID of the Linode to create this configuration profile under.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"label": schema.StringAttribute{
				Required:    true,
				Description: "The Config's label for display purposes only.",
			},
			"comments": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional field for arbitrary User comments on this Config.",
			},
			"kernel": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "A Kernel ID to boot a Linode with. Defaults to \"linode/latest-64bit\".",
				Default:     stringdefault.StaticString("linode/latest-64bit"),
			},
			"memory_limit": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The memory limit of the Linode.",
			},
			"root_device": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The root device to boot. If no value or an invalid value is provided, root device will default to /dev/sda.",
				Default:     stringdefault.StaticString("/dev/sda"),
			},
			"run_level": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Defines the state of your Linode after booting.",
				Default:     stringdefault.StaticString("default"),
				Validators: []validator.String{
					stringvalidator.OneOfCaseInsensitive("default", "single", "binbash"),
				},
			},
			"virt_mode": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Controls the virtualization mode.",
				Default:     stringdefault.StaticString("paravirt"),
				Validators: []validator.String{
					stringvalidator.OneOfCaseInsensitive("paravirt", "fullvirt"),
				},
			},
			"booted": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "If true, the Linode will be booted to running state. If false, the Linode will be shutdown. If undefined, no action will be taken.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			// device: new preferred set block — conflicts with devices
			"device": schema.SetNestedBlock{
				Description: "Blocks for device disks in a Linode's configuration profile.",
				Validators: []validator.Set{
					setvalidator.ConflictsWith(path.MatchRoot("devices")),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"device_name": schema.StringAttribute{
							Required:    true,
							Description: "The device slot identifier (for example, sda, sdb) to map a disk or volume into",
							Validators: []validator.String{
								stringvalidator.OneOf(helper.GetConfigDeviceKeys()...),
							},
						},
						"disk_id": schema.Int64Attribute{
							Optional:    true,
							Description: "The Disk ID to map to this disk slot",
						},
						"volume_id": schema.Int64Attribute{
							Optional:    true,
							Description: "The Block Storage volume ID to map to this disk slot",
						},
					},
				},
			},

			// devices: deprecated named-map block (MaxItems:1 → ListNestedBlock + SizeAtMost(1))
			// Kept as a block (not converted to SingleNestedAttribute) for backward HCL compat.
			"devices": schema.ListNestedBlock{
				Description:        "A dictionary of device disks to use as a device map in a Linode's configuration profile. Deprecated: use `device` instead.",
				DeprecationMessage: "Devices attribute is deprecated in favor of `device`.",
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
					listvalidator.ConflictsWith(path.MatchRoot("device")),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: devicesNestedAttrs,
				},
			},

			// helpers: 0-1 helper settings block
			"helpers": schema.ListNestedBlock{
				Description: "Helpers enabled when booting to this Linode Config.",
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"devtmpfs_automount": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Populates the /dev directory early during boot without udev.",
							Default:     booldefault.StaticBool(true),
						},
						"distro": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Helps maintain correct inittab/upstart console device.",
							Default:     booldefault.StaticBool(true),
						},
						"modules_dep": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Creates a modules dependency file for the Kernel you run.",
							Default:     booldefault.StaticBool(true),
						},
						"network": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Automatically configures static networking.",
							Default:     booldefault.StaticBool(true),
						},
						"updatedb_disabled": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Description: "Disables updatedb cron job to avoid disk thrashing.",
							Default:     booldefault.StaticBool(true),
						},
					},
				},
			},

			// interface: 0-n interfaces
			"interface": schema.ListNestedBlock{
				Description: "An array of Network Interfaces to add to this Linode's Configuration Profile.",
				NestedObject: schema.NestedBlockObject{
					Attributes: interfaceSchemaAttributes(),
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_instance_config")

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client
	linodeID := int(plan.LinodeID.ValueInt64())

	createOpts := linodego.InstanceConfigCreateOptions{
		Label:       plan.Label.ValueString(),
		Comments:    plan.Comments.ValueString(),
		Kernel:      plan.Kernel.ValueString(),
		MemoryLimit: int(plan.MemoryLimit.ValueInt64()),
		RunLevel:    plan.RunLevel.ValueString(),
		VirtMode:    plan.VirtMode.ValueString(),
	}

	if !plan.RootDevice.IsNull() && !plan.RootDevice.IsUnknown() {
		v := plan.RootDevice.ValueString()
		createOpts.RootDevice = &v
	}

	// Expand helpers
	if !plan.Helpers.IsNull() && !plan.Helpers.IsUnknown() {
		var helpers []HelpersModel
		resp.Diagnostics.Append(plan.Helpers.ElementsAs(ctx, &helpers, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(helpers) > 0 {
			createOpts.Helpers = expandHelpersModel(helpers[0])
		}
	}

	// Expand interfaces
	ifaces, d := expandInterfacesFromList(ctx, plan.Interface)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	createOpts.Interfaces = ifaces

	// Expand devices — prefer `device` (set) over `devices` (deprecated named block)
	devices, d := expandDevicesFromPlan(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if devices != nil {
		createOpts.Devices = *devices
	}

	tflog.Debug(ctx, "client.CreateInstanceConfig(...)", map[string]any{"options": createOpts})
	cfg, err := client.CreateInstanceConfig(ctx, linodeID, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create Linode instance config", err.Error())
		return
	}

	plan.ID = types.StringValue(strconv.Itoa(cfg.ID))

	// Apply boot status if `booted` was set in config
	var configBooted types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("booted"), &configBooted)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !configBooted.IsNull() {
		deadlineSeconds := getDeadlineSeconds(ctx)
		if err := applyBootStatus(ctx, &client, linodeID, cfg.ID, deadlineSeconds,
			plan.Booted.ValueBool(), false); err != nil {
			resp.Diagnostics.AddError("Failed to update boot status", err.Error())
			return
		}
	}

	// Refresh from API
	d = r.readInto(ctx, &plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_instance_config")

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFramework(ctx, state)

	d := r.readInto(ctx, &state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.ValueString() == "" {
		// readInto signals removal by clearing ID
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_instance_config")

	var plan, state ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse config ID", err.Error())
		return
	}
	linodeID := int(plan.LinodeID.ValueInt64())

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{
		"id":        id,
		"linode_id": linodeID,
	})

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

	if !plan.Device.Equal(state.Device) {
		devices, d := expandDeviceSetFromList(ctx, plan.Device)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		putRequest.Devices = &devices
		shouldUpdate = true
	}

	if !plan.Devices.Equal(state.Devices) {
		devices, d := expandDevicesNamedFromList(ctx, plan.Devices)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		if devices != nil {
			putRequest.Devices = devices
		}
		shouldUpdate = true
	}

	if !plan.Helpers.Equal(state.Helpers) {
		var helpers []HelpersModel
		resp.Diagnostics.Append(plan.Helpers.ElementsAs(ctx, &helpers, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(helpers) > 0 {
			putRequest.Helpers = expandHelpersModel(helpers[0])
		}
		shouldUpdate = true
	}

	// Determine if VPC power-off is needed
	inst, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		resp.Diagnostics.AddError("Error finding the specified Linode Instance", err.Error())
		return
	}

	bootedConfigID, err := helper.GetCurrentBootedConfig(ctx, &client, linodeID)
	if err != nil {
		tflog.Warn(ctx, fmt.Sprintf("failed to get current booted config of Linode %d", linodeID))
	}

	isBootedConfig := bootedConfigID == id && inst.Status == linodego.InstanceRunning

	powerOffRequired := false
	if !plan.Interface.Equal(state.Interface) {
		ifaces, d := expandInterfacesFromList(ctx, plan.Interface)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		putRequest.Interfaces = ifaces

		config, err := client.GetInstanceConfig(ctx, linodeID, id)
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to get config %d", id), err.Error())
			return
		}
		powerOffRequired = instancehelpers.VPCInterfaceIncluded(config.Interfaces, putRequest.Interfaces) && isBootedConfig
		shouldUpdate = true
	}

	// Check whether booted was set in config (manages boot)
	var configBooted types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("booted"), &configBooted)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managedBoot := !configBooted.IsNull()
	shouldPowerBackOn := !managedBoot && powerOffRequired

	deadlineSeconds := getDeadlineSeconds(ctx)

	if shouldUpdate {
		if powerOffRequired {
			if err := instancehelpers.ShutdownInstanceForOfflineOperation(
				ctx, &client, r.Meta.Config.SkipImplicitReboots.ValueBool(), linodeID, deadlineSeconds,
				"VPC interface update",
			); err != nil {
				resp.Diagnostics.AddError("Failed to shutdown Linode instance for VPC interface update", err.Error())
				return
			}
		}

		tflog.Debug(ctx, "client.UpdateInstanceConfig(...)", map[string]any{"options": putRequest})
		if _, err := client.UpdateInstanceConfig(ctx, linodeID, id, putRequest); err != nil {
			resp.Diagnostics.AddError("Failed to update instance config", err.Error())
			return
		}

		if shouldPowerBackOn {
			if err := helper.BootInstanceSync(ctx, &client, linodeID, id, deadlineSeconds); err != nil {
				resp.Diagnostics.AddError("Failed to boot instance after VPC interface update", err.Error())
				return
			}
		}
	}

	shouldReboot := isBootedConfig && shouldUpdate && !powerOffRequired && !r.Meta.Config.SkipImplicitReboots.ValueBool()
	if managedBoot {
		if err := applyBootStatus(ctx, &client, linodeID, id, deadlineSeconds,
			plan.Booted.ValueBool(), shouldReboot); err != nil {
			resp.Diagnostics.AddError("Failed to update boot status", err.Error())
			return
		}
	}

	// Refresh from API
	d := r.readInto(ctx, &plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_instance_config")

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFramework(ctx, state)

	client := r.Meta.Client
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to parse config ID", err.Error())
		return
	}
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
			fmt.Sprintf("Can't delete config %d in Linode %d", id, linodeID),
			"the resource lock on the Linode prohibits deletion of its subresources, which includes this config",
		)
		return
	}

	deadlineSeconds := getDeadlineSeconds(ctx)

	if booted, err := isConfigBooted(ctx, &client, inst, id, state.Booted.ValueBool()); err != nil {
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
		if _, err := p.WaitForFinished(ctx, deadlineSeconds); err != nil {
			resp.Diagnostics.AddError("Failed to wait for instance shutdown", err.Error())
			return
		}
		tflog.Debug(ctx, "Instance shutdown complete")
	}

	tflog.Debug(ctx, "client.DeleteInstanceConfig(...)")
	if err := client.DeleteInstanceConfig(ctx, linodeID, id); err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error deleting Linode Instance Config %d", id), err.Error())
		return
	}
}

func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	tflog.Debug(ctx, "Import linode_instance_config", map[string]any{"id": req.ID})

	if !strings.Contains(req.ID, ",") {
		resp.Diagnostics.AddError(
			"Invalid import ID format",
			"Expected format: <linode_id>,<config_id>",
		)
		return
	}

	parts := strings.SplitN(req.ID, ",", 2)

	linodeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Linode ID in import string", err.Error())
		return
	}
	configID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid config ID in import string", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.FormatInt(configID, 10))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("linode_id"), linodeID)...)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// readInto fetches the resource from the API and populates plan/state.
// Sets plan.ID to "" and returns no error if the resource no longer exists.
func (r *Resource) readInto(ctx context.Context, m *ResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	client := r.Meta.Client

	id, err := strconv.Atoi(m.ID.ValueString())
	if err != nil {
		diags.AddError("Failed to parse config ID", err.Error())
		return diags
	}
	linodeID := int(m.LinodeID.ValueInt64())

	cfg, err := client.GetInstanceConfig(ctx, linodeID, id)
	if linodego.IsNotFound(err) {
		tflog.Warn(ctx, fmt.Sprintf(
			"removing Instance Config ID %q from state because it no longer exists", m.ID.ValueString(),
		))
		m.ID = types.StringValue("")
		return diags
	}
	if err != nil {
		diags.AddError("Failed to get instance config", err.Error())
		return diags
	}

	inst, err := client.GetInstance(ctx, linodeID)
	if err != nil {
		diags.AddError("Failed to get instance", err.Error())
		return diags
	}

	instNetworking, err := client.GetInstanceIPAddresses(ctx, linodeID)
	if err != nil {
		diags.AddError("Failed to get instance networking", err.Error())
		return diags
	}

	configBooted, err := isConfigBooted(ctx, &client, inst, cfg.ID, m.Booted.ValueBool())
	if err != nil {
		diags.AddError("Failed to check instance boot status", err.Error())
		return diags
	}

	m.ID = types.StringValue(strconv.Itoa(cfg.ID))
	m.LinodeID = types.Int64Value(int64(linodeID))
	m.Label = types.StringValue(cfg.Label)
	m.Comments = types.StringValue(cfg.Comments)
	m.Kernel = types.StringValue(cfg.Kernel)
	m.MemoryLimit = types.Int64Value(int64(cfg.MemoryLimit))
	m.RootDevice = types.StringValue(cfg.RootDevice)
	m.RunLevel = types.StringValue(cfg.RunLevel)
	m.VirtMode = types.StringValue(cfg.VirtMode)
	m.Booted = types.BoolValue(configBooted)

	// Flatten interfaces
	if ifaces := helper.FlattenInterfaces(cfg.Interfaces); len(ifaces) > 0 {
		ifaceList, d := flattenInterfacesToList(ctx, ifaces)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		m.Interface = ifaceList
	} else {
		m.Interface = types.ListValueMust(
			types.ObjectType{AttrTypes: interfaceAttrTypes()},
			[]attr.Value{},
		)
	}

	// Flatten device (set) — prefer populating from API data
	if cfg.Devices != nil {
		deviceSet, d := flattenDeviceMapToSet(ctx, *cfg.Devices)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		m.Device = deviceSet

		devicesList, d := flattenDeviceMapToNamedList(ctx, *cfg.Devices)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		m.Devices = devicesList
	} else {
		m.Device = types.SetValueMust(
			types.ObjectType{AttrTypes: deviceBlockAttrTypes()},
			[]attr.Value{},
		)
		m.Devices = types.ListValueMust(
			types.ObjectType{AttrTypes: devicesAttrTypes()},
			[]attr.Value{},
		)
	}

	// Flatten helpers
	if cfg.Helpers != nil {
		helpersList, d := flattenHelpersList(ctx, *cfg.Helpers)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		m.Helpers = helpersList
	} else {
		m.Helpers = types.ListValueMust(
			types.ObjectType{AttrTypes: helpersAttrTypes()},
			[]attr.Value{},
		)
	}

	// Set connection info (best-effort; ssh host from networking)
	if instNetworking != nil && len(instNetworking.IPv4.Public) > 0 {
		tflog.Debug(ctx, "instance public IPv4 for SSH", map[string]any{
			"host": instNetworking.IPv4.Public[0].Address,
		})
	}

	return diags
}

// getDeadlineSeconds extracts remaining seconds from ctx deadline, defaulting to 15 min.
func getDeadlineSeconds(ctx context.Context) int {
	if deadline, ok := ctx.Deadline(); ok {
		return int(time.Until(deadline).Seconds())
	}
	return int((15 * time.Minute).Seconds())
}

func populateLogAttributesFramework(ctx context.Context, m ResourceModel) context.Context {
	return helper.SetLogFieldBulk(ctx, map[string]any{
		"linode_id": m.LinodeID.ValueInt64(),
		"id":        m.ID.ValueString(),
	})
}

// ---------------------------------------------------------------------------
// Expand helpers (config → API)
// ---------------------------------------------------------------------------

func expandDevicesFromPlan(ctx context.Context, plan ResourceModel) (*linodego.InstanceConfigDeviceMap, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Prefer `device` (set) if non-empty
	if !plan.Device.IsNull() && !plan.Device.IsUnknown() && len(plan.Device.Elements()) > 0 {
		dm, d := expandDeviceSetFromList(ctx, plan.Device)
		diags.Append(d...)
		return &dm, diags
	}

	// Fall back to `devices` (named block)
	if !plan.Devices.IsNull() && !plan.Devices.IsUnknown() && len(plan.Devices.Elements()) > 0 {
		dm, d := expandDevicesNamedFromList(ctx, plan.Devices)
		diags.Append(d...)
		return dm, diags
	}

	return nil, diags
}

func expandDeviceSetFromList(ctx context.Context, deviceSet types.Set) (linodego.InstanceConfigDeviceMap, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result linodego.InstanceConfigDeviceMap

	var devices []DeviceBlockModel
	diags.Append(deviceSet.ElementsAs(ctx, &devices, false)...)
	if diags.HasError() {
		return result, diags
	}

	for _, d := range devices {
		dev := linodego.InstanceConfigDevice{
			DiskID:   int(d.DiskID.ValueInt64()),
			VolumeID: int(d.VolumeID.ValueInt64()),
		}
		setDeviceOnMap(&result, d.DeviceName.ValueString(), &dev)
	}
	return result, diags
}

func expandDevicesNamedFromList(ctx context.Context, devicesList types.List) (*linodego.InstanceConfigDeviceMap, diag.Diagnostics) {
	var diags diag.Diagnostics

	if devicesList.IsNull() || devicesList.IsUnknown() || len(devicesList.Elements()) == 0 {
		return nil, diags
	}

	var result linodego.InstanceConfigDeviceMap

	// The list has at most 1 element (enforced by SizeAtMost(1)).
	// Access the element as types.Object so we can iterate dynamically over the device keys.
	elems := devicesList.Elements()
	if len(elems) == 0 {
		return nil, diags
	}

	obj, ok := elems[0].(types.Object)
	if !ok {
		diags.AddError("Unexpected element type", "Expected types.Object for devices block element")
		return nil, diags
	}

	attrs := obj.Attributes()
	for slot, slotVal := range attrs {
		slotList, ok := slotVal.(types.List)
		if !ok || slotList.IsNull() || slotList.IsUnknown() || len(slotList.Elements()) == 0 {
			continue
		}
		var slots []DeviceSlotModel
		diags.Append(slotList.ElementsAs(ctx, &slots, false)...)
		if diags.HasError() || len(slots) == 0 {
			continue
		}
		dev := linodego.InstanceConfigDevice{
			DiskID:   int(slots[0].DiskID.ValueInt64()),
			VolumeID: int(slots[0].VolumeID.ValueInt64()),
		}
		setDeviceOnMap(&result, slot, &dev)
	}

	if diags.HasError() {
		return nil, diags
	}

	return &result, diags
}

func setDeviceOnMap(dm *linodego.InstanceConfigDeviceMap, slot string, dev *linodego.InstanceConfigDevice) {
	// Use reflection to set the field by its uppercase name (e.g. "sda" → field "SDA").
	field := reflect.Indirect(reflect.ValueOf(dm)).FieldByName(strings.ToUpper(slot))
	if field.IsValid() {
		field.Set(reflect.ValueOf(dev))
	}
}

func expandHelpersModel(h HelpersModel) *linodego.InstanceConfigHelpers {
	return &linodego.InstanceConfigHelpers{
		DevTmpFsAutomount: h.DevtmpfsAutomount.ValueBool(),
		Distro:            h.Distro.ValueBool(),
		ModulesDep:        h.ModulesDep.ValueBool(),
		Network:           h.Network.ValueBool(),
		UpdateDBDisabled:  h.UpdatedbDisabled.ValueBool(),
	}
}

func expandInterfacesFromList(ctx context.Context, ifaceList types.List) ([]linodego.InstanceConfigInterfaceCreateOptions, diag.Diagnostics) {
	var diags diag.Diagnostics

	if ifaceList.IsNull() || ifaceList.IsUnknown() || len(ifaceList.Elements()) == 0 {
		return nil, diags
	}

	// Unmarshal to []map[string]any, then convert to []any for the helper.
	var rawMaps []map[string]any
	diags.Append(ifaceList.ElementsAs(ctx, &rawMaps, false)...)
	if diags.HasError() {
		return nil, diags
	}

	rawIfaces := make([]any, len(rawMaps))
	for i, m := range rawMaps {
		rawIfaces[i] = m
	}

	return helper.ExpandConfigInterfaces(ctx, rawIfaces), diags
}

// ---------------------------------------------------------------------------
// Flatten helpers (API → state)
// ---------------------------------------------------------------------------

func flattenDeviceMapToSet(ctx context.Context, dm linodego.InstanceConfigDeviceMap) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: deviceBlockAttrTypes()}

	fields := getDeviceMapFieldsAPI(dm)
	elems := make([]attr.Value, 0, len(fields))
	for _, pair := range fields {
		name := pair[0].(string)
		dev := pair[1].(*linodego.InstanceConfigDevice)
		obj, d := types.ObjectValue(deviceBlockAttrTypes(), map[string]attr.Value{
			"device_name": types.StringValue(name),
			"disk_id":     types.Int64Value(int64(dev.DiskID)),
			"volume_id":   types.Int64Value(int64(dev.VolumeID)),
		})
		diags.Append(d...)
		elems = append(elems, obj)
	}
	if diags.HasError() {
		return types.SetNull(elemType), diags
	}
	return types.SetValue(elemType, elems)
}

func flattenDeviceMapToNamedList(ctx context.Context, dm linodego.InstanceConfigDeviceMap) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	devicesObjType := types.ObjectType{AttrTypes: devicesAttrTypes()}
	listType := types.ListType{ElemType: devicesObjType}

	slotAttrs := make(map[string]attr.Value)
	for _, key := range helper.GetConfigDeviceKeys() {
		slotAttrs[key] = types.ListValueMust(
			types.ObjectType{AttrTypes: deviceSlotAttrTypes()},
			[]attr.Value{},
		)
	}

	for _, pair := range getDeviceMapFieldsAPI(dm) {
		name := pair[0].(string)
		dev := pair[1].(*linodego.InstanceConfigDevice)
		slotObj, d := types.ObjectValue(deviceSlotAttrTypes(), map[string]attr.Value{
			"disk_id":   types.Int64Value(int64(dev.DiskID)),
			"volume_id": types.Int64Value(int64(dev.VolumeID)),
		})
		diags.Append(d...)
		slotAttrs[name] = types.ListValueMust(
			types.ObjectType{AttrTypes: deviceSlotAttrTypes()},
			[]attr.Value{slotObj},
		)
	}
	if diags.HasError() {
		return types.ListNull(devicesObjType), diags
	}

	obj, d := types.ObjectValue(devicesAttrTypes(), slotAttrs)
	diags.Append(d...)
	if diags.HasError() {
		return types.ListNull(devicesObjType), diags
	}
	return types.ListValue(listType.ElemType, []attr.Value{obj})
}

func flattenHelpersList(ctx context.Context, h linodego.InstanceConfigHelpers) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	objType := types.ObjectType{AttrTypes: helpersAttrTypes()}
	obj, d := types.ObjectValue(helpersAttrTypes(), map[string]attr.Value{
		"devtmpfs_automount": types.BoolValue(h.DevTmpFsAutomount),
		"distro":             types.BoolValue(h.Distro),
		"modules_dep":        types.BoolValue(h.ModulesDep),
		"network":            types.BoolValue(h.Network),
		"updatedb_disabled":  types.BoolValue(h.UpdateDBDisabled),
	})
	diags.Append(d...)
	if diags.HasError() {
		return types.ListNull(objType), diags
	}
	return types.ListValue(objType, []attr.Value{obj})
}

// getDeviceMapFieldsAPI returns non-nil device slots from a DeviceMap using reflection.
// Field names are lowercased (e.g. "SDA" → "sda") to match Linode API slot names.
func getDeviceMapFieldsAPI(dm linodego.InstanceConfigDeviceMap) [][2]any {
	reflectMap := reflect.ValueOf(dm)
	result := make([][2]any, 0)
	for i := 0; i < reflectMap.NumField(); i++ {
		field := reflectMap.Field(i).Interface().(*linodego.InstanceConfigDevice)
		if field == nil {
			continue
		}
		fieldName := strings.ToLower(reflectMap.Type().Field(i).Name)
		result = append(result, [2]any{fieldName, field})
	}
	return result
}

// ---------------------------------------------------------------------------
// Interface schema and flatten helpers (forwarded to instance helper maps)
// ---------------------------------------------------------------------------

// interfaceAttrTypes returns attr.Type for an interface block element.
// This must match the NestedObject attributes defined in Schema().
func interfaceAttrTypes() map[string]attr.Type {
	// Minimal set matching what FlattenInterface returns.
	// Adjust if the instance.InterfaceSchema has changed.
	return map[string]attr.Type{
		"purpose":      types.StringType,
		"ipam_address": types.StringType,
		"label":        types.StringType,
		"id":           types.Int64Type,
		"vpc_id":       types.Int64Type,
		"subnet_id":    types.Int64Type,
		"primary":      types.BoolType,
		"active":       types.BoolType,
		"ip_ranges":    types.ListType{ElemType: types.StringType},
		"ipv4": types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
			"vpc":     types.StringType,
			"nat_1_1": types.StringType,
		}}},
		"ipv6": types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
			"is_public": types.BoolType,
			"slaac": types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
				"range":          types.StringType,
				"assigned_range": types.StringType,
				"address":        types.StringType,
			}}},
			"range": types.ListType{ElemType: types.ObjectType{AttrTypes: map[string]attr.Type{
				"range":          types.StringType,
				"assigned_range": types.StringType,
			}}},
		}}},
	}
}

// flattenInterfacesToList converts []map[string]any from helper.FlattenInterfaces
// into a typed types.List for the framework model.
func flattenInterfacesToList(ctx context.Context, ifaces []map[string]any) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := types.ObjectType{AttrTypes: interfaceAttrTypes()}

	elems := make([]attr.Value, 0, len(ifaces))
	for _, iface := range ifaces {
		attrVals, d := ifaceMapToAttrValues(ctx, iface)
		diags.Append(d...)
		if diags.HasError() {
			return types.ListNull(elemType), diags
		}
		obj, d := types.ObjectValue(interfaceAttrTypes(), attrVals)
		diags.Append(d...)
		elems = append(elems, obj)
	}
	if diags.HasError() {
		return types.ListNull(elemType), diags
	}
	return types.ListValue(elemType, elems)
}

func ifaceMapToAttrValues(ctx context.Context, m map[string]any) (map[string]attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics

	ipv4ObjType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"vpc":     types.StringType,
		"nat_1_1": types.StringType,
	}}
	ipv6SlaacElemType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"range": types.StringType, "assigned_range": types.StringType, "address": types.StringType,
	}}
	ipv6RangeElemType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"range": types.StringType, "assigned_range": types.StringType,
	}}
	ipv6ObjType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"is_public": types.BoolType,
		"slaac":     types.ListType{ElemType: ipv6SlaacElemType},
		"range":     types.ListType{ElemType: ipv6RangeElemType},
	}}

	ipRanges := types.ListValueMust(types.StringType, []attr.Value{})
	if v, ok := m["ip_ranges"]; ok && v != nil {
		switch vt := v.(type) {
		case []string:
			elems := make([]attr.Value, len(vt))
			for i, s := range vt {
				elems[i] = types.StringValue(s)
			}
			ipRanges = types.ListValueMust(types.StringType, elems)
		case []any:
			elems := make([]attr.Value, len(vt))
			for i, s := range vt {
				elems[i] = types.StringValue(fmt.Sprintf("%v", s))
			}
			ipRanges = types.ListValueMust(types.StringType, elems)
		}
	}

	ipv4Val := types.ListValueMust(ipv4ObjType, []attr.Value{})
	if v, ok := m["ipv4"]; ok && v != nil {
		if rawSlice, ok := v.([]map[string]any); ok && len(rawSlice) > 0 {
			s := rawSlice[0]
			obj, d := types.ObjectValue(ipv4ObjType.AttrTypes, map[string]attr.Value{
				"vpc":     types.StringValue(fmt.Sprintf("%v", s["vpc"])),
				"nat_1_1": types.StringValue(fmt.Sprintf("%v", s["nat_1_1"])),
			})
			diags.Append(d...)
			ipv4Val = types.ListValueMust(ipv4ObjType, []attr.Value{obj})
		}
	}

	ipv6Val := types.ListValueMust(ipv6ObjType, []attr.Value{})
	if v, ok := m["ipv6"]; ok && v != nil {
		if rawSlice, ok := v.([]map[string]any); ok && len(rawSlice) > 0 {
			s := rawSlice[0]
			isPublic := false
			if ip, ok := s["is_public"].(bool); ok {
				isPublic = ip
			}

			slaacList := types.ListValueMust(ipv6SlaacElemType, []attr.Value{})
			if slaacs, ok := s["slaac"].([]map[string]any); ok {
				slaacElems := make([]attr.Value, 0, len(slaacs))
				for _, sl := range slaacs {
					obj, d := types.ObjectValue(ipv6SlaacElemType.AttrTypes, map[string]attr.Value{
						"range":          types.StringValue(fmt.Sprintf("%v", sl["range"])),
						"assigned_range": types.StringValue(fmt.Sprintf("%v", sl["assigned_range"])),
						"address":        types.StringValue(fmt.Sprintf("%v", sl["address"])),
					})
					diags.Append(d...)
					slaacElems = append(slaacElems, obj)
				}
				slaacList = types.ListValueMust(ipv6SlaacElemType, slaacElems)
			}

			rangeList := types.ListValueMust(ipv6RangeElemType, []attr.Value{})
			if ranges, ok := s["range"].([]map[string]any); ok {
				rangeElems := make([]attr.Value, 0, len(ranges))
				for _, rng := range ranges {
					obj, d := types.ObjectValue(ipv6RangeElemType.AttrTypes, map[string]attr.Value{
						"range":          types.StringValue(fmt.Sprintf("%v", rng["range"])),
						"assigned_range": types.StringValue(fmt.Sprintf("%v", rng["assigned_range"])),
					})
					diags.Append(d...)
					rangeElems = append(rangeElems, obj)
				}
				rangeList = types.ListValueMust(ipv6RangeElemType, rangeElems)
			}

			obj, d := types.ObjectValue(ipv6ObjType.AttrTypes, map[string]attr.Value{
				"is_public": types.BoolValue(isPublic),
				"slaac":     slaacList,
				"range":     rangeList,
			})
			diags.Append(d...)
			ipv6Val = types.ListValueMust(ipv6ObjType, []attr.Value{obj})
		}
	}

	return map[string]attr.Value{
		"purpose":      types.StringValue(fmt.Sprintf("%v", m["purpose"])),
		"ipam_address": types.StringValue(fmt.Sprintf("%v", m["ipam_address"])),
		"label":        types.StringValue(fmt.Sprintf("%v", m["label"])),
		"id":           types.Int64Value(int64(asInt(m["id"]))),
		"vpc_id":       types.Int64Value(int64(asInt(m["vpc_id"]))),
		"subnet_id":    types.Int64Value(int64(asInt(m["subnet_id"]))),
		"primary":      types.BoolValue(asBool(m["primary"])),
		"active":       types.BoolValue(asBool(m["active"])),
		"ip_ranges":    ipRanges,
		"ipv4":         ipv4Val,
		"ipv6":         ipv6Val,
	}, diags
}

func asInt(v any) int {
	if v == nil {
		return 0
	}
	switch vt := v.(type) {
	case int:
		return vt
	case int64:
		return int(vt)
	case float64:
		return int(vt)
	}
	return 0
}

func asBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// interfaceSchemaAttributes returns the schema attributes for a single interface block element.
// This is a subset matching the fields that FlattenInterface returns.
func interfaceSchemaAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"purpose": schema.StringAttribute{
			Required:    true,
			Description: "The type of interface ('public', 'vlan', or 'vpc').",
		},
		"ipam_address": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "This Network Interface's private IP address in Classless Inter-Domain Routing (CIDR) notation.",
		},
		"label": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "The name of the VLAN to join. Only applies to interfaces with the 'vlan' purpose.",
		},
		"id": schema.Int64Attribute{
			Computed:    true,
			Description: "The unique ID representing this Interface.",
		},
		"vpc_id": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Description: "The ID of the VPC configured for this Interface.",
		},
		"subnet_id": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Description: "The ID of the subnet to join.",
		},
		"primary": schema.BoolAttribute{
			Optional:    true,
			Computed:    true,
			Description: "Whether the interface is the primary interface that should have the default route.",
		},
		"active": schema.BoolAttribute{
			Computed:    true,
			Description: "Returns true if the Interface is in use, meaning that Linode has been booted using the Configuration Profile to which the Interface belongs.",
		},
		"ip_ranges": schema.ListAttribute{
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
			Description: "List of VPC IPv4 ranges that can be routed to this Interface.",
		},
		"ipv4": schema.ListNestedAttribute{
			Optional:    true,
			Computed:    true,
			Description: "The IPv4 configuration of the VPC interface.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"vpc": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The IP from the VPC subnet to use for this Interface.",
					},
					"nat_1_1": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The 1:1 NAT IPv4 address to use for this Interface.",
					},
				},
			},
		},
		"ipv6": schema.ListNestedAttribute{
			Optional:    true,
			Computed:    true,
			Description: "The IPv6 configuration of the VPC interface.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"is_public": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Whether the IPv6 range is public.",
					},
					"slaac": schema.ListNestedAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The SLAAC IPv6 address for this Interface.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"range": schema.StringAttribute{
									Computed: true,
								},
								"assigned_range": schema.StringAttribute{
									Computed: true,
								},
								"address": schema.StringAttribute{
									Computed: true,
								},
							},
						},
					},
					"range": schema.ListNestedAttribute{
						Optional:    true,
						Computed:    true,
						Description: "IPv6 range that can be routed to this Interface.",
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
	}
}
