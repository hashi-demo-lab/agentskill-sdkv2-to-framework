package domain

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
	"github.com/linode/terraform-provider-linode/v3/linode/helper/setplanmodifiers"
)

// domainSecondsDescription is the shared description suffix for TTL-like domain fields
// (previously in schema_resource.go alongside the SDKv2 schema).
const domainSecondsDescription = "Valid values are 0, 30, 120, 300, 3600, 7200, 14400, 28800, " +
	"57600, 86400, 172800, 345600, 604800, 1209600, and 2419200 - any other value will be rounded to " +
	"the nearest valid value."

// domainSecondsAccepted is the ordered list of valid second values for domain TTL-like fields.
// The Linode API rounds any other value to the nearest entry in this list.
var domainSecondsAccepted = []int64{
	30, 120, 300, 3600, 7200, 14400, 28800, 57600, 86400, 172800, 345600, 604800, 1209600, 2419200,
}

// roundDomainSeconds returns the nearest accepted value for a domain TTL-like field.
// A value of 0 remains 0. Values exceeding the maximum are clamped to the maximum.
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

// domainSecondsPlanModifier is a plan modifier for int64 TTL-like domain fields.
//
// Migrated from SDKv2 DiffSuppressFunc (helper.DomainSecondsDiffSuppressor).
//
// It adjusts the planned value to the rounded value the API will actually return,
// preventing spurious perpetual diffs when the practitioner specifies a value between
// two valid steps. This modifier is non-destructive: the Create/Update functions still
// send the user's original value to the API, which performs the actual rounding.
type domainSecondsPlanModifier struct{}

func (m domainSecondsPlanModifier) Description(_ context.Context) string {
	return "Rounds the planned value to the nearest valid domain-seconds boundary to suppress diffs after the API normalises the field."
}

func (m domainSecondsPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m domainSecondsPlanModifier) PlanModifyInt64(
	_ context.Context,
	req planmodifier.Int64Request,
	resp *planmodifier.Int64Response,
) {
	// Leave null or unknown values alone — nothing to round.
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}
	resp.PlanValue = types.Int64Value(roundDomainSeconds(req.PlanValue.ValueInt64()))
}

// domainSecondsMod returns the plan modifier for domain TTL-like int64 fields.
func domainSecondsMod() planmodifier.Int64 {
	return domainSecondsPlanModifier{}
}

// emptyStringSet is a reusable empty set default for set attributes.
var emptyStringSet = func() types.Set {
	v, _ := types.SetValue(types.StringType, []attr.Value{})
	return v
}()

// frameworkResourceSchema is the plugin-framework schema for the linode_domain resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The unique ID of this Domain.",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
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
			Validators: []validator.String{
				stringvalidator.OneOf("master", "slave"),
			},
			PlanModifiers: []planmodifier.String{
				// ForceNew: true in SDKv2.
				stringplanmodifier.RequiresReplace(),
			},
		},
		"group": schema.StringAttribute{
			Description: "The group this Domain belongs to. This is for display purposes only.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.String{
				stringvalidator.LengthBetween(0, 50),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"status": schema.StringAttribute{
			Description: "Used to control whether this Domain is currently being rendered.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"description": schema.StringAttribute{
			Description: "A description for this Domain. This is for display purposes only.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.String{
				stringvalidator.LengthBetween(0, 253),
			},
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"master_ips": schema.SetAttribute{
			Description: "The IP addresses representing the master DNS for this Domain.",
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
		},
		"axfr_ips": schema.SetAttribute{
			Description: "The list of IPs that may perform a zone transfer for this Domain. This is potentially " +
				"dangerous, and should be set to an empty list unless you intend to use it.",
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
		},
		// DiffSuppressFunc helper.DomainSecondsDiffSuppressor() migrated to domainSecondsPlanModifier.
		"ttl_sec": schema.Int64Attribute{
			Description: "'Time to Live' - the amount of time in seconds that this Domain's records may be " +
				"cached by resolvers or other domain servers. " + domainSecondsDescription,
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
				domainSecondsMod(),
			},
		},
		// DiffSuppressFunc helper.DomainSecondsDiffSuppressor() migrated to domainSecondsPlanModifier.
		"retry_sec": schema.Int64Attribute{
			Description: "The interval, in seconds, at which a failed refresh should be retried. " +
				domainSecondsDescription,
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
				domainSecondsMod(),
			},
		},
		// DiffSuppressFunc helper.DomainSecondsDiffSuppressor() migrated to domainSecondsPlanModifier.
		"expire_sec": schema.Int64Attribute{
			Description: "The amount of time in seconds that may pass before this Domain is no longer " +
				domainSecondsDescription,
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
				domainSecondsMod(),
			},
		},
		// DiffSuppressFunc helper.DomainSecondsDiffSuppressor() migrated to domainSecondsPlanModifier.
		"refresh_sec": schema.Int64Attribute{
			Description: "The amount of time in seconds before this Domain should be refreshed. " +
				domainSecondsDescription,
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
				domainSecondsMod(),
			},
		},
		"soa_email": schema.StringAttribute{
			Description: "Start of Authority email address. This is required for master Domains.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"tags": schema.SetAttribute{
			Description: "An array of tags applied to this object. Tags are for organizational purposes only.",
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
			// ComputedWithDefault("tags", []string{}) migrated: Default ensures empty set
			// when the practitioner omits the attribute.
			Default: setdefault.StaticValue(emptyStringSet),
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
				// CaseInsensitiveSet CustomizeDiff migrated to a plan modifier.
				setplanmodifiers.CaseInsensitiveSet(),
			},
		},
	},
}

