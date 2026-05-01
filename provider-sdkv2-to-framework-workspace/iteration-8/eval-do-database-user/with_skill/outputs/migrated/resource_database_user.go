package database

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/internal/mutexkv"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
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

var mutexKV = mutexkv.NewMutexKV()

var (
	_ resource.Resource                = &databaseUserResource{}
	_ resource.ResourceWithConfigure   = &databaseUserResource{}
	_ resource.ResourceWithImportState = &databaseUserResource{}
	_ resource.ResourceWithModifyPlan  = &databaseUserResource{}
)

// NewDatabaseUserResource returns the framework resource replacing ResourceDigitalOceanDatabaseUser.
func NewDatabaseUserResource() resource.Resource {
	return &databaseUserResource{}
}

type databaseUserResource struct {
	client *godo.Client
}

// databaseUserModel is the Terraform state model for digitalocean_database_user.
type databaseUserModel struct {
	ID              types.String `tfsdk:"id"`
	ClusterID       types.String `tfsdk:"cluster_id"`
	Name            types.String `tfsdk:"name"`
	MySQLAuthPlugin types.String `tfsdk:"mysql_auth_plugin"`
	Settings        types.List   `tfsdk:"settings"`
	Role            types.String `tfsdk:"role"`
	// password: Sensitive + Computed — NOT WriteOnly.
	// The DigitalOcean API returns the password in the CreateUser response.
	// Terraform must persist it to state so downstream resources can reference
	// it (e.g. digitalocean_database_user.x.password). WriteOnly is not
	// appropriate here because WriteOnly attributes are never stored in state,
	// which would break any cross-resource reference and any ImportStateVerify
	// assertion. Sensitive: true is the correct choice — the value is redacted
	// from plan output and logs while still being stored and readable from state.
	Password   types.String `tfsdk:"password"`
	AccessCert types.String `tfsdk:"access_cert"`
	AccessKey  types.String `tfsdk:"access_key"`
}

// userSettingsModel is the model for each element of the settings block.
type userSettingsModel struct {
	ACL           types.List `tfsdk:"acl"`
	OpenSearchACL types.List `tfsdk:"opensearch_acl"`
}

// userACLModel is the model for each acl block inside settings.
type userACLModel struct {
	ID         types.String `tfsdk:"id"`
	Topic      types.String `tfsdk:"topic"`
	Permission types.String `tfsdk:"permission"`
}

// userOpenSearchACLModel is the model for each opensearch_acl block inside settings.
type userOpenSearchACLModel struct {
	Index      types.String `tfsdk:"index"`
	Permission types.String `tfsdk:"permission"`
}

func (r *databaseUserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_user"
}

