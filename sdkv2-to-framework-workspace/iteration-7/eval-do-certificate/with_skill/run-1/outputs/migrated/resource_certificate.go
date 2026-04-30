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

// Compile-time interface checks. A missing method becomes a build error.
var (
	_ resource.Resource                   = &certificateResource{}
	_ resource.ResourceWithConfigure      = &certificateResource{}
	_ resource.ResourceWithImportState    = &certificateResource{}
	_ resource.ResourceWithUpgradeState   = &certificateResource{}
)

// NewCertificateResource is the framework constructor used by the provider's
// Resources() registration.
func NewCertificateResource() resource.Resource {
	return &certificateResource{}
}

type certificateResource struct {
	client *godo.Client
}

// certificateModel is the typed model for the current (V1) schema.
//
// Note: the secret-bearing attributes (private_key, leaf_certificate,
// certificate_chain) are plain types.String. SDKv2 used StateFunc=HashString
// to persist a hash of the user's value, not the raw value, so that the raw
// secret never appeared in plans and the diff was stable across runs. We
// preserve that semantics in the framework by:
//
//   - reading the *raw* user input from req.Plan / req.Config in Create,
//   - sending the raw value to the API,
//   - hashing the value ourselves before writing it back to state.
//
// We deliberately do NOT use a custom type whose ValueFromString() hashes its
// input. That pattern is broken: req.Plan, req.Config, and req.State all
// decode through the same custom type, so by the time Create reads the plan
// the secret is already hashed and the API call sends the hash. See
// references/state-and-types.md "Destructive StateFunc — do NOT use a
// destructive custom type".
//
// WriteOnly (framework v1.14+) is the longer-term cleanup, but flipping
// existing Sensitive attributes to WriteOnly is a practitioner-visible
// breaking change (state assertions on the hashed value would break) so it is
// deferred to a major version bump.
type certificateModel struct {
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
	UUID             types.String `tfsdk:"uuid"`
}

// certificateModelV0 mirrors the SDKv2 V0 schema (no `uuid` attribute, `id`
// was the godo certificate ID rather than the certificate name). It is only
// used by UpgradeState.
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

			// uuid: V1-introduced computed attribute; can change on auto-renewal
			// of a lets_encrypt certificate, so we do NOT use UseStateForUnknown
			// here (we want it to refresh from the API on each plan).
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("domains")),
				},
			},

			"leaf_certificate": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("domains")),
				},
			},

			"certificate_chain": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("domains")),
				},
			},

			// `domains`:
			//   - SDKv2 had ConflictsWith on the secret triple, which we
			//     express on the secret-side attributes via stringvalidator.
			//   - SDKv2 had a DiffSuppressFunc that ignored drift on `domains`
			//     when `type == "custom"` because the API computes the
			//     domains from the certificate SANs. The framework
			//     replacement is `Computed: true` + UseStateForUnknown — the
			//     API populates the value on Create/Read, and the prior
			//     state is reused on plan (avoiding "(known after apply)" or
			//     spurious diff).
			"domains": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
					setplanmodifier.UseStateForUnknown(),
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
		// Provider Configure has not run yet (e.g., during validation).
		return
	}

	combined, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("expected *config.CombinedConfig, got %T", req.ProviderData),
		)
		return
	}

	r.client = combined.GodoClient()
}

func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Passthrough: certificate ID-as-state is the certificate name (V1).
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// UpgradeState ports the SDKv2 V0→V1 upgrader. Note: framework upgraders are
// single-step; the map key is the *prior* version, and the body must produce
// the current state. Here the chain is just one step (V0→V1) so the
// transformation matches the SDKv2 MigrateCertificateStateV0toV1 verbatim:
// move id→uuid and name→id.
func (r *certificateResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
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
			},
			StateUpgrader: upgradeCertificateStateV0toV1,
		},
	}
}

