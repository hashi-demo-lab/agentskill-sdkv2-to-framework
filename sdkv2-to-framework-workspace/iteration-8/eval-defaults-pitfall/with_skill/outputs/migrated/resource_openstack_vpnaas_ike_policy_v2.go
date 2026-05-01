package openstack

import (
	"context"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithConfigure   = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// NewIKEPolicyV2Resource returns a new instance of the IKE policy resource.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
}

type ikePolicyV2Resource struct {
	config *Config
}

// ikePolicyV2LifetimeModel represents the lifetime nested block.
type ikePolicyV2LifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

// ikePolicyV2Model is the full resource model.
type ikePolicyV2Model struct {
	ID                   types.String               `tfsdk:"id"`
	Region               types.String               `tfsdk:"region"`
	Name                 types.String               `tfsdk:"name"`
	Description          types.String               `tfsdk:"description"`
	AuthAlgorithm        types.String               `tfsdk:"auth_algorithm"`
	EncryptionAlgorithm  types.String               `tfsdk:"encryption_algorithm"`
	PFS                  types.String               `tfsdk:"pfs"`
	Phase1NegotiationMode types.String              `tfsdk:"phase1_negotiation_mode"`
	IKEVersion           types.String               `tfsdk:"ike_version"`
	Lifetime             []ikePolicyV2LifetimeModel `tfsdk:"lifetime"`
	TenantID             types.String               `tfsdk:"tenant_id"`
	ValueSpecs           types.Map                  `tfsdk:"value_specs"`
}

func (r *ikePolicyV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpnaas_ike_policy_v2"
}

func (r *ikePolicyV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a VPNaaS IKE Policy resource within OpenStack.",
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
					mapRequiresReplace(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"lifetime": schema.SetNestedBlock{
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
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
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
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

	lifetime := ikePolicyV2LifetimeCreateOpts(plan.Lifetime)
	authAlgorithm := ikepolicies.AuthAlgorithm(plan.AuthAlgorithm.ValueString())
	encryptionAlgorithm := ikepolicies.EncryptionAlgorithm(plan.EncryptionAlgorithm.ValueString())
	pfs := ikepolicies.PFS(plan.PFS.ValueString())
	ikeVersion := ikepolicies.IKEVersion(plan.IKEVersion.ValueString())
	phase1NegotiationMode := ikepolicies.Phase1NegotiationMode(plan.Phase1NegotiationMode.ValueString())

	valueSpecs := ikePolicyV2MapValueSpecs(plan.ValueSpecs)

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

	_, waitErr := waitForState(ctx,
		waitForIKEPolicyV2Creation(ctx, networkingClient, policy.ID),
		[]string{"PENDING_CREATE"},
		[]string{"ACTIVE"},
		2*time.Second,
		10*time.Minute,
	)
	if waitErr != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", policy.ID),
			waitErr.Error(),
		)
		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)

	// Re-read to pick up server-assigned defaults.
	r.readIntoModel(ctx, networkingClient, policy.ID, &plan, resp.Diagnostics)
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

	networkingClient, err := r.config.NetworkingV2Client(ctx, state.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	r.readIntoModel(ctx, networkingClient, state.ID.ValueString(), &state, resp.Diagnostics)
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

	networkingClient, err := r.config.NetworkingV2Client(ctx, plan.Region.ValueString())
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
		lifetime := ikePolicyV2LifetimeUpdateOpts(plan.Lifetime)
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

		_, waitErr := waitForState(ctx,
			waitForIKEPolicyV2Update(ctx, networkingClient, state.ID.ValueString()),
			[]string{"PENDING_UPDATE"},
			[]string{"ACTIVE"},
			2*time.Second,
			10*time.Minute,
		)
		if waitErr != nil {
			resp.Diagnostics.AddError("Error waiting for IKE policy update", waitErr.Error())
			return
		}
	}

	plan.ID = state.ID
	r.readIntoModel(ctx, networkingClient, state.ID.ValueString(), &plan, resp.Diagnostics)
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

	log.Printf("[DEBUG] Destroy IKE policy: %s", state.ID.ValueString())

	networkingClient, err := r.config.NetworkingV2Client(ctx, state.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	_, waitErr := waitForState(ctx,
		waitForIKEPolicyV2Deletion(ctx, networkingClient, state.ID.ValueString()),
		[]string{"ACTIVE"},
		[]string{"DELETED"},
		2*time.Second,
		10*time.Minute,
	)
	if waitErr != nil {
		resp.Diagnostics.AddError("Error deleting IKE policy", waitErr.Error())
	}
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readIntoModel reads the policy from the API and populates the model.
func (r *ikePolicyV2Resource) readIntoModel(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	id string,
	model *ikePolicyV2Model,
	diags interface{ HasError() bool },
) {
	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", id)

	policy, err := ikepolicies.Get(ctx, client, id).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, 404) {
			log.Printf("[WARN] IKE policy %s not found; removing from state", id)
			return
		}
		// We don't have a direct reference to resp.Diagnostics here, so we use a
		// type assertion to append to it.
		type diagAppender interface {
			AddError(summary, detail string)
		}
		if d, ok := diags.(diagAppender); ok {
			d.AddError("Error reading IKE policy", fmt.Sprintf("%s %s: %s", "IKE policy", id, err.Error()))
		}
		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", id, policy)

	model.Name = types.StringValue(policy.Name)
	model.Description = types.StringValue(policy.Description)
	model.AuthAlgorithm = types.StringValue(policy.AuthAlgorithm)
	model.EncryptionAlgorithm = types.StringValue(policy.EncryptionAlgorithm)
	model.TenantID = types.StringValue(policy.TenantID)
	model.PFS = types.StringValue(policy.PFS)
	model.Phase1NegotiationMode = types.StringValue(policy.Phase1NegotiationMode)
	model.IKEVersion = types.StringValue(policy.IKEVersion)

	model.Lifetime = []ikePolicyV2LifetimeModel{
		{
			Units: types.StringValue(string(policy.Lifetime.Units)),
			Value: types.Int64Value(int64(policy.Lifetime.Value)),
		},
	}
}

// ---- helpers ----------------------------------------------------------------

func waitForIKEPolicyV2Deletion(ctx context.Context, client *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		err := ikepolicies.Delete(ctx, client, id).Err
		if err == nil {
			return "", "DELETED", nil
		}
		return nil, "ACTIVE", err
	}
}

