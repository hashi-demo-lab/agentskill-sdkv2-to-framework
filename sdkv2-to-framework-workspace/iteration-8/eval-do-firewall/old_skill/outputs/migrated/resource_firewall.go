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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
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

// NewFirewallResource returns a new framework resource for digitalocean_firewall.
func NewFirewallResource() resource.Resource {
	return &firewallResource{}
}

// firewallResource is the concrete framework resource type.
type firewallResource struct {
	client *godo.Client
}

// ---------------------------------------------------------------------------
// Model structs
// ---------------------------------------------------------------------------

type firewallModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Status         types.String `tfsdk:"status"`
	CreatedAt      types.String `tfsdk:"created_at"`
	DropletIDs     types.Set    `tfsdk:"droplet_ids"`
	InboundRules   types.Set    `tfsdk:"inbound_rule"`
	OutboundRules  types.Set    `tfsdk:"outbound_rule"`
	Tags           types.Set    `tfsdk:"tags"`
	PendingChanges types.List   `tfsdk:"pending_changes"`
}

type firewallInboundRuleModel struct {
	Protocol              types.String `tfsdk:"protocol"`
	PortRange             types.String `tfsdk:"port_range"`
	SourceAddresses       types.Set    `tfsdk:"source_addresses"`
	SourceDropletIDs      types.Set    `tfsdk:"source_droplet_ids"`
	SourceLoadBalancerUID types.Set    `tfsdk:"source_load_balancer_uids"`
	SourceKubernetesIDs   types.Set    `tfsdk:"source_kubernetes_ids"`
	SourceTags            types.Set    `tfsdk:"source_tags"`
}

type firewallOutboundRuleModel struct {
	Protocol                   types.String `tfsdk:"protocol"`
	PortRange                  types.String `tfsdk:"port_range"`
	DestinationAddresses       types.Set    `tfsdk:"destination_addresses"`
	DestinationDropletIDs      types.Set    `tfsdk:"destination_droplet_ids"`
	DestinationLoadBalancerUID types.Set    `tfsdk:"destination_load_balancer_uids"`
	DestinationKubernetesIDs   types.Set    `tfsdk:"destination_kubernetes_ids"`
	DestinationTags            types.Set    `tfsdk:"destination_tags"`
}

// attrTypes — needed for types.ObjectType and types.SetValue / types.ListValue calls.
var (
	inboundRuleAttrTypes = map[string]attr.Type{
		"protocol":                  types.StringType,
		"port_range":                types.StringType,
		"source_addresses":          types.SetType{ElemType: types.StringType},
		"source_droplet_ids":        types.SetType{ElemType: types.Int64Type},
		"source_load_balancer_uids": types.SetType{ElemType: types.StringType},
		"source_kubernetes_ids":     types.SetType{ElemType: types.StringType},
		"source_tags":               types.SetType{ElemType: types.StringType},
	}

	outboundRuleAttrTypes = map[string]attr.Type{
		"protocol":                       types.StringType,
		"port_range":                     types.StringType,
		"destination_addresses":          types.SetType{ElemType: types.StringType},
		"destination_droplet_ids":        types.SetType{ElemType: types.Int64Type},
		"destination_load_balancer_uids": types.SetType{ElemType: types.StringType},
		"destination_kubernetes_ids":     types.SetType{ElemType: types.StringType},
		"destination_tags":               types.SetType{ElemType: types.StringType},
	}

	pendingChangeAttrTypes = map[string]attr.Type{
		"droplet_id": types.Int64Type,
		"removing":   types.BoolType,
		"status":     types.StringType,
	}
)

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (r *firewallResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall"
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

func (r *firewallResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	inboundRuleNestedAttrs := map[string]schema.Attribute{
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
	}

	outboundRuleNestedAttrs := map[string]schema.Attribute{
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
	}

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
			"droplet_ids": schema.SetAttribute{
				ElementType: types.Int64Type,
				Optional:    true,
			},
			"inbound_rule": schema.SetNestedAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: inboundRuleNestedAttrs,
				},
			},
			"outbound_rule": schema.SetNestedAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: outboundRuleNestedAttrs,
				},
			},
			"tags": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
			},
			"pending_changes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"droplet_id": schema.Int64Attribute{Optional: true, Computed: true},
						"removing":   schema.BoolAttribute{Optional: true, Computed: true},
						"status":     schema.StringAttribute{Optional: true, Computed: true},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Configure
// ---------------------------------------------------------------------------

func (r *firewallResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	combined, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *config.CombinedConfig, got: %T", req.ProviderData),
		)
		return
	}
	r.client = combined.GodoClient()
}

