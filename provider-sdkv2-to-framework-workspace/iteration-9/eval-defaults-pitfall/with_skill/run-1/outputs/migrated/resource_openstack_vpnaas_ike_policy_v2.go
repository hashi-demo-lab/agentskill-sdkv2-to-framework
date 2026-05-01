package openstack

import (
	"context"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure interface compliance at compile time.
var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// NewIKEPolicyV2Resource returns a new framework resource for openstack_vpnaas_ike_policy_v2.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

// ikePolicyV2Model is the typed model used for all state/plan operations.
type ikePolicyV2Model struct {
	ID                   types.String `tfsdk:"id"`
	Region               types.String `tfsdk:"region"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	AuthAlgorithm        types.String `tfsdk:"auth_algorithm"`
	EncryptionAlgorithm  types.String `tfsdk:"encryption_algorithm"`
	PFS                  types.String `tfsdk:"pfs"`
	Phase1NegotiationMode types.String `tfsdk:"phase1_negotiation_mode"`
	IKEVersion           types.String `tfsdk:"ike_version"`
	Lifetime             []ikePolicyV2LifetimeModel `tfsdk:"lifetime"`
	TenantID             types.String `tfsdk:"tenant_id"`
	ValueSpecs           types.Map    `tfsdk:"value_specs"`
}

// ikePolicyV2LifetimeModel represents a single lifetime block.
type ikePolicyV2LifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

func (r *ikePolicyV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpnaas_ike_policy_v2"
}

func (r *ikePolicyV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

	lifetime := ikePolicyV2LifetimeCreateOptsFromModel(plan.Lifetime)
	valueSpecs := ikePolicyV2ValueSpecsFromModel(plan.ValueSpecs)

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

	if _, err = waitForState(ctx,
		waitForIKEPolicyV2Creation(ctx, networkingClient, policy.ID),
		[]string{"PENDING_CREATE"}, []string{"ACTIVE"},
		2*time.Second, 10*time.Minute,
	); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", policy.ID),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)
	r.refreshModel(ctx, networkingClient, policy.ID, &plan, &resp.Diagnostics)
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

	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", state.ID.ValueString())

	networkingClient, err := r.config.NetworkingV2Client(ctx, state.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	policy, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, 404) {
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

	ikePolicyV2SetModelFromAPI(policy, &state)
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
	hasChange := false

	if plan.Name != state.Name {
		name := plan.Name.ValueString()
		opts.Name = &name
		hasChange = true
	}

	if plan.Description != state.Description {
		description := plan.Description.ValueString()
		opts.Description = &description
		hasChange = true
	}

	if plan.PFS != state.PFS {
		opts.PFS = ikepolicies.PFS(plan.PFS.ValueString())
		hasChange = true
	}

	if plan.AuthAlgorithm != state.AuthAlgorithm {
		opts.AuthAlgorithm = ikepolicies.AuthAlgorithm(plan.AuthAlgorithm.ValueString())
		hasChange = true
	}

	if plan.EncryptionAlgorithm != state.EncryptionAlgorithm {
		opts.EncryptionAlgorithm = ikepolicies.EncryptionAlgorithm(plan.EncryptionAlgorithm.ValueString())
		hasChange = true
	}

	if plan.Phase1NegotiationMode != state.Phase1NegotiationMode {
		opts.Phase1NegotiationMode = ikepolicies.Phase1NegotiationMode(plan.Phase1NegotiationMode.ValueString())
		hasChange = true
	}

	if plan.IKEVersion != state.IKEVersion {
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
		if err = ikepolicies.Update(ctx, networkingClient, state.ID.ValueString(), opts).Err; err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error updating openstack_vpnaas_ike_policy_v2 %s", state.ID.ValueString()),
				err.Error(),
			)
			return
		}

		if _, err = waitForState(ctx,
			waitForIKEPolicyV2Update(ctx, networkingClient, state.ID.ValueString()),
			[]string{"PENDING_UPDATE"}, []string{"ACTIVE"},
			2*time.Second, 10*time.Minute,
		); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	plan.ID = state.ID
	plan.Region = state.Region
	r.refreshModel(ctx, networkingClient, state.ID.ValueString(), &plan, &resp.Diagnostics)
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

	if _, err = waitForState(ctx,
		waitForIKEPolicyV2Deletion(ctx, networkingClient, state.ID.ValueString()),
		[]string{"ACTIVE"}, []string{"DELETED"},
		2*time.Second, 10*time.Minute,
	); err != nil {
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

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// refreshModel re-reads the policy from the API and populates model fields.
func (r *ikePolicyV2Resource) refreshModel(
	ctx context.Context,
	networkingClient *gophercloud.ServiceClient,
	id string,
	model *ikePolicyV2Model,
	diagnostics *diag.Diagnostics,
) {
	policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
	if err != nil {
		diagnostics.AddError(
			fmt.Sprintf("Error reading openstack_vpnaas_ike_policy_v2 %s after write", id),
			err.Error(),
		)
		return
	}
	ikePolicyV2SetModelFromAPI(policy, model)
}

// ikePolicyV2SetModelFromAPI copies API response fields into the typed model.
func ikePolicyV2SetModelFromAPI(policy *ikepolicies.Policy, model *ikePolicyV2Model) {
	model.Name = types.StringValue(policy.Name)
	model.Description = types.StringValue(policy.Description)
	model.AuthAlgorithm = types.StringValue(policy.AuthAlgorithm)
	model.EncryptionAlgorithm = types.StringValue(policy.EncryptionAlgorithm)
	model.TenantID = types.StringValue(policy.TenantID)
	model.PFS = types.StringValue(policy.PFS)
	model.Phase1NegotiationMode = types.StringValue(policy.Phase1NegotiationMode)
	model.IKEVersion = types.StringValue(policy.IKEVersion)

	// Lifetime is returned as a single nested object; we store it as a slice of at most one.
	model.Lifetime = []ikePolicyV2LifetimeModel{
		{
			Units: types.StringValue(string(policy.Lifetime.Units)),
			Value: types.Int64Value(int64(policy.Lifetime.Value)),
		},
	}
}

// ikePolicyV2LifetimeCreateOptsFromModel converts the typed model slice into gophercloud opts.
func ikePolicyV2LifetimeCreateOptsFromModel(lifetime []ikePolicyV2LifetimeModel) ikepolicies.LifetimeCreateOpts {
	opts := ikepolicies.LifetimeCreateOpts{}
	for _, l := range lifetime {
		opts.Units = ikePolicyV2UnitFromString(l.Units.ValueString())
		opts.Value = int(l.Value.ValueInt64())
	}
	return opts
}

// ikePolicyV2LifetimeUpdateOptsFromModel converts the typed model slice into update opts.
func ikePolicyV2LifetimeUpdateOptsFromModel(lifetime []ikePolicyV2LifetimeModel) ikepolicies.LifetimeUpdateOpts {
	opts := ikepolicies.LifetimeUpdateOpts{}
	for _, l := range lifetime {
		opts.Units = ikePolicyV2UnitFromString(l.Units.ValueString())
		opts.Value = int(l.Value.ValueInt64())
	}
	return opts
}

// ikePolicyV2UnitFromString converts a string into a gophercloud Unit.
func ikePolicyV2UnitFromString(v string) ikepolicies.Unit {
	switch v {
	case "kilobytes":
		return ikepolicies.UnitKilobytes
	case "seconds":
		return ikepolicies.UnitSeconds
	}
	return ""
}

// ikePolicyV2ValueSpecsFromModel extracts value_specs from the types.Map.
func ikePolicyV2ValueSpecsFromModel(m types.Map) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := make(map[string]string, len(m.Elements()))
	for k, v := range m.Elements() {
		if sv, ok := v.(types.String); ok {
			out[k] = sv.ValueString()
		}
	}
	return out
}

// lifetimeModelsEqual compares two lifetime slices for equality.
func lifetimeModelsEqual(a, b []ikePolicyV2LifetimeModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// waitForState is a minimal context-aware poll loop replacing retry.StateChangeConf.
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

func waitForIKEPolicyV2Deletion(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		err := ikepolicies.Delete(ctx, networkingClient, id).Err
		if err == nil {
			return "", "DELETED", nil
		}
		return nil, "ACTIVE", err
	}
}

func waitForIKEPolicyV2Creation(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_CREATE", nil
		}
		return policy, "ACTIVE", nil
	}
}

func waitForIKEPolicyV2Update(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err != nil {
			return "", "PENDING_UPDATE", nil
		}
		return policy, "ACTIVE", nil
	}
}
