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

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseUserV1Resource{}
	_ resource.ResourceWithConfigure   = &databaseUserV1Resource{}
	_ resource.ResourceWithImportState = &databaseUserV1Resource{}
)

// NewDatabaseUserV1Resource returns the framework resource implementation
// for openstack_db_user_v1.
func NewDatabaseUserV1Resource() resource.Resource {
	return &databaseUserV1Resource{}
}

type databaseUserV1Resource struct {
	config *Config
}

// databaseUserV1Model is the typed state model for openstack_db_user_v1.
//
// The Password attribute is WriteOnly: it lives only in req.Config during
// Create and is never persisted to state. State.Get / Plan.Get for Password
// will always return null after Create completes.
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
				Computed:            true,
				Description:         "The composite resource ID in the form 'instance_id/name'.",
				MarkdownDescription: "The composite resource ID in the form `instance_id/name`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"region": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "The region in which to create the database user. If omitted, the provider-level region is used.",
				MarkdownDescription: "The region in which to create the database user. If omitted, the provider-level region is used.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The user name. Changing this forces a new resource to be created.",
				MarkdownDescription: "The user name. Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"instance_id": schema.StringAttribute{
				Required:            true,
				Description:         "The ID of the database instance the user belongs to. Changing this forces a new resource to be created.",
				MarkdownDescription: "The ID of the database instance the user belongs to. Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			// password is Sensitive AND WriteOnly: practitioners supply it in
			// config, the provider sends it to the API on Create, and Terraform
			// never persists it to state. As a write-only attribute it must
			// NOT be Computed. Read it from req.Config inside CRUD methods,
			// not req.Plan or req.State (both will be null).
			"password": schema.StringAttribute{
				Required:            true,
				Sensitive:           true,
				WriteOnly:           true,
				Description:         "The password for the database user. This value is write-only: it is supplied via configuration but never persisted to Terraform state.",
				MarkdownDescription: "The password for the database user. This value is write-only: it is supplied via configuration but never persisted to Terraform state.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"host": schema.StringAttribute{
				Optional:            true,
				Description:         "The host from which the user is allowed to connect. Changing this forces a new resource to be created.",
				MarkdownDescription: "The host from which the user is allowed to connect. Changing this forces a new resource to be created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"databases": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "A set of database names the user has access to.",
				MarkdownDescription: "A set of database names the user has access to.",
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

func (r *databaseUserV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *openstack.Config, got %T. Please report this issue.", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

// resolveRegion mirrors the SDKv2 GetRegion helper for framework state.
// If the region attribute is null/unknown, fall back to the provider region.
func (r *databaseUserV1Resource) resolveRegion(region types.String) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	return r.config.Region
}

func (r *databaseUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read the write-only password from req.Config — it is NOT in plan/state.
	var config databaseUserV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

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

	region := r.resolveRegion(plan.Region)

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

	var rawDatabases []string

	if !plan.Databases.IsNull() && !plan.Databases.IsUnknown() {
		resp.Diagnostics.Append(plan.Databases.ElementsAs(ctx, &rawDatabases, false)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}

	rawDBAny := make([]any, 0, len(rawDatabases))
	for _, db := range rawDatabases {
		rawDBAny = append(rawDBAny, db)
	}

	var usersList users.BatchCreateOpts
	usersList = append(usersList, users.CreateOpts{
		Name:      userName,
		Password:  config.Password.ValueString(),
		Host:      plan.Host.ValueString(),
		Databases: expandDatabaseUserV1Databases(rawDBAny),
	})

	if err := users.Create(ctx, databaseV1Client, instanceID, usersList).ExtractErr(); err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_db_user_v1",
			err.Error(),
		)

		return
	}

	// Wait until the user shows up as ACTIVE.
	if _, err := waitForDatabaseUserV1State(
		ctx,
		databaseUserV1StateRefreshFunc(ctx, databaseV1Client, instanceID, userName),
		[]string{"BUILD"}, []string{"ACTIVE"},
		3*time.Second, createTimeout,
	); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_db_user_v1 %s to be created", userName),
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", instanceID, userName))
	plan.Region = types.StringValue(region)

	// Re-read the user to populate computed fields (databases) from the API.
	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading openstack_db_user_v1 %s after create", userName),
			err.Error(),
		)

		return
	}

	if !exists {
		resp.Diagnostics.AddError(
			"openstack_db_user_v1 vanished after create",
			fmt.Sprintf("user %s did not exist when re-read after a successful create", userName),
		)

		return
	}

	dbs := flattenDatabaseUserV1Databases(userObj.Databases)

	dbSet, dDiags := types.SetValueFrom(ctx, types.StringType, dbs)
	resp.Diagnostics.Append(dDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	plan.Databases = dbSet
	// Host is not returned reliably; preserve the planned host (may be null).
	if plan.Host.IsUnknown() {
		plan.Host = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseUserV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseUserV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.resolveRegion(state.Region)

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
			"Invalid openstack_db_user_v1 ID",
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

	dbSet, dDiags := types.SetValueFrom(ctx, types.StringType, dbs)
	resp.Diagnostics.Append(dDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	state.Databases = dbSet

	// password is write-only — never read from API, never persisted to state.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *databaseUserV1Resource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All mutable attributes on this resource use RequiresReplace, so Update
	// is never invoked in practice. Implement as a no-op for interface
	// completeness.
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

	region := r.resolveRegion(state.Region)

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
			"Invalid openstack_db_user_v1 ID",
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
		// Treat 404 as already-deleted (parity with SDKv2 CheckDeleted).
		if gophercloud.ResponseCodeIs(err, 404) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_db_user_v1 %s", state.ID.ValueString()),
			err.Error(),
		)

		return
	}
}

func (r *databaseUserV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import IDs are passed through as the composite "instance_id/name"; the
	// Read method then populates the rest of the state from the API.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// waitForDatabaseUserV1State polls refresh() until it returns a state in
// target, the deadline is hit, or ctx is cancelled. Replaces SDKv2's
// retry.StateChangeConf.WaitForStateContext for this resource.
func waitForDatabaseUserV1State(
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
