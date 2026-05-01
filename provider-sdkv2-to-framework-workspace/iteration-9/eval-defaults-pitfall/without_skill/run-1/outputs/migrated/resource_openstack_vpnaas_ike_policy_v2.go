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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ikePolicyV2Resource{}
	_ resource.ResourceWithImportState = &ikePolicyV2Resource{}
)

// ikePolicyV2Resource defines the resource implementation.
type ikePolicyV2Resource struct {
	config *Config
}

// ikePolicyV2ResourceModel describes the resource data model.
type ikePolicyV2ResourceModel struct {
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

// ikePolicyV2LifetimeModel describes the lifetime nested block attributes.
type ikePolicyV2LifetimeModel struct {
	Units types.String `tfsdk:"units"`
	Value types.Int64  `tfsdk:"value"`
}

// ikePolicyV2LifetimeAttrTypes holds the attribute types for the lifetime object.
var ikePolicyV2LifetimeAttrTypes = map[string]attr.Type{
	"units": types.StringType,
	"value": types.Int64Type,
}

// NewIKEPolicyV2Resource creates a new instance of the resource.
func NewIKEPolicyV2Resource() resource.Resource {
	return &ikePolicyV2Resource{}
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
					ikePolicyV2MapRequiresReplace{},
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
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.config = config
}

func (r *ikePolicyV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ikePolicyV2ResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := ikePolicyV2GetRegion(data.Region.ValueString(), r.config)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	lifetime := ikePolicyV2LifetimeCreateOpts(ctx, data.Lifetime, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	valueSpecs := ikePolicyV2MapValueSpecs(ctx, data.ValueSpecs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	opts := IKEPolicyCreateOpts{
		ikepolicies.CreateOpts{
			Name:                  data.Name.ValueString(),
			Description:           data.Description.ValueString(),
			TenantID:              data.TenantID.ValueString(),
			Lifetime:              &lifetime,
			AuthAlgorithm:         ikepolicies.AuthAlgorithm(data.AuthAlgorithm.ValueString()),
			EncryptionAlgorithm:   ikepolicies.EncryptionAlgorithm(data.EncryptionAlgorithm.ValueString()),
			PFS:                   ikepolicies.PFS(data.PFS.ValueString()),
			IKEVersion:            ikepolicies.IKEVersion(data.IKEVersion.ValueString()),
			Phase1NegotiationMode: ikepolicies.Phase1NegotiationMode(data.Phase1NegotiationMode.ValueString()),
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
		Timeout:    10 * time.Minute,
		Delay:      0,
		MinTimeout: 2 * time.Second,
	}

	_, err = stateConf.WaitForStateContext(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for openstack_vpnaas_ike_policy_v2 to become active",
			fmt.Sprintf("IKE policy %s: %s", policy.ID, err),
		)
		return
	}

	log.Printf("[DEBUG] IKE policy created: %#v", policy)

	data.ID = types.StringValue(policy.ID)

	ikePolicyV2ReadInto(ctx, networkingClient, policy.ID, &data, r.config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ikePolicyV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ikePolicyV2ResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := ikePolicyV2GetRegion(data.Region.ValueString(), r.config)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	ikePolicyV2ReadInto(ctx, networkingClient, data.ID.ValueString(), &data, r.config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ikePolicyV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ikePolicyV2ResourceModel
	var state ikePolicyV2ResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := ikePolicyV2GetRegion(plan.Region.ValueString(), r.config)

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
		lifetime := ikePolicyV2LifetimeUpdateOpts(ctx, plan.Lifetime, &resp.Diagnostics)
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
			Timeout:    10 * time.Minute,
			Delay:      0,
			MinTimeout: 2 * time.Second,
		}
		if _, err = stateConf.WaitForStateContext(ctx); err != nil {
			resp.Diagnostics.AddError("Error waiting for openstack_vpnaas_ike_policy_v2 update", err.Error())
			return
		}
	}

	plan.ID = state.ID

	ikePolicyV2ReadInto(ctx, networkingClient, state.ID.ValueString(), &plan, r.config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ikePolicyV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ikePolicyV2ResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Destroy IKE policy: %s", data.ID.ValueString())

	region := ikePolicyV2GetRegion(data.Region.ValueString(), r.config)

	networkingClient, err := r.config.NetworkingV2Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack networking client", err.Error())
		return
	}

	stateConf := &retry.StateChangeConf{
		Pending:    []string{"ACTIVE"},
		Target:     []string{"DELETED"},
		Refresh:    waitForIKEPolicyDeletion(ctx, networkingClient, data.ID.ValueString()),
		Timeout:    10 * time.Minute,
		Delay:      0,
		MinTimeout: 2 * time.Second,
	}

	if _, err = stateConf.WaitForStateContext(ctx); err != nil {
		resp.Diagnostics.AddError("Error deleting openstack_vpnaas_ike_policy_v2", err.Error())
	}
}

func (r *ikePolicyV2Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ikePolicyV2GetRegion returns the configured region, falling back to provider default.
func ikePolicyV2GetRegion(region string, config *Config) string {
	if region != "" {
		return region
	}
	return config.Region
}

// ikePolicyV2ReadInto reads the remote IKE policy and populates the model.
func ikePolicyV2ReadInto(ctx context.Context, networkingClient *gophercloud.ServiceClient, id string, data *ikePolicyV2ResourceModel, config *Config, diagnostics *diag.Diagnostics) {
	log.Printf("[DEBUG] Retrieve information about IKE policy: %s", id)

	policy, err := ikepolicies.Get(ctx, networkingClient, id).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			data.ID = types.StringValue("")
			return
		}
		diagnostics.AddError("Error reading openstack_vpnaas_ike_policy_v2", err.Error())
		return
	}

	log.Printf("[DEBUG] Read OpenStack IKE Policy %s: %#v", id, policy)

	data.Name = types.StringValue(policy.Name)
	data.Description = types.StringValue(policy.Description)
	data.AuthAlgorithm = types.StringValue(policy.AuthAlgorithm)
	data.EncryptionAlgorithm = types.StringValue(policy.EncryptionAlgorithm)
	data.TenantID = types.StringValue(policy.TenantID)
	data.PFS = types.StringValue(policy.PFS)
	data.Phase1NegotiationMode = types.StringValue(policy.Phase1NegotiationMode)
	data.IKEVersion = types.StringValue(policy.IKEVersion)
	data.Region = types.StringValue(ikePolicyV2GetRegion(data.Region.ValueString(), config))

	// Build the lifetime set
	lifetimeObj, diags := types.ObjectValue(
		ikePolicyV2LifetimeAttrTypes,
		map[string]attr.Value{
			"units": types.StringValue(policy.Lifetime.Units),
			"value": types.Int64Value(int64(policy.Lifetime.Value)),
		},
	)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}

	lifetimeSet, diags := types.SetValue(
		types.ObjectType{AttrTypes: ikePolicyV2LifetimeAttrTypes},
		[]attr.Value{lifetimeObj},
	)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return
	}

	data.Lifetime = lifetimeSet
}