// ---------------------------------------------------------------------------
// ModifyPlan — replaces CustomizeDiff
// ---------------------------------------------------------------------------

func (r *firewallResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// On destroy the plan is null — nothing to validate.
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
			"At least one rule must be specified",
			"The firewall must have at least one inbound_rule or outbound_rule.",
		)
		return
	}

	// Validate inbound rules: port_range required unless protocol is icmp.
	if !plan.InboundRules.IsNull() && !plan.InboundRules.IsUnknown() {
		var inboundRules []firewallInboundRuleModel
		resp.Diagnostics.Append(plan.InboundRules.ElementsAs(ctx, &inboundRules, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for _, rule := range inboundRules {
			protocol := rule.Protocol.ValueString()
			portRange := rule.PortRange.ValueString()
			if protocol != "icmp" && portRange == "" {
				resp.Diagnostics.AddError(
					"port_range required",
					"`port_range` of inbound rules is required if protocol is `tcp` or `udp`",
				)
			}
		}
	}

	// Validate outbound rules: port_range required unless protocol is icmp.
	if !plan.OutboundRules.IsNull() && !plan.OutboundRules.IsUnknown() {
		var outboundRules []firewallOutboundRuleModel
		resp.Diagnostics.Append(plan.OutboundRules.ElementsAs(ctx, &outboundRules, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for _, rule := range outboundRules {
			protocol := rule.Protocol.ValueString()
			portRange := rule.PortRange.ValueString()
			if protocol != "icmp" && portRange == "" {
				resp.Diagnostics.AddError(
					"port_range required",
					"`port_range` of outbound rules is required if protocol is `tcp` or `udp`",
				)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *firewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts, diags := r.buildFirewallRequest(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Firewall create configuration: %#v", opts)

	fw, _, err := r.client.Firewalls.Create(ctx, opts)
	if err != nil {
		resp.Diagnostics.AddError("Error creating firewall", err.Error())
		return
	}

	plan.ID = types.StringValue(fw.ID)
	log.Printf("[INFO] Firewall ID: %s", fw.ID)

	resp.Diagnostics.Append(r.refreshState(ctx, fw, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (r *firewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	fw, httpResp, err := r.client.Firewalls.Get(ctx, state.ID.ValueString())
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			log.Printf("[WARN] DigitalOcean Firewall (%s) not found", state.ID.ValueString())
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error retrieving firewall", err.Error())
		return
	}

	resp.Diagnostics.Append(r.refreshState(ctx, fw, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *firewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state firewallModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Carry the ID from state — it's Computed+UseStateForUnknown, but since
	// we derive the request from the plan, copy it explicitly.
	plan.ID = state.ID

	opts, diags := r.buildFirewallRequest(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Firewall update configuration: %#v", opts)

	fw, _, err := r.client.Firewalls.Update(ctx, plan.ID.ValueString(), opts)
	if err != nil {
		resp.Diagnostics.AddError("Error updating firewall", err.Error())
		return
	}

	resp.Diagnostics.Append(r.refreshState(ctx, fw, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

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
// ImportState
// ---------------------------------------------------------------------------

func (r *firewallResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// buildFirewallRequest converts a plan model into a godo FirewallRequest.
func (r *firewallResource) buildFirewallRequest(ctx context.Context, plan firewallModel) (*godo.FirewallRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	opts := &godo.FirewallRequest{
		Name: plan.Name.ValueString(),
	}

	// droplet_ids
	if !plan.DropletIDs.IsNull() && !plan.DropletIDs.IsUnknown() {
		var ids []int64
		diags.Append(plan.DropletIDs.ElementsAs(ctx, &ids, false)...)
		intIDs := make([]int, len(ids))
		for i, id := range ids {
			intIDs[i] = int(id)
		}
		opts.DropletIDs = intIDs
	}

	// inbound_rule
	if !plan.InboundRules.IsNull() && !plan.InboundRules.IsUnknown() {
		var rules []firewallInboundRuleModel
		diags.Append(plan.InboundRules.ElementsAs(ctx, &rules, false)...)
		if diags.HasError() {
			return nil, diags
		}
		opts.InboundRules = expandInboundRules(ctx, rules, &diags)
	}

	// outbound_rule
	if !plan.OutboundRules.IsNull() && !plan.OutboundRules.IsUnknown() {
		var rules []firewallOutboundRuleModel
		diags.Append(plan.OutboundRules.ElementsAs(ctx, &rules, false)...)
		if diags.HasError() {
			return nil, diags
		}
		opts.OutboundRules = expandOutboundRules(ctx, rules, &diags)
	}

	// tags
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string
		diags.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		opts.Tags = tags
	}

	return opts, diags
}

// expandInboundRules converts framework inbound rule models into godo InboundRules.
func expandInboundRules(ctx context.Context, rules []firewallInboundRuleModel, diags *diag.Diagnostics) []godo.InboundRule {
	result := make([]godo.InboundRule, 0, len(rules))
	for _, rule := range rules {
		var src godo.Sources

		if !rule.SourceDropletIDs.IsNull() && !rule.SourceDropletIDs.IsUnknown() {
			var ids []int64
			diags.Append(rule.SourceDropletIDs.ElementsAs(ctx, &ids, false)...)
			intIDs := make([]int, len(ids))
			for i, id := range ids {
				intIDs[i] = int(id)
			}
			src.DropletIDs = intIDs
		}

		if !rule.SourceAddresses.IsNull() && !rule.SourceAddresses.IsUnknown() {
			var addrs []string
			diags.Append(rule.SourceAddresses.ElementsAs(ctx, &addrs, false)...)
			src.Addresses = addrs
		}

		if !rule.SourceLoadBalancerUID.IsNull() && !rule.SourceLoadBalancerUID.IsUnknown() {
			var uids []string
			diags.Append(rule.SourceLoadBalancerUID.ElementsAs(ctx, &uids, false)...)
			src.LoadBalancerUIDs = uids
		}

		if !rule.SourceKubernetesIDs.IsNull() && !rule.SourceKubernetesIDs.IsUnknown() {
			var ids []string
			diags.Append(rule.SourceKubernetesIDs.ElementsAs(ctx, &ids, false)...)
			src.KubernetesIDs = ids
		}

		if !rule.SourceTags.IsNull() && !rule.SourceTags.IsUnknown() {
			var tags []string
			diags.Append(rule.SourceTags.ElementsAs(ctx, &tags, false)...)
			src.Tags = tags
		}

		result = append(result, godo.InboundRule{
			Protocol:  rule.Protocol.ValueString(),
			PortRange: rule.PortRange.ValueString(),
			Sources:   &src,
		})
	}
	return result
}

// expandOutboundRules converts framework outbound rule models into godo OutboundRules.
func expandOutboundRules(ctx context.Context, rules []firewallOutboundRuleModel, diags *diag.Diagnostics) []godo.OutboundRule {
	result := make([]godo.OutboundRule, 0, len(rules))
	for _, rule := range rules {
		var dest godo.Destinations

		if !rule.DestinationDropletIDs.IsNull() && !rule.DestinationDropletIDs.IsUnknown() {
			var ids []int64
			diags.Append(rule.DestinationDropletIDs.ElementsAs(ctx, &ids, false)...)
			intIDs := make([]int, len(ids))
			for i, id := range ids {
				intIDs[i] = int(id)
			}
			dest.DropletIDs = intIDs
		}

		if !rule.DestinationAddresses.IsNull() && !rule.DestinationAddresses.IsUnknown() {
			var addrs []string
			diags.Append(rule.DestinationAddresses.ElementsAs(ctx, &addrs, false)...)
			dest.Addresses = addrs
		}

		if !rule.DestinationLoadBalancerUID.IsNull() && !rule.DestinationLoadBalancerUID.IsUnknown() {
			var uids []string
			diags.Append(rule.DestinationLoadBalancerUID.ElementsAs(ctx, &uids, false)...)
			dest.LoadBalancerUIDs = uids
		}

		if !rule.DestinationKubernetesIDs.IsNull() && !rule.DestinationKubernetesIDs.IsUnknown() {
			var ids []string
			diags.Append(rule.DestinationKubernetesIDs.ElementsAs(ctx, &ids, false)...)
			dest.KubernetesIDs = ids
		}

		if !rule.DestinationTags.IsNull() && !rule.DestinationTags.IsUnknown() {
			var tags []string
			diags.Append(rule.DestinationTags.ElementsAs(ctx, &tags, false)...)
			dest.Tags = tags
		}

		result = append(result, godo.OutboundRule{
			Protocol:     rule.Protocol.ValueString(),
			PortRange:    rule.PortRange.ValueString(),
			Destinations: &dest,
		})
	}
	return result
}

// refreshState populates a firewallModel from a godo.Firewall API response.
func (r *firewallResource) refreshState(ctx context.Context, fw *godo.Firewall, m *firewallModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.Name = types.StringValue(fw.Name)
	m.Status = types.StringValue(fw.Status)
	m.CreatedAt = types.StringValue(fw.Created)

	// droplet_ids
	dropletIDs := make([]attr.Value, len(fw.DropletIDs))
	for i, id := range fw.DropletIDs {
		dropletIDs[i] = types.Int64Value(int64(id))
	}
	dropletIDSet, d := types.SetValue(types.Int64Type, dropletIDs)
	diags.Append(d...)
	m.DropletIDs = dropletIDSet

	// tags
	tagVals := make([]attr.Value, len(fw.Tags))
	for i, t := range fw.Tags {
		tagVals[i] = types.StringValue(t)
	}
	tagSet, d := types.SetValue(types.StringType, tagVals)
	diags.Append(d...)
	m.Tags = tagSet

	// inbound_rules
	inboundObjs := make([]attr.Value, 0, len(fw.InboundRules))
	for _, rule := range fw.InboundRules {
		portRange := rule.PortRange
		if portRange == "0" {
			if rule.Protocol != "icmp" {
				portRange = "all"
			} else {
				portRange = ""
			}
		}

		obj, d := types.ObjectValue(inboundRuleAttrTypes, map[string]attr.Value{
			"protocol":                  types.StringValue(rule.Protocol),
			"port_range":                types.StringValue(portRange),
			"source_addresses":          stringSetValue(rule.Sources.Addresses),
			"source_droplet_ids":        intSetValue(rule.Sources.DropletIDs),
			"source_load_balancer_uids": stringSetValue(rule.Sources.LoadBalancerUIDs),
			"source_kubernetes_ids":     stringSetValue(rule.Sources.KubernetesIDs),
			"source_tags":               stringSetValue(rule.Sources.Tags),
		})
		diags.Append(d...)
		inboundObjs = append(inboundObjs, obj)
	}
	inboundSet, d := types.SetValue(types.ObjectType{AttrTypes: inboundRuleAttrTypes}, inboundObjs)
	diags.Append(d...)
	m.InboundRules = inboundSet

	// outbound_rules
	outboundObjs := make([]attr.Value, 0, len(fw.OutboundRules))
	for _, rule := range fw.OutboundRules {
		portRange := rule.PortRange
		if portRange == "0" {
			if rule.Protocol != "icmp" {
				portRange = "all"
			} else {
				portRange = ""
			}
		}

		obj, d := types.ObjectValue(outboundRuleAttrTypes, map[string]attr.Value{
			"protocol":                       types.StringValue(rule.Protocol),
			"port_range":                     types.StringValue(portRange),
			"destination_addresses":          stringSetValue(rule.Destinations.Addresses),
			"destination_droplet_ids":        intSetValue(rule.Destinations.DropletIDs),
			"destination_load_balancer_uids": stringSetValue(rule.Destinations.LoadBalancerUIDs),
			"destination_kubernetes_ids":     stringSetValue(rule.Destinations.KubernetesIDs),
			"destination_tags":               stringSetValue(rule.Destinations.Tags),
		})
		diags.Append(d...)
		outboundObjs = append(outboundObjs, obj)
	}
	outboundSet, d := types.SetValue(types.ObjectType{AttrTypes: outboundRuleAttrTypes}, outboundObjs)
	diags.Append(d...)
	m.OutboundRules = outboundSet

	// pending_changes
	pendingObjs := make([]attr.Value, 0, len(fw.PendingChanges))
	for _, change := range fw.PendingChanges {
		obj, d := types.ObjectValue(pendingChangeAttrTypes, map[string]attr.Value{
			"droplet_id": types.Int64Value(int64(change.DropletID)),
			"removing":   types.BoolValue(change.Removing),
			"status":     types.StringValue(change.Status),
		})
		diags.Append(d...)
		pendingObjs = append(pendingObjs, obj)
	}
	pendingList, d := types.ListValue(types.ObjectType{AttrTypes: pendingChangeAttrTypes}, pendingObjs)
	diags.Append(d...)
	m.PendingChanges = pendingList

	return diags
}

// stringSetValue converts a []string into a types.Set of strings.
func stringSetValue(ss []string) types.Set {
	if len(ss) == 0 {
		return types.SetValueMust(types.StringType, []attr.Value{})
	}
	vals := make([]attr.Value, len(ss))
	for i, s := range ss {
		vals[i] = types.StringValue(s)
	}
	return types.SetValueMust(types.StringType, vals)
}

// intSetValue converts a []int into a types.Set of Int64.
func intSetValue(ids []int) types.Set {
	if len(ids) == 0 {
		return types.SetValueMust(types.Int64Type, []attr.Value{})
	}
	vals := make([]attr.Value, len(ids))
	for i, id := range ids {
		vals[i] = types.Int64Value(int64(id))
	}
	return types.SetValueMust(types.Int64Type, vals)
}
