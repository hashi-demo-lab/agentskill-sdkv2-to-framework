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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var mutexKV = mutexkv.NewMutexKV()

var (
	_ resource.Resource                = &databaseUserResource{}
	_ resource.ResourceWithConfigure   = &databaseUserResource{}
	_ resource.ResourceWithImportState = &databaseUserResource{}
)

func NewDatabaseUserResource() resource.Resource {
	return &databaseUserResource{}
}

type databaseUserResource struct {
	client *godo.Client
}

// databaseUserModel maps to the Terraform state for digitalocean_database_user.
type databaseUserModel struct {
	ID              types.String           `tfsdk:"id"`
	ClusterID       types.String           `tfsdk:"cluster_id"`
	Name            types.String           `tfsdk:"name"`
	MySQLAuthPlugin types.String           `tfsdk:"mysql_auth_plugin"`
	Settings        []databaseUserSettings `tfsdk:"settings"`
	Role            types.String           `tfsdk:"role"`
	Password        types.String           `tfsdk:"password"`
	AccessCert      types.String           `tfsdk:"access_cert"`
	AccessKey       types.String           `tfsdk:"access_key"`
}

type databaseUserSettings struct {
	ACL           []databaseUserACL         `tfsdk:"acl"`
	OpenSearchACL []databaseUserOpenSearchACL `tfsdk:"opensearch_acl"`
}

type databaseUserACL struct {
	ID         types.String `tfsdk:"id"`
	Topic      types.String `tfsdk:"topic"`
	Permission types.String `tfsdk:"permission"`
}

type databaseUserOpenSearchACL struct {
	Index      types.String `tfsdk:"index"`
	Permission types.String `tfsdk:"permission"`
}

func (r *databaseUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_user"
}

func (r *databaseUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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
			"mysql_auth_plugin": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString(godo.SQLAuthPluginCachingSHA2),
				Validators: []validator.String{
					stringvalidator.OneOf(
						godo.SQLAuthPluginNative,
						godo.SQLAuthPluginCachingSHA2,
					),
				},
			},
			"role": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// password is Sensitive and Computed: the API returns it on create and
			// Terraform stores it in state (redacted from plan output). Write-only
			// must NOT be applied here because password is read back via
			// GetUser on subsequent plans — it must round-trip through state.
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
			// `settings` is a true repeating block (no MaxItems: 1) used with
			// block syntax in existing practitioner configs — kept as
			// ListNestedBlock for backward-compat.
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

func (r *databaseUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*config.CombinedConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *config.CombinedConfig, got: %T", req.ProviderData),
		)
		return
	}
	r.client = cfg.GodoClient()
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

	if !plan.MySQLAuthPlugin.IsNull() && !plan.MySQLAuthPlugin.IsUnknown() {
		plugin := plan.MySQLAuthPlugin.ValueString()
		// Only set MySQL settings when explicitly configured (not just the default).
		if plugin != "" {
			opts.MySQLSettings = &godo.DatabaseMySQLUserSettings{
				AuthPlugin: plugin,
			}
		}
	}

	if len(plan.Settings) > 0 {
		opts.Settings = expandUserSettingsFramework(plan.Settings)
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

	// Role
	if user.Role == "" {
		plan.Role = types.StringValue("normal")
	} else {
		plan.Role = types.StringValue(user.Role)
	}

	// Password — API returns it on create; store in state.
	if user.Password != "" {
		plan.Password = types.StringValue(user.Password)
	}

	if user.MySQLSettings != nil {
		plan.MySQLAuthPlugin = types.StringValue(user.MySQLSettings.AuthPlugin)
	}

	if user.AccessCert != "" {
		plan.AccessCert = types.StringValue(user.AccessCert)
	}
	if user.AccessKey != "" {
		plan.AccessKey = types.StringValue(user.AccessKey)
	}

	// set userSettings only on CreateUser, due to CreateUser responses including
	// `settings` but GetUser responses not including `settings`
	if user.Settings != nil {
		plan.Settings = flattenUserSettingsFramework(user.Settings)
	}

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

	user, apiResp, err := r.client.Databases.GetUser(ctx, clusterID, name)
	if err != nil {
		if apiResp != nil && apiResp.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error retrieving Database User", err.Error())
		return
	}

	if user.Role == "" {
		state.Role = types.StringValue("normal")
	} else {
		state.Role = types.StringValue(user.Role)
	}

	// GetUser does not return password for MongoDB — don't overwrite the value
	// that was stored in state on create.
	if user.Password != "" {
		state.Password = types.StringValue(user.Password)
	}

	if user.MySQLSettings != nil {
		state.MySQLAuthPlugin = types.StringValue(user.MySQLSettings.AuthPlugin)
	}

	if user.AccessCert != "" {
		state.AccessCert = types.StringValue(user.AccessCert)
	}
	if user.AccessKey != "" {
		state.AccessKey = types.StringValue(user.AccessKey)
	}

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
		if plugin != "" {
			authReq.MySQLSettings = &godo.DatabaseMySQLUserSettings{
				AuthPlugin: plugin,
			}
		} else {
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

	if !settingsEqual(plan.Settings, state.Settings) {
		updateReq := &godo.DatabaseUpdateUserRequest{}
		if len(plan.Settings) > 0 {
			updateReq.Settings = expandUserSettingsFramework(plan.Settings)
		}
		_, _, err := r.client.Databases.UpdateUser(ctx, clusterID, name, updateReq)
		if err != nil {
			resp.Diagnostics.AddError("Error updating settings for DatabaseUser", err.Error())
			return
		}
	}

	// Re-read to get the latest state from the API.
	var readState databaseUserModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &readState)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Carry plan values for mutable fields and refresh computed fields.
	readState.MySQLAuthPlugin = plan.MySQLAuthPlugin
	readState.Settings = plan.Settings

	user, apiResp, err := r.client.Databases.GetUser(ctx, clusterID, name)
	if err != nil {
		if apiResp != nil && apiResp.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Database User after update", err.Error())
		return
	}

	if user.Role == "" {
		readState.Role = types.StringValue("normal")
	} else {
		readState.Role = types.StringValue(user.Role)
	}
	if user.Password != "" {
		readState.Password = types.StringValue(user.Password)
	}
	if user.MySQLSettings != nil {
		readState.MySQLAuthPlugin = types.StringValue(user.MySQLSettings.AuthPlugin)
	}
	if user.AccessCert != "" {
		readState.AccessCert = types.StringValue(user.AccessCert)
	}
	if user.AccessKey != "" {
		readState.AccessKey = types.StringValue(user.AccessKey)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, readState)...)
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
	if !strings.Contains(req.ID, ",") {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Must use the ID of the source database cluster and the name of the user joined with a comma (e.g. `id,name`)",
		)
		return
	}
	s := strings.SplitN(req.ID, ",", 2)
	clusterID := s[0]
	name := s[1]
	id := makeDatabaseUserID(clusterID, name)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster_id"), clusterID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// makeDatabaseUserID is unchanged — kept for state-ID compatibility.
