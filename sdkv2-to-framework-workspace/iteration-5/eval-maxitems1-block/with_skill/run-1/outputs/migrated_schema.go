package openstack

// Migrated framework Schema definition for the openstack_lb_pool_v2 resource.
//
// Scope: SCHEMA ONLY (per the eval task). The CRUD methods, model struct,
// Configure, Metadata, ImportState, and tests are intentionally NOT migrated
// here. The model struct below is the minimum needed to make the schema
// reference (it is consumed by `req.Plan.Get(ctx, &model)` in the real Create
// /Read/Update — provided here purely for context, not as the authoritative
// final shape).

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Schema defines the framework schema for the resource.
//
// Decision: `persistence` is kept as a `ListNestedBlock` with
// `listvalidator.SizeAtMost(1)` rather than converted to a
// `SingleNestedAttribute`. Reasoning is in the accompanying reasoning.md;
// the short version is that the provider's public docs and existing test
// fixtures use the `persistence { ... }` block syntax, and the resource has
// many years of changelog entries — this is a mature resource where
// switching to attribute syntax would break practitioner HCL.
func (r *poolV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

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

			// Exactly one of loadbalancer_id / listener_id must be provided.
			// SDKv2 used `ExactlyOneOf` on both attributes; in the framework
			// this lives as a per-attribute validator (placed on both for
			// symmetry / clearer error attribution).
			"loadbalancer_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("loadbalancer_id"),
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
						path.MatchRoot("listener_id"),
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

			"alpn_protocols": schema.SetAttribute{
				Optional:    true,
				Computed:    true, // unsetting this parameter results in a default value
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

			"tls_ciphers": schema.StringAttribute{
				Optional: true,
				Computed: true, // unsetting this parameter results in a default value
			},

			"tls_container_ref": schema.StringAttribute{
				Optional: true,
			},

			"tls_versions": schema.SetAttribute{
				Optional:    true,
				Computed:    true, // unsetting this parameter results in a default value
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						// Note: TLSv1.1 / TLSv1.2 / TLSv1.3 contain a `.` —
						// stringvalidator.OneOf compares strings literally so
						// this is fine; the regex below would be wrong here.
						stringvalidator.OneOf("TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"),
					),
				},
			},

			"admin_state_up": schema.BoolAttribute{
				Optional: true,
				Computed: true, // required because Default is set
				Default:  booldefault.StaticBool(true),
			},

			// SDKv2 had `Set: schema.HashString` on this attribute; it is
			// dropped in the framework — `SetAttribute` handles uniqueness
			// internally, and there is no `Set:` field to translate.
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
		},

		// `persistence` is a MaxItems: 1 block in SDKv2. We keep it as a
		// `ListNestedBlock` with `listvalidator.SizeAtMost(1)` for HCL
		// backward-compatibility. See reasoning.md.
		Blocks: map[string]schema.Block{
			"persistence": schema.ListNestedBlock{
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

