// Copyright IBM Corp. 2014, 2025
// SPDX-License-Identifier: MPL-2.0

package applications

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/go-azure-helpers/lang/pointer"
	"github.com/hashicorp/go-azure-helpers/lang/response"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/applications/stable/application"
	"github.com/hashicorp/go-azure-sdk/microsoft-graph/common-types/stable"
	"github.com/hashicorp/go-azure-sdk/sdk/nullable"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-provider-azuread/internal/clients"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/consistency"
	"github.com/hashicorp/terraform-provider-azuread/internal/helpers/tf"
	"github.com/hashicorp/terraform-provider-azuread/internal/services/applications/parse"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                 = &applicationPasswordFrameworkResource{}
	_ resource.ResourceWithConfigure    = &applicationPasswordFrameworkResource{}
	_ resource.ResourceWithImportState  = &applicationPasswordFrameworkResource{}
	_ resource.ResourceWithUpgradeState = &applicationPasswordFrameworkResource{}
)

// NewApplicationPasswordResource is the factory used when registering this resource with
// the framework provider.
func NewApplicationPasswordResource() resource.Resource {
	return &applicationPasswordFrameworkResource{}
}

// applicationPasswordFrameworkResource is the framework resource type.
type applicationPasswordFrameworkResource struct {
	client *application.ApplicationClient
}

// applicationPasswordModel is the typed model for all state/plan operations.
// The tfsdk tags must exactly match the schema attribute names.
type applicationPasswordModel struct {
	// id stores the full credential ID: {objectId}/password/{keyId}
	// This is the Terraform resource ID managed by the framework.
	ID                types.String `tfsdk:"id"`
	ApplicationId     types.String `tfsdk:"application_id"`
	DisplayName       types.String `tfsdk:"display_name"`
	StartDate         types.String `tfsdk:"start_date"`
	EndDate           types.String `tfsdk:"end_date"`
	EndDateRelative   types.String `tfsdk:"end_date_relative"`
	RotateWhenChanged types.Map    `tfsdk:"rotate_when_changed"`
	KeyId             types.String `tfsdk:"key_id"`
	Value             types.String `tfsdk:"value"`
}

// --------------------------------------------------------------------------
// resource.Resource
// --------------------------------------------------------------------------

func (r *applicationPasswordFrameworkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application_password"
}

func (r *applicationPasswordFrameworkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// SchemaVersion 1 — V0→V1 upgrader is in UpgradeState below.
		Version: 1,

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"application_id": schema.StringAttribute{
				Description: "The resource ID of the application for which this password should be created",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					applicationIDValidator{},
				},
			},

			"display_name": schema.StringAttribute{
				Description: "A display name for the password",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"start_date": schema.StringAttribute{
				Description: "The start date from which the password is valid, formatted as an RFC3339 date string (e.g. `2018-01-01T01:02:03Z`). If this isn't specified, the current date is used",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					rfc3339TimeValidator{},
				},
			},

			"end_date": schema.StringAttribute{
				Description: "The end date until which the password is valid, formatted as an RFC3339 date string (e.g. `2018-01-01T01:02:03Z`)",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					rfc3339TimeValidator{},
					stringvalidator.ConflictsWith(path.Expressions{path.MatchRoot("end_date_relative")}...),
				},
			},

			"end_date_relative": schema.StringAttribute{
				Description:        "A relative duration for which the password is valid until, for example `240h` (10 days) or `2400h30m`. Changing this field forces a new resource to be created",
				Optional:           true,
				DeprecationMessage: "The `end_date_relative` property is deprecated and will be removed in a future version of the AzureAD provider. Please instead use the Terraform `timeadd()` function to calculate a value for the `end_date` property.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(path.Expressions{path.MatchRoot("end_date")}...),
				},
			},

			"rotate_when_changed": schema.MapAttribute{
				Description: "Arbitrary map of values that, when changed, will trigger rotation of the password",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},

			"key_id": schema.StringAttribute{
				Description: "A UUID used to uniquely identify this password credential",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"value": schema.StringAttribute{
				Description: "The password for this application, which is generated by Azure Active Directory",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// --------------------------------------------------------------------------
// resource.ResourceWithConfigure
// --------------------------------------------------------------------------

func (r *applicationPasswordFrameworkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*clients.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected provider data type",
			fmt.Sprintf("expected *clients.Client, got %T", req.ProviderData),
		)
		return
	}
	r.client = c.Applications.ApplicationClient
}

// --------------------------------------------------------------------------
// CRUD
// --------------------------------------------------------------------------

