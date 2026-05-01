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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

type ikePolicyV2Model struct {
	ID                    types.String `tfsdk:"id"`
	Region                types.String `tfsdk:"region"`
	Name                  types.String `tfsdk:"name"`
	Description           types.String `tfsdk:"description"`
	AuthAlgorithm         types.String `tfsdk:"auth_algorithm"`
	EncryptionAlgorithm   types.String `tfsdk:"encryption_algorithm"`
	PFS                   types.String `tfsdk:"pfs"`
	Phase1NegotiationMode types.String `tfsdk:"phase1_negotiation_mode"`
	IKEVersion            types.String `tfsdk:"ike_version"`
	TenantID              types.String `tfsdk:"tenant_id"`
	ValueSpecs            types.Map    `tfsdk:"value_specs"`
	Lifetime              []ikePolicyV2LifetimeModel `tfsdk:"lifetime"`
}

type ikePolicyV2LifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
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
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
			"auth_algorithm": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("sha1"),
			},
			"encryption_algorithm": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("aes-128"),
			},
			"pfs": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("group5"),
			},
			"phase1_negotiation_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("main"),
			},
			"ike_version": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("v1"),
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
		},
	}
}

func (r *ikePolicyV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected provider data type",
			fmt.Sprintf("expected *Config, got %T", req.ProviderData),
		)
		return
	}
	r.config = config
}

