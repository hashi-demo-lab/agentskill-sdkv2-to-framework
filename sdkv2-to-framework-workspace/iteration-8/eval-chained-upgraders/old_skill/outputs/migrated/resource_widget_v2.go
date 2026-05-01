// Migrated to terraform-plugin-framework.
//
// SDKv2 had two CHAINED upgraders (V0→V1, V1→V2).
// Framework requires SINGLE-STEP semantics: each UpgradeState entry
// produces the CURRENT (V2) state in one call.
// UpgradeState map has keys 0 and 1; V0 does NOT call V1.

package widget

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time assertion that widgetResource implements ResourceWithUpgradeState.
var _ resource.ResourceWithUpgradeState = &widgetResource{}

// widgetResource is the framework resource type.
type widgetResource struct{}

func (r *widgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

// Schema returns the CURRENT (V2) schema.
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
			// V2: "host_port" was split into separate typed fields.
			"host": schema.StringAttribute{
				Required: true,
			},
			"port": schema.Int64Attribute{
				Required: true,
			},
			// V2: tags added.
			"tags": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// widgetModel is the current (V2) state model.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// widgetModelV0 matches the V0 prior schema: id, name, address.
type widgetModelV0 struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
}

// widgetModelV1 matches the V1 prior schema: id, name, host_port.
type widgetModelV1 struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	HostPort types.String `tfsdk:"host_port"`
}

// priorSchemaV0 describes the schema as it existed at version 0.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true},
			"name":    schema.StringAttribute{Required: true},
			"address": schema.StringAttribute{Required: true},
		},
	}
}

// priorSchemaV1 describes the schema as it existed at version 1.
func priorSchemaV1() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true},
			"name":      schema.StringAttribute{Required: true},
			"host_port": schema.StringAttribute{Required: true},
		},
	}
}

// UpgradeState returns a map with entries for every prior schema version.
// Each entry produces the CURRENT (V2) state directly — no chaining.
func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// V0 → current (V2): rename "address" to "host_port", then split into "host"/"port"; add "tags".
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeFromV0,
		},
		// V1 → current (V2): split "host_port" into "host"/"port"; add "tags".
		1: {
			PriorSchema:   priorSchemaV1(),
			StateUpgrader: upgradeFromV1,
		},
	}
}

// upgradeFromV0 reads V0 state (id, name, address) and produces current (V2) state.
// It does NOT call upgradeFromV1 — all transformations are composed inline.
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// V0→V1 transformation: "address" becomes "host_port".
	hostPort := prior.Address.ValueString()

	// V1→V2 transformation: split "host_port" into "host" and "port".
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

// upgradeFromV1 reads V1 state (id, name, host_port) and produces current (V2) state.
func upgradeFromV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior widgetModelV1
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// V1→V2 transformation: split "host_port" into "host" and "port".
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

// splitHostPort parses a "host:port" string into its components.
// Returns host string and port int64. Port defaults to 0 if absent or unparseable.
func splitHostPort(hostPort string) (string, int64) {
	parts := strings.SplitN(hostPort, ":", 2)
	host := parts[0]
	var port int64
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &port)
	}
	return host, port
}

// CRUD stubs — content mirrors the SDKv2 stubs (logic is out of scope for this migration).

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
