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
	_ resource.Resource                = &certificateResource{}
	_ resource.ResourceWithConfigure   = &certificateResource{}
	_ resource.ResourceWithImportState = &certificateResource{}
	_ resource.ResourceWithUpgradeState = &certificateResource{}
)

// NewCertificateResource returns the framework resource implementation.
func NewCertificateResource() resource.Resource {
	return &certificateResource{}
}

type certificateResource struct {
	client *godo.Client
}

// certificateModel is the typed model for the current (V1) schema.
//
// IMPORTANT: PrivateKey, LeafCertificate, and CertificateChain are
// `types.String` (not a custom type). The raw values flow from
// req.Config to the API call in Create; the resource then writes the
// SHA-1 hash to state via resp.State.SetAttribute. This mirrors the
// SDKv2 StateFunc behaviour without the destructive-custom-type
// foot-gun (a CustomType whose ValueFromString hashes would also
// hash req.Config — by the time Create reads the raw secret, it would
// already be hashed and the API call would fail silently).
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
			// uuid will change on auto-renewal of a lets_encrypt certificate
			// — DO NOT use UseStateForUnknown here.
			"uuid": schema.StringAttribute{
				Computed: true,
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
			"private_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				// Plan modifiers run in slice order. Hash-suppress runs first
				// so RequiresReplace sees state == plan when the raw config
				// hashes to the stored hash (no replace).
				PlanModifiers: []planmodifier.String{
					hashSuppressPlanModifier{},
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"leaf_certificate": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					hashSuppressPlanModifier{},
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"certificate_chain": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					hashSuppressPlanModifier{},
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
					// Replicates the SDKv2 DiffSuppressFunc behaviour: when
					// type == "custom" the API computes domains, so suppress
					// any diff (carry state forward in the plan).
					customTypeDomainsSuppressPlanModifier{},
					setplanmodifier.RequiresReplace(),
					setplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Set{
					setvalidator.ConflictsWith(
						path.MatchRoot("private_key"),
						path.MatchRoot("leaf_certificate"),
						path.MatchRoot("certificate_chain"),
					),
					// Per-element non-empty check (SDKv2 had ValidateFunc:
					// validation.NoZeroValues on the inner Elem).
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
			},
			"not_after": schema.StringAttribute{
				Computed: true,
			},
			"sha1_fingerprint": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *certificateResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cc, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected *config.CombinedConfig, got %T", req.ProviderData),
		)
		return
	}
	r.client = cc.GodoClient()
}

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Read the RAW values from config so we can build the API request with
	// the unhashed secret material. Plan values may have been replaced with
	// state (hashed) by the hashSuppressPlanModifier when nothing changed.
	var cfg certificateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	certType := plan.Type.ValueString()
	if certType == "custom" || certType == "" {
		if cfg.PrivateKey.IsNull() || cfg.PrivateKey.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`private_key` is required for when type is `custom` or empty",
			)
			return
		}
		if cfg.LeafCertificate.IsNull() || cfg.LeafCertificate.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`leaf_certificate` is required for when type is `custom` or empty",
			)
			return
		}
	} else if certType == "lets_encrypt" {
		if cfg.Domains.IsNull() || len(cfg.Domains.Elements()) == 0 {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`domains` is required for when type is `lets_encrypt`",
			)
			return
		}
	}

	log.Printf("[INFO] Create a Certificate Request")

	certReq := &godo.CertificateRequest{
		Name: plan.Name.ValueString(),
		Type: certType,
	}
	if !cfg.PrivateKey.IsNull() && !cfg.PrivateKey.IsUnknown() {
		certReq.PrivateKey = cfg.PrivateKey.ValueString()
	}
	if !cfg.LeafCertificate.IsNull() && !cfg.LeafCertificate.IsUnknown() {
		certReq.LeafCertificate = cfg.LeafCertificate.ValueString()
	}
	if !cfg.CertificateChain.IsNull() && !cfg.CertificateChain.IsUnknown() {
		certReq.CertificateChain = cfg.CertificateChain.ValueString()
	}
	if !cfg.Domains.IsNull() && !cfg.Domains.IsUnknown() {
		domains := make([]string, 0, len(cfg.Domains.Elements()))
		diags := cfg.Domains.ElementsAs(ctx, &domains, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		certReq.DNSNames = domains
	}

	log.Printf("[DEBUG] Certificate Create: %#v", certReq)
	cert, _, err := r.client.Certificates.Create(ctx, certReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Certificate", err.Error())
		return
	}

	// The certificate name is the primary identifier (lets_encrypt UUIDs
	// rotate on auto-renewal — name is stable).
	plan.ID = types.StringValue(cert.Name)
	plan.UUID = types.StringValue(cert.ID)
	plan.Type = types.StringValue(cert.Type)

	// Wait until the certificate is verified.
	log.Printf("[INFO] Waiting for certificate (%s) to have state 'verified'", cert.Name)
	createTimeout := 5 * time.Minute // SDKv2 had no explicit Create timeout; framework default behaviour mirrors the SDK 20m, but the resource has no Timeouts block — keep something reasonable.
	finalCert, err := waitForCertificateState(
		ctx,
		r.client,
		cert.ID,
		[]string{"pending"},
		[]string{"verified"},
		3*time.Second,
		createTimeout,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for certificate to become active",
			fmt.Sprintf("certificate %q: %s", plan.Name.ValueString(), err),
		)
		return
	}

	// Populate the rest of the state from the final certificate.
	plan.State = types.StringValue(finalCert.State)
	plan.NotAfter = types.StringValue(finalCert.NotAfter)
	plan.SHA1Fingerprint = types.StringValue(finalCert.SHA1Fingerprint)
	plan.Domains = flattenDomainsSet(finalCert.DNSNames)

	// Cert-material attributes: store the SHA-1 hash of the raw config value
	// (matching SDKv2 StateFunc semantics) — the raw is never persisted to
	// state. If the practitioner didn't set the field, leave it null.
	plan.PrivateKey = hashOrNull(cfg.PrivateKey)
	plan.LeafCertificate = hashOrNull(cfg.LeafCertificate)
	plan.CertificateChain = hashOrNull(cfg.CertificateChain)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[INFO] Reading the details of the Certificate %s", state.ID.ValueString())
	cert, err := FindCertificateByName(r.client, state.ID.ValueString())
	if cert == nil && err != nil && strings.Contains(err.Error(), "not found") {
		log.Printf("[WARN] DigitalOcean Certificate (%s) not found", state.ID.ValueString())
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving Certificate", err.Error())
		return
	}

	// API-derived fields. cert-material attributes (private_key /
	// leaf_certificate / certificate_chain) are NOT echoed by the API — leave
	// the prior state values (the stored hashes) unchanged. This matches
	// SDKv2 behaviour: the SDKv2 Read also skipped d.Set for these fields.
	state.Name = types.StringValue(cert.Name)
	state.UUID = types.StringValue(cert.ID)
	state.Type = types.StringValue(cert.Type)
	state.State = types.StringValue(cert.State)
	state.NotAfter = types.StringValue(cert.NotAfter)
	state.SHA1Fingerprint = types.StringValue(cert.SHA1Fingerprint)
	state.Domains = flattenDomainsSet(cert.DNSNames)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is not used at the schema level (every user-settable attribute is
// ForceNew/RequiresReplace), but the framework requires the method.
func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *certificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[INFO] Deleting Certificate: %s", state.ID.ValueString())
	cert, err := FindCertificateByName(r.client, state.ID.ValueString())
	if err != nil {
		// SDKv2 surfaced this as a hard error rather than a missing-resource
		// no-op; preserve that.
		if strings.Contains(err.Error(), "not found") {
			return
		}
		resp.Diagnostics.AddError("Error retrieving Certificate", err.Error())
		return
	}
	if cert == nil {
		return
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		_, derr := r.client.Certificates.Delete(ctx, cert.ID)
		if derr == nil {
			return
		}
		if util.IsDigitalOceanError(derr, http.StatusForbidden, "Make sure the certificate is not in use before deleting it") {
			log.Printf("[DEBUG] Received %s, retrying certificate deletion", derr.Error())
			if time.Now().After(deadline) {
				resp.Diagnostics.AddError("Error deleting Certificate", derr.Error())
				return
			}
			select {
			case <-ctx.Done():
				resp.Diagnostics.AddError("Error deleting Certificate", ctx.Err().Error())
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}
		resp.Diagnostics.AddError("Error deleting Certificate", derr.Error())
		return
	}
}

func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ----- State upgrade (V0 -> V1) ---------------------------------------------

// V0 schema mirrors the SDKv2 V0 shape. `id` was the certificate UUID; in V1
// `id` becomes the certificate name and `uuid` carries the original ID.
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
				Computed:    true,
			},
			"type":             schema.StringAttribute{Optional: true, Computed: true},
			"state":            schema.StringAttribute{Computed: true},
			"not_after":        schema.StringAttribute{Computed: true},
			"sha1_fingerprint": schema.StringAttribute{Computed: true},
		},
	}
}

