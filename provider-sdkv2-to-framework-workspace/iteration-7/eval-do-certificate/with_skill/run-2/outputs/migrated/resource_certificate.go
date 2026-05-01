package certificate

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/util"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                   = &certificateResource{}
	_ resource.ResourceWithConfigure      = &certificateResource{}
	_ resource.ResourceWithImportState    = &certificateResource{}
	_ resource.ResourceWithUpgradeState   = &certificateResource{}
	_ resource.ResourceWithValidateConfig = &certificateResource{}
)

// NewCertificateResource returns the framework certificate resource.
func NewCertificateResource() resource.Resource {
	return &certificateResource{}
}

type certificateResource struct {
	config *config.CombinedConfig
}

// certificateModel mirrors the current (V1) schema.
type certificateModel struct {
	ID               types.String `tfsdk:"id"`
	UUID             types.String `tfsdk:"uuid"`
	Name             types.String `tfsdk:"name"`
	PrivateKey       types.String `tfsdk:"private_key"`
	LeafCertificate  types.String `tfsdk:"leaf_certificate"`
	CertificateChain types.String `tfsdk:"certificate_chain"`
	Domains          types.Set    `tfsdk:"domains"`
	Type             types.String `tfsdk:"type"`
	State            types.String `tfsdk:"state"`
	NotAfter         types.String `tfsdk:"not_after"`
	SHA1Fingerprint  types.String `tfsdk:"sha1_fingerprint"`
}

// certificateModelV0 mirrors the prior (V0) schema (no "uuid" attribute and
// the resource id was the godo certificate id rather than the name).
type certificateModelV0 struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	PrivateKey       types.String `tfsdk:"private_key"`
	LeafCertificate  types.String `tfsdk:"leaf_certificate"`
	CertificateChain types.String `tfsdk:"certificate_chain"`
	Domains          types.Set    `tfsdk:"domains"`
	Type             types.String `tfsdk:"type"`
	State            types.String `tfsdk:"state"`
	NotAfter         types.String `tfsdk:"not_after"`
	SHA1Fingerprint  types.String `tfsdk:"sha1_fingerprint"`
}

func (r *certificateResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *certificateResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			// Note that this UUID will change on auto-renewal of a
			// lets_encrypt certificate.
			"uuid": schema.StringAttribute{
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
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},

			// private_key is hashed into state explicitly in Create (see
			// notes.md) — the schema attribute itself is a plain types.String
			// so the raw plan value is available to CRUD before we hash. The
			// hashSuppressModifier keeps the existing state value when the
			// hash of the new (raw) plan value matches it, faithfully
			// reproducing the SDKv2 DiffSuppressFunc behaviour.
			"private_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					hashSuppressModifier{},
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},

			"leaf_certificate": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					hashSuppressModifier{},
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},

			"certificate_chain": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					hashSuppressModifier{},
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},

			"domains": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Set{
					// For "custom" certs the API computes domains; carry
					// state forward in that case (suppresses diff).
					customDomainsModifier{},
					setplanmodifier.RequiresReplace(),
				},
				Validators: []validator.Set{
					setvalidator.ConflictsWith(
						path.MatchRoot("private_key"),
						path.MatchRoot("leaf_certificate"),
						path.MatchRoot("certificate_chain"),
					),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},

			"type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("custom"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("custom", "lets_encrypt"),
				},
			},

			"state": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"not_after": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"sha1_fingerprint": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *certificateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *config.CombinedConfig, got %T", req.ProviderData),
		)
		return
	}
	r.config = cfg
}

