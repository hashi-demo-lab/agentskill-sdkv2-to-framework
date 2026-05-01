// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	tfe "github.com/hashicorp/go-tfe"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface checks
var _ resource.Resource = &resourceTFETeam{}
var _ resource.ResourceWithConfigure = &resourceTFETeam{}
var _ resource.ResourceWithImportState = &resourceTFETeam{}

// NewTeamResource is the framework resource constructor.
func NewTeamResource() resource.Resource {
	return &resourceTFETeam{}
}

// resourceTFETeam implements the tfe_team resource type.
type resourceTFETeam struct {
	config ConfiguredClient
}

// modelTFETeam maps schema attributes (and the kept-as-block organization_access)
// to a typed Go model. The `organization_access` field is modelled as a list of
// modelTFETeamOrganizationAccess because in SDKv2 it was a TypeList with
// MaxItems:1 — practitioners write it as a block (`organization_access { ... }`)
// and the prior state path was `organization_access.0.foo`. Keeping it as a
// ListNestedBlock + SizeAtMost(1) preserves both the HCL syntax and the
// existing state path so it is not a breaking change.
type modelTFETeam struct {
	ID                         types.String                          `tfsdk:"id"`
	Name                       types.String                          `tfsdk:"name"`
	Organization               types.String                          `tfsdk:"organization"`
	Visibility                 types.String                          `tfsdk:"visibility"`
	SSOTeamID                  types.String                          `tfsdk:"sso_team_id"`
	AllowMemberTokenManagement types.Bool                            `tfsdk:"allow_member_token_management"`
	OrganizationAccess         []modelTFETeamOrganizationAccess      `tfsdk:"organization_access"`
}

// modelTFETeamOrganizationAccess models a single organization_access block
// element. There can be at most one (enforced by listvalidator.SizeAtMost(1)).
type modelTFETeamOrganizationAccess struct {
	ManagePolicies           types.Bool `tfsdk:"manage_policies"`
	ManagePolicyOverrides    types.Bool `tfsdk:"manage_policy_overrides"`
	ManageWorkspaces         types.Bool `tfsdk:"manage_workspaces"`
	ManageVCSSettings        types.Bool `tfsdk:"manage_vcs_settings"`
	ManageProviders          types.Bool `tfsdk:"manage_providers"`
	ManageModules            types.Bool `tfsdk:"manage_modules"`
	ManageRunTasks           types.Bool `tfsdk:"manage_run_tasks"`
	ManageProjects           types.Bool `tfsdk:"manage_projects"`
	ReadWorkspaces           types.Bool `tfsdk:"read_workspaces"`
	ReadProjects             types.Bool `tfsdk:"read_projects"`
	ManageMembership         types.Bool `tfsdk:"manage_membership"`
	ManageTeams              types.Bool `tfsdk:"manage_teams"`
	ManageOrganizationAccess types.Bool `tfsdk:"manage_organization_access"`
	AccessSecretTeams        types.Bool `tfsdk:"access_secret_teams"`
	ManageAgentPools         types.Bool `tfsdk:"manage_agent_pools"`
}

// Metadata implements resource.Resource.
func (r *resourceTFETeam) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

// Configure implements resource.ResourceWithConfigure.
func (r *resourceTFETeam) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Provider isn't configured yet — early exit so we don't panic on the
	// nil ProviderData passed during ValidateResourceConfig and similar
	// pre-Configure RPCs.
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(ConfiguredClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected resource Configure type",
			fmt.Sprintf("Expected tfe.ConfiguredClient, got %T. This is a bug in the tfe provider, so please report it on GitHub.", req.ProviderData),
		)
		return
	}
	r.config = client
}

// Schema implements resource.Resource.
func (r *resourceTFETeam) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					// Pitfall: Computed id without UseStateForUnknown shows
					// "(known after apply)" on every plan and can cascade
					// spurious replacements onto dependents.
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"organization": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					// SDKv2 ForceNew → RequiresReplace plan modifier.
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"visibility": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					// SDKv2 ValidateFunc: validation.StringInSlice([...], false)
					stringvalidator.OneOf("secret", "organization"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"sso_team_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					// Server may echo this attribute back; preserve prior
					// state when the practitioner has not changed the value.
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_member_token_management": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				// SDKv2 Default:true → framework defaults package.
				Default: booldefault.StaticBool(true),
			},
		},
		Blocks: map[string]schema.Block{
			// organization_access was SDKv2 TypeList + MaxItems:1 + Elem:Resource.
			// Practitioners write `organization_access { ... }` block syntax (see
			// the resource docs and many public tfe modules), and existing state
			// paths are list-shaped (`organization_access.0.<field>`). Keeping
			// this as a ListNestedBlock + listvalidator.SizeAtMost(1) preserves
			// both the HCL block syntax and the existing state path — converting
			// to SingleNestedAttribute would break user HCL.
			"organization_access": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"manage_policies": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_policy_overrides": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_workspaces": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_vcs_settings": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_providers": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_modules": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_run_tasks": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_projects": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"read_workspaces": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"read_projects": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_membership": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_teams": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_organization_access": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"access_secret_teams": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"manage_agent_pools": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
					},
				},
			},
		},
	}
}

