// Migrated from SDKv2 to terraform-plugin-framework.
//
// SchemaVersion: 2 with TWO chained state upgraders (V0→V1 and V1→V2)
// translated to single-step semantics: UpgradeState() returns a map with
// entries keyed at 0 AND 1, each producing the CURRENT (V2) state directly.
// The V0 upgrader does NOT call V1's upgrader — it applies both transformations
// inline.

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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Ensure widgetResource implements the necessary interfaces.
var (
	_ resource.Resource                 = (*widgetResource)(nil)
	_ resource.ResourceWithImportState  = (*widgetResource)(nil)
	_ resource.ResourceWithUpgradeState = (*widgetResource)(nil)
)

// widgetResource is the framework resource implementation.
type widgetResource struct{}

// NewWidgetResource returns a new instance of widgetResource.
func NewWidgetResource() resource.Resource {
	return &widgetResource{}
}

func (r *widgetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

// widgetModel is the data model for the V2 schema.
type widgetModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
	Port types.Int64  `tfsdk:"port"`
	Tags types.Map    `tfsdk:"tags"`
}

// widgetV2Schema returns the current (V2) schema definition.
// Kept as a function so it can be shared between Schema() and UpgradeState().
func widgetV2Schema() schema.Schema {
	return schema.Schema{
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

// Schema returns the current (V2) schema.
func (r *widgetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = widgetV2Schema()
}

// UpgradeState implements resource.ResourceWithUpgradeState.
// It provides two upgraders:
//   - key 0: upgrades V0 state (with "address" field) directly to V2
//   - key 1: upgrades V1 state (with "host_port" field) directly to V2
//
// Neither upgrader chains to the other; each produces final V2 state in one call.
func (r *widgetResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// --- V0 → V2 (single step, does NOT call V1 upgrader) ---
		//
		// V0 schema had: id (string), name (string), address (string).
		// V0→V1 renamed "address" to "host_port".
		// V1→V2 split "host_port" into "host" (string) and "port" (number), added "tags".
		// Both transformations are applied inline here.
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id":      schema.StringAttribute{Computed: true},
					"name":    schema.StringAttribute{Required: true},
					"address": schema.StringAttribute{Required: true},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				// Define the V0 tftypes object for raw state decoding.
				v0Type := tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"id":      tftypes.String,
						"name":    tftypes.String,
						"address": tftypes.String,
					},
				}

				// Decode the raw V0 JSON state into a tftypes.Value.
				v0Val, err := req.RawState.Unmarshal(v0Type)
				if err != nil {
					resp.Diagnostics.AddError("Failed to unmarshal V0 state", err.Error())
					return
				}

				// Extract the individual attribute values.
				var rawAttrs map[string]tftypes.Value
				if err := v0Val.As(&rawAttrs); err != nil {
					resp.Diagnostics.AddError("Failed to read V0 state attributes", err.Error())
					return
				}

				var id, name, address string
				if err := rawAttrs["id"].As(&id); err != nil {
					resp.Diagnostics.AddError("Failed to read V0 id", err.Error())
					return
				}
				if err := rawAttrs["name"].As(&name); err != nil {
					resp.Diagnostics.AddError("Failed to read V0 name", err.Error())
					return
				}
				if err := rawAttrs["address"].As(&address); err != nil {
					resp.Diagnostics.AddError("Failed to read V0 address", err.Error())
					return
				}

				// Apply V0→V1 transformation: "address" is equivalent to "host_port".
				hostPort := address

				// Apply V1→V2 transformation: split "host_port" → "host" + "port".
				host, portStr := splitHostPort(hostPort)
				portVal, parseErr := strconv.ParseInt(portStr, 10, 64)
				if parseErr != nil {
					portVal = 0
				}

				// Build the final V2 model. Tags default to empty map (V2 addition).
				upgradedState := widgetModel{
					ID:   types.StringValue(id),
					Name: types.StringValue(name),
					Host: types.StringValue(host),
					Port: types.Int64Value(portVal),
					Tags: types.MapValueMust(types.StringType, map[string]attr.Value{}),
				}

				// Write the V2 state.
				resp.State = tfsdk.State{
					Schema: widgetV2Schema(),
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgradedState)...)
			},
		},

		// --- V1 → V2 (single step) ---
		//
		// V1 schema had: id (string), name (string), host_port (string).
		// V1→V2 splits "host_port" into "host" (string) and "port" (number), adds "tags".
		1: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id":        schema.StringAttribute{Computed: true},
					"name":      schema.StringAttribute{Required: true},
					"host_port": schema.StringAttribute{Required: true},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				// Define the V1 tftypes object for raw state decoding.
				v1Type := tftypes.Object{
					AttributeTypes: map[string]tftypes.Type{
						"id":        tftypes.String,
						"name":      tftypes.String,
						"host_port": tftypes.String,
					},
				}

				// Decode the raw V1 JSON state into a tftypes.Value.
				v1Val, err := req.RawState.Unmarshal(v1Type)
				if err != nil {
					resp.Diagnostics.AddError("Failed to unmarshal V1 state", err.Error())
					return
				}

				// Extract the individual attribute values.
				var rawAttrs map[string]tftypes.Value
				if err := v1Val.As(&rawAttrs); err != nil {
					resp.Diagnostics.AddError("Failed to read V1 state attributes", err.Error())
					return
				}

				var id, name, hostPort string
				if err := rawAttrs["id"].As(&id); err != nil {
					resp.Diagnostics.AddError("Failed to read V1 id", err.Error())
					return
				}
				if err := rawAttrs["name"].As(&name); err != nil {
					resp.Diagnostics.AddError("Failed to read V1 name", err.Error())
					return
				}
				if err := rawAttrs["host_port"].As(&hostPort); err != nil {
					resp.Diagnostics.AddError("Failed to read V1 host_port", err.Error())
					return
				}

				// Apply V1→V2 transformation: split "host_port" → "host" + "port".
				host, portStr := splitHostPort(hostPort)
				portVal, parseErr := strconv.ParseInt(portStr, 10, 64)
				if parseErr != nil {
					portVal = 0
				}

				// Build the final V2 model. Tags default to empty map (V2 addition).
				upgradedState := widgetModel{
					ID:   types.StringValue(id),
					Name: types.StringValue(name),
					Host: types.StringValue(host),
					Port: types.Int64Value(portVal),
					Tags: types.MapValueMust(types.StringType, map[string]attr.Value{}),
				}

				// Write the V2 state.
				resp.State = tfsdk.State{
					Schema: widgetV2Schema(),
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgradedState)...)
			},
		},
	}
}

// splitHostPort splits a "host:port" string into its components.
// Returns (host, port) where port is "0" if not present.
func splitHostPort(hostPort string) (host string, port string) {
	parts := strings.SplitN(hostPort, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], "0"
}

// ImportState implements resource.ResourceWithImportState using passthrough on id.
func (r *widgetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// Create implements resource.Resource.
func (r *widgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Implementation omitted — focus of this file is the state upgrader logic.
}

// Read implements resource.Resource.
func (r *widgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Implementation omitted — focus of this file is the state upgrader logic.
}

// Update implements resource.Resource.
func (r *widgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Implementation omitted — focus of this file is the state upgrader logic.
}

// Delete implements resource.Resource.
func (r *widgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Implementation omitted — focus of this file is the state upgrader logic.
}
