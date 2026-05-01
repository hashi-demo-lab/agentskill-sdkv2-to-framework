package github

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UpgradeState wires the V0 -> current (V1) state upgrader in single-step
// semantics: each map entry produces the *current* state, not the next-version
// state. Per references/state-upgrade.md: "the upgrader keyed at 0 produces the
// current state, not V1 state" — for a single V0 -> V1 jump this is a 1:1
// translation, but it is still important to set Version on the schema and to
// supply PriorSchema explicitly.
//
// We bind the upgrader as a method on the resource type so that it has access
// to r.meta (the Owner) for the API call to look up the repository ID. SDKv2
// passed `m interface{}` to the upgrader; the framework signature does not, so
// closing over the configured client is the idiomatic substitute.
func (r *actionsEnvironmentSecretResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorSchemaActionsEnvironmentSecretV0(),
			StateUpgrader: r.upgradeStateV0ToCurrent,
		},
	}
}

// priorSchemaActionsEnvironmentSecretV0 mirrors the V0 attribute set from the
// SDKv2 file resource_github_actions_environment_secret_migration.go. ForceNew,
// validators, and Sensitive flags are NOT required at the prior-schema level
// (the framework only needs the shape to deserialise), but mirroring them keeps
// behaviour predictable if a practitioner runs `terraform plan` on a stale state
// before terraform-apply has run the upgrader.
func priorSchemaActionsEnvironmentSecretV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"repository": schema.StringAttribute{
				Required: true,
			},
			"environment": schema.StringAttribute{
				Required: true,
			},
			"secret_name": schema.StringAttribute{
				Required: true,
			},
			"encrypted_value": schema.StringAttribute{
				Optional: true,
			},
			"plaintext_value": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
			},
			"created_at": schema.StringAttribute{
				Computed: true,
			},
			"updated_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *actionsEnvironmentSecretResource) upgradeStateV0ToCurrent(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	if r.meta == nil {
		resp.Diagnostics.AddError(
			"provider not configured",
			"actions_environment_secret state upgrader needs a configured GitHub client; resource.Configure must run first",
		)
		return
	}

	var prior actionsEnvironmentSecretModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("[DEBUG] GitHub Actions Environment Secret Attributes before migration: %#v", prior)

	repoName := prior.Repository.ValueString()
	envName := prior.Environment.ValueString()
	secretName := prior.SecretName.ValueString()

	if repoName == "" {
		resp.Diagnostics.AddError("repository missing in V0 state", "repository must be a non-empty string")
		return
	}
	if envName == "" {
		resp.Diagnostics.AddError("environment missing in V0 state", "environment must be a non-empty string")
		return
	}
	if secretName == "" {
		resp.Diagnostics.AddError("secret_name missing in V0 state", "secret_name must be a non-empty string")
		return
	}

	client := r.meta.v3client
	owner := r.meta.name

	repo, _, err := client.Repositories.Get(ctx, owner, repoName)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to retrieve repository during state upgrade",
			fmt.Sprintf("repository %q: %s", repoName, err),
		)
		return
	}
	repoID := int(repo.GetID())

	id, err := buildID(repoName, escapeIDPart(envName), secretName)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to build id during state upgrade",
			fmt.Sprintf("repository %q environment %q secret %q: %s", repoName, envName, secretName, err),
		)
		return
	}

	// Translate V0 -> current. New attributes that did not exist in V0 are set
	// to null; the practitioner's next plan/apply will reconcile them via the
	// usual Read/Update path. The V1 schema treats encrypted_value /
	// plaintext_value as deprecated aliases for value_encrypted / value, so we
	// preserve whichever the V0 state had on the deprecated fields.
	current := actionsEnvironmentSecretModel{
		ID:              types.StringValue(id),
		Repository:      prior.Repository,
		RepositoryID:    types.Int64Value(int64(repoID)),
		Environment:     prior.Environment,
		SecretName:      prior.SecretName,
		KeyID:           types.StringNull(),
		Value:           types.StringNull(),
		ValueEncrypted:  types.StringNull(),
		EncryptedValue:  prior.EncryptedValue,
		PlaintextValue:  prior.PlaintextValue,
		CreatedAt:       prior.CreatedAt,
		UpdatedAt:       prior.UpdatedAt,
		RemoteUpdatedAt: types.StringNull(),
	}

	log.Printf("[DEBUG] GitHub Actions Environment Secret Attributes after migration: %#v", current)

	resp.Diagnostics.Append(resp.State.Set(ctx, &current)...)
}
