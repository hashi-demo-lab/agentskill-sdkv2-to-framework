package database

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/digitalocean/godo"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"
	"github.com/digitalocean/terraform-provider-digitalocean/internal/mutexkv"
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

// Compile-time interface assertions.
var (
	_ resource.Resource                = &databaseUserResource{}
	_ resource.ResourceWithConfigure   = &databaseUserResource{}
	_ resource.ResourceWithImportState = &databaseUserResource{}
)

// NewDatabaseUserResource returns the framework resource implementation for
// digitalocean_database_user.
func NewDatabaseUserResource() resource.Resource {
	return &databaseUserResource{}
}

type databaseUserResource struct {
	client *godo.Client
}

// databaseUserModel is the Go model corresponding to the resource schema.
// Field names map to schema attribute names via the `tfsdk` struct tag.
type databaseUserModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	ClusterID       types.String `tfsdk:"cluster_id"`
	MySQLAuthPlugin types.String `tfsdk:"mysql_auth_plugin"`
	Role            types.String `tfsdk:"role"`
	// Password is Sensitive + Computed (NOT WriteOnly): the API returns it on
	// resource creation and downstream resources / outputs may reference it.
	// See notes.md for the Sensitive-vs-WriteOnly decision rationale.
	Password   types.String `tfsdk:"password"`
	AccessCert types.String `tfsdk:"access_cert"`
	AccessKey  types.String `tfsdk:"access_key"`
	Settings   types.List   `tfsdk:"settings"`
}

// Nested models for blocks. They are populated via Plan.Get / used to
// (de)serialise the typed `types.List` representations of the blocks.
type userSettingsModel struct {
	ACL           types.List `tfsdk:"acl"`
	OpenSearchACL types.List `tfsdk:"opensearch_acl"`
}

type userACLModel struct {
	ID         types.String `tfsdk:"id"`
	Topic      types.String `tfsdk:"topic"`
	Permission types.String `tfsdk:"permission"`
}

type userOpenSearchACLModel struct {
	Index      types.String `tfsdk:"index"`
	Permission types.String `tfsdk:"permission"`
}

// Object types for the nested types.List instances above. Used when
// constructing the typed values in Read/Create.
func userACLObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":         types.StringType,
			"topic":      types.StringType,
			"permission": types.StringType,
		},
	}
}

func userOpenSearchACLObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"index":      types.StringType,
			"permission": types.StringType,
		},
	}
}

func userSettingsObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"acl":            types.ListType{ElemType: userACLObjectType()},
			"opensearch_acl": types.ListType{ElemType: userOpenSearchACLObjectType()},
		},
	}
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
			"name": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
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
			"mysql_auth_plugin": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf(
						godo.SQLAuthPluginNative,
						godo.SQLAuthPluginCachingSHA2,
					),
				},
				// Replaces the SDKv2 DiffSuppressFunc that returned "no diff"
				// when the API reported caching_sha2_password (the default)
				// and the config was unset. Plan modifier preserves the
				// state value when the practitioner has not set the field.
				PlanModifiers: []planmodifier.String{
					mysqlAuthPluginDefaultModifier{},
				},
			},
			"role": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// Sensitive (NOT WriteOnly): API-returned, stored in state for
			// downstream references. Decision rule: "does Terraform need to
			// read this value back later? Yes -> Sensitive only." Also,
			// WriteOnly + Computed cannot coexist (the framework rejects it
			// at provider boot).
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
			// Practitioners write `settings { acl { ... } }` block syntax in
			// HCL, so the SDKv2 TypeList of *schema.Resource is preserved as
			// a ListNestedBlock to keep the user-facing HCL identical.
			"settings": schema.ListNestedBlock{
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
											stringvalidator.OneOf(
												"admin",
												"consume",
												"produce",
												"produceconsume",
											),
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
											stringvalidator.OneOf(
												"deny",
												"admin",
												"read",
												"write",
												"readwrite",
											),
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

// mysqlAuthPluginDefaultModifier replaces the SDKv2 DiffSuppressFunc:
//
//	old == godo.SQLAuthPluginCachingSHA2 && new == ""
//
// When the practitioner clears `mysql_auth_plugin` from config but the API
// reports the default `caching_sha2_password` value, keep the state value to
// avoid a perpetual diff.
type mysqlAuthPluginDefaultModifier struct{}

func (m mysqlAuthPluginDefaultModifier) Description(ctx context.Context) string {
	return "Suppresses diffs when config is empty and prior state is the API default."
}

func (m mysqlAuthPluginDefaultModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m mysqlAuthPluginDefaultModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() {
		return
	}
	if !req.ConfigValue.IsNull() {
		return
	}
	if req.StateValue.ValueString() == godo.SQLAuthPluginCachingSHA2 {
		resp.PlanValue = req.StateValue
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
			fmt.Sprintf("Expected *config.CombinedConfig, got %T.", req.ProviderData),
		)
		return
	}
	r.client = combined.GodoClient()
}

