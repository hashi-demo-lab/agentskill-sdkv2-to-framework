package openstack

import (
	"context"
	"fmt"
	"time"

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

// NewDatabaseUserV1Resource is the constructor used by the framework provider's
// Resources() method.
func NewDatabaseUserV1Resource() resource.Resource {
	return &databaseUserV1Resource{}
}

type databaseUserV1Resource struct {
	config *Config
}

// databaseUserV1Model is the typed model for openstack_db_user_v1.
//
// NOTE: Password is Sensitive + WriteOnly. WriteOnly attributes are present in
// req.Config during Create/Update but are NEVER persisted to state — so we read
// them from req.Config rather than req.Plan, and the field stays null in the
// model used to write state. This is a major-version-bump breaking change vs
// the SDKv2 form, which persisted the password (sensitive but state-resident).
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

			// Major-version bump: password is now Sensitive + WriteOnly.
			// WriteOnly cannot be combined with Computed; the value is supplied
			// via config but not stored in state. Practitioners must always
			// provide the password in config; subsequent reads will not see
			// drift because there is no state value to compare against.
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
			},
		},
		Blocks: map[string]schema.Block{
			// Preserve the SDKv2 block syntax (`timeouts { ... }`) for
			// practitioners — using timeouts.Block rather than timeouts.Attributes.
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

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)

		return
	}

	r.config = config
}

func (r *databaseUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Read plan (for non-write-only fields).
	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read config separately to retrieve the WriteOnly password — it is null
	// in plan/state but present in config. This split is the framework's
	// supported way to use a WriteOnly value in CRUD methods.
	var configModel databaseUserV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &configModel)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := plan.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack database client",
			err.Error(),
		)

		return
	}

	// Apply the create timeout to the context.
	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	userName := plan.Name.ValueString()
	instanceID := plan.InstanceID.ValueString()

	// Expand the databases Set into the gophercloud BatchCreateOpts.
	rawDatabases := make([]any, 0, len(plan.Databases.Elements()))

	if !plan.Databases.IsNull() && !plan.Databases.IsUnknown() {
		var dbs []string

		resp.Diagnostics.Append(plan.Databases.ElementsAs(ctx, &dbs, false)...)
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
		Password:  configModel.Password.ValueString(),
		Host:      plan.Host.ValueString(),
		Databases: expandDatabaseUserV1Databases(rawDatabases),
	})

	if err := users.Create(ctx, databaseV1Client, instanceID, usersList).ExtractErr(); err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_db_user_v1",
			err.Error(),
		)

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

	// Populate computed fields and persist state. Password is WriteOnly — leave
	// it as plan's null value; do NOT copy from configModel.
	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", instanceID, userName))
	plan.Region = types.StringValue(region)

	// Read back databases from the API for the computed side of the attribute.
	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil || !exists {
		// Surface as create failure rather than silently dropping.
		resp.Diagnostics.AddError(
			"Error reading openstack_db_user_v1 after create",
			fmt.Sprintf("user %s on instance %s could not be found post-create: %v", userName, instanceID, err),
		)

		return
	}

	dbList := flattenDatabaseUserV1Databases(userObj.Databases)

	dbSet, dbDiags := types.SetValueFrom(ctx, types.StringType, dbList)
	resp.Diagnostics.Append(dbDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Databases = dbSet

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseUserV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseUserV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack database client",
			err.Error(),
		)

		return
	}

	instanceID, userName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_user_v1")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing openstack_db_user_v1 ID",
			err.Error(),
		)

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

	state.Name = types.StringValue(userName)
	state.InstanceID = types.StringValue(instanceID)
	state.Region = types.StringValue(region)

	dbList := flattenDatabaseUserV1Databases(userObj.Databases)

	dbSet, dbDiags := types.SetValueFrom(ctx, types.StringType, dbList)
	resp.Diagnostics.Append(dbDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Databases = dbSet

	// Password remains null in state — it is WriteOnly and not round-tripped.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable in normal flow because every non-computed attribute is
// RequiresReplace. We still implement it (the framework requires the method)
// and just persist the plan as-is; no real change should ever land here.
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

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack database client",
			err.Error(),
		)

		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	instanceID, userName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_user_v1")
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing openstack_db_user_v1 ID",
			err.Error(),
		)

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
		// Already gone. Framework will remove from state on return.
		return
	}

	if err := users.Delete(ctx, databaseV1Client, instanceID, userName).ExtractErr(); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting openstack_db_user_v1",
			err.Error(),
		)

		return
	}
}

// ImportState passes through the import string (expected shape: "<instanceID>/<userName>")
// straight to the resource ID. The Read method will then parse it and populate
// the rest of state. The password attribute is WriteOnly and will not appear in
// imported state — the test harness must use ImportStateVerifyIgnore for it.
func (r *databaseUserV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
