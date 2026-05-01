package domain

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
	linodesetplanmodifiers "github.com/linode/terraform-provider-linode/v3/linode/helper/setplanmodifiers"
)

const domainSecondsDescription = "Valid values are 0, 30, 120, 300, 3600, 7200, 14400, 28800, " +
	"57600, 86400, 172800, 345600, 604800, 1209600, and 2419200 - any other value will be rounded to " +
	"the nearest valid value."

// domainSecondsAccepted is the list of valid Linode domain TTL-like values.
var domainSecondsAccepted = []int64{
	30, 120, 300, 3600, 7200, 14400, 28800, 57600, 86400, 172800, 345600, 604800, 1209600, 2419200,
}

// roundDomainSeconds rounds n to the nearest accepted Linode domain seconds value.
func roundDomainSeconds(n int64) int64 {
	if n == 0 {
		return 0
	}
	for _, v := range domainSecondsAccepted {
		if n <= v {
			return v
		}
	}
	return domainSecondsAccepted[len(domainSecondsAccepted)-1]
}

// domainSecondsPlanModifier is a plan modifier that suppresses diffs when the
// planned value rounds to the same accepted Linode domain seconds value as the
// state value (equivalent to SDK DiffSuppressFunc via helper.DomainSecondsDiffSuppressor).
type domainSecondsPlanModifier struct{}

func DomainSecondsPlanModifier() planmodifier.Int64 {
	return &domainSecondsPlanModifier{}
}

func (m *domainSecondsPlanModifier) Description(_ context.Context) string {
	return "Suppresses diffs when the planned value rounds to the same accepted domain seconds value as the state value."
}

func (m *domainSecondsPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m *domainSecondsPlanModifier) PlanModifyInt64(
	_ context.Context,
	req planmodifier.Int64Request,
	resp *planmodifier.Int64Response,
) {
	// Only act when both plan and state are known.
	if req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		return
	}
	if req.StateValue.IsUnknown() || req.StateValue.IsNull() {
		return
	}

	planned := req.PlanValue.ValueInt64()
	state := req.StateValue.ValueInt64()

	// If the rounded planned value equals the current state value, suppress the diff
	// by keeping the state value in the plan.
	if roundDomainSeconds(planned) == state {
		resp.PlanValue = req.StateValue
	}
}

// frameworkResourceSchema is the terraform-plugin-framework schema for linode_domain.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			Description: "The unique ID of the domain.",
			Computed:    true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"domain": schema.StringAttribute{
			Description: "The domain this Domain represents. These must be unique in our system; you cannot have " +
				"two Domains representing the same domain.",
			Required: true,
		},
		"type": schema.StringAttribute{
			Description: "If this Domain represents the authoritative source of information for the domain it " +
				"describes, or if it is a read-only copy of a master (also called a slave).",
			Required: true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
			Validators: []validator.String{
				stringvalidator.OneOf("master", "slave"),
			},
		},
		"group": schema.StringAttribute{
			Description: "The group this Domain belongs to. This is for display purposes only.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
			Validators: []validator.String{
				stringvalidator.LengthBetween(0, 50),
			},
		},
		"status": schema.StringAttribute{
			Description: "Used to control whether this Domain is currently being rendered.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("active"),
		},
		"description": schema.StringAttribute{
			Description: "A description for this Domain. This is for display purposes only.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
			Validators: []validator.String{
				stringvalidator.LengthBetween(0, 253),
			},
		},
		"master_ips": schema.SetAttribute{
			ElementType: types.StringType,
			Description: "The IP addresses representing the master DNS for this Domain.",
			Optional:    true,
			Computed:    true,
			Default:     helper.EmptySetDefault(types.StringType),
		},
		"axfr_ips": schema.SetAttribute{
			ElementType: types.StringType,
			Description: "The list of IPs that may perform a zone transfer for this Domain. This is potentially " +
				"dangerous, and should be set to an empty list unless you intend to use it.",
			Optional: true,
			Computed: true,
			Default:  helper.EmptySetDefault(types.StringType),
		},
		"ttl_sec": schema.Int64Attribute{
			Description: "'Time to Live' - the amount of time in seconds that this Domain's records may be " +
				"cached by resolvers or other domain servers. " + domainSecondsDescription,
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(0),
			PlanModifiers: []planmodifier.Int64{
				DomainSecondsPlanModifier(),
			},
		},
		"retry_sec": schema.Int64Attribute{
			Description: "The interval, in seconds, at which a failed refresh should be retried. " +
				domainSecondsDescription,
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(0),
			PlanModifiers: []planmodifier.Int64{
				DomainSecondsPlanModifier(),
			},
		},
		"expire_sec": schema.Int64Attribute{
			Description: "The amount of time in seconds that may pass before this Domain is no longer " +
				domainSecondsDescription,
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(0),
			PlanModifiers: []planmodifier.Int64{
				DomainSecondsPlanModifier(),
			},
		},
		"refresh_sec": schema.Int64Attribute{
			Description: "The amount of time in seconds before this Domain should be refreshed. " +
				domainSecondsDescription,
			Optional: true,
			Computed: true,
			Default:  int64default.StaticInt64(0),
			PlanModifiers: []planmodifier.Int64{
				DomainSecondsPlanModifier(),
			},
		},
		"soa_email": schema.StringAttribute{
			Description: "Start of Authority email address. This is required for master Domains.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString(""),
		},
		"tags": schema.SetAttribute{
			ElementType: types.StringType,
			Optional:    true,
			Computed:    true,
			Description: "An array of tags applied to this object. Tags are for organizational purposes only.",
			Default:     helper.EmptySetDefault(types.StringType),
			PlanModifiers: []planmodifier.Set{
				// Equivalent to customdiff.CaseInsensitiveSet("tags")
				linodesetplanmodifiers.CaseInsensitiveSet(),
			},
		},
	},
}

