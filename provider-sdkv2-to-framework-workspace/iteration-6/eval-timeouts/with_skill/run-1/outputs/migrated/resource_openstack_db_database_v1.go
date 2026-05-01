package openstack

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
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

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseDatabaseV1Resource{}
	_ resource.ResourceWithConfigure   = &databaseDatabaseV1Resource{}
	_ resource.ResourceWithImportState = &databaseDatabaseV1Resource{}
)

// NewDatabaseDatabaseV1Resource constructs the framework resource.
func NewDatabaseDatabaseV1Resource() resource.Resource {
	return &databaseDatabaseV1Resource{}
}

// databaseDatabaseV1Resource is the framework implementation of
// openstack_db_database_v1.
type databaseDatabaseV1Resource struct {
	config *Config
}

// databaseDatabaseV1Model is the typed state/plan model for the resource.
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
			// Preserves the SDKv2 'timeouts { create = "..." }' block syntax.
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
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
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

	region := getRegionForResource(plan.Region, r.config)

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

	// Inline replacement for retry.StateChangeConf — poll until the database
	// reports ACTIVE, allowing BUILD as a pending state.
	if _, err := waitForDatabaseDatabaseV1State(
		ctx,
		databaseDatabaseV1RefreshFunc(ctx, databaseV1Client, instanceID, dbName),
		[]string{"BUILD"},
		[]string{"ACTIVE"},
		3*time.Second,
		createTimeout,
	); err != nil {
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

	region := getRegionForResource(state.Region, r.config)

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
		// Equivalent to SDKv2 d.SetId("") — drop from state so Terraform recreates.
		resp.State.RemoveResource(ctx)

		return
	}

	state.InstanceID = types.StringValue(instanceID)
	state.Name = types.StringValue(dbName)
	state.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *databaseDatabaseV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All non-timeouts attributes are RequiresReplace; updates only need to
	// pass the new plan (including timeouts) through to state.
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

	region := getRegionForResource(state.Region, r.config)

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
		// Mirror SDKv2 CheckDeleted: 404 means already gone, swallow.
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
	// SDKv2 used ImportStatePassthroughContext, which simply copies req.ID
	// into the resource's id attribute. The framework equivalent:
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// getRegionForResource mirrors the SDKv2 GetRegion helper for the framework
// model. It returns the per-resource region if set, otherwise the
// provider-level default.
func getRegionForResource(region types.String, config *Config) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	return config.Region
}

// databaseDatabaseV1RefreshFunc is the framework-friendly equivalent of the
// SDKv2 retry.StateRefreshFunc: identical signature, no helper/retry import.
func databaseDatabaseV1RefreshFunc(ctx context.Context, client *gophercloud.ServiceClient, instanceID, dbName string) func() (any, string, error) {
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

// waitForDatabaseDatabaseV1State is the inline replacement for
// retry.StateChangeConf.WaitForStateContext. It polls refresh until state ∈
// target, returns an error if state ∉ pending ∪ target, and respects ctx
// cancellation as well as the explicit timeout deadline.
func waitForDatabaseDatabaseV1State(
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
			return v, fmt.Errorf("unexpected state %q (pending=%s, target=%s)",
				state, strings.Join(pending, ","), strings.Join(target, ","))
		}

		if time.Now().After(deadline) {
			return v, fmt.Errorf("timeout after %s waiting for state %s (last state=%q)",
				timeout, strings.Join(target, ","), state)
		}

		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-ticker.C:
		}
	}
}
