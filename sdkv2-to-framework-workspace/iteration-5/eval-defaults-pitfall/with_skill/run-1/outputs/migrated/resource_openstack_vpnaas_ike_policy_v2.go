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
)

// Compile-time interface checks.
var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// NewIKEPolicyV2Resource returns a new framework-based resource for the
// openstack_vpnaas_ike_policy_v2 resource.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

// ikePolicyV2Model is the typed model for the resource. The tfsdk tags
// must match the schema attribute names exactly.
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
	TenantID              types.String   `tfsdk:"tenant_id"`
	ValueSpecs            types.Map      `tfsdk:"value_specs"`
	Lifetime              types.Set      `tfsdk:"lifetime"`
	Timeouts              timeouts.Value `tfsdk:"timeouts"`
}

// ikePolicyV2LifetimeModel is the per-element model for the `lifetime` set.
type ikePolicyV2LifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

func (r *ikePolicyV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpnaas_ike_policy_v2"
}

func (r *ikePolicyV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
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
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"auth_algorithm": schema.StringAttribute{
				Optional: true,
				Computed: true,
				// Default lives on the per-type defaults package — NOT in PlanModifiers.
				Default: stringdefault.StaticString("sha1"),
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
				ElementType: types.StringType,
				Optional:    true,
				// SDKv2 had ForceNew on value_specs; framework uses a plan modifier.
				// (mapplanmodifier is available in current framework versions.)
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
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

		return
	}

	lifetime, ldiags := buildLifetimeCreateOpts(ctx, plan.Lifetime)
	resp.Diagnostics.Append(ldiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	valueSpecs, vdiags := mapStringFromTypesMap(ctx, plan.ValueSpecs)
	resp.Diagnostics.Append(vdiags...)

	if resp.Diagnostics.HasError() {
		return
	}

	authAlgorithm := ikepolicies.AuthAlgorithm(plan.AuthAlgorithm.ValueString())
	encryptionAlgorithm := ikepolicies.EncryptionAlgorithm(plan.EncryptionAlgorithm.ValueString())
	pfs := ikepolicies.PFS(plan.PFS.ValueString())
	ikeVersion := ikepolicies.IKEVersion(plan.IKEVersion.ValueString())
	phase1Mode := ikepolicies.Phase1NegotiationMode(plan.Phase1NegotiationMode.ValueString())

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
			Phase1NegotiationMode: phase1Mode,
		},
		valueSpecs,
	}

	log.Printf("[DEBUG] Create IKE policy: %#v", opts)

	policy, err := ikepolicies.Create(ctx, networkingClient, opts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating openstack_vpnaas_ike_policy_v2",
			err.Error(),
		)

		return
	}

	if err := waitForIKEPolicyActive(ctx, networkingClient, policy.ID); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", policy.ID),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)
	plan.Region = types.StringValue(region)

	pdiags := populateModelFromPolicy(&plan, policy)
	resp.Diagnostics.Append(pdiags...)

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
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", state.ID.ValueString())

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading openstack_vpnaas_ike_policy_v2 %s", state.ID.ValueString()),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", state.ID.ValueString(), policy)

	state.Region = types.StringValue(region)

	pdiags := populateModelFromPolicy(&state, policy)
	resp.Diagnostics.Append(pdiags...)

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

	region := r.regionFor(plan)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

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
		lifetime, ldiags := buildLifetimeUpdateOpts(ctx, plan.Lifetime)
		resp.Diagnostics.Append(ldiags...)

		if resp.Diagnostics.HasError() {
			return
		}

		opts.Lifetime = &lifetime
		hasChange = true
	}

	log.Printf("[DEBUG] Updating IKE policy with id %s: %#v", plan.ID.ValueString(), opts)

	if hasChange {
		if err := ikepolicies.Update(ctx, networkingClient, plan.ID.ValueString(), opts).Err; err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error updating openstack_vpnaas_ike_policy_v2 %s", plan.ID.ValueString()),
				err.Error(),
			)

			return
		}

		if err := waitForIKEPolicyActive(ctx, networkingClient, plan.ID.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active after update", plan.ID.ValueString()),
				err.Error(),
			)

			return
		}
	}

	policy, err := ikepolicies.Get(ctx, networkingClient, plan.ID.ValueString()).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading openstack_vpnaas_ike_policy_v2 %s after update", plan.ID.ValueString()),
			err.Error(),
		)

		return
	}

	plan.Region = types.StringValue(region)

	pdiags := populateModelFromPolicy(&plan, policy)
	resp.Diagnostics.Append(pdiags...)

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
		resp.Diagnostics.AddError(
			"Error creating OpenStack networking client",
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Destroy IKE policy: %s", state.ID.ValueString())

	if err := waitForIKEPolicyDeletedFramework(ctx, networkingClient, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_vpnaas_ike_policy_v2 %s", state.ID.ValueString()),
			err.Error(),
		)

		return
	}
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// regionFor returns the resource region — the configured value if set, the
// provider-level region otherwise.
func (r *ikePolicyV2Resource) regionFor(m ikePolicyV2Model) string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}

	if r.config != nil {
		return r.config.Region
	}

	return ""
}

