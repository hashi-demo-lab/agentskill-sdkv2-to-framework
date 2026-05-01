package openstack

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// stateRefreshFunc is a small in-package replacement for the SDKv2
// helper retry.StateRefreshFunc; the framework does not ship a retry helper,
// so we keep a focused poller for the create / update / delete state
// transitions used by this resource.
type stateRefreshFunc func() (any, string, error)

// stateChangeConf is the local equivalent of the SDKv2 helper
// retry.StateChangeConf used by the IKE policy resource's create / update /
// delete waits.
type stateChangeConf struct {
	Pending    []string
	Target     []string
	Refresh    stateRefreshFunc
	Timeout    time.Duration
	Delay      time.Duration
	MinTimeout time.Duration
}

// WaitForStateContext polls Refresh until the state is in Target or the
// context is done. Mirrors the SDKv2 helper's signature so the call sites in
// Create / Update / Delete read the same.
func (c *stateChangeConf) WaitForStateContext(ctx context.Context) (any, error) {
	if c.MinTimeout <= 0 {
		c.MinTimeout = 1 * time.Second
	}
	if c.Delay > 0 {
		select {
		case <-time.After(c.Delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	deadline := time.Now().Add(c.Timeout)
	for {
		raw, state, err := c.Refresh()
		if err != nil {
			return raw, err
		}
		for _, t := range c.Target {
			if state == t {
				return raw, nil
			}
		}
		matched := false
		for _, p := range c.Pending {
			if state == p {
				matched = true
				break
			}
		}
		if !matched {
			return raw, fmt.Errorf("unexpected state %q (expected one of %v or target %v)", state, c.Pending, c.Target)
		}
		if time.Now().After(deadline) {
			return raw, fmt.Errorf("timeout waiting for state %v", c.Target)
		}
		select {
		case <-time.After(c.MinTimeout):
		case <-ctx.Done():
			return raw, ctx.Err()
		}
	}
}

// Compile-time interface assertions.
var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// NewIKEPolicyV2Resource is the framework constructor for
// openstack_vpnaas_ike_policy_v2.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

// ikePolicyV2Model is the typed plan/state representation.
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

// lifetimeModel maps the lifetime { units, value } block.
type lifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

// lifetimeAttrTypes describes the shape of the lifetime nested object — used
// when constructing computed lifetime values from API responses.
var lifetimeAttrTypes = map[string]attr.Type{
	"units": types.StringType,
	"value": types.Int64Type,
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
			// Default = stringdefault.StaticString — note: a Default is NOT a
			// plan modifier in the framework; it's its own field. Attributes
			// with a Default must be Computed (so the framework can populate
			// the plan with the default when the practitioner omits the
			// attribute).
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
					mapplanmodifier.RequiresReplace(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			// lifetime is a true repeating-set block in the SDKv2 form
			// (TypeSet + Elem = &schema.Resource), so we keep it as a block
			// to preserve practitioner HCL: `lifetime { units = "..." value = ... }`.
			// See references/blocks.md (true repeating blocks → keep as block).
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
			// Timeouts.Block preserves the existing `timeouts { ... }` block
			// HCL syntax that the SDKv2 resource accepted via
			// schema.ResourceTimeout.
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
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
			fmt.Sprintf("expected *openstack.Config, got %T", req.ProviderData),
		)
		return
	}
	r.config = cfg
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// regionFor returns the region from the model, falling back to the provider's
// configured region — equivalent to the SDKv2 GetRegion helper.
func (r *ikePolicyV2Resource) regionFor(model ikePolicyV2Model) string {
	if !model.Region.IsNull() && !model.Region.IsUnknown() && model.Region.ValueString() != "" {
		return model.Region.ValueString()
	}
	return r.config.Region
}

// mapValueSpecsFromModel reflects the SDKv2 MapValueSpecs helper for a typed map.
func mapValueSpecsFromModel(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	out := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return out, nil
	}
	var raw map[string]string
	diags := m.ElementsAs(ctx, &raw, false)
	if diags.HasError() {
		return nil, diags
	}
	for k, v := range raw {
		out[k] = v
	}
	return out, nil
}

// lifetimeFromModel pulls the lifetime block out of a Set value as a single
// LifetimeCreateOpts (matching the SDKv2 helper that iterated the set and
// took the last entry's values).
func lifetimeFromModel(ctx context.Context, set types.Set) (ikepolicies.LifetimeCreateOpts, error) {
	opts := ikepolicies.LifetimeCreateOpts{}
	if set.IsNull() || set.IsUnknown() {
		return opts, nil
	}
	var rows []lifetimeModel
	diags := set.ElementsAs(ctx, &rows, false)
	if diags.HasError() {
		return opts, fmt.Errorf("failed to read lifetime block: %s", diags)
	}
	for _, row := range rows {
		if !row.Units.IsNull() && !row.Units.IsUnknown() {
			opts.Units = ikePolicyV2UnitFromString(row.Units.ValueString())
		}
		if !row.Value.IsNull() && !row.Value.IsUnknown() {
			opts.Value = int(row.Value.ValueInt64())
		}
	}
	return opts, nil
}

func lifetimeUpdateFromModel(ctx context.Context, set types.Set) (ikepolicies.LifetimeUpdateOpts, error) {
	opts := ikepolicies.LifetimeUpdateOpts{}
	if set.IsNull() || set.IsUnknown() {
		return opts, nil
	}
	var rows []lifetimeModel
	diags := set.ElementsAs(ctx, &rows, false)
	if diags.HasError() {
		return opts, fmt.Errorf("failed to read lifetime block: %s", diags)
	}
	for _, row := range rows {
		if !row.Units.IsNull() && !row.Units.IsUnknown() {
			opts.Units = ikePolicyV2UnitFromString(row.Units.ValueString())
		}
		if !row.Value.IsNull() && !row.Value.IsUnknown() {
			opts.Value = int(row.Value.ValueInt64())
		}
	}
	return opts, nil
}

func ikePolicyV2UnitFromString(v string) ikepolicies.Unit {
	switch v {
	case "kilobytes":
		return ikepolicies.UnitKilobytes
	case "seconds":
		return ikepolicies.UnitSeconds
	}
	return ""
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

	region := r.regionFor(plan)
	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	lifetime, err := lifetimeFromModel(ctx, plan.Lifetime)
	if err != nil {
		resp.Diagnostics.AddError("Invalid lifetime block", err.Error())
		return
	}

	valueSpecs, vDiags := mapValueSpecsFromModel(ctx, plan.ValueSpecs)
	resp.Diagnostics.Append(vDiags...)
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

	stateConf := &stateChangeConf{
		Pending:    []string{"PENDING_CREATE"},
		Target:     []string{"ACTIVE"},
		Refresh:    waitForIKEPolicyCreation(ctx, networkingClient, policy.ID),
		Timeout:    createTimeout,
		Delay:      0,
		MinTimeout: 2 * time.Second,
	}
	if _, err = stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for IKE policy to become active",
			fmt.Sprintf("policy %s: %s", policy.ID, err),
		)
		return
	}

	plan.ID = types.StringValue(policy.ID)
	plan.Region = types.StringValue(region)
	resp.Diagnostics.Append(r.populateModelFromAPI(ctx, &plan, policy)...)
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

	region := r.regionFor(state)
	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading IKE policy",
			fmt.Sprintf("%s: %s", state.ID.ValueString(), err),
		)
		return
	}

	state.Region = types.StringValue(region)
	resp.Diagnostics.Append(r.populateModelFromAPI(ctx, &state, policy)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// populateModelFromAPI mirrors the SDKv2 d.Set(...) cluster in the original
// Read function; returns any conversion diagnostics.
func (r *ikePolicyV2Resource) populateModelFromAPI(_ context.Context, model *ikePolicyV2Model, policy *ikepolicies.Policy) diag.Diagnostics {
	var allDiags diag.Diagnostics

	model.Name = types.StringValue(policy.Name)
	model.Description = types.StringValue(policy.Description)
	model.AuthAlgorithm = types.StringValue(string(policy.AuthAlgorithm))
	model.EncryptionAlgorithm = types.StringValue(string(policy.EncryptionAlgorithm))
	model.TenantID = types.StringValue(policy.TenantID)
	model.PFS = types.StringValue(string(policy.PFS))
	model.Phase1NegotiationMode = types.StringValue(string(policy.Phase1NegotiationMode))
	model.IKEVersion = types.StringValue(string(policy.IKEVersion))

	lifetimeObj, oDiags := types.ObjectValue(lifetimeAttrTypes, map[string]attr.Value{
		"units": types.StringValue(string(policy.Lifetime.Units)),
		"value": types.Int64Value(int64(policy.Lifetime.Value)),
	})
	allDiags.Append(oDiags...)
	if oDiags.HasError() {
		return allDiags
	}
	setVal, sDiags := types.SetValue(types.ObjectType{AttrTypes: lifetimeAttrTypes}, []attr.Value{lifetimeObj})
	allDiags.Append(sDiags...)
	if sDiags.HasError() {
		return allDiags
	}
	model.Lifetime = setVal
	return allDiags
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

	region := r.regionFor(plan)
	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
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
		desc := plan.Description.ValueString()
		opts.Description = &desc
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
		lifetime, lErr := lifetimeUpdateFromModel(ctx, plan.Lifetime)
		if lErr != nil {
			resp.Diagnostics.AddError("Invalid lifetime block", lErr.Error())
			return
		}
		opts.Lifetime = &lifetime
		hasChange = true
	}

	log.Printf("[DEBUG] Updating IKE policy with id %s: %#v", state.ID.ValueString(), opts)

	if hasChange {
		err = ikepolicies.Update(ctx, networkingClient, state.ID.ValueString(), opts).Err
		if err != nil {
			resp.Diagnostics.AddError("Error updating IKE policy", err.Error())
			return
		}
		stateConf := &stateChangeConf{
			Pending:    []string{"PENDING_UPDATE"},
			Target:     []string{"ACTIVE"},
			Refresh:    waitForIKEPolicyUpdate(ctx, networkingClient, state.ID.ValueString()),
			Timeout:    updateTimeout,
			Delay:      0,
			MinTimeout: 2 * time.Second,
		}
		if _, err = stateConf.WaitForStateContext(ctx); err != nil {
			resp.Diagnostics.AddError("Error waiting for IKE policy update", err.Error())
			return
		}
	}

	// Re-read so the state reflects the API view (mirrors original behaviour
	// which ended Update with a Read call).
	policy, gErr := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if gErr != nil {
		resp.Diagnostics.AddError("Error reading IKE policy after update", gErr.Error())
		return
	}
	plan.ID = state.ID
	plan.Region = types.StringValue(region)
	resp.Diagnostics.Append(r.populateModelFromAPI(ctx, &plan, policy)...)
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

	region := r.regionFor(state)
	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	log.Printf("[DEBUG] Destroy IKE policy: %s", state.ID.ValueString())

	stateConf := &stateChangeConf{
		Pending:    []string{"ACTIVE"},
		Target:     []string{"DELETED"},
		Refresh:    waitForIKEPolicyDeletion(ctx, networkingClient, state.ID.ValueString()),
		Timeout:    deleteTimeout,
		Delay:      0,
		MinTimeout: 2 * time.Second,
	}
	if _, err = stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError("Error deleting IKE policy", err.Error())
		return
	}
}

