package firewall

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &firewallResource{}
	_ resource.ResourceWithConfigure   = &firewallResource{}
	_ resource.ResourceWithImportState = &firewallResource{}
	_ resource.ResourceWithModifyPlan  = &firewallResource{}
)

// NewFirewallResource returns a new instance of the firewall resource.
func NewFirewallResource() resource.Resource {
	return &firewallResource{}
}

type firewallResource struct {
	client *godo.Client
}

// ---------------------------------------------------------------------------
// Model types
// ---------------------------------------------------------------------------

type firewallModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	DropletIDs     types.Set    `tfsdk:"droplet_ids"`
	InboundRules   types.Set    `tfsdk:"inbound_rule"`
	OutboundRules  types.Set    `tfsdk:"outbound_rule"`
	Status         types.String `tfsdk:"status"`
	CreatedAt      types.String `tfsdk:"created_at"`
	PendingChanges types.List   `tfsdk:"pending_changes"`
	Tags           types.Set    `tfsdk:"tags"`
}

type inboundRuleModel struct {
	Protocol              types.String `tfsdk:"protocol"`
	PortRange             types.String `tfsdk:"port_range"`
	SourceAddresses       types.Set    `tfsdk:"source_addresses"`
	SourceDropletIDs      types.Set    `tfsdk:"source_droplet_ids"`
	SourceLoadBalancerUID types.Set    `tfsdk:"source_load_balancer_uids"`
	SourceKubernetesIDs   types.Set    `tfsdk:"source_kubernetes_ids"`
	SourceTags            types.Set    `tfsdk:"source_tags"`
}

type outboundRuleModel struct {
	Protocol                   types.String `tfsdk:"protocol"`
	PortRange                  types.String `tfsdk:"port_range"`
	DestinationAddresses       types.Set    `tfsdk:"destination_addresses"`
	DestinationDropletIDs      types.Set    `tfsdk:"destination_droplet_ids"`
	DestinationLoadBalancerUID types.Set    `tfsdk:"destination_load_balancer_uids"`
	DestinationKubernetesIDs   types.Set    `tfsdk:"destination_kubernetes_ids"`
	DestinationTags            types.Set    `tfsdk:"destination_tags"`
}

type pendingChangeModel struct {
	DropletID types.Int64  `tfsdk:"droplet_id"`
	Removing  types.Bool   `tfsdk:"removing"`
	Status    types.String `tfsdk:"status"`
}

// ---------------------------------------------------------------------------
// Attribute type objects (for building Set/List values)
// ---------------------------------------------------------------------------

var inboundRuleAttrTypes = map[string]attr.Type{
	"protocol":                  types.StringType,
	"port_range":                types.StringType,
	"source_addresses":          types.SetType{ElemType: types.StringType},
	"source_droplet_ids":        types.SetType{ElemType: types.Int64Type},
	"source_load_balancer_uids": types.SetType{ElemType: types.StringType},
	"source_kubernetes_ids":     types.SetType{ElemType: types.StringType},
	"source_tags":               types.SetType{ElemType: types.StringType},
}

var outboundRuleAttrTypes = map[string]attr.Type{
	"protocol":                       types.StringType,
	"port_range":                     types.StringType,
	"destination_addresses":          types.SetType{ElemType: types.StringType},
	"destination_droplet_ids":        types.SetType{ElemType: types.Int64Type},
	"destination_load_balancer_uids": types.SetType{ElemType: types.StringType},
	"destination_kubernetes_ids":     types.SetType{ElemType: types.StringType},
	"destination_tags":               types.SetType{ElemType: types.StringType},
}

var pendingChangeAttrTypes = map[string]attr.Type{
	"droplet_id": types.Int64Type,
	"removing":   types.BoolType,
	"status":     types.StringType,
}

// ---------------------------------------------------------------------------
// resource.Resource
// ---------------------------------------------------------------------------

func (r *firewallResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall"
}

