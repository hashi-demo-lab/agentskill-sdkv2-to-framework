package github

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/go-github/v85/github"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface checks. A missing method becomes a compile error.
var (
	_ resource.Resource                   = &actionsEnvironmentSecretResource{}
	_ resource.ResourceWithConfigure      = &actionsEnvironmentSecretResource{}
	_ resource.ResourceWithImportState    = &actionsEnvironmentSecretResource{}
	_ resource.ResourceWithUpgradeState   = &actionsEnvironmentSecretResource{}
	_ resource.ResourceWithModifyPlan     = &actionsEnvironmentSecretResource{}
)

func NewActionsEnvironmentSecretResource() resource.Resource {
	return &actionsEnvironmentSecretResource{}
}

type actionsEnvironmentSecretResource struct {
	meta *Owner
}

// actionsEnvironmentSecretModel is the framework typed model for the current
// (V1) resource schema. Field names map by tfsdk tag.
type actionsEnvironmentSecretModel struct {
	ID              types.String `tfsdk:"id"`
	Repository      types.String `tfsdk:"repository"`
	RepositoryID    types.Int64  `tfsdk:"repository_id"`
	Environment     types.String `tfsdk:"environment"`
	SecretName      types.String `tfsdk:"secret_name"`
	KeyID           types.String `tfsdk:"key_id"`
	Value           types.String `tfsdk:"value"`
	ValueEncrypted  types.String `tfsdk:"value_encrypted"`
	EncryptedValue  types.String `tfsdk:"encrypted_value"`
	PlaintextValue  types.String `tfsdk:"plaintext_value"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
	RemoteUpdatedAt types.String `tfsdk:"remote_updated_at"`
}

// V0 prior model — only the attributes that existed at SchemaVersion 0.
// Used by the framework to deserialise legacy state via PriorSchema.
type actionsEnvironmentSecretModelV0 struct {
	ID             types.String `tfsdk:"id"`
	Repository     types.String `tfsdk:"repository"`
	Environment    types.String `tfsdk:"environment"`
	SecretName     types.String `tfsdk:"secret_name"`
	EncryptedValue types.String `tfsdk:"encrypted_value"`
	PlaintextValue types.String `tfsdk:"plaintext_value"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (r *actionsEnvironmentSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_actions_environment_secret"
}

func (r *actionsEnvironmentSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"repository": schema.StringAttribute{
				Required:    true,
				Description: "Name of the repository.",
			},
			"repository_id": schema.Int64Attribute{
				Computed:    true,
				Description: "ID of the repository.",
				PlanModifiers: []planmodifier.Int64{
					// Stable across refresh unless the repo actually changes — see ModifyPlan.
				},
			},
			"environment": schema.StringAttribute{
				Required:    true,
				Description: "Name of the environment.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"secret_name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the secret.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					secretNameValidator{},
				},
			},
			"key_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "ID of the public key used to encrypt the secret.",
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("value_encrypted")),
					stringvalidator.ConflictsWith(
						path.MatchRoot("value"),
						path.MatchRoot("plaintext_value"),
					),
				},
			},
			"value": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Plaintext value to be encrypted.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("value"),
						path.MatchRoot("value_encrypted"),
						path.MatchRoot("encrypted_value"),
						path.MatchRoot("plaintext_value"),
					),
				},
			},
			"value_encrypted": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Value encrypted with the GitHub public key, defined by key_id, in Base64 format.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`),
						"must be a valid base64 string",
					),
				},
			},
			"encrypted_value": schema.StringAttribute{
				Optional:           true,
				Sensitive:          true,
				Description:        "Encrypted value of the secret using the GitHub public key in Base64 format.",
				DeprecationMessage: "Use value_encrypted and key_id.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`),
						"must be a valid base64 string",
					),
				},
			},
			"plaintext_value": schema.StringAttribute{
				Optional:           true,
				Sensitive:          true,
				Description:        "Plaintext value of the secret to be encrypted.",
				DeprecationMessage: "Use value.",
			},
			"created_at": schema.StringAttribute{
				Computed:    true,
				Description: "Date of 'actions_environment_secret' creation.",
			},
			"updated_at": schema.StringAttribute{
				Computed:    true,
				Description: "Date of 'actions_environment_secret' update.",
			},
			"remote_updated_at": schema.StringAttribute{
				Computed:    true,
				Description: "Date of remote 'actions_environment_secret' update.",
			},
		},
	}
}

func (r *actionsEnvironmentSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	meta, ok := req.ProviderData.(*Owner)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected provider data",
			fmt.Sprintf("expected *Owner, got %T", req.ProviderData),
		)
		return
	}
	r.meta = meta
}

