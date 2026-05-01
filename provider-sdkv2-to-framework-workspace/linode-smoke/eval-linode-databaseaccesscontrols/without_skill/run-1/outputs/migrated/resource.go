package databaseaccesscontrols

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	frameworkvalidator "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

// defaultUpdateTimeout is the default timeout duration for update operations
// (waiting for database allow_list update events to complete).
const defaultUpdateTimeout = 60 * time.Minute

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &Resource{}
var _ resource.ResourceWithImportState = &Resource{}

// NewResource returns a new instance of the database access controls resource.
func NewResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_database_access_controls",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
				TimeoutOpts: &timeouts.Opts{
					Update: true,
				},
			},
		),
	}
}

// Resource implements resource.Resource for linode_database_access_controls.
type Resource struct {
	helper.BaseResource
}

// ResourceModel is the Terraform state model for this resource.
type ResourceModel struct {
	ID           types.String   `tfsdk:"id"`
	DatabaseID   types.Int64    `tfsdk:"database_id"`
	DatabaseType types.String   `tfsdk:"database_type"`
	AllowList    types.Set      `tfsdk:"allow_list"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The composite resource ID (<database_id>:<database_type>).",
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
				frameworkvalidator.OneOfCaseInsensitive(databaseshared.ValidDatabaseTypes...),
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

// formatID produces the composite resource ID from its components.
func formatID(dbID int64, dbType string) string {
	return fmt.Sprintf("%d:%s", dbID, dbType)
}

// parseID parses a composite "<database_id>:<database_type>" resource ID.
func parseID(id string) (int64, string, error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, "", fmt.Errorf(
			"invalid composite ID %q: expected format \"<database_id>:<database_type>\"", id,
		)
	}

	dbID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid database_id in composite ID %q: %w", id, err)
	}

	return dbID, parts[1], nil
}

// Create implements resource.Resource.
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

	dbID := plan.DatabaseID.ValueInt64()
	dbType := plan.DatabaseType.ValueString()

	// Write the composite ID to state early so the resource is tracked even if
	// a subsequent step (e.g. polling) fails.
	plan.ID = types.StringValue(formatID(dbID, dbType))
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), plan.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client

	allowList, d := expandAllowList(ctx, plan.AllowList)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, d := plan.Timeouts.Update(ctx, defaultUpdateTimeout)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeoutSeconds := helper.FrameworkSafeFloat64ToInt(updateTimeout.Seconds(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(
		applyAllowList(ctx, client, dbType, int(dbID), allowList, timeoutSeconds)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(refreshState(ctx, client, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
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

	client := r.Meta.Client

	dbID := int(state.DatabaseID.ValueInt64())
	dbType := state.DatabaseType.ValueString()

	allowList, err := getAllowListByEngine(ctx, *client, dbType, dbID)
	if err != nil {
		if linodego.IsNotFound(err) {
			resp.Diagnostics.AddWarning(
				fmt.Sprintf("Removing linode_database_access_controls %s from State", state.ID.ValueString()),
				"The database no longer exists; removing the access controls resource from state.",
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to Read allow_list for database %d", dbID),
			err.Error(),
		)
		return
	}

	allowListSet, d := types.SetValueFrom(ctx, types.StringType, allowList)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.AllowList = allowListSet
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update "+r.Config.Name)

	var plan, state ResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client

	if !plan.AllowList.Equal(state.AllowList) {
		dbID := int(plan.DatabaseID.ValueInt64())
		dbType := plan.DatabaseType.ValueString()

		allowList, d := expandAllowList(ctx, plan.AllowList)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}

		updateTimeout, d := plan.Timeouts.Update(ctx, defaultUpdateTimeout)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		timeoutSeconds := helper.FrameworkSafeFloat64ToInt(updateTimeout.Seconds(), &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}

		resp.Diagnostics.Append(
			applyAllowList(ctx, client, dbType, dbID, allowList, timeoutSeconds)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	resp.Diagnostics.Append(refreshState(ctx, client, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
// "Deleting" the access controls resource means clearing the allow_list.
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

	client := r.Meta.Client
	dbID := int(state.DatabaseID.ValueInt64())
	dbType := state.DatabaseType.ValueString()

	updateTimeout, d := state.Timeouts.Update(ctx, defaultUpdateTimeout)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeoutSeconds := helper.FrameworkSafeFloat64ToInt(updateTimeout.Seconds(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(
		applyAllowList(ctx, client, dbType, dbID, []string{}, timeoutSeconds)...,
	)
}

// ImportState implements resource.ResourceWithImportState.
// The import ID must have the format "<database_id>:<database_type>" (e.g. "123:mysql").
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
				"Failed to parse import ID %q. "+
					"Expected format: \"<database_id>:<database_type>\" (e.g. \"123:mysql\"). "+
					"Error: %s",
				req.ID, err.Error(),
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database_id"), dbID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("database_type"), dbType)...)
}

// applyAllowList idempotently applies the desired allow_list to the database,
// polling until the update event finishes. If the existing list already matches,
// no API call is made.
func applyAllowList(
	ctx context.Context,
	client *linodego.Client,
	dbType string,
	dbID int,
	allowList []string,
	timeoutSeconds int,
) diag.Diagnostics {
	var d diag.Diagnostics

	// Fetch current allow list to avoid a spurious update (the API only emits
	// a database_update event when the value actually changes).
	current, err := getAllowListByEngine(ctx, *client, dbType, dbID)
	if err != nil {
		d.AddError(
			fmt.Sprintf("Failed to read allow_list for database %d", dbID),
			err.Error(),
		)
		return d
	}

	if stringSlicesEqual(current, allowList) {
		return d
	}

	updatePoller, err := client.NewEventPoller(ctx, dbID, linodego.EntityDatabase, linodego.ActionDatabaseUpdate)
	if err != nil {
		d.AddError("Failed to create update EventPoller", err.Error())
		return d
	}

	switch dbType {
	case "mysql":
		if _, err := client.UpdateMySQLDatabase(ctx, dbID, linodego.MySQLUpdateOptions{
			AllowList: &allowList,
		}); err != nil {
			d.AddError(fmt.Sprintf("Failed to update MySQL database %d allow_list", dbID), err.Error())
			return d
		}
	case "postgresql":
		if _, err := client.UpdatePostgresDatabase(ctx, dbID, linodego.PostgresUpdateOptions{
			AllowList: &allowList,
		}); err != nil {
			d.AddError(fmt.Sprintf("Failed to update PostgreSQL database %d allow_list", dbID), err.Error())
			return d
		}
	default:
		d.AddError("Invalid database type", fmt.Sprintf("Unknown database type: %s", dbType))
		return d
	}

	if _, err := updatePoller.WaitForFinished(ctx, timeoutSeconds); err != nil {
		d.AddError("Failed to wait for allow_list update event to complete", err.Error())
		return d
	}

	return d
}

// getAllowListByEngine fetches the allow_list for a database identified by engine
// type and numeric ID.
func getAllowListByEngine(ctx context.Context, client linodego.Client, engine string, id int) ([]string, error) {
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

// expandAllowList converts a framework types.Set of strings to a plain []string.
func expandAllowList(ctx context.Context, set types.Set) ([]string, diag.Diagnostics) {
	if set.IsNull() || set.IsUnknown() {
		return []string{}, nil
	}

	var result []string
	d := set.ElementsAs(ctx, &result, false)
	if result == nil {
		result = []string{}
	}

	return result, d
}

// refreshState reads the current allow_list from the API and populates the model.
func refreshState(ctx context.Context, client *linodego.Client, model *ResourceModel) diag.Diagnostics {
	var d diag.Diagnostics

	dbID := int(model.DatabaseID.ValueInt64())
	dbType := model.DatabaseType.ValueString()

	allowList, err := getAllowListByEngine(ctx, *client, dbType, dbID)
	if err != nil {
		d.AddError(
			fmt.Sprintf("Failed to read allow_list for database %d during refresh", dbID),
			err.Error(),
		)
		return d
	}

	allowListSet, diags := types.SetValueFrom(ctx, types.StringType, allowList)
	d.Append(diags...)
	if d.HasError() {
		return d
	}

	model.AllowList = allowListSet
	model.ID = types.StringValue(formatID(int64(dbID), dbType))

	return d
}

// stringSlicesEqual returns true if a and b contain the same strings, regardless
// of order and ignoring duplicates.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}

	return true
}
