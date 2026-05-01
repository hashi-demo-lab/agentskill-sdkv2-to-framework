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
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &IKEPolicyV2Resource{}
var _ resource.ResourceWithImportState = &IKEPolicyV2Resource{}

func NewIKEPolicyV2Resource() resource.Resource {
	return &IKEPolicyV2Resource{}
}

type IKEPolicyV2Resource struct {
	config *Config
}

type IKEPolicyV2ResourceModel struct {
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

type IKEPolicyV2LifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

var ikePolicyV2LifetimeAttrTypes = map[string]attr.Type{
	"units": types.StringType,
	"value": types.Int64Type,
}

func (r *IKEPolicyV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpnaas_ike_policy_v2"
}

func (r *IKEPolicyV2Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

func (r *IKEPolicyV2Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.config = config
}

func (r *IKEPolicyV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IKEPolicyV2ResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
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

	lifetime := ikePolicyV2LifetimeCreateOptsFromPlan(ctx, plan.Lifetime, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	valueSpecs := ikePolicyV2ValueSpecsFromPlan(ctx, plan.ValueSpecs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := IKEPolicyCreateOpts{
		ikepolicies.CreateOpts{
			Name:                  plan.Name.ValueString(),
			Description:           plan.Description.ValueString(),
			TenantID:              plan.TenantID.ValueString(),
			Lifetime:              lifetime,
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

	if err = ikePolicyV2WaitForActive(ctx, networkingClient, policy.ID, createTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to become active", policy.ID),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	plan.ID = types.StringValue(policy.ID)
	plan.Region = types.StringValue(region)

	r.readInto(ctx, networkingClient, policy.ID, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IKEPolicyV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IKEPolicyV2ResourceModel

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

	r.readInto(ctx, networkingClient, state.ID.ValueString(), region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *IKEPolicyV2Resource) readInto(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string, region string, model *IKEPolicyV2ResourceModel, diagnostics *diag.Diagnostics) {
	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", id)

	policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			model.ID = types.StringNull()
			return
		}
		diagnostics.AddError(fmt.Sprintf("Error retrieving openstack_vpnaas_ike_policy_v2 %s", id), err.Error())
		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", id, policy)

	model.ID = types.StringValue(policy.ID)
	model.Name = types.StringValue(policy.Name)
	model.Description = types.StringValue(policy.Description)
	model.AuthAlgorithm = types.StringValue(policy.AuthAlgorithm)
	model.EncryptionAlgorithm = types.StringValue(policy.EncryptionAlgorithm)
	model.TenantID = types.StringValue(policy.TenantID)
	model.PFS = types.StringValue(policy.PFS)
	model.Phase1NegotiationMode = types.StringValue(policy.Phase1NegotiationMode)
	model.IKEVersion = types.StringValue(policy.IKEVersion)
	model.Region = types.StringValue(region)

	// Set the lifetime.
	lifetimeModel := IKEPolicyV2LifetimeModel{
		Units: types.StringValue(string(policy.Lifetime.Units)),
		Value: types.Int64Value(int64(policy.Lifetime.Value)),
	}

	lifetimeObj, d := types.ObjectValueFrom(ctx, ikePolicyV2LifetimeAttrTypes, lifetimeModel)
	diagnostics.Append(d...)
	if diagnostics.HasError() {
		return
	}

	lifetimeSet, d := types.SetValue(types.ObjectType{AttrTypes: ikePolicyV2LifetimeAttrTypes}, []attr.Value{lifetimeObj})
	diagnostics.Append(d...)
	if diagnostics.HasError() {
		return
	}

	model.Lifetime = lifetimeSet
}

func (r *IKEPolicyV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IKEPolicyV2ResourceModel
	var state IKEPolicyV2ResourceModel

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
		lifetime := ikePolicyV2LifetimeUpdateOptsFromPlan(ctx, plan.Lifetime, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		opts.Lifetime = lifetime
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

		if err = ikePolicyV2WaitForActive(ctx, networkingClient, state.ID.ValueString(), updateTimeout); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to update", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	plan.ID = state.ID
	plan.Region = state.Region

	r.readInto(ctx, networkingClient, state.ID.ValueString(), region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *IKEPolicyV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IKEPolicyV2ResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
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

	if err = ikePolicyV2WaitForDeleted(ctx, networkingClient, state.ID.ValueString(), deleteTimeout); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error waiting for openstack_vpnaas_ike_policy_v2 %s to delete", state.ID.ValueString()),
			err.Error(),
		)
	}
}

func (r *IKEPolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	region := r.config.Region

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	var state IKEPolicyV2ResourceModel
	state.ID = types.StringValue(req.ID)

	r.readInto(ctx, networkingClient, req.ID, region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// --- polling helpers ---

// ikePolicyV2WaitForActive polls until the IKE policy reaches ACTIVE state.
func ikePolicyV2WaitForActive(ctx context.Context, client *gophercloud.ServiceClient, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		policy, err := ikepolicies.Get(ctx, client, id).Extract()
		if err != nil {
			return err
		}
		// The API currently returns a policy immediately — treat any successful
		// Get as ACTIVE. Adjust if the API surfaces an explicit status field.
		if policy != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for openstack_vpnaas_ike_policy_v2 %s to become active", id)
		}
		time.Sleep(2 * time.Second)
	}
}

// ikePolicyV2WaitForDeleted polls until the IKE policy is deleted.
func ikePolicyV2WaitForDeleted(ctx context.Context, client *gophercloud.ServiceClient, id string, timeout time.Duration) error {
	// Attempt the delete first.
	if err := ikepolicies.Delete(ctx, client, id).Err; err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return nil
		}
		return err
	}

	deadline := time.Now().Add(timeout)
	for {
		_, err := ikepolicies.Get(ctx, client, id).Extract()
		if err != nil {
			if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return nil
			}
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for openstack_vpnaas_ike_policy_v2 %s to be deleted", id)
		}
		time.Sleep(2 * time.Second)
	}
}

// --- field helpers ---

func ikePolicyV2Unit(v string) ikepolicies.Unit {
	switch v {
	case "kilobytes":
		return ikepolicies.UnitKilobytes
	case "seconds":
		return ikepolicies.UnitSeconds
	}

	return ""
}

func ikePolicyV2LifetimeCreateOptsFromPlan(ctx context.Context, lifetimeSet types.Set, diagnostics *diag.Diagnostics) *ikepolicies.LifetimeCreateOpts {
	if lifetimeSet.IsNull() || lifetimeSet.IsUnknown() || len(lifetimeSet.Elements()) == 0 {
		return nil
	}

	var items []IKEPolicyV2LifetimeModel

	diagnostics.Append(lifetimeSet.ElementsAs(ctx, &items, false)...)
	if diagnostics.HasError() || len(items) == 0 {
		return nil
	}

	item := items[0]

	return &ikepolicies.LifetimeCreateOpts{
		Units: ikePolicyV2Unit(item.Units.ValueString()),
		Value: int(item.Value.ValueInt64()),
	}
}

func ikePolicyV2LifetimeUpdateOptsFromPlan(ctx context.Context, lifetimeSet types.Set, diagnostics *diag.Diagnostics) *ikepolicies.LifetimeUpdateOpts {
	if lifetimeSet.IsNull() || lifetimeSet.IsUnknown() || len(lifetimeSet.Elements()) == 0 {
		return nil
	}

	var items []IKEPolicyV2LifetimeModel

	diagnostics.Append(lifetimeSet.ElementsAs(ctx, &items, false)...)
	if diagnostics.HasError() || len(items) == 0 {
		return nil
	}

	item := items[0]

	return &ikepolicies.LifetimeUpdateOpts{
		Units: ikePolicyV2Unit(item.Units.ValueString()),
		Value: int(item.Value.ValueInt64()),
	}
}

func ikePolicyV2ValueSpecsFromPlan(ctx context.Context, m types.Map, diagnostics *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}

	result := make(map[string]string)

	diagnostics.Append(m.ElementsAs(ctx, &result, false)...)

	return result
}
