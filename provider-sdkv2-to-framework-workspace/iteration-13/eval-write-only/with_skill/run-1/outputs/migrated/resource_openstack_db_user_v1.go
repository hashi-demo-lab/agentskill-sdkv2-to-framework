package openstack

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/db/v1/users"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &databaseUserV1Resource{}
	_ resource.ResourceWithConfigure   = &databaseUserV1Resource{}
	_ resource.ResourceWithImportState = &databaseUserV1Resource{}
)

// NewDatabaseUserV1Resource returns a new framework resource for
// openstack_db_user_v1.
func NewDatabaseUserV1Resource() resource.Resource {
	return &databaseUserV1Resource{}
}

type databaseUserV1Resource struct {
	config *Config
}

// databaseUserV1Model is the resource state/plan/config model.
//
// Note: Password is a write-only attribute. It is supplied by the practitioner
// in config but is NOT persisted to state. CRUD methods read it from
// req.Config rather than req.Plan / req.State.
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

func (r *databaseUserV1Resource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_db_user_v1"
}

func (r *databaseUserV1Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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

			// Password is write-only: provided in config, never persisted to
			// state. Sensitive AND WriteOnly. WriteOnly cannot coexist with
			// Computed, so this attribute must be Required (or Optional).
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
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
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

func (r *databaseUserV1Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = config
}

func (r *databaseUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Password is WriteOnly — it is in the practitioner-supplied Config but
	// not in Plan/State. Read it from req.Config.
	var configModel databaseUserV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &configModel)...)

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

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack database client",
			err.Error(),
		)

		return
	}

	userName := plan.Name.ValueString()
	instanceID := plan.InstanceID.ValueString()

	rawDatabases := make([]string, 0, len(plan.Databases.Elements()))
	if !plan.Databases.IsNull() && !plan.Databases.IsUnknown() {
		resp.Diagnostics.Append(plan.Databases.ElementsAs(ctx, &rawDatabases, false)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}

	rawDatabasesAny := make([]any, len(rawDatabases))
	for i, v := range rawDatabases {
		rawDatabasesAny[i] = v
	}

	var usersList users.BatchCreateOpts
	usersList = append(usersList, users.CreateOpts{
		Name:      userName,
		Password:  configModel.Password.ValueString(),
		Host:      plan.Host.ValueString(),
		Databases: expandDatabaseUserV1Databases(rawDatabasesAny),
	})

	if err := users.Create(ctx, databaseV1Client, instanceID, usersList).ExtractErr(); err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_db_user_v1",
			err.Error(),
		)

		return
	}

	if _, err := waitForDatabaseUserV1Active(ctx, databaseV1Client, instanceID, userName, createTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_db_user_v1 %s to be created", userName),
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", instanceID, userName))
	plan.Region = types.StringValue(region)

	// Re-read databases from the API to populate Computed value.
	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error checking if openstack_db_user_v1 %s exists", plan.ID.ValueString()),
			err.Error(),
		)

		return
	}

	if !exists {
		resp.Diagnostics.AddError(
			"openstack_db_user_v1 not found after create",
			fmt.Sprintf("user %q on instance %q was not found post-create", userName, instanceID),
		)

		return
	}

	dbs := flattenDatabaseUserV1Databases(userObj.Databases)
	dbsSet, dbsDiags := types.SetValueFrom(ctx, types.StringType, dbs)
	resp.Diagnostics.Append(dbsDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	plan.Databases = dbsSet
	plan.Name = types.StringValue(userName)

	// Password is WriteOnly — clear it before writing state.
	plan.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
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

	dbs := flattenDatabaseUserV1Databases(userObj.Databases)
	dbsSet, dbsDiags := types.SetValueFrom(ctx, types.StringType, dbs)
	resp.Diagnostics.Append(dbsDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	state.Databases = dbsSet

	// Password is WriteOnly — never round-tripped through state.
	state.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *databaseUserV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All non-computed user-facing attributes (name, instance_id, password,
	// host, region) carry RequiresReplace, so any user-driven change forces
	// recreation. There is no in-place update path; this method exists only
	// to satisfy the Resource interface.
	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Password is WriteOnly — clear it before writing state.
	plan.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseUserV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Note: Plan is null on Delete; read prior state instead.
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
		// Mirror SDKv2 CheckDeleted behaviour: a 404 here means already gone.
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return
		}

		resp.Diagnostics.AddError(
			"Error deleting openstack_db_user_v1",
			err.Error(),
		)

		return
	}
}

func (r *databaseUserV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// waitForDatabaseUserV1Active polls until the named user reaches the ACTIVE
// state on the given instance. Replaces SDKv2's retry.StateChangeConf for this
// resource so the migrated file does not import terraform-plugin-sdk/v2.
func waitForDatabaseUserV1Active(ctx context.Context, client *gophercloud.ServiceClient, instanceID, userName string, timeout time.Duration) (any, error) {
	pending := []string{"BUILD"}
	target := []string{"ACTIVE"}

	const pollInterval = 3 * time.Second

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		v, state, err := databaseUserV1Refresh(ctx, client, instanceID, userName)
		if err != nil {
			return v, err
		}

		if slices.Contains(target, state) {
			return v, nil
		}

		if !slices.Contains(pending, state) {
			return v, fmt.Errorf("unexpected state %q (pending=%v, target=%v)", state, pending, target)
		}

		if time.Now().After(deadline) {
			return v, fmt.Errorf("timeout after %s waiting for user %s to reach %v (last state=%q)", timeout, userName, target, state)
		}

		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-ticker.C:
		}
	}
}

// databaseUserV1Refresh reports the current state of the named user on the
// instance, returning ACTIVE if found and BUILD otherwise.
func databaseUserV1Refresh(ctx context.Context, client *gophercloud.ServiceClient, instanceID, userName string) (any, string, error) {
	pages, err := users.List(client, instanceID).AllPages(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("Unable to retrieve OpenStack database users: %w", err)
	}

	allUsers, err := users.ExtractUsers(pages)
	if err != nil {
		return nil, "", fmt.Errorf("Unable to extract OpenStack database users: %w", err)
	}

	for _, v := range allUsers {
		if v.Name == userName {
			return v, "ACTIVE", nil
		}
	}

	return nil, "BUILD", nil
}