// Create implements the framework Resource Create method.
func (r *actionsEnvironmentSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan actionsEnvironmentSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.upsert(ctx, &plan, true); err != nil {
		resp.Diagnostics.AddError("failed to create environment secret", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *actionsEnvironmentSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state actionsEnvironmentSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.meta.v3client
	owner := r.meta.name

	repoName := state.Repository.ValueString()
	envName := state.Environment.ValueString()
	secretName := state.SecretName.ValueString()

	// Recover repository_id if missing (e.g., post-import). Read is allowed to
	// hit the API; ImportState is not.
	repoID := int(state.RepositoryID.ValueInt64())
	if repoID == 0 {
		repo, _, err := client.Repositories.Get(ctx, owner, repoName)
		if err != nil {
			resp.Diagnostics.AddError("failed to look up repository", err.Error())
			return
		}
		repoID = int(repo.GetID())
		state.RepositoryID = types.Int64Value(int64(repoID))
	}

	secret, _, err := client.Actions.GetEnvSecret(ctx, repoID, url.PathEscape(envName), secretName)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) {
			if ghErr.Response.StatusCode == http.StatusNotFound {
				log.Printf("[INFO] Removing environment secret %s from state because it no longer exists in GitHub", state.ID.ValueString())
				resp.State.RemoveResource(ctx)
				return
			}
		}
		resp.Diagnostics.AddError("failed to fetch environment secret", err.Error())
		return
	}

	id, err := buildID(repoName, escapeIDPart(envName), secretName)
	if err != nil {
		resp.Diagnostics.AddError("failed to build id", err.Error())
		return
	}
	state.ID = types.StringValue(id)

	// Eventually-consistent: only set if not already populated.
	if state.CreatedAt.IsNull() || state.CreatedAt.ValueString() == "" {
		state.CreatedAt = types.StringValue(secret.CreatedAt.String())
	}
	if state.UpdatedAt.IsNull() || state.UpdatedAt.ValueString() == "" {
		state.UpdatedAt = types.StringValue(secret.UpdatedAt.String())
	}
	state.RemoteUpdatedAt = types.StringValue(secret.UpdatedAt.String())

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *actionsEnvironmentSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan actionsEnvironmentSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state actionsEnvironmentSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Carry repository_id forward — Update body re-derives it.
	plan.RepositoryID = state.RepositoryID

	if err := r.upsert(ctx, &plan, false); err != nil {
		resp.Diagnostics.AddError("failed to update environment secret", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *actionsEnvironmentSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state actionsEnvironmentSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.meta.v3client
	repoID := int(state.RepositoryID.ValueInt64())
	envName := state.Environment.ValueString()
	secretName := state.SecretName.ValueString()

	log.Printf("[INFO] Deleting actions environment secret: %s", state.ID.ValueString())
	if _, err := client.Actions.DeleteEnvSecret(ctx, repoID, url.PathEscape(envName), secretName); err != nil {
		resp.Diagnostics.AddError("failed to delete environment secret", err.Error())
		return
	}
}

// upsert encapsulates the shared Create/Update logic.
func (r *actionsEnvironmentSecretResource) upsert(ctx context.Context, m *actionsEnvironmentSecretModel, isCreate bool) error {
	client := r.meta.v3client
	owner := r.meta.name

	repoName := m.Repository.ValueString()
	envName := m.Environment.ValueString()
	secretName := m.SecretName.ValueString()
	keyID := m.KeyID.ValueString()
	encryptedValue := firstNonEmpty(m.ValueEncrypted.ValueString(), m.EncryptedValue.ValueString())

	escapedEnvName := url.PathEscape(envName)

	var repoID int
	if isCreate {
		repo, _, err := client.Repositories.Get(ctx, owner, repoName)
		if err != nil {
			return err
		}
		repoID = int(repo.GetID())
	} else {
		repoID = int(m.RepositoryID.ValueInt64())
	}

	var publicKey string
	if len(keyID) == 0 || len(encryptedValue) == 0 {
		ki, pk, err := getEnvironmentPublicKeyDetails(ctx, r.meta, repoID, escapedEnvName)
		if err != nil {
			return err
		}
		keyID = ki
		publicKey = pk
	}

	if len(encryptedValue) == 0 {
		plaintextValue := firstNonEmpty(m.Value.ValueString(), m.PlaintextValue.ValueString())
		encryptedBytes, err := encryptPlaintext(plaintextValue, publicKey)
		if err != nil {
			return err
		}
		encryptedValue = base64.StdEncoding.EncodeToString(encryptedBytes)
	}

	secret := github.EncryptedSecret{
		Name:           secretName,
		KeyID:          keyID,
		EncryptedValue: encryptedValue,
	}
	if _, err := client.Actions.CreateOrUpdateEnvSecret(ctx, repoID, escapedEnvName, &secret); err != nil {
		return err
	}

	id, err := buildID(repoName, escapeIDPart(envName), secretName)
	if err != nil {
		return err
	}
	m.ID = types.StringValue(id)
	m.RepositoryID = types.Int64Value(int64(repoID))
	m.KeyID = types.StringValue(keyID)

	// GitHub API does not return on create/update — look up to populate timestamps.
	if remote, _, lookupErr := client.Actions.GetEnvSecret(ctx, repoID, escapedEnvName, secretName); lookupErr == nil {
		m.CreatedAt = types.StringValue(remote.CreatedAt.String())
		m.UpdatedAt = types.StringValue(remote.UpdatedAt.String())
		m.RemoteUpdatedAt = types.StringValue(remote.UpdatedAt.String())
	} else if !isCreate {
		m.UpdatedAt = types.StringNull()
		m.RemoteUpdatedAt = types.StringNull()
	}
	return nil
}

// ImportState parses the composite ID `repo:envEscaped:secretName` and sets just
// enough state for Read to populate the rest. Per references/import.md:
// "Don't call the API client. ... Set just enough that Read can find the resource."
func (r *actionsEnvironmentSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	repoName, envNamePart, secretName, err := parseID3(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"invalid import ID",
			fmt.Sprintf("expected '<repository>:<environment>:<secret_name>' (with environment URL-escaped via the project's escapeIDPart), got %q: %s", req.ID, err),
		)
		return
	}
	envName := unescapeIDPart(envNamePart)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("repository"), repoName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("environment"), envName)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("secret_name"), secretName)...)
}

