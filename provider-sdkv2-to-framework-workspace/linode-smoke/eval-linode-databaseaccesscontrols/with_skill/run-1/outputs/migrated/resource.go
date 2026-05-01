package databaseaccesscontrols

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
	"github.com/linode/terraform-provider-linode/v3/linode/helper/databaseshared"
)

const (
	defaultUpdateTimeout = 30 * time.Minute
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// ResourceModel holds the Terraform state for linode_database_access_controls.
type ResourceModel struct {
	ID           types.String `tfsdk:"id"`
	DatabaseID   types.Int64  `tfsdk:"database_id"`
	DatabaseType types.String `tfsdk:"database_type"`
	AllowList    types.Set    `tfsdk:"allow_list"`
}

var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The composite identifier for this resource (\"<database_id>:<database_type>\").",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"database_id": schema.Int64Attribute{
			Description: "The ID of the database to manage the allow list for.",
			Required:    true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.RequiresReplace(),
			},
		},
		"database_type": schema.StringAttribute{
			Description: "The type of the database to manage the allow list for.",
			Required:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
			Validators: []validator.String{
				stringvalidator.OneOfCaseInsensitive(databaseshared.ValidDatabaseTypes...),
			},
		},
		"allow_list": schema.SetAttribute{
			Description: "A list of IP addresses that can access the Managed Database. " +
				"Each item can be a single IP address or a range in CIDR format.",
			Required:    true,
			ElementType: types.StringType,
		},
	},
}

func NewResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_database_access_controls",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

type Resource struct {
	helper.BaseResource
}

