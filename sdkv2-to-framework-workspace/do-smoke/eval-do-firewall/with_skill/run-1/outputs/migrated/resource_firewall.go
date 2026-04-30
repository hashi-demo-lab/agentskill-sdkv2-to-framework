package firewall

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
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

// Compile-time interface assertions: every framework capability the
// resource implements must be guarded so missing methods become a
// compile error instead of a silent runtime gap.
var (
	_ resource.Resource                   = &firewallResource{}
	_ resource.ResourceWithConfigure      = &firewallResource{}
	_ resource.ResourceWithImportState    = &firewallResource{}
	_ resource.ResourceWithModifyPlan     = &firewallResource{}
)

// NewFirewallResource is the framework constructor used to register the
// resource with the provider. Mirrors the role of
// ResourceDigitalOceanFirewall() in the SDKv2 implementation.
func NewFirewallResource() resource.Resource {
	return &firewallResource{}
}

type firewallResource struct {
	client *godo.Client
}

// ----- Models -----

// firewallResourceModel is the typed projection of the schema. The
// struct tags map straight to the schema attribute / block names.
type firewallResourceModel struct {
	ID             types.String          `tfsdk:"id"`
	Name           types.String          `tfsdk:"name"`
	DropletIDs     types.Set             `tfsdk:"droplet_ids"`
	Tags           types.Set             `tfsdk:"tags"`
	Status         types.String          `tfsdk:"status"`
	CreatedAt      types.String          `tfsdk:"created_at"`
	PendingChanges []pendingChangeModel  `tfsdk:"pending_changes"`
	InboundRules   []inboundRuleModel    `tfsdk:"inbound_rule"`
	OutboundRules  []outboundRuleModel   `tfsdk:"outbound_rule"`
}

type inboundRuleModel struct {
	Protocol               types.String `tfsdk:"protocol"`
	PortRange              types.String `tfsdk:"port_range"`
	SourceAddresses        types.Set    `tfsdk:"source_addresses"`
	SourceDropletIDs       types.Set    `tfsdk:"source_droplet_ids"`
	SourceLoadBalancerUIDs types.Set    `tfsdk:"source_load_balancer_uids"`
	SourceKubernetesIDs    types.Set    `tfsdk:"source_kubernetes_ids"`
	SourceTags             types.Set    `tfsdk:"source_tags"`
}

type outboundRuleModel struct {
	Protocol                    types.String `tfsdk:"protocol"`
	PortRange                   types.String `tfsdk:"port_range"`
	DestinationAddresses        types.Set    `tfsdk:"destination_addresses"`
	DestinationDropletIDs       types.Set    `tfsdk:"destination_droplet_ids"`
	DestinationLoadBalancerUIDs types.Set    `tfsdk:"destination_load_balancer_uids"`
	DestinationKubernetesIDs    types.Set    `tfsdk:"destination_kubernetes_ids"`
	DestinationTags             types.Set    `tfsdk:"destination_tags"`
}

type pendingChangeModel struct {
	DropletID types.Int64  `tfsdk:"droplet_id"`
	Removing  types.Bool   `tfsdk:"removing"`
	Status    types.String `tfsdk:"status"`
}

// ----- Identification -----

func (r *firewallResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall"
}

// ----- Schema -----
//
// `inbound_rule` and `outbound_rule` are true repeating blocks
// (TypeSet of &schema.Resource without MaxItems) — kept as
// `SetNestedBlock`s so existing practitioner HCL keeps parsing.
//
// `pending_changes` was a Computed list of nested in SDKv2; the
// framework forbids Computed blocks, so it becomes a Computed
// `ListNestedAttribute`.
func (r *firewallResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
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
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(
							regexp.MustCompile(`^[a-zA-Z0-9:\-_]{1,255}$`),
							"tags may contain lowercase letters, numbers, colons, dashes, and underscores; there is a limit of 255 characters per tag",
						),
					),
				},
			},
			"status": schema.StringAttribute{
				Computed: true,
			},
			"created_at": schema.StringAttribute{
				Computed: true,
			},
			// Computed blocks are not allowed in the framework, so the
			// SDKv2 `pending_changes` block becomes a Computed
			// ListNestedAttribute. The HCL form is read-only anyway, so
			// no practitioner config is broken.
			"pending_changes": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"droplet_id": schema.Int64Attribute{Computed: true},
						"removing":   schema.BoolAttribute{Computed: true},
						"status":     schema.StringAttribute{Computed: true},
					},
				},
			},
		},
		Blocks: map[string]schema.Block{
			"inbound_rule":  inboundRuleBlock(),
			"outbound_rule": outboundRuleBlock(),
		},
	}
}

