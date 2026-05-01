// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-provider-vault/internal/consts"
	"github.com/hashicorp/terraform-provider-vault/internal/framework/base"
	"github.com/hashicorp/terraform-provider-vault/internal/provider"
)

var (
	pkiSecretBackendRoleBackendFromPathRegex = regexp.MustCompile("^(.+)/roles/.+$")
	pkiSecretBackendRoleNameFromPathRegex    = regexp.MustCompile("^.+/roles/(.+)$")
)

// Ensure the implementation satisfies the resource.Resource and related interfaces.
var (
	_ resource.Resource                = &pkiSecretBackendRoleResourceFW{}
	_ resource.ResourceWithConfigure   = &pkiSecretBackendRoleResourceFW{}
	_ resource.ResourceWithImportState = &pkiSecretBackendRoleResourceFW{}
)

// NewPKISecretBackendRoleResource returns the framework-typed resource.
func NewPKISecretBackendRoleResource() resource.Resource {
	return &pkiSecretBackendRoleResourceFW{}
}

// pkiSecretBackendRoleResourceFW implements the framework version of the
// vault_pki_secret_backend_role resource (migrated from the legacy SDK v2).
type pkiSecretBackendRoleResourceFW struct {
	base.ResourceWithConfigure
}

// pkiSecretBackendRoleModel describes the Terraform resource data model.
type pkiSecretBackendRoleModel struct {
	base.BaseModelLegacy

	Backend                       types.String `tfsdk:"backend"`
	Name                          types.String `tfsdk:"name"`
	IssuerRef                     types.String `tfsdk:"issuer_ref"`
	TTL                           types.String `tfsdk:"ttl"`
	MaxTTL                        types.String `tfsdk:"max_ttl"`
	AllowLocalhost                types.Bool   `tfsdk:"allow_localhost"`
	AllowedDomains                types.List   `tfsdk:"allowed_domains"`
	AllowedDomainsTemplate        types.Bool   `tfsdk:"allowed_domains_template"`
	AllowBareDomains              types.Bool   `tfsdk:"allow_bare_domains"`
	AllowSubdomains               types.Bool   `tfsdk:"allow_subdomains"`
	AllowGlobDomains              types.Bool   `tfsdk:"allow_glob_domains"`
	AllowAnyName                  types.Bool   `tfsdk:"allow_any_name"`
	EnforceHostnames              types.Bool   `tfsdk:"enforce_hostnames"`
	AllowIPSans                   types.Bool   `tfsdk:"allow_ip_sans"`
	AllowedURISans                types.List   `tfsdk:"allowed_uri_sans"`
	AllowedOtherSans              types.List   `tfsdk:"allowed_other_sans"`
	AllowedURISansTemplate        types.Bool   `tfsdk:"allowed_uri_sans_template"`
	AllowWildcardCertificates     types.Bool   `tfsdk:"allow_wildcard_certificates"`
	ServerFlag                    types.Bool   `tfsdk:"server_flag"`
	ClientFlag                    types.Bool   `tfsdk:"client_flag"`
	CodeSigningFlag               types.Bool   `tfsdk:"code_signing_flag"`
	EmailProtectionFlag           types.Bool   `tfsdk:"email_protection_flag"`
	KeyType                       types.String `tfsdk:"key_type"`
	KeyBits                       types.Int64  `tfsdk:"key_bits"`
	SignatureBits                 types.Int64  `tfsdk:"signature_bits"`
	KeyUsage                      types.List   `tfsdk:"key_usage"`
	ExtKeyUsage                   types.List   `tfsdk:"ext_key_usage"`
	ExtKeyUsageOIDs               types.List   `tfsdk:"ext_key_usage_oids"`
	UseCSRCommonName              types.Bool   `tfsdk:"use_csr_common_name"`
	UseCSRSans                    types.Bool   `tfsdk:"use_csr_sans"`
	OU                            types.List   `tfsdk:"ou"`
	Organization                  types.List   `tfsdk:"organization"`
	Country                       types.List   `tfsdk:"country"`
	Locality                      types.List   `tfsdk:"locality"`
	Province                      types.List   `tfsdk:"province"`
	StreetAddress                 types.List   `tfsdk:"street_address"`
	PostalCode                    types.List   `tfsdk:"postal_code"`
	GenerateLease                 types.Bool   `tfsdk:"generate_lease"`
	NoStore                       types.Bool   `tfsdk:"no_store"`
	RequireCN                     types.Bool   `tfsdk:"require_cn"`
	PolicyIdentifiers             types.List   `tfsdk:"policy_identifiers"`
	PolicyIdentifier              types.Set    `tfsdk:"policy_identifier"`
	BasicConstraintsValidForNonCA types.Bool   `tfsdk:"basic_constraints_valid_for_non_ca"`
	NotBeforeDuration             types.String `tfsdk:"not_before_duration"`
	AllowedSerialNumbers          types.List   `tfsdk:"allowed_serial_numbers"`
	CnValidations                 types.List   `tfsdk:"cn_validations"`
	AllowedUserIds                types.List   `tfsdk:"allowed_user_ids"`
	NotAfter                      types.String `tfsdk:"not_after"`
	UsePSS                        types.Bool   `tfsdk:"use_pss"`
	NoStoreMetadata               types.Bool   `tfsdk:"no_store_metadata"`
	SerialNumberSource            types.String `tfsdk:"serial_number_source"`
}