// DomainResourceModel holds the Terraform state for a linode_domain resource.
type DomainResourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Domain      types.String `tfsdk:"domain"`
	Type        types.String `tfsdk:"type"`
	Group       types.String `tfsdk:"group"`
	Status      types.String `tfsdk:"status"`
	Description types.String `tfsdk:"description"`
	MasterIPs   types.Set    `tfsdk:"master_ips"`
	AXFRIPs     types.Set    `tfsdk:"axfr_ips"`
	TTLSec      types.Int64  `tfsdk:"ttl_sec"`
	RetrySec    types.Int64  `tfsdk:"retry_sec"`
	ExpireSec   types.Int64  `tfsdk:"expire_sec"`
	RefreshSec  types.Int64  `tfsdk:"refresh_sec"`
	SOAEmail    types.String `tfsdk:"soa_email"`
	Tags        types.Set    `tfsdk:"tags"`
}

// flattenDomain populates a DomainResourceModel from a linodego.Domain.
func (m *DomainResourceModel) flattenDomain(ctx context.Context, domain *linodego.Domain, diags *[]string) {
	m.ID = types.Int64Value(int64(domain.ID))
	m.Domain = types.StringValue(domain.Domain)
	m.Type = types.StringValue(string(domain.Type))
	m.Group = types.StringValue(domain.Group)
	m.Status = types.StringValue(string(domain.Status))
	m.Description = types.StringValue(domain.Description)
	m.TTLSec = types.Int64Value(int64(domain.TTLSec))
	m.RetrySec = types.Int64Value(int64(domain.RetrySec))
	m.ExpireSec = types.Int64Value(int64(domain.ExpireSec))
	m.RefreshSec = types.Int64Value(int64(domain.RefreshSec))
	m.SOAEmail = types.StringValue(domain.SOAEmail)

	masterIPs := make([]attr.Value, len(domain.MasterIPs))
	for i, ip := range domain.MasterIPs {
		masterIPs[i] = types.StringValue(ip)
	}
	masterIPsSet, d := types.SetValue(types.StringType, masterIPs)
	if d.HasError() {
		*diags = append(*diags, fmt.Sprintf("failed to convert master_ips: %v", d.Errors()))
	} else {
		m.MasterIPs = masterIPsSet
	}

	axfrIPs := make([]attr.Value, len(domain.AXfrIPs))
	for i, ip := range domain.AXfrIPs {
		axfrIPs[i] = types.StringValue(ip)
	}
	axfrIPsSet, d := types.SetValue(types.StringType, axfrIPs)
	if d.HasError() {
		*diags = append(*diags, fmt.Sprintf("failed to convert axfr_ips: %v", d.Errors()))
	} else {
		m.AXFRIPs = axfrIPsSet
	}

	tags := make([]attr.Value, len(domain.Tags))
	for i, tag := range domain.Tags {
		tags[i] = types.StringValue(tag)
	}
	tagsSet, d := types.SetValue(types.StringType, tags)
	if d.HasError() {
		*diags = append(*diags, fmt.Sprintf("failed to convert tags: %v", d.Errors()))
	} else {
		m.Tags = tagsSet
	}
}

// DomainResource is the framework resource implementation.
type DomainResource struct {
	helper.BaseResource
}