func (r *applicationPasswordFrameworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan applicationPasswordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	applicationId, err := stable.ParseApplicationID(plan.ApplicationId.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("application_id"), "Parsing `application_id`", err.Error())
		return
	}

	credential, err := passwordCredentialForModel(plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Generating password credentials",
			fmt.Sprintf("Generating password credentials for %s: %s", applicationId, err),
		)
		return
	}
	if credential == nil {
		resp.Diagnostics.AddError(
			"Generating password credentials",
			fmt.Sprintf("nil credential was returned for %s", applicationId),
		)
		return
	}

	tf.LockByName(applicationResourceName, applicationId.ApplicationId)
	defer tf.UnlockByName(applicationResourceName, applicationId.ApplicationId)

	getResp, err := r.client.GetApplication(ctx, *applicationId, application.DefaultGetApplicationOperationOptions())
	if err != nil {
		if response.WasNotFound(getResp.HttpResponse) {
			resp.Diagnostics.AddAttributeError(path.Root("application_id"), "Application not found", fmt.Sprintf("%s was not found", applicationId))
			return
		}
		resp.Diagnostics.AddAttributeError(path.Root("application_id"), "Retrieving application", fmt.Sprintf("Retrieving %s: %s", applicationId, err))
		return
	}

	app := getResp.Model
	if app == nil || app.Id == nil {
		resp.Diagnostics.AddError("API error", fmt.Sprintf("nil application or application with nil ID was returned for %s", applicationId))
		return
	}

	addPwResp, err := r.client.AddPassword(
		ctx,
		*applicationId,
		application.AddPasswordRequest{PasswordCredential: credential},
		application.DefaultAddPasswordOperationOptions(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Adding password", fmt.Sprintf("Adding password for %s: %s", applicationId, err))
		return
	}

	newCredential := addPwResp.Model
	if newCredential == nil {
		resp.Diagnostics.AddError("API error", fmt.Sprintf("nil credential received when adding password for %s", applicationId))
		return
	}
	if newCredential.KeyId.IsNull() {
		resp.Diagnostics.AddError("API error", fmt.Sprintf("nil or empty keyId received for %s", applicationId))
		return
	}

	password := newCredential.SecretText.GetOrZero()
	if len(password) == 0 {
		resp.Diagnostics.AddError("API error", fmt.Sprintf("nil or empty password received for %s", applicationId))
		return
	}

	credentialID := parse.NewCredentialID(applicationId.ApplicationId, "password", newCredential.KeyId.GetOrZero())

	// Wait for the credential to appear in the application manifest — can take several minutes.
	// Uses a simple poll loop to avoid importing the SDKv2 retry helper.
	const (
		pollInterval   = 1 * time.Second
		consecutiveHit = 5
	)
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(15 * time.Minute)
	}

	var consecutiveSeen int
	var polledForCredential *stable.PasswordCredential
	for time.Now().Before(deadline) {
		pollResp, pollErr := r.client.GetApplication(ctx, *applicationId, application.DefaultGetApplicationOperationOptions())
		if pollErr != nil {
			resp.Diagnostics.AddError("Waiting for credential", fmt.Sprintf("polling application for %s: %s", applicationId, pollErr))
			return
		}
		found := passwordCredentialByKeyID(pollResp.Model.PasswordCredentials, credentialID.KeyId)
		if found != nil {
			consecutiveSeen++
			polledForCredential = found
		} else {
			consecutiveSeen = 0
			polledForCredential = nil
		}
		if consecutiveSeen >= consecutiveHit {
			break
		}
		// Sleep respects context cancellation.
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Waiting for credential", fmt.Sprintf("context cancelled waiting for password credential for %s: %s", applicationId, ctx.Err()))
			return
		case <-time.After(pollInterval):
		}
	}

	if polledForCredential == nil {
		resp.Diagnostics.AddError("Waiting for credential", fmt.Sprintf("password credential not found in application manifest for %s after polling", applicationId))
		return
	}

	plan.ID = types.StringValue(credentialID.String())
	plan.KeyId = types.StringValue(credentialID.KeyId)
	plan.Value = types.StringValue(password)
	plan.ApplicationId = types.StringValue(applicationId.ID())

	// Persist the state before Read so the value is not lost.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Populate computed fields via Read.
	readResp := &resource.ReadResponse{State: resp.State}
	r.Read(ctx, resource.ReadRequest{State: resp.State}, readResp)
	resp.Diagnostics.Append(readResp.Diagnostics...)
	if !readResp.State.Raw.IsNull() {
		resp.State = readResp.State
	}
}

