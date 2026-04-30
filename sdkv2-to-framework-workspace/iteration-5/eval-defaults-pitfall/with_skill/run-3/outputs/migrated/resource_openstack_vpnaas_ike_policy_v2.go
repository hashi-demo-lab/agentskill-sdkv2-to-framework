package openstack

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// NewIKEPolicyV2Resource returns the framework resource implementation.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

// ikePolicyV2Model is the typed model for openstack_vpnaas_ike_policy_v2.
type ikePolicyV2Model struct {
	ID                    types.String   `tfsdk:"id"`
	Region                types.String   `tfsdk:"region"`
	Name                  types.String   `tfsdk:"name"`
	Description           types.String   `tfsdk:"description"`
	AuthAlgorithm         types.String   `tfsdk:"auth_algorithm"`
	EncryptionAlgorithm   types.String   `tfsdk:"encryption_algorithm"`
	PFS                   types.String   `tfsdk:"pfs"`
	Phase1NegotiationMode types.String   `tfsdk:"phase1_negotiation_mode"`
	IKEVersion            types.String   `tfsdk:"ike_version"`
	Lifetime              types.Set      `tfsdk:"lifetime"`
	TenantID              types.String   `tfsdk:"tenant_id"`
	ValueSpecs            types.Map      `tfsdk:"value_specs"`
	Timeouts              timeouts.Value `tfsdk:"timeouts"`
}

// lifetimeAttrTypes mirrors the lifetime block schema.
func lifetimeAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"units": types.StringType,
		"value": types.Int64Type,
	}
}

func (r *ikePolicyV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpnaas_ike_policy_v2"
}

func (r *ikePolicyV2Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// auth_algorithm: SDKv2 Default "sha1" → framework stringdefault.StaticString.
			// Default attributes must be Computed: true (framework requirement).
			"auth_algorithm": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("sha1"),
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(ikepolicies.AuthAlgorithmSHA1),
						string(ikepolicies.AuthAlgorithmSHA256),
						string(ikepolicies.AuthAlgorithmSHA384),
						string(ikepolicies.AuthAlgorithmSHA512),
						string(ikepolicies.AuthAlgorithmAESXCBC),
						string(ikepolicies.AuthAlgorithmAESCMAC),
					),
				},
			},
			// encryption_algorithm: SDKv2 Default "aes-128" → stringdefault.StaticString.
			"encryption_algorithm": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("aes-128"),
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(ikepolicies.EncryptionAlgorithm3DES),
						string(ikepolicies.EncryptionAlgorithmAES128),
						string(ikepolicies.EncryptionAlgorithmAES256),
						string(ikepolicies.EncryptionAlgorithmAES192),
						string(ikepolicies.EncryptionAlgorithmAES128CTR),
						string(ikepolicies.EncryptionAlgorithmAES192CTR),
						string(ikepolicies.EncryptionAlgorithmAES256CTR),
						string(ikepolicies.EncryptionAlgorithmAES128CCM8),
						string(ikepolicies.EncryptionAlgorithmAES192CCM8),
						string(ikepolicies.EncryptionAlgorithmAES256CCM8),
						string(ikepolicies.EncryptionAlgorithmAES128CCM12),
						string(ikepolicies.EncryptionAlgorithmAES192CCM12),
						string(ikepolicies.EncryptionAlgorithmAES256CCM12),
						string(ikepolicies.EncryptionAlgorithmAES128CCM16),
						string(ikepolicies.EncryptionAlgorithmAES192CCM16),
						string(ikepolicies.EncryptionAlgorithmAES256CCM16),
						string(ikepolicies.EncryptionAlgorithmAES128GCM8),
						string(ikepolicies.EncryptionAlgorithmAES192GCM8),
						string(ikepolicies.EncryptionAlgorithmAES256GCM8),
						string(ikepolicies.EncryptionAlgorithmAES128GCM12),
						string(ikepolicies.EncryptionAlgorithmAES192GCM12),
						string(ikepolicies.EncryptionAlgorithmAES256GCM12),
						string(ikepolicies.EncryptionAlgorithmAES128GCM16),
						string(ikepolicies.EncryptionAlgorithmAES192GCM16),
						string(ikepolicies.EncryptionAlgorithmAES256GCM16),
					),
				},
			},
			// pfs: SDKv2 Default "group5" → stringdefault.StaticString.
			"pfs": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("group5"),
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(ikepolicies.PFSGroup2),
						string(ikepolicies.PFSGroup5),
						string(ikepolicies.PFSGroup14),
						string(ikepolicies.PFSGroup15),
						string(ikepolicies.PFSGroup16),
						string(ikepolicies.PFSGroup17),
						string(ikepolicies.PFSGroup18),
						string(ikepolicies.PFSGroup19),
						string(ikepolicies.PFSGroup20),
						string(ikepolicies.PFSGroup21),
						string(ikepolicies.PFSGroup22),
						string(ikepolicies.PFSGroup23),
						string(ikepolicies.PFSGroup24),
						string(ikepolicies.PFSGroup25),
						string(ikepolicies.PFSGroup26),
						string(ikepolicies.PFSGroup27),
						string(ikepolicies.PFSGroup28),
						string(ikepolicies.PFSGroup29),
						string(ikepolicies.PFSGroup30),
						string(ikepolicies.PFSGroup31),
					),
				},
			},
			// phase1_negotiation_mode: SDKv2 Default "main" → stringdefault.StaticString.
			"phase1_negotiation_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("main"),
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(ikepolicies.Phase1NegotiationModeMain),
					),
				},
			},
			// ike_version: SDKv2 Default "v1" → stringdefault.StaticString.
			"ike_version": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("v1"),
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(ikepolicies.IKEVersionv1),
						string(ikepolicies.IKEVersionv2),
					),
				},
			},
			"tenant_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"value_specs": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					// value_specs was ForceNew in SDKv2.
					mapRequiresReplace{},
				},
			},
		},
		Blocks: map[string]schema.Block{
			// lifetime stays as a (set) block to preserve practitioner HCL.
			// SDKv2 had Type: TypeSet, Computed: true, Optional: true with a
			// nested Resource. Blocks can't be Computed in the framework, so
			// computed-equivalent behaviour is achieved by setting the
			// attribute from the API in Read/Create/Update.
			"lifetime": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"units": schema.StringAttribute{
							Optional: true,
							Computed: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"value": schema.Int64Attribute{
							Optional: true,
							Computed: true,
						},
					},
				},
			},
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// mapRequiresReplace is the Map equivalent of stringplanmodifier.RequiresReplace.
// Used to translate ForceNew on the value_specs attribute.
type mapRequiresReplace struct{}

