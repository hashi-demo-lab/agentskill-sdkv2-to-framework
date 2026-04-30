package openstack

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/db/v1/users"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseUserV1Resource{}
	_ resource.ResourceWithConfigure   = &databaseUserV1Resource{}
	_ resource.ResourceWithImportState = &databaseUserV1Resource{}
)

// NewDatabaseUserV1Resource is the resource factory used by the framework
// provider plumbing to register openstack_db_user_v1.
func NewDatabaseUserV1Resource() resource.Resource {
	return &databaseUserV1Resource{}
}

type databaseUserV1Resource struct {
	config *Config
}

// databaseUserV1Model maps schema attributes to a typed Go struct.
//
// NOTE on Password: this is now a write-only attribute. Terraform does not
// persist it to state; it is supplied via req.Config in CRUD methods and is
// null on req.Plan / req.State. See the SKILL reference
// `sensitive-and-writeonly.md`.
type databaseUserV1Model struct {
	ID         types.String   `tfsdk:"id"`
	Region     types.String   `tfsdk:"region"`
	Name       types.String   `tfsdk:"name"`
	InstanceID types.String   `tfsdk:"instance_id"`
	Password   types.String   `tfsdk:"password"`
	Host       types.String   `tfsdk:"host"`
	Databases  types.Set      `tfsdk:"databases"`
	Timeouts   timeouts.Value `tfsdk:"timeouts"`
}

func (r *databaseUserV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_db_user_v1"
}

func (r *databaseUserV1Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
			"instance_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// password: Sensitive + WriteOnly. The value is supplied via
			// config but is never persisted to state. This is a
			// practitioner-visible breaking change vs the previous
			// (Sensitive-but-stored) shape and is appropriate for a major
			// version bump. WriteOnly cannot be combined with Computed
			// (the framework rejects that combination at provider boot).
			"password": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				WriteOnly: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"host": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"databases": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				// Set: schema.HashString from SDKv2 is dropped — the
				// framework SetAttribute handles uniqueness internally.
				// SDKv2 had no ForceNew on databases, so no plan
				// modifiers are needed here.
			},
		},
		Blocks: map[string]schema.Block{
			// Preserve the SDKv2 `timeouts { create = "..."; delete = "..." }`
			// block syntax for backward compatibility. Defaults are applied
			// per-CRUD method, not on the schema.
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *databaseUserV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *Config, got %T. Please report this to the provider authors.", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *databaseUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Read everything from Config — `password` is WriteOnly and is only
	// available on req.Config, never on req.Plan.
	var data databaseUserV1Model

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := data.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	region := r.regionFromModel(data)

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())

		return
	}

	userName := data.Name.ValueString()
	instanceID := data.InstanceID.ValueString()

	rawDatabases := make([]any, 0)

	if !data.Databases.IsNull() && !data.Databases.IsUnknown() {
		var dbs []string

		resp.Diagnostics.Append(data.Databases.ElementsAs(ctx, &dbs, false)...)

		if resp.Diagnostics.HasError() {
			return
		}

		for _, d := range dbs {
			rawDatabases = append(rawDatabases, d)
		}
	}

	var usersList users.BatchCreateOpts

	usersList = append(usersList, users.CreateOpts{
		Name:      userName,
		Password:  data.Password.ValueString(),
		Host:      data.Host.ValueString(),
		Databases: expandDatabaseUserV1Databases(rawDatabases),
	})

	if err := users.Create(ctx, databaseV1Client, instanceID, usersList).ExtractErr(); err != nil {
		resp.Diagnostics.AddError("Error creating openstack_db_user_v1", err.Error())

		return
	}

	stateConf := &retry.StateChangeConf{
		Pending:    []string{"BUILD"},
		Target:     []string{"ACTIVE"},
		Refresh:    databaseUserV1StateRefreshFunc(ctx, databaseV1Client, instanceID, userName),
		Timeout:    createTimeout,
		Delay:      0,
		MinTimeout: 3 * time.Second,
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_db_user_v1 %s to be created", userName),
			err.Error(),
		)

		return
	}

	id := fmt.Sprintf("%s/%s", instanceID, userName)

	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading back openstack_db_user_v1 %s", id),
			err.Error(),
		)

		return
	}

	if !exists {
		resp.Diagnostics.AddError(
			"openstack_db_user_v1 not found after create",
			fmt.Sprintf("user %q on instance %q was not found in API response", userName, instanceID),
		)

		return
	}

	dbNames := flattenDatabaseUserV1Databases(userObj.Databases)

	dbSet, dbDiags := types.SetValueFrom(ctx, types.StringType, dbNames)
	resp.Diagnostics.Append(dbDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	state := databaseUserV1Model{
		ID:         types.StringValue(id),
		Region:     types.StringValue(region),
		Name:       types.StringValue(userName),
		InstanceID: types.StringValue(instanceID),
		Password:   types.StringNull(), // WriteOnly — not persisted.
		Host:       data.Host,
		Databases:  dbSet,
		Timeouts:   data.Timeouts,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *databaseUserV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseUserV1Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromModel(state)

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())

		return
	}

	instanceID, userName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_user_v1")
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource ID", err.Error())

		return
	}

	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error checking if openstack_db_user_v1 %s exists", state.ID.ValueString()),
			err.Error(),
		)

		return
	}

	if !exists {
		resp.State.RemoveResource(ctx)

		return
	}

	dbNames := flattenDatabaseUserV1Databases(userObj.Databases)

	dbSet, dbDiags := types.SetValueFrom(ctx, types.StringType, dbNames)
	resp.Diagnostics.Append(dbDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	state.Name = types.StringValue(userName)
	state.InstanceID = types.StringValue(instanceID)
	state.Region = types.StringValue(region)
	state.Databases = dbSet
	// Password is WriteOnly — leave whatever's in state (null) untouched.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable in practice — every user-visible attribute carries
// RequiresReplace, so any change forces destroy/create. Implemented to
// satisfy the resource.Resource interface.
func (r *databaseUserV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseUserV1Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseUserV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseUserV1Model

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

	region := r.regionFromModel(state)

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())

		return
	}

	instanceID, userName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_user_v1")
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource ID", err.Error())

		return
	}

	exists, _, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error checking if openstack_db_user_v1 %s exists", state.ID.ValueString()),
			err.Error(),
		)

		return
	}

	if !exists {
		return
	}

	if err := users.Delete(ctx, databaseV1Client, instanceID, userName).ExtractErr(); err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_db_user_v1 %s", state.ID.ValueString()),
			err.Error(),
		)

		return
	}
}

// ImportState supports `terraform import openstack_db_user_v1.foo
// <instance_id>/<user_name>`. The composite ID is also written to
// `instance_id` and `name` so Read can locate the user without re-parsing.
func (r *databaseUserV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected '<instance_id>/<user_name>', got %q", req.ID),
		)

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}

// regionFromModel mirrors the SDKv2 helper GetRegion: returns the resource's
// region if set/known, otherwise falls back to the provider-configured region.
func (r *databaseUserV1Resource) regionFromModel(m databaseUserV1Model) string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}

	if r.config != nil {
		return r.config.Region
	}

	return ""
}
