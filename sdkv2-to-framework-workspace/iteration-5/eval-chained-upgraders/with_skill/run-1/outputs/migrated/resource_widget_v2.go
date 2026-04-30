// Migrated to terraform-plugin-framework.
//
// SchemaVersion: 2 with chained SDKv2 upgraders (V0 -> V1 -> V2) is
// translated to single-step framework semantics: UpgradeState() returns a
// map keyed by the *prior* version, where each entry produces the CURRENT
// (V2) state directly, in one call. There is no chain — V0's upgrader does
// NOT call V1's upgrader; instead the V0 -> V2 transformation is composed
// by hand inside upgradeFromV0.
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

// Compile-time interface assertions: a missing method becomes a build error.
var (
	_ resource.Resource                 = &widgetResource{}
	_ resource.ResourceWithImportState  = &widgetResource{}
	_ resource.ResourceWithUpgradeState = &widgetResource{}
)

// NewWidgetResource is the constructor wired into the provider's Resources().
func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

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

// ---------------------------------------------------------------------------
// CRUD — stubs (content irrelevant to this eval; the upgrader logic is the focus).
// ---------------------------------------------------------------------------

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
	// no-op
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// State upgrade: chained SDKv2 upgraders flattened to single-step framework form.
// ---------------------------------------------------------------------------
//
// SDKv2 had:
//   V0 -> V1: rename "address" to "host_port"
//   V1 -> V2: split "host_port" into "host" + "port"; default "tags" to {}
//
// Framework requires each entry produce the *current* (V2) state directly.
// There are TWO entries (0 and 1), not three. V0 must NOT call V1's upgrader;
// the V0 -> V2 transformation is composed inline by hand.

// widgetModelV0 matches the V0 SDKv2 schema (id, name, address).
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 matches the V1 SDKv2 schema (id, name, host_port).
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

func (r *widgetResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Prior version 0 -> CURRENT (V2). NOT a chain: this entry produces
		// V2 state directly. It does not call the V1 upgrader.
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeFromV0,
		},
		// Prior version 1 -> CURRENT (V2). Equivalent to the SDKv2 V1->V2 step.
		1: {
			PriorSchema:   priorSchemaV1(),
			StateUpgrader: upgradeFromV1,
		},
	}
}

// upgradeFromV0 takes V0 state straight to the current (V2) state. The SDKv2
// V0->V1 rename and V1->V2 split are composed by hand here — V0's upgrader
// must NOT delegate to V1's upgrader (the framework calls each entry
// independently with its matching PriorSchema).
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Compose V0->V1->V2 transformations inline.
	//   Step 1 (was V0->V1): "address" carried the host:port string and was
	//   renamed to "host_port". We skip the intermediate name and read it
	//   directly out of prior.Address.
	//   Step 2 (was V1->V2): split it into "host" + "port"; default "tags" to {}.
	host, port := splitHostPort(prior.Address.ValueString())

	emptyTags, mapDiags := types.MapValue(types.StringType, map[string]attr.Value{})
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
	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// upgradeFromV1 takes V1 state straight to the current (V2) state. This is
// the framework-port of the SDKv2 V1->V2 upgrader.
func upgradeFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV1
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host, port := splitHostPort(prior.HostPort.ValueString())

	emptyTags, mapDiags := types.MapValue(types.StringType, map[string]attr.Value{})
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
	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// splitHostPort encapsulates the "host:port" -> (host, port) parse used by
// both the V0 (composed) and V1 transformations. Mirrors the SDKv2 V1->V2
// upgrader's behaviour: missing/unparsable port becomes 0.
func splitHostPort(hp string) (string, int64) {
	parts := strings.SplitN(hp, ":", 2)
	host := parts[0]
	if len(parts) != 2 {
		return host, 0
	}
	p, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return host, 0
	}
	return host, p
}
