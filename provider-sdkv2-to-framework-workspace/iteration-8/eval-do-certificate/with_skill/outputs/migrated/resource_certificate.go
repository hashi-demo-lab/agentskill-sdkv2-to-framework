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
)

// Compile-time interface assertions.
var (
	_ resource.Resource                 = &certificateResource{}
	_ resource.ResourceWithConfigure    = &certificateResource{}
	_ resource.ResourceWithImportState  = &certificateResource{}
	_ resource.ResourceWithUpgradeState = &certificateResource{}
)

// NewCertificateResource constructs the framework resource.
func NewCertificateResource() resource.Resource {
	return &certificateResource{}
}

type certificateResource struct {
	client *godo.Client
}

// certificateModel is the typed state/plan model for schema version 1.
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

// certificateModelV0 is the typed prior-state model for schema version 0.
// The V0 schema did not have a uuid attribute; id held the certificate API ID.
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

// Metadata implements resource.Resource.
func (r *certificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

// Schema implements resource.Resource.
// This is schema version 1. Version 0 is captured in priorSchemaV0() below.
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

			// uuid holds the DigitalOcean API certificate ID.
			// When the certificate type is lets_encrypt, the API ID changes on
			// auto-renewal; the practitioner-facing resource ID is the cert name.
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

			// private_key is stored as a SHA-1 hash in state to avoid persisting
			// the raw secret. The hash is computed in Create (mirroring the SDKv2
			// StateFunc behaviour). DiffSuppressFunc for old statefiles with raw
			// values is intentionally dropped; any re-plan will detect a diff and
			// the resource will be recreated (ForceNew semantics preserved).
			"private_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					// Mirrors SDKv2 ConflictsWith on private_key.
					stringvalidator.ConflictsWith(path.MatchRoot("domains")),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			// leaf_certificate is stored as a SHA-1 hash in state.
			"leaf_certificate": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("domains")),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			// certificate_chain is stored as a SHA-1 hash in state.
			"certificate_chain": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.MatchRoot("domains")),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			// domains is used for lets_encrypt certificates.
			// ConflictsWith replaces the SDKv2 ConflictsWith schema field.
			// The SDKv2 DiffSuppressFunc that suppressed domain diffs for custom
			// certs is intentionally dropped: Read does not populate domains for
			// custom certs, so state stays empty and no diff arises.
			"domains": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
					setvalidator.ConflictsWith(
						path.MatchRoot("private_key"),
						path.MatchRoot("leaf_certificate"),
						path.MatchRoot("certificate_chain"),
					),
				},
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
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

// Configure implements resource.ResourceWithConfigure.
func (r *certificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cc, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *config.CombinedConfig, got %T", req.ProviderData),
		)
		return
	}
	r.client = cc.GodoClient()
}

