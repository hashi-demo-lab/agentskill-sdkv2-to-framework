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
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                 = &certificateResource{}
	_ resource.ResourceWithConfigure    = &certificateResource{}
	_ resource.ResourceWithImportState  = &certificateResource{}
	_ resource.ResourceWithUpgradeState = &certificateResource{}
)

// NewCertificateResource returns a new framework resource for digitalocean_certificate.
func NewCertificateResource() resource.Resource {
	return &certificateResource{}
}

type certificateResource struct {
	client *godo.Client
}

// certificateModel is the typed model for the current (v1) schema.
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

// certificateModelV0 is the typed model for the v0 schema (no uuid field; id = API uuid).
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

// ----------------------------------------------------------------------------
// Metadata & Schema
// ----------------------------------------------------------------------------

func (r *certificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *certificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// uuid is the DigitalOcean API-assigned certificate UUID.  When a
			// let's_encrypt cert renews, the UUID changes but the name (= id) stays.
			"uuid": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// private_key, leaf_certificate, and certificate_chain:
			// SDKv2 StateFunc hashed the raw value before storing it in state.
			// The equivalent here is to hash on Create/Update and store the hash.
			// The hashSuppressPlanModifier suppresses a diff when the configured
			// raw value hashes to the value already in state (matching the
			// SDKv2 DiffSuppressFunc behaviour for old statefiles).
			"private_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					hashSuppressPlanModifier{},
				},
			},
			"leaf_certificate": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					hashSuppressPlanModifier{},
				},
			},
			"certificate_chain": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					hashSuppressPlanModifier{},
				},
			},
			// domains: ConflictsWith → setvalidator.ConflictsWith.
			// The domains attribute is computed for custom certs and should be
			// ignored in diffs → domainsSuppressPlanModifier.
			"domains": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Validators: []validator.Set{
					setvalidator.ConflictsWith(
						path.MatchRoot("private_key"),
						path.MatchRoot("leaf_certificate"),
						path.MatchRoot("certificate_chain"),
					),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
					domainsSuppressPlanModifier{},
				},
			},
			"type": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("custom"),
				Validators: []validator.String{
					stringvalidator.OneOf("custom", "lets_encrypt"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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

// ----------------------------------------------------------------------------
// Plan modifiers
// ----------------------------------------------------------------------------

// hashSuppressPlanModifier suppresses a diff when the state already holds the
// SHA1 hash of the configured value.  This matches the SDKv2 DiffSuppressFunc
// that compared old == d.Get(...) for private_key, leaf_certificate, and
// certificate_chain — "old statefiles" stored the full PEM; newer ones store
// only the hash.
type hashSuppressPlanModifier struct{}

func (hashSuppressPlanModifier) Description(_ context.Context) string {
	return "Suppresses diff when state holds the SHA1 hash of the configured value."
}
func (hashSuppressPlanModifier) MarkdownDescription(_ context.Context) string {
	return "Suppresses diff when state holds the SHA1 hash of the configured value."
}
func (hashSuppressPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	planVal := req.PlanValue.ValueString()
	stateVal := req.StateValue.ValueString()
	if planVal != "" && util.HashString(planVal) == stateVal {
		// The raw value hashes to what's in state — suppress the diff.
		resp.PlanValue = req.StateValue
	}
}

// domainsSuppressPlanModifier suppresses a diff on "domains" when the cert
// type is "custom" — the API computes domains for custom certs so the field
// is server-managed; matches the SDKv2 DiffSuppressFunc.
type domainsSuppressPlanModifier struct{}

func (domainsSuppressPlanModifier) Description(_ context.Context) string {
	return "Suppresses diff on domains when certificate type is custom."
}
func (domainsSuppressPlanModifier) MarkdownDescription(_ context.Context) string {
	return "Suppresses diff on domains when certificate type is custom."
}
func (domainsSuppressPlanModifier) PlanModifySet(ctx context.Context, req planmodifier.SetRequest, resp *planmodifier.SetResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	var certType types.String
	// GetAttribute appends diagnostics internally; ignore the return — if the
	// read fails (unknown during plan) we simply don't suppress.
	_ = req.Config.GetAttribute(ctx, path.Root("type"), &certType)
	if !certType.IsNull() && !certType.IsUnknown() && certType.ValueString() == "custom" {
		resp.PlanValue = req.StateValue
	}
}

// ----------------------------------------------------------------------------
// UpgradeState  (SchemaVersion 0 → 1)
// ----------------------------------------------------------------------------

func (r *certificateResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Single upgrader: V0 → current (V1).
		// V0 shape: id = API UUID, no uuid field.
		// V1 shape: id = cert name (stable across renewals), uuid = API UUID.
		0: {
			PriorSchema: priorSchemaV0(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior certificateModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				current := certificateModel{
					ID:               prior.Name, // new stable id is the cert name
					UUID:             prior.ID,   // old id (API UUID) becomes uuid
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
			},
		},
	}
}

// priorSchemaV0 describes the V0 schema so the framework can deserialise old state.
func priorSchemaV0() *schema.Schema {
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

// MigrateCertificateStateV0toV1 is preserved so the existing unit test
// (TestResourceExampleInstanceStateUpgradeV0) continues to compile and pass.
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

// ----------------------------------------------------------------------------
// Configure
// ----------------------------------------------------------------------------

func (r *certificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ----------------------------------------------------------------------------
// ImportState
// ----------------------------------------------------------------------------

func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ----------------------------------------------------------------------------
// Create
// ----------------------------------------------------------------------------

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Cross-attribute validation (mirrors the SDKv2 Create checks).
	certType := plan.Type.ValueString()
	switch certType {
	case "custom", "":
		if plan.PrivateKey.IsNull() || plan.PrivateKey.IsUnknown() || plan.PrivateKey.ValueString() == "" {
			resp.Diagnostics.AddError("Missing required attribute",
				"`private_key` is required for when type is `custom` or empty")
		}
		if plan.LeafCertificate.IsNull() || plan.LeafCertificate.IsUnknown() || plan.LeafCertificate.ValueString() == "" {
			resp.Diagnostics.AddError("Missing required attribute",
				"`leaf_certificate` is required for when type is `custom` or empty")
		}
	case "lets_encrypt":
		if plan.Domains.IsNull() || plan.Domains.IsUnknown() || len(plan.Domains.Elements()) == 0 {
			resp.Diagnostics.AddError("Missing required attribute",
				"`domains` is required for when type is `lets_encrypt`")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	certReq := r.buildCertificateRequest(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Certificate Create: %#v", certReq))
	cert, _, err := r.client.Certificates.Create(ctx, certReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Certificate", err.Error())
		return
	}

	// The stable id is the certificate name (not the API UUID) so that
	// let's_encrypt renewals don't change the resource identity.
	plan.ID = types.StringValue(cert.Name)
	plan.UUID = types.StringValue(cert.ID)

	// StateFunc equivalent: store hashes of cert material rather than the raw PEM.
	if !plan.PrivateKey.IsNull() && !plan.PrivateKey.IsUnknown() {
		plan.PrivateKey = types.StringValue(util.HashString(plan.PrivateKey.ValueString()))
	}
	if !plan.LeafCertificate.IsNull() && !plan.LeafCertificate.IsUnknown() {
		plan.LeafCertificate = types.StringValue(util.HashString(plan.LeafCertificate.ValueString()))
	}
	if !plan.CertificateChain.IsNull() && !plan.CertificateChain.IsUnknown() {
		plan.CertificateChain = types.StringValue(util.HashString(plan.CertificateChain.ValueString()))
	}

	tflog.Info(ctx, fmt.Sprintf("Waiting for certificate (%s) to have state 'verified'", cert.Name))
	uuid := cert.ID
	_, waitErr := waitForCertificateState(
		ctx,
		func() (interface{}, string, error) {
			c, _, err := r.client.Certificates.Get(ctx, uuid)
			if err != nil {
				return nil, "", fmt.Errorf("error retrieving certificate: %s", err)
			}
			return c, c.State, nil
		},
		[]string{"pending"},
		[]string{"verified"},
		10*time.Second,
		3*time.Minute,
	)
	if waitErr != nil {
		resp.Diagnostics.AddError(
			"Error waiting for certificate to become active",
			fmt.Sprintf("Error waiting for certificate (%s) to become active: %s", cert.Name, waitErr),
		)
		return
	}

	// Refresh computed fields from the API (state, not_after, sha1_fingerprint, domains).
	// We preserve the hashed cert material we already set above.
	finalCert, err := FindCertificateByName(r.client, cert.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading Certificate after create", err.Error())
		return
	}
	plan.State = types.StringValue(finalCert.State)
	plan.NotAfter = types.StringValue(finalCert.NotAfter)
	plan.SHA1Fingerprint = types.StringValue(finalCert.SHA1Fingerprint)
	plan.Type = types.StringValue(finalCert.Type)

	domainSet, domainsDiags := flattenCertificateDomains(ctx, finalCert.DNSNames)
	resp.Diagnostics.Append(domainsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Domains = domainSet

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ----------------------------------------------------------------------------
// Read
// ----------------------------------------------------------------------------

func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Reading the details of the Certificate %s", state.ID.ValueString()))
	cert, err := FindCertificateByName(r.client, state.ID.ValueString())
	if cert == nil && err != nil && strings.Contains(err.Error(), "not found") {
		tflog.Warn(ctx, fmt.Sprintf("DigitalOcean Certificate (%s) not found", state.ID.ValueString()))
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving Certificate", err.Error())
		return
	}

	state.Name = types.StringValue(cert.Name)
	state.UUID = types.StringValue(cert.ID)
	state.Type = types.StringValue(cert.Type)
	state.State = types.StringValue(cert.State)
	state.NotAfter = types.StringValue(cert.NotAfter)
	state.SHA1Fingerprint = types.StringValue(cert.SHA1Fingerprint)

	domainSet, domainsDiags := flattenCertificateDomains(ctx, cert.DNSNames)
	resp.Diagnostics.Append(domainsDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Domains = domainSet

	// Note: private_key, leaf_certificate, certificate_chain are write-only
	// from the API's perspective — it never returns them.  We intentionally
	// leave those fields as they are in state (the hash we wrote on Create).

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ----------------------------------------------------------------------------
// Update
// ----------------------------------------------------------------------------

func (r *certificateResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All schema attributes carry RequiresReplace; Update is never invoked.
}

// ----------------------------------------------------------------------------
// Delete
// ----------------------------------------------------------------------------

func (r *certificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Info(ctx, fmt.Sprintf("Deleting Certificate: %s", state.ID.ValueString()))
	cert, err := FindCertificateByName(r.client, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving Certificate", err.Error())
		return
	}
	if cert == nil {
		return
	}

	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		_, delErr := r.client.Certificates.Delete(ctx, cert.ID)
		if delErr == nil {
			break
		}
		if util.IsDigitalOceanError(delErr, http.StatusForbidden, "Make sure the certificate is not in use before deleting it") {
			tflog.Debug(ctx, fmt.Sprintf("Received %s, retrying certificate deletion", delErr.Error()))
			if time.Now().After(deadline) {
				resp.Diagnostics.AddError("Error deleting Certificate",
					fmt.Sprintf("timeout waiting to delete certificate: %s", delErr))
				return
			}
			select {
			case <-ctx.Done():
				resp.Diagnostics.AddError("Error deleting Certificate", ctx.Err().Error())
				return
			case <-ticker.C:
			}
			continue
		}
		resp.Diagnostics.AddError("Error deleting Certificate", delErr.Error())
		return
	}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// buildCertificateRequest constructs a *godo.CertificateRequest from the plan.
// Diagnostics are appended to *diags so the caller checks HasError() afterwards.
func (r *certificateResource) buildCertificateRequest(ctx context.Context, plan certificateModel, diags *diag.Diagnostics) *godo.CertificateRequest {
	certReq := &godo.CertificateRequest{
		Name: plan.Name.ValueString(),
		Type: plan.Type.ValueString(),
	}
	if !plan.PrivateKey.IsNull() && !plan.PrivateKey.IsUnknown() {
		certReq.PrivateKey = plan.PrivateKey.ValueString()
	}
	if !plan.LeafCertificate.IsNull() && !plan.LeafCertificate.IsUnknown() {
		certReq.LeafCertificate = plan.LeafCertificate.ValueString()
	}
	if !plan.CertificateChain.IsNull() && !plan.CertificateChain.IsUnknown() {
		certReq.CertificateChain = plan.CertificateChain.ValueString()
	}
	if !plan.Domains.IsNull() && !plan.Domains.IsUnknown() {
		var domains []string
		diags.Append(plan.Domains.ElementsAs(ctx, &domains, false)...)
		certReq.DNSNames = domains
	}
	return certReq
}

// flattenCertificateDomains converts a []string to a types.Set(string).
func flattenCertificateDomains(ctx context.Context, domains []string) (types.Set, diag.Diagnostics) {
	if len(domains) == 0 {
		return types.SetValueMust(types.StringType, []attr.Value{}), diag.Diagnostics{}
	}
	return types.SetValueFrom(ctx, types.StringType, domains)
}

// waitForCertificateState polls refresh until the returned state is in target.
// It replaces terraform-plugin-sdk/v2/helper/retry.StateChangeConf.
func waitForCertificateState(
	ctx context.Context,
	refresh func() (interface{}, string, error),
	pending, target []string,
	pollInterval, timeout time.Duration,
) (interface{}, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		v, state, err := refresh()
		if err != nil {
			return v, err
		}
		if slices.Contains(target, state) {
			return v, nil
		}
		if !slices.Contains(pending, state) {
			return v, fmt.Errorf("unexpected state %q (pending=%v, target=%v)", state, pending, target)
		}
		if time.Now().After(deadline) {
			return v, fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, state)
		}
		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-ticker.C:
		}
	}
}