// ResourceModel is the Terraform state model for the linode_domain resource.
type ResourceModel struct {
	ID          types.String   `tfsdk:"id"`
	Domain      types.String   `tfsdk:"domain"`
	Type        types.String   `tfsdk:"type"`
	Group       types.String   `tfsdk:"group"`
	Status      types.String   `tfsdk:"status"`
	Description types.String   `tfsdk:"description"`
	MasterIPs   types.Set      `tfsdk:"master_ips"`
	AXFRIPs     types.Set      `tfsdk:"axfr_ips"`
	TTLSec      types.Int64    `tfsdk:"ttl_sec"`
	RetrySec    types.Int64    `tfsdk:"retry_sec"`
	ExpireSec   types.Int64    `tfsdk:"expire_sec"`
	RefreshSec  types.Int64    `tfsdk:"refresh_sec"`
	SOAEmail    types.String   `tfsdk:"soa_email"`
	Tags        []types.String `tfsdk:"tags"`
}

func (m *ResourceModel) parseDomain(ctx context.Context, domain *linodego.Domain) {
	m.ID = types.StringValue(strconv.Itoa(domain.ID))
	m.Domain = types.StringValue(domain.Domain)
	m.Type = types.StringValue(string(domain.Type))
	m.Group = types.StringValue(domain.Group)
	m.Status = types.StringValue(string(domain.Status))
	m.Description = types.StringValue(domain.Description)
	m.SOAEmail = types.StringValue(domain.SOAEmail)
	m.TTLSec = types.Int64Value(int64(domain.TTLSec))
	m.RetrySec = types.Int64Value(int64(domain.RetrySec))
	m.ExpireSec = types.Int64Value(int64(domain.ExpireSec))
	m.RefreshSec = types.Int64Value(int64(domain.RefreshSec))
	m.Tags = helper.StringSliceToFramework(domain.Tags)

	masterIPs, _ := types.SetValueFrom(ctx, types.StringType, domain.MasterIPs)
	m.MasterIPs = masterIPs

	axfrIPs, _ := types.SetValueFrom(ctx, types.StringType, domain.AXfrIPs)
	m.AXFRIPs = axfrIPs
}

var (
	_ resource.Resource                = &DomainResource{}
	_ resource.ResourceWithConfigure   = &DomainResource{}
	_ resource.ResourceWithImportState = &DomainResource{}
	_ resource.ResourceWithModifyPlan  = &DomainResource{}
)