func (r *applicationPasswordFrameworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state applicationPasswordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve IDs from state. The credential ID is stored in id; application_id
	// is also stored separately for convenience.
	id, err := parse.PasswordID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Parsing credential ID from state", err.Error())
		return
	}

	applicationId := stable.NewApplicationID(id.ObjectId)

	getResp, err := r.client.GetApplication(ctx, applicationId, application.DefaultGetApplicationOperationOptions())
	if err != nil {
		if response.WasNotFound(getResp.HttpResponse) {
			log.Printf("[DEBUG] %s for password credential %q was not found - removing from state!", applicationId, id.KeyId)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddAttributeError(
			path.Root("application_id"),
			"Retrieving application",
			fmt.Sprintf("Retrieving %s: %s", applicationId, err),
		)
		return
	}

	app := getResp.Model
	if app == nil {
		resp.Diagnostics.AddError("API error", fmt.Sprintf("model was nil for %s", applicationId))
		return
	}

	credential := passwordCredentialByKeyID(app.PasswordCredentials, id.KeyId)
	if credential == nil {
		log.Printf("[DEBUG] Password credential %q (application %q) was not found - removing from state!", id.KeyId, id.ObjectId)
		resp.State.RemoveResource(ctx)
		return
	}

	state.ApplicationId = types.StringValue(applicationId.ID())

	if credential.DisplayName != nil {
		state.DisplayName = types.StringValue(credential.DisplayName.GetOrZero())
	} else if credential.CustomKeyIdentifier != nil {
		displayName, decodeErr := base64.StdEncoding.DecodeString(credential.CustomKeyIdentifier.GetOrZero())
		if decodeErr != nil {
			resp.Diagnostics.AddAttributeError(path.Root("display_name"), "Parsing CustomKeyIdentifier", decodeErr.Error())
			return
		}
		state.DisplayName = types.StringValue(string(displayName))
	}

	state.KeyId = types.StringValue(id.KeyId)

	if !credential.StartDateTime.IsNull() {
		state.StartDate = types.StringValue(credential.StartDateTime.GetOrZero())
	}
	if !credential.EndDateTime.IsNull() {
		state.EndDate = types.StringValue(credential.EndDateTime.GetOrZero())
	}

	// value is write-only — only set on Create; not returned by the API.
	// Leave state.Value as whatever is already in state.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *applicationPasswordFrameworkResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
	// All mutable attributes carry RequiresReplace, so Update is never called.
	// The framework requires this method even for immutable resources.
}

func (r *applicationPasswordFrameworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state applicationPasswordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := parse.PasswordID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Parsing credential ID from state", err.Error())
		return
	}

	applicationId := stable.NewApplicationID(id.ObjectId)

	tf.LockByName(applicationResourceName, id.ObjectId)
	defer tf.UnlockByName(applicationResourceName, id.ObjectId)

	removeReq := application.RemovePasswordRequest{KeyId: pointer.To(id.KeyId)}
	if _, err = r.client.RemovePassword(ctx, applicationId, removeReq, application.DefaultRemovePasswordOperationOptions()); err != nil {
		resp.Diagnostics.AddError(
			"Removing password credential",
			fmt.Sprintf("Removing password credential %q from %s: %s", id.KeyId, applicationId, err),
		)
		return
	}

	// Wait for the credential to disappear from the application manifest.
	if err = consistency.WaitForDeletion(ctx, func(ctx context.Context) (*bool, error) {
		getResp, getErr := r.client.GetApplication(ctx, applicationId, application.DefaultGetApplicationOperationOptions())
		if getErr != nil {
			return nil, getErr
		}
		app := getResp.Model
		if app == nil {
			return nil, errors.New("model was nil")
		}
		return pointer.To(passwordCredentialByKeyID(app.PasswordCredentials, id.KeyId) != nil), nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"Waiting for password credential deletion",
			fmt.Sprintf("Waiting for deletion of password credential %q from %s: %s", id.KeyId, applicationId, err),
		)
	}
}

// --------------------------------------------------------------------------
// resource.ResourceWithImportState
// --------------------------------------------------------------------------

func (r *applicationPasswordFrameworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID is the full credential string: {objectId}/password/{keyId}
	id, err := parse.PasswordID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID in the form {objectId}/password/{keyId}, parsing %q failed: %s", req.ID, err),
		)
		return
	}

	applicationId := stable.NewApplicationID(id.ObjectId)
	credentialID := parse.NewCredentialID(id.ObjectId, "password", id.KeyId)

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), credentialID.String())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("application_id"), applicationId.ID())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key_id"), id.KeyId)...)
}

