package openstack

import (
	"context"
	"time"

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
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
)

// lbPoolV2ResourceModel is the top-level Terraform state model for this resource.
type lbPoolV2ResourceModel struct {
	ID               types.String   `tfsdk:"id"`
	Region           types.String   `tfsdk:"region"`
	TenantID         types.String   `tfsdk:"tenant_id"`
	Name             types.String   `tfsdk:"name"`
	Description      types.String   `tfsdk:"description"`
	Protocol         types.String   `tfsdk:"protocol"`
	LoadbalancerID   types.String   `tfsdk:"loadbalancer_id"`
	ListenerID       types.String   `tfsdk:"listener_id"`
	LBMethod         types.String   `tfsdk:"lb_method"`
	ALPNProtocols    types.Set      `tfsdk:"alpn_protocols"`
	CATLSContainerRef types.String  `tfsdk:"ca_tls_container_ref"`
	CRLContainerRef  types.String   `tfsdk:"crl_container_ref"`
	TLSEnabled       types.Bool     `tfsdk:"tls_enabled"`
	TLSCiphers       types.String   `tfsdk:"tls_ciphers"`
	TLSContainerRef  types.String   `tfsdk:"tls_container_ref"`
	TLSVersions      types.Set      `tfsdk:"tls_versions"`
	AdminStateUp     types.Bool     `tfsdk:"admin_state_up"`
	Tags             types.Set      `tfsdk:"tags"`
	// persistence is kept as a block — see reasoning.md
	Persistence      []lbPoolV2PersistenceModel `tfsdk:"persistence"`
	Timeouts         timeouts.Value `tfsdk:"timeouts"`
}

// lbPoolV2PersistenceModel represents a single persistence block.
type lbPoolV2PersistenceModel struct {
	Type       types.String `tfsdk:"type"`
	CookieName types.String `tfsdk:"cookie_name"`
}

// Schema implements resource.Resource.
func (r *lbPoolV2Resource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
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

			// Exactly one of loadbalancer_id or listener_id must be set.
			// SDKv2 ExactlyOneOf is expressed here as cross-attribute validators
			// (listvalidator.ExactlyOneOf on each attribute); in practice the
			// provider's Configure/Create logic also enforces the constraint.
			"loadbalancer_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					// Replaces ExactlyOneOf — enforced at plan time.
					stringvalidator.ExactlyOneOf(
						"loadbalancer_id",
						"listener_id",
					),
				},
			},

			"listener_id": schema.StringAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				// No validator needed here — ExactlyOneOf on loadbalancer_id
				// already covers the pair.
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
				Computed:    true, // unsetting results in a provider default
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
				Computed: true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},

			"tls_ciphers": schema.StringAttribute{
				Optional: true,
				Computed: true, // unsetting results in a provider default
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"tls_container_ref": schema.StringAttribute{
				Optional: true,
			},

			"tls_versions": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true, // unsetting results in a provider default
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
			},
		},

		// Blocks — only "persistence" and "timeouts" use block syntax.
		// "persistence" (TypeList/MaxItems:1 in SDKv2) is kept as
		// ListNestedBlock rather than SingleNestedAttribute to preserve the
		// practitioner-facing HCL syntax. See reasoning.md for full decision.
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

			// timeouts block — framework-timeouts package
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
				CreateDescription: "Timeout for creating the lb pool (default 10m).",
				UpdateDescription: "Timeout for updating the lb pool (default 10m).",
				DeleteDescription: "Timeout for deleting the lb pool (default 10m).",
			}),
		},
	}
}

// defaultTimeout returns the SDKv2-equivalent default (10 minutes for all ops).
const lbPoolV2DefaultTimeout = 10 * time.Minute
