// Copyright IBM Corp. 2014, 2025
// SPDX-License-Identifier: MPL-2.0

package applications

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/go-azure-sdk/microsoft-graph/common-types/stable"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-provider-azuread/internal/services/applications/parse"
)

// applicationPasswordPriorSchemaV0 returns the framework schema representation
// of the SDKv2 V0 schema defined in
// internal/services/applications/migrations/application_password_resource.go.
//
// The V0 schema had different attribute names and used a two-segment ID format:
//
//	{objectId}/{keyId}   (no "/password/" middle segment)
//
// Notable differences from V1:
//   - "application_object_id" instead of "application_id"
//   - "description" instead of "display_name"
//   - No "rotate_when_changed"
//   - "value" was Required (user-supplied); in V1 it is Computed (API-generated)
func applicationPasswordPriorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},

			"application_object_id": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"key_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"value": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"start_date": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"end_date": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},

			"end_date_relative": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// applicationPasswordModelV0 is the typed model for deserialising V0 state.
// The tfsdk tags must exactly match applicationPasswordPriorSchemaV0's attribute names.
type applicationPasswordModelV0 struct {
	ID               types.String `tfsdk:"id"`
	ApplicationObjId types.String `tfsdk:"application_object_id"`
	KeyId            types.String `tfsdk:"key_id"`
	Description      types.String `tfsdk:"description"`
	Value            types.String `tfsdk:"value"`
	StartDate        types.String `tfsdk:"start_date"`
	EndDate          types.String `tfsdk:"end_date"`
	EndDateRelative  types.String `tfsdk:"end_date_relative"`
}

// upgradeApplicationPasswordStateV0 upgrades V0 state directly to the current (V1)
// schema. The framework requires each upgrader to produce the current state in a
// single call — no chaining.
//
// What this upgrader does:
//  1. Reads the old two-segment ID ({objectId}/{keyId}) and rewrites it as the
//     three-segment V1 form ({objectId}/password/{keyId}) using parse.OldPasswordID.
//  2. Re-maps "application_object_id" → "application_id" (as the full resource path).
//  3. Re-maps "description" → "display_name".
//  4. Sets "rotate_when_changed" to null (the attribute did not exist in V0).
func upgradeApplicationPasswordStateV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior applicationPasswordModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	log.Println("[DEBUG] Migrating azuread_application_password state from V0 to V1 (framework)")

	// Rewrite the ID from the old two-segment format to the three-segment format.
	oldID := prior.ID.ValueString()
	newCred, err := parse.OldPasswordID(oldID)
	if err != nil {
		resp.Diagnostics.AddError(
			"State upgrade failed",
			fmt.Sprintf("generating new ID from %q: %s", oldID, err),
		)
		return
	}

	// Build the full application resource ID from the object ID.
	newApplicationId := stable.NewApplicationID(newCred.ObjectId)

	current := applicationPasswordModel{
		ID:                types.StringValue(newCred.String()),
		ApplicationId:     types.StringValue(newApplicationId.ID()),
		DisplayName:       prior.Description,
		KeyId:             types.StringValue(newCred.KeyId),
		Value:             prior.Value,
		StartDate:         prior.StartDate,
		EndDate:           prior.EndDate,
		EndDateRelative:   prior.EndDateRelative,
		RotateWhenChanged: types.MapNull(types.StringType),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
