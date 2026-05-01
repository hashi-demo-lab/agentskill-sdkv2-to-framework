// migrated_schema.go
// Framework schema for openstack_lb_pool_v2.
// Source: openstack/resource_openstack_lb_pool_v2.go (SDKv2)
// Scope: schema only — CRUD methods and tests are unchanged.

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

// Ensure lbPoolV2Resource implements resource.Resource.
var _ resource.Resource = &lbPoolV2Resource{}

type lbPoolV2Resource struct{}

func (r *lbPoolV2Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lb_pool_v2"
}

// Schema returns the framework schema for openstack_lb_pool_v2.
//
// MaxItems:1 decision — "persistence" kept as ListNestedBlock:
//   The persistence block is a long-standing, documented feature of the
//   openstack_lb_pool_v2 resource. Practitioners in production write:
//       persistence { type = "HTTP_COOKIE" cookie_name = "..." }
//   Converting to SingleNestedAttribute would change this to:
//       persistence = { type = "HTTP_COOKIE" cookie_name = "..." }
//   which is a breaking HCL change for any existing config.  The OpenStack
//   provider is not undergoing a major-version bump, so backward-compat is
//   required.  The block is therefore preserved as ListNestedBlock with a
//   SizeAtMost(1) validator, exactly as recommended by references/blocks.md
//   for the "backward-compat" path.
func (r *lbPoolV2Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

			// Exactly one of loadbalancer_id or listener_id must be set.
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

			// alpn_protocols: TypeSet of primitive strings → SetAttribute.
			// Computed because unsetting results in an API-supplied default.
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

			// tls_ciphers: Computed because unsetting results in an API-supplied default.
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

			// tls_versions: TypeSet of primitive strings → SetAttribute.
			// Computed because unsetting results in an API-supplied default.
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

			// admin_state_up: Default: true in SDKv2.
			// In the framework, Default lives in the defaults package and the
			// attribute must be Computed: true so the framework can insert the
			// default into the plan.
			"admin_state_up": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},

			// tags: TypeSet of strings; Set: schema.HashString is dropped —
			// framework SetAttribute handles uniqueness internally.
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
		},

		Blocks: map[string]schema.Block{
			// persistence — MaxItems:1 block kept as ListNestedBlock.
			//
			// Decision: backward-compat (Output A from SKILL.md / blocks.md).
			// The openstack_lb_pool_v2 resource is mature; practitioners write
			// the block syntax.  Converting to SingleNestedAttribute would be a
			// breaking HCL change without a major-version bump.
			//
			// SizeAtMost(1) on the list validator enforces the original MaxItems:1
			// constraint in the framework.
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
