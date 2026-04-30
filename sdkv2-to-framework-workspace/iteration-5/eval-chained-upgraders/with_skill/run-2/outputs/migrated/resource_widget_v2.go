// Migrated to terraform-plugin-framework.
//
// SchemaVersion: 2 with framework single-step upgraders.
//
// SDKv2 had two CHAINED upgraders (V0 -> V1 -> V2). The framework calls each
// upgrader independently with the matching PriorSchema, so we provide TWO
// entries in UpgradeState() — keyed `0` and `1` — and each must produce the
// CURRENT (V2) state directly. The `0` entry composes the V0->V1 and V1->V2
// transformations inline; it does NOT call the V1 upgrader.

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
	_ resource.Resource                 = &widgetResource{}
	_ resource.ResourceWithImportState  = &widgetResource{}
	_ resource.ResourceWithUpgradeState = &widgetResource{}
)

func NewWidgetResource() resource.Resource { return &widgetResource{} }

type widgetResource struct{}

// Current (V2) model.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// Prior model V0: id, name, address.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// Prior model V1: id, name, host_port.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
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
			// V2 split the legacy "host_port" string into typed fields.
			"host": schema.StringAttribute{
				Required: true,
			},
			"port": schema.Int64Attribute{
				Required: true,
			},
			// V2 added tags.
			"tags": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// Prior schemas — describe the SHAPE the framework must deserialise from.
// They mirror the SDKv2 V0/V1 resourceWidgetV0/V1 schemas exactly.

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

// UpgradeState — TWO entries for an SDKv2 chain V0 -> V1 -> V2.
// Each entry produces the CURRENT (V2) state directly; entry `0` does NOT
// call entry `1`'s upgrader — the V0->V1 transformation is composed inline.
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

// upgradeFromV0 composes the SDKv2 V0->V1 (rename address -> host_port) and
// V1->V2 (split host_port -> host+port, default tags) transformations into a
// single direct V0 -> V2 transformation. It does NOT call upgradeFromV1.
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Inline V0 -> V1: address becomes the host_port string.
	hostPort := prior.Address.ValueString()

	// Inline V1 -> V2: split host:port; default tags to empty map.
	host, port := splitHostPort(hostPort)

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: emptyStringMap(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// upgradeFromV1 ports the SDKv2 V1->V2 transformation directly.
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

// splitHostPort mirrors the SDKv2 V1->V2 split logic. Port becomes a real
// int64 in the framework (V2 schema is Int64Attribute), so we parse here.
func splitHostPort(hp string) (string, int64) {
	parts := strings.SplitN(hp, ":", 2)
	host := parts[0]
	if len(parts) == 2 {
		if p, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			return host, p
		}
	}
	return host, 0
}

// emptyStringMap mirrors the SDKv2 upgrader's `raw["tags"] = map[string]interface{}{}`
// default — a known, empty map (NOT null) of string values.
func emptyStringMap() types.Map {
	return types.MapValueMust(types.StringType, map[string]attr.Value{})
}

// CRUD stubs — content irrelevant to the migration; provided so the file
// satisfies resource.Resource.
func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
}
func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}
func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}
func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