// Create implements resource.Resource.
func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	certType := plan.Type.ValueString()
	switch certType {
	case "custom":
		if plan.PrivateKey.IsNull() || plan.PrivateKey.IsUnknown() {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`private_key` is required for when type is `custom` or empty",
			)
		}
		if plan.LeafCertificate.IsNull() || plan.LeafCertificate.IsUnknown() {
			resp.Diagnostics.AddError(
				"Missing required attribute",
				"`leaf_certificate` is required for when type is `custom` or empty",
			)
		}
		if resp.Diagnostics.HasError() {
			return
		}
	case "lets_encrypt":
		if plan.Domains.IsNull() || plan.Domains.IsUnknown() {
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
		var domainElems []types.String
		resp.Diagnostics.Append(plan.Domains.ElementsAs(ctx, &domainElems, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		certReq.DNSNames = make([]string, len(domainElems))
		for i, d := range domainElems {
			certReq.DNSNames[i] = d.ValueString()
		}
	}

	log.Printf("[DEBUG] Certificate Create: %#v", certReq)
	cert, _, err := r.client.Certificates.Create(ctx, certReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Certificate", err.Error())
		return
	}

	// When the certificate type is lets_encrypt, the certificate API ID changes
	// on auto-renewal; use the certificate name as the Terraform resource ID.
	plan.ID = types.StringValue(cert.Name)
	plan.UUID = types.StringValue(cert.ID)

	// Hash the sensitive PEM material before storing in state, mirroring the
	// SDKv2 StateFunc behaviour. Diffs remain stable on subsequent plans because
	// the stored value is always the hash, not the raw PEM.
	if !plan.PrivateKey.IsNull() && !plan.PrivateKey.IsUnknown() {
		plan.PrivateKey = types.StringValue(util.HashString(plan.PrivateKey.ValueString()))
	}
	if !plan.LeafCertificate.IsNull() && !plan.LeafCertificate.IsUnknown() {
		plan.LeafCertificate = types.StringValue(util.HashString(plan.LeafCertificate.ValueString()))
	}
	if !plan.CertificateChain.IsNull() && !plan.CertificateChain.IsUnknown() {
		plan.CertificateChain = types.StringValue(util.HashString(plan.CertificateChain.ValueString()))
	}

	// Persist preliminary state before waiting, so the resource ID is recorded
	// even if the wait times out.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[INFO] Waiting for certificate (%s) to have state 'verified'", cert.Name)

	certID := cert.ID
	_, waitErr := waitForState(
		ctx,
		func() (any, string, error) {
			c, _, e := r.client.Certificates.Get(ctx, certID)
			if e != nil {
				return nil, "", fmt.Errorf("error retrieving certificate: %s", e)
			}
			return c, c.State, nil
		},
		[]string{"pending"},
		[]string{"verified"},
		10*time.Second,
		10*time.Minute,
	)
	if waitErr != nil {
		resp.Diagnostics.AddError(
			"Error waiting for certificate to become active",
			fmt.Sprintf("certificate %s: %s", cert.Name, waitErr),
		)
		return
	}

	// Re-read from the API to populate all computed fields (state, not_after, etc.).
	r.readIntoState(ctx, plan.ID.ValueString(), &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read implements resource.Resource.
func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// When the certificate type is lets_encrypt, the certificate API ID changes
	// on auto-renewal; look up by name (the Terraform resource ID).
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

	domains, domainDiags := flattenCertificateDomains(ctx, cert.DNSNames)
	resp.Diagnostics.Append(domainDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Domains = domains

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update implements resource.Resource.
// All certificate attributes are ForceNew; this method satisfies the interface
// but will never be called in practice.
func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete implements resource.Resource.
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
		_, delErr := r.client.Certificates.Delete(ctx, cert.ID)
		if delErr == nil {
			break
		}
		if util.IsDigitalOceanError(delErr, http.StatusForbidden, "Make sure the certificate is not in use before deleting it") {
			log.Printf("[DEBUG] Received %s, retrying certificate deletion", delErr.Error())
			if time.Now().After(deadline) {
				resp.Diagnostics.AddError("Error deleting Certificate", delErr.Error())
				return
			}
			select {
			case <-ctx.Done():
				resp.Diagnostics.AddError("Context cancelled", ctx.Err().Error())
				return
			case <-time.After(1 * time.Second):
				continue
			}
		}
		resp.Diagnostics.AddError("Error deleting Certificate", delErr.Error())
		return
	}
}

// ImportState implements resource.ResourceWithImportState.
func (r *certificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// UpgradeState implements resource.ResourceWithUpgradeState.
//
// Schema version 0 did not have a uuid attribute; id held the DigitalOcean API
// certificate ID. Version 1 swaps the two: id becomes the certificate name
// (stable across renewals) and uuid stores the API certificate ID.
func (r *certificateResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Upgrader for V0 → current (V1). Each framework upgrader produces the
		// current schema's state directly — there is no chain.
		0: {
			PriorSchema: priorSchemaV0(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior certificateModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}
				log.Println("[DEBUG] Migrating certificate schema from v0 to v1.")

				current := certificateModel{
					// V1: id = certificate name (stable), uuid = API cert ID.
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
			},
		},
	}
}

// priorSchemaV0 returns the framework schema representation of the SDKv2 V0
// shape: no uuid attribute, id holds the DigitalOcean API certificate ID.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":                schema.StringAttribute{Computed: true},
			"name":              schema.StringAttribute{Required: true},
			"private_key":       schema.StringAttribute{Optional: true, Sensitive: true},
			"leaf_certificate":  schema.StringAttribute{Optional: true},
			"certificate_chain": schema.StringAttribute{Optional: true},
			"domains":           schema.SetAttribute{ElementType: types.StringType, Optional: true},
			"type":              schema.StringAttribute{Optional: true, Computed: true},
			"state":             schema.StringAttribute{Computed: true},
			"not_after":         schema.StringAttribute{Computed: true},
			"sha1_fingerprint":  schema.StringAttribute{Computed: true},
		},
	}
}

