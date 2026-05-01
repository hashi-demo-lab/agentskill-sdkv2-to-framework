// Migrated from SDKv2 to terraform-plugin-framework.
//
// SchemaVersion 2 with UpgradeState() map entries at keys 0 AND 1.
// Each entry produces the CURRENT (V2) schema state in one call.
// The V0 upgrader composes the V0→V1 and V1→V2 transformations directly
// without calling upgradeFromV1 (no chain habit).

package widget

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure widgetResource implements all required interfaces.
var _ resource.Resource = &widgetResource{}
var _ resource.ResourceWithImportState = &widgetResource{}
var _ resource.ResourceWithUpgradeState = &widgetResource{}

// widgetResource is the framework implementation of the widget resource.
type widgetResource struct{}

// widgetModel is the current (V2) state model.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// widgetModelV0 is the prior state model for schema version 0.
// V0 had: id, name, address (combined host:port string).
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 is the prior state model for schema version 1.
// V1 had: id, name, host_port (single string, renamed from address).
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// NewWidgetResource returns a new widgetResource.
func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

// Metadata sets the resource type name.
func (r *widgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

// Schema returns the current (V2) schema.
func (r *widgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 2,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"host": schema.StringAttribute{
				Required: true,
			},
			"port": schema.Int64Attribute{
				Required: true,
			},
			"tags": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// priorSchemaV0 returns the framework schema describing the V0 state shape.
// V0: id, name, address (combined host:port or plain address string).
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

// priorSchemaV1 returns the framework schema describing the V1 state shape.
// V1: id, name, host_port (single string, renamed from address).
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"name":      schema.StringAttribute{Required: true},
			"host_port": schema.StringAttribute{Required: true},
		},
	}
}

// splitHostPort splits a "host:port" string into host and int64 port.
// If no colon is present, host is the full string and port is 0.
func splitHostPort(hostPort string) (string, int64) {
	parts := strings.SplitN(hostPort, ":", 2)
	host := parts[0]
	var port int64
	if len(parts) == 2 {
		if p, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			port = p
		}
	}
	return host, port
}

// emptyStringMap returns an empty types.Map with string element type.
func emptyStringMap() types.Map {
	return types.MapValueMust(types.StringType, map[string]attr.Value{})
}

// UpgradeState returns the map of state upgraders for schema versions 0 and 1.
// Each entry produces the CURRENT (V2) schema state in one call — no chaining.
func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: r.upgradeFromV0,
		},
		1: {
			PriorSchema:   priorSchemaV1(),
			StateUpgrader: r.upgradeFromV1,
		},
	}
}

// upgradeFromV0 upgrades V0 state directly to the current (V2) schema.
//
// It composes both transformations in a single step:
//   - V0→V1: "address" field renamed to "host_port" (same value)
//   - V1→V2: "host_port" split into typed "host" (string) and "port" (int64);
//     "tags" initialised to an empty map.
//
// This function does NOT call upgradeFromV1 — the transformations are inlined.
func (r *widgetResource) upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// V0→V1: address is already the "host:port" string (same semantics as host_port).
	// V1→V2: split host:port into typed host + port.
	host, port := splitHostPort(prior.Address.ValueString())

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: emptyStringMap(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// upgradeFromV1 upgrades V1 state directly to the current (V2) schema.
//
// It splits "host_port" into typed "host" (string) and "port" (int64) fields,
// and initialises "tags" to an empty map.
func (r *widgetResource) upgradeFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV1
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host, port := splitHostPort(prior.HostPort.ValueString())

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: emptyStringMap(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// ImportState passes through the resource ID for import.
func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Create creates the widget resource.
func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: call API to create widget.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read reads the widget resource.
func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state widgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: call API to read widget.
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update updates the widget resource.
func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: call API to update widget.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the widget resource.
func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state widgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: call API to delete widget.
}
