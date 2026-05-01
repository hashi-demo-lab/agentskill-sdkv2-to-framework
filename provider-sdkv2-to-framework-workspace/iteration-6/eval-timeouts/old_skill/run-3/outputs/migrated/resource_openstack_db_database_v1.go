package openstack

import (
	"context"
	"fmt"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/db/v1/databases"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

var (
	_ resource.Resource                = &databaseDatabaseV1Resource{}
	_ resource.ResourceWithConfigure   = &databaseDatabaseV1Resource{}
	_ resource.ResourceWithImportState = &databaseDatabaseV1Resource{}
)

// NewDatabaseDatabaseV1Resource is the framework constructor for the
// openstack_db_database_v1 resource.
func NewDatabaseDatabaseV1Resource() resource.Resource {
	return &databaseDatabaseV1Resource{}
}

type databaseDatabaseV1Resource struct {
	config *Config
}

// databaseDatabaseV1Model maps the resource schema attributes to a typed
// model used by the framework's Plan/State/Config Get/Set helpers.
type databaseDatabaseV1Model struct {
	ID         types.String   `tfsdk:"id"`
	Region     types.String   `tfsdk:"region"`
	Name       types.String   `tfsdk:"name"`
	InstanceID types.String   `tfsdk:"instance_id"`
	Timeouts   timeouts.Value `tfsdk:"timeouts"`
}

func (r *databaseDatabaseV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_db_database_v1"
}

func (r *databaseDatabaseV1Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *databaseDatabaseV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *databaseDatabaseV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseDatabaseV1Model

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

	region := r.regionFromPlan(plan)

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack database client",
			err.Error(),
		)

		return
	}

	dbName := plan.Name.ValueString()
	instanceID := plan.InstanceID.ValueString()

	var dbs databases.BatchCreateOpts
	dbs = append(dbs, databases.CreateOpts{
		Name: dbName,
	})

	exists, err := databaseDatabaseV1Exists(ctx, databaseV1Client, instanceID, dbName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error checking openstack_db_database_v1 %s status on %s", dbName, instanceID),
			err.Error(),
		)

		return
	}

	if exists {
		resp.Diagnostics.AddError(
			"Database already exists",
			fmt.Sprintf("openstack_db_database_v1 %s already exists on instance %s", dbName, instanceID),
		)

		return
	}

	if err := databases.Create(ctx, databaseV1Client, instanceID, dbs).ExtractErr(); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error creating openstack_db_database_v1 %s on %s", dbName, instanceID),
			err.Error(),
		)

		return
	}

	stateConf := &retry.StateChangeConf{
		Pending:    []string{"BUILD"},
		Target:     []string{"ACTIVE"},
		Refresh:    databaseDatabaseV1StateRefreshFunc(ctx, databaseV1Client, instanceID, dbName),
		Timeout:    createTimeout,
		Delay:      0,
		MinTimeout: 3 * time.Second,
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_db_database_v1 %s on %s to become ready", dbName, instanceID),
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", instanceID, dbName))
	plan.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseDatabaseV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseDatabaseV1Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromPlan(state)

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack database client",
			err.Error(),
		)

		return
	}

	instanceID, dbName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_database_v1")
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid resource ID",
			err.Error(),
		)

		return
	}

	exists, err := databaseDatabaseV1Exists(ctx, databaseV1Client, instanceID, dbName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error checking if openstack_db_database_v1 %s exists", state.ID.ValueString()),
			err.Error(),
		)

		return
	}

	if !exists {
		resp.State.RemoveResource(ctx)

		return
	}

	state.InstanceID = types.StringValue(instanceID)
	state.Name = types.StringValue(dbName)
	state.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op: every user-facing attribute on this resource is
// ForceNew/RequiresReplace, so the framework will never actually invoke
// Update — but the interface still requires the method.
func (r *databaseDatabaseV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseDatabaseV1Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseDatabaseV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseDatabaseV1Model

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

	region := r.regionFromPlan(state)

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack database client",
			err.Error(),
		)

		return
	}

	instanceID, dbName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_database_v1")
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid resource ID",
			err.Error(),
		)

		return
	}

	exists, err := databaseDatabaseV1Exists(ctx, databaseV1Client, instanceID, dbName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error checking if openstack_db_database_v1 %s exists", state.ID.ValueString()),
			err.Error(),
		)

		return
	}

	if !exists {
		return
	}

	if err := databases.Delete(ctx, databaseV1Client, instanceID, dbName).ExtractErr(); err != nil {
		if gophercloud.ResponseCodeIs(err, 404) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_db_database_v1 %s", state.ID.ValueString()),
			err.Error(),
		)

		return
	}
}

func (r *databaseDatabaseV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// regionFromPlan returns the region attribute on the model, falling back
// to the provider-level region when not set. Mirrors the SDKv2 helper
// GetRegion but operates on the typed framework model.
func (r *databaseDatabaseV1Resource) regionFromPlan(m databaseDatabaseV1Model) string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}

	if r.config != nil {
		return r.config.Region
	}

	return ""
}