func inboundRuleBlock() schema.SetNestedBlock {
	return schema.SetNestedBlock{
		NestedObject: schema.NestedBlockObject{
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
					Optional:    true,
					ElementType: types.StringType,
					Validators: []validator.Set{
						setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
					},
				},
				"source_droplet_ids": schema.SetAttribute{
					Optional:    true,
					ElementType: types.Int64Type,
				},
				"source_load_balancer_uids": schema.SetAttribute{
					Optional:    true,
					ElementType: types.StringType,
					Validators: []validator.Set{
						setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
					},
				},
				"source_kubernetes_ids": schema.SetAttribute{
					Optional:    true,
					ElementType: types.StringType,
					Validators: []validator.Set{
						setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
					},
				},
				"source_tags": schema.SetAttribute{
					Optional:    true,
					ElementType: types.StringType,
				},
			},
		},
	}
}

func outboundRuleBlock() schema.SetNestedBlock {
	return schema.SetNestedBlock{
		NestedObject: schema.NestedBlockObject{
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
					Optional:    true,
					ElementType: types.StringType,
					Validators: []validator.Set{
						setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
					},
				},
				"destination_droplet_ids": schema.SetAttribute{
					Optional:    true,
					ElementType: types.Int64Type,
				},
				"destination_load_balancer_uids": schema.SetAttribute{
					Optional:    true,
					ElementType: types.StringType,
					Validators: []validator.Set{
						setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
					},
				},
				"destination_kubernetes_ids": schema.SetAttribute{
					Optional:    true,
					ElementType: types.StringType,
					Validators: []validator.Set{
						setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
					},
				},
				"destination_tags": schema.SetAttribute{
					Optional:    true,
					ElementType: types.StringType,
				},
			},
		},
	}
}

// ----- Configure -----

func (r *firewallResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *config.CombinedConfig, got %T.", req.ProviderData),
		)
		return
	}
	r.client = cfg.GodoClient()
}

// ----- ModifyPlan: cross-attribute validation -----
//
// Translation of the SDKv2 `CustomizeDiff` block. The original
// validated three things:
//   1. At least one of `inbound_rule` / `outbound_rule` must be set.
//   2. Every inbound rule whose protocol != "icmp" must have a
//      `port_range`.
//   3. Same rule for outbound rules.
// All three are cross-attribute / cross-block checks, so they belong
// at the resource level rather than as per-attribute validators.
//
// In the framework that means `ResourceWithModifyPlan.ModifyPlan`
// (per references/plan-modifiers.md, "CustomizeDiff -> ModifyPlan").
// We short-circuit on the destroy phase (Plan.Raw.IsNull()) because
// there's nothing to validate when the resource is being removed.
func (r *firewallResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Destroy: practitioner removed the resource. No plan to validate.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan firewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Rule 1: at least one rule must be specified. Mirrors the
	// SDKv2 check `!hasInbound && !hasOutbound`.
	if len(plan.InboundRules) == 0 && len(plan.OutboundRules) == 0 {
		resp.Diagnostics.AddError(
			"At least one rule must be specified",
			"A digitalocean_firewall resource must declare at least one inbound_rule or outbound_rule block.",
		)
		return
	}

	// Rule 2: inbound rules require a port_range unless icmp.
	for i, rule := range plan.InboundRules {
		if rule.Protocol.IsNull() || rule.Protocol.IsUnknown() {
			continue
		}
		if rule.Protocol.ValueString() == "icmp" {
			continue
		}
		if rule.PortRange.IsNull() || rule.PortRange.IsUnknown() || rule.PortRange.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("inbound_rule").AtListIndex(i).AtName("port_range"),
				"Missing port_range",
				"`port_range` of inbound rules is required if protocol is `tcp` or `udp`",
			)
		}
	}

	// Rule 3: same shape for outbound rules.
	for i, rule := range plan.OutboundRules {
		if rule.Protocol.IsNull() || rule.Protocol.IsUnknown() {
			continue
		}
		if rule.Protocol.ValueString() == "icmp" {
			continue
		}
		if rule.PortRange.IsNull() || rule.PortRange.IsUnknown() || rule.PortRange.ValueString() == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("outbound_rule").AtListIndex(i).AtName("port_range"),
				"Missing port_range",
				"`port_range` of outbound rules is required if protocol is `tcp` or `udp`",
			)
		}
	}
}

