package openstack

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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

var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// NewIKEPolicyV2Resource returns the framework resource implementation for
// openstack_vpnaas_ike_policy_v2.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

type ikePolicyV2Lifetime struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

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

func (r *ikePolicyV2Resource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpnaas_ike_policy_v2"
}

func (r *ikePolicyV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"auth_algorithm": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("sha1"),
				Validators: []validator.String{
					ikePolicyV2AuthAlgorithmValidator{},
				},
			},
			"encryption_algorithm": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("aes-128"),
				Validators: []validator.String{
					ikePolicyV2EncryptionAlgorithmValidator{},
				},
			},
			"pfs": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("group5"),
				Validators: []validator.String{
					ikePolicyV2PFSValidator{},
				},
			},
			"phase1_negotiation_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("main"),
				Validators: []validator.String{
					ikePolicyV2Phase1NegotiationModeValidator{},
				},
			},
			"ike_version": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("v1"),
				Validators: []validator.String{
					ikePolicyV2IKEVersionValidator{},
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
					mapForceNewPlanModifier{},
				},
			},
		},
		Blocks: map[string]schema.Block{
			"lifetime": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"units": schema.StringAttribute{
							Optional: true,
							Computed: true,
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

func (r *ikePolicyV2Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected provider data",
			fmt.Sprintf("expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *ikePolicyV2Resource) regionFromPlan(s types.String) string {
	if !s.IsNull() && !s.IsUnknown() && s.ValueString() != "" {
		return s.ValueString()
	}

	return r.config.Region
}

func (r *ikePolicyV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ikePolicyV2Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromPlan(plan.Region)

	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

		return
	}

	lifetime, lifetimeDiags := ikePolicyV2LifetimeCreateOptsFromPlan(ctx, plan.Lifetime)
	resp.Diagnostics.Append(lifetimeDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	valueSpecs, vsDiags := mapStringFromTypesMap(ctx, plan.ValueSpecs)
	resp.Diagnostics.Append(vsDiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	opts := IKEPolicyCreateOpts{
		ikepolicies.CreateOpts{
			Name:                  plan.Name.ValueString(),
			Description:           plan.Description.ValueString(),
			TenantID:              plan.TenantID.ValueString(),
			Lifetime:              &lifetime,
			AuthAlgorithm:         ikepolicies.AuthAlgorithm(plan.AuthAlgorithm.ValueString()),
			EncryptionAlgorithm:   ikepolicies.EncryptionAlgorithm(plan.EncryptionAlgorithm.ValueString()),
			PFS:                   ikepolicies.PFS(plan.PFS.ValueString()),
			IKEVersion:            ikepolicies.IKEVersion(plan.IKEVersion.ValueString()),
			Phase1NegotiationMode: ikepolicies.Phase1NegotiationMode(plan.Phase1NegotiationMode.ValueString()),
		},
		valueSpecs,
	}

	log.Printf("[DEBUG] Create IKE policy: %#v", opts)

	policy, err := ikepolicies.Create(ctx, networkingClient, opts).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error creating IKE policy", err.Error())

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

	r.fillModelFromPolicy(&plan, policy, &resp.Diagnostics)

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

	region := r.regionFromPlan(state.Region)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", state.ID.ValueString())

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, 404) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Error reading IKE policy", err.Error())

		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", state.ID.ValueString(), policy)

	state.Region = types.StringValue(region)
	r.fillModelFromPolicy(&state, policy, &resp.Diagnostics)

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

	region := r.regionFromPlan(plan.Region)

	updateTimeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

		return
	}

	opts := ikepolicies.UpdateOpts{}

	hasChange := false

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
		lifetime, ldiags := ikePolicyV2LifetimeUpdateOptsFromPlan(ctx, plan.Lifetime)
		resp.Diagnostics.Append(ldiags...)

		if resp.Diagnostics.HasError() {
			return
		}

		opts.Lifetime = &lifetime
		hasChange = true
	}

	log.Printf("[DEBUG] Updating IKE policy with id %s: %#v", state.ID.ValueString(), opts)

	if hasChange {
		if err := ikepolicies.Update(ctx, networkingClient, state.ID.ValueString(), opts).Err; err != nil {
			resp.Diagnostics.AddError("Error updating IKE policy", err.Error())

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
			resp.Diagnostics.AddError("Error waiting for IKE policy update", err.Error())

			return
		}
	}

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error refreshing IKE policy", err.Error())

		return
	}

	plan.ID = state.ID
	plan.Region = types.StringValue(region)

	r.fillModelFromPolicy(&plan, policy, &resp.Diagnostics)

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

	region := r.regionFromPlan(state.Region)

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

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
		resp.Diagnostics.AddError("Error deleting IKE policy", err.Error())

		return
	}
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// fillModelFromPolicy writes API values into the framework model.
func (r *ikePolicyV2Resource) fillModelFromPolicy(m *ikePolicyV2Model, policy *ikepolicies.Policy, diags *diag.Diagnostics) {
	m.Name = types.StringValue(policy.Name)
	m.Description = types.StringValue(policy.Description)
	m.AuthAlgorithm = types.StringValue(string(policy.AuthAlgorithm))
	m.EncryptionAlgorithm = types.StringValue(string(policy.EncryptionAlgorithm))
	m.TenantID = types.StringValue(policy.TenantID)
	m.PFS = types.StringValue(string(policy.PFS))
	m.Phase1NegotiationMode = types.StringValue(string(policy.Phase1NegotiationMode))
	m.IKEVersion = types.StringValue(string(policy.IKEVersion))

	lifetimeAttrTypes := map[string]attr.Type{
		"units": types.StringType,
		"value": types.Int64Type,
	}
	lifetimeObjectType := types.ObjectType{AttrTypes: lifetimeAttrTypes}

	lifetimeObj, objDiags := types.ObjectValue(lifetimeAttrTypes, map[string]attr.Value{
		"units": types.StringValue(string(policy.Lifetime.Units)),
		"value": types.Int64Value(int64(policy.Lifetime.Value)),
	})
	diags.Append(objDiags...)

	if objDiags.HasError() {
		return
	}

	lifetimeSet, setDiags := types.SetValue(lifetimeObjectType, []attr.Value{lifetimeObj})
	diags.Append(setDiags...)

	if setDiags.HasError() {
		return
	}

	m.Lifetime = lifetimeSet
}

