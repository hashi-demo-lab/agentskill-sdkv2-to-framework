// Migrated from SDKv2 to terraform-plugin-framework.
//
// SchemaVersion is 2. UpgradeState() returns two entries:
//   - key 0: upgrades V0 state (address field) directly to current V2 state
//   - key 1: upgrades V1 state (host_port field) directly to current V2 state
//
// V0's upgrader composes both transformations inline (V0→V1 rename and V1→V2 split)
// without calling V1's upgrader. This implements single-step semantics as required
// by terraform-plugin-framework.

package widget

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure widgetResource implements ResourceWithUpgradeState.
var _ resource.ResourceWithUpgradeState = &widgetResource{}

type widgetResource struct{}

func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

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

// widgetModel is the typed model for the current (V2) schema.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// --- State upgrader prior schemas and models ---

// priorSchemaV0 returns the schema that V0 state was written against.
// V0 had: id, name, address.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

// widgetModelV0 is the typed model for V0 prior state.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// priorSchemaV1 returns the schema that V1 state was written against.
// V1 had: id, name, host_port.
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"name":      schema.StringAttribute{Required: true},
			"host_port": schema.StringAttribute{Required: true},
		},
	}
}

// widgetModelV1 is the typed model for V1 prior state.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// UpgradeState returns a map with entries for each prior schema version.
// Each entry directly produces the current (V2) state in a single call.
func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// V0 → current (V2): compose V0→V1 rename and V1→V2 split inline.
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeWidgetFromV0,
		},
		// V1 → current (V2): split host_port into host and port; add tags.
		1: {
			PriorSchema:   priorSchemaV1(),
			StateUpgrader: upgradeWidgetFromV1,
		},
	}
}

// upgradeWidgetFromV0 upgrades V0 state directly to the current (V2) state.
// Composes the V0→V1 transformation (rename address→host_port) with the
// V1→V2 transformation (split host_port into host and port, add tags) inline.
// It does NOT call upgradeWidgetFromV1.
func upgradeWidgetFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Step 1 (inline V0→V1): "address" was renamed to "host_port".
	hostPort := prior.Address.ValueString()

	// Step 2 (inline V1→V2): split "host_port" into "host" and "port".
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

// upgradeWidgetFromV1 upgrades V1 state directly to the current (V2) state.
// Splits host_port into host and port and initialises tags to an empty map.
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

// splitHostPort splits a "host:port" string into its components.
// Returns host and port as typed values. Port defaults to 0 if absent or unparseable.
func splitHostPort(hostPort string) (string, int64) {
	parts := strings.SplitN(hostPort, ":", 2)
	host := parts[0]
	var port int64
	if len(parts) == 2 {
		_, err := parsePort(parts[1], &port)
		if err != nil {
			port = 0
		}
	}
	return host, port
}

// parsePort parses a port string into an int64.
func parsePort(s string, out *int64) (bool, diag.Diagnostics) {
	var d diag.Diagnostics
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			d.AddError("Invalid port", "Port must be a numeric string, got: "+s)
			return false, d
		}
		n = n*10 + int64(c-'0')
	}
	*out = n
	return true, d
}

// --- CRUD stubs ---

func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
}

func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Passthrough import: Terraform sets the ID; Read will populate remaining state.
	var state widgetModel
	state.ID = types.StringValue(req.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
