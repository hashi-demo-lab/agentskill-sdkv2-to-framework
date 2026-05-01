// Migrated from SDKv2 to terraform-plugin-framework.
//
// SchemaVersion: 2 with TWO single-step state upgraders.
// UpgradeState() returns map entries keyed at 0 AND 1, each producing
// current (V2) state directly — the SDKv2 chain is NOT preserved.
// V0 upgrader composes V0→V1 and V1→V2 transformations inline.

package widget

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure interface compliance.
var _ resource.Resource = &widgetResource{}
var _ resource.ResourceWithImportState = &widgetResource{}
var _ resource.ResourceWithUpgradeState = &widgetResource{}

// widgetResource is the framework implementation.
type widgetResource struct{}

func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

func (r *widgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

// widgetModel is the current (V2) typed model.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// Schema returns the current V2 schema.
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

// --- Prior-version schemas and models ---

// priorSchemaV0 returns the framework schema matching the V0 SDKv2 shape:
// id, name, address.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

// priorSchemaV1 returns the framework schema matching the V1 SDKv2 shape:
// id, name, host_port.
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"name":      schema.StringAttribute{Required: true},
			"host_port": schema.StringAttribute{Required: true},
		},
	}
}

// widgetModelV0 is the typed model for V0 prior state.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 is the typed model for V1 prior state.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// --- UpgradeState ---

// UpgradeState returns a map with entries for version 0 and version 1,
// each producing current (V2) state in a single call.
func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeFromV0,
		},
		1: {
			PriorSchema:   priorSchemaV1(),
			StateUpgrader: upgradeFromV1,
		},
	}
}

// upgradeFromV0 migrates V0 state directly to V2 (current).
// It composes the V0→V1 transformation (rename address→host_port) with
// the V1→V2 transformation (split host_port→host+port, add tags) inline.
// It does NOT call upgradeFromV1.
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// V0→V1 transformation: address becomes host_port.
	hostPort := prior.Address.ValueString()

	// V1→V2 transformation: split host_port into host and port.
	host, port, diags := splitHostPort(hostPort)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// upgradeFromV1 migrates V1 state directly to V2 (current).
func upgradeFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV1
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// V1→V2 transformation: split host_port into host and port.
	host, port, diags := splitHostPort(prior.HostPort.ValueString())
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// splitHostPort parses a "host:port" string into host and port components.
func splitHostPort(hostPort string) (string, int64, diag.Diagnostics) {
	var diags diag.Diagnostics
	parts := strings.SplitN(hostPort, ":", 2)
	host := parts[0]
	var port int64
	if len(parts) == 2 {
		p, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			diags.AddError(
				"Invalid port value",
				fmt.Sprintf("Cannot parse port %q as integer: %s", parts[1], err),
			)
			return host, 0, diags
		}
		port = p
	}
	return host, port, diags
}

// --- CRUD ---

func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: call API, set plan.ID.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state widgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: refresh from API.
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: call API.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state widgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// TODO: call API to delete.
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Passthrough: import by ID.
	if req.ID == "" {
		resp.Diagnostics.AddError("Import error", "ID must not be empty")
		return
	}
	var state widgetModel
	state.ID = types.StringValue(req.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