// populateModelFromPolicy mirrors the SDKv2 d.Set(...) calls in Read.
func populateModelFromPolicy(m *ikePolicyV2Model, policy *ikepolicies.Policy) diag.Diagnostics {
	var diags diag.Diagnostics

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
	lifetimeObjType := types.ObjectType{AttrTypes: lifetimeAttrTypes}

	lifetimeElem, oDiags := types.ObjectValue(
		lifetimeAttrTypes,
		map[string]attr.Value{
			"units": types.StringValue(string(policy.Lifetime.Units)),
			"value": types.Int64Value(int64(policy.Lifetime.Value)),
		},
	)
	diags.Append(oDiags...)

	lifetimeSet, sDiags := types.SetValue(lifetimeObjType, []attr.Value{lifetimeElem})
	diags.Append(sDiags...)

	m.Lifetime = lifetimeSet

	return diags
}

func mapStringFromTypesMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out, nil
	}

	raw := map[string]string{}

	d := m.ElementsAs(ctx, &raw, false)
	if d.HasError() {
		return out, d
	}

	return raw, nil
}

func buildLifetimeCreateOpts(ctx context.Context, set types.Set) (ikepolicies.LifetimeCreateOpts, diag.Diagnostics) {
	opts := ikepolicies.LifetimeCreateOpts{}

	if set.IsNull() || set.IsUnknown() {
		return opts, nil
	}

	var elems []ikePolicyV2LifetimeModel

	d := set.ElementsAs(ctx, &elems, false)
	if d.HasError() {
		return opts, d
	}

	for _, e := range elems {
		opts.Units = ikePolicyUnitFromString(e.Units.ValueString())
		opts.Value = int(e.Value.ValueInt64())
	}

	return opts, nil
}

func buildLifetimeUpdateOpts(ctx context.Context, set types.Set) (ikepolicies.LifetimeUpdateOpts, diag.Diagnostics) {
	opts := ikepolicies.LifetimeUpdateOpts{}

	if set.IsNull() || set.IsUnknown() {
		return opts, nil
	}

	var elems []ikePolicyV2LifetimeModel

	d := set.ElementsAs(ctx, &elems, false)
	if d.HasError() {
		return opts, d
	}

	for _, e := range elems {
		opts.Units = ikePolicyUnitFromString(e.Units.ValueString())
		opts.Value = int(e.Value.ValueInt64())
	}

	return opts, nil
}

func ikePolicyUnitFromString(v string) ikepolicies.Unit {
	switch v {
	case "kilobytes":
		return ikepolicies.UnitKilobytes
	case "seconds":
		return ikepolicies.UnitSeconds
	}

	return ""
}

// waitForIKEPolicyActive polls the API until the policy is reachable (i.e. has
// transitioned from PENDING_CREATE / PENDING_UPDATE to ACTIVE). Mirrors the
// SDKv2 retry.StateChangeConf loop.
func waitForIKEPolicyActive(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Minute)
	}

	for {
		_, err := ikepolicies.Get(ctx, client, id).Extract()
		if err == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for IKE policy %s to become active: %w", id, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// waitForIKEPolicyDeletedFramework drives the gophercloud Delete call and
// retries on transient failures, mirroring the SDKv2 StateChangeConf loop.
func waitForIKEPolicyDeletedFramework(ctx context.Context, client *gophercloud.ServiceClient, id string) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Minute)
	}

	for {
		err := ikepolicies.Delete(ctx, client, id).Err
		if err == nil {
			return nil
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return nil
		}

		if time.Now().After(deadline) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