func (r *databaseUserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cluster_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			// mysql_auth_plugin is Optional + Computed so that the API default
			// (caching_sha2_password) is preserved when the practitioner omits it.
			// UseStateForUnknown carries the prior value forward across plans,
			// replicating the SDKv2 DiffSuppressFunc that suppressed diffs when
			// old == caching_sha2_password and new == "".
			"mysql_auth_plugin": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(
						godo.SQLAuthPluginNative,
						godo.SQLAuthPluginCachingSHA2,
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// password is Sensitive AND Computed. See the struct-field comment above
			// for the full rationale for why WriteOnly must NOT be used here.
			"password": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"access_cert": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"access_key": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			// settings is a TypeList (no MaxItems: 1) whose block syntax
			// (`settings { acl { ... } }`) is used in practitioner configs. Keep
			// as ListNestedBlock to avoid an HCL breaking change. A SizeAtMost(1)
			// validator enforces the original cardinality constraint.
			"settings": schema.ListNestedBlock{
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"acl": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"id": schema.StringAttribute{
										Computed: true,
										PlanModifiers: []planmodifier.String{
											stringplanmodifier.UseStateForUnknown(),
										},
									},
									"topic": schema.StringAttribute{
										Required: true,
										Validators: []validator.String{
											stringvalidator.LengthAtLeast(1),
										},
									},
									"permission": schema.StringAttribute{
										Required: true,
										Validators: []validator.String{
											stringvalidator.OneOf("admin", "consume", "produce", "produceconsume"),
										},
									},
								},
							},
						},
						"opensearch_acl": schema.ListNestedBlock{
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"index": schema.StringAttribute{
										Required: true,
										Validators: []validator.String{
											stringvalidator.LengthAtLeast(1),
										},
									},
									"permission": schema.StringAttribute{
										Required: true,
										Validators: []validator.String{
											stringvalidator.OneOf("deny", "admin", "read", "write", "readwrite"),
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *databaseUserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan handles the mysql_auth_plugin diff-suppression logic.
// When state holds caching_sha2_password and the plan has null/unknown
// (practitioner omitted the attribute), we carry state forward to suppress
// the spurious diff. This replicates the SDKv2 DiffSuppressFunc:
//
//	func(k, old, new string, d *schema.ResourceData) bool {
//	    return old == godo.SQLAuthPluginCachingSHA2 && new == ""
//	}
func (r *databaseUserResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Only applies to update (both plan and state populated).
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}

	var plan, state databaseUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.MySQLAuthPlugin.ValueString() == godo.SQLAuthPluginCachingSHA2 &&
		(plan.MySQLAuthPlugin.IsNull() || plan.MySQLAuthPlugin.IsUnknown() || plan.MySQLAuthPlugin.ValueString() == "") {
		resp.Diagnostics.Append(
			resp.Plan.SetAttribute(ctx, path.Root("mysql_auth_plugin"), state.MySQLAuthPlugin)...,
		)
	}
}

func (r *databaseUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()

	opts := &godo.DatabaseCreateUserRequest{
		Name: plan.Name.ValueString(),
	}

	if !plan.MySQLAuthPlugin.IsNull() && !plan.MySQLAuthPlugin.IsUnknown() && plan.MySQLAuthPlugin.ValueString() != "" {
		opts.MySQLSettings = &godo.DatabaseMySQLUserSettings{
			AuthPlugin: plan.MySQLAuthPlugin.ValueString(),
		}
	}

	if !plan.Settings.IsNull() && !plan.Settings.IsUnknown() && len(plan.Settings.Elements()) > 0 {
		settings, d := expandUserSettingsFromPlan(ctx, plan.Settings)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		opts.Settings = settings
	}

	// Prevent parallel creation of users for same cluster.
	key := fmt.Sprintf("digitalocean_database_cluster/%s/users", clusterID)
	mutexKV.Lock(key)
	defer mutexKV.Unlock(key)

	log.Printf("[DEBUG] Database User create configuration: %#v", opts)
	user, _, err := r.client.Databases.CreateUser(ctx, clusterID, opts)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Database User", err.Error())
		return
	}

	plan.ID = types.StringValue(makeDatabaseUserID(clusterID, user.Name))
	log.Printf("[INFO] Database User Name: %s", user.Name)

	// Set settings only from the CreateUser response. GetUser does not include
	// settings in its response, so we must persist them from the create call.
	settingsVal, d := flattenUserSettingsToState(ctx, user.Settings)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Settings = settingsVal

	setModelFromUser(&plan, user)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	name := state.Name.ValueString()

	user, httpResp, err := r.client.Databases.GetUser(ctx, clusterID, name)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error retrieving Database User", err.Error())
		return
	}

	setModelFromUser(&state, user)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *databaseUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state databaseUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()
	name := plan.Name.ValueString()

	if !plan.MySQLAuthPlugin.Equal(state.MySQLAuthPlugin) {
		authReq := &godo.DatabaseResetUserAuthRequest{}
		plugin := plan.MySQLAuthPlugin.ValueString()
		if plugin == "" {
			// If blank, restore default value.
			plugin = godo.SQLAuthPluginCachingSHA2
		}
		authReq.MySQLSettings = &godo.DatabaseMySQLUserSettings{
			AuthPlugin: plugin,
		}
		_, _, err := r.client.Databases.ResetUserAuth(ctx, clusterID, name, authReq)
		if err != nil {
			resp.Diagnostics.AddError("Error updating mysql_auth_plugin for DatabaseUser", err.Error())
			return
		}
	}

	if !plan.Settings.Equal(state.Settings) {
		updateReq := &godo.DatabaseUpdateUserRequest{}
		if !plan.Settings.IsNull() && !plan.Settings.IsUnknown() && len(plan.Settings.Elements()) > 0 {
			settings, d := expandUserSettingsFromPlan(ctx, plan.Settings)
			resp.Diagnostics.Append(d...)
			if resp.Diagnostics.HasError() {
				return
			}
			updateReq.Settings = settings
		}
		_, _, err := r.client.Databases.UpdateUser(ctx, clusterID, name, updateReq)
		if err != nil {
			resp.Diagnostics.AddError("Error updating settings for DatabaseUser", err.Error())
			return
		}
	}

	// Re-read to get the latest state from the API.
	user, httpResp, err := r.client.Databases.GetUser(ctx, clusterID, name)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error retrieving Database User after update", err.Error())
		return
	}

	setModelFromUser(&plan, user)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *databaseUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	name := state.Name.ValueString()

	// Prevent parallel deletion of users for same cluster.
	key := fmt.Sprintf("digitalocean_database_cluster/%s/users", clusterID)
	mutexKV.Lock(key)
	defer mutexKV.Unlock(key)

	log.Printf("[INFO] Deleting Database User: %s", state.ID.ValueString())
	_, err := r.client.Databases.DeleteUser(ctx, clusterID, name)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting Database User", err.Error())
	}
}

