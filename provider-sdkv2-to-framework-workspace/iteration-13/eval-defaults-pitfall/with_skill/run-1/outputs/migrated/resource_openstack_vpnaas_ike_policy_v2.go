package openstack

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"slices"
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// NewIKEPolicyV2Resource is the framework constructor for the
// openstack_vpnaas_ike_policy_v2 resource.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

// ikePolicyV2Model is the typed model for the resource state/plan/config.
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

// ikePolicyLifetimeModel mirrors the lifetime nested block.
type ikePolicyLifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
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
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
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
			"Unexpected provider data",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
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

	region := frameworkGetRegion(plan.Region, r.config)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	lifetime, ldiags := ikePolicyLifetimeFromModel(ctx, plan.Lifetime)
	resp.Diagnostics.Append(ldiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	valueSpecs, vdiags := mapStringFromTerraform(ctx, plan.ValueSpecs)
	resp.Diagnostics.Append(vdiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	authAlgorithm := ikepolicies.AuthAlgorithm(plan.AuthAlgorithm.ValueString())
	encryptionAlgorithm := ikepolicies.EncryptionAlgorithm(plan.EncryptionAlgorithm.ValueString())
	pfs := ikepolicies.PFS(plan.PFS.ValueString())
	ikeVersion := ikepolicies.IKEVersion(plan.IKEVersion.ValueString())
	phase1NegotiationMode := ikepolicies.Phase1NegotiationMode(plan.Phase1NegotiationMode.ValueString())

	opts := IKEPolicyCreateOpts{
		ikepolicies.CreateOpts{
			Name:                  plan.Name.ValueString(),
			Description:           plan.Description.ValueString(),
			TenantID:              plan.TenantID.ValueString(),
			Lifetime:              lifetime,
			AuthAlgorithm:         authAlgorithm,
			EncryptionAlgorithm:   encryptionAlgorithm,
			PFS:                   pfs,
			IKEVersion:            ikeVersion,
			Phase1NegotiationMode: phase1NegotiationMode,
		},
		valueSpecs,
	}
	log.Printf("[DEBUG] Create IKE policy: %#v", opts)

	policy, err := ikepolicies.Create(ctx, networkingClient, opts).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error creating IKE policy", err.Error())

		return
	}

	if _, err := waitForIKEPolicyState(
		ctx,
		waitForIKEPolicyCreationFunc(ctx, networkingClient, policy.ID),
		[]string{"PENDING_CREATE"},
		[]string{"ACTIVE"},
		2*time.Second,
		createTimeout,
	); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", policy.ID),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)

	if diags := r.refreshFromAPI(ctx, &plan, networkingClient, region); diags.HasError() {
		resp.Diagnostics.Append(diags...)

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

	region := frameworkGetRegion(state.Region, r.config)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", state.ID.ValueString())

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)

			return
		}

		resp.Diagnostics.AddError("Error retrieving IKE policy", err.Error())

		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", state.ID.ValueString(), policy)

	if diags := setIKEPolicyState(ctx, &state, policy, region); diags.HasError() {
		resp.Diagnostics.Append(diags...)

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

	region := frameworkGetRegion(state.Region, r.config)

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
		lifetimeUpdate, ldiags := ikePolicyLifetimeUpdateFromModel(ctx, plan.Lifetime)
		resp.Diagnostics.Append(ldiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		opts.Lifetime = lifetimeUpdate
		hasChange = true
	}

	log.Printf("[DEBUG] Updating IKE policy with id %s: %#v", state.ID.ValueString(), opts)

	if hasChange {
		if err := ikepolicies.Update(ctx, networkingClient, state.ID.ValueString(), opts).Err; err != nil {
			resp.Diagnostics.AddError("Error updating IKE policy", err.Error())

			return
		}

		if _, err := waitForIKEPolicyState(
			ctx,
			waitForIKEPolicyUpdateFunc(ctx, networkingClient, state.ID.ValueString()),
			[]string{"PENDING_UPDATE"},
			[]string{"ACTIVE"},
			2*time.Second,
			updateTimeout,
		); err != nil {
			resp.Diagnostics.AddError("Error waiting for IKE policy update", err.Error())

			return
		}
	}

	plan.ID = state.ID

	if diags := r.refreshFromAPI(ctx, &plan, networkingClient, region); diags.HasError() {
		resp.Diagnostics.Append(diags...)

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

	region := frameworkGetRegion(state.Region, r.config)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())

		return
	}

	log.Printf("[DEBUG] Destroy IKE policy: %s", state.ID.ValueString())

	if _, err := waitForIKEPolicyState(
		ctx,
		waitForIKEPolicyDeletionFunc(ctx, networkingClient, state.ID.ValueString()),
		[]string{"ACTIVE"},
		[]string{"DELETED"},
		2*time.Second,
		deleteTimeout,
	); err != nil {
		resp.Diagnostics.AddError("Error deleting IKE policy", err.Error())

		return
	}
}

// refreshFromAPI re-reads the policy and populates the model with API values.
func (r *ikePolicyV2Resource) refreshFromAPI(
	ctx context.Context,
	model *ikePolicyV2Model,
	networkingClient *gophercloud.ServiceClient,
	region string,
) (diags diag.Diagnostics) {
	policy, err := ikepolicies.Get(ctx, networkingClient, model.ID.ValueString()).Extract()
	if err != nil {
		diags.AddError("Error retrieving IKE policy", err.Error())

		return diags
	}

	if d := setIKEPolicyState(ctx, model, policy, region); d.HasError() {
		diags.Append(d...)
	}

	return diags
}