// resolveOrganization returns the `organization` attribute from the plan/state
// or, failing that, the provider-level default.
func (r *resourceTFETeam) resolveOrganization(ctx context.Context, data AttrGettable) (string, error) {
	var orgName string
	diags := r.config.dataOrDefaultOrganization(ctx, data, &orgName)
	if diags.HasError() {
		return "", fmt.Errorf("could not resolve organization: %s", diags.Errors())
	}
	return orgName, nil
}

// orgAccessOptionsFromModel converts the (at most one) nested-block element
// into the tfe.OrganizationAccessOptions API struct. Returns nil when the
// practitioner did not specify the block.
func orgAccessOptionsFromModel(blocks []modelTFETeamOrganizationAccess) *tfe.OrganizationAccessOptions {
	if len(blocks) == 0 {
		return nil
	}
	oa := blocks[0]
	return &tfe.OrganizationAccessOptions{
		ManagePolicies:           tfe.Bool(oa.ManagePolicies.ValueBool()),
		ManagePolicyOverrides:    tfe.Bool(oa.ManagePolicyOverrides.ValueBool()),
		ManageWorkspaces:         tfe.Bool(oa.ManageWorkspaces.ValueBool()),
		ManageVCSSettings:        tfe.Bool(oa.ManageVCSSettings.ValueBool()),
		ManageProviders:          tfe.Bool(oa.ManageProviders.ValueBool()),
		ManageModules:            tfe.Bool(oa.ManageModules.ValueBool()),
		ManageRunTasks:           tfe.Bool(oa.ManageRunTasks.ValueBool()),
		ManageProjects:           tfe.Bool(oa.ManageProjects.ValueBool()),
		ReadProjects:             tfe.Bool(oa.ReadProjects.ValueBool()),
		ReadWorkspaces:           tfe.Bool(oa.ReadWorkspaces.ValueBool()),
		ManageMembership:         tfe.Bool(oa.ManageMembership.ValueBool()),
		ManageTeams:              tfe.Bool(oa.ManageTeams.ValueBool()),
		ManageOrganizationAccess: tfe.Bool(oa.ManageOrganizationAccess.ValueBool()),
		AccessSecretTeams:        tfe.Bool(oa.AccessSecretTeams.ValueBool()),
		ManageAgentPools:         tfe.Bool(oa.ManageAgentPools.ValueBool()),
	}
}

// orgAccessFromAPI maps the tfe.OrganizationAccess API struct into the
// list-of-one nested-block model.
func orgAccessFromAPI(oa *tfe.OrganizationAccess) []modelTFETeamOrganizationAccess {
	if oa == nil {
		return nil
	}
	return []modelTFETeamOrganizationAccess{{
		ManagePolicies:           types.BoolValue(oa.ManagePolicies),
		ManagePolicyOverrides:    types.BoolValue(oa.ManagePolicyOverrides),
		ManageWorkspaces:         types.BoolValue(oa.ManageWorkspaces),
		ManageVCSSettings:        types.BoolValue(oa.ManageVCSSettings),
		ManageProviders:          types.BoolValue(oa.ManageProviders),
		ManageModules:            types.BoolValue(oa.ManageModules),
		ManageRunTasks:           types.BoolValue(oa.ManageRunTasks),
		ManageProjects:           types.BoolValue(oa.ManageProjects),
		ReadProjects:             types.BoolValue(oa.ReadProjects),
		ReadWorkspaces:           types.BoolValue(oa.ReadWorkspaces),
		ManageMembership:         types.BoolValue(oa.ManageMembership),
		ManageTeams:              types.BoolValue(oa.ManageTeams),
		ManageOrganizationAccess: types.BoolValue(oa.ManageOrganizationAccess),
		AccessSecretTeams:        types.BoolValue(oa.AccessSecretTeams),
		ManageAgentPools:         types.BoolValue(oa.ManageAgentPools),
	}}
}

// modelFromAPI builds a modelTFETeam from the API team value, preserving the
// caller-supplied organization (which the API does not echo back on its own).
func modelFromAPI(team *tfe.Team, orgName string) modelTFETeam {
	return modelTFETeam{
		ID:                         types.StringValue(team.ID),
		Name:                       types.StringValue(team.Name),
		Organization:               types.StringValue(orgName),
		Visibility:                 types.StringValue(team.Visibility),
		SSOTeamID:                  types.StringValue(team.SSOTeamID),
		AllowMemberTokenManagement: types.BoolValue(team.AllowMemberTokenManagement),
		OrganizationAccess:         orgAccessFromAPI(team.OrganizationAccess),
	}
}