func (r *firewallResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"droplet_ids": schema.SetAttribute{
				ElementType: types.Int64Type,
				Optional:    true,
			},
			"inbound_rule": schema.SetNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"protocol": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf("tcp", "udp", "icmp"),
							},
						},
						"port_range": schema.StringAttribute{
							Optional: true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"source_addresses": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
						"source_droplet_ids": schema.SetAttribute{
							ElementType: types.Int64Type,
							Optional:    true,
						},
						"source_load_balancer_uids": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
						"source_kubernetes_ids": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
						"source_tags": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
					},
				},
			},
			"outbound_rule": schema.SetNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"protocol": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf("tcp", "udp", "icmp"),
							},
						},
						"port_range": schema.StringAttribute{
							Optional: true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"destination_addresses": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
						"destination_droplet_ids": schema.SetAttribute{
							ElementType: types.Int64Type,
							Optional:    true,
						},
						"destination_load_balancer_uids": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
						"destination_kubernetes_ids": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
						"destination_tags": schema.SetAttribute{
							ElementType: types.StringType,
							Optional:    true,
						},
					},
				},
			},
			"status": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"pending_changes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"droplet_id": schema.Int64Attribute{
							Optional: true,
						},
						"removing": schema.BoolAttribute{
							Optional: true,
						},
						"status": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
			"tags": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// resource.ResourceWithConfigure
// ---------------------------------------------------------------------------

func (r *firewallResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	combined, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *config.CombinedConfig, got %T", req.ProviderData),
		)
		return
	}
	r.client = combined.GodoClient()
}

// ---------------------------------------------------------------------------
// resource.ResourceWithModifyPlan  (replaces CustomizeDiff)
// ---------------------------------------------------------------------------