// setIKEPolicyState writes API-returned values onto the model.
func setIKEPolicyState(ctx context.Context, model *ikePolicyV2Model, policy *ikepolicies.Policy, region string) diag.Diagnostics {
	var diags diag.Diagnostics

	model.ID = types.StringValue(policy.ID)
	model.Name = types.StringValue(policy.Name)
	model.Description = types.StringValue(policy.Description)
	model.AuthAlgorithm = types.StringValue(string(policy.AuthAlgorithm))
	model.EncryptionAlgorithm = types.StringValue(string(policy.EncryptionAlgorithm))
	model.TenantID = types.StringValue(policy.TenantID)
	model.PFS = types.StringValue(string(policy.PFS))
	model.Phase1NegotiationMode = types.StringValue(string(policy.Phase1NegotiationMode))
	model.IKEVersion = types.StringValue(string(policy.IKEVersion))
	model.Region = types.StringValue(region)

	lifetimeSet, lDiags := ikePolicyLifetimeToSet(ctx, policy.Lifetime)
	diags.Append(lDiags...)
	model.Lifetime = lifetimeSet

	return diags
}

// ikePolicyLifetimeFromModel extracts a (single) lifetime from the model's
// Set-typed lifetime attribute and returns SDK create-opts.
func ikePolicyLifetimeFromModel(ctx context.Context, set types.Set) (*ikepolicies.LifetimeCreateOpts, diag.Diagnostics) {
	var diags diag.Diagnostics

	if set.IsNull() || set.IsUnknown() {
		return &ikepolicies.LifetimeCreateOpts{}, diags
	}

	var lifetimes []ikePolicyLifetimeModel

	d := set.ElementsAs(ctx, &lifetimes, false)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	out := &ikepolicies.LifetimeCreateOpts{}

	for _, lt := range lifetimes {
		out.Units = resourceIKEPolicyV2Unit(lt.Units.ValueString())
		out.Value = int(lt.Value.ValueInt64())
	}

	return out, diags
}

func ikePolicyLifetimeUpdateFromModel(ctx context.Context, set types.Set) (*ikepolicies.LifetimeUpdateOpts, diag.Diagnostics) {
	var diags diag.Diagnostics

	if set.IsNull() || set.IsUnknown() {
		return &ikepolicies.LifetimeUpdateOpts{}, diags
	}

	var lifetimes []ikePolicyLifetimeModel

	d := set.ElementsAs(ctx, &lifetimes, false)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	out := &ikepolicies.LifetimeUpdateOpts{}

	for _, lt := range lifetimes {
		out.Units = resourceIKEPolicyV2Unit(lt.Units.ValueString())
		out.Value = int(lt.Value.ValueInt64())
	}

	return out, diags
}

func ikePolicyLifetimeToSet(ctx context.Context, lifetime ikepolicies.Lifetime) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics

	elemTypes := map[string]attr.Type{
		"units": types.StringType,
		"value": types.Int64Type,
	}
	objType := types.ObjectType{AttrTypes: elemTypes}

	value, d := types.ObjectValue(elemTypes, map[string]attr.Value{
		"units": types.StringValue(string(lifetime.Units)),
		"value": types.Int64Value(int64(lifetime.Value)),
	})
	diags.Append(d...)

	if diags.HasError() {
		return types.SetNull(objType), diags
	}

	set, d := types.SetValue(objType, []attr.Value{value})
	diags.Append(d...)

	if diags.HasError() {
		return types.SetNull(objType), diags
	}

	return set, diags
}

// mapStringFromTerraform converts a types.Map into map[string]string.
func mapStringFromTerraform(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	if m.IsNull() || m.IsUnknown() {
		return map[string]string{}, diags
	}

	out := make(map[string]string, len(m.Elements()))
	d := m.ElementsAs(ctx, &out, false)
	diags.Append(d...)

	return out, diags
}

// frameworkGetRegion mirrors the SDKv2 GetRegion helper for typed values.
func frameworkGetRegion(region types.String, cfg *Config) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}

	if cfg != nil {
		return cfg.Region
	}

	return ""
}

// waitForIKEPolicyState replaces SDKv2 retry.StateChangeConf — context-aware
// polling between the pending and target states.
func waitForIKEPolicyState(
	ctx context.Context,
	refresh func() (any, string, error),
	pending, target []string,
	pollInterval, timeout time.Duration,
) (any, error) {
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

func waitForIKEPolicyDeletionFunc(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		err := ikepolicies.Delete(ctx, networkingClient, id).Err
		if err == nil {
			return "", "DELETED", nil
		}

		return nil, "ACTIVE", err
	}
}

func waitForIKEPolicyCreationFunc(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_CREATE", nil
		}

		return policy, "ACTIVE", nil
	}
}

func waitForIKEPolicyUpdateFunc(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_UPDATE", nil
		}

		return policy, "ACTIVE", nil
	}
}

// resourceIKEPolicyV2Unit maps a string into an SDK Unit constant.
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
