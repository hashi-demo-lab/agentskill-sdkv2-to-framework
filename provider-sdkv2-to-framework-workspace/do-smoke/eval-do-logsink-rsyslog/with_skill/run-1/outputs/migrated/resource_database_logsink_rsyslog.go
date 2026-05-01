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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions. Any missing method becomes a compile error
// here (rather than a confusing "doesn't satisfy ..." at the registration site).
var (
	_ resource.Resource                   = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithConfigure      = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithImportState    = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithModifyPlan     = &databaseLogsinkRsyslogResource{}
	_ resource.ResourceWithIdentity       = &databaseLogsinkRsyslogResource{}
)

// NewDatabaseLogsinkRsyslogResource is the framework constructor wired into
// the provider's Resources() list.
func NewDatabaseLogsinkRsyslogResource() resource.Resource {
	return &databaseLogsinkRsyslogResource{}
}

type databaseLogsinkRsyslogResource struct {
	config *config.CombinedConfig
}

// databaseLogsinkRsyslogModel is the typed mirror of the resource schema.
// Field tags MUST match the `tfsdk` attribute names in Schema().
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

// databaseLogsinkRsyslogIdentityModel is the practitioner-visible identity for
// modern `import { identity = {...} }` blocks (Terraform 1.12+).
type databaseLogsinkRsyslogIdentityModel struct {
	ClusterID types.String `tfsdk:"cluster_id"`
	LogsinkID types.String `tfsdk:"logsink_id"`
}

func (r *databaseLogsinkRsyslogResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_logsink_rsyslog"
}

func (r *databaseLogsinkRsyslogResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected *config.CombinedConfig, got %T", req.ProviderData),
		)
		return
	}
	r.config = cfg
}

func (r *databaseLogsinkRsyslogResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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
				Validators: []validator.String{
					// SDKv2 validation.NoZeroValues -> non-empty string.
					stringvalidator.LengthAtLeast(1),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Display name for the logsink",
				PlanModifiers: []planmodifier.String{
					// ForceNew on the original schema, plus the customdiff.ForceNewIfChange
					// leg in the SDKv2 CustomizeDiff — both reduce to RequiresReplace here.
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
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

// IdentitySchema implements ResourceWithIdentity. The identity exposes the
// composite-ID pieces (cluster_id, logsink_id) so practitioners on Terraform
// 1.12+ can import via:
//
//	import {
//	  to = digitalocean_database_logsink_rsyslog.foo
//	  identity = {
//	    cluster_id = "deadbeef-..."
//	    logsink_id = "01234567-..."
//	  }
//	}
//
// The legacy CLI `terraform import ... cluster_id,logsink_id` form keeps
// working — see ImportState below.
func (r *databaseLogsinkRsyslogResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"cluster_id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "UUID of the source database cluster",
			},
			"logsink_id": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "API-issued sink_id",
			},
		},
	}
}

// ModifyPlan replaces the SDKv2 customdiff.All(...) chain. The original had
// two legs:
//  1. customdiff.ForceNewIfChange("name", ...) — already covered by
//     RequiresReplace on the `name` attribute, so no logic needed here.
//  2. validateLogsinkCustomDiff(...) — cross-attribute validation we must
//     replicate.
//
// We short-circuit when the resource is being destroyed (Plan.Raw is null).
func (r *databaseLogsinkRsyslogResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Destroy: nothing to validate.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateLogsinkPlanRsyslog(&plan); err != nil {
		resp.Diagnostics.AddError("Invalid logsink configuration", err.Error())
		return
	}
}

func (r *databaseLogsinkRsyslogResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.config.GodoClient()
	clusterID := plan.ClusterID.ValueString()

	createReq := &godo.DatabaseCreateLogsinkRequest{
		Name:   plan.Name.ValueString(),
		Type:   "rsyslog",
		Config: expandLogsinkConfigRsyslogFromModel(&plan),
	}

	log.Printf("[DEBUG] Database logsink rsyslog create configuration: %#v", createReq)
	logsink, _, err := client.Databases.CreateLogsink(ctx, clusterID, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating database logsink rsyslog", err.Error())
		return
	}

	log.Printf("[DEBUG] API Response logsink: %#v", logsink)

	plan.LogsinkID = types.StringValue(logsink.ID)
	plan.ID = types.StringValue(createLogsinkID(clusterID, logsink.ID))

	// Populate state from the API response, then write state and identity.
	flattenLogsinkConfigRsyslogIntoModel(&plan, logsink.Config)
	if logsink.Name != "" {
		plan.Name = types.StringValue(logsink.Name)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Identity.Set(ctx, databaseLogsinkRsyslogIdentityModel{
		ClusterID: plan.ClusterID,
		LogsinkID: plan.LogsinkID,
	})...)
}

func (r *databaseLogsinkRsyslogResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkID(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID format",
			fmt.Sprintf("got %q, expected 'cluster_id,logsink_id'", state.ID.ValueString()),
		)
		return
	}

	client := r.config.GodoClient()
	logsink, httpResp, err := client.Databases.GetLogsink(ctx, clusterID, logsinkID)
	if err != nil {
		// 404: resource gone — drop from state so Terraform recreates on next apply.
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

	flattenLogsinkConfigRsyslogIntoModel(&state, logsink.Config)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Keep identity in sync (in case it was missing after a legacy import).
	resp.Diagnostics.Append(resp.Identity.Set(ctx, databaseLogsinkRsyslogIdentityModel{
		ClusterID: state.ClusterID,
		LogsinkID: state.LogsinkID,
	})...)
}

func (r *databaseLogsinkRsyslogResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkID(plan.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID format",
			fmt.Sprintf("got %q, expected 'cluster_id,logsink_id'", plan.ID.ValueString()),
		)
		return
	}

	client := r.config.GodoClient()
	updateReq := &godo.DatabaseUpdateLogsinkRequest{
		Config: expandLogsinkConfigRsyslogFromModel(&plan),
	}

	log.Printf("[DEBUG] Database logsink rsyslog update configuration: %#v", updateReq)
	if _, err := client.Databases.UpdateLogsink(ctx, clusterID, logsinkID, updateReq); err != nil {
		resp.Diagnostics.AddError("Error updating database logsink rsyslog", err.Error())
		return
	}

	// Re-read so server-side normalisations show up in state.
	logsink, _, err := client.Databases.GetLogsink(ctx, clusterID, logsinkID)
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving database logsink rsyslog after update", err.Error())
		return
	}
	if logsink != nil {
		plan.Name = types.StringValue(logsink.Name)
		plan.LogsinkID = types.StringValue(logsink.ID)
		flattenLogsinkConfigRsyslogIntoModel(&plan, logsink.Config)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Identity.Set(ctx, databaseLogsinkRsyslogIdentityModel{
		ClusterID: plan.ClusterID,
		LogsinkID: plan.LogsinkID,
	})...)
}

func (r *databaseLogsinkRsyslogResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseLogsinkRsyslogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID, logsinkID := splitLogsinkID(state.ID.ValueString())
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid logsink ID format",
			fmt.Sprintf("got %q, expected 'cluster_id,logsink_id'", state.ID.ValueString()),
		)
		return
	}

	log.Printf("[INFO] Deleting database logsink rsyslog: %s", state.ID.ValueString())
	client := r.config.GodoClient()
	if _, err := client.Databases.DeleteLogsink(ctx, clusterID, logsinkID); err != nil {
		// Treat 404 as success (already removed) — preserves SDKv2 behaviour.
		if godoErr, ok := err.(*godo.ErrorResponse); ok && godoErr.Response != nil && godoErr.Response.StatusCode == 404 {
			log.Printf("[INFO] Database logsink rsyslog %s was already deleted", state.ID.ValueString())
			return
		}
		resp.Diagnostics.AddError("Error deleting database logsink rsyslog", err.Error())
		return
	}
}

// ImportState supports BOTH the legacy CLI form and the modern identity form:
//
//   - Legacy:  terraform import digitalocean_database_logsink_rsyslog.foo CLUSTER,LOGSINK
//     (req.ID is set, req.Identity is empty)
//   - Modern:  import { identity = { cluster_id = ..., logsink_id = ... } }
//     (req.ID empty, req.Identity populated)
//
// They are mutually exclusive — branch on req.ID.
func (r *databaseLogsinkRsyslogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Modern path — Terraform 1.12+ supplies identity.
	if req.ID == "" {
		var identity databaseLogsinkRsyslogIdentityModel
		resp.Diagnostics.Append(req.Identity.Get(ctx, &identity)...)
		if resp.Diagnostics.HasError() {
			return
		}
		clusterID := identity.ClusterID.ValueString()
		logsinkID := identity.LogsinkID.ValueString()
		if clusterID == "" || logsinkID == "" {
			resp.Diagnostics.AddError(
				"Invalid import identity",
				"both cluster_id and logsink_id must be supplied in the import identity",
			)
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), createLogsinkID(clusterID, logsinkID))...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("logsink_id"), logsinkID)...)
		return
	}

	// Legacy path — composite-ID string from the CLI.
	clusterID, logsinkID := splitLogsinkID(req.ID)
	if clusterID == "" || logsinkID == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("must use the format 'cluster_id,logsink_id' for import (e.g. 'deadbeef-dead-4aa5-beef-deadbeef347d,01234567-89ab-cdef-0123-456789abcdef'), got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("logsink_id"), logsinkID)...)

	// Mirror onto identity so the imported resource has both forms populated.
	resp.Diagnostics.Append(resp.Identity.Set(ctx, databaseLogsinkRsyslogIdentityModel{
		ClusterID: types.StringValue(clusterID),
		LogsinkID: types.StringValue(logsinkID),
	})...)
}