func (r *certificateResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeCertificateStateV0,
		},
	}
}

func upgradeCertificateStateV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior certificateModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Println("[DEBUG] Migrating certificate schema from v0 to v1.")
	// V0: id == certificate UUID. V1: id == certificate name (stable across
	// lets_encrypt auto-renewal); uuid carries the V0 id.
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
	resp.Diagnostics.Append(resp.State.Set(ctx, &current)...)
}

// MigrateCertificateStateV0toV1 is preserved for the existing unit test that
// drives the upgrader against a raw map. The framework upgrader operates on
// typed state; this helper keeps the same shape semantics for that test.
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

// ----- Plan modifiers -------------------------------------------------------

// hashSuppressPlanModifier carries the prior-state value (the stored hash)
// forward when the new config value's SHA-1 hash equals the state value.
// This lets practitioners keep the raw cert material in their HCL config
// (with its trailing newline) without triggering a replace cycle on every
// plan, mirroring the SDKv2 DiffSuppressFunc behaviour.
type hashSuppressPlanModifier struct{}

func (m hashSuppressPlanModifier) Description(ctx context.Context) string {
	return "Suppresses diffs when the SHA-1 hash of the configured value matches the prior-state hash."
}

func (m hashSuppressPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m hashSuppressPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Skip on resource creation (no state) and destruction (no plan).
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	// If the config value hashes to the stored state hash, carry state
	// forward so RequiresReplace doesn't fire.
	if util.HashString(req.ConfigValue.ValueString()) == req.StateValue.ValueString() {
		resp.PlanValue = req.StateValue
	}
}