// ----------------------------------------------------------------------------
// Helpers translating between framework typed values and gophercloud structs.
// ----------------------------------------------------------------------------

func ikePolicyV2LifetimeCreateOptsFromPlan(ctx context.Context, set types.Set) (ikepolicies.LifetimeCreateOpts, diag.Diagnostics) {
	out := ikepolicies.LifetimeCreateOpts{}

	if set.IsNull() || set.IsUnknown() {
		return out, nil
	}

	var elements []ikePolicyV2Lifetime

	diags := set.ElementsAs(ctx, &elements, false)
	if diags.HasError() {
		return out, diags
	}

	for _, e := range elements {
		out.Units = resourceIKEPolicyV2Unit(e.Units.ValueString())
		out.Value = int(e.Value.ValueInt64())
	}

	return out, nil
}

func ikePolicyV2LifetimeUpdateOptsFromPlan(ctx context.Context, set types.Set) (ikepolicies.LifetimeUpdateOpts, diag.Diagnostics) {
	out := ikepolicies.LifetimeUpdateOpts{}

	if set.IsNull() || set.IsUnknown() {
		return out, nil
	}

	var elements []ikePolicyV2Lifetime

	diags := set.ElementsAs(ctx, &elements, false)
	if diags.HasError() {
		return out, diags
	}

	for _, e := range elements {
		out.Units = resourceIKEPolicyV2Unit(e.Units.ValueString())
		out.Value = int(e.Value.ValueInt64())
	}

	return out, nil
}

func resourceIKEPolicyV2Unit(v string) ikepolicies.Unit {
	switch v {
	case "kilobytes":
		return ikepolicies.UnitKilobytes
	case "seconds":
		return ikepolicies.UnitSeconds
	}

	return ""
}

// mapStringFromTypesMap reads a types.Map (with string element type) into a
// map[string]string. Returns an empty map if the value is null/unknown.
func mapStringFromTypesMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return map[string]string{}, nil
	}

	out := map[string]string{}

	diags := m.ElementsAs(ctx, &out, false)

	return out, diags
}

// ----------------------------------------------------------------------------
// Plan modifiers
// ----------------------------------------------------------------------------

// mapForceNewPlanModifier triggers a replacement when the value of a Map
// attribute changes. Used for value_specs which was ForceNew under SDKv2.
type mapForceNewPlanModifier struct{}

func (mapForceNewPlanModifier) Description(context.Context) string {
	return "force resource replacement when this attribute changes"
}

