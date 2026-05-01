package openstack

import (
	"context"
	"fmt"
	"slices"
	"time"

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

// NewDatabaseUserV1Resource returns the framework implementation of
// openstack_db_user_v1.
func NewDatabaseUserV1Resource() resource.Resource {
	return &databaseUserV1Resource{}
}

type databaseUserV1Resource struct {
	config *Config
}

// databaseUserV1Model maps the resource schema to a typed Go model.
//
// Notes on the password field — this is a *major-version* migration where the
// previous SDKv2 schema marked password as Sensitive only. We are now flipping
// it to WriteOnly + Sensitive (NOT Computed); the value is supplied through
// req.Config and is intentionally not persisted in state. Pair with
// ImportStateVerifyIgnore in tests.
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

			// Major version bump: password becomes Sensitive + WriteOnly.
			// WriteOnly attributes MUST be Required or Optional (never Computed)
			// and are not persisted in state. The value is read from
			// req.Config in Create. See references/sensitive-and-writeonly.md.
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
			// Preserve SDKv2-style block syntax for `timeouts { ... }`.
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
			fmt.Sprintf("Expected *Config, got %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *databaseUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// WriteOnly password is only available on req.Config — read the *config*
	// (not the plan) to obtain it. Other fields are also present in the plan,
	// but reading from config keeps the password in scope.
	var config databaseUserV1Model

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := config.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	region := config.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())

		return
	}

	userName := config.Name.ValueString()
	instanceID := config.InstanceID.ValueString()

	// Convert databases set into the gophercloud expand structure.
	rawDatabases := make([]any, 0)

	if !config.Databases.IsNull() && !config.Databases.IsUnknown() {
		var dbList []string

		resp.Diagnostics.Append(config.Databases.ElementsAs(ctx, &dbList, false)...)

		if resp.Diagnostics.HasError() {
			return
		}

		for _, d := range dbList {
			rawDatabases = append(rawDatabases, d)
		}
	}

	var usersList users.BatchCreateOpts
	usersList = append(usersList, users.CreateOpts{
		Name:      userName,
		Password:  config.Password.ValueString(),
		Host:      config.Host.ValueString(),
		Databases: expandDatabaseUserV1Databases(rawDatabases),
	})

	if err := users.Create(ctx, databaseV1Client, instanceID, usersList).ExtractErr(); err != nil {
		resp.Diagnostics.AddError("Error creating openstack_db_user_v1", err.Error())

		return
	}

	// Inline replacement for retry.StateChangeConf: poll until ACTIVE.
	refresh := func() (any, string, error) {
		pages, err := users.List(databaseV1Client, instanceID).AllPages(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("unable to retrieve openstack database users: %w", err)
		}

		allUsers, err := users.ExtractUsers(pages)
		if err != nil {
			return nil, "", fmt.Errorf("unable to extract openstack database users: %w", err)
		}

		for _, v := range allUsers {
			if v.Name == userName {
				return v, "ACTIVE", nil
			}
		}

		return nil, "BUILD", nil
	}

	if _, err := waitForDatabaseUserV1State(ctx, refresh, []string{"BUILD"}, []string{"ACTIVE"}, 3*time.Second, createTimeout); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for openstack_db_user_v1 to be created",
			fmt.Sprintf("user %s: %s", userName, err.Error()),
		)

		return
	}

	// Build state from config (password is WriteOnly so won't be persisted).
	state := config
	state.ID = types.StringValue(fmt.Sprintf("%s/%s", instanceID, userName))
	state.Region = types.StringValue(region)

	// Read the user back to populate computed fields (databases).
	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError("Error reading openstack_db_user_v1 after create", err.Error())

		return
	}

	if !exists {
		resp.Diagnostics.AddError(
			"openstack_db_user_v1 not found after create",
			fmt.Sprintf("user %s does not exist on instance %s after creation", userName, instanceID),
		)

		return
	}

	dbs := flattenDatabaseUserV1Databases(userObj.Databases)

	dbSet, dbDiags := types.SetValueFrom(ctx, types.StringType, dbs)
	resp.Diagnostics.Append(dbDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	state.Databases = dbSet

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
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
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())

		return
	}

	instanceID, userName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_user_v1")
	if err != nil {
		resp.Diagnostics.AddError("Error parsing openstack_db_user_v1 ID", err.Error())

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

	dbSet, dbDiags := types.SetValueFrom(ctx, types.StringType, dbs)
	resp.Diagnostics.Append(dbDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	state.Databases = dbSet

	// password is WriteOnly — never round-tripped into state. Leave it as-is
	// (will be null in state for imports / refresh).

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every user-facing attribute carries RequiresReplace.
// We still implement it (resource.Resource interface) and copy the plan to
// state so the framework is happy if it is ever invoked.
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

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())

		return
	}

	instanceID, userName, err := parsePairedIDs(state.ID.ValueString(), "openstack_db_user_v1")
	if err != nil {
		resp.Diagnostics.AddError("Error parsing openstack_db_user_v1 ID", err.Error())

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
		// Treat 404 as success (already deleted).
		if checkDeletedFramework(err) {
			return
		}

		resp.Diagnostics.AddError("Error deleting openstack_db_user_v1", err.Error())

		return
	}
}

// ImportState parses the composite ID `instance_id/name` and writes both into
// state along with id; Read fills in the rest.
func (r *databaseUserV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	instanceID, userName, err := parsePairedIDs(req.ID, "openstack_db_user_v1")
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("expected 'instance_id/name', got %q: %s", req.ID, err.Error()),
		)

		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), instanceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), userName)...)
}

// waitForDatabaseUserV1State replaces the SDKv2 retry.StateChangeConf used by
// this file in its previous incarnation. The shared SDKv2 helper
// `databaseUserV1StateRefreshFunc` returned a `retry.StateRefreshFunc` whose
// underlying signature is identical to the inline `refresh` closure here, so
// we pass the closure directly and avoid importing helper/retry from the
// migrated file.
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

// checkDeletedFramework is a framework-friendly counterpart to the SDKv2
// CheckDeleted helper: returns true when the error indicates the resource is
// already gone (HTTP 404). This avoids a dependency on *schema.ResourceData
// in the migrated file.
func checkDeletedFramework(err error) bool {
	if err == nil {
		return false
	}

	type statusCoder interface {
		StatusCode() int
	}

	if sc, ok := err.(statusCoder); ok && sc.StatusCode() == 404 {
		return true
	}

	type errDefault404 interface {
		Error404() bool
	}

	if e, ok := err.(errDefault404); ok && e.Error404() {
		return true
	}

	return false
}