func upgradeCertificateStateV0toV1(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior certificateModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if prior.ID.IsNull() && prior.Name.IsNull() {
		log.Println("[DEBUG] Empty state; nothing to migrate.")
	} else {
		log.Println("[DEBUG] Migrating certificate schema from v0 to v1.")
	}

	// V0 → V1: the V0 `id` (godo certificate UUID) is moved to the new `uuid`
	// attribute; `id` is replaced with the certificate name so that
	// lets_encrypt auto-renewals (which mint a new UUID) don't change the
	// resource address.
	current := certificateModel{
		ID:               prior.Name,
		Name:             prior.Name,
		PrivateKey:       prior.PrivateKey,
		LeafCertificate:  prior.LeafCertificate,
		CertificateChain: prior.CertificateChain,
		Domains:          prior.Domains,
		Type:             prior.Type,
		State:            prior.State,
		NotAfter:         prior.NotAfter,
		SHA1Fingerprint:  prior.SHA1Fingerprint,
		UUID:             prior.ID,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	certificateType := plan.Type.ValueString()
	if certificateType == "" {
		certificateType = "custom"
	}

	if certificateType == "custom" {
		if plan.PrivateKey.IsNull() || plan.PrivateKey.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`private_key` is required for when type is `custom` or empty",
			)
			return
		}
		if plan.LeafCertificate.IsNull() || plan.LeafCertificate.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`leaf_certificate` is required for when type is `custom` or empty",
			)
			return
		}
	} else if certificateType == "lets_encrypt" {
		if plan.Domains.IsNull() || len(plan.Domains.Elements()) == 0 {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`domains` is required for when type is `lets_encrypt`",
			)
			return
		}
	}

	log.Printf("[INFO] Create a Certificate Request")

	createReq := &godo.CertificateRequest{
		Name: plan.Name.ValueString(),
		Type: certificateType,
	}

	if !plan.PrivateKey.IsNull() && plan.PrivateKey.ValueString() != "" {
		createReq.PrivateKey = plan.PrivateKey.ValueString()
	}
	if !plan.LeafCertificate.IsNull() && plan.LeafCertificate.ValueString() != "" {
		createReq.LeafCertificate = plan.LeafCertificate.ValueString()
	}
	if !plan.CertificateChain.IsNull() && plan.CertificateChain.ValueString() != "" {
		createReq.CertificateChain = plan.CertificateChain.ValueString()
	}

	if !plan.Domains.IsNull() && !plan.Domains.IsUnknown() {
		var domains []string
		resp.Diagnostics.Append(plan.Domains.ElementsAs(ctx, &domains, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createReq.DNSNames = domains
	}

	log.Printf("[DEBUG] Certificate Create: %#v", createReq)
	cert, _, err := r.client.Certificates.Create(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Certificate", err.Error())
		return
	}

	// Wait for the cert to become 'verified' (replaces SDKv2
	// retry.StateChangeConf — the framework has no equivalent helper).
	log.Printf("[INFO] Waiting for certificate (%s) to have state 'verified'", cert.Name)
	final, err := waitForCertificateState(
		ctx,
		r.client,
		cert.ID,
		[]string{"pending"},
		[]string{"verified"},
		3*time.Second,
		15*time.Minute,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for certificate (%s) to become active", cert.Name),
			err.Error(),
		)
		return
	}
	if final != nil {
		cert = final
	}

	// Hash the secrets explicitly before writing them to state. SDKv2's
	// StateFunc=HashString did this implicitly; in the framework we do it
	// here. Existing state (from the SDKv2 release line) also stored hashes,
	// so this preserves the on-disk format and keeps existing tests'
	// HashString assertions valid.
	plan.PrivateKey = hashedOrNull(plan.PrivateKey)
	plan.LeafCertificate = hashedOrNull(plan.LeafCertificate)
	plan.CertificateChain = hashedOrNull(plan.CertificateChain)

	plan.ID = types.StringValue(cert.Name)
	plan.UUID = types.StringValue(cert.ID)
	plan.Name = types.StringValue(cert.Name)
	plan.Type = types.StringValue(cert.Type)
	plan.State = types.StringValue(cert.State)
	plan.NotAfter = types.StringValue(cert.NotAfter)
	plan.SHA1Fingerprint = types.StringValue(cert.SHA1Fingerprint)

	domainsValue, diags := flattenDomains(ctx, cert.DNSNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Domains = domainsValue

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
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

	state.Name = types.StringValue(cert.Name)
	state.UUID = types.StringValue(cert.ID)
	state.Type = types.StringValue(cert.Type)
	state.State = types.StringValue(cert.State)
	state.NotAfter = types.StringValue(cert.NotAfter)
	state.SHA1Fingerprint = types.StringValue(cert.SHA1Fingerprint)

	domainsValue, diags := flattenDomains(ctx, cert.DNSNames)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Domains = domainsValue

	// Note: the secret-bearing attributes (private_key, leaf_certificate,
	// certificate_chain) are NOT round-tripped from the API — DigitalOcean
	// never returns them. We leave whatever was already in state alone (the
	// hash from Create). If the practitioner changes the cert material in
	// config, the RequiresReplace plan modifier triggers replacement, and the
	// SDKv2 DiffSuppressFunc check `new != "" && old == d.Get(...)` is
	// preserved by the fact that `old` (the hashed prior state) compared
	// against the same hashed-on-write Plan value is stable.

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All user-controllable attributes carry RequiresReplace, so any change
	// produces a replacement, not an in-place update. There is nothing for
	// Update to do beyond writing the plan back to state.
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

	log.Printf("[INFO] Deleting Certificate: %s", state.ID.ValueString())
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
	for {
		_, err = r.client.Certificates.Delete(ctx, cert.ID)
		if err == nil {
			return
		}
		if !util.IsDigitalOceanError(err, http.StatusForbidden, "Make sure the certificate is not in use before deleting it") {
			resp.Diagnostics.AddError("Error deleting Certificate", err.Error())
			return
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError(
				"Error deleting Certificate",
				fmt.Sprintf("timeout after %s waiting for certificate (%s) to be deletable: %s", timeout, cert.ID, err),
			)
			return
		}
		log.Printf("[DEBUG] Received %s, retrying certificate deletion", err.Error())
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Error deleting Certificate", ctx.Err().Error())
			return
		case <-time.After(1 * time.Second):
		}
	}
}