func (m mapForceNewPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (mapForceNewPlanModifier) PlanModifyMap(ctx context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	if req.StateValue.IsNull() {
		return
	}

	if !req.PlanValue.Equal(req.StateValue) {
		resp.RequiresReplace = true
	}
}

// ----------------------------------------------------------------------------
// Validators (port of the SDKv2 ValidateFunc helpers).
// ----------------------------------------------------------------------------

type ikePolicyV2AuthAlgorithmValidator struct{}

func (ikePolicyV2AuthAlgorithmValidator) Description(context.Context) string {
	return "must be a supported auth algorithm"
}

func (v ikePolicyV2AuthAlgorithmValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (ikePolicyV2AuthAlgorithmValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	switch ikepolicies.AuthAlgorithm(req.ConfigValue.ValueString()) {
	case ikepolicies.AuthAlgorithmSHA1,
		ikepolicies.AuthAlgorithmSHA256,
		ikepolicies.AuthAlgorithmSHA384,
		ikepolicies.AuthAlgorithmSHA512,
		ikepolicies.AuthAlgorithmAESXCBC,
		ikepolicies.AuthAlgorithmAESCMAC:
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"invalid auth_algorithm",
		fmt.Sprintf("unknown %q for openstack_vpnaas_ike_policy_v2", req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2EncryptionAlgorithmValidator struct{}

func (ikePolicyV2EncryptionAlgorithmValidator) Description(context.Context) string {
	return "must be a supported encryption algorithm"
}

func (v ikePolicyV2EncryptionAlgorithmValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (ikePolicyV2EncryptionAlgorithmValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	switch ikepolicies.EncryptionAlgorithm(req.ConfigValue.ValueString()) {
	case ikepolicies.EncryptionAlgorithm3DES,
		ikepolicies.EncryptionAlgorithmAES128,
		ikepolicies.EncryptionAlgorithmAES256,
		ikepolicies.EncryptionAlgorithmAES192,
		ikepolicies.EncryptionAlgorithmAES128CTR,
		ikepolicies.EncryptionAlgorithmAES192CTR,
		ikepolicies.EncryptionAlgorithmAES256CTR,
		ikepolicies.EncryptionAlgorithmAES128CCM8,
		ikepolicies.EncryptionAlgorithmAES192CCM8,
		ikepolicies.EncryptionAlgorithmAES256CCM8,
		ikepolicies.EncryptionAlgorithmAES128CCM12,
		ikepolicies.EncryptionAlgorithmAES192CCM12,
		ikepolicies.EncryptionAlgorithmAES256CCM12,
		ikepolicies.EncryptionAlgorithmAES128CCM16,
		ikepolicies.EncryptionAlgorithmAES192CCM16,
		ikepolicies.EncryptionAlgorithmAES256CCM16,
		ikepolicies.EncryptionAlgorithmAES128GCM8,
		ikepolicies.EncryptionAlgorithmAES192GCM8,
		ikepolicies.EncryptionAlgorithmAES256GCM8,
		ikepolicies.EncryptionAlgorithmAES128GCM12,
		ikepolicies.EncryptionAlgorithmAES192GCM12,
		ikepolicies.EncryptionAlgorithmAES256GCM12,
		ikepolicies.EncryptionAlgorithmAES128GCM16,
		ikepolicies.EncryptionAlgorithmAES192GCM16,
		ikepolicies.EncryptionAlgorithmAES256GCM16:
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"invalid encryption_algorithm",
		fmt.Sprintf("unknown %q for openstack_vpnaas_ike_policy_v2", req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2PFSValidator struct{}

func (ikePolicyV2PFSValidator) Description(context.Context) string {
	return "must be a supported PFS group"
}

func (v ikePolicyV2PFSValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (ikePolicyV2PFSValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	switch ikepolicies.PFS(req.ConfigValue.ValueString()) {
	case ikepolicies.PFSGroup2,
		ikepolicies.PFSGroup5,
		ikepolicies.PFSGroup14,
		ikepolicies.PFSGroup15,
		ikepolicies.PFSGroup16,
		ikepolicies.PFSGroup17,
		ikepolicies.PFSGroup18,
		ikepolicies.PFSGroup19,
		ikepolicies.PFSGroup20,
		ikepolicies.PFSGroup21,
		ikepolicies.PFSGroup22,
		ikepolicies.PFSGroup23,
		ikepolicies.PFSGroup24,
		ikepolicies.PFSGroup25,
		ikepolicies.PFSGroup26,
		ikepolicies.PFSGroup27,
		ikepolicies.PFSGroup28,
		ikepolicies.PFSGroup29,
		ikepolicies.PFSGroup30,
		ikepolicies.PFSGroup31:
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"invalid pfs",
		fmt.Sprintf("unknown %q for openstack_vpnaas_ike_policy_v2", req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2IKEVersionValidator struct{}

func (ikePolicyV2IKEVersionValidator) Description(context.Context) string {
	return "must be a supported IKE version"
}

func (v ikePolicyV2IKEVersionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (ikePolicyV2IKEVersionValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	switch ikepolicies.IKEVersion(req.ConfigValue.ValueString()) {
	case ikepolicies.IKEVersionv1, ikepolicies.IKEVersionv2:
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"invalid ike_version",
		fmt.Sprintf("unknown %q for openstack_vpnaas_ike_policy_v2", req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2Phase1NegotiationModeValidator struct{}

func (ikePolicyV2Phase1NegotiationModeValidator) Description(context.Context) string {
	return "must be a supported phase1 negotiation mode"
}

func (v ikePolicyV2Phase1NegotiationModeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (ikePolicyV2Phase1NegotiationModeValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	switch ikepolicies.Phase1NegotiationMode(req.ConfigValue.ValueString()) {
	case ikepolicies.Phase1NegotiationModeMain:
		return
	}

	resp.Diagnostics.AddAttributeError(
		req.Path,
		"invalid phase1_negotiation_mode",
		fmt.Sprintf("unknown %q for openstack_vpnaas_ike_policy_v2", req.ConfigValue.ValueString()),
	)
}
