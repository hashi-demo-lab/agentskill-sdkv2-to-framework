package openstack

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/db/v1/users"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	sdkretry "github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseUserV1Resource{}
	_ resource.ResourceWithConfigure   = &databaseUserV1Resource{}
	_ resource.ResourceWithImportState = &databaseUserV1Resource{}
)

// NewDatabaseUserV1Resource is the constructor used by the framework provider's
// Resources() list.
func NewDatabaseUserV1Resource() resource.Resource {
	return &databaseUserV1Resource{}
}

type databaseUserV1Resource struct {
	config *Config
}

// databaseUserV1Model is the framework schema/state model for the resource.
//
// NOTE on `password`:
//   - It is now Sensitive AND WriteOnly (NOT Computed). The practitioner supplies
//     it via config, but Terraform never persists it to state. Read it from
//     req.Config inside Create — req.Plan / req.State will be null.
//   - This is a major-version-bump change (state-breaking for any consumer who
//     was reading openstack_db_user_v1.<name>.password from state). Tests must
//     use ImportStateVerifyIgnore: []string{"password"}.
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
			// Major version bump: password is Sensitive + WriteOnly, NOT Computed.
			// WriteOnly + Computed is rejected by the framework — see
			// references/sensitive-and-writeonly.md "Hard rules" section.
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
				// Note: SDKv2 schema did NOT mark `databases` as ForceNew, so
				// we deliberately do not add RequiresReplace here. (The other
				// attributes on the resource were all ForceNew, but databases
				// was Optional+Computed only.)
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
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
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue to the provider maintainers.", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

// resolveRegion returns the configured region for the resource, falling back to
// the provider's default region when the attribute is null/unknown.
func (r *databaseUserV1Resource) resolveRegion(planRegion types.String) string {
	if !planRegion.IsNull() && !planRegion.IsUnknown() && planRegion.ValueString() != "" {
		return planRegion.ValueString()
	}

	return r.config.Region
}

func (r *databaseUserV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// password is WriteOnly — it lives in req.Config, not req.Plan. Read both:
	// Config for the secret, Plan for the rest (and to write back as state).
	var cfg databaseUserV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, dgs := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(dgs...)
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
	password := cfg.Password.ValueString() // from req.Config — WriteOnly

	rawDatabases, dgs := setStringsToAnySlice(ctx, plan.Databases)
	resp.Diagnostics.Append(dgs...)
	if resp.Diagnostics.HasError() {
		return
	}

	usersList := users.BatchCreateOpts{
		users.CreateOpts{
			Name:      userName,
			Password:  password,
			Host:      plan.Host.ValueString(),
			Databases: expandDatabaseUserV1Databases(rawDatabases),
		},
	}

	if err := users.Create(ctx, databaseV1Client, instanceID, usersList).ExtractErr(); err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_db_user_v1",
			err.Error(),
		)

		return
	}

	stateConf := &sdkretry.StateChangeConf{
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

	id := fmt.Sprintf("%s/%s", instanceID, userName)

	// Re-read the user to populate the (potentially server-side-extended)
	// databases set.
	exists, userObj, err := databaseUserV1Exists(ctx, databaseV1Client, instanceID, userName)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error checking if openstack_db_user_v1 %s exists", id),
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(id)
	plan.Region = types.StringValue(region)
	plan.Name = types.StringValue(userName)
	plan.InstanceID = types.StringValue(instanceID)

	// password is WriteOnly: do NOT persist the value to state.
	plan.Password = types.StringNull()

	if exists {
		dbList := flattenDatabaseUserV1Databases(userObj.Databases)
		dbSetVal, setDiags := types.SetValueFrom(ctx, types.StringType, dbList)
		resp.Diagnostics.Append(setDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		plan.Databases = dbSetVal
	} else if plan.Databases.IsUnknown() {
		// Defensive fallback: the StateRefresh said ACTIVE but the user
		// vanished mid-call. Set to empty rather than leaving Unknown.
		plan.Databases = types.SetValueMust(types.StringType, nil)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
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
	dbSetVal, setDiags := types.SetValueFrom(ctx, types.StringType, dbList)
	resp.Diagnostics.Append(setDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.Databases = dbSetVal

	// password remains null — it's WriteOnly and never round-tripped.
	state.Password = types.StringNull()

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is unreachable on this resource (every attribute uses RequiresReplace),
// but the resource.Resource interface requires the method.
func (r *databaseUserV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseUserV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseUserV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseUserV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, dgs := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(dgs...)
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
		// Mirror SDKv2 CheckDeleted's 404-swallowing behaviour.
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
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
	// The composite ID is "instance_id/user_name". Read parses it from state.ID,
	// so passthrough is correct.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// setStringsToAnySlice converts a framework types.Set of strings into a []any
// — the shape the existing expandDatabaseUserV1Databases helper (in
// db_user_v1.go) accepts unchanged.
func setStringsToAnySlice(ctx context.Context, set types.Set) ([]any, diag.Diagnostics) {
	if set.IsNull() || set.IsUnknown() {
		return nil, nil
	}

	var elements []string

	dgs := set.ElementsAs(ctx, &elements, false)
	if dgs.HasError() {
		return nil, dgs
	}

	out := make([]any, 0, len(elements))
	for _, e := range elements {
		out = append(out, e)
	}

	return out, dgs
}
