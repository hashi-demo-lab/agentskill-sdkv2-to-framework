// Migrated to terraform-plugin-framework.
//
// SDKv2 had two CHAINED state upgraders (V0→V1 and V1→V2). The framework
// uses single-step semantics: UpgradeState() returns a map keyed by the
// PRIOR version, and each entry must produce the CURRENT (V2) state in
// one call. So we expose entries for both 0 and 1, where the V0 entry
// composes the SDKv2 V0→V1 and V1→V2 transformations into a single direct
// V0→V2 upgrade, and the V1 entry ports the SDKv2 V1→V2 transformation
// directly. The V0 upgrader does NOT chain into the V1 upgrader — the
// transformation is inlined to keep each upgrader independently testable
// and to satisfy framework single-step semantics.

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

// Compile-time interface assertions. Missing methods become compile errors.
var (
	_ resource.Resource                   = &widgetResource{}
	_ resource.ResourceWithImportState    = &widgetResource{}
	_ resource.ResourceWithUpgradeState   = &widgetResource{}
)

func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

type widgetResource struct{}

// widgetModel is the CURRENT (V2) typed model.
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

// CRUD stubs — content irrelevant to the migration; the upgrader logic is the focus.

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
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// -----------------------------------------------------------------------------
// State upgrade — single-step semantics.
//
// SDKv2 chain (replaced):
//   V0 → V1: rename "address" → "host_port"
//   V1 → V2: split "host_port" into "host" (string) + "port" (int);
//            default "tags" to empty map.
//
// Framework: each entry below produces the V2 state directly. The V0 entry
// performs both transformations inline; it does NOT call upgradeFromV1.
// -----------------------------------------------------------------------------

func (r *widgetResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
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

// V0 prior shape: id, name, address.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
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
			"address": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

// V1 prior shape: id, name, host_port.
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
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
			"host_port": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

// widgetModelV0 mirrors priorSchemaV0() exactly. tfsdk tags must match.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 mirrors priorSchemaV1() exactly. tfsdk tags must match.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// upgradeFromV0 produces V2 state directly (composes legacy V0→V1→V2).
//
// IMPORTANT: this must NOT delegate to upgradeFromV1. The framework calls
// the upgrader keyed by the prior version exactly once and expects the
// CURRENT (V2) state back. Inlining the transformation keeps each upgrader
// independently testable.
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Step 1 of the legacy chain: V0 "address" was effectively renamed to
	// "host_port" in V1. We don't need to materialise V1 here — we go
	// straight to V2 by reusing the V1→V2 split logic on prior.Address.
	host, port := splitHostPort(prior.Address.ValueString())

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		// V2 introduced "tags"; default to empty map (matches the legacy
		// V1→V2 upgrader behaviour).
		Tags: emptyStringMap(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// upgradeFromV1 produces V2 state directly. Ports the SDKv2 V1→V2 upgrader.
func upgradeFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
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

// splitHostPort mirrors the legacy V1→V2 split. The SDKv2 upgrader stored
// "port" as a string (the comment said the framework would coerce); we
// parse to int64 here so the V2 typed model matches the schema.
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

// emptyStringMap returns an empty map[string]string framework value. The
// two-arg form of MapValue returns (Map, diag.Diagnostics); on an empty
// map it can't fail, so we discard the diagnostics. (Using MapValueMust
// is also safe here, but MapValue keeps the contract explicit.)
func emptyStringMap() types.Map {
	m, _ := types.MapValue(types.StringType, map[string]attr.Value{})
	return m
}