func (r *databaseUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := plan.ClusterID.ValueString()
	name := plan.Name.ValueString()

	opts := &godo.DatabaseCreateUserRequest{Name: name}

	if !plan.MySQLAuthPlugin.IsNull() && !plan.MySQLAuthPlugin.IsUnknown() && plan.MySQLAuthPlugin.ValueString() != "" {
		opts.MySQLSettings = &godo.DatabaseMySQLUserSettings{
			AuthPlugin: plan.MySQLAuthPlugin.ValueString(),
		}
	}

	settings, diags := expandSettingsList(ctx, plan.Settings)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	opts.Settings = settings

	// Prevent parallel creation of users for the same cluster.
	key := fmt.Sprintf("digitalocean_database_cluster/%s/users", clusterID)
	mutexKV.Lock(key)
	defer mutexKV.Unlock(key)

	log.Printf("[DEBUG] Database User create configuration: %#v", opts)
	user, _, err := r.client.Databases.CreateUser(ctx, clusterID, opts)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Database User", err.Error())
		return
	}
	log.Printf("[INFO] Database User Name: %s", user.Name)

	plan.ID = types.StringValue(makeDatabaseUserID(clusterID, user.Name))

	// CreateUser responses include `settings`; GetUser responses do not, so
	// capture them here.
	settingsValue, fdiags := flattenUserSettings(ctx, user.Settings)
	resp.Diagnostics.Append(fdiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Settings = settingsValue

	applyDatabaseUserAttributes(&plan, user)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
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

	applyDatabaseUserAttributes(&state, user)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
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
		if !plan.MySQLAuthPlugin.IsNull() && !plan.MySQLAuthPlugin.IsUnknown() && plan.MySQLAuthPlugin.ValueString() != "" {
			authReq.MySQLSettings = &godo.DatabaseMySQLUserSettings{
				AuthPlugin: plan.MySQLAuthPlugin.ValueString(),
			}
		} else {
			// If blank, restore the default value.
			authReq.MySQLSettings = &godo.DatabaseMySQLUserSettings{
				AuthPlugin: godo.SQLAuthPluginCachingSHA2,
			}
		}

		_, _, err := r.client.Databases.ResetUserAuth(ctx, clusterID, name, authReq)
		if err != nil {
			resp.Diagnostics.AddError("Error updating mysql_auth_plugin for DatabaseUser", err.Error())
			return
		}
	}

	if !plan.Settings.Equal(state.Settings) {
		updateReq := &godo.DatabaseUpdateUserRequest{}
		settings, diags := expandSettingsList(ctx, plan.Settings)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateReq.Settings = settings

		_, _, err := r.client.Databases.UpdateUser(ctx, clusterID, name, updateReq)
		if err != nil {
			resp.Diagnostics.AddError("Error updating settings for DatabaseUser", err.Error())
			return
		}
	}

	// Read latest from API to refresh computed fields.
	user, _, err := r.client.Databases.GetUser(ctx, clusterID, name)
	if err != nil {
		resp.Diagnostics.AddError("Error retrieving Database User after update", err.Error())
		return
	}

	// Carry forward the prior password from state if the API doesn't return
	// it on read (matches the SDKv2 behaviour: "Don't overwrite the password
	// set on create").
	merged := plan
	merged.Password = state.Password
	applyDatabaseUserAttributes(&merged, user)

	resp.Diagnostics.Append(resp.State.Set(ctx, &merged)...)
}

func (r *databaseUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	clusterID := state.ClusterID.ValueString()
	name := state.Name.ValueString()

	// Prevent parallel deletion of users for the same cluster.
	key := fmt.Sprintf("digitalocean_database_cluster/%s/users", clusterID)
	mutexKV.Lock(key)
	defer mutexKV.Unlock(key)

	log.Printf("[INFO] Deleting Database User: %s", state.ID.ValueString())
	_, err := r.client.Databases.DeleteUser(ctx, clusterID, name)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting Database User", err.Error())
		return
	}
}

func (r *databaseUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !strings.Contains(req.ID, ",") {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"must use the ID of the source database cluster and the name of the user joined with a comma (e.g. `id,name`)",
		)
		return
	}
	parts := strings.SplitN(req.ID, ",", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"must use the ID of the source database cluster and the name of the user joined with a comma (e.g. `id,name`)",
		)
		return
	}

	clusterID := parts[0]
	name := parts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), makeDatabaseUserID(clusterID, name))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// applyDatabaseUserAttributes copies API-returned values into the typed