// Create implements resource.Resource.
func (r *resourceTFETeam) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan modelTFETeam
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgName, err := r.resolveOrganization(ctx, req.Plan)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving organization", err.Error())
		return
	}

	options := tfe.TeamCreateOptions{
		Name:                       tfe.String(plan.Name.ValueString()),
		OrganizationAccess:         orgAccessOptionsFromModel(plan.OrganizationAccess),
		AllowMemberTokenManagement: tfe.Bool(plan.AllowMemberTokenManagement.ValueBool()),
	}
	if !plan.Visibility.IsNull() && !plan.Visibility.IsUnknown() && plan.Visibility.ValueString() != "" {
		options.Visibility = tfe.String(plan.Visibility.ValueString())
	}
	if !plan.SSOTeamID.IsNull() && !plan.SSOTeamID.IsUnknown() && plan.SSOTeamID.ValueString() != "" {
		options.SSOTeamID = tfe.String(plan.SSOTeamID.ValueString())
	}

	log.Printf("[DEBUG] Create team %s for organization: %s", plan.Name.ValueString(), orgName)
	team, err := r.config.Client.Teams.Create(ctx, orgName, options)
	if err != nil {
		if errors.Is(err, tfe.ErrResourceNotFound) {
			entitlements, _ := r.config.Client.Organizations.ReadEntitlements(ctx, orgName)
			if entitlements == nil {
				resp.Diagnostics.AddError(
					"Error creating team",
					fmt.Sprintf("Error creating team %s for organization %s: %s", plan.Name.ValueString(), orgName, err.Error()),
				)
				return
			}
			if !entitlements.Teams {
				resp.Diagnostics.AddError(
					"Error creating team",
					fmt.Sprintf("Error creating team %s for organization %s: missing entitlements to create teams", plan.Name.ValueString(), orgName),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Error creating team",
			fmt.Sprintf("Error creating team %s for organization %s: %s", plan.Name.ValueString(), orgName, err.Error()),
		)
		return
	}

	state := modelFromAPI(team, orgName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Read implements resource.Resource.
func (r *resourceTFETeam) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state modelTFETeam
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Read configuration of team: %s", state.ID.ValueString())
	team, err := r.config.Client.Teams.Read(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, tfe.ErrResourceNotFound) {
			log.Printf("[DEBUG] Team %s no longer exists", state.ID.ValueString())
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading team",
			fmt.Sprintf("Error reading configuration of team %s: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	new := modelFromAPI(team, state.Organization.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &new)...)
}

// Update implements resource.Resource.
func (r *resourceTFETeam) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state modelTFETeam
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	options := tfe.TeamUpdateOptions{
		Name:                       tfe.String(plan.Name.ValueString()),
		OrganizationAccess:         orgAccessOptionsFromModel(plan.OrganizationAccess),
		AllowMemberTokenManagement: tfe.Bool(plan.AllowMemberTokenManagement.ValueBool()),
	}
	if !plan.Visibility.IsNull() && !plan.Visibility.IsUnknown() && plan.Visibility.ValueString() != "" {
		options.Visibility = tfe.String(plan.Visibility.ValueString())
	}
	// Match SDKv2 behaviour: explicit empty string clears sso_team_id when not set.
	if !plan.SSOTeamID.IsNull() && !plan.SSOTeamID.IsUnknown() && plan.SSOTeamID.ValueString() != "" {
		options.SSOTeamID = tfe.String(plan.SSOTeamID.ValueString())
	} else {
		options.SSOTeamID = tfe.String("")
	}

	log.Printf("[DEBUG] Update team: %s", state.ID.ValueString())
	team, err := r.config.Client.Teams.Update(ctx, state.ID.ValueString(), options)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating team",
			fmt.Sprintf("Error updating team %s: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	new := modelFromAPI(team, state.Organization.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &new)...)
}

// Delete implements resource.Resource.
//
// Pitfall: req.Plan is null on Delete; we must read the prior id from
// req.State.
func (r *resourceTFETeam) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state modelTFETeam
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] Delete team: %s", state.ID.ValueString())
	err := r.config.Client.Teams.Delete(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, tfe.ErrResourceNotFound) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting team",
			fmt.Sprintf("Error deleting team %s: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState implements resource.ResourceWithImportState.
//
// Import formats:
//   - <ORGANIZATION NAME>/<TEAM ID>
//   - <ORGANIZATION NAME>/<TEAM NAME>
func (r *resourceTFETeam) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	s := strings.SplitN(req.ID, "/", 2)
	if len(s) != 2 {
		resp.Diagnostics.AddError(
			"Error importing team",
			fmt.Sprintf("invalid team import format: %s (expected <ORGANIZATION>/<TEAM ID> or <ORGANIZATION>/<TEAM NAME>)", req.ID),
		)
		return
	}

	orgName := s[0]
	teamNameOrID := s[1]

	// First, see whether s[1] is a team ID we can read directly.
	if isResourceIDFormat("team", teamNameOrID) {
		if team, err := r.config.Client.Teams.Read(ctx, teamNameOrID); err == nil {
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), team.ID)...)
			resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization"), orgName)...)
			return
		}
	}

	// Fall back to looking it up by name.
	team, err := fetchTeamByName(ctx, r.config.Client, orgName, teamNameOrID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error importing team",
			fmt.Sprintf("no team found with name or ID %s in organization %s: %s", teamNameOrID, orgName, err.Error()),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), team.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("organization"), orgName)...)
}