// ikePolicyV2LifetimeCreateOpts converts a types.Set of lifetime blocks into
// ikepolicies.LifetimeCreateOpts.
func ikePolicyV2LifetimeCreateOpts(ctx context.Context, lifetimeSet types.Set, diagnostics *diag.Diagnostics) ikepolicies.LifetimeCreateOpts {
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

// ikePolicyV2LifetimeUpdateOpts converts a types.Set of lifetime blocks into
// ikepolicies.LifetimeUpdateOpts.
func ikePolicyV2LifetimeUpdateOpts(ctx context.Context, lifetimeSet types.Set, diagnostics *diag.Diagnostics) ikepolicies.LifetimeUpdateOpts {
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

// ikePolicyV2MapValueSpecs converts a types.Map of value_specs into map[string]string.
func ikePolicyV2MapValueSpecs(ctx context.Context, valueSpecs types.Map, diagnostics *diag.Diagnostics) map[string]string {
	if valueSpecs.IsNull() || valueSpecs.IsUnknown() {
		return nil
	}

	result := make(map[string]string)
	diagnostics.Append(valueSpecs.ElementsAs(ctx, &result, false)...)

	return result
}

// ikePolicyV2MapRequiresReplace is a plan modifier for Map attributes that
// triggers resource replacement when the value changes.
type ikePolicyV2MapRequiresReplace struct{}

func (m ikePolicyV2MapRequiresReplace) PlanModifyMap(_ context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	if req.StateValue.IsNull() {
		return
	}
	if req.ConfigValue.Equal(req.StateValue) {
		return
	}
	resp.RequiresReplace = true
}

func (m ikePolicyV2MapRequiresReplace) Description(_ context.Context) string {
	return "If the value of this attribute changes, Terraform will destroy and recreate the resource."
}

func (m ikePolicyV2MapRequiresReplace) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}
