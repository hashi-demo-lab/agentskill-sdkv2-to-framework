// Migrated to terraform-plugin-framework.
//
// Original SDKv2 resource was at SchemaVersion: 2 with two CHAINED state
// upgraders (V0 -> V1 -> V2). The framework requires single-step upgraders:
// each entry in the UpgradeState() map must produce the CURRENT (V2) state
// directly. So we have two entries:
//
//   0: PriorSchema = V0 shape ("address"), produces V2 in one call.
//   1: PriorSchema = V1 shape ("host_port"), produces V2 in one call.
//
// The V0 upgrader does NOT call the V1 upgrader. The V0->V1 and V1->V2
// transformations are composed inline inside upgradeWidgetFromV0.

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

// Compile-time assertions: confirm widgetResource implements the optional
// framework interfaces we rely on. Missing a method becomes a compile error
// rather than a silent runtime failure.
var (
	_ resource.Resource                 = &widgetResource{}
	_ resource.ResourceWithImportState  = &widgetResource{}
	_ resource.ResourceWithUpgradeState = &widgetResource{}
)

// NewWidgetResource is the resource constructor registered with the provider.
func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

type widgetResource struct{}

// ---------------------------------------------------------------------------
// Current (V2) model and schema
// ---------------------------------------------------------------------------

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
				// SDKv2 ForceNew -> framework RequiresReplace plan modifier.
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

// ---------------------------------------------------------------------------
// CRUD stubs (content irrelevant to this fixture; the upgrader logic is
// the focus). Kept minimal so the file compiles.
// ---------------------------------------------------------------------------

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
	// Nothing to do for the synthetic fixture.
}

// ImportState replaces the SDKv2 ImportStatePassthroughContext importer:
// pass the import ID straight through to the "id" attribute.
func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// State upgraders (single-step, framework-style)
// ---------------------------------------------------------------------------
//
// Map keys are PRIOR versions. Each upgrader emits CURRENT (V2) state.
// The V0 entry must NOT chain through V1 — it composes both legacy
// transformations inline.

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

// ----- V0 prior schema and model -----

// V0 had: id, name, address (legacy "host:port" string).
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// upgradeWidgetFromV0 composes the legacy V0->V1 (address -> host_port)
// and V1->V2 (split host_port into host/port; default empty tags) transforms
// into a single direct V0 -> V2 upgrade. It does NOT call upgradeWidgetFromV1.
func upgradeWidgetFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Inline composition of V0 -> V1 -> V2.
	// V0 -> V1: rename "address" to "host_port" (same string).
	hostPort := prior.Address.ValueString()
	// V1 -> V2: split "host_port" into "host" and "port".
	host, port := splitHostPort(hostPort)

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		// V2 introduced tags; default to an empty map (matching the SDKv2
		// V1->V2 upgrader, which inserted map[string]interface{}{} when tags
		// was absent).
		Tags: types.MapValueMust(types.StringType, map[string]attr.Value{}),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// ----- V1 prior schema and model -----

// V1 had: id, name, host_port (single string).
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"name":      schema.StringAttribute{Required: true},
			"host_port": schema.StringAttribute{Required: true},
		},
	}
}

type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// upgradeWidgetFromV1 ports the SDKv2 V1->V2 transformation directly.
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

// splitHostPort matches the legacy SDKv2 upgrader: split on the first ":".
// Returns ("", 0) for an empty string; defaults port to 0 when missing or
// not parseable as int (the SDKv2 version stored "0" as a string and let the
// framework coerce — here we coerce explicitly to int64 since the V2 schema
// types port as Int64).
func splitHostPort(hp string) (string, int64) {
	if hp == "" {
		return "", 0
	}
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
