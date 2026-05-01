// Package openstack contains the migrated schema for the openstack_lb_pool_v2
// resource. Only the schema is migrated here; CRUD methods and tests are out
// of scope for this task.
package openstack

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
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

// Ensure lbPoolV2Resource satisfies the resource.Resource interface.
var _ resource.Resource = &lbPoolV2Resource{}

// lbPoolV2Resource is the framework resource type for openstack_lb_pool_v2.
type lbPoolV2Resource struct{}

// Metadata satisfies resource.Resource.
func (r *lbPoolV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_pool_v2"
}

// Schema returns the framework schema for openstack_lb_pool_v2.
// Only the schema is implemented here; CRUD methods are not migrated in this
// task.
func (r *lbPoolV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// ----------------------------------------------------------------
		// Attributes — scalar and collection fields
		// ----------------------------------------------------------------
		Attributes: map[string]schema.Attribute{
			// id: synthesised by the framework; Computed + UseStateForUnknown
			// so Terraform does not show it as unknown on every plan.
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			// region: Optional+Computed (the Computed flag covers the case
			// where the provider config implies the region). ForceNew in
			// SDKv2 → RequiresReplace plan modifier. UseStateForUnknown
			// suppresses spurious "(known after apply)" on plans.
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"tenant_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"name": schema.StringAttribute{
				Optional: true,
			},

			"description": schema.StringAttribute{
				Optional: true,
			},

			// protocol: Required, ForceNew, validated against an allowed set.
			"protocol": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(
						"TCP", "UDP", "HTTP", "HTTPS", "PROXY", "SCTP", "PROXYV2",
					),
				},
			},

			// loadbalancer_id / listener_id: exactly one must be provided.
			// The SDKv2 ExactlyOneOf field becomes a per-attribute cross-
			// attribute validator in the framework. Both attributes carry the
			// validator pointing at the other, which is the idiomatic pattern.
			//
			// NOTE: For symmetric mutual-exclusion constraints the resource-level
			// ConfigValidators approach (resourcevalidator.ExactlyOneOf) is
			// cleaner — see the comment at the bottom of this file.
			"loadbalancer_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("listener_id"),
					),
				},
			},

			"listener_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("loadbalancer_id"),
					),
				},
			},

			"lb_method": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(
						"ROUND_ROBIN", "LEAST_CONNECTIONS", "SOURCE_IP", "SOURCE_IP_PORT",
					),
				},
			},

			// alpn_protocols: TypeSet of primitive strings in SDKv2.
			// Framework: SetAttribute{ElementType: types.StringType}.
			// Computed because the API applies a default when this is unset.
			// Element-level ValidateFunc → setvalidator.ValueStringsAre.
			"alpn_protocols": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("http/1.0", "http/1.1", "h2"),
					),
				},
			},

			"ca_tls_container_ref": schema.StringAttribute{
				Optional: true,
			},

			"crl_container_ref": schema.StringAttribute{
				Optional: true,
			},

			"tls_enabled": schema.BoolAttribute{
				Optional: true,
			},

			// tls_ciphers: Computed because the API returns a default when
			// this is not configured.
			"tls_ciphers": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},

			"tls_container_ref": schema.StringAttribute{
				Optional: true,
			},

			// tls_versions: TypeSet of primitive strings.
			// Computed because the API applies a default when unset.
			"tls_versions": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"),
					),
				},
			},

			// admin_state_up: SDKv2 had Default: true.
			// Framework: Default field from the booldefault package (NOT a
			// plan modifier — see references/plan-modifiers.md "Default is not
			// a plan modifier"). An attribute with Default must be Computed so
			// the framework can inject it into the plan.
			"admin_state_up": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},

			// tags: TypeSet of primitive strings.
			// The SDKv2 Set: schema.HashString is dropped — the framework
			// handles set uniqueness internally; no hash function is needed.
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
		},

		// ----------------------------------------------------------------
		// Blocks — persistence (MaxItems: 1)
		//
		// DECISION: ListNestedBlock + listvalidator.SizeAtMost(1)
		//
		// The persistence block (TypeList, MaxItems:1, Elem &schema.Resource)
		// is kept as a BLOCK rather than converted to SingleNestedAttribute.
		// Full justification: see reasoning.md.
		//
		// Summary: openstack_lb_pool_v2 is a long-lived, production-used
		// resource. Practitioners write `persistence { type = "..." }` in
		// HCL today. Converting to SingleNestedAttribute changes the syntax
		// to `persistence = { type = "..." }`, which is a breaking change
		// for all existing configs. No major-version bump is in scope.
		// Per references/blocks.md decision tree Q1 (practitioners use block
		// syntax in production → keep as block), the correct output is
		// ListNestedBlock + listvalidator.SizeAtMost(1).
		//
		// ListNestedBlock is chosen over SingleNestedBlock because the SDKv2
		// runtime stored state under the list-indexed path "persistence.0.*".
		// Keeping the list-shaped state path avoids backward state
		// compatibility issues when reading existing state.
		// ----------------------------------------------------------------
		Blocks: map[string]schema.Block{
			"persistence": schema.ListNestedBlock{
				// SizeAtMost(1) encodes the SDKv2 MaxItems: 1 constraint.
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOf(
									"SOURCE_IP", "HTTP_COOKIE", "APP_COOKIE",
								),
							},
						},
						"cookie_name": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Alternative ExactlyOneOf implementation using ConfigValidators
//
// For symmetric mutual-exclusion constraints, a single resource-level
// validator is cleaner than reciprocal per-attribute validators. To use this:
//
//   var _ resource.ResourceWithConfigValidators = &lbPoolV2Resource{}
//
//   import "github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
//
//   func (r *lbPoolV2Resource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
//       return []resource.ConfigValidator{
//           resourcevalidator.ExactlyOneOf(
//               path.MatchRoot("loadbalancer_id"),
//               path.MatchRoot("listener_id"),
//           ),
//       }
//   }
//
// When this approach is adopted, remove the per-attribute
// stringvalidator.ExactlyOneOf from both loadbalancer_id and listener_id
// to avoid double-firing the same check.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Timeouts
//
// The SDKv2 resource declares Create/Update/Delete timeouts of 10 minutes.
// In the framework, timeouts are handled by the
// terraform-plugin-framework-timeouts package. A "timeouts" block attribute
// is added to the schema; the model struct carries a timeouts.Value field.
// Timeouts migration is outside the scope of this schema-only task.
// See references/timeouts.md.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Import
//
// The SDKv2 resourcePoolV2Import inspects the retrieved pool object to
// determine whether to populate listener_id or loadbalancer_id. In the
// framework this becomes ResourceWithImportState.ImportState using req.ID.
// Import migration is outside the scope of this schema-only task.
// See references/import.md.
// ---------------------------------------------------------------------------

// Placeholder CRUD stubs required to satisfy resource.Resource.
// These are not migrated in this schema-only task.
func (r *lbPoolV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
}
func (r *lbPoolV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}
func (r *lbPoolV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}
func (r *lbPoolV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}
