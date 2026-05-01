package database

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithConfigure   = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithImportState = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithModifyPlan  = &databaseLogsinkRsyslogResource{}
)

// NewDatabaseLogsinkRsyslogResource is the framework constructor used by the provider.
func NewDatabaseLogsinkRsyslogResource() resource.Resource {
	return &databaseLogsinkRsyslogResource{}
}

// databaseLogsinkRsyslogResource is the framework resource type.
type databaseLogsinkRsyslogResource struct {
	client *godo.Client
}

// databaseLogsinkRsyslogModel is the Terraform state/plan model for this resource.
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

// Metadata sets the resource type name.
func (r *databaseLogsinkRsyslogResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_logsink_rsyslog"
}

// Schema defines the framework schema for this resource.
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
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name for the logsink",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"server": schema.StringAttribute{
				Required:    true,
				Description: "Hostname or IP address of the rsyslog server",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"port": schema.Int64Attribute{
				Required:    true,
				Description: "Port number for the rsyslog server (1-65535)",
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
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
				Validators: []validator.String{
					stringvalidator.OneOf("rfc5424", "rfc3164", "custom"),
				},
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

// Configure wires the provider client into the resource.
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

// ModifyPlan replaces SDKv2 CustomizeDiff.
// It enforces cross-attribute constraints that cannot be expressed per-attribute:
//   - format == "custom" requires logline to be non-empty
//   - tls must be true when ca_cert, client_cert, or client_key is set
//   - client_cert and client_key must both be set (or both unset) for mTLS
//
// The SDKv2 CustomizeDiff also contained a ForceNewIfChange("name") leg, but
// "name" is already declared with RequiresReplace() in the schema, making that
// leg redundant in the framework. Only the validation logic is carried over.
func (r *databaseLogsinkRsyslogResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip validation during destroy.
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
	if format == "custom" && logline == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("logline"),
			"logline required for custom format",
			"logline is required when format is 'custom'",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	tls := plan.TLS.ValueBool()
	caCert := plan.CACert.ValueString()
	clientCert := plan.ClientCert.ValueString()
	clientKey := plan.ClientKey.ValueString()

	if !tls && (caCert != "" || clientCert != "" || clientKey != "") {
		resp.Diagnostics.AddAttributeError(
			path.Root("tls"),
			"tls required when certificates are set",
			"tls must be true when ca_cert, client_cert, or client_key is set",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if (clientCert != "" || clientKey != "") && (clientCert == "" || clientKey == "") {
		resp.Diagnostics.AddError(
			"Incomplete mTLS configuration",
			"both client_cert and client_key must be set for mTLS",
		)
	}
}

// Create creates the rsyslog logsink via the DigitalOcean API.
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

	plan.ID = types.StringValue(createLogsinkID(clusterID, logsink.ID))
	plan.LogsinkID = types.StringValue(logsink.ID)

	// Refresh state from API response to pick up any server-side defaults.
	readModel, diags := r.readLogsink(ctx, clusterID, logsink.ID, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, readModel)...)
}

// Read refreshes local state from the DigitalOcean API.
func (r *databaseLogsinkRsyslogResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkIDFW(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID",
			fmt.Sprintf("Invalid logsink ID format: %s", state.ID.ValueString()),
		)
		return
	}

	readModel, diags := r.readLogsink(ctx, clusterID, logsinkID, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if readModel == nil {
		// Resource no longer exists; remove from state.
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, readModel)...)
}

// Update updates the rsyslog logsink configuration.
func (r *databaseLogsinkRsyslogResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseLogsinkRsyslogModel
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkIDFW(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID",
			fmt.Sprintf("Invalid logsink ID format: %s", state.ID.ValueString()),
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

	readModel, diags := r.readLogsink(ctx, clusterID, logsinkID, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, readModel)...)
}

// Delete removes the rsyslog logsink.
func (r *databaseLogsinkRsyslogResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkIDFW(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID",
			fmt.Sprintf("Invalid logsink ID format: %s", state.ID.ValueString()),
		)
		return
	}

	log.Printf("[INFO] Deleting database logsink rsyslog: %s", state.ID.ValueString())
	_, err := r.client.Databases.DeleteLogsink(ctx, clusterID, logsinkID)
	if err != nil {
		if godoErr, ok := err.(*godo.ErrorResponse); ok && godoErr.Response.StatusCode == 404 {
			// Already deleted — treat as success.
			log.Printf("[INFO] Database logsink rsyslog %s was already deleted", state.ID.ValueString())
			return
		}
		resp.Diagnostics.AddError("Error deleting database logsink rsyslog", err.Error())
	}
}