// --------------------------------------------------------------------------
// resource.ResourceWithUpgradeState
// --------------------------------------------------------------------------

// UpgradeState maps prior schema versions to upgrader functions.
// There is one prior version (V0 in SDKv2). The V0 schema used a different
// set of attributes and an old {objectId}/{keyId} ID format.
// In the framework each upgrader must produce the *current* (V1) state
// directly — no chaining.
func (r *applicationPasswordFrameworkResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   applicationPasswordPriorSchemaV0(),
			StateUpgrader: upgradeApplicationPasswordStateV0,
		},
	}
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// passwordCredentialByKeyID finds a password credential by key ID.
// Inlined to avoid the SDKv2-coupled helpers/credentials package.
func passwordCredentialByKeyID(creds *[]stable.PasswordCredential, keyID string) *stable.PasswordCredential {
	if creds == nil {
		return nil
	}
	for _, cred := range *creds {
		if strings.EqualFold(cred.KeyId.GetOrZero(), keyID) {
			c := cred
			return &c
		}
	}
	return nil
}

// passwordCredentialForModel builds a PasswordCredential from the plan model.
// This replaces credentials.PasswordCredentialForResource (which takes *ResourceData).
func passwordCredentialForModel(m applicationPasswordModel) (*stable.PasswordCredential, error) {
	credential := stable.PasswordCredential{}

	if !m.DisplayName.IsNull() && !m.DisplayName.IsUnknown() && m.DisplayName.ValueString() != "" {
		credential.DisplayName = nullable.Value(m.DisplayName.ValueString())
	}

	if !m.StartDate.IsNull() && !m.StartDate.IsUnknown() && m.StartDate.ValueString() != "" {
		startDate, err := time.Parse(time.RFC3339, m.StartDate.ValueString())
		if err != nil {
			return nil, fmt.Errorf("unable to parse start_date %q: %w", m.StartDate.ValueString(), err)
		}
		credential.StartDateTime = nullable.Value(startDate.Format(time.RFC3339))
	}

	if !m.EndDate.IsNull() && !m.EndDate.IsUnknown() && m.EndDate.ValueString() != "" {
		expiry, err := time.Parse(time.RFC3339, m.EndDate.ValueString())
		if err != nil {
			return nil, fmt.Errorf("unable to parse end_date %q: %w", m.EndDate.ValueString(), err)
		}
		credential.EndDateTime = nullable.Value(expiry.Format(time.RFC3339))
	} else if !m.EndDateRelative.IsNull() && !m.EndDateRelative.IsUnknown() && m.EndDateRelative.ValueString() != "" {
		duration, err := time.ParseDuration(m.EndDateRelative.ValueString())
		if err != nil {
			return nil, fmt.Errorf("unable to parse end_date_relative %q as a duration: %w", m.EndDateRelative.ValueString(), err)
		}
		var base time.Time
		if credential.StartDateTime != nil && !credential.StartDateTime.IsNull() {
			base, err = time.Parse(time.RFC3339, credential.StartDateTime.GetOrZero())
			if err != nil {
				return nil, fmt.Errorf("unable to re-parse start_date: %w", err)
			}
		} else {
			base = time.Now()
		}
		credential.EndDateTime = nullable.Value(base.Add(duration).Format(time.RFC3339))
	}

	return &credential, nil
}

// --------------------------------------------------------------------------
// Custom validators
// --------------------------------------------------------------------------

// applicationIDValidator checks that a string parses as a valid Application resource ID.
// This replaces the SDKv2 ValidateFunc: stable.ValidateApplicationID.
type applicationIDValidator struct{}

func (v applicationIDValidator) Description(_ context.Context) string {
	return "value must be a valid Application resource ID (/applications/{applicationId})"
}

func (v applicationIDValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v applicationIDValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := stable.ParseApplicationID(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Application ID", err.Error())
	}
}

// rfc3339TimeValidator checks that a string is a valid RFC3339 timestamp.
// This replaces the SDKv2 ValidateFunc: validation.IsRFC3339Time.
type rfc3339TimeValidator struct{}

func (v rfc3339TimeValidator) Description(_ context.Context) string {
	return "value must be a valid RFC3339 timestamp"
}

func (v rfc3339TimeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v rfc3339TimeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := time.Parse(time.RFC3339, req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid RFC3339 timestamp",
			fmt.Sprintf("%q is not a valid RFC3339 time: %s", req.ConfigValue.ValueString(), err),
		)
	}
}
