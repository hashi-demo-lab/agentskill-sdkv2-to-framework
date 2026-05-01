package openstack

import (
	"context"
	"time"

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

// Ensure the resource satisfies the framework interface.
var _ resource.ResourceWithConfigValidators = (*lbPoolV2Resource)(nil)

// lbPoolV2Resource is a placeholder type; only the Schema method is shown here.
type lbPoolV2Resource struct{}

func (r *lbPoolV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a V2 pool resource within OpenStack.",

		Blocks: map[string]schema.Block{
			// persistence uses a ListNestedBlock (not SingleNestedAttribute) because
			// in the SDK v2 schema it was TypeList/MaxItems:1. The framework idiom that
			// preserves identical HCL syntax (a single block body, written without
			// index brackets) is ListNestedBlock. SingleNestedAttribute would force
			// users to write it as an attribute assignment with = {...}, which is a
			// breaking configuration change. See reasoning.md for full discussion.
			"persistence": schema.ListNestedBlock{
				Description: "Defines the method of persistence to use for the pool. Can be either 'SOURCE_IP', 'HTTP_COOKIE', or 'APP_COOKIE'.",
				Validators: []validator.List{
					// Enforce the MaxItems: 1 constraint from the SDK schema.
					listvalidator.SizeAtMost(1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required:    true,
							Description: "The type of persistence mode. One of: SOURCE_IP, HTTP_COOKIE, APP_COOKIE.",
							Validators: []validator.String{
								stringvalidator.OneOf("SOURCE_IP", "HTTP_COOKIE", "APP_COOKIE"),
							},
						},
						"cookie_name": schema.StringAttribute{
							Optional:    true,
							Description: "The name of the cookie if persistence mode is set appropriately.",
						},
					},
				},
			},
		},

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique ID for the pool.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"region": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The region in which to obtain the V2 Networking client.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"tenant_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Required for admins. The UUID of the tenant who owns the pool.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Human-readable name for the pool.",
			},

			"description": schema.StringAttribute{
				Optional:    true,
				Description: "Human-readable description for the pool.",
			},

			"protocol": schema.StringAttribute{
				Required:    true,
				Description: "The protocol — either TCP, UDP, HTTP, HTTPS, PROXY, SCTP, or PROXYV2.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("TCP", "UDP", "HTTP", "HTTPS", "PROXY", "SCTP", "PROXYV2"),
				},
			},

			// ExactlyOneOf constraint: exactly one of loadbalancer_id / listener_id
			// must be set. In the framework this is expressed with
			// ExactlyOneOfValidator cross-attribute validators (shown inline).
			"loadbalancer_id": schema.StringAttribute{
				Optional:    true,
				Description: "The load balancer on which to provision this pool. Exactly one of loadbalancer_id or listener_id must be provided.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					// Cross-attribute validation: exactly one of the two must be set.
					// This requires a ConfigValidator on the resource; the attribute-level
					// ExactlyOneOf helper is available in terraform-plugin-framework-validators.
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("loadbalancer_id"),
						path.MatchRoot("listener_id"),
					),
				},
			},

			"listener_id": schema.StringAttribute{
				Optional:    true,
				Description: "The Listener on which the members of the pool will be associated with. Exactly one of loadbalancer_id or listener_id must be provided.",
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
				Required:    true,
				Description: "The load balancing algorithm to distribute traffic to the pool's members. One of: ROUND_ROBIN, LEAST_CONNECTIONS, SOURCE_IP, SOURCE_IP_PORT.",
				Validators: []validator.String{
					stringvalidator.OneOf("ROUND_ROBIN", "LEAST_CONNECTIONS", "SOURCE_IP", "SOURCE_IP_PORT"),
				},
			},

			"alpn_protocols": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "A list of ALPN protocols. Computed when not set (server provides defaults).",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("http/1.0", "http/1.1", "h2"),
					),
				},
			},

			"ca_tls_container_ref": schema.StringAttribute{
				Optional:    true,
				Description: "The ref of the key manager service secret containing a PEM format CA certificate bundle for tls_enabled pools.",
			},

			"crl_container_ref": schema.StringAttribute{
				Optional:    true,
				Description: "The URI of the key manager service secret containing a PEM format certificate revocation list (CRL) file to be used for tls_enabled pools.",
			},

			"tls_enabled": schema.BoolAttribute{
				Optional:    true,
				Description: "When true, connections to backend member servers will use TLS encryption.",
			},

			"tls_ciphers": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "List of ciphers in OpenSSL format (colon-separated). Computed when not set (server provides defaults).",
			},

			"tls_container_ref": schema.StringAttribute{
				Optional:    true,
				Description: "The ref of the key manager service secret containing a PKCS12 format certificate/key bundle for tls_enabled pools for TLS client authentication to the member servers.",
			},

			"tls_versions": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Description: "A list of TLS protocol versions. Computed when not set (server provides defaults).",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.OneOf("TLSv1", "TLSv1.1", "TLSv1.2", "TLSv1.3"),
					),
				},
			},

			"admin_state_up": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The administrative state of the pool. A valid value is true (UP) or false (DOWN). Default is true.",
				Default:     booldefault.StaticBool(true),
			},

			"tags": schema.SetAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "A list of simple strings assigned to the pool.",
			},
		},
	}
}

// Timeouts returns the configured timeouts for create/update/delete operations.
// This is separate from Schema() but shown here for completeness.
func (r *lbPoolV2Resource) timeouts() timeouts.Attributes {
	// Using the terraform-plugin-framework-timeouts helper:
	//   github.com/hashicorp/terraform-plugin-framework-timeouts
	return timeouts.Attributes{
		Create: timeouts.AttributeDefault(10 * time.Minute),
		Update: timeouts.AttributeDefault(10 * time.Minute),
		Delete: timeouts.AttributeDefault(10 * time.Minute),
	}
}