func (r *ikePolicyV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ikePolicyV2Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkingClient, err := r.config.NetworkingV2Client(ctx, plan.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	// Convert value_specs map
	valueSpecs := make(map[string]string)
	if !plan.ValueSpecs.IsNull() && !plan.ValueSpecs.IsUnknown() {
		for k, v := range plan.ValueSpecs.Elements() {
			if sv, ok := v.(types.String); ok {
				valueSpecs[k] = sv.ValueString()
			}
		}
	}

	// Build lifetime opts
	lifetime := ikePolicyV2LifetimeCreateOptsFromModel(plan.Lifetime)

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
		resp.Diagnostics.AddError("Error creating openstack_vpnaas_ike_policy_v2", err.Error())
		return
	}

	_, err = waitForIKEPolicyV2State(
		ctx,
		waitForIKEPolicyCreationV2(ctx, networkingClient, policy.ID),
		[]string{"PENDING_CREATE"},
		[]string{"ACTIVE"},
		2*time.Second,
		10*time.Minute,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", policy.ID),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)

	// Read back to populate computed fields
	r.readIntoModel(ctx, networkingClient, policy.ID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ikePolicyV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ikePolicyV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkingClient, err := r.config.NetworkingV2Client(ctx, state.Region.ValueString())
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
			fmt.Sprintf("Error reading openstack_vpnaas_ike_policy_v2 %s", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", state.ID.ValueString(), policy)

	state.Name = types.StringValue(policy.Name)
	state.Description = types.StringValue(policy.Description)
	state.AuthAlgorithm = types.StringValue(policy.AuthAlgorithm)
	state.EncryptionAlgorithm = types.StringValue(policy.EncryptionAlgorithm)
	state.TenantID = types.StringValue(policy.TenantID)
	state.PFS = types.StringValue(policy.PFS)
	state.Phase1NegotiationMode = types.StringValue(policy.Phase1NegotiationMode)
	state.IKEVersion = types.StringValue(policy.IKEVersion)
	// Region stays as-is from state (set on create, immutable)

	// Set lifetime block
	state.Lifetime = []ikePolicyV2LifetimeModel{
		{
			Units: types.StringValue(string(policy.Lifetime.Units)),
			Value: types.Int64Value(int64(policy.Lifetime.Value)),
		},
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *ikePolicyV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ikePolicyV2Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkingClient, err := r.config.NetworkingV2Client(ctx, state.Region.ValueString())
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

	if !lifetimeModelsEqual(plan.Lifetime, state.Lifetime) {
		lifetime := ikePolicyV2LifetimeUpdateOptsFromModel(plan.Lifetime)
		opts.Lifetime = &lifetime
		hasChange = true
	}

	log.Printf("[DEBUG] Updating IKE policy with id %s: %#v", state.ID.ValueString(), opts)

	if hasChange {
		err = ikepolicies.Update(ctx, networkingClient, state.ID.ValueString(), opts).Err
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error updating openstack_vpnaas_ike_policy_v2 %s", state.ID.ValueString()),
				err.Error(),
			)
			return
		}

		_, err = waitForIKEPolicyV2State(
			ctx,
			waitForIKEPolicyUpdateV2(ctx, networkingClient, state.ID.ValueString()),
			[]string{"PENDING_UPDATE"},
			[]string{"ACTIVE"},
			2*time.Second,
			10*time.Minute,
		)
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	// Read back to get updated state
	plan.ID = state.ID
	plan.Region = state.Region
	r.readIntoModel(ctx, networkingClient, state.ID.ValueString(), &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ikePolicyV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ikePolicyV2Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Destroy IKE policy: %s", state.ID.ValueString())

	networkingClient, err := r.config.NetworkingV2Client(ctx, state.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	_, err = waitForIKEPolicyV2State(
		ctx,
		waitForIKEPolicyDeletionV2(ctx, networkingClient, state.ID.ValueString()),
		[]string{"ACTIVE"},
		[]string{"DELETED"},
		2*time.Second,
		10*time.Minute,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting openstack_vpnaas_ike_policy_v2 %s", state.ID.ValueString()),
			err.Error(),
		)
	}
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readIntoModel fetches the policy from the API and populates the model's computed fields.
func (r *ikePolicyV2Resource) readIntoModel(ctx context.Context, client *gophercloud.ServiceClient, id string, m *ikePolicyV2Model, diagnostics *diag.Diagnostics) {
	policy, err := ikepolicies.Get(ctx, client, id).Extract()
	if err != nil {
		diagnostics.AddError(
			fmt.Sprintf("Error reading openstack_vpnaas_ike_policy_v2 %s", id),
			err.Error(),
		)
		return
	}
	m.Name = types.StringValue(policy.Name)
	m.Description = types.StringValue(policy.Description)
	m.AuthAlgorithm = types.StringValue(policy.AuthAlgorithm)
	m.EncryptionAlgorithm = types.StringValue(policy.EncryptionAlgorithm)
	m.TenantID = types.StringValue(policy.TenantID)
	m.PFS = types.StringValue(policy.PFS)
	m.Phase1NegotiationMode = types.StringValue(policy.Phase1NegotiationMode)
	m.IKEVersion = types.StringValue(policy.IKEVersion)
	m.Lifetime = []ikePolicyV2LifetimeModel{
		{
			Units: types.StringValue(string(policy.Lifetime.Units)),
			Value: types.Int64Value(int64(policy.Lifetime.Value)),
		},
	}
	// Region is preserved from the caller (plan or state) as it's not returned by the API
}

// waitForIKEPolicyV2State is the inline replacement for retry.StateChangeConf.
func waitForIKEPolicyV2State(
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

func waitForIKEPolicyDeletionV2(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		err := ikepolicies.Delete(ctx, networkingClient, id).Err
		if err == nil {
			return "", "DELETED", nil
		}
		return nil, "ACTIVE", err
	}
}

func waitForIKEPolicyCreationV2(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_CREATE", nil
		}
		return policy, "ACTIVE", nil
	}
}

func waitForIKEPolicyUpdateV2(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_UPDATE", nil
		}
		return policy, "ACTIVE", nil
	}
}

func ikePolicyV2LifetimeCreateOptsFromModel(lifetime []ikePolicyV2LifetimeModel) ikepolicies.LifetimeCreateOpts {
	opts := ikepolicies.LifetimeCreateOpts{}
	for _, l := range lifetime {
		opts.Units = resourceIKEPolicyV2Unit(l.Units.ValueString())
		opts.Value = int(l.Value.ValueInt64())
	}
	return opts
}

func ikePolicyV2LifetimeUpdateOptsFromModel(lifetime []ikePolicyV2LifetimeModel) ikepolicies.LifetimeUpdateOpts {
	opts := ikepolicies.LifetimeUpdateOpts{}
	for _, l := range lifetime {
		opts.Units = resourceIKEPolicyV2Unit(l.Units.ValueString())
		opts.Value = int(l.Value.ValueInt64())
	}
	return opts
}

func lifetimeModelsEqual(a, b []ikePolicyV2LifetimeModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Units != b[i].Units || a[i].Value != b[i].Value {
			return false
		}
	}
	return true
}