// ModifyPlan implements the resource-level diff manipulation that was performed
// by SDKv2's customdiff.All(diffRepository, diffSecret). Two responsibilities:
//
//  1. diffRepository: if "repository" changed AND the prior repository_id no
//     longer matches the GitHub-side ID for the new name, force replacement.
//     This distinguishes a *rename* (same repo ID, no replacement) from a
//     replacement (new repo, requires recreate).
//  2. diffSecret: if remote_updated_at differs from updated_at, set updated_at
//     to the new remote value (or mark unknown), so subsequent plans don't
//     show drift on the timestamp.
func (r *actionsEnvironmentSecretResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip on destroy.
	if req.Plan.Raw.IsNull() {
		return
	}
	// Skip on create — no prior state, nothing to compare.
	if req.State.Raw.IsNull() {
		return
	}

	var plan, state actionsEnvironmentSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 1. Repository rename vs replacement.
	if !plan.Repository.Equal(state.Repository) && r.meta != nil {
		client := r.meta.v3client
		owner := r.meta.name
		newName := plan.Repository.ValueString()

		repo, _, err := client.Repositories.Get(ctx, owner, newName)
		if err != nil {
			var ghErr *github.ErrorResponse
			if errors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
				resp.RequiresReplace = path.Paths{path.Root("repository")}
			} else {
				// Network or other API failure — surface but do not force-replace.
				resp.Diagnostics.AddWarning(
					"could not verify repository identity for rename detection",
					err.Error(),
				)
			}
		} else {
			oldRepoID := state.RepositoryID.ValueInt64()
			if oldRepoID != repo.GetID() {
				resp.RequiresReplace = path.Paths{path.Root("repository")}
			}
		}
	}

	// 2. Drift on remote_updated_at -> updated_at handling.
	if !plan.RemoteUpdatedAt.Equal(state.RemoteUpdatedAt) {
		remoteUpdatedAt := plan.RemoteUpdatedAt.ValueString()
		if remoteUpdatedAt != "" {
			updatedAt := plan.UpdatedAt.ValueString()
			if updatedAt != remoteUpdatedAt {
				if updatedAt == "" {
					plan.UpdatedAt = types.StringValue(remoteUpdatedAt)
				} else {
					plan.UpdatedAt = types.StringUnknown()
				}
				resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
			}
		}
	}
}

// getEnvironmentPublicKeyDetails fetches the GitHub Actions environment-level
// public key (id + base64 key body) used to encrypt secrets. Carried over
// verbatim from the SDKv2 resource — no behavioural change.
func getEnvironmentPublicKeyDetails(ctx context.Context, meta *Owner, repoID int, envNameEscaped string) (string, string, error) {
	client := meta.v3client

	publicKey, _, err := client.Actions.GetEnvPublicKey(ctx, repoID, envNameEscaped)
	if err != nil {
		return "", "", err
	}

	return publicKey.GetKeyID(), publicKey.GetKey(), nil
}

// firstNonEmpty returns the first argument that is non-empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// secretNameValidator ports util.go's validateSecretNameFunc into a framework
// validator. Same regex, same GITHUB_ prefix rule.
type secretNameValidator struct{}

func (secretNameValidator) Description(ctx context.Context) string {
	return "secret name must match [a-zA-Z_][a-zA-Z0-9_]* and not start with GITHUB_"
}
func (secretNameValidator) MarkdownDescription(ctx context.Context) string {
	return "secret name must match `[a-zA-Z_][a-zA-Z0-9_]*` and not start with `GITHUB_`"
}
func (secretNameValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	name := req.ConfigValue.ValueString()
	if !secretNameRegexp.MatchString(name) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"invalid secret name",
			"secret names can only contain alphanumeric characters or underscores and must not start with a number",
		)
	}
	if strings.HasPrefix(strings.ToUpper(name), "GITHUB_") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"invalid secret name",
			"secret names must not start with the GITHUB_ prefix",
		)
	}
}
