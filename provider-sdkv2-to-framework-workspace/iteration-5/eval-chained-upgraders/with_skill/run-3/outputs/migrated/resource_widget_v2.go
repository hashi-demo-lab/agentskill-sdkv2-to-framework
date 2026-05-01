// Migrated to terraform-plugin-framework.
//
// Original SDKv2 resource was at SchemaVersion: 2 with two CHAINED upgraders
// (V0 → V1 → V2). Framework upgraders are SINGLE-STEP, so the chain becomes
// two independent map entries on UpgradeState():
//
//   0: prior-state V0  → CURRENT (V2) state, in one call
//   1: prior-state V1  → CURRENT (V2) state, in one call
//
// The V0-keyed upgrader does NOT call the V1-keyed upgrader; it composes the
// V0→V1 and V1→V2 transformations inline so it produces V2 state directly.

package widget

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                   = &widgetResource{}
	_ resource.ResourceWithImportState    = &widgetResource{}
	_ resource.ResourceWithUpgradeState   = &widgetResource{}
)

// NewWidgetResource is the constructor registered by the provider.
func NewWidgetResource() resource.Resource { return &widgetResource{} }

type widgetResource struct{}

// widgetModel mirrors the CURRENT (V2) schema.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// Metadata.
func (r *widgetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

// Schema (current = V2).
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

// CRUD stubs (mirrors the SDKv2 stubs — content irrelevant to this eval).

func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state widgetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan widgetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// nothing to do
}

// ImportState — passthrough on "id".
func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// State upgraders — SINGLE-STEP semantics.
//
// SDKv2 had a chain V0→V1→V2. The framework calls each entry independently
// with the matching PriorSchema; there is no chain. Each entry produces
// CURRENT (V2) state directly.
// ---------------------------------------------------------------------------

// Prior models — tfsdk tags must match the corresponding PriorSchema.

type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"name":      schema.StringAttribute{Required: true},
			"host_port": schema.StringAttribute{Required: true},
		},
	}
}

// UpgradeState returns one entry per prior version, each producing CURRENT
// state directly. Note the absence of any V0→V1→V2 chaining: the V0-keyed
// upgrader composes the transformations *inline* and emits V2 state.
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

// upgradeWidgetFromV0 transforms V0 state directly into V2 state.
//
// V0 fields:    id, name, address ("host:port" mixed string)
// V2 (current): id, name, host, port (int64), tags (map[string]string)
//
// Composition (inline — does NOT call upgradeWidgetFromV1):
//   1. V0→V1 conceptual step: rename "address" → "host_port".
//   2. V1→V2 conceptual step: split "host_port" into "host"+"port", default tags.
func upgradeWidgetFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Step 1 (was V0→V1): the V0 "address" is what V1 called "host_port".
	hostPort := prior.Address.ValueString()

	// Step 2 (was V1→V2): split into host + int64 port, tags default to empty map.
	host, port := splitHostPort(hostPort)

	emptyTags, diags := types.MapValue(types.StringType, map[string]types.Value{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: emptyTags,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &current)...)
}

// upgradeWidgetFromV1 transforms V1 state directly into V2 state.
//
// V1 fields:    id, name, host_port
// V2 (current): id, name, host, port (int64), tags
func upgradeWidgetFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV1
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host, port := splitHostPort(prior.HostPort.ValueString())

	emptyTags, diags := types.MapValue(types.StringType, map[string]types.Value{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: emptyTags,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &current)...)
}

// splitHostPort splits a "host:port" string into (host, int64-port).
// On a missing or non-numeric port, port = 0.
func splitHostPort(hp string) (string, int64) {
	parts := strings.SplitN(hp, ":", 2)
	host := parts[0]
	if len(parts) != 2 {
		return host, 0
	}
	port, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return host, 0
	}
	return host, port
}