// NewResource returns a new plugin-framework resource for linode_domain.
func NewResource() resource.Resource {
	return &DomainResource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_domain",
				IDAttr: "id",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

// DomainResource implements the linode_domain resource using terraform-plugin-framework.
type DomainResource struct {
	helper.BaseResource
}

// ModifyPlan implements resource.ResourceWithModifyPlan.
//
// Migrated from SDKv2 CustomizeDiff:
//   - linodediffs.ComputedWithDefault("tags", []string{}) — handled via setdefault.StaticValue
//     on the tags attribute; no additional plan logic needed here. The ModifyPlan body is kept
//     as a guard to ensure destroy short-circuits cleanly.
//   - linodediffs.CaseInsensitiveSet("tags") — handled by setplanmodifiers.CaseInsensitiveSet()
//     on the tags attribute plan modifier.
func (r *DomainResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	// Short-circuit on destroy — no plan modifications needed.
	if req.Plan.Raw.IsNull() {
		return
	}
}

func (r *DomainResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_domain")

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client

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
	}

	for _, tag := range plan.Tags {
		if !tag.IsNull() && !tag.IsUnknown() {
			createOpts.Tags = append(createOpts.Tags, tag.ValueString())
		}
	}

	if !plan.MasterIPs.IsNull() && !plan.MasterIPs.IsUnknown() {
		var masterIPs []string
		resp.Diagnostics.Append(plan.MasterIPs.ElementsAs(ctx, &masterIPs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.MasterIPs = masterIPs
	}

	if !plan.AXFRIPs.IsNull() && !plan.AXFRIPs.IsUnknown() {
		var axfrIPs []string
		resp.Diagnostics.Append(plan.AXFRIPs.ElementsAs(ctx, &axfrIPs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.AXfrIPs = axfrIPs
	}

	tflog.Debug(ctx, "client.CreateDomain(...)", map[string]any{
		"options": createOpts,
	})

	domain, err := client.CreateDomain(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Linode Domain", err.Error())
		return
	}

	plan.parseDomain(ctx, domain)

	ctx = tflog.SetField(ctx, "domain_id", domain.ID)
	tflog.Debug(ctx, "Created linode_domain", map[string]any{"id": domain.ID})

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DomainResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "domain_id", state.ID.ValueString())
	tflog.Debug(ctx, "Read linode_domain")

	client := r.Meta.Client

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing Linode Domain ID",
			fmt.Sprintf("Error parsing Domain ID %s as int: %s", state.ID.ValueString(), err),
		)
		return
	}

	domain, err := client.GetDomain(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			// Resource gone — remove from state to trigger recreation.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Linode Domain", err.Error())
		return
	}

	state.parseDomain(ctx, domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *DomainResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan, state ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "domain_id", state.ID.ValueString())
	tflog.Debug(ctx, "Update linode_domain")

	client := r.Meta.Client

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing Linode Domain ID",
			fmt.Sprintf("Error parsing Domain ID %s as int: %s", state.ID.ValueString(), err),
		)
		return
	}

	// status is Computed — read from plan if it has a known value, else fall back to state.
	status := plan.Status
	if status.IsUnknown() {
		status = state.Status
	}

	updateOpts := linodego.DomainUpdateOptions{
		Domain:      plan.Domain.ValueString(),
		Status:      linodego.DomainStatus(status.ValueString()),
		Group:       plan.Group.ValueString(),
		Description: plan.Description.ValueString(),
		SOAEmail:    plan.SOAEmail.ValueString(),
		RetrySec:    int(plan.RetrySec.ValueInt64()),
		ExpireSec:   int(plan.ExpireSec.ValueInt64()),
		RefreshSec:  int(plan.RefreshSec.ValueInt64()),
		TTLSec:      int(plan.TTLSec.ValueInt64()),
	}

	if !plan.MasterIPs.Equal(state.MasterIPs) {
		var masterIPs []string
		resp.Diagnostics.Append(plan.MasterIPs.ElementsAs(ctx, &masterIPs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.MasterIPs = masterIPs
	}

	if !plan.AXFRIPs.Equal(state.AXFRIPs) {
		var axfrIPs []string
		resp.Diagnostics.Append(plan.AXFRIPs.ElementsAs(ctx, &axfrIPs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.AXfrIPs = axfrIPs
	}

	if !tagsEqual(plan.Tags, state.Tags) {
		for _, tag := range plan.Tags {
			if !tag.IsNull() && !tag.IsUnknown() {
				updateOpts.Tags = append(updateOpts.Tags, tag.ValueString())
			}
		}
	}

	tflog.Debug(ctx, "client.UpdateDomain(...)", map[string]any{
		"options": updateOpts,
	})

	domain, err := client.UpdateDomain(ctx, id, updateOpts)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating Linode Domain %d", id),
			err.Error(),
		)
		return
	}

	plan.parseDomain(ctx, domain)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *DomainResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	// Delete reads from req.State, NOT req.Plan (which is null on destroy).
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "domain_id", state.ID.ValueString())
	tflog.Debug(ctx, "Delete linode_domain")

	client := r.Meta.Client

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing Linode Domain ID",
			fmt.Sprintf("Error parsing Domain ID %s as int", state.ID.ValueString()),
		)
		return
	}

	tflog.Debug(ctx, "client.DeleteDomain(...)")

	if err := client.DeleteDomain(ctx, id); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting Linode Domain %d", id),
			err.Error(),
		)
	}
}

func (r *DomainResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	tflog.Debug(ctx, "Import linode_domain")
	// Passthrough the string ID to the "id" attribute; Read will populate all other fields.
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...,
	)
}

// tagsEqual returns true if two []types.String slices contain the same known string values,
// irrespective of order. Used for HasChange-equivalent detection on the tags set.
func tagsEqual(a, b []types.String) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]struct{}, len(a))
	for _, v := range a {
		aMap[v.ValueString()] = struct{}{}
	}
	for _, v := range b {
		if _, ok := aMap[v.ValueString()]; !ok {
			return false
		}
	}
	return true
}
