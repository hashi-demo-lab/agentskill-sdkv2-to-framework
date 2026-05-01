package openstack

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2/openstack/db/v1/users"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &dbUserV1Resource{}
	_ resource.ResourceWithConfigure   = &dbUserV1Resource{}
	_ resource.ResourceWithImportState = &dbUserV1Resource{}
)

// NewDBUserV1Resource returns a new framework resource for openstack_db_user_v1.
func NewDBUserV1Resource() resource.Resource {
	return &dbUserV1Resource{}
}

type dbUserV1Resource struct {
	config *Config
}

type dbUserV1Model struct {
	ID         types.String `tfsdk:"id"`
	Region     types.String `tfsdk:"region"`
	Name       types.String `tfsdk:"name"`
	InstanceID types.String `tfsdk:"instance_id"`
	Password   types.String `tfsdk:"password"`
	Host       types.String `tfsdk:"host"`
	Databases  types.Set    `tfsdk:"databases"`
}

func (r *dbUserV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_db_user_v1"
}

func (r *dbUserV1Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
			// password is write-only: Terraform supplies it on Create but never reads
			// it back. It is NOT Computed (WriteOnly and Computed cannot coexist).
			// Read it from req.Config (where the practitioner wrote it), not req.Plan.
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
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *dbUserV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected provider data type",
			fmt.Sprintf("expected *Config, got %T", req.ProviderData),
		)
		return
	}

	r.config = config
}

func (r *dbUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Read plan for non-write-only fields.
	var plan dbUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read the write-only password from config (not available in plan/state).
	var config dbUserV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := plan.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	databaseV1Client, err := r.config.DatabaseV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack database client", err.Error())
		return
	}

	userName := plan.Name.ValueString()
	instanceID := plan.InstanceID.ValueString()

	var rawDatabases []string
	resp.Diagnostics.Append(plan.Databases.ElementsAs(ctx, &rawDatabases, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var usersList users.BatchCreateOpts
	usersList = append(usersList, users.CreateOpts{
		Name:      userName,
		Password:  config.Password.ValueString(),
		Host:      plan.Host.ValueString(),
		Databases: expandDatabaseUserV1Databases(toAnySlice(rawDatabases)),
	})

	err = users.Create(ctx, databaseV1Client, instanceID, usersList).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError("Error creating openstack_db_user_v1", err.Error())
		return
	}

	_, err = waitForState(
		ctx,
		databaseUserV1StateRefreshFunc(ctx, databaseV1Client, instanceID, userName),
		[]string{"BUILD"},
		[]string{"ACTIVE"},
		3*time.Second,
		10*time.Minute,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_db_user_v1 %s to be created", userName),
			err.Error(),
		)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s/%s", instanceID, userName))
	plan.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dbUserV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dbUserV1Model
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
	state.Region = types.StringValue(region)

	dbNames := flattenDatabaseUserV1Databases(userObj.Databases)
	dbsVal, diags := types.SetValueFrom(ctx, types.StringType, dbNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Databases = dbsVal

	// password is write-only and not persisted; leave it as-is in state (null).

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is not implemented because all attributes are ForceNew (RequiresReplace).
// The framework will never call Update for this resource.
func (r *dbUserV1Resource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"openstack_db_user_v1 does not support in-place update",
		"All attributes require replacement. This is an internal error.",
	)
}

func (r *dbUserV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dbUserV1Model
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

	err = users.Delete(ctx, databaseV1Client, instanceID, userName).ExtractErr()
	if err != nil {
		resp.Diagnostics.AddError("Error deleting openstack_db_user_v1", err.Error())
	}
}

func (r *dbUserV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// ID format: <instance_id>/<user_name>
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"invalid import ID",
			fmt.Sprintf("expected '<instance_id>/<user_name>', got %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}

// waitForState polls refresh until the returned state is in target or an
// unexpected state is returned. It replaces retry.StateChangeConf.WaitForStateContext.
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

// toAnySlice converts a []string to []any for use with helper functions that
// accept []any (e.g., expandDatabaseUserV1Databases).
func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