// ----- Import -----

func (r *firewallResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ----- Create -----

func (r *firewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts, diags := buildFirewallRequest(ctx, &plan)
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

	resp.Diagnostics.Append(applyFirewallToModel(ctx, fw, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ----- Read -----

func (r *firewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallResourceModel
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

	resp.Diagnostics.Append(applyFirewallToModel(ctx, fw, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ----- Update -----

func (r *firewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state firewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	opts, diags := buildFirewallRequest(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Firewall update configuration: %#v", opts)
	_, _, err := r.client.Firewalls.Update(ctx, plan.ID.ValueString(), opts)
	if err != nil {
		resp.Diagnostics.AddError("Error updating firewall", err.Error())
		return
	}

	fw, _, err := r.client.Firewalls.Get(ctx, plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving firewall after update", err.Error())
		return
	}
	resp.Diagnostics.Append(applyFirewallToModel(ctx, fw, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ----- Delete -----

func (r *firewallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[INFO] Deleting firewall: %s", state.ID.ValueString())
	_, err := r.client.Firewalls.Delete(ctx, state.ID.ValueString())
	if err != nil {
		// Mirror the SDKv2 behaviour of treating a missing remote
		// firewall as a successful delete.
		if strings.Contains(err.Error(), "404 Not Found") {
			return
		}
		resp.Diagnostics.AddError("Error deleting firewall", err.Error())
		return
	}
}

// ----- Helpers: build request / apply API response -----

// buildFirewallRequest projects the typed plan model onto a godo
// FirewallRequest. Each helper in this section replaces an SDKv2
// expand*/flatten* helper that operated on `*schema.Set`/
// `interface{}` collections in firewalls.go; we keep the migration
// self-contained so resource_firewall.go has no SDKv2 imports.
func buildFirewallRequest(ctx context.Context, plan *firewallResourceModel) (*godo.FirewallRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	opts := &godo.FirewallRequest{
		Name: plan.Name.ValueString(),
	}

	dropletIDs, d := setToInts(ctx, plan.DropletIDs)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}
	opts.DropletIDs = dropletIDs

	tags, d := setToStrings(ctx, plan.Tags)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}
	opts.Tags = tags

	for _, rule := range plan.InboundRules {
		var src godo.Sources

		ids, d := setToInts(ctx, rule.SourceDropletIDs)
		diags.Append(d...)
		src.DropletIDs = ids

		addrs, d := setToStrings(ctx, rule.SourceAddresses)
		diags.Append(d...)
		src.Addresses = addrs

		lbs, d := setToStrings(ctx, rule.SourceLoadBalancerUIDs)
		diags.Append(d...)
		src.LoadBalancerUIDs = lbs

		k8s, d := setToStrings(ctx, rule.SourceKubernetesIDs)
		diags.Append(d...)
		src.KubernetesIDs = k8s

		srcTags, d := setToStrings(ctx, rule.SourceTags)
		diags.Append(d...)
		src.Tags = srcTags

		opts.InboundRules = append(opts.InboundRules, godo.InboundRule{
			Protocol:  rule.Protocol.ValueString(),
			PortRange: rule.PortRange.ValueString(),
			Sources:   &src,
		})
	}

	for _, rule := range plan.OutboundRules {
		var dest godo.Destinations

		ids, d := setToInts(ctx, rule.DestinationDropletIDs)
		diags.Append(d...)
		dest.DropletIDs = ids

		addrs, d := setToStrings(ctx, rule.DestinationAddresses)
		diags.Append(d...)
		dest.Addresses = addrs

		lbs, d := setToStrings(ctx, rule.DestinationLoadBalancerUIDs)
		diags.Append(d...)
		dest.LoadBalancerUIDs = lbs

		k8s, d := setToStrings(ctx, rule.DestinationKubernetesIDs)
		diags.Append(d...)
		dest.KubernetesIDs = k8s

		dTags, d := setToStrings(ctx, rule.DestinationTags)
		diags.Append(d...)
		dest.Tags = dTags

		opts.OutboundRules = append(opts.OutboundRules, godo.OutboundRule{
			Protocol:     rule.Protocol.ValueString(),
			PortRange:    rule.PortRange.ValueString(),
			Destinations: &dest,
		})
	}

	if diags.HasError() {
		return nil, diags
	}
	return opts, diags
}

// applyFirewallToModel writes API response data into the model in
// place. Used by Create/Read/Update so the same code path populates
// every Computed field.
func applyFirewallToModel(ctx context.Context, fw *godo.Firewall, m *firewallResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.StringValue(fw.ID)
	m.Name = types.StringValue(fw.Name)
	m.Status = types.StringValue(fw.Status)
	m.CreatedAt = types.StringValue(fw.Created)

	dropletIDs, d := intsToSet(fw.DropletIDs)
	diags.Append(d...)
	m.DropletIDs = dropletIDs

	tags, d := stringsToSet(fw.Tags)
	diags.Append(d...)
	m.Tags = tags

	m.InboundRules = flattenInboundRules(ctx, fw.InboundRules, &diags)
	m.OutboundRules = flattenOutboundRules(ctx, fw.OutboundRules, &diags)

	m.PendingChanges = make([]pendingChangeModel, 0, len(fw.PendingChanges))
	for _, pc := range fw.PendingChanges {
		m.PendingChanges = append(m.PendingChanges, pendingChangeModel{
			DropletID: types.Int64Value(int64(pc.DropletID)),
			Removing:  types.BoolValue(pc.Removing),
			Status:    types.StringValue(pc.Status),
		})
	}

	return diags
}

// ----- set/list <-> typed.Set conversions -----

func setToStrings(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	out := make([]string, 0, len(s.Elements()))
	diags := s.ElementsAs(ctx, &out, false)
	return out, diags
}

func setToInts(ctx context.Context, s types.Set) ([]int, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	out := make([]int, 0, len(s.Elements()))
	diags := s.ElementsAs(ctx, &out, false)
	return out, diags
}

func stringsToSet(in []string) (types.Set, diag.Diagnostics) {
	if in == nil {
		return types.SetNull(types.StringType), nil
	}
	elems := make([]attr.Value, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		elems = append(elems, types.StringValue(v))
	}
	return types.SetValue(types.StringType, elems)
}

func intsToSet(in []int) (types.Set, diag.Diagnostics) {
	if in == nil {
		return types.SetNull(types.Int64Type), nil
	}
	elems := make([]attr.Value, 0, len(in))
	for _, v := range in {
		elems = append(elems, types.Int64Value(int64(v)))
	}
	return types.SetValue(types.Int64Type, elems)
}

// ----- inbound/outbound rule flatten helpers -----
//
// These mirror the SDKv2 flattenFirewall{Inbound,Outbound}Rules funcs:
// they translate the API response into the typed model, applying the
// "API returns 0 for `all`" special-case so users see `port_range =
// "all"` in state rather than the literal "0".

func flattenInboundRules(ctx context.Context, rules []godo.InboundRule, diags *diag.Diagnostics) []inboundRuleModel {
	if rules == nil {
		return nil
	}
	out := make([]inboundRuleModel, 0, len(rules))
	for _, rule := range rules {
		m := inboundRuleModel{
			Protocol:               types.StringValue(rule.Protocol),
			SourceAddresses:        types.SetNull(types.StringType),
			SourceDropletIDs:       types.SetNull(types.Int64Type),
			SourceLoadBalancerUIDs: types.SetNull(types.StringType),
			SourceKubernetesIDs:    types.SetNull(types.StringType),
			SourceTags:             types.SetNull(types.StringType),
		}
		m.PortRange = portRangeForState(rule.Protocol, rule.PortRange)

		if rule.Sources != nil {
			if rule.Sources.Addresses != nil {
				v, d := stringsToSet(rule.Sources.Addresses)
				diags.Append(d...)
				m.SourceAddresses = v
			}
			if rule.Sources.DropletIDs != nil {
				v, d := intsToSet(rule.Sources.DropletIDs)
				diags.Append(d...)
				m.SourceDropletIDs = v
			}
			if rule.Sources.LoadBalancerUIDs != nil {
				v, d := stringsToSet(rule.Sources.LoadBalancerUIDs)
				diags.Append(d...)
				m.SourceLoadBalancerUIDs = v
			}
			if rule.Sources.KubernetesIDs != nil {
				v, d := stringsToSet(rule.Sources.KubernetesIDs)
				diags.Append(d...)
				m.SourceKubernetesIDs = v
			}
			if rule.Sources.Tags != nil {
				v, d := stringsToSet(rule.Sources.Tags)
				diags.Append(d...)
				m.SourceTags = v
			}
		}
		out = append(out, m)
	}
	return out
}

func flattenOutboundRules(ctx context.Context, rules []godo.OutboundRule, diags *diag.Diagnostics) []outboundRuleModel {
	if rules == nil {
		return nil
	}
	out := make([]outboundRuleModel, 0, len(rules))
	for _, rule := range rules {
		m := outboundRuleModel{
			Protocol:                    types.StringValue(rule.Protocol),
			DestinationAddresses:        types.SetNull(types.StringType),
			DestinationDropletIDs:       types.SetNull(types.Int64Type),
			DestinationLoadBalancerUIDs: types.SetNull(types.StringType),
			DestinationKubernetesIDs:    types.SetNull(types.StringType),
			DestinationTags:             types.SetNull(types.StringType),
		}
		m.PortRange = portRangeForState(rule.Protocol, rule.PortRange)

		if rule.Destinations != nil {
			if rule.Destinations.Addresses != nil {
				v, d := stringsToSet(rule.Destinations.Addresses)
				diags.Append(d...)
				m.DestinationAddresses = v
			}
			if rule.Destinations.DropletIDs != nil {
				v, d := intsToSet(rule.Destinations.DropletIDs)
				diags.Append(d...)
				m.DestinationDropletIDs = v
			}
			if rule.Destinations.LoadBalancerUIDs != nil {
				v, d := stringsToSet(rule.Destinations.LoadBalancerUIDs)
				diags.Append(d...)
				m.DestinationLoadBalancerUIDs = v
			}
			if rule.Destinations.KubernetesIDs != nil {
				v, d := stringsToSet(rule.Destinations.KubernetesIDs)
				diags.Append(d...)
				m.DestinationKubernetesIDs = v
			}
			if rule.Destinations.Tags != nil {
				v, d := stringsToSet(rule.Destinations.Tags)
				diags.Append(d...)
				m.DestinationTags = v
			}
		}
		out = append(out, m)
	}
	return out
}

// portRangeForState replicates the SDKv2 flatten helper's "API
// returns 0 means all (for non-icmp)" mapping.
func portRangeForState(protocol, portRange string) types.String {
	if portRange == "0" {
		if protocol != "icmp" {
			return types.StringValue("all")
		}
		return types.StringNull()
	}
	if portRange == "" {
		return types.StringNull()
	}
	return types.StringValue(portRange)
}