// ImportState parses the composite import ID ("<database_id>:<database_type>") and
// seeds the partial state so that Read can complete population.
func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	dbID, dbType, err := parseID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf(
				"Expected import ID in the format '<database_id>:<database_type>', got %q: %s",
				req.ID, err,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database_id"), int64(dbID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database_type"), dbType)...)
	// Seed allow_list with an empty set; Read will overwrite with the actual value.
	resp.Diagnostics.Append(resp.State.SetAttribute(
		ctx, path.Root("allow_list"),
		types.SetValueMust(types.StringType, []attr.Value{}),
	)...)
}

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create "+r.Config.Name)

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dbID := helper.FrameworkSafeInt64ToInt(plan.DatabaseID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	dbType := plan.DatabaseType.ValueString()

	// Assign the composite ID before applying the allow_list.
	plan.ID = types.StringValue(formatID(dbID, dbType))

	allowList := expandAllowList(ctx, plan.AllowList, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := updateDBAllowListByEngine(ctx, *r.Meta.Client, dbType, dbID, allowList, defaultUpdateTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to set allow_list for database %d", dbID),
			err.Error(),
		)
		return
	}

	// Re-read to populate state from the API (allow_list may be normalized).
	readInto(ctx, *r.Meta.Client, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read "+r.Config.Name)

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if helper.FrameworkAttemptRemoveResourceForEmptyID(ctx, state.ID, resp) {
		return
	}

	dbID := helper.FrameworkSafeInt64ToInt(state.DatabaseID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	dbType := state.DatabaseType.ValueString()

	allowList, err := getDBAllowListByEngine(ctx, *r.Meta.Client, dbType, dbID)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf(
				"[WARN] removing database_access_controls %q from state because the database no longer exists",
				state.ID.ValueString(),
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to get allow list for database %d", dbID),
			err.Error(),
		)
		return
	}

	allowListSet, diags := types.SetValueFrom(ctx, types.StringType, allowList)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.AllowList = allowListSet
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update "+r.Config.Name)

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dbID := helper.FrameworkSafeInt64ToInt(plan.DatabaseID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	dbType := plan.DatabaseType.ValueString()

	allowList := expandAllowList(ctx, plan.AllowList, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := updateDBAllowListByEngine(ctx, *r.Meta.Client, dbType, dbID, allowList, defaultUpdateTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to update allow_list for database %d", dbID),
			err.Error(),
		)
		return
	}

	// Re-read to populate state from the API.
	readInto(ctx, *r.Meta.Client, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete "+r.Config.Name)

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	dbID := helper.FrameworkSafeInt64ToInt(state.DatabaseID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	dbType := state.DatabaseType.ValueString()

	// On delete, clear the allow list on the backing database.
	if err := updateDBAllowListByEngine(ctx, *r.Meta.Client, dbType, dbID, []string{}, defaultUpdateTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to clear allow_list for database %d on delete", dbID),
			err.Error(),
		)
	}
}

// readInto populates model.AllowList from the API. It is a shared helper used
// after Create/Update to refresh state without a full Read round-trip.
func readInto(
	ctx context.Context,
	client linodego.Client,
	model *ResourceModel,
	diags *diag.Diagnostics,
) {
	dbID := helper.FrameworkSafeInt64ToInt(model.DatabaseID.ValueInt64(), diags)
	if diags.HasError() {
		return
	}
	dbType := model.DatabaseType.ValueString()

	allowList, err := getDBAllowListByEngine(ctx, client, dbType, dbID)
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to get allow list for database %d after update", dbID),
			err.Error(),
		)
		return
	}

	allowListSet, d := types.SetValueFrom(ctx, types.StringType, allowList)
	diags.Append(d...)
	if diags.HasError() {
		return
	}

	model.AllowList = allowListSet
}

// updateDBAllowListByEngine applies the desired allow list to the database,
// skipping the API call entirely when the current list already matches.
func updateDBAllowListByEngine(
	ctx context.Context,
	client linodego.Client,
	engine string,
	id int,
	allowList []string,
	timeout time.Duration,
) error {
	// Fetch the current list to avoid a no-op update that still triggers an event wait.
	current, err := getDBAllowListByEngine(ctx, client, engine, id)
	if err != nil {
		return fmt.Errorf("failed to get current allow_list for database: %w", err)
	}

	if stringSlicesEqual(current, allowList) {
		return nil
	}

	updatePoller, err := client.NewEventPoller(ctx, id, linodego.EntityDatabase, linodego.ActionDatabaseUpdate)
	if err != nil {
		return fmt.Errorf("failed to create update EventPoller: %w", err)
	}

	timeoutSeconds, err := helper.SafeFloat64ToInt(timeout.Seconds())
	if err != nil {
		return fmt.Errorf("invalid timeout duration: %w", err)
	}

	switch engine {
	case "mysql":
		if _, err := client.UpdateMySQLDatabase(ctx, id, linodego.MySQLUpdateOptions{
			AllowList: &allowList,
		}); err != nil {
			return err
		}
	case "postgresql":
		if _, err := client.UpdatePostgresDatabase(ctx, id, linodego.PostgresUpdateOptions{
			AllowList: &allowList,
		}); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid database engine: %s", engine)
	}

	if _, err := updatePoller.WaitForFinished(ctx, timeoutSeconds); err != nil {
		return fmt.Errorf("failed to wait for update event completion: %w", err)
	}

	return nil
}

// getDBAllowListByEngine fetches the current allow list for the given database.
func getDBAllowListByEngine(ctx context.Context, client linodego.Client, engine string, id int) ([]string, error) {
	switch engine {
	case "mysql":
		db, err := client.GetMySQLDatabase(ctx, id)
		if err != nil {
			return nil, err
		}
		return db.AllowList, nil
	case "postgresql":
		db, err := client.GetPostgresDatabase(ctx, id)
		if err != nil {
			return nil, err
		}
		return db.AllowList, nil
	}
	return nil, fmt.Errorf("invalid database type: %s", engine)
}

// expandAllowList extracts a []string from a types.Set.
func expandAllowList(ctx context.Context, set types.Set, diags *diag.Diagnostics) []string {
	var items []string
	diags.Append(set.ElementsAs(ctx, &items, false)...)
	if items == nil {
		items = []string{}
	}
	return items
}

// stringSlicesEqual returns true when both slices contain the same elements
// (order-independent, as allow_list is modelled as a set).
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, v := range a {
		m[v]++
	}
	for _, v := range b {
		m[v]--
		if m[v] < 0 {
			return false
		}
	}
	return true
}

func formatID(dbID int, dbType string) string {
	return fmt.Sprintf("%d:%s", dbID, dbType)
}

func parseID(id string) (int, string, error) {
	split := strings.Split(id, ":")
	if len(split) != 2 {
		return 0, "", fmt.Errorf("invalid number of segments")
	}

	dbID, err := strconv.Atoi(split[0])
	if err != nil {
		return 0, "", err
	}

	return dbID, split[1], nil
}
