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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Schema implements resource.Resource.
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

			// Exactly one of loadbalancer_id or listener_id must be provided.
			"loadbalancer_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.Expressions{
							path.MatchRoot("loadbalancer_id"),
							path.MatchRoot("listener_id"),
						}...,
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
						path.Expressions{
							path.MatchRoot("loadbalancer_id"),
							path.MatchRoot("listener_id"),
						}...,
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
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true, // unsetting results in a provider-side default
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
				Computed: true, // unsetting results in a provider-side default
			},

			"tls_container_ref": schema.StringAttribute{
				Optional: true,
			},

			"tls_versions": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true, // unsetting results in a provider-side default
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"),
					),
				},
			},

			"admin_state_up": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},

			"tags": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				// No hash function needed — the framework handles set uniqueness internally.
			},
		},

		// "persistence" is a MaxItems:1 block. See reasoning.md for the decision.
		// It is kept as a ListNestedBlock (with SizeAtMost(1)) to preserve the
		// existing block HCL syntax that practitioners already use.
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
