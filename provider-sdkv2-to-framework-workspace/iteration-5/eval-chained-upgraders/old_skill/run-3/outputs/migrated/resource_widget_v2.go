// Migrated to terraform-plugin-framework.
//
// SchemaVersion: 2 with two state upgraders. The original SDKv2 shape used
// CHAINED upgraders (V0→V1→V2). The framework requires SINGLE-STEP semantics:
// each entry in UpgradeState() takes prior-version state directly to the
// CURRENT (V2) state in one call. The V0 entry must therefore COMPOSE the
// V0→V1 and V1→V2 transformations inline; it does NOT call the V1 upgrader.
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

// Compile-time interface assertions — a missing method is a compile error.
var (
	_ resource.Resource                   = &widgetResource{}
	_ resource.ResourceWithImportState    = &widgetResource{}
	_ resource.ResourceWithUpgradeState   = &widgetResource{}
)

func NewWidgetResource() resource.Resource { return &widgetResource{} }

type widgetResource struct{}

// widgetModel matches the CURRENT (V2) schema.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

func (r *widgetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

func (r *widgetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 2,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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

func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state widgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// no-op (stub)
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// -----------------------------------------------------------------------------
// State upgraders — SINGLE-STEP (NOT chained).
//
// The SDKv2 source had two chained upgraders:
//   V0 → V1 (rename "address" → "host_port")
//   V1 → V2 (split "host_port" → "host"+"port"; default "tags" to {})
//
// The framework requires each entry to produce the CURRENT (V2) state directly:
//   key 0 (V0 → V2): COMPOSE V0→V1 and V1→V2 transformations inline
//   key 1 (V1 → V2): port the V1→V2 transformation directly
// -----------------------------------------------------------------------------

func (r *widgetResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeWidgetFromV0,
		},
		1: {
			PriorSchema:   priorSchemaV1(),
			StateUpgrader: upgradeWidgetFromV1,
		},
	}
}

// priorSchemaV0 mirrors the SDKv2 V0 shape: id, name, address.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

// priorSchemaV1 mirrors the SDKv2 V1 shape: id, name, host_port.
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"name":      schema.StringAttribute{Required: true},
			"host_port": schema.StringAttribute{Required: true},
		},
	}
}

// widgetModelV0 matches priorSchemaV0.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 matches priorSchemaV1.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// splitHostPort converts a "host:port" string into typed host + port. If port
// is missing or unparseable, it falls back to 0 (matching the SDKv2 V1→V2
// upgrader's behaviour, which emitted "0" on a malformed value).
func splitHostPort(hp string) (string, int64) {
	parts := strings.SplitN(hp, ":", 2)
	host := parts[0]
	var port int64
	if len(parts) == 2 {
		if p, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			port = p
		}
	}
	return host, port
}

// upgradeWidgetFromV0 takes V0 state directly to V2 state.
//
// This COMPOSES the V0→V1 (rename "address" → "host_port") and V1→V2
// (split "host_port" into "host"+"port"; default tags to empty map)
// transformations in a single call. It does NOT delegate to upgradeWidgetFromV1.
func upgradeWidgetFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// V0→V1 (inlined): "address" effectively becomes the host:port string.
	hostPort := prior.Address.ValueString()

	// V1→V2 (inlined): split into host + port; default tags to empty map.
	host, port := splitHostPort(hostPort)

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// upgradeWidgetFromV1 takes V1 state directly to V2 state.
//
// This is a direct port of the SDKv2 V1→V2 upgrader: split "host_port" into
// "host"+"port" and default "tags" to an empty map.
func upgradeWidgetFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
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
		Tags: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