// ValidateConfig replaces the SDKv2 per-CRUD presence checks. Running them at
// validation time matches framework idiom (Validators run before Plan).
func (r *certificateResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg certificateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// `type` may be unknown during validation if other unknown values feed it.
	if cfg.Type.IsUnknown() {
		return
	}

	// Default value handling: when not set, behaviour matches "custom".
	certType := cfg.Type.ValueString()
	if cfg.Type.IsNull() {
		certType = "custom"
	}

	switch certType {
	case "custom":
		if cfg.PrivateKey.IsNull() {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`private_key` is required for when type is `custom` or empty",
			)
		}
		if cfg.LeafCertificate.IsNull() {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`leaf_certificate` is required for when type is `custom` or empty",
			)
		}
	case "lets_encrypt":
		if cfg.Domains.IsNull() {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`domains` is required for when type is `lets_encrypt`",
			)
		}
	}
}

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.config.GodoClient()

	certReq, diags := buildCertificateRequest(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[INFO] Create a Certificate Request")
	log.Printf("[DEBUG] Certificate Create: %#v", certReq)
	cert, _, err := client.Certificates.Create(ctx, certReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Certificate", err.Error())
		return
	}

	// When the certificate type is lets_encrypt, the certificate ID will
	// change when it's renewed, so we have to rely on the certificate name
	// as the primary identifier instead.
	plan.ID = types.StringValue(cert.Name)
	plan.UUID = types.StringValue(cert.ID)

	// Translate the SDKv2 StateFunc: never persist the raw key/cert material
	// to state — store the SHA1 hash explicitly. See notes.md.
	if !plan.PrivateKey.IsNull() && !plan.PrivateKey.IsUnknown() {
		plan.PrivateKey = types.StringValue(util.HashString(plan.PrivateKey.ValueString()))
	}
	if !plan.LeafCertificate.IsNull() && !plan.LeafCertificate.IsUnknown() {
		plan.LeafCertificate = types.StringValue(util.HashString(plan.LeafCertificate.ValueString()))
	}
	if !plan.CertificateChain.IsNull() && !plan.CertificateChain.IsUnknown() {
		plan.CertificateChain = types.StringValue(util.HashString(plan.CertificateChain.ValueString()))
	}

	log.Printf("[INFO] Waiting for certificate (%s) to have state 'verified'", cert.Name)
	timeout := 10 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 && d < timeout {
			timeout = d
		}
	}

	finalCert, err := waitForCertificateState(
		ctx,
		client,
		cert.ID,
		[]string{"pending"},
		[]string{"verified"},
		3*time.Second,
		timeout,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for certificate (%s) to become active", cert.Name),
			err.Error(),
		)
		return
	}

	// Mirror the SDKv2 Create→Read pattern: refresh computed fields from the
	// final API response. Do NOT touch the hashed cert/key fields.
	r.applyCertToModel(ctx, finalCert, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.config.GodoClient()

	log.Printf("[INFO] Reading the details of the Certificate %s", state.ID.ValueString())
	cert, err := FindCertificateByName(client, state.ID.ValueString())
	if err != nil && strings.Contains(err.Error(), "not found") {
		log.Printf("[WARN] DigitalOcean Certificate (%s) not found", state.ID.ValueString())
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving Certificate", err.Error())
		return
	}

	r.applyCertToModel(ctx, cert, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Note: private_key, leaf_certificate, certificate_chain are intentionally
	// NOT refreshed from the API — the upstream API never returns them. The
	// hashed values written by Create remain in state until a forced replace
	// supersedes them. This matches SDKv2 behaviour.

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every user-facing attribute is ForceNew (RequiresReplace), so a non-
	// replace update should never surface practitioner changes. We still
	// implement Update so the framework satisfies the interface; we simply
	// pass the plan through to state.
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *certificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.config.GodoClient()

	log.Printf("[INFO] Deleting Certificate: %s", state.ID.ValueString())
	cert, err := FindCertificateByName(client, state.ID.ValueString())
	if err != nil && !strings.Contains(err.Error(), "not found") {
		resp.Diagnostics.AddError("Error retrieving Certificate", err.Error())
		return
	}
	if cert == nil {
		return
	}

	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)
	for {
		_, err := client.Certificates.Delete(ctx, cert.ID)
		if err == nil {
			return
		}
		if util.IsDigitalOceanError(err, http.StatusForbidden, "Make sure the certificate is not in use before deleting it") {
			log.Printf("[DEBUG] Received %s, retrying certificate deletion", err.Error())
			if time.Now().After(deadline) {
				resp.Diagnostics.AddError("Error deleting Certificate",
					fmt.Sprintf("timeout after %s: %s", timeout, err))
				return
			}
			select {
			case <-ctx.Done():
				resp.Diagnostics.AddError("Error deleting Certificate", ctx.Err().Error())
				return
			case <-time.After(1 * time.Second):
				continue
			}
		}
		resp.Diagnostics.AddError("Error deleting Certificate", err.Error())
		return
	}
}

