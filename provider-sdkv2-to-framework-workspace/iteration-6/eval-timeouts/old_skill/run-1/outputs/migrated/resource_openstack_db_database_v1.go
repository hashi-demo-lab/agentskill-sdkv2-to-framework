package openstack

import (
	"context"
	"fmt"
	"net/http"
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

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseDatabaseV1Resource{}
	_ resource.ResourceWithConfigure   = &databaseDatabaseV1Resource{}
	_ resource.ResourceWithImportState = &databaseDatabaseV1Resource{}
)

// NewDatabaseDatabaseV1Resource is the framework-resource constructor.
func NewDatabaseDatabaseV1Resource() resource.Resource {
	return &databaseDatabaseV1Resource{}
}

type databaseDatabaseV1Resource struct {
	config *Config
}

// databaseDatabaseV1Model is the typed state/plan model for this resource.
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
			// Use Block (not Attributes) to preserve the existing
			// `timeouts { create = "..." }` HCL syntax that practitioners'
			// configurations already use under the SDKv2 implementation.
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

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got %T. This is a provider bug.", req.ProviderData),
		)

		return
	}

	r.config = config
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

	region := getRegionFromPlan(plan.Region, r.config)

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

	dbs := databases.BatchCreateOpts{
		databases.CreateOpts{
			Name: dbName,
		},
	}

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
			"Resource already exists",
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

	region := getRegionFromPlan(state.Region, r.config)

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
			"Error parsing openstack_db_database_v1 ID",
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
		// Resource has been deleted out-of-band — drop it from state
		// so Terraform plans a recreate.
		resp.State.RemoveResource(ctx)

		return
	}

	state.InstanceID = types.StringValue(instanceID)
	state.Name = types.StringValue(dbName)
	state.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is required by the resource.Resource interface, but every
// attribute on this resource forces replacement, so this method is
// never invoked in practice.
func (r *databaseDatabaseV1Resource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
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

	region := getRegionFromPlan(state.Region, r.config)

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
			"Error parsing openstack_db_database_v1 ID",
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
		// Mirror SDKv2 CheckDeleted: a 404 on delete is not an error,
		// the resource is already gone.
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
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
	// Original SDKv2 importer was ImportStatePassthroughContext —
	// the import string is taken as-is and Read populates the rest.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// getRegionFromPlan mirrors GetRegion from util.go but operates on the
// typed framework model: if the user explicitly set `region`, use it;
// otherwise fall back to the provider-level region.
func getRegionFromPlan(planRegion types.String, config *Config) string {
	if !planRegion.IsNull() && !planRegion.IsUnknown() && planRegion.ValueString() != "" {
		return planRegion.ValueString()
	}

	return config.Region
}
