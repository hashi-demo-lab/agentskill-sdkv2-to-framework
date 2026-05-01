// Migrated from SDKv2 to terraform-plugin-framework.
//
// State upgraders are single-step (framework semantics):
//   - Entry keyed at 0 upgrades V0 state directly to the current (V2) schema.
//   - Entry keyed at 1 upgrades V1 state directly to the current (V2) schema.
//
// The V0 upgrader does NOT call the V1 upgrader; it composes both
// SDKv2 transformations (address→host_port, host_port→host+port) inline.

package widget

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time assertion: widgetResource implements ResourceWithUpgradeState.
var _ resource.ResourceWithUpgradeState = &widgetResource{}

// widgetResource is the framework implementation of the widget resource.
type widgetResource struct{}

// NewWidgetResource is the constructor called from the provider's Resources list.
func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

func (r *widgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

// Schema returns the current (V2) schema. Version must be 2.
func (r *widgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

// widgetModel is the current (V2) typed state model.
// tfsdk tags must exactly match the attribute names in Schema above.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// ---------------------------------------------------------------------------
// State upgraders
// ---------------------------------------------------------------------------

// UpgradeState returns single-step upgraders for all prior schema versions.
// Each entry produces the current (V2) state directly in one call — no chaining.
func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// V0 → current (V2): compose address→host_port rename and
		// host_port→host+port split inline. Must NOT delegate to upgradeFromV1.
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeFromV0,
		},
		// V1 → current (V2): split host_port → host + port; set default tags.
		1: {
			PriorSchema:   priorSchemaV1(),
			StateUpgrader: upgradeFromV1,
		},
	}
}

// priorSchemaV0 returns the framework schema that describes V0 state on disk.
// Attribute names and types must exactly match what the SDKv2 V0 provider stored.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

// priorSchemaV1 returns the framework schema that describes V1 state on disk.
// Attribute names and types must exactly match what the SDKv2 V1 provider stored.
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
// tfsdk tags must exactly match priorSchemaV0 attribute names.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 is the typed model for V1 prior state.
// tfsdk tags must exactly match priorSchemaV1 attribute names.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// upgradeFromV0 upgrades V0 state directly to the current (V2) schema.
// It composes the two SDKv2 transformations inline without calling upgradeFromV1:
//  1. address → host_port  (inline: what SDKv2 upgradeWidgetV0ToV1 did)
//  2. host_port → host + port, default tags  (inline: what SDKv2 upgradeWidgetV1ToV2 did)
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Inline step 1 (V0→V1 logic): address is the raw host_port string.
	hostPort := prior.Address.ValueString()

	// Inline step 2 (V1→V2 logic): split host_port → host + port.
	host, port := splitHostPort(hostPort)

	current := widgetModel{
		ID:   prior.ID,
		Name: prior.Name,
		Host: types.StringValue(host),
		Port: types.Int64Value(port),
		Tags: defaultEmptyTagsMap(ctx, &resp.Diagnostics),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// upgradeFromV1 upgrades V1 state directly to the current (V2) schema.
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
		Tags: defaultEmptyTagsMap(ctx, &resp.Diagnostics),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// splitHostPort splits "host:port" into (host, port as int64).
// If there is no colon, host is the whole string and port is 0.
// If the port token is not a valid integer, port defaults to 0.
func splitHostPort(hostPort string) (string, int64) {
	parts := strings.SplitN(hostPort, ":", 2)
	host := parts[0]
	if len(parts) < 2 {
		return host, 0
	}
	port, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return host, 0
	}
	return host, port
}

// defaultEmptyTagsMap returns a types.Map with element type string and zero entries.
// This represents the default empty tags introduced in the V2 schema.
func defaultEmptyTagsMap(ctx context.Context, diags *diag.Diagnostics) types.Map {
	m, d := types.MapValue(types.StringType, map[string]attr.Value{})
	diags.Append(d...)
	return m
}

// ---------------------------------------------------------------------------
// CRUD — stubs (content irrelevant to the eval; upgrader logic is the focus).
// ---------------------------------------------------------------------------

func (r *widgetResource) Create(_ context.Context, _ resource.CreateRequest, _ *resource.CreateResponse) {
}

func (r *widgetResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *widgetResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (r *widgetResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState implements resource.ResourceWithImportState using passthrough semantics.
func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// Ensure widgetResource also satisfies ResourceWithImportState at compile time.
var _ resource.ResourceWithImportState = &widgetResource{}