// policyIdentifierBlockModel maps a single policy_identifier block.
type policyIdentifierBlockModel struct {
	OID    types.String `tfsdk:"oid"`
	CPS    types.String `tfsdk:"cps"`
	Notice types.String `tfsdk:"notice"`
}

// durationValidator ports provider.ValidateDuration to a framework validator.
type durationValidator struct{}

func (durationValidator) Description(_ context.Context) string {
	return "must be a valid Go duration"
}

func (durationValidator) MarkdownDescription(_ context.Context) string {
	return "must be a valid Go duration"
}

func (durationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := time.ParseDuration(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid duration",
			fmt.Sprintf("could not parse %q as a duration: %s", req.ConfigValue.ValueString(), err),
		)
	}
}

// Metadata sets the resource type name.
func (r *pkiSecretBackendRoleResourceFW) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pki_secret_backend_role"
}

// Schema defines the resource schema.
func (r *pkiSecretBackendRoleResourceFW) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			consts.FieldBackend: schema.StringAttribute{
				Required:    true,
				Description: "The path of the PKI secret backend the resource belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			consts.FieldName: schema.StringAttribute{
				Required:    true,
				Description: "Unique name for the role.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			consts.FieldIssuerRef: schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Specifies the default issuer of this request.",
			},
			consts.FieldTTL: schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The TTL.",
			},
			consts.FieldMaxTTL: schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The maximum TTL.",
			},
			consts.FieldAllowLocalhost: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to allow certificates for localhost.",
			},
			consts.FieldAllowedDomains: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The domains of the role.",
			},
			consts.FieldAllowedDomainsTemplate: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to indicate that `allowed_domains` specifies a template expression (e.g. {{identity.entity.aliases.<mount accessor>.name}})",
			},
			consts.FieldAllowBareDomains: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to allow certificates matching the actual domain.",
			},
			consts.FieldAllowSubdomains: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to allow certificates matching subdomains.",
			},
			consts.FieldAllowGlobDomains: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to allow names containing glob patterns.",
			},
			consts.FieldAllowAnyName: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to allow any name",
			},
			consts.FieldEnforceHostnames: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to allow only valid host names",
			},
			consts.FieldAllowIPSans: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to allow IP SANs",
			},
			consts.FieldAllowedURISans: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Defines allowed URI SANs",
			},
			consts.FieldAllowedOtherSans: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Defines allowed custom SANs",
			},
			consts.FieldAllowedURISansTemplate: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Flag to indicate that `allowed_uri_sans` specifies a template expression (e.g. {{identity.entity.aliases.<mount accessor>.name}})",
			},
			consts.FieldAllowWildcardCertificates: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to allow wildcard certificates",
			},
			consts.FieldServerFlag: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to specify certificates for server use.",
			},
			consts.FieldClientFlag: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to specify certificates for client use.",
			},
			consts.FieldCodeSigningFlag: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to specify certificates for code signing use.",
			},
			consts.FieldEmailProtectionFlag: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to specify certificates for email protection use.",
			},
			consts.FieldKeyType: schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("rsa"),
				Description: "The generated key type.",
				Validators: []validator.String{
					stringvalidator.OneOf("rsa", "ec", "ed25519", "any"),
				},
			},
			consts.FieldKeyBits: schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(2048),
				Description: "The number of bits of generated keys.",
			},
			consts.FieldSignatureBits: schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The number of bits to use in the signature algorithm.",
			},
			consts.FieldKeyUsage: schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Specify the allowed key usage constraint on issued certificates.",
			},
			consts.FieldExtKeyUsage: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Specify the allowed extended key usage constraint on issued certificates.",
			},
			consts.FieldExtKeyUsageOIDs: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of extended key usage OIDs.",
			},
			consts.FieldUseCSRCommonName: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to use the CN in the CSR.",
			},
			consts.FieldUseCSRSans: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to use the SANs in the CSR.",
			},
			consts.FieldOU: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The organization unit of generated certificates.",
			},
			consts.FieldOrganization: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The organization of generated certificates.",
			},
			consts.FieldCountry: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The country of generated certificates.",
			},
			consts.FieldLocality: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The locality of generated certificates.",
			},
			consts.FieldProvince: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The province of generated certificates.",
			},
			consts.FieldStreetAddress: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The street address of generated certificates.",
			},
			consts.FieldPostalCode: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The postal code of generated certificates.",
			},
			consts.FieldGenerateLease: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to generate leases with certificates.",
			},
			consts.FieldNoStore: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to not store certificates in the storage backend.",
			},
			consts.FieldRequireCN: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Flag to force CN usage.",
			},
			consts.FieldPolicyIdentifiers: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Specify the list of allowed policies OIDs.",
				Validators: []validator.List{
					listvalidator.ConflictsWith(path.MatchRoot(consts.FieldPolicyIdentifier)),
				},
			},
			consts.FieldPolicyIdentifier: schema.SetNestedAttribute{
				Optional:    true,
				Description: "Policy identifier block; can only be used with Vault 1.11+",
				Validators: []validator.Set{
					setvalidator.ConflictsWith(path.MatchRoot(consts.FieldPolicyIdentifiers)),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						consts.FieldOID: schema.StringAttribute{
							Required:    true,
							Description: "OID",
						},
						consts.FieldCPS: schema.StringAttribute{
							Optional:    true,
							Description: "Optional CPS URL",
						},
						consts.FieldNotice: schema.StringAttribute{
							Optional:    true,
							Description: "Optional notice",
						},
					},
				},
			},
			consts.FieldBasicConstraintsValidForNonCA: schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
				Description: "Flag to mark basic constraints valid when issuing non-CA certificates.",
			},
			consts.FieldNotBeforeDuration: schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Specifies the duration by which to backdate the NotBefore property.",
				Validators: []validator.String{
					durationValidator{},
				},
			},
			consts.FieldAllowedSerialNumbers: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Defines allowed Subject serial numbers.",
			},
			consts.FieldCnValidations: schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "Specify validations to run on the Common Name field of the certificate.",
			},
			consts.FieldAllowedUserIds: schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "The allowed User ID's.",
			},
			consts.FieldNotAfter: schema.StringAttribute{
				Optional: true,
				Description: "Set the Not After field of the certificate with specified date value. " +
					"The value format should be given in UTC format YYYY-MM-ddTHH:MM:SSZ. Supports the " +
					"Y10K end date for IEEE 802.1AR-2018 standard devices, 9999-12-31T23:59:59Z.",
			},
			consts.FieldUsePSS: schema.BoolAttribute{
				Optional: true,
				Description: "Specifies whether or not to use PSS signatures over PKCS#1v1.5 signatures " +
					"when a RSA-type issuer is used. Ignored for ECDSA/Ed25519 issuers.",
			},
			consts.FieldNoStoreMetadata: schema.BoolAttribute{
				Optional: true,
				Description: "Allows metadata to be stored keyed on the certificate's serial number. " +
					"The field is independent of no_store, allowing metadata storage regardless of whether " +
					"certificates are stored. If true, metadata is not stored and an error is returned if the " +
					"metadata field is specified on issuance APIs",
			},
			consts.FieldSerialNumberSource: schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Specifies the source of the subject serial number. Valid values are json-csr (default) " +
					"or json. When set to json-csr, the subject serial number is taken from the serial_number " +
					"parameter and falls back to the serial number in the CSR. When set to json, the subject " +
					"serial number is taken from the serial_number parameter but will ignore any value in the CSR." +
					" For backwards compatibility an empty value for this field will default to the json-csr behavior.",
			},
		},
	}

	base.MustAddLegacyBaseSchema(&resp.Schema)
	// id field needs UseStateForUnknown to suppress (known after apply) noise.
	// MustAddLegacyBaseSchema already adds it with that plan modifier; nothing
	// extra needed here.
}