func NewResource() resource.Resource {
	return &DomainResource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_domain",
				IDAttr: "id",
				IDType: types.Int64Type,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

func (r *DomainResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_domain")

	var plan DomainResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client

	masterIPs := make([]string, 0)
	resp.Diagnostics.Append(plan.MasterIPs.ElementsAs(ctx, &masterIPs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	axfrIPs := make([]string, 0)
	resp.Diagnostics.Append(plan.AXFRIPs.ElementsAs(ctx, &axfrIPs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tags := make([]string, 0)
	resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createOpts := linodego.DomainCreateOptions{
		Domain:      plan.Domain.ValueString(),
		Type:        linodego.DomainType(plan.Type.ValueString()),
		Group:       plan.Group.ValueString(),
		Description: plan.Description.ValueString(),
		SOAEmail:    plan.SOAEmail.ValueString(),
		RetrySec:    int(plan.RetrySec.ValueInt64()),
		ExpireSec:   int(plan.ExpireSec.ValueInt64()),
		RefreshSec:  int(plan.RefreshSec.ValueInt64()),
		TTLSec:      int(plan.TTLSec.ValueInt64()),
		Tags:        tags,
	}

	if len(masterIPs) > 0 {
		createOpts.MasterIPs = masterIPs
	}

	if len(axfrIPs) > 0 {
		createOpts.AXfrIPs = axfrIPs
	}

	tflog.Debug(ctx, "client.CreateDomain(...)", map[string]any{
		"options": createOpts,
	})

	domain, err := client.CreateDomain(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Linode Domain",
			err.Error(),
		)
		return
	}

	plan.ID = types.Int64Value(int64(domain.ID))
	ctx = tflog.SetField(ctx, "domain_id", domain.ID)

	// Re-read to pick up server-side defaults.
	domain, err = client.GetDomain(ctx, domain.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading created Linode Domain",
			err.Error(),
		)
		return
	}

	var flattenErrors []string
	plan.flattenDomain(ctx, domain, &flattenErrors)
	for _, e := range flattenErrors {
		resp.Diagnostics.AddError("Error flattening domain", e)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DomainResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_domain")

	var state DomainResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "domain_id", state.ID.ValueInt64())

	client := r.Meta.Client

	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := client.GetDomain(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			resp.Diagnostics.AddWarning(
				"Domain no longer exists",
				fmt.Sprintf("Removing Linode Domain ID %d from state because it no longer exists", id),
			)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Linode Domain",
			err.Error(),
		)
		return
	}

	var flattenErrors []string
	state.flattenDomain(ctx, domain, &flattenErrors)
	for _, e := range flattenErrors {
		resp.Diagnostics.AddError("Error flattening domain", e)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DomainResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_domain")

	var plan, state DomainResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "domain_id", state.ID.ValueInt64())

	client := r.Meta.Client

	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	masterIPs := make([]string, 0)
	resp.Diagnostics.Append(plan.MasterIPs.ElementsAs(ctx, &masterIPs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	axfrIPs := make([]string, 0)
	resp.Diagnostics.Append(plan.AXFRIPs.ElementsAs(ctx, &axfrIPs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tags := make([]string, 0)
	resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateOpts := linodego.DomainUpdateOptions{
		Domain:      plan.Domain.ValueString(),
		Status:      linodego.DomainStatus(plan.Status.ValueString()),
		Group:       plan.Group.ValueString(),
		Description: plan.Description.ValueString(),
		SOAEmail:    plan.SOAEmail.ValueString(),
		RetrySec:    int(plan.RetrySec.ValueInt64()),
		ExpireSec:   int(plan.ExpireSec.ValueInt64()),
		RefreshSec:  int(plan.RefreshSec.ValueInt64()),
		TTLSec:      int(plan.TTLSec.ValueInt64()),
		Tags:        tags,
		MasterIPs:   masterIPs,
		AXfrIPs:     axfrIPs,
	}

	tflog.Debug(ctx, "client.UpdateDomain(...)", map[string]any{
		"options": updateOpts,
	})

	_, err := client.UpdateDomain(ctx, id, updateOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating Linode Domain %d", id),
			err.Error(),
		)
		return
	}

	// Re-read to get authoritative state.
	domain, err := client.GetDomain(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading updated Linode Domain",
			err.Error(),
		)
		return
	}

	plan.ID = state.ID

	var flattenErrors []string
	plan.flattenDomain(ctx, domain, &flattenErrors)
	for _, e := range flattenErrors {
		resp.Diagnostics.AddError("Error flattening domain", e)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DomainResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_domain")

	var state DomainResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "domain_id", state.ID.ValueInt64())

	client := r.Meta.Client

	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "client.DeleteDomain(...)")

	err := client.DeleteDomain(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting Linode Domain %d", id),
			err.Error(),
		)
		return
	}
}

// ModifyPlan implements the framework equivalent of the SDK CustomizeDiff.
//
// The SDKv2 resource used:
//   - customdiff.ComputedWithDefault("tags", []string{}) – handled by Default+Computed in schema
//   - customdiff.CaseInsensitiveSet("tags") – handled by setplanmodifiers.CaseInsensitiveSet() in schema
//
// No additional ModifyPlan logic is needed here because both behaviours are
// encoded directly in the schema attribute definitions above.
func (r *DomainResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	// Both ComputedWithDefault and CaseInsensitiveSet are handled declaratively
	// in the schema (Default + CaseInsensitiveSet plan modifier). Nothing
	// additional is required here.
}
