// Migrated to the terraform-plugin-framework.
//
// The original source had a chained pair of upgraders (V0 -> V1 -> V2). The
// framework uses single-step semantics: the UpgradeState() map returns one
// entry per prior version, and each entry produces the CURRENT (V2) state
// directly. The V0 entry composes the V0->V1 and V1->V2 transformations
// inline; it does NOT delegate to the V1 entry's upgrader.

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

// Compile-time interface assertions. A missing method becomes a build error
// rather than a runtime surprise.
var (
	_ resource.Resource                 = &widgetResource{}
	_ resource.ResourceWithImportState  = &widgetResource{}
	_ resource.ResourceWithUpgradeState = &widgetResource{}
)

// NewWidgetResource is the framework constructor used by the provider.
func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

type widgetResource struct{}

// widgetModel mirrors the CURRENT (V2) schema.
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
		Version: 2, // current schema version (matches SDKv2 SchemaVersion: 2)
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

// CRUD stubs — content irrelevant to this migration eval (state-upgrade focus).
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
	// No-op stub.
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// State upgraders — single-step framework semantics.
//
// SDKv2 had two CHAINED upgraders (V0→V1→V2). The framework calls each map
// entry independently, so each entry must produce the CURRENT (V2) state in
// one call. The `0` entry composes the SDKv2 V0→V1 and V1→V2 transformations
// inline; it does NOT delegate to upgradeFromV1.
// ---------------------------------------------------------------------------

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

// priorSchemaV0 — V0 had a single legacy "address" string mixing host:port.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"address": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

// priorSchemaV1 — V1 renamed "address" → "host_port" (still a single string).
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"host_port": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

// widgetModelV0 mirrors priorSchemaV0.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 mirrors priorSchemaV1.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// splitHostPort parses a "host:port" string into typed (host, port) values.
// Used by both V0 and V1 upgraders — keeping the helper free-standing avoids
// any temptation to call upgradeFromV1 from upgradeFromV0.
func splitHostPort(hp string) (host string, port int64) {
	parts := strings.SplitN(hp, ":", 2)
	host = parts[0]
	if len(parts) == 2 {
		if p, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			port = p
		}
	}
	return host, port
}

// upgradeWidgetFromV0 transforms V0 state DIRECTLY into the current (V2)
// state. It composes the original SDKv2 V0→V1 (rename address → host_port)
// and V1→V2 (split host_port; default tags) transformations inline. It must
// NOT call upgradeWidgetFromV1.
func upgradeWidgetFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// SDKv2 V0→V1 step inline: address became the host_port string.
	hostPort := prior.Address.ValueString()

	// SDKv2 V1→V2 step inline: split host_port into host + port; tags
	// defaults to an empty map.
	host, port := splitHostPort(hostPort)

	emptyTags, mapDiags := types.MapValue(types.StringType, map[string]types.Value{})
	resp.Diagnostics.Append(mapDiags...)
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

// upgradeWidgetFromV1 transforms V1 state DIRECTLY into the current (V2)
// state. Ports the SDKv2 V1→V2 transformation: split host_port into host +
// port; default tags to an empty map.
func upgradeWidgetFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV1
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host, port := splitHostPort(prior.HostPort.ValueString())

	emptyTags, mapDiags := types.MapValue(types.StringType, map[string]types.Value{})
	resp.Diagnostics.Append(mapDiags...)
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