func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// UpgradeState implements the framework's single-step state upgrader. The
// SDKv2 V0→V1 logic remapped the resource id to the certificate name and
// surfaced the original UUID separately.
func (r *certificateResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorCertificateSchemaV0(),
			StateUpgrader: upgradeCertificateStateV0toV1,
		},
	}
}

// priorCertificateSchemaV0 mirrors the SDKv2 V0 schema shape so the framework
// can deserialise prior states.
func priorCertificateSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":                schema.StringAttribute{Computed: true},
			"name":              schema.StringAttribute{Required: true},
			"private_key":       schema.StringAttribute{Optional: true, Sensitive: true},
			"leaf_certificate":  schema.StringAttribute{Optional: true},
			"certificate_chain": schema.StringAttribute{Optional: true},
			"domains": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
			},
			"type":             schema.StringAttribute{Optional: true, Computed: true},
			"state":            schema.StringAttribute{Computed: true},
			"not_after":        schema.StringAttribute{Computed: true},
			"sha1_fingerprint": schema.StringAttribute{Computed: true},
		},
	}
}

func upgradeCertificateStateV0toV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior certificateModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Println("[DEBUG] Migrating certificate schema from v0 to v1.")

	// V0 stored godo's UUID as the resource id; V1 promotes the certificate
	// name to id and tracks the UUID separately.
	current := certificateModel{
		ID:               prior.Name,
		UUID:             prior.ID,
		Name:             prior.Name,
		PrivateKey:       prior.PrivateKey,
		LeafCertificate:  prior.LeafCertificate,
		CertificateChain: prior.CertificateChain,
		Domains:          prior.Domains,
		Type:             prior.Type,
		State:            prior.State,
		NotAfter:         prior.NotAfter,
		SHA1Fingerprint:  prior.SHA1Fingerprint,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

// MigrateCertificateStateV0toV1 is preserved for the existing unit test
// (which exercises the raw map-based transformation). The framework upgrader
// above mirrors the same logic so both paths stay in sync.
func MigrateCertificateStateV0toV1(ctx context.Context, rawState map[string]interface{}, meta interface{}) (map[string]interface{}, error) {
	if len(rawState) == 0 {
		log.Println("[DEBUG] Empty state; nothing to migrate.")
		return rawState, nil
	}
	log.Println("[DEBUG] Migrating certificate schema from v0 to v1.")

	rawState["uuid"] = rawState["id"]
	rawState["id"] = rawState["name"]

	return rawState, nil
}

// ----- helpers -----

func buildCertificateRequest(ctx context.Context, plan certificateModel) (*godo.CertificateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	req := &godo.CertificateRequest{
		Name: plan.Name.ValueString(),
		Type: plan.Type.ValueString(),
	}
	if req.Type == "" {
		req.Type = "custom"
	}

	if !plan.PrivateKey.IsNull() && !plan.PrivateKey.IsUnknown() {
		req.PrivateKey = plan.PrivateKey.ValueString()
	}
	if !plan.LeafCertificate.IsNull() && !plan.LeafCertificate.IsUnknown() {
		req.LeafCertificate = plan.LeafCertificate.ValueString()
	}
	if !plan.CertificateChain.IsNull() && !plan.CertificateChain.IsUnknown() {
		req.CertificateChain = plan.CertificateChain.ValueString()
	}
	if !plan.Domains.IsNull() && !plan.Domains.IsUnknown() {
		var domains []string
		diags.Append(plan.Domains.ElementsAs(ctx, &domains, false)...)
		req.DNSNames = domains
	}

	return req, diags
}

func (r *certificateResource) applyCertToModel(ctx context.Context, cert *godo.Certificate, m *certificateModel, diags *diag.Diagnostics) {
	m.Name = types.StringValue(cert.Name)
	m.UUID = types.StringValue(cert.ID)
	m.Type = types.StringValue(cert.Type)
	m.State = types.StringValue(cert.State)
	m.NotAfter = types.StringValue(cert.NotAfter)
	m.SHA1Fingerprint = types.StringValue(cert.SHA1Fingerprint)
	if m.ID.IsNull() || m.ID.IsUnknown() {
		m.ID = types.StringValue(cert.Name)
	}

	domainsVal, d := types.SetValueFrom(ctx, types.StringType, filterEmpty(cert.DNSNames))
	diags.Append(d...)
	if d.HasError() {
		return
	}
	m.Domains = domainsVal
}

func filterEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// waitForCertificateState replaces SDKv2's retry.StateChangeConf for the
// "wait until verified" loop in Create.
func waitForCertificateState(
	ctx context.Context,
	client *godo.Client,
	uuid string,
	pending, target []string,
	pollInterval, timeout time.Duration,
) (*godo.Certificate, error) {
	deadline := time.Now().Add(timeout)
	// Initial delay matches the SDKv2 Delay: 10 * time.Second behaviour.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		cert, _, err := client.Certificates.Get(ctx, uuid)
		if err != nil {
			return nil, fmt.Errorf("Error retrieving certificate: %s", err)
		}
		state := cert.State
		if slices.Contains(target, state) {
			return cert, nil
		}
		if !slices.Contains(pending, state) {
			return cert, fmt.Errorf("unexpected state %q (pending=%v, target=%v)", state, pending, target)
		}
		if time.Now().After(deadline) {
			return cert, fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, state)
		}
		select {
		case <-ctx.Done():
			return cert, ctx.Err()
		case <-ticker.C:
		}
	}
}