// model. Mirrors the SDKv2 helper of the same intent.
func applyDatabaseUserAttributes(m *databaseUserModel, user *godo.DatabaseUser) {
	if user.Role == "" {
		m.Role = types.StringValue("normal")
	} else {
		m.Role = types.StringValue(user.Role)
	}

	// Mirror SDKv2 logic: API may return blank password on subsequent reads
	// (e.g. MongoDB after create); don't overwrite the value already in state.
	if user.Password != "" {
		m.Password = types.StringValue(user.Password)
	} else if m.Password.IsUnknown() || m.Password.IsNull() {
		// On first Create the model arrives with unknown; if the API also
		// returned blank, surface an empty string rather than leaving
		// unknown in state (which would fail data-consistency checks).
		m.Password = types.StringValue("")
	}

	if user.MySQLSettings != nil {
		m.MySQLAuthPlugin = types.StringValue(user.MySQLSettings.AuthPlugin)
	} else if m.MySQLAuthPlugin.IsUnknown() {
		m.MySQLAuthPlugin = types.StringNull()
	}

	if user.AccessCert != "" {
		m.AccessCert = types.StringValue(user.AccessCert)
	} else if m.AccessCert.IsUnknown() {
		m.AccessCert = types.StringValue("")
	}
	if user.AccessKey != "" {
		m.AccessKey = types.StringValue(user.AccessKey)
	} else if m.AccessKey.IsUnknown() {
		m.AccessKey = types.StringValue("")
	}
}

// makeDatabaseUserID composes the resource ID. Unchanged from SDKv2.
func makeDatabaseUserID(clusterID string, name string) string {
	return fmt.Sprintf("%s/user/%s", clusterID, name)
}

// expandSettingsList converts the typed settings list into a godo request
// body. Returns nil settings when the list is null/unknown/empty.
func expandSettingsList(ctx context.Context, list types.List) (*godo.DatabaseUserSettings, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}
	var settingsBlocks []userSettingsModel
	d := list.ElementsAs(ctx, &settingsBlocks, false)
	diags.Append(d...)
	if d.HasError() {
		return nil, diags
	}
	if len(settingsBlocks) == 0 {
		return &godo.DatabaseUserSettings{}, diags
	}
	first := settingsBlocks[0]

	out := &godo.DatabaseUserSettings{}

	if !first.ACL.IsNull() && !first.ACL.IsUnknown() {
		var acls []userACLModel
		d := first.ACL.ElementsAs(ctx, &acls, false)
		diags.Append(d...)
		if d.HasError() {
			return nil, diags
		}
		for _, a := range acls {
			out.ACL = append(out.ACL, &godo.KafkaACL{
				Topic:      a.Topic.ValueString(),
				Permission: a.Permission.ValueString(),
			})
		}
	}

	if !first.OpenSearchACL.IsNull() && !first.OpenSearchACL.IsUnknown() {
		var acls []userOpenSearchACLModel
		d := first.OpenSearchACL.ElementsAs(ctx, &acls, false)
		diags.Append(d...)
		if d.HasError() {
			return nil, diags
		}
		for _, a := range acls {
			out.OpenSearchACL = append(out.OpenSearchACL, &godo.OpenSearchACL{
				Index:      a.Index.ValueString(),
				Permission: a.Permission.ValueString(),
			})
		}
	}

	return out, diags
}

// flattenUserSettings converts a godo settings struct into the typed
// settings list used in state.
func flattenUserSettings(ctx context.Context, settings *godo.DatabaseUserSettings) (types.List, diag.Diagnostics) {
	settingsListType := types.ListType{ElemType: userSettingsObjectType()}

	if settings == nil {
		return types.ListNull(userSettingsObjectType()), nil
	}

	var diags diag.Diagnostics

	aclElems := make([]attr.Value, 0, len(settings.ACL))
	for _, a := range settings.ACL {
		obj, d := types.ObjectValue(
			userACLObjectType().AttrTypes,
			map[string]attr.Value{
				"id":         types.StringValue(a.ID),
				"topic":      types.StringValue(a.Topic),
				"permission": types.StringValue(normalizePermission(a.Permission)),
			},
		)
		diags.Append(d...)
		if d.HasError() {
			return types.ListNull(userSettingsObjectType()), diags
		}
		aclElems = append(aclElems, obj)
	}
	aclList, d := types.ListValue(userACLObjectType(), aclElems)
	diags.Append(d...)
	if d.HasError() {
		return types.ListNull(userSettingsObjectType()), diags
	}

	openSearchElems := make([]attr.Value, 0, len(settings.OpenSearchACL))
	for _, a := range settings.OpenSearchACL {
		obj, d := types.ObjectValue(
			userOpenSearchACLObjectType().AttrTypes,
			map[string]attr.Value{
				"index":      types.StringValue(a.Index),
				"permission": types.StringValue(normalizeOpenSearchPermission(a.Permission)),
			},
		)
		diags.Append(d...)
		if d.HasError() {
			return types.ListNull(userSettingsObjectType()), diags
		}
		openSearchElems = append(openSearchElems, obj)
	}
	osList, d := types.ListValue(userOpenSearchACLObjectType(), openSearchElems)
	diags.Append(d...)
	if d.HasError() {
		return types.ListNull(userSettingsObjectType()), diags
	}

	settingsObj, d := types.ObjectValue(
		userSettingsObjectType().AttrTypes,
		map[string]attr.Value{
			"acl":            aclList,
			"opensearch_acl": osList,
		},
	)
	diags.Append(d...)
	if d.HasError() {
		return types.ListNull(userSettingsObjectType()), diags
	}

	settingsList, d := types.ListValue(settingsListType.ElemType, []attr.Value{settingsObj})
	diags.Append(d...)
	return settingsList, diags
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