func waitForIKEPolicyV2Creation(ctx context.Context, client *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, client, id).Extract()
		if err != nil {
			return "", "PENDING_CREATE", nil
		}
		return policy, "ACTIVE", nil
	}
}

func waitForIKEPolicyV2Update(ctx context.Context, client *gophercloud.ServiceClient, id string) func() (any, string, error) {
	return func() (any, string, error) {
		policy, err := ikepolicies.Get(ctx, client, id).Extract()
		if err != nil {
			return "", "PENDING_UPDATE", nil
		}
		return policy, "ACTIVE", nil
	}
}

func ikePolicyV2Unit(v string) ikepolicies.Unit {
	switch v {
	case "kilobytes":
		return ikepolicies.UnitKilobytes
	case "seconds":
		return ikepolicies.UnitSeconds
	}
	return ""
}

func ikePolicyV2LifetimeCreateOpts(lifetime []ikePolicyV2LifetimeModel) ikepolicies.LifetimeCreateOpts {
	opts := ikepolicies.LifetimeCreateOpts{}
	for _, lt := range lifetime {
		opts.Units = ikePolicyV2Unit(lt.Units.ValueString())
		opts.Value = int(lt.Value.ValueInt64())
	}
	return opts
}

func ikePolicyV2LifetimeUpdateOpts(lifetime []ikePolicyV2LifetimeModel) ikepolicies.LifetimeUpdateOpts {
	opts := ikepolicies.LifetimeUpdateOpts{}
	for _, lt := range lifetime {
		opts.Units = ikePolicyV2Unit(lt.Units.ValueString())
		opts.Value = int(lt.Value.ValueInt64())
	}
	return opts
}

func lifetimeModelsEqual(a, b []ikePolicyV2LifetimeModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Units.Equal(b[i].Units) || !a[i].Value.Equal(b[i].Value) {
			return false
		}
	}
	return true
}

// ikePolicyV2MapValueSpecs converts the types.Map value_specs into a map[string]string.
func ikePolicyV2MapValueSpecs(m types.Map) map[string]string {
	result := make(map[string]string)
	for k, v := range m.Elements() {
		if sv, ok := v.(types.String); ok {
			result[k] = sv.ValueString()
		}
	}
	return result
}

// waitForState is the framework replacement for retry.StateChangeConf.WaitForStateContext.
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

// mapRequiresReplace is a plan modifier for MapAttribute that triggers replacement.
type mapRequiresReplaceModifier struct{}

func mapRequiresReplace() planmodifier.Map { return mapRequiresReplaceModifier{} }

func (m mapRequiresReplaceModifier) Description(_ context.Context) string {
	return "If the value of this attribute changes, Terraform will destroy and recreate the resource."
}

func (m mapRequiresReplaceModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m mapRequiresReplaceModifier) PlanModifyMap(_ context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	if req.StateValue.IsNull() {
		return
	}
	if !req.PlanValue.Equal(req.StateValue) {
		resp.RequiresReplace = true
	}
}