func (r *databaseUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The original SDKv2 importer required a comma-separated "clusterID,name" format.
	if !strings.Contains(req.ID, ",") {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Must use the ID of the source database cluster and the name of the user joined with a comma (e.g. `id,name`)",
		)
		return
	}
	parts := strings.SplitN(req.ID, ",", 2)
	clusterID := parts[0]
	name := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), makeDatabaseUserID(clusterID, name))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// setModelFromUser populates state model fields from a godo.DatabaseUser.
func setModelFromUser(m *databaseUserModel, user *godo.DatabaseUser) {
	if user.Role == "" {
		m.Role = types.StringValue("normal")
	} else {
		m.Role = types.StringValue(user.Role)
	}

	// The password is blank when GETing MongoDB clusters post-create.
	// Don't overwrite the password that was set on create (preserve state value).
	if user.Password != "" {
		m.Password = types.StringValue(user.Password)
	}

	if user.MySQLSettings != nil {
		m.MySQLAuthPlugin = types.StringValue(user.MySQLSettings.AuthPlugin)
	}

	// access_cert and access_key are only set for Kafka users.
	if user.AccessCert != "" {
		m.AccessCert = types.StringValue(user.AccessCert)
	}
	if user.AccessKey != "" {
		m.AccessKey = types.StringValue(user.AccessKey)
	}
}

func makeDatabaseUserID(clusterID string, name string) string {
	return fmt.Sprintf("%s/user/%s", clusterID, name)
}

// expandUserSettingsFromPlan converts the plan's settings list to godo.DatabaseUserSettings.
func expandUserSettingsFromPlan(ctx context.Context, settingsList types.List) (*godo.DatabaseUserSettings, diag.Diagnostics) {
	var diags diag.Diagnostics

	var settings []userSettingsModel
	diags.Append(settingsList.ElementsAs(ctx, &settings, false)...)
	if diags.HasError() || len(settings) == 0 {
		return &godo.DatabaseUserSettings{}, diags
	}

	s := settings[0]

	acls, d := expandACLsFromList(ctx, s.ACL)
	diags.Append(d...)

	opensearchACLs, d := expandOpenSearchACLsFromList(ctx, s.OpenSearchACL)
	diags.Append(d...)

	return &godo.DatabaseUserSettings{
		ACL:           acls,
		OpenSearchACL: opensearchACLs,
	}, diags
}

func expandACLsFromList(ctx context.Context, aclList types.List) ([]*godo.KafkaACL, diag.Diagnostics) {
	var diags diag.Diagnostics
	if aclList.IsNull() || aclList.IsUnknown() {
		return nil, diags
	}

	var acls []userACLModel
	diags.Append(aclList.ElementsAs(ctx, &acls, false)...)
	if diags.HasError() {
		return nil, diags
	}

	result := make([]*godo.KafkaACL, len(acls))
	for i, a := range acls {
		result[i] = &godo.KafkaACL{
			Topic:      a.Topic.ValueString(),
			Permission: a.Permission.ValueString(),
		}
	}
	return result, diags
}

