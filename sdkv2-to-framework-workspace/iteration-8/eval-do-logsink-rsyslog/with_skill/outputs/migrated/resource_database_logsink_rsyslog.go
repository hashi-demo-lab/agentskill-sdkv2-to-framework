package database

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithConfigure   = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithImportState = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithModifyPlan  = &databaseLogsinkRsyslogResource{}
)

// NewDatabaseLogsinkRsyslogResource is the constructor used by the provider.
func NewDatabaseLogsinkRsyslogResource() resource.Resource {
	return &databaseLogsinkRsyslogResource{}
}

type databaseLogsinkRsyslogResource struct {
	client *godo.Client
}

// databaseLogsinkRsyslogModel is the typed model for state / plan.
type databaseLogsinkRsyslogModel struct {
	ID             types.String `tfsdk:"id"`
	ClusterID      types.String `tfsdk:"cluster_id"`
	Name           types.String `tfsdk:"name"`
	Server         types.String `tfsdk:"server"`
	Port           types.Int64  `tfsdk:"port"`
	TLS            types.Bool   `tfsdk:"tls"`
	Format         types.String `tfsdk:"format"`
	Logline        types.String `tfsdk:"logline"`
	StructuredData types.String `tfsdk:"structured_data"`
	CACert         types.String `tfsdk:"ca_cert"`
	ClientCert     types.String `tfsdk:"client_cert"`
	ClientKey      types.String `tfsdk:"client_key"`
	LogsinkID      types.String `tfsdk:"logsink_id"`
}

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_logsink_rsyslog"
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Composite ID of the logsink resource",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required:    true,
				Description: "UUID of the source database cluster that will forward logs",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name for the logsink",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"server": schema.StringAttribute{
				Required:    true,
				Description: "Hostname or IP address of the rsyslog server",
			},
			"port": schema.Int64Attribute{
				Required:    true,
				Description: "Port number for the rsyslog server (1-65535)",
			},
			"tls": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Enable TLS encryption for rsyslog connection",
			},
			"format": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("rfc5424"),
				Description: "Log format: rfc5424, rfc3164, or custom",
			},
			"logline": schema.StringAttribute{
				Optional:    true,
				Description: "Custom logline template (required when format is 'custom')",
			},
			"structured_data": schema.StringAttribute{
				Optional:    true,
				Description: "Structured data for rsyslog",
			},
			"ca_cert": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "CA certificate for TLS verification (PEM format)",
			},
			"client_cert": schema.StringAttribute{
				Optional:    true,
				Description: "Client certificate for mTLS (PEM format)",
			},
			"client_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Client private key for mTLS (PEM format)",
			},
			"logsink_id": schema.StringAttribute{
				Computed:    true,
				Description: "The API sink_id returned by DigitalOcean",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Configure (provider client injection)
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	combined, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *config.CombinedConfig, got: %T", req.ProviderData),
		)
		return
	}
	r.client = combined.GodoClient()
}