func (mapRequiresReplace) Description(_ context.Context) string {
	return "If the value of this attribute changes, Terraform will destroy and recreate the resource."
}

func (mapRequiresReplace) MarkdownDescription(_ context.Context) string {
	return "If the value of this attribute changes, Terraform will destroy and recreate the resource."
}

func (mapRequiresReplace) PlanModifyMap(_ context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	if req.StateValue.IsNull() {
		return
	}

	if req.PlanValue.IsNull() {
		return
	}

	if !req.PlanValue.Equal(req.StateValue) {
		resp.RequiresReplace = true
	}
}

func (r *ikePolicyV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *ikePolicyV2Resource) regionFromPlan(plan ikePolicyV2Model) string {
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
		return plan.Region.ValueString()
	}

	return r.config.Region
}

func (r *ikePolicyV2Resource) regionFromState(state ikePolicyV2Model) string {
	if !state.Region.IsNull() && !state.Region.IsUnknown() && state.Region.ValueString() != "" {
		return state.Region.ValueString()
	}

	return r.config.Region
}

func (r *ikePolicyV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ikePolicyV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	region := r.regionFromPlan(plan)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	lifetime, lDiags := buildIKEPolicyLifetimeCreateOptsFromPlan(ctx, plan.Lifetime)
	resp.Diagnostics.Append(lDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	valueSpecs, vDiags := mapValueSpecsFromPlan(ctx, plan.ValueSpecs)
	resp.Diagnostics.Append(vDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	authAlgorithm := ikepolicies.AuthAlgorithm(plan.AuthAlgorithm.ValueString())
	encryptionAlgorithm := ikepolicies.EncryptionAlgorithm(plan.EncryptionAlgorithm.ValueString())
	pfs := ikepolicies.PFS(plan.PFS.ValueString())
	ikeVersion := ikepolicies.IKEVersion(plan.IKEVersion.ValueString())
	phase1NegMode := ikepolicies.Phase1NegotiationMode(plan.Phase1NegotiationMode.ValueString())

	opts := IKEPolicyCreateOpts{
		ikepolicies.CreateOpts{
			Name:                  plan.Name.ValueString(),
			Description:           plan.Description.ValueString(),
			TenantID:              plan.TenantID.ValueString(),
			Lifetime:              &lifetime,
			AuthAlgorithm:         authAlgorithm,
			EncryptionAlgorithm:   encryptionAlgorithm,
			PFS:                   pfs,
			IKEVersion:            ikeVersion,
			Phase1NegotiationMode: phase1NegMode,
		},
		valueSpecs,
	}

	log.Printf("[DEBUG] Create IKE policy: %#v", opts)

	policy, err := ikepolicies.Create(ctx, networkingClient, opts).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error creating openstack_vpnaas_ike_policy_v2", err.Error())

		return
	}

	stateConf := &retry.StateChangeConf{
		Pending:    []string{"PENDING_CREATE"},
		Target:     []string{"ACTIVE"},
		Refresh:    waitForIKEPolicyCreation(ctx, networkingClient, policy.ID),
		Timeout:    createTimeout,
		Delay:      0,
		MinTimeout: 2 * time.Second,
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for openstack_vpnaas_ike_policy_v2 to become active",
			fmt.Sprintf("policy %s: %s", policy.ID, err),
		)

		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)
	plan.Region = types.StringValue(region)

	pDiags := populateModelFromPolicy(ctx, &plan, policy)
	resp.Diagnostics.Append(pDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ikePolicyV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ikePolicyV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromState(state)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", state.ID.ValueString())

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, 404) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Error reading openstack_vpnaas_ike_policy_v2", err.Error())

		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", state.ID.ValueString(), policy)

	state.Region = types.StringValue(region)

	pDiags := populateModelFromPolicy(ctx, &state, policy)
	resp.Diagnostics.Append(pDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ikePolicyV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ikePolicyV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	region := r.regionFromPlan(plan)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	opts := ikepolicies.UpdateOpts{}

	var hasChange bool

	if !plan.Name.Equal(state.Name) {
		name := plan.Name.ValueString()
		opts.Name = &name
		hasChange = true
	}

	if !plan.Description.Equal(state.Description) {
		description := plan.Description.ValueString()
		opts.Description = &description
		hasChange = true
	}

	if !plan.PFS.Equal(state.PFS) {
		opts.PFS = ikepolicies.PFS(plan.PFS.ValueString())
		hasChange = true
	}

	if !plan.AuthAlgorithm.Equal(state.AuthAlgorithm) {
		opts.AuthAlgorithm = ikepolicies.AuthAlgorithm(plan.AuthAlgorithm.ValueString())
		hasChange = true
	}

	if !plan.EncryptionAlgorithm.Equal(state.EncryptionAlgorithm) {
		opts.EncryptionAlgorithm = ikepolicies.EncryptionAlgorithm(plan.EncryptionAlgorithm.ValueString())
		hasChange = true
	}

	if !plan.Phase1NegotiationMode.Equal(state.Phase1NegotiationMode) {
		opts.Phase1NegotiationMode = ikepolicies.Phase1NegotiationMode(plan.Phase1NegotiationMode.ValueString())
		hasChange = true
	}

	if !plan.IKEVersion.Equal(state.IKEVersion) {
		opts.IKEVersion = ikepolicies.IKEVersion(plan.IKEVersion.ValueString())
		hasChange = true
	}

	if !plan.Lifetime.Equal(state.Lifetime) {
		lifetime, lDiags := buildIKEPolicyLifetimeUpdateOptsFromPlan(ctx, plan.Lifetime)
		resp.Diagnostics.Append(lDiags...)

		if resp.Diagnostics.HasError() {
			return
		}

		opts.Lifetime = &lifetime
		hasChange = true
	}

	log.Printf("[DEBUG] Updating IKE policy with id %s: %#v", state.ID.ValueString(), opts)

	if hasChange {
		err = ikepolicies.Update(ctx, networkingClient, state.ID.ValueString(), opts).Err
		if err != nil {
			resp.Diagnostics.AddError("Error updating openstack_vpnaas_ike_policy_v2", err.Error())

			return
		}

		stateConf := &retry.StateChangeConf{
			Pending:    []string{"PENDING_UPDATE"},
			Target:     []string{"ACTIVE"},
			Refresh:    waitForIKEPolicyUpdate(ctx, networkingClient, state.ID.ValueString()),
			Timeout:    updateTimeout,
			Delay:      0,
			MinTimeout: 2 * time.Second,
		}

		if _, err := stateConf.WaitForStateContext(ctx); err != nil {
			resp.Diagnostics.AddError("Error waiting for openstack_vpnaas_ike_policy_v2 update", err.Error())

			return
		}
	}

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error reading openstack_vpnaas_ike_policy_v2 after update", err.Error())

		return
	}

	plan.ID = state.ID
	plan.Region = types.StringValue(region)

	pDiags := populateModelFromPolicy(ctx, &plan, policy)
	resp.Diagnostics.Append(pDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ikePolicyV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ikePolicyV2Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	region := r.regionFromState(state)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	log.Printf("[DEBUG] Destroy IKE policy: %s", state.ID.ValueString())

	stateConf := &retry.StateChangeConf{
		Pending:    []string{"ACTIVE"},
		Target:     []string{"DELETED"},
		Refresh:    waitForIKEPolicyDeletion(ctx, networkingClient, state.ID.ValueString()),
		Timeout:    deleteTimeout,
		Delay:      0,
		MinTimeout: 2 * time.Second,
	}

	if _, err := stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError("Error deleting openstack_vpnaas_ike_policy_v2", err.Error())

		return
	}
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// populateModelFromPolicy copies API response fields into the model.
func populateModelFromPolicy(_ context.Context, m *ikePolicyV2Model, policy *ikepolicies.Policy) diag.Diagnostics {
	var diagsOut diag.Diagnostics

	m.Name = types.StringValue(policy.Name)
	m.Description = types.StringValue(policy.Description)
	m.AuthAlgorithm = types.StringValue(string(policy.AuthAlgorithm))
	m.EncryptionAlgorithm = types.StringValue(string(policy.EncryptionAlgorithm))
	m.PFS = types.StringValue(string(policy.PFS))
	m.Phase1NegotiationMode = types.StringValue(string(policy.Phase1NegotiationMode))
	m.IKEVersion = types.StringValue(string(policy.IKEVersion))
	m.TenantID = types.StringValue(policy.TenantID)

	lifetimeObj, oDiags := types.ObjectValue(lifetimeAttrTypes(), map[string]attr.Value{
		"units": types.StringValue(string(policy.Lifetime.Units)),
		"value": types.Int64Value(int64(policy.Lifetime.Value)),
	})
	diagsOut.Append(oDiags...)

	if diagsOut.HasError() {
		return diagsOut
	}

	lifetimeSet, sDiags := types.SetValue(types.ObjectType{AttrTypes: lifetimeAttrTypes()}, []attr.Value{lifetimeObj})
	diagsOut.Append(sDiags...)

	if diagsOut.HasError() {
		return diagsOut
	}

	m.Lifetime = lifetimeSet

	return diagsOut
}

// helpers ------------------------------------------------------------------

// buildIKEPolicyLifetimeCreateOptsFromPlan converts the plan's lifetime set
// (max one element in practice) into ikepolicies.LifetimeCreateOpts.
func buildIKEPolicyLifetimeCreateOptsFromPlan(ctx context.Context, lifetime types.Set) (ikepolicies.LifetimeCreateOpts, diag.Diagnostics) {
	var out ikepolicies.LifetimeCreateOpts

	if lifetime.IsNull() || lifetime.IsUnknown() {
		return out, nil
	}

	type lifetimeElem struct {
		Units types.String `tfsdk:"units"`
		Value types.Int64  `tfsdk:"value"`
	}

	var elems []lifetimeElem

	diags := lifetime.ElementsAs(ctx, &elems, false)
	if diags.HasError() {
		return out, diags
	}

	for _, e := range elems {
		if !e.Units.IsNull() && !e.Units.IsUnknown() {
			out.Units = resourceIKEPolicyV2Unit(e.Units.ValueString())
		}

		if !e.Value.IsNull() && !e.Value.IsUnknown() {
			out.Value = int(e.Value.ValueInt64())
		}
	}

	return out, nil
}

func buildIKEPolicyLifetimeUpdateOptsFromPlan(ctx context.Context, lifetime types.Set) (ikepolicies.LifetimeUpdateOpts, diag.Diagnostics) {
	var out ikepolicies.LifetimeUpdateOpts

	if lifetime.IsNull() || lifetime.IsUnknown() {
		return out, nil
	}

	type lifetimeElem struct {
		Units types.String `tfsdk:"units"`
		Value types.Int64  `tfsdk:"value"`
	}

	var elems []lifetimeElem

	diags := lifetime.ElementsAs(ctx, &elems, false)
	if diags.HasError() {
		return out, diags
	}

	for _, e := range elems {
		if !e.Units.IsNull() && !e.Units.IsUnknown() {
			out.Units = resourceIKEPolicyV2Unit(e.Units.ValueString())
		}

		if !e.Value.IsNull() && !e.Value.IsUnknown() {
			out.Value = int(e.Value.ValueInt64())
		}
	}

	return out, nil
}

func mapValueSpecsFromPlan(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return map[string]string{}, nil
	}

	out := map[string]string{}

	diags := m.ElementsAs(ctx, &out, false)

	return out, diags
}

func resourceIKEPolicyV2Unit(v string) ikepolicies.Unit {
	var unit ikepolicies.Unit

	switch v {
	case "kilobytes":
		unit = ikepolicies.UnitKilobytes
	case "seconds":
		unit = ikepolicies.UnitSeconds
	}

	return unit
}

func waitForIKEPolicyDeletion(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) retry.StateRefreshFunc {
	return func() (any, string, error) {
		err := ikepolicies.Delete(ctx, networkingClient, id).Err
		if err == nil {
			return "", "DELETED", nil
		}

		return nil, "ACTIVE", err
	}
}

func waitForIKEPolicyCreation(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) retry.StateRefreshFunc {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_CREATE", nil
		}

		return policy, "ACTIVE", nil
	}
}

func waitForIKEPolicyUpdate(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) retry.StateRefreshFunc {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_UPDATE", nil
		}

		return policy, "ACTIVE", nil
	}
}