// createLogsinkID creates a composite ID for logsink resources.
// Format: <cluster_id>,<logsink_id>
func createLogsinkID(clusterID string, logsinkID string) string {
	return fmt.Sprintf("%s,%s", clusterID, logsinkID)
}

// splitLogsinkID splits a composite logsink ID into cluster ID and logsink ID.
func splitLogsinkID(id string) (string, string) {
	parts := strings.SplitN(id, ",", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// expandLogsinkConfigRsyslogFromModel converts a typed plan/state model into
// the godo request body. Mirrors the SDKv2 expandLogsinkConfigRsyslog().
func expandLogsinkConfigRsyslogFromModel(m *databaseLogsinkRsyslogModel) *godo.DatabaseLogsinkConfig {
	cfg := &godo.DatabaseLogsinkConfig{
		Server: m.Server.ValueString(),
		Port:   int(m.Port.ValueInt64()),
		TLS:    m.TLS.ValueBool(),
		Format: m.Format.ValueString(),
	}
	if !m.Logline.IsNull() && !m.Logline.IsUnknown() && m.Logline.ValueString() != "" {
		cfg.Logline = m.Logline.ValueString()
	}
	if !m.StructuredData.IsNull() && !m.StructuredData.IsUnknown() && m.StructuredData.ValueString() != "" {
		cfg.SD = m.StructuredData.ValueString()
	}
	if !m.CACert.IsNull() && !m.CACert.IsUnknown() && m.CACert.ValueString() != "" {
		cfg.CA = strings.TrimSpace(m.CACert.ValueString())
	}
	if !m.ClientCert.IsNull() && !m.ClientCert.IsUnknown() && m.ClientCert.ValueString() != "" {
		cfg.Cert = strings.TrimSpace(m.ClientCert.ValueString())
	}
	if !m.ClientKey.IsNull() && !m.ClientKey.IsUnknown() && m.ClientKey.ValueString() != "" {
		cfg.Key = strings.TrimSpace(m.ClientKey.ValueString())
	}
	return cfg
}

// flattenLogsinkConfigRsyslogIntoModel writes the godo response into a typed
// model. Mirrors the SDKv2 flattenLogsinkConfigRsyslog() function — keeps the
// "only set when non-empty" semantics so no spurious diffs appear.
func flattenLogsinkConfigRsyslogIntoModel(m *databaseLogsinkRsyslogModel, cfg *godo.DatabaseLogsinkConfig) {
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

// validateLogsinkPlanRsyslog replicates the cross-field validation that lived
// in validateLogsinkCustomDiff(diff, "rsyslog") in the SDKv2 schema. Called
// from ModifyPlan.
//
// Rules (verbatim from the SDKv2 logic):
//   - format == "custom" requires non-empty logline.
//   - any TLS cert field set requires tls = true.
//   - client_cert and client_key must be set together (mTLS pair).
func validateLogsinkPlanRsyslog(m *databaseLogsinkRsyslogModel) error {
	// Format/logline pairing. We only enforce when both are known (config can
	// legitimately be unknown during plan when interpolated from another resource).
	if !m.Format.IsUnknown() && !m.Format.IsNull() {
		format := m.Format.ValueString()
		var logline string
		if !m.Logline.IsUnknown() && !m.Logline.IsNull() {
			logline = m.Logline.ValueString()
		}
		if format == "custom" && strings.TrimSpace(logline) == "" {
			return fmt.Errorf("logline is required when format is 'custom'")
		}
	}

	// TLS gating for cert fields.
	tls := false
	if !m.TLS.IsUnknown() && !m.TLS.IsNull() {
		tls = m.TLS.ValueBool()
	}

	caCert := stringValueOrEmpty(m.CACert)
	clientCert := stringValueOrEmpty(m.ClientCert)
	clientKey := stringValueOrEmpty(m.ClientKey)

	if !tls && (caCert != "" || clientCert != "" || clientKey != "") {
		return fmt.Errorf("tls must be true when ca_cert, client_cert, or client_key is set")
	}

	// Mutual mTLS pairing.
	if (clientCert != "" || clientKey != "") && (clientCert == "" || clientKey == "") {
		return fmt.Errorf("both client_cert and client_key must be set for mTLS")
	}

	return nil
}

func stringValueOrEmpty(s types.String) string {
	if s.IsNull() || s.IsUnknown() {
		return ""
	}
	return s.ValueString()
}