// waitForCertificateState replaces SDKv2 retry.StateChangeConf for the
// pending→verified loop. Returns the final certificate (so the caller can
// pick up the API-populated DNSNames / SHA1Fingerprint after verification)
// and an error if the wait fails.
func waitForCertificateState(
	ctx context.Context,
	client *godo.Client,
	uuid string,
	pending, target []string,
	pollInterval, timeout time.Duration,
) (*godo.Certificate, error) {
	deadline := time.Now().Add(timeout)
	// Initial 10s delay to mirror SDKv2 StateChangeConf{Delay: 10*time.Second}.
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
			return cert, fmt.Errorf("unexpected state %q (pending=%v, target=%v)", cert.State, pending, target)
		}
		if time.Now().After(deadline) {
			return cert, fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, cert.State)
		}
		select {
		case <-ctx.Done():
			return cert, ctx.Err()
		case <-ticker.C:
		}
	}
}

// hashedOrNull preserves null/unknown but hashes any known string value, so
// the schema attribute carries the same SHA1-hex form SDKv2's StateFunc
// produced.
func hashedOrNull(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() {
		return types.StringNull()
	}
	s := v.ValueString()
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(util.HashString(s))
}

func flattenDomains(ctx context.Context, domains []string) (types.Set, diag.Diagnostics) {
	if domains == nil {
		domains = []string{}
	}
	cleaned := make([]string, 0, len(domains))
	for _, d := range domains {
		if d != "" {
			cleaned = append(cleaned, d)
		}
	}
	return types.SetValueFrom(ctx, types.StringType, cleaned)
}
