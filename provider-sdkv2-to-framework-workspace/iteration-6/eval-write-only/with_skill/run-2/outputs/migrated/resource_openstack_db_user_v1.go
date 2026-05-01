package openstack

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/db/v1/users"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

// NewDatabaseUserV1Resource is the framework constructor for openstack_db_user_v1.
func NewDatabaseUserV1Resource() resource.Resource {
	return &databaseUserV1Resource{}
}

type databaseUserV1Resource struct {
	config *Config
}

// databaseUserV1Model is the framework state model for openstack_db_user_v1.
//
// NOTE: Password is Sensitive + WriteOnly. The value is supplied by the
// practitioner via config, used at Create time, and never persisted to state.
// Read it from req.Config (not req.Plan / req.State) in CRUD methods. See
// references/sensitive-and-writeonly.md.
type databaseUserV1Model struct {
	ID         types.String `tfsdk:"id"`
	Region     types.String `tfsdk:"region"`
	Name       types.String `tfsdk:"name"`
	InstanceID types.String `tfsdk:"instance_id"`
	Password   types.String `tfsdk:"password"`
	Host       types.String `tfsdk:"host"`
	Databases  types.Set    `tfsdk:"databases"`
}

func (r *databaseUserV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_db_user_v1"
}

func (r *databaseUserV1Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
			// It is NOT Computed (write-only and computed cannot coexist).
			// The value is supplied via config, used in Create, and never
			// stored in state. Read from req.Config in CRUD methods.
			// See references/sensitive-and-writeonly.md.
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
	}
}

func (r *databaseUserV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *databaseUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Password is WriteOnly — it is null in Plan/State; read it from Config.
	var config databaseUserV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(plan.Region)

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

	rawDatabases, diags := setToStringSlice(ctx, plan.Databases)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rawDatabasesAny := make([]any, len(rawDatabases))
	for i, v := range rawDatabases {
		rawDatabasesAny[i] = v
	}

	var usersList users.BatchCreateOpts
	usersList = append(usersList, users.CreateOpts{
		Name:      userName,
		Password:  config.Password.ValueString(),
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

	createTimeout := 10 * time.Minute

	pollCtx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	if err := waitForDatabaseUserV1(
		pollCtx, databaseV1Client, instanceID, userName,
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

	// Refresh databases from the API so Computed fields are populated.
	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading openstack_db_user_v1 %s after create", plan.ID.ValueString()),
			err.Error(),
		)

		return
	}

	if !exists {
		resp.Diagnostics.AddError(
			"openstack_db_user_v1 not found after create",
			fmt.Sprintf("user %s on instance %s did not appear in the API after create", userName, instanceID),
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

	// Password is WriteOnly — must be null in state. Plan.Get already
	// returns it as null, but be explicit here so the intent is obvious
	// and future refactors can't accidentally persist it.
	plan.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseUserV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseUserV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(state.Region)

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

	// Password is WriteOnly — keep it null in state.
	state.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is a no-op shim. Every user-supplied attribute on this resource
// is RequiresReplace, so any change forces destroy+recreate. The framework
// requires the method to exist; leaving an explicit shim avoids confusion.
func (r *databaseUserV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseUserV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseUserV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFor(state.Region)

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
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_db_user_v1 %s", state.ID.ValueString()),
			err.Error(),
		)

		return
	}
}

func (r *databaseUserV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// regionFor returns the resource-level region if set, otherwise the
// provider-level region. Mirrors the SDKv2 GetRegion helper.
func (r *databaseUserV1Resource) regionFor(region types.String) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	return r.config.Region
}

// setToStringSlice converts a types.Set of string elements into []string.
func setToStringSlice(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}

	var out []string

	diags := s.ElementsAs(ctx, &out, false)

	return out, diags
}

// waitForDatabaseUserV1 replaces SDKv2 retry.StateChangeConf for this
// resource. Polls the API until the user reaches one of the target
// states or the deadline expires.
func waitForDatabaseUserV1(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	instanceID, userName string,
	pending, target []string,
	pollInterval, timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	refresh := databaseUserV1StateRefreshFunc(ctx, client, instanceID, userName)

	for {
		_, state, err := refresh()
		if err != nil {
			return err
		}

		if slices.Contains(target, state) {
			return nil
		}

		if !slices.Contains(pending, state) {
			return fmt.Errorf("unexpected state %q (pending=%v, target=%v)", state, pending, target)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, state)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