// ----- plan modifiers -----

// hashSuppressModifier reproduces the SDKv2 DiffSuppressFunc that was attached
// to private_key / leaf_certificate / certificate_chain. The state value is
// always the SHA1 hash of the original material (written explicitly by
// Create); the plan/config value, on the other hand, is the raw material
// practitioners supply each plan. This modifier carries the prior (hashed)
// state value into the plan when the hash of the new raw value matches it —
// preventing a spurious replace on every plan even though the schema attribute
// itself is RequiresReplace.
type hashSuppressModifier struct{}

func (m hashSuppressModifier) Description(ctx context.Context) string {
	return "Suppresses diffs when the hash of the configured value equals the hashed value already stored in state."
}

func (m hashSuppressModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m hashSuppressModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	stateVal := req.StateValue.ValueString()
	configVal := req.ConfigValue.ValueString()
	if stateVal == "" || configVal == "" {
		return
	}
	// If the practitioner's raw input hashes to the current state value, keep
	// state — no real change.
	if util.HashString(configVal) == stateVal {
		resp.PlanValue = req.StateValue
	}
}

// customDomainsModifier reproduces the SDKv2 DiffSuppressFunc on `domains`
// that ignored config-vs-state diffs when type == "custom" (the API computes
// domains for custom certs).
type customDomainsModifier struct{}

func (m customDomainsModifier) Description(ctx context.Context) string {
	return "Carries the prior state's domains forward when the certificate type is `custom` (the API computes domains)."
}

func (m customDomainsModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m customDomainsModifier) PlanModifySet(ctx context.Context, req planmodifier.SetRequest, resp *planmodifier.SetResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	// Pull `type` out of the plan to decide whether to suppress.
	var typeVal types.String
	d := req.Plan.GetAttribute(ctx, path.Root("type"), &typeVal)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if typeVal.IsNull() || typeVal.IsUnknown() {
		return
	}
	if typeVal.ValueString() == "custom" {
		resp.PlanValue = req.StateValue
	}
}

// Compile-time assertions for plan modifier interfaces.
var (
	_ planmodifier.String = hashSuppressModifier{}
	_ planmodifier.Set    = customDomainsModifier{}
)