// readIntoState is a shared helper that populates a certificateModel from the
// API. It is called after Create's wait loop to refresh all computed fields.
func (r *certificateResource) readIntoState(
	ctx context.Context,
	id string,
	state *certificateModel,
	diagnostics *diag.Diagnostics,
) {
	cert, err := FindCertificateByName(r.client, id)
	if err != nil {
		diagnostics.AddError("Error retrieving Certificate", err.Error())
		return
	}
	state.Name = types.StringValue(cert.Name)
	state.UUID = types.StringValue(cert.ID)
	state.Type = types.StringValue(cert.Type)
	state.State = types.StringValue(cert.State)
	state.NotAfter = types.StringValue(cert.NotAfter)
	state.SHA1Fingerprint = types.StringValue(cert.SHA1Fingerprint)

	domains, domainDiags := flattenCertificateDomains(ctx, cert.DNSNames)
	diagnostics.Append(domainDiags...)
	if diagnostics.HasError() {
		return
	}
	state.Domains = domains
}

// flattenCertificateDomains converts a slice of domain strings to a types.Set.
func flattenCertificateDomains(_ context.Context, domains []string) (types.Set, diag.Diagnostics) {
	if len(domains) == 0 {
		return types.SetValueMust(types.StringType, []attr.Value{}), nil
	}
	elems := make([]attr.Value, 0, len(domains))
	for _, d := range domains {
		if d != "" {
			elems = append(elems, types.StringValue(d))
		}
	}
	return types.SetValue(types.StringType, elems)
}

// MigrateCertificateStateV0toV1 is the raw-map migration function originally
// used by SDKv2's StateUpgraders. It is kept as an exported function to allow
// unit tests to exercise the V0→V1 transformation logic without needing a live
// provider. The framework upgrade path (UpgradeState) calls equivalent typed
// logic via the framework's UpgradeStateRequest/Response.
func MigrateCertificateStateV0toV1(_ context.Context, rawState map[string]interface{}, _ interface{}) (map[string]interface{}, error) {
	if len(rawState) == 0 {
		log.Println("[DEBUG] Empty state; nothing to migrate.")
		return rawState, nil
	}
	log.Println("[DEBUG] Migrating certificate schema from v0 to v1.")

	// When the certificate type is lets_encrypt, the certificate
	// ID will change when it's renewed, so we have to rely on the
	// certificate name as the primary identifier instead.
	rawState["uuid"] = rawState["id"]
	rawState["id"] = rawState["name"]

	return rawState, nil
}

// waitForState polls refresh until the reported state is in target or an error
// occurs. It replaces terraform-plugin-sdk/v2/helper/retry.StateChangeConf.
func waitForState(
	ctx context.Context,
	refresh func() (any, string, error),
	pending, target []string,
	pollInterval, timeout time.Duration,
) (any, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		v, st, err := refresh()
		if err != nil {
			return v, err
		}
		if slices.Contains(target, st) {
			return v, nil
		}
		if !slices.Contains(pending, st) {
			return v, fmt.Errorf("unexpected state %q (pending=%v, target=%v)", st, pending, target)
		}
		if time.Now().After(deadline) {
			return v, fmt.Errorf("timeout after %s waiting for %v (last state=%q)", timeout, target, st)
		}
		select {
		case <-ctx.Done():
			return v, ctx.Err()
		case <-ticker.C:
		}
	}
}