// customTypeDomainsSuppressPlanModifier replicates the SDKv2 DiffSuppressFunc
// on `domains`: when type == "custom", domains are computed by the API so
// suppress any plan-time diff by carrying state forward.
type customTypeDomainsSuppressPlanModifier struct{}

func (m customTypeDomainsSuppressPlanModifier) Description(ctx context.Context) string {
	return "When the certificate type is `custom`, suppress diffs on `domains` (computed by the API)."
}

func (m customTypeDomainsSuppressPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m customTypeDomainsSuppressPlanModifier) PlanModifySet(ctx context.Context, req planmodifier.SetRequest, resp *planmodifier.SetResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	// Read the planned `type`; if it's "custom", carry state forward.
	var planType types.String
	diags := req.Plan.GetAttribute(ctx, path.Root("type"), &planType)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !planType.IsNull() && !planType.IsUnknown() && planType.ValueString() == "custom" {
		resp.PlanValue = req.StateValue
	}
}

// ----- Helpers --------------------------------------------------------------

func hashOrNull(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return types.StringNull()
	}
	return types.StringValue(util.HashString(v.ValueString()))
}

func flattenDomainsSet(domains []string) types.Set {
	if domains == nil {
		return types.SetNull(types.StringType)
	}
	elems := make([]string, 0, len(domains))
	for _, v := range domains {
		if v != "" {
			elems = append(elems, v)
		}
	}
	out, _ := types.SetValueFrom(context.Background(), types.StringType, elems)
	return out
}

// waitForCertificateState polls the godo Certificates.Get endpoint until the
// certificate's state is in `target`, returning the latest certificate.
//
// Replaces SDKv2's helper/retry.StateChangeConf.WaitForStateContext (no
// framework equivalent — see references/resources.md).
func waitForCertificateState(
	ctx context.Context,
	client *godo.Client,
	uuid string,
	pending, target []string,
	pollInterval, timeout time.Duration,
) (*godo.Certificate, error) {
	deadline := time.Now().Add(timeout)
	// Initial 10s delay matches the SDKv2 stateConf.Delay.
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
		if slices.Contains(target, cert.State) {
			return cert, nil
		}
		if !slices.Contains(pending, cert.State) {
			return cert, fmt.Errorf("unexpected certificate state %q", cert.State)
		}
		if time.Now().After(deadline) {
			return cert, fmt.Errorf("timeout waiting for certificate to become %v (last state=%q)", target, cert.State)
		}
		select {
		case <-ctx.Done():
			return cert, ctx.Err()
		case <-ticker.C:
		}
	}
}