// ---------------------------------------------------------------------------
// Wait/refresh helpers — unchanged from the SDKv2 version (they don't touch
// schema or schema.ResourceData).
// ---------------------------------------------------------------------------

func waitForIKEPolicyDeletion(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) stateRefreshFunc {
	return func() (any, string, error) {
		err := ikepolicies.Delete(ctx, networkingClient, id).Err
		if err == nil {
			return "", "DELETED", nil
		}
		return nil, "ACTIVE", err
	}
}

func waitForIKEPolicyCreation(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) stateRefreshFunc {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_CREATE", nil
		}
		return policy, "ACTIVE", nil
	}
}

func waitForIKEPolicyUpdate(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) stateRefreshFunc {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_UPDATE", nil
		}
		return policy, "ACTIVE", nil
	}
}

// ---------------------------------------------------------------------------
// Validators — port of the SDKv2 ValidateFunc closures to framework
// validator.String implementations. Kept in this file (rather than a shared
// location) to mirror the original layout.
// ---------------------------------------------------------------------------

type ikePolicyV2AuthAlgorithmValidator struct{}

func (ikePolicyV2AuthAlgorithmValidator) Description(_ context.Context) string {
	return "must be a valid IKE policy auth algorithm"
}
func (ikePolicyV2AuthAlgorithmValidator) MarkdownDescription(_ context.Context) string {
	return "must be a valid IKE policy auth algorithm"
}
func (ikePolicyV2AuthAlgorithmValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
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
		"Invalid auth_algorithm",
		fmt.Sprintf("unknown %q %s for openstack_vpnaas_ike_policy_v2", req.Path.String(), req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2EncryptionAlgorithmValidator struct{}

func (ikePolicyV2EncryptionAlgorithmValidator) Description(_ context.Context) string {
	return "must be a valid IKE policy encryption algorithm"
}
func (ikePolicyV2EncryptionAlgorithmValidator) MarkdownDescription(_ context.Context) string {
	return "must be a valid IKE policy encryption algorithm"
}
func (ikePolicyV2EncryptionAlgorithmValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
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
		"Invalid encryption_algorithm",
		fmt.Sprintf("unknown %q %s for openstack_vpnaas_ike_policy_v2", req.Path.String(), req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2PFSValidator struct{}

func (ikePolicyV2PFSValidator) Description(_ context.Context) string {
	return "must be a valid IKE policy PFS group"
}
func (ikePolicyV2PFSValidator) MarkdownDescription(_ context.Context) string {
	return "must be a valid IKE policy PFS group"
}
func (ikePolicyV2PFSValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
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
		"Invalid pfs",
		fmt.Sprintf("unknown %q %s for openstack_vpnaas_ike_policy_v2", req.Path.String(), req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2IKEVersionValidator struct{}

func (ikePolicyV2IKEVersionValidator) Description(_ context.Context) string {
	return "must be a valid IKE version"
}
func (ikePolicyV2IKEVersionValidator) MarkdownDescription(_ context.Context) string {
	return "must be a valid IKE version"
}
func (ikePolicyV2IKEVersionValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	switch ikepolicies.IKEVersion(req.ConfigValue.ValueString()) {
	case ikepolicies.IKEVersionv1, ikepolicies.IKEVersionv2:
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid ike_version",
		fmt.Sprintf("unknown %q %s for openstack_vpnaas_ike_policy_v2", req.Path.String(), req.ConfigValue.ValueString()),
	)
}

type ikePolicyV2Phase1NegotiationModeValidator struct{}

func (ikePolicyV2Phase1NegotiationModeValidator) Description(_ context.Context) string {
	return "must be a valid IKE phase1 negotiation mode"
}
func (ikePolicyV2Phase1NegotiationModeValidator) MarkdownDescription(_ context.Context) string {
	return "must be a valid IKE phase1 negotiation mode"
}
func (ikePolicyV2Phase1NegotiationModeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	switch ikepolicies.Phase1NegotiationMode(req.ConfigValue.ValueString()) {
	case ikepolicies.Phase1NegotiationModeMain:
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid phase1_negotiation_mode",
		fmt.Sprintf("unknown %q %s for openstack_vpnaas_ike_policy_v2", req.Path.String(), req.ConfigValue.ValueString()),
	)
}