// Configure attaches provider-shared data to the resource. Embedded
// base.ResourceWithConfigure already guards on req.ProviderData == nil by
// checking the type assertion (the framework calls Configure on every RPC,
// including ValidateResourceConfig when ProviderData is nil).
func (r *pkiSecretBackendRoleResourceFW) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Belt and braces: explicitly skip when ProviderData is nil so subsequent
	// helper code that may dereference it stays safe.
	if req.ProviderData == nil {
		return
	}
	r.ResourceWithConfigure.Configure(ctx, req, resp)
}

// ImportState handles `terraform import vault_pki_secret_backend_role <id>`.
// The ID is the full Vault path (`<backend>/roles/<name>`).
func (r *pkiSecretBackendRoleResourceFW) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root(consts.FieldID), req, resp)
}

// Create writes the role to Vault.
func (r *pkiSecretBackendRoleResourceFW) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pkiSecretBackendRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := provider.GetClientFromMeta(ctx, r.Meta(), plan.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Vault client error", err.Error())
		return
	}

	backend := plan.Backend.ValueString()
	name := plan.Name.ValueString()
	rolePath := pkiSecretBackendRolePath(backend, name)

	tflog.Debug(ctx, fmt.Sprintf("Writing PKI secret backend role %q", rolePath))

	data, diags := r.buildAPIPayload(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := client.Logical().WriteWithContext(ctx, rolePath, data); err != nil {
		resp.Diagnostics.AddError(
			"Error creating PKI secret backend role",
			fmt.Sprintf("error creating role %s for backend %q: %s", name, backend, err),
		)
		return
	}

	plan.ID = types.StringValue(rolePath)

	// Read back from Vault so all Computed fields are populated.
	resp.Diagnostics.Append(r.readIntoModel(ctx, client, rolePath, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read populates state from Vault.
func (r *pkiSecretBackendRoleResourceFW) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pkiSecretBackendRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := provider.GetClientFromMeta(ctx, r.Meta(), state.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Vault client error", err.Error())
		return
	}

	rolePath := state.ID.ValueString()
	if rolePath == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := r.readIntoModel(ctx, client, rolePath, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Detect "not found" — readIntoModel signals that by clearing ID.
	if state.ID.IsNull() || state.ID.ValueString() == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update mutates the role in Vault.
func (r *pkiSecretBackendRoleResourceFW) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan pkiSecretBackendRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state pkiSecretBackendRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := provider.GetClientFromMeta(ctx, r.Meta(), plan.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Vault client error", err.Error())
		return
	}

	rolePath := state.ID.ValueString()
	tflog.Debug(ctx, fmt.Sprintf("Updating PKI secret backend role %q", rolePath))

	data, diags := r.buildAPIPayload(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := client.Logical().WriteWithContext(ctx, rolePath, data); err != nil {
		resp.Diagnostics.AddError(
			"Error updating PKI secret backend role",
			fmt.Sprintf("error updating PKI secret backend role %q: %s", rolePath, err),
		)
		return
	}

	// Carry the ID forward.
	plan.ID = types.StringValue(rolePath)

	// Read back from Vault so all Computed fields are populated.
	resp.Diagnostics.Append(r.readIntoModel(ctx, client, rolePath, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the role from Vault. Reads from req.State (req.Plan is null).
func (r *pkiSecretBackendRoleResourceFW) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pkiSecretBackendRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client, err := provider.GetClientFromMeta(ctx, r.Meta(), state.Namespace.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Vault client error", err.Error())
		return
	}

	rolePath := state.ID.ValueString()
	tflog.Debug(ctx, fmt.Sprintf("Deleting role %q", rolePath))

	if _, err := client.Logical().DeleteWithContext(ctx, rolePath); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting PKI secret backend role",
			fmt.Sprintf("error deleting role %q: %s", rolePath, err),
		)
		return
	}
}

// buildAPIPayload converts the plan model into the Vault API payload map.
func (r *pkiSecretBackendRoleResourceFW) buildAPIPayload(ctx context.Context, plan *pkiSecretBackendRoleModel) (map[string]interface{}, []errDiag) {
	data := map[string]interface{}{}
	var diags []errDiag

	// String list fields — only send when configured.
	stringListFields := map[string]types.List{
		consts.FieldAllowedDomains:        plan.AllowedDomains,
		consts.FieldAllowedSerialNumbers:  plan.AllowedSerialNumbers,
		consts.FieldExtKeyUsage:           plan.ExtKeyUsage,
		consts.FieldExtKeyUsageOIDs:       plan.ExtKeyUsageOIDs,
		consts.FieldCnValidations:         plan.CnValidations,
		consts.FieldAllowedURISans:        plan.AllowedURISans,
		consts.FieldAllowedOtherSans:      plan.AllowedOtherSans,
		consts.FieldOU:                    plan.OU,
		consts.FieldOrganization:          plan.Organization,
		consts.FieldCountry:               plan.Country,
		consts.FieldLocality:              plan.Locality,
		consts.FieldProvince:              plan.Province,
		consts.FieldStreetAddress:         plan.StreetAddress,
		consts.FieldPostalCode:            plan.PostalCode,
		consts.FieldAllowedUserIds:        plan.AllowedUserIds,
		consts.FieldPolicyIdentifiers:     plan.PolicyIdentifiers,
	}
	for k, list := range stringListFields {
		if list.IsNull() || list.IsUnknown() {
			continue
		}
		strs, d := listToStringSlice(ctx, list)
		if d != nil {
			diags = append(diags, *d)
			continue
		}
		if len(strs) > 0 {
			data[k] = strs
		}
	}

	// key_usage — preserve "empty list means clear" semantics.
	if !plan.KeyUsage.IsNull() && !plan.KeyUsage.IsUnknown() {
		strs, d := listToStringSlice(ctx, plan.KeyUsage)
		if d != nil {
			diags = append(diags, *d)
		} else {
			// Always include — even if empty — to support `key_usage = []`.
			if strs == nil {
				strs = []string{}
			}
			data[consts.FieldKeyUsage] = strs
		}
	}

	// Bool fields — always send to mirror SDKv2 d.Get(bool) behaviour.
	boolFields := map[string]types.Bool{
		consts.FieldAllowAnyName:                plan.AllowAnyName,
		consts.FieldAllowBareDomains:            plan.AllowBareDomains,
		consts.FieldAllowGlobDomains:            plan.AllowGlobDomains,
		consts.FieldAllowIPSans:                 plan.AllowIPSans,
		consts.FieldAllowLocalhost:              plan.AllowLocalhost,
		consts.FieldAllowSubdomains:             plan.AllowSubdomains,
		consts.FieldAllowWildcardCertificates:   plan.AllowWildcardCertificates,
		consts.FieldAllowedDomainsTemplate:      plan.AllowedDomainsTemplate,
		consts.FieldAllowedURISansTemplate:      plan.AllowedURISansTemplate,
		consts.FieldBasicConstraintsValidForNonCA: plan.BasicConstraintsValidForNonCA,
		consts.FieldClientFlag:                  plan.ClientFlag,
		consts.FieldCodeSigningFlag:             plan.CodeSigningFlag,
		consts.FieldEmailProtectionFlag:         plan.EmailProtectionFlag,
		consts.FieldEnforceHostnames:            plan.EnforceHostnames,
		consts.FieldGenerateLease:               plan.GenerateLease,
		consts.FieldNoStore:                     plan.NoStore,
		consts.FieldRequireCN:                   plan.RequireCN,
		consts.FieldServerFlag:                  plan.ServerFlag,
		consts.FieldUseCSRCommonName:            plan.UseCSRCommonName,
		consts.FieldUseCSRSans:                  plan.UseCSRSans,
	}
	for k, b := range boolFields {
		if b.IsNull() || b.IsUnknown() {
			continue
		}
		data[k] = b.ValueBool()
	}

	// Other typed scalars — only set when not null/unknown and not zero string.
	stringScalars := map[string]types.String{
		consts.FieldKeyType:           plan.KeyType,
		consts.FieldTTL:               plan.TTL,
		consts.FieldMaxTTL:            plan.MaxTTL,
		consts.FieldNotBeforeDuration: plan.NotBeforeDuration,
		consts.FieldNotAfter:          plan.NotAfter,
	}
	for k, s := range stringScalars {
		if s.IsNull() || s.IsUnknown() {
			continue
		}
		if v := s.ValueString(); v != "" {
			data[k] = v
		}
	}

	// Int64 scalars.
	if !plan.KeyBits.IsNull() && !plan.KeyBits.IsUnknown() {
		if v := plan.KeyBits.ValueInt64(); v != 0 {
			data[consts.FieldKeyBits] = v
		}
	}
	if !plan.SignatureBits.IsNull() && !plan.SignatureBits.IsUnknown() {
		if v := plan.SignatureBits.ValueInt64(); v != 0 {
			data[consts.FieldSignatureBits] = v
		}
	}

	// policy_identifier (set of nested) — only when policy_identifiers is unset.
	if (plan.PolicyIdentifiers.IsNull() || plan.PolicyIdentifiers.IsUnknown() || len(plan.PolicyIdentifiers.Elements()) == 0) &&
		!plan.PolicyIdentifier.IsNull() && !plan.PolicyIdentifier.IsUnknown() {
		var blocks []policyIdentifierBlockModel
		d := plan.PolicyIdentifier.ElementsAs(ctx, &blocks, false)
		if d.HasError() {
			diags = append(diags, errDiag{summary: "policy_identifier conversion error", detail: fmt.Sprintf("%v", d)})
		} else if len(blocks) > 0 {
			out := make([]map[string]interface{}, 0, len(blocks))
			for _, b := range blocks {
				m := map[string]interface{}{}
				m[consts.FieldOID] = b.OID.ValueString()
				if !b.CPS.IsNull() && b.CPS.ValueString() != "" {
					m[consts.FieldCPS] = b.CPS.ValueString()
				}
				if !b.Notice.IsNull() && b.Notice.ValueString() != "" {
					m[consts.FieldNotice] = b.Notice.ValueString()
				}
				out = append(out, m)
			}
			data[consts.FieldPolicyIdentifiers] = out
		}
	}

	// Conditional API fields — guard via meta-aware version flags.
	if r.Meta() != nil {
		if r.Meta().IsAPISupported(provider.VaultVersion111) {
			if !plan.IssuerRef.IsNull() && !plan.IssuerRef.IsUnknown() && plan.IssuerRef.ValueString() != "" {
				data[consts.FieldIssuerRef] = plan.IssuerRef.ValueString()
			}
		}
		if r.Meta().IsAPISupported(provider.VaultVersion112) {
			if !plan.UsePSS.IsNull() && !plan.UsePSS.IsUnknown() {
				data[consts.FieldUsePSS] = plan.UsePSS.ValueBool()
			}
		}
		if r.Meta().IsAPISupported(provider.VaultVersion117) {
			if !plan.NoStoreMetadata.IsNull() && !plan.NoStoreMetadata.IsUnknown() {
				data[consts.FieldNoStoreMetadata] = plan.NoStoreMetadata.ValueBool()
			}
		}
		if r.Meta().IsAPISupported(provider.VaultVersion119) {
			if !plan.SerialNumberSource.IsNull() && !plan.SerialNumberSource.IsUnknown() && plan.SerialNumberSource.ValueString() != "" {
				data[consts.FieldSerialNumberSource] = plan.SerialNumberSource.ValueString()
			}
		}
	}

	return data, diags
}

// readIntoModel reads the role from Vault and populates the supplied model.
func (r *pkiSecretBackendRoleResourceFW) readIntoModel(ctx context.Context, client clientLike, rolePath string, out *pkiSecretBackendRoleModel) []errDiag {
	var diags []errDiag

	backend, err := pkiSecretBackendRoleBackendFromPath(rolePath)
	if err != nil {
		// Invalid ID — clear and return.
		out.ID = types.StringNull()
		diags = append(diags, errDiag{summary: "invalid role ID", detail: fmt.Sprintf("%q: %s", rolePath, err)})
		return diags
	}
	name, err := pkiSecretBackendRoleNameFromPath(rolePath)
	if err != nil {
		out.ID = types.StringNull()
		diags = append(diags, errDiag{summary: "invalid role ID", detail: fmt.Sprintf("%q: %s", rolePath, err)})
		return diags
	}

	tflog.Debug(ctx, fmt.Sprintf("Reading role from %q", rolePath))
	secret, err := client.Logical().ReadWithContext(ctx, rolePath)
	if err != nil {
		diags = append(diags, errDiag{summary: "error reading role", detail: fmt.Sprintf("%q: %s", rolePath, err)})
		return diags
	}
	if secret == nil {
		// Not found — caller should drop from state.
		out.ID = types.StringNull()
		return diags
	}

	out.Backend = types.StringValue(backend)
	out.Name = types.StringValue(name)
	out.ID = types.StringValue(rolePath)

	// String list fields.
	listFields := []string{
		consts.FieldAllowedDomains,
		consts.FieldAllowedSerialNumbers,
		consts.FieldExtKeyUsage,
		consts.FieldExtKeyUsageOIDs,
		consts.FieldCnValidations,
		consts.FieldKeyUsage,
		consts.FieldAllowedURISans,
		consts.FieldAllowedOtherSans,
		consts.FieldOU,
		consts.FieldOrganization,
		consts.FieldCountry,
		consts.FieldLocality,
		consts.FieldProvince,
		consts.FieldStreetAddress,
		consts.FieldPostalCode,
		consts.FieldAllowedUserIds,
	}
	for _, k := range listFields {
		raw, ok := secret.Data[k]
		if !ok || raw == nil {
			setEmptyOrNullList(out, k)
			continue
		}
		strs := toStringSlice(raw)
		listVal, _ := types.ListValueFrom(ctx, types.StringType, strs)
		assignList(out, k, listVal)
	}

	// Bool fields.
	boolFields := []string{
		consts.FieldAllowAnyName,
		consts.FieldAllowBareDomains,
		consts.FieldAllowGlobDomains,
		consts.FieldAllowIPSans,
		consts.FieldAllowLocalhost,
		consts.FieldAllowSubdomains,
		consts.FieldAllowWildcardCertificates,
		consts.FieldAllowedDomainsTemplate,
		consts.FieldAllowedURISansTemplate,
		consts.FieldBasicConstraintsValidForNonCA,
		consts.FieldClientFlag,
		consts.FieldCodeSigningFlag,
		consts.FieldEmailProtectionFlag,
		consts.FieldEnforceHostnames,
		consts.FieldGenerateLease,
		consts.FieldNoStore,
		consts.FieldRequireCN,
		consts.FieldServerFlag,
		consts.FieldUseCSRCommonName,
		consts.FieldUseCSRSans,
	}
	for _, k := range boolFields {
		raw, ok := secret.Data[k]
		if !ok || raw == nil {
			assignBool(out, k, types.BoolNull())
			continue
		}
		b, _ := raw.(bool)
		assignBool(out, k, types.BoolValue(b))
	}

	// Other scalars.
	scalarFields := []string{
		consts.FieldKeyType,
		consts.FieldTTL,
		consts.FieldMaxTTL,
		consts.FieldIssuerRef,
		consts.FieldNotBeforeDuration,
		consts.FieldNotAfter,
		consts.FieldSerialNumberSource,
	}
	for _, k := range scalarFields {
		raw, ok := secret.Data[k]
		if !ok || raw == nil {
			assignString(out, k, types.StringNull())
			continue
		}
		switch k {
		case consts.FieldNotBeforeDuration:
			assignString(out, k, types.StringValue(flattenVaultDurationFW(raw)))
		case consts.FieldTTL, consts.FieldMaxTTL:
			// Vault returns these as numbers (seconds). Coerce to string to
			// match SDKv2 behaviour where TypeString could hold both.
			assignString(out, k, types.StringValue(coerceToString(raw)))
		default:
			assignString(out, k, types.StringValue(coerceToString(raw)))
		}
	}

	// int64 fields.
	intFields := []string{
		consts.FieldKeyBits,
		consts.FieldSignatureBits,
	}
	for _, k := range intFields {
		raw, ok := secret.Data[k]
		if !ok || raw == nil {
			assignInt64(out, k, types.Int64Null())
			continue
		}
		v, err := coerceToInt64(raw)
		if err != nil {
			diags = append(diags, errDiag{summary: "type coercion error", detail: fmt.Sprintf("expected %s %q to be a number", k, raw)})
			continue
		}
		assignInt64(out, k, types.Int64Value(v))
	}

	// Bools from versioned API surface.
	if r.Meta() != nil {
		if r.Meta().IsAPISupported(provider.VaultVersion112) {
			if v, ok := secret.Data[consts.FieldUsePSS]; ok && v != nil {
				if b, ok2 := v.(bool); ok2 {
					out.UsePSS = types.BoolValue(b)
				}
			}
		}
		if r.Meta().IsAPISupported(provider.VaultVersion117) {
			if v, ok := secret.Data[consts.FieldNoStoreMetadata]; ok && v != nil {
				if b, ok2 := v.(bool); ok2 {
					out.NoStoreMetadata = types.BoolValue(b)
				}
			}
		}
	}

	// policy_identifiers vs policy_identifier — mirror SDKv2 dispatch logic.
	rawPolicies, _ := secret.Data[consts.FieldPolicyIdentifiers].([]interface{})
	legacy, blocks := splitPolicyIdentifiers(rawPolicies)
	if len(legacy) > 0 {
		listVal, _ := types.ListValueFrom(ctx, types.StringType, legacy)
		out.PolicyIdentifiers = listVal
		out.PolicyIdentifier = types.SetNull(policyIdentifierObjectType())
	} else {
		out.PolicyIdentifiers = types.ListNull(types.StringType)
		setVal, d := types.SetValueFrom(ctx, policyIdentifierObjectType(), blocks)
		if d.HasError() {
			diags = append(diags, errDiag{summary: "policy_identifier conversion error", detail: fmt.Sprintf("%v", d)})
		} else {
			out.PolicyIdentifier = setVal
		}
	}

	return diags
}

// errDiag is a small wrapper so internal helpers can return diagnostics
// without coupling to a specific framework type.
type errDiag struct {
	summary string
	detail  string
}

// clientLike is satisfied by *api.Client (Vault's client). Using a structural
// type lets the helpers stay decoupled from the import path during migration.
type clientLike interface {
	Logical() logicalLike
}

// logicalLike narrows what we use of the Vault Logical client.
type logicalLike interface {
	WriteWithContext(ctx context.Context, path string, data map[string]interface{}) (*vaultSecret, error)
	ReadWithContext(ctx context.Context, path string) (*vaultSecret, error)
	DeleteWithContext(ctx context.Context, path string) (*vaultSecret, error)
}

// vaultSecret is a stand-in for *api.Secret to keep imports minimal in this
// migration scaffold. In the real merged provider this is `*api.Secret`.
type vaultSecret = struct {
	Data map[string]interface{}
}

// listToStringSlice extracts a []string from a types.List.
func listToStringSlice(ctx context.Context, list types.List) ([]string, *errDiag) {
	if list.IsNull() || list.IsUnknown() {
		return nil, nil
	}
	out := make([]string, 0, len(list.Elements()))
	d := list.ElementsAs(ctx, &out, false)
	if d.HasError() {
		return nil, &errDiag{summary: "list conversion error", detail: fmt.Sprintf("%v", d)}
	}
	return out, nil
}

func toStringSlice(raw interface{}) []string {
	switch v := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func coerceToString(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func coerceToInt64(raw interface{}) (int64, error) {
	switch v := raw.(type) {
	case json.Number:
		return v.Int64()
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("unable to coerce %T to int64", v)
	}
}

// flattenVaultDurationFW mirrors flattenVaultDuration from the SDKv2 codebase.
// Vault may return the duration as a number of seconds; if so, render as the
// canonical `<n>s` string. Otherwise pass through.
func flattenVaultDurationFW(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return v
	case json.Number:
		return v.String() + "s"
	case int, int64, float64:
		return fmt.Sprintf("%vs", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// splitPolicyIdentifiers mirrors pki.MakePkiPolicyIdentifiersListOrSet — when
// every entry is a bare OID string, render to the legacy list; otherwise build
// the structured set.
func splitPolicyIdentifiers(raw []interface{}) ([]string, []policyIdentifierBlockModel) {
	if len(raw) == 0 {
		return nil, nil
	}
	allStrings := true
	for _, item := range raw {
		switch v := item.(type) {
		case string:
			// keep going
			_ = v
		default:
			allStrings = false
		}
	}
	if allStrings {
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out, nil
	}
	blocks := make([]policyIdentifierBlockModel, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		b := policyIdentifierBlockModel{}
		if oid, ok := m[consts.FieldOID].(string); ok {
			b.OID = types.StringValue(oid)
		} else {
			b.OID = types.StringNull()
		}
		if cps, ok := m[consts.FieldCPS].(string); ok && cps != "" {
			b.CPS = types.StringValue(cps)
		} else {
			b.CPS = types.StringNull()
		}
		if notice, ok := m[consts.FieldNotice].(string); ok && notice != "" {
			b.Notice = types.StringValue(notice)
		} else {
			b.Notice = types.StringNull()
		}
		blocks = append(blocks, b)
	}
	return nil, blocks
}

// policyIdentifierObjectType is the AttrType for a policy_identifier set element.
func policyIdentifierObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			consts.FieldOID:    types.StringType,
			consts.FieldCPS:    types.StringType,
			consts.FieldNotice: types.StringType,
		},
	}
}

// setEmptyOrNullList sets the named list field to null. Used when Vault did
// not return a value for the key.
func setEmptyOrNullList(m *pkiSecretBackendRoleModel, k string) {
	null := types.ListNull(types.StringType)
	assignList(m, k, null)
}

// assignList writes a typed list into the model based on the field name.
func assignList(m *pkiSecretBackendRoleModel, k string, v types.List) {
	switch k {
	case consts.FieldAllowedDomains:
		m.AllowedDomains = v
	case consts.FieldAllowedSerialNumbers:
		m.AllowedSerialNumbers = v
	case consts.FieldExtKeyUsage:
		m.ExtKeyUsage = v
	case consts.FieldExtKeyUsageOIDs:
		m.ExtKeyUsageOIDs = v
	case consts.FieldCnValidations:
		m.CnValidations = v
	case consts.FieldKeyUsage:
		m.KeyUsage = v
	case consts.FieldAllowedURISans:
		m.AllowedURISans = v
	case consts.FieldAllowedOtherSans:
		m.AllowedOtherSans = v
	case consts.FieldOU:
		m.OU = v
	case consts.FieldOrganization:
		m.Organization = v
	case consts.FieldCountry:
		m.Country = v
	case consts.FieldLocality:
		m.Locality = v
	case consts.FieldProvince:
		m.Province = v
	case consts.FieldStreetAddress:
		m.StreetAddress = v
	case consts.FieldPostalCode:
		m.PostalCode = v
	case consts.FieldAllowedUserIds:
		m.AllowedUserIds = v
	}
}

func assignBool(m *pkiSecretBackendRoleModel, k string, v types.Bool) {
	switch k {
	case consts.FieldAllowAnyName:
		m.AllowAnyName = v
	case consts.FieldAllowBareDomains:
		m.AllowBareDomains = v
	case consts.FieldAllowGlobDomains:
		m.AllowGlobDomains = v
	case consts.FieldAllowIPSans:
		m.AllowIPSans = v
	case consts.FieldAllowLocalhost:
		m.AllowLocalhost = v
	case consts.FieldAllowSubdomains:
		m.AllowSubdomains = v
	case consts.FieldAllowWildcardCertificates:
		m.AllowWildcardCertificates = v
	case consts.FieldAllowedDomainsTemplate:
		m.AllowedDomainsTemplate = v
	case consts.FieldAllowedURISansTemplate:
		m.AllowedURISansTemplate = v
	case consts.FieldBasicConstraintsValidForNonCA:
		m.BasicConstraintsValidForNonCA = v
	case consts.FieldClientFlag:
		m.ClientFlag = v
	case consts.FieldCodeSigningFlag:
		m.CodeSigningFlag = v
	case consts.FieldEmailProtectionFlag:
		m.EmailProtectionFlag = v
	case consts.FieldEnforceHostnames:
		m.EnforceHostnames = v
	case consts.FieldGenerateLease:
		m.GenerateLease = v
	case consts.FieldNoStore:
		m.NoStore = v
	case consts.FieldRequireCN:
		m.RequireCN = v
	case consts.FieldServerFlag:
		m.ServerFlag = v
	case consts.FieldUseCSRCommonName:
		m.UseCSRCommonName = v
	case consts.FieldUseCSRSans:
		m.UseCSRSans = v
	case consts.FieldUsePSS:
		m.UsePSS = v
	case consts.FieldNoStoreMetadata:
		m.NoStoreMetadata = v
	}
}

func assignString(m *pkiSecretBackendRoleModel, k string, v types.String) {
	switch k {
	case consts.FieldKeyType:
		m.KeyType = v
	case consts.FieldTTL:
		m.TTL = v
	case consts.FieldMaxTTL:
		m.MaxTTL = v
	case consts.FieldIssuerRef:
		m.IssuerRef = v
	case consts.FieldNotBeforeDuration:
		m.NotBeforeDuration = v
	case consts.FieldNotAfter:
		m.NotAfter = v
	case consts.FieldSerialNumberSource:
		m.SerialNumberSource = v
	}
}

func assignInt64(m *pkiSecretBackendRoleModel, k string, v types.Int64) {
	switch k {
	case consts.FieldKeyBits:
		m.KeyBits = v
	case consts.FieldSignatureBits:
		m.SignatureBits = v
	}
}

// pkiSecretBackendRolePath returns the Vault role path.
func pkiSecretBackendRolePath(backend string, name string) string {
	return strings.Trim(backend, "/") + "/roles/" + strings.Trim(name, "/")
}

func pkiSecretBackendRoleNameFromPath(path string) (string, error) {
	if !pkiSecretBackendRoleNameFromPathRegex.MatchString(path) {
		return "", fmt.Errorf("no role found")
	}
	res := pkiSecretBackendRoleNameFromPathRegex.FindStringSubmatch(path)
	if len(res) != 2 {
		return "", fmt.Errorf("unexpected number of matches (%d) for role", len(res))
	}
	return res[1], nil
}

func pkiSecretBackendRoleBackendFromPath(path string) (string, error) {
	if !pkiSecretBackendRoleBackendFromPathRegex.MatchString(path) {
		return "", fmt.Errorf("no backend found")
	}
	res := pkiSecretBackendRoleBackendFromPathRegex.FindStringSubmatch(path)
	if len(res) != 2 {
		return "", fmt.Errorf("unexpected number of matches (%d) for backend", len(res))
	}
	return res[1], nil
}