// ---------------------------------------------------------------------------
// ModifyPlan — replaces CustomizeDiff
//
// Two legs (matching customdiff.All order):
//  1. ForceNew when "name" changes — handled via RequiresReplace on the attribute
//     (already wired in the schema above); no additional logic needed here.
//  2. Cross-attribute validation: custom format requires logline; TLS certs
//     require tls=true; partial mTLS pair is invalid.
//
// Note: the name ForceNew case is already covered by the RequiresReplace plan
// modifier on the "name" attribute, so only validation logic is needed here.
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Don't validate during destroy.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	format := plan.Format.ValueString()
	logline := strings.TrimSpace(plan.Logline.ValueString())
	tls := plan.TLS.ValueBool()
	caCert := plan.CACert.ValueString()
	clientCert := plan.ClientCert.ValueString()
	clientKey := plan.ClientKey.ValueString()

	// Leg 1: format=custom requires logline
	if format == "custom" && logline == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink configuration",
			"logline is required when format is 'custom'",
		)
	}

	// Leg 2: cert fields require tls=true
	if !tls && (caCert != "" || clientCert != "" || clientKey != "") {
		resp.Diagnostics.AddError(
			"Invalid logsink configuration",
			"tls must be true when ca_cert, client_cert, or client_key is set",
		)
	}

	// Leg 3: partial mTLS pair
	if (clientCert != "" || clientKey != "") && (clientCert == "" || clientKey == "") {
		resp.Diagnostics.AddError(
			"Invalid logsink configuration",
			"both client_cert and client_key must be set for mTLS",
		)
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()

	createReq := &godo.DatabaseCreateLogsinkRequest{
		Name:   plan.Name.ValueString(),
		Type:   "rsyslog",
		Config: expandLogsinkConfigRsyslogFW(&plan),
	}

	log.Printf("[DEBUG] Database logsink rsyslog create configuration: %#v", createReq)
	logsink, _, err := r.client.Databases.CreateLogsink(ctx, clusterID, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating database logsink rsyslog", err.Error())
		return
	}

	log.Printf("[DEBUG] API Response logsink: %#v", logsink)
	log.Printf("[DEBUG] Logsink ID: '%s'", logsink.ID)
	log.Printf("[DEBUG] Logsink Name: '%s'", logsink.Name)
	log.Printf("[DEBUG] Logsink Type: '%s'", logsink.Type)

	plan.ID = types.StringValue(createLogsinkIDFW(clusterID, logsink.ID))
	plan.LogsinkID = types.StringValue(logsink.ID)
	plan.ClusterID = types.StringValue(clusterID)
	plan.Name = types.StringValue(logsink.Name)

	flattenLogsinkConfigRsyslogFW(&plan, logsink.Config)

	log.Printf("[INFO] Database logsink rsyslog ID: %s", logsink.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkIDFW(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID format",
			fmt.Sprintf("Expected 'cluster_id,logsink_id', got: %q", state.ID.ValueString()),
		)
		return
	}

	logsink, httpResp, err := r.client.Databases.GetLogsink(ctx, clusterID, logsinkID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error retrieving database logsink rsyslog", err.Error())
		return
	}

	if logsink == nil {
		resp.Diagnostics.AddError("Error retrieving database logsink rsyslog", "logsink is nil")
		return
	}

	state.ClusterID = types.StringValue(clusterID)
	state.Name = types.StringValue(logsink.Name)
	state.LogsinkID = types.StringValue(logsink.ID)

	flattenLogsinkConfigRsyslogFW(&state, logsink.Config)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Carry the composite ID and logsink_id forward from state (they are computed).
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	plan.LogsinkID = state.LogsinkID

	clusterID, logsinkID := splitLogsinkIDFW(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID format",
			fmt.Sprintf("Expected 'cluster_id,logsink_id', got: %q", state.ID.ValueString()),
		)
		return
	}

	updateReq := &godo.DatabaseUpdateLogsinkRequest{
		Config: expandLogsinkConfigRsyslogFW(&plan),
	}

	log.Printf("[DEBUG] Database logsink rsyslog update configuration: %#v", updateReq)
	_, err := r.client.Databases.UpdateLogsink(ctx, clusterID, logsinkID, updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating database logsink rsyslog", err.Error())
		return
	}

	// Re-read to refresh state from API.
	logsink, httpResp, err := r.client.Databases.GetLogsink(ctx, clusterID, logsinkID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading database logsink rsyslog after update", err.Error())
		return
	}

	plan.Name = types.StringValue(logsink.Name)
	plan.LogsinkID = types.StringValue(logsink.ID)
	flattenLogsinkConfigRsyslogFW(&plan, logsink.Config)

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkIDFW(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID format",
			fmt.Sprintf("Expected 'cluster_id,logsink_id', got: %q", state.ID.ValueString()),
		)
		return
	}

	log.Printf("[INFO] Deleting database logsink rsyslog: %s", state.ID.ValueString())
	_, err := r.client.Databases.DeleteLogsink(ctx, clusterID, logsinkID)
	if err != nil {
		if godoErr, ok := err.(*godo.ErrorResponse); ok && godoErr.Response.StatusCode == 404 {
			log.Printf("[INFO] Database logsink rsyslog %s was already deleted", state.ID.ValueString())
			return
		}
		resp.Diagnostics.AddError("Error deleting database logsink rsyslog", err.Error())
	}
}

// ---------------------------------------------------------------------------
// ImportState — composite-ID parsing
//
// The SDKv2 importer used 'cluster_id,logsink_id' as the composite separator.
// We parse that here and seed state so Read can fetch the full resource.
// ---------------------------------------------------------------------------

func (r *databaseLogsinkRsyslogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID, logsinkID := splitLogsinkIDFW(req.ID)
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Must use the format 'cluster_id,logsink_id' for import (e.g. 'deadbeef-dead-4aa5-beef-deadbeef347d,01234567-89ab-cdef-0123-456789abcdef'), got: %q", req.ID),
		)
		return
	}

	// Seed the composite "id" attribute; Read will fill in the rest.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("logsink_id"), logsinkID)...)
}

// ---------------------------------------------------------------------------
// Helper functions (framework-local variants; no *schema.ResourceData)
// ---------------------------------------------------------------------------

// createLogsinkIDFW creates a composite ID for logsink resources.
// Format: <cluster_id>,<logsink_id>
func createLogsinkIDFW(clusterID, logsinkID string) string {
	return fmt.Sprintf("%s,%s", clusterID, logsinkID)
}

// splitLogsinkIDFW splits a composite logsink ID into cluster ID and logsink ID.
func splitLogsinkIDFW(id string) (string, string) {
	parts := strings.SplitN(id, ",", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// expandLogsinkConfigRsyslogFW converts the typed plan/state model to a godo config.
func expandLogsinkConfigRsyslogFW(m *databaseLogsinkRsyslogModel) *godo.DatabaseLogsinkConfig {
	cfg := &godo.DatabaseLogsinkConfig{}

	cfg.Server = m.Server.ValueString()
	cfg.Port = int(m.Port.ValueInt64())
	cfg.TLS = m.TLS.ValueBool()
	cfg.Format = m.Format.ValueString()

	if !m.Logline.IsNull() && !m.Logline.IsUnknown() {
		cfg.Logline = m.Logline.ValueString()
	}
	if !m.StructuredData.IsNull() && !m.StructuredData.IsUnknown() {
		cfg.SD = m.StructuredData.ValueString()
	}
	if !m.CACert.IsNull() && !m.CACert.IsUnknown() {
		cfg.CA = strings.TrimSpace(m.CACert.ValueString())
	}
	if !m.ClientCert.IsNull() && !m.ClientCert.IsUnknown() {
		cfg.Cert = strings.TrimSpace(m.ClientCert.ValueString())
	}
	if !m.ClientKey.IsNull() && !m.ClientKey.IsUnknown() {
		cfg.Key = strings.TrimSpace(m.ClientKey.ValueString())
	}

	return cfg
}

// flattenLogsinkConfigRsyslogFW writes the API response back into the model in place.
func flattenLogsinkConfigRsyslogFW(m *databaseLogsinkRsyslogModel, cfg *godo.DatabaseLogsinkConfig) {
	if cfg == nil {
		return
	}

	if cfg.Server != "" {
		m.Server = types.StringValue(cfg.Server)
	}
	if cfg.Port != 0 {
		m.Port = types.Int64Value(int64(cfg.Port))
	}
	m.TLS = types.BoolValue(cfg.TLS)
	if cfg.Format != "" {
		m.Format = types.StringValue(cfg.Format)
	}
	if cfg.Logline != "" {
		m.Logline = types.StringValue(cfg.Logline)
	}
	if cfg.SD != "" {
		m.StructuredData = types.StringValue(cfg.SD)
	}
	if cfg.CA != "" {
		m.CACert = types.StringValue(strings.TrimSpace(cfg.CA))
	}
	if cfg.Cert != "" {
		m.ClientCert = types.StringValue(strings.TrimSpace(cfg.Cert))
	}
	if cfg.Key != "" {
		m.ClientKey = types.StringValue(strings.TrimSpace(cfg.Key))
	}
}

// validateLogsinkPortFW validates port is in range 1-65535.
// Used as a standalone validator (not a schema.ValidateFunc).
func validateLogsinkPortFW(port int64) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}
	return nil
}

// validateRsyslogFormatFW validates format is one of the allowed values.
func validateRsyslogFormatFW(format string) error {
	validFormats := []string{"rfc5424", "rfc3164", "custom"}
	for _, f := range validFormats {
		if format == f {
			return nil
		}
	}
	return fmt.Errorf("format must be one of: %s, got %q", strings.Join(validFormats, ", "), format)
}