func (r *firewallResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Short-circuit on destroy.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan firewallModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasInbound := !plan.InboundRules.IsNull() && !plan.InboundRules.IsUnknown() && len(plan.InboundRules.Elements()) > 0
	hasOutbound := !plan.OutboundRules.IsNull() && !plan.OutboundRules.IsUnknown() && len(plan.OutboundRules.Elements()) > 0

	if !hasInbound && !hasOutbound {
		resp.Diagnostics.AddError(
			"At least one firewall rule required",
			"At least one inbound_rule or outbound_rule must be specified.",
		)
		return
	}

	// Validate inbound rules: non-icmp protocols require port_range.
	if hasInbound {
		var inboundRules []inboundRuleModel
		resp.Diagnostics.Append(plan.InboundRules.ElementsAs(ctx, &inboundRules, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for _, rule := range inboundRules {
			protocol := rule.Protocol.ValueString()
			portRange := rule.PortRange.ValueString()
			if protocol != "icmp" && portRange == "" {
				resp.Diagnostics.AddAttributeError(
					path.Root("inbound_rule"),
					"port_range required",
					"`port_range` of inbound rules is required if protocol is `tcp` or `udp`",
				)
				return
			}
		}
	}

	// Validate outbound rules: non-icmp protocols require port_range.
	if hasOutbound {
		var outboundRules []outboundRuleModel
		resp.Diagnostics.Append(plan.OutboundRules.ElementsAs(ctx, &outboundRules, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for _, rule := range outboundRules {
			protocol := rule.Protocol.ValueString()
			portRange := rule.PortRange.ValueString()
			if protocol != "icmp" && portRange == "" {
				resp.Diagnostics.AddAttributeError(
					path.Root("outbound_rule"),
					"port_range required",
					"`port_range` of outbound rules is required if protocol is `tcp` or `udp`",
				)
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// resource.ResourceWithImportState
// ---------------------------------------------------------------------------

func (r *firewallResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func (r *firewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts, diags := buildFirewallRequest(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Firewall create configuration: %#v", opts)

	firewall, _, err := r.client.Firewalls.Create(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError("Error creating firewall", err.Error())
		return
	}

	plan.ID = types.StringValue(firewall.ID)
	log.Printf("[INFO] Firewall ID: %s", firewall.ID)

	state, diags := firewallToModel(ctx, firewall)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *firewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	firewall, httpResp, err := r.client.Firewalls.Get(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			log.Printf("[WARN] DigitalOcean Firewall (%s) not found", state.ID.ValueString())
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error retrieving firewall", err.Error())
		return
	}

	newState, diags := firewallToModel(ctx, firewall)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *firewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state firewallModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts, diags := buildFirewallRequest(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Firewall update configuration: %#v", opts)

	_, _, err := r.client.Firewalls.Update(ctx, state.ID.ValueString(), opts)
	if err != nil {
		resp.Diagnostics.AddError("Error updating firewall", err.Error())
		return
	}

	firewall, _, err := r.client.Firewalls.Get(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading firewall after update", err.Error())
		return
	}

	newState, diags2 := firewallToModel(ctx, firewall)
	resp.Diagnostics.Append(diags2...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, newState)...)
}

func (r *firewallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[INFO] Deleting firewall: %s", state.ID.ValueString())

	_, err := r.client.Firewalls.Delete(ctx, state.ID.ValueString())
	if err != nil && strings.Contains(err.Error(), "404 Not Found") {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting firewall", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Helpers: build API request from model
// ---------------------------------------------------------------------------

func buildFirewallRequest(ctx context.Context, plan firewallModel) (*godo.FirewallRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	opts := &godo.FirewallRequest{
		Name: plan.Name.ValueString(),
	}

	// Droplet IDs
	if !plan.DropletIDs.IsNull() && !plan.DropletIDs.IsUnknown() {
		var ids []int64
		diags.Append(plan.DropletIDs.ElementsAs(ctx, &ids, false)...)
		if diags.HasError() {
			return nil, diags
		}
		ints := make([]int, len(ids))
		for i, v := range ids {
			ints[i] = int(v)
		}
		opts.DropletIDs = ints
	}

	// Inbound rules
	if !plan.InboundRules.IsNull() && !plan.InboundRules.IsUnknown() {
		var rules []inboundRuleModel
		diags.Append(plan.InboundRules.ElementsAs(ctx, &rules, false)...)
		if diags.HasError() {
			return nil, diags
		}
		opts.InboundRules = make([]godo.InboundRule, 0, len(rules))
		for _, r := range rules {
			rule, d := expandInboundRule(ctx, r)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			opts.InboundRules = append(opts.InboundRules, rule)
		}
	}

	// Outbound rules
	if !plan.OutboundRules.IsNull() && !plan.OutboundRules.IsUnknown() {
		var rules []outboundRuleModel
		diags.Append(plan.OutboundRules.ElementsAs(ctx, &rules, false)...)
		if diags.HasError() {
			return nil, diags
		}
		opts.OutboundRules = make([]godo.OutboundRule, 0, len(rules))
		for _, r := range rules {
			rule, d := expandOutboundRule(ctx, r)
			diags.Append(d...)
			if diags.HasError() {
				return nil, diags
			}
			opts.OutboundRules = append(opts.OutboundRules, rule)
		}
	}

	// Tags
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tagList []string
		diags.Append(plan.Tags.ElementsAs(ctx, &tagList, false)...)
		if diags.HasError() {
			return nil, diags
		}
		opts.Tags = tagList
	}

	return opts, diags
}

func expandInboundRule(ctx context.Context, r inboundRuleModel) (godo.InboundRule, diag.Diagnostics) {
	var diags diag.Diagnostics
	var src godo.Sources

	if !r.SourceDropletIDs.IsNull() && !r.SourceDropletIDs.IsUnknown() {
		var ids []int64
		diags.Append(r.SourceDropletIDs.ElementsAs(ctx, &ids, false)...)
		src.DropletIDs = int64SliceToInt(ids)
	}
	if !r.SourceAddresses.IsNull() && !r.SourceAddresses.IsUnknown() {
		var addrs []string
		diags.Append(r.SourceAddresses.ElementsAs(ctx, &addrs, false)...)
		src.Addresses = addrs
	}
	if !r.SourceLoadBalancerUID.IsNull() && !r.SourceLoadBalancerUID.IsUnknown() {
		var uids []string
		diags.Append(r.SourceLoadBalancerUID.ElementsAs(ctx, &uids, false)...)
		src.LoadBalancerUIDs = uids
	}
	if !r.SourceKubernetesIDs.IsNull() && !r.SourceKubernetesIDs.IsUnknown() {
		var kIDs []string
		diags.Append(r.SourceKubernetesIDs.ElementsAs(ctx, &kIDs, false)...)
		src.KubernetesIDs = kIDs
	}
	if !r.SourceTags.IsNull() && !r.SourceTags.IsUnknown() {
		var tags []string
		diags.Append(r.SourceTags.ElementsAs(ctx, &tags, false)...)
		src.Tags = tags
	}

	return godo.InboundRule{
		Protocol:  r.Protocol.ValueString(),
		PortRange: r.PortRange.ValueString(),
		Sources:   &src,
	}, diags
}

func expandOutboundRule(ctx context.Context, r outboundRuleModel) (godo.OutboundRule, diag.Diagnostics) {
	var diags diag.Diagnostics
	var dest godo.Destinations

	if !r.DestinationDropletIDs.IsNull() && !r.DestinationDropletIDs.IsUnknown() {
		var ids []int64
		diags.Append(r.DestinationDropletIDs.ElementsAs(ctx, &ids, false)...)
		dest.DropletIDs = int64SliceToInt(ids)
	}
	if !r.DestinationAddresses.IsNull() && !r.DestinationAddresses.IsUnknown() {
		var addrs []string
		diags.Append(r.DestinationAddresses.ElementsAs(ctx, &addrs, false)...)
		dest.Addresses = addrs
	}
	if !r.DestinationLoadBalancerUID.IsNull() && !r.DestinationLoadBalancerUID.IsUnknown() {
		var uids []string
		diags.Append(r.DestinationLoadBalancerUID.ElementsAs(ctx, &uids, false)...)
		dest.LoadBalancerUIDs = uids
	}
	if !r.DestinationKubernetesIDs.IsNull() && !r.DestinationKubernetesIDs.IsUnknown() {
		var kIDs []string
		diags.Append(r.DestinationKubernetesIDs.ElementsAs(ctx, &kIDs, false)...)
		dest.KubernetesIDs = kIDs
	}
	if !r.DestinationTags.IsNull() && !r.DestinationTags.IsUnknown() {
		var tags []string
		diags.Append(r.DestinationTags.ElementsAs(ctx, &tags, false)...)
		dest.Tags = tags
	}

	return godo.OutboundRule{
		Protocol:     r.Protocol.ValueString(),
		PortRange:    r.PortRange.ValueString(),
		Destinations: &dest,
	}, diags
}

// ---------------------------------------------------------------------------
// Helpers: flatten API response into model
// ---------------------------------------------------------------------------

func firewallToModel(ctx context.Context, fw *godo.Firewall) (firewallModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	m := firewallModel{
		ID:        types.StringValue(fw.ID),
		Name:      types.StringValue(fw.Name),
		Status:    types.StringValue(fw.Status),
		CreatedAt: types.StringValue(fw.Created),
	}

	// Droplet IDs
	dropletIDs := make([]attr.Value, len(fw.DropletIDs))
	for i, id := range fw.DropletIDs {
		dropletIDs[i] = types.Int64Value(int64(id))
	}
	dropletSet, d := types.SetValue(types.Int64Type, dropletIDs)
	diags.Append(d...)
	m.DropletIDs = dropletSet

	// Tags
	m.Tags = stringSliceToSet(fw.Tags)

	// Inbound rules
	inboundObjs := make([]attr.Value, len(fw.InboundRules))
	for i, rule := range fw.InboundRules {
		obj, d := flattenInboundRule(ctx, rule)
		diags.Append(d...)
		inboundObjs[i] = obj
	}
	inboundSet, d := types.SetValue(types.ObjectType{AttrTypes: inboundRuleAttrTypes}, inboundObjs)
	diags.Append(d...)
	m.InboundRules = inboundSet

	// Outbound rules
	outboundObjs := make([]attr.Value, len(fw.OutboundRules))
	for i, rule := range fw.OutboundRules {
		obj, d := flattenOutboundRule(ctx, rule)
		diags.Append(d...)
		outboundObjs[i] = obj
	}
	outboundSet, d := types.SetValue(types.ObjectType{AttrTypes: outboundRuleAttrTypes}, outboundObjs)
	diags.Append(d...)
	m.OutboundRules = outboundSet

	// Pending changes
	pendingObjs := make([]attr.Value, len(fw.PendingChanges))
	for i, change := range fw.PendingChanges {
		obj, d := types.ObjectValue(pendingChangeAttrTypes, map[string]attr.Value{
			"droplet_id": types.Int64Value(int64(change.DropletID)),
			"removing":   types.BoolValue(change.Removing),
			"status":     types.StringValue(change.Status),
		})
		diags.Append(d...)
		pendingObjs[i] = obj
	}
	pendingList, d := types.ListValue(types.ObjectType{AttrTypes: pendingChangeAttrTypes}, pendingObjs)
	diags.Append(d...)
	m.PendingChanges = pendingList

	return m, diags
}

func flattenInboundRule(_ context.Context, rule godo.InboundRule) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics

	portRange := rule.PortRange
	if portRange == "0" && rule.Protocol != "icmp" {
		portRange = "all"
	} else if portRange == "0" {
		portRange = ""
	}

	src := rule.Sources

	sourceAddresses := stringSliceToSet(src.Addresses)
	sourceDropletIDs := intSliceToInt64Set(src.DropletIDs)
	sourceLBUIDs := stringSliceToSet(src.LoadBalancerUIDs)
	sourceK8sIDs := stringSliceToSet(src.KubernetesIDs)
	sourceTags := stringSliceToSet(src.Tags)

	obj, d := types.ObjectValue(inboundRuleAttrTypes, map[string]attr.Value{
		"protocol":                  types.StringValue(rule.Protocol),
		"port_range":                types.StringValue(portRange),
		"source_addresses":          sourceAddresses,
		"source_droplet_ids":        sourceDropletIDs,
		"source_load_balancer_uids": sourceLBUIDs,
		"source_kubernetes_ids":     sourceK8sIDs,
		"source_tags":               sourceTags,
	})
	diags.Append(d...)
	return obj, diags
}

func flattenOutboundRule(_ context.Context, rule godo.OutboundRule) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics

	portRange := rule.PortRange
	if portRange == "0" && rule.Protocol != "icmp" {
		portRange = "all"
	} else if portRange == "0" {
		portRange = ""
	}

	dest := rule.Destinations

	destAddresses := stringSliceToSet(dest.Addresses)
	destDropletIDs := intSliceToInt64Set(dest.DropletIDs)
	destLBUIDs := stringSliceToSet(dest.LoadBalancerUIDs)
	destK8sIDs := stringSliceToSet(dest.KubernetesIDs)
	destTags := stringSliceToSet(dest.Tags)

	obj, d := types.ObjectValue(outboundRuleAttrTypes, map[string]attr.Value{
		"protocol":                       types.StringValue(rule.Protocol),
		"port_range":                     types.StringValue(portRange),
		"destination_addresses":          destAddresses,
		"destination_droplet_ids":        destDropletIDs,
		"destination_load_balancer_uids": destLBUIDs,
		"destination_kubernetes_ids":     destK8sIDs,
		"destination_tags":               destTags,
	})
	diags.Append(d...)
	return obj, diags
}

// ---------------------------------------------------------------------------
// Small utilities
// ---------------------------------------------------------------------------

func int64SliceToInt(in []int64) []int {
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}

func stringSliceToSet(in []string) types.Set {
	vals := make([]attr.Value, len(in))
	for i, s := range in {
		vals[i] = types.StringValue(s)
	}
	set, _ := types.SetValue(types.StringType, vals)
	return set
}

func intSliceToInt64Set(in []int) types.Set {
	vals := make([]attr.Value, len(in))
	for i, v := range in {
		vals[i] = types.Int64Value(int64(v))
	}
	set, _ := types.SetValue(types.Int64Type, vals)
	return set
}