func makeDatabaseUserID(clusterID string, name string) string {
	return fmt.Sprintf("%s/user/%s", clusterID, name)
}

// settingsEqual is a simple deep-equality check for the settings slice used
// in Update to decide whether a settings update call is needed.
func settingsEqual(a, b []databaseUserSettings) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i].ACL) != len(b[i].ACL) {
			return false
		}
		for j := range a[i].ACL {
			if !a[i].ACL[j].Topic.Equal(b[i].ACL[j].Topic) ||
				!a[i].ACL[j].Permission.Equal(b[i].ACL[j].Permission) {
				return false
			}
		}
		if len(a[i].OpenSearchACL) != len(b[i].OpenSearchACL) {
			return false
		}
		for j := range a[i].OpenSearchACL {
			if !a[i].OpenSearchACL[j].Index.Equal(b[i].OpenSearchACL[j].Index) ||
				!a[i].OpenSearchACL[j].Permission.Equal(b[i].OpenSearchACL[j].Permission) {
				return false
			}
		}
	}
	return true
}

func expandUserSettingsFramework(settings []databaseUserSettings) *godo.DatabaseUserSettings {
	if len(settings) == 0 {
		return &godo.DatabaseUserSettings{}
	}
	s := settings[0]
	return &godo.DatabaseUserSettings{
		ACL:           expandUserACLsFramework(s.ACL),
		OpenSearchACL: expandOpenSearchUserACLsFramework(s.OpenSearchACL),
	}
}

func expandUserACLsFramework(acls []databaseUserACL) []*godo.KafkaACL {
	result := make([]*godo.KafkaACL, 0, len(acls))
	for _, a := range acls {
		result = append(result, &godo.KafkaACL{
			Topic:      a.Topic.ValueString(),
			Permission: a.Permission.ValueString(),
		})
	}
	return result
}

func expandOpenSearchUserACLsFramework(acls []databaseUserOpenSearchACL) []*godo.OpenSearchACL {
	result := make([]*godo.OpenSearchACL, 0, len(acls))
	for _, a := range acls {
		result = append(result, &godo.OpenSearchACL{
			Index:      a.Index.ValueString(),
			Permission: a.Permission.ValueString(),
		})
	}
	return result
}

func flattenUserSettingsFramework(settings *godo.DatabaseUserSettings) []databaseUserSettings {
	if settings == nil {
		return nil
	}
	return []databaseUserSettings{
		{
			ACL:           flattenUserACLsFramework(settings.ACL),
			OpenSearchACL: flattenOpenSearchUserACLsFramework(settings.OpenSearchACL),
		},
	}
}

func flattenUserACLsFramework(acls []*godo.KafkaACL) []databaseUserACL {
	result := make([]databaseUserACL, len(acls))
	for i, acl := range acls {
		result[i] = databaseUserACL{
			ID:         types.StringValue(acl.ID),
			Topic:      types.StringValue(acl.Topic),
			Permission: types.StringValue(normalizePermission(acl.Permission)),
		}
	}
	return result
}

func flattenOpenSearchUserACLsFramework(acls []*godo.OpenSearchACL) []databaseUserOpenSearchACL {
	result := make([]databaseUserOpenSearchACL, len(acls))
	for i, acl := range acls {
		result[i] = databaseUserOpenSearchACL{
			Index:      types.StringValue(acl.Index),
			Permission: types.StringValue(normalizeOpenSearchPermission(acl.Permission)),
		}
	}
	return result
}

// normalizePermission and normalizeOpenSearchPermission are unchanged from the
// SDKv2 implementation.
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

