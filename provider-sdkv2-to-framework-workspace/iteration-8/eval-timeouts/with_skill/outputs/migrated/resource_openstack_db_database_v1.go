package openstack

import (
	"context"
	"fmt"
	"slices"
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
)

var (
	_ resource.Resource                = &dbDatabaseV1Resource{}
	_ resource.ResourceWithConfigure   = &dbDatabaseV1Resource{}
	_ resource.ResourceWithImportState = &dbDatabaseV1Resource{}
)

// NewDBDatabaseV1Resource returns a new instance of the resource.
func NewDBDatabaseV1Resource() resource.Resource {
	return &dbDatabaseV1Resource{}
}

type dbDatabaseV1Resource struct {
	config *Config
}

type dbDatabaseV1Model struct {
	ID         types.String   `tfsdk:"id"`
	Region     types.String   `tfsdk:"region"`
	Name       types.String   `tfsdk:"name"`
	InstanceID types.String   `tfsdk:"instance_id"`
	Timeouts   timeouts.Value `tfsdk:"timeouts"`
}

func (r *dbDatabaseV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_db_database_v1"
}

func (r *dbDatabaseV1Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (r *dbDatabaseV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)
		return
	}

	r.config = config
}

func (r *dbDatabaseV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dbDatabaseV1Model
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

	region := r.config.Region
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() {
		region = plan.Region.ValueString()
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())
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
			"Error checking openstack_db_database_v1 status",
			fmt.Sprintf("Error checking openstack_db_database_v1 %s status on %s: %s", dbName, instanceID, err),
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

	err = databases.Create(ctx, databaseV1Client, instanceID, dbs).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_db_database_v1",
			fmt.Sprintf("Error creating openstack_db_database_v1 %s on %s: %s", dbName, instanceID, err),
		)
		return
	}

	_, err = waitForState(
		ctx,
		databaseDatabaseV1StateRefreshFuncFramework(ctx, databaseV1Client, instanceID, dbName),
		[]string{"BUILD"},
		[]string{"ACTIVE"},
		3*time.Second,
		createTimeout,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for openstack_db_database_v1 to become ready",
			fmt.Sprintf("Error waiting for openstack_db_database_v1 %s on %s to become ready: %s", dbName, instanceID, err),
		)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", instanceID, dbName))
	plan.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dbDatabaseV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dbDatabaseV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.config.Region
	if !state.Region.IsNull() && !state.Region.IsUnknown() {
		region = state.Region.ValueString()
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())
		return
	}

	instanceID, dbName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_database_v1")
	if err != nil {
		resp.Diagnostics.AddError("Error parsing resource ID", err.Error())
		return
	}

	exists, err := databaseDatabaseV1Exists(ctx, databaseV1Client, instanceID, dbName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error checking if openstack_db_database_v1 exists",
			fmt.Sprintf("Error checking if openstack_db_database_v1 %s exists: %s", state.ID.ValueString(), err),
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

// Update is not implemented — all attributes are ForceNew (RequiresReplace).
func (r *dbDatabaseV1Resource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"openstack_db_database_v1 does not support in-place updates; all attributes require replacement.",
	)
}

func (r *dbDatabaseV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dbDatabaseV1Model
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

	region := r.config.Region
	if !state.Region.IsNull() && !state.Region.IsUnknown() {
		region = state.Region.ValueString()
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())
		return
	}

	instanceID, dbName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_database_v1")
	if err != nil {
		resp.Diagnostics.AddError("Error parsing resource ID", err.Error())
		return
	}

	exists, err := databaseDatabaseV1Exists(ctx, databaseV1Client, instanceID, dbName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error checking if openstack_db_database_v1 exists",
			fmt.Sprintf("Error checking if openstack_db_database_v1 %s exists: %s", state.ID.ValueString(), err),
		)
		return
	}

	if !exists {
		return
	}

	err = databases.Delete(ctx, databaseV1Client, instanceID, dbName).ExtractErr()
	if err != nil {
		if !gophercloud.ResponseCodeIs(err, 404) {
			resp.Diagnostics.AddError(
				"Error deleting openstack_db_database_v1",
				fmt.Sprintf("Error deleting openstack_db_database_v1 %s: %s", state.ID.ValueString(), err),
			)
		}
	}
}

func (r *dbDatabaseV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// databaseDatabaseV1StateRefreshFuncFramework returns a refresh function
// compatible with the inline waitForState helper (no helper/retry import).
func databaseDatabaseV1StateRefreshFuncFramework(ctx context.Context, client *gophercloud.ServiceClient, instanceID string, dbName string) func() (any, string, error) {
	return func() (any, string, error) {
		pages, err := databases.List(client, instanceID).AllPages(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("Unable to retrieve OpenStack databases: %w", err)
		}

		allDatabases, err := databases.ExtractDBs(pages)
		if err != nil {
			return nil, "", fmt.Errorf("Unable to extract OpenStack databases: %w", err)
		}

		for _, v := range allDatabases {
			if v.Name == dbName {
				return v, "ACTIVE", nil
			}
		}

		return nil, "BUILD", nil
	}
}

// waitForState polls refresh until the returned state is in target, is not in
// pending, the context is cancelled, or the timeout elapses.
func waitForState(
	ctx context.Context,
	refresh func() (any, string, error),
	pending, target []string,
	pollInterval, timeout time.Duration,
) (any, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		v, state, err := refresh()
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
			return v, fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, state)
		}
		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-ticker.C:
		}
	}
}

