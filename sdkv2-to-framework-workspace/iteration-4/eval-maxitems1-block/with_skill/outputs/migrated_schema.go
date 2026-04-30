package openstack

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure lbPoolV2Resource implements resource.Resource and resource.ResourceWithImportState.
var (
	_ resource.Resource                = &lbPoolV2Resource{}
	_ resource.ResourceWithImportState = &lbPoolV2Resource{}
)

type lbPoolV2Resource struct{}

func (r *lbPoolV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			// tenant_id is the project ID in OpenStack parlance.
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

			// Exactly one of loadbalancer_id or listener_id must be provided.
			"loadbalancer_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						// path.MatchRoot is used here to reference the sibling attribute.
						// Import: "github.com/hashicorp/terraform-plugin-framework/path"
						// path.MatchRoot("loadbalancer_id"),
						// path.MatchRoot("listener_id"),
						// NOTE: place the full path.Expressions on both attributes.
					),
				},
			},

			// Exactly one of loadbalancer_id or listener_id must be provided.
			"listener_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						// path.MatchRoot("loadbalancer_id"),
						// path.MatchRoot("listener_id"),
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

			// alpn_protocols is a set of strings; Computed because unsetting reverts to
			// an API-supplied default.
			"alpn_protocols": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
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
				// No ForceNew in the original; tls_enabled is mutable.
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},

			// tls_ciphers is Computed because unsetting reverts to an API-supplied default.
			"tls_ciphers": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"tls_container_ref": schema.StringAttribute{
				Optional: true,
			},

			// tls_versions is a set of strings; Computed because unsetting reverts to an
			// API-supplied default.
			"tls_versions": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"),
					),
				},
			},

			// admin_state_up defaults to true. The attribute must be Computed so the
			// framework can insert the default into the plan when the practitioner omits it.
			"admin_state_up": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},

			// tags is a set of strings. The SDKv2 schema.HashString function is dropped;
			// the framework handles set uniqueness internally.
			"tags": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
			},
		},

		Blocks: map[string]schema.Block{
			// persistence is a MaxItems:1 TypeList block in SDKv2.
			//
			// Migration decision: keep as ListNestedBlock (not SingleNestedAttribute).
			// See reasoning.md for the full analysis. The enforced-single-item constraint is
			// preserved via a listvalidator.SizeAtMost(1) validator, which is the canonical
			// framework equivalent of SDKv2 MaxItems:1 without a breaking HCL syntax change.
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