// ImportState handles `terraform import digitalocean_database_logsink_rsyslog.x cluster_id,logsink_id`.
// The composite ID is parsed and written into state so that Read can fetch the resource.
func (r *databaseLogsinkRsyslogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	clusterID, logsinkID := splitLogsinkIDFW(req.ID)
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("must use the format 'cluster_id,logsink_id' for import (e.g. 'deadbeef-dead-4aa5-beef-deadbeef347d,01234567-89ab-cdef-0123-456789abcdef'), got: %q", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("logsink_id"), logsinkID)...)
}

// readLogsink is a shared helper that fetches the logsink from the API and maps it
// onto a model.  Returns (nil, nil) when the resource no longer exists (404).
// The caller supplies the prior model so that write-only/sensitive fields that
// the API does not echo back can be carried forward from local state.
func (r *databaseLogsinkRsyslogResource) readLogsink(
	ctx context.Context,
	clusterID, logsinkID string,
	prior *databaseLogsinkRsyslogModel,
) (*databaseLogsinkRsyslogModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	logsink, httpResp, err := r.client.Databases.GetLogsink(ctx, clusterID, logsinkID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			return nil, nil
		}
		diags.AddError("Error retrieving database logsink rsyslog", err.Error())
		return nil, diags
	}
	if logsink == nil {
		diags.AddError("Error retrieving database logsink rsyslog", "logsink is nil")
		return nil, diags
	}

	m := &databaseLogsinkRsyslogModel{
		ID:        types.StringValue(createLogsinkID(clusterID, logsink.ID)),
		ClusterID: types.StringValue(clusterID),
		Name:      types.StringValue(logsink.Name),
		LogsinkID: types.StringValue(logsink.ID),
	}

	if logsink.Config != nil {
		cfg := logsink.Config
		m.Server = types.StringValue(cfg.Server)
		m.Port = types.Int64Value(int64(cfg.Port))
		m.TLS = types.BoolValue(cfg.TLS)
		if cfg.Format != "" {
			m.Format = types.StringValue(cfg.Format)
		} else {
			m.Format = types.StringValue("rfc5424")
		}
		if cfg.Logline != "" {
			m.Logline = types.StringValue(cfg.Logline)
		} else {
			m.Logline = types.StringNull()
		}
		if cfg.SD != "" {
			m.StructuredData = types.StringValue(cfg.SD)
		} else {
			m.StructuredData = types.StringNull()
		}
		// Sensitive fields: the API may not echo these back; carry forward from prior state.
		if cfg.CA != "" {
			m.CACert = types.StringValue(strings.TrimSpace(cfg.CA))
		} else if prior != nil {
			m.CACert = prior.CACert
		} else {
			m.CACert = types.StringNull()
		}
		if cfg.Cert != "" {
			m.ClientCert = types.StringValue(strings.TrimSpace(cfg.Cert))
		} else if prior != nil {
			m.ClientCert = prior.ClientCert
		} else {
			m.ClientCert = types.StringNull()
		}
		if cfg.Key != "" {
			m.ClientKey = types.StringValue(strings.TrimSpace(cfg.Key))
		} else if prior != nil {
			m.ClientKey = prior.ClientKey
		} else {
			m.ClientKey = types.StringNull()
		}
	}

	return m, diags
}

// expandLogsinkConfigRsyslogFW converts a plan/state model to the godo request config.
func expandLogsinkConfigRsyslogFW(m *databaseLogsinkRsyslogModel) *godo.DatabaseLogsinkConfig {
	cfg := &godo.DatabaseLogsinkConfig{
		Server: m.Server.ValueString(),
		Port:   int(m.Port.ValueInt64()),
		TLS:    m.TLS.ValueBool(),
		Format: m.Format.ValueString(),
	}
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

// splitLogsinkIDFW splits a composite logsink ID (cluster_id,logsink_id) into its parts.
// Returns ("", "") on invalid input.
func splitLogsinkIDFW(id string) (string, string) {
	parts := strings.SplitN(id, ",", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}
