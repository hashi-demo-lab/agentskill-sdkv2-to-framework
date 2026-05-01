package openstack

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var _ resource.Resource = &ikePolicyV2Resource{}
var _ resource.ResourceWithImportState = &ikePolicyV2Resource{}

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
	Lifetime              types.Set    `tfsdk:"lifetime"`
	TenantID              types.String `tfsdk:"tenant_id"`
	ValueSpecs            types.Map    `tfsdk:"value_specs"`
}

type ikePolicyV2LifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

var ikePolicyV2LifetimeAttrTypes = map[string]attr.Type{
	"units": types.StringType,
	"value": types.Int64Type,
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
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
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
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
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

	region := plan.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	lifetime := r.expandLifetimeCreateOpts(ctx, plan.Lifetime, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	valueSpecs := r.expandValueSpecs(ctx, plan.ValueSpecs, &resp.Diagnostics)
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

	if err := ikePolicyV2WaitForActive(ctx, networkingClient, policy.ID, 10*time.Minute); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", policy.ID),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)

	r.readIntoModel(ctx, networkingClient, policy.ID, region, &plan, &resp.Diagnostics)
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

	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", state.ID.ValueString())

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

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
		resp.Diagnostics.AddError("Error retrieving IKE policy", err.Error())
		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", state.ID.ValueString(), policy)

	r.readIntoModel(ctx, networkingClient, policy.ID, region, &state, &resp.Diagnostics)
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

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

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
		lifetime := r.expandLifetimeUpdateOpts(ctx, plan.Lifetime, &resp.Diagnostics)
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
			resp.Diagnostics.AddError("Error updating IKE policy", err.Error())
			return
		}

		if err := ikePolicyV2WaitForActive(ctx, networkingClient, state.ID.ValueString(), 10*time.Minute); err != nil {
			resp.Diagnostics.AddError("Error waiting for IKE policy to become active after update", err.Error())
			return
		}
	}

	plan.ID = state.ID

	r.readIntoModel(ctx, networkingClient, state.ID.ValueString(), region, &plan, &resp.Diagnostics)
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

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	if err := ikepolicies.Delete(ctx, networkingClient, state.ID.ValueString()).Err; err != nil {
		resp.Diagnostics.AddError("Error deleting IKE policy", err.Error())
		return
	}

	deadline := time.Now().Add(10 * time.Minute)
	for {
		_, err := ikepolicies.Get(ctx, networkingClient, state.ID.ValueString()).Extract()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return
			}
			resp.Diagnostics.AddError("Error checking IKE policy deletion", err.Error())
			return
		}
		if time.Now().After(deadline) {
			resp.Diagnostics.AddError("Timeout waiting for IKE policy deletion", state.ID.ValueString())
			return
		}
		time.Sleep(2 * time.Second)
	}
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	var state ikePolicyV2Model
	state.ID = types.StringValue(req.ID)

	region := r.config.Region

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	r.readIntoModel(ctx, networkingClient, req.ID, region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ikePolicyV2Resource) readIntoModel(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string, region string, model *ikePolicyV2Model, diagnostics *diag.Diagnostics) {
	policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
	if err != nil {
		diagnostics.AddError("Error reading IKE policy", err.Error())
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
	model.Region = types.StringValue(region)

	lifetimeObj, diags := types.ObjectValue(ikePolicyV2LifetimeAttrTypes, map[string]attr.Value{
		"units": types.StringValue(string(policy.Lifetime.Units)),
		"value": types.Int64Value(int64(policy.Lifetime.Value)),
	})
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}

	lifetimeSet, diags := types.SetValue(types.ObjectType{AttrTypes: ikePolicyV2LifetimeAttrTypes}, []attr.Value{lifetimeObj})
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}

	model.Lifetime = lifetimeSet
}

func (r *ikePolicyV2Resource) expandLifetimeCreateOpts(ctx context.Context, lifetimeSet types.Set, diagnostics *diag.Diagnostics) ikepolicies.LifetimeCreateOpts {
	opts := ikepolicies.LifetimeCreateOpts{}

	if lifetimeSet.IsNull() || lifetimeSet.IsUnknown() {
		return opts
	}

	var lifetimes []ikePolicyV2LifetimeModel
	diagnostics.Append(lifetimeSet.ElementsAs(ctx, &lifetimes, false)...)
	if diagnostics.HasError() {
		return opts
	}

	for _, lt := range lifetimes {
		opts.Units = resourceIKEPolicyV2Unit(lt.Units.ValueString())
		opts.Value = int(lt.Value.ValueInt64())
	}

	return opts
}

func (r *ikePolicyV2Resource) expandLifetimeUpdateOpts(ctx context.Context, lifetimeSet types.Set, diagnostics *diag.Diagnostics) ikepolicies.LifetimeUpdateOpts {
	opts := ikepolicies.LifetimeUpdateOpts{}

	if lifetimeSet.IsNull() || lifetimeSet.IsUnknown() {
		return opts
	}

	var lifetimes []ikePolicyV2LifetimeModel
	diagnostics.Append(lifetimeSet.ElementsAs(ctx, &lifetimes, false)...)
	if diagnostics.HasError() {
		return opts
	}

	for _, lt := range lifetimes {
		opts.Units = resourceIKEPolicyV2Unit(lt.Units.ValueString())
		opts.Value = int(lt.Value.ValueInt64())
	}

	return opts
}

func (r *ikePolicyV2Resource) expandValueSpecs(ctx context.Context, valueSpecs types.Map, diagnostics *diag.Diagnostics) map[string]string {
	if valueSpecs.IsNull() || valueSpecs.IsUnknown() {
		return nil
	}

	result := make(map[string]string)
	diagnostics.Append(valueSpecs.ElementsAs(ctx, &result, false)...)
	return result
}

// ikePolicyV2WaitForActive polls until the IKE policy is in ACTIVE state or timeout expires.
func ikePolicyV2WaitForActive(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
		if err == nil && policy != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for IKE policy %s to become active", id)
		}
		time.Sleep(2 * time.Second)
	}
}