func expandOpenSearchACLsFromList(ctx context.Context, aclList types.List) ([]*godo.OpenSearchACL, diag.Diagnostics) {
	var diags diag.Diagnostics
	if aclList.IsNull() || aclList.IsUnknown() {
		return nil, diags
	}

	var acls []userOpenSearchACLModel
	diags.Append(aclList.ElementsAs(ctx, &acls, false)...)
	if diags.HasError() {
		return nil, diags
	}

	result := make([]*godo.OpenSearchACL, len(acls))
	for i, a := range acls {
		result[i] = &godo.OpenSearchACL{
			Index:      a.Index.ValueString(),
			Permission: a.Permission.ValueString(),
		}
	}
	return result, diags
}

// flattenUserSettingsToState converts godo.DatabaseUserSettings to a types.List for state.
func flattenUserSettingsToState(ctx context.Context, settings *godo.DatabaseUserSettings) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	settingsObjType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"acl":           types.ListType{ElemType: aclObjectType()},
			"opensearch_acl": types.ListType{ElemType: opensearchACLObjectType()},
		},
	}

	if settings == nil {
		return types.ListValueMust(settingsObjType, []attr.Value{}), diags
	}

	aclList, d := flattenACLsToList(ctx, settings.ACL)
	diags.Append(d...)

	opensearchList, d := flattenOpenSearchACLsToList(ctx, settings.OpenSearchACL)
	diags.Append(d...)

	if diags.HasError() {
		return types.ListNull(settingsObjType), diags
	}

	settingsObj, d := types.ObjectValue(
		map[string]attr.Type{
			"acl":           types.ListType{ElemType: aclObjectType()},
			"opensearch_acl": types.ListType{ElemType: opensearchACLObjectType()},
		},
		map[string]attr.Value{
			"acl":           aclList,
			"opensearch_acl": opensearchList,
		},
	)
	diags.Append(d...)
	if diags.HasError() {
		return types.ListNull(settingsObjType), diags
	}

	listVal, d := types.ListValue(settingsObjType, []attr.Value{settingsObj})
	diags.Append(d...)
	return listVal, diags
}

func aclObjectType() attr.Type {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":         types.StringType,
			"topic":      types.StringType,
			"permission": types.StringType,
		},
	}
}

func opensearchACLObjectType() attr.Type {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"index":      types.StringType,
			"permission": types.StringType,
		},
	}
}

func flattenACLsToList(ctx context.Context, acls []*godo.KafkaACL) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := aclObjectType()

	if len(acls) == 0 {
		return types.ListValueMust(elemType, []attr.Value{}), diags
	}

	elems := make([]attr.Value, len(acls))
	for i, acl := range acls {
		obj, d := types.ObjectValue(
			map[string]attr.Type{
				"id":         types.StringType,
				"topic":      types.StringType,
				"permission": types.StringType,
			},
			map[string]attr.Value{
				"id":         types.StringValue(acl.ID),
				"topic":      types.StringValue(acl.Topic),
				"permission": types.StringValue(normalizePermission(acl.Permission)),
			},
		)
		diags.Append(d...)
		elems[i] = obj
	}

	listVal, d := types.ListValue(elemType, elems)
	diags.Append(d...)
	return listVal, diags
}

func flattenOpenSearchACLsToList(ctx context.Context, acls []*godo.OpenSearchACL) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elemType := opensearchACLObjectType()

	if len(acls) == 0 {
		return types.ListValueMust(elemType, []attr.Value{}), diags
	}

	elems := make([]attr.Value, len(acls))
	for i, acl := range acls {
		obj, d := types.ObjectValue(
			map[string]attr.Type{
				"index":      types.StringType,
				"permission": types.StringType,
			},
			map[string]attr.Value{
				"index":      types.StringValue(acl.Index),
				"permission": types.StringValue(normalizeOpenSearchPermission(acl.Permission)),
			},
		)
		diags.Append(d...)
		elems[i] = obj
	}

	listVal, d := types.ListValue(elemType, elems)
	diags.Append(d...)
	return listVal, diags
}

func normalizePermission(p string) string {
	pLower := strings.ToLower(p)
	switch pLower {
	case "admin", "produce", "consume":
		return pLower
	case "produceconsume", "produce_consume", "readwrite", "read_write":
		return "produceconsume"
	}
	return ""
}

func normalizeOpenSearchPermission(p string) string {
	pLower := strings.ToLower(p)
	switch pLower {
	case "admin", "deny", "read", "write":
		return pLower
	case "readwrite", "read_write":
		return "readwrite"
	}
	return ""
}
