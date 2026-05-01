package lke

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	k8scondition "github.com/linode/linodego/k8s/pkg/condition"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
	"github.com/linode/terraform-provider-linode/v3/linode/helper/setplanmodifiers"
	"github.com/linode/terraform-provider-linode/v3/linode/lkenodepool"
)

const (
	createLKETimeout = 35 * time.Minute
	updateLKETimeout = 40 * time.Minute
	deleteLKETimeout = 20 * time.Minute
	TierEnterprise   = "enterprise"
	TierStandard     = "standard"
)

// Ensure interface compliance.
var (
	_ resource.Resource                = &lkeClusterResource{}
	_ resource.ResourceWithConfigure   = &lkeClusterResource{}
	_ resource.ResourceWithImportState = &lkeClusterResource{}
	_ resource.ResourceWithModifyPlan  = &lkeClusterResource{}
)

// lkeClusterResource is the resource implementation.
type lkeClusterResource struct {
	helper.BaseResource
}

// NewResource returns a new lke cluster resource.
func NewResource() resource.Resource {
	return &lkeClusterResource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_lke_cluster",
				IDType: types.Int64Type,
				Schema: &lkeClusterSchema,
				TimeoutOpts: &timeouts.Opts{
					Create: true,
					Update: true,
					Delete: true,
				},
			},
		),
	}
}

// lkeClusterResourceModel is the Terraform data model for the resource.
type lkeClusterResourceModel struct {
	ID               types.Int64    `tfsdk:"id"`
	Label            types.String   `tfsdk:"label"`
	K8sVersion       types.String   `tfsdk:"k8s_version"`
	APLEnabled       types.Bool     `tfsdk:"apl_enabled"`
	Tags             types.Set      `tfsdk:"tags"`
	ExternalPoolTags types.Set      `tfsdk:"external_pool_tags"`
	Region           types.String   `tfsdk:"region"`
	APIEndpoints     types.List     `tfsdk:"api_endpoints"`
	Kubeconfig       types.String   `tfsdk:"kubeconfig"`
	DashboardURL     types.String   `tfsdk:"dashboard_url"`
	Status           types.String   `tfsdk:"status"`
	Tier             types.String   `tfsdk:"tier"`
	SubnetID         types.Int64    `tfsdk:"subnet_id"`
	VpcID            types.Int64    `tfsdk:"vpc_id"`
	StackType        types.String   `tfsdk:"stack_type"`
	Pool             types.List     `tfsdk:"pool"`
	ControlPlane     types.List     `tfsdk:"control_plane"`
	Timeouts         timeouts.Value `tfsdk:"timeouts"`
}

// poolModel is the data model for an LKE node pool within the resource.
type poolModel struct {
	ID             types.Int64  `tfsdk:"id"`
	Label          types.String `tfsdk:"label"`
	Count          types.Int64  `tfsdk:"count"`
	Type           types.String `tfsdk:"type"`
	FirewallID     types.Int64  `tfsdk:"firewall_id"`
	Labels         types.Map    `tfsdk:"labels"`
	Taint          types.Set    `tfsdk:"taint"`
	Tags           types.Set    `tfsdk:"tags"`
	DiskEncryption types.String `tfsdk:"disk_encryption"`
	Nodes          types.List   `tfsdk:"nodes"`
	Autoscaler     types.List   `tfsdk:"autoscaler"`
	K8sVersion     types.String `tfsdk:"k8s_version"`
	UpdateStrategy types.String `tfsdk:"update_strategy"`
}

// autoscalerModel is the data model for a node pool autoscaler.
type autoscalerModel struct {
	Min types.Int64 `tfsdk:"min"`
	Max types.Int64 `tfsdk:"max"`
}

// nodeModel is the data model for a node in a pool.
type nodeModel struct {
	ID         types.String `tfsdk:"id"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	Status     types.String `tfsdk:"status"`
}

// taintModel is the data model for a node pool taint.
type taintModel struct {
	Effect types.String `tfsdk:"effect"`
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
}

// controlPlaneModel is the data model for the control_plane block.
type controlPlaneModel struct {
	HighAvailability types.Bool `tfsdk:"high_availability"`
	AuditLogsEnabled types.Bool `tfsdk:"audit_logs_enabled"`
	ACL              types.List `tfsdk:"acl"`
}

// aclModel is the data model for the control_plane ACL block.
type aclModel struct {
	Enabled   types.Bool `tfsdk:"enabled"`
	Addresses types.List `tfsdk:"addresses"`
}

// aclAddressesModel is the data model for the ACL addresses block.
type aclAddressesModel struct {
	IPv4 types.Set `tfsdk:"ipv4"`
	IPv6 types.Set `tfsdk:"ipv6"`
}

// Attribute types for nested objects.
var autoscalerAttrTypes = map[string]attr.Type{
	"min": types.Int64Type,
	"max": types.Int64Type,
}

var nodeAttrTypes = map[string]attr.Type{
	"id":          types.StringType,
	"instance_id": types.Int64Type,
	"status":      types.StringType,
}

var taintAttrTypes = map[string]attr.Type{
	"effect": types.StringType,
	"key":    types.StringType,
	"value":  types.StringType,
}

var aclAddressesAttrTypes = map[string]attr.Type{
	"ipv4": types.SetType{ElemType: types.StringType},
	"ipv6": types.SetType{ElemType: types.StringType},
}

var aclAttrTypes = map[string]attr.Type{
	"enabled":   types.BoolType,
	"addresses": types.ListType{ElemType: types.ObjectType{AttrTypes: aclAddressesAttrTypes}},
}

var controlPlaneAttrTypes = map[string]attr.Type{
	"high_availability":  types.BoolType,
	"audit_logs_enabled": types.BoolType,
	"acl":                types.ListType{ElemType: types.ObjectType{AttrTypes: aclAttrTypes}},
}

var poolAttrTypes = map[string]attr.Type{
	"id":              types.Int64Type,
	"label":           types.StringType,
	"count":           types.Int64Type,
	"type":            types.StringType,
	"firewall_id":     types.Int64Type,
	"labels":          types.MapType{ElemType: types.StringType},
	"taint":           types.SetType{ElemType: types.ObjectType{AttrTypes: taintAttrTypes}},
	"tags":            types.SetType{ElemType: types.StringType},
	"disk_encryption": types.StringType,
	"nodes":           types.ListType{ElemType: types.ObjectType{AttrTypes: nodeAttrTypes}},
	"autoscaler":      types.ListType{ElemType: types.ObjectType{AttrTypes: autoscalerAttrTypes}},
	"k8s_version":     types.StringType,
	"update_strategy": types.StringType,
}

// lkeClusterSchema is the Terraform schema for the resource.
var lkeClusterSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			Computed: true,
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"label": schema.StringAttribute{
			Required:    true,
			Description: "The unique label for the cluster.",
		},
		"k8s_version": schema.StringAttribute{
			Required: true,
			Description: "The desired Kubernetes version for this Kubernetes cluster in the format of <major>.<minor>. " +
				"The latest supported patch version will be deployed.",
		},
		"apl_enabled": schema.BoolAttribute{
			Description: "Enables the App Platform Layer for this cluster. " +
				"Note: v4beta only and may not currently be available to all users.",
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.RequiresReplace(),
			},
		},
		"tags": schema.SetAttribute{
			ElementType: types.StringType,
			Optional:    true,
			Computed:    true,
			Description: "An array of tags applied to this object. Tags are for organizational purposes only.",
			PlanModifiers: []planmodifier.Set{
				setplanmodifiers.CaseInsensitiveSet(),
			},
		},
		"external_pool_tags": schema.SetAttribute{
			ElementType: types.StringType,
			Optional:    true,
			Description: "An array of tags indicating that node pools having those tags are defined with a separate nodepool resource, rather than inside the current cluster resource.",
		},
		"region": schema.StringAttribute{
			Required:    true,
			Description: "This cluster's location.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"api_endpoints": schema.ListAttribute{
			ElementType: types.StringType,
			Computed:    true,
			Description: "The API endpoints for the cluster.",
		},
		"kubeconfig": schema.StringAttribute{
			Computed:    true,
			Sensitive:   true,
			Description: "The Base64-encoded Kubeconfig for the cluster.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"dashboard_url": schema.StringAttribute{
			Computed:    true,
			Description: "The dashboard URL of the cluster.",
		},
		"status": schema.StringAttribute{
			Computed:    true,
			Description: "The status of the cluster.",
		},
		"tier": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "The desired Kubernetes tier.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"subnet_id": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Description: "The ID of the VPC subnet to use for the Kubernetes cluster. This subnet must be dual stack (IPv4 and IPv6 should both be enabled). ",
		},
		"vpc_id": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Description: "The ID of the VPC to use for the Kubernetes cluster.",
		},
		"stack_type": schema.StringAttribute{
			Optional:    true,
			Computed:    true,
			Description: "The networking stack type of the Kubernetes cluster.",
			Validators: []validator.String{
				stringvalidator.OneOf(
					string(linodego.LKEClusterStackIPv4),
					string(linodego.LKEClusterDualStack),
				),
			},
		},
		"pool": schema.ListNestedAttribute{
			Optional:    true,
			Description: "A node pool in the cluster. At least one pool is required for standard tier clusters.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Computed:    true,
						Description: "The ID of the Node Pool.",
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"label": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The label of the Node Pool.",
					},
					"count": schema.Int64Attribute{
						Optional:    true,
						Computed:    true,
						Description: "The number of nodes in the Node Pool.",
					},
					"type": schema.StringAttribute{
						Required:    true,
						Description: "A Linode Type for all of the nodes in the Node Pool.",
					},
					"firewall_id": schema.Int64Attribute{
						Optional:    true,
						Computed:    true,
						Description: "The ID of the Firewall to attach to nodes in this node pool.",
					},
					"labels": schema.MapAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Computed:    true,
						Description: "Key-value pairs added as labels to nodes in the node pool. " +
							"Labels help classify your nodes and to easily select subsets of objects.",
					},
					"taint": schema.SetNestedAttribute{
						Optional:    true,
						Description: "Kubernetes taints to add to node pool nodes. Taints help control how pods are scheduled onto nodes, specifically allowing them to repel certain pods.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"effect": schema.StringAttribute{
									Required:    true,
									Description: "The Kubernetes taint effect.",
									Validators: []validator.String{
										stringvalidator.OneOf(
											string(linodego.LKENodePoolTaintEffectNoExecute),
											string(linodego.LKENodePoolTaintEffectNoSchedule),
											string(linodego.LKENodePoolTaintEffectPreferNoSchedule),
										),
									},
								},
								"key": schema.StringAttribute{
									Required:    true,
									Description: "The Kubernetes taint key.",
									Validators: []validator.String{
										stringvalidator.LengthAtLeast(1),
									},
								},
								"value": schema.StringAttribute{
									Required:    true,
									Description: "The Kubernetes taint value.",
									Validators: []validator.String{
										stringvalidator.LengthAtLeast(1),
									},
								},
							},
						},
					},
					"tags": schema.SetAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Computed:    true,
						Description: "A set of tags applied to this node pool.",
					},
					"disk_encryption": schema.StringAttribute{
						Computed:    true,
						Description: "The disk encryption policy for the nodes in this pool.",
					},
					"nodes": schema.ListNestedAttribute{
						Computed:    true,
						Description: "The nodes in the node pool.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{
									Computed:    true,
									Description: "The ID of the node.",
								},
								"instance_id": schema.Int64Attribute{
									Computed:    true,
									Description: "The ID of the underlying Linode instance.",
								},
								"status": schema.StringAttribute{
									Computed:    true,
									Description: "The status of the node.",
								},
							},
						},
					},
					"autoscaler": schema.ListNestedAttribute{
						Optional:    true,
						Computed:    true,
						Description: "When specified, the number of nodes autoscales within the defined minimum and maximum values.",
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"min": schema.Int64Attribute{
									Required:    true,
									Description: "The minimum number of nodes to autoscale to.",
								},
								"max": schema.Int64Attribute{
									Required:    true,
									Description: "The maximum number of nodes to autoscale to.",
								},
							},
						},
					},
					"k8s_version": schema.StringAttribute{
						Computed:    true,
						Optional:    true,
						Description: "The desired Kubernetes version for this pool. This is only available for Enterprise clusters.",
					},
					"update_strategy": schema.StringAttribute{
						Computed:    true,
						Optional:    true,
						Description: "The strategy for updating the node pool k8s version. For LKE enterprise only and may not currently available to all users.",
						Validators: []validator.String{
							stringvalidator.OneOf(
								string(linodego.LKENodePoolOnRecycle),
								string(linodego.LKENodePoolRollingUpdate),
							),
						},
					},
				},
			},
		},
	},
	Blocks: map[string]schema.Block{
		// control_plane has been in the provider since launch; practitioners write
		// block syntax in HCL.  Keep as ListNestedBlock + SizeAtMost(1) to preserve
		// the control_plane.0.<field> state paths from SDKv2.
		"control_plane": schema.ListNestedBlock{
			Description: "Defines settings for the Kubernetes Control Plane.",
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"high_availability": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Defines whether High Availability is enabled for the Control Plane Components of the cluster.",
					},
					"audit_logs_enabled": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Enables audit logs on the cluster's control plane.",
					},
				},
				Blocks: map[string]schema.Block{
					"acl": schema.ListNestedBlock{
						Description: "Defines the ACL configuration for an LKE cluster's control plane.",
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"enabled": schema.BoolAttribute{
									Optional:    true,
									Computed:    true,
									Description: "Defines default policy. A value of true results in a default policy of DENY. A value of false results in default policy of ALLOW, and has the same effect as deleting the ACL configuration.",
								},
							},
							Blocks: map[string]schema.Block{
								"addresses": schema.ListNestedBlock{
									Description: "A list of ip addresses to allow.",
									Validators: []validator.List{
										listvalidator.SizeAtMost(1),
									},
									NestedObject: schema.NestedBlockObject{
										Attributes: map[string]schema.Attribute{
											"ipv4": schema.SetAttribute{
												ElementType: types.StringType,
												Optional:    true,
												Computed:    true,
												Description: "A set of individual ipv4 addresses or CIDRs to ALLOW.",
											},
											"ipv6": schema.SetAttribute{
												ElementType: types.StringType,
												Optional:    true,
												Computed:    true,
												Description: "A set of individual ipv6 addresses or CIDRs to ALLOW.",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		// timeouts block is injected by BaseResource.Schema via TimeoutOpts.
	},
}

// ModifyPlan implements resource.ResourceWithModifyPlan.
// Translates the SDKv2 customdiff.All chain:
//   1. customDiffValidatePoolForStandardTier
//   2. customDiffValidateOptionalCount
//   3. customDiffValidateUpdateStrategyWithTier
//   4. SDKv2ValidateFieldRequiresAPIVersion(v4beta, "tier")
//   5. ComputedWithDefault("tags", []string{})
//   6. CaseInsensitiveSet("tags") — already handled via plan modifier on the attribute.
func (r *lkeClusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Skip on destroy.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan lkeClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tierValue := plan.Tier.ValueString()
	tierIsStandard := plan.Tier.IsNull() || plan.Tier.IsUnknown() || tierValue == TierStandard || tierValue == ""
	tierIsEnterprise := !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && tierValue == TierEnterprise

	// Leg 1: standard tier requires at least one pool.
	if tierIsStandard {
		pools := r.planPools(ctx, plan, resp)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(pools) == 0 {
			resp.Diagnostics.AddError(
				"Invalid configuration",
				"at least one pool is required for standard tier clusters",
			)
			return
		}
	}

	// Leg 2: each pool must have a count or an autoscaler.
	if !plan.Pool.IsNull() && !plan.Pool.IsUnknown() {
		pools := r.planPools(ctx, plan, resp)
		if resp.Diagnostics.HasError() {
			return
		}
		invalidPools := make([]string, 0)
		for i, pool := range pools {
			countIsZeroOrNull := pool.Count.IsNull() || pool.Count.IsUnknown() || pool.Count.ValueInt64() == 0
			if !countIsZeroOrNull {
				continue
			}
			hasAutoscaler := false
			if !pool.Autoscaler.IsNull() && !pool.Autoscaler.IsUnknown() {
				var scalers []autoscalerModel
				resp.Diagnostics.Append(pool.Autoscaler.ElementsAs(ctx, &scalers, false)...)
				if resp.Diagnostics.HasError() {
					return
				}
				hasAutoscaler = len(scalers) > 0
			}
			if !hasAutoscaler {
				invalidPools = append(invalidPools, fmt.Sprintf("pool.%d", i))
			}
		}
		if len(invalidPools) > 0 {
			resp.Diagnostics.AddError(
				"Invalid pool configuration",
				fmt.Sprintf(
					"%s: `count` must be defined when no autoscaler is defined",
					strings.Join(invalidPools, ", "),
				),
			)
			return
		}
	}

	// Leg 3: update_strategy requires enterprise tier.
	if !tierIsEnterprise && !plan.Pool.IsNull() && !plan.Pool.IsUnknown() {
		pools := r.planPools(ctx, plan, resp)
		if resp.Diagnostics.HasError() {
			return
		}
		invalidPools := make([]string, 0)
		for i, pool := range pools {
			if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
				invalidPools = append(invalidPools, fmt.Sprintf("pool.%d", i))
			}
		}
		if len(invalidPools) > 0 {
			resp.Diagnostics.AddError(
				"Invalid pool configuration",
				fmt.Sprintf(
					"%s: `update_strategy` can only be configured when tier is set to \"enterprise\"",
					strings.Join(invalidPools, ", "),
				),
			)
			return
		}
	}

	// Leg 4: tier field requires api_version=v4beta.
	if !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && plan.Tier.ValueString() != "" {
		if r.Meta != nil && r.Meta.Config != nil {
			apiVersion := r.Meta.Config.APIVersion.ValueString()
			if !strings.EqualFold(apiVersion, helper.APIVersionV4Beta) {
				resp.Diagnostics.AddError(
					"Invalid API version",
					fmt.Sprintf(
						"tier: The api_version provider argument must be set to '%s' to use this field.",
						helper.APIVersionV4Beta,
					),
				)
				return
			}
		}
	}

	// Leg 5: default tags to empty set when not configured (ComputedWithDefault equivalent).
	if plan.Tags.IsNull() {
		emptyTags, d := types.SetValueFrom(ctx, types.StringType, []string{})
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tags"), emptyTags)...)
	}
}

// planPools extracts pool models from the current plan; errors are appended to resp.
func (r *lkeClusterResource) planPools(
	ctx context.Context,
	plan lkeClusterResourceModel,
	resp *resource.ModifyPlanResponse,
) []poolModel {
	if plan.Pool.IsNull() || plan.Pool.IsUnknown() {
		return nil
	}
	var pools []poolModel
	resp.Diagnostics.Append(plan.Pool.ElementsAs(ctx, &pools, false)...)
	return pools
}

// Create implements resource.Resource.
func (r *lkeClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lkeClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, d := plan.Timeouts.Create(ctx, createLKETimeout)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{"cluster_id": "new"})
	tflog.Debug(ctx, "Create linode_lke_cluster")

	client := r.Meta.Client

	createOpts := linodego.LKEClusterCreateOptions{
		Label:      plan.Label.ValueString(),
		Region:     plan.Region.ValueString(),
		K8sVersion: plan.K8sVersion.ValueString(),
	}

	if !plan.Tier.IsNull() && !plan.Tier.IsUnknown() {
		createOpts.Tier = plan.Tier.ValueString()
	}
	if !plan.APLEnabled.IsNull() && !plan.APLEnabled.IsUnknown() {
		createOpts.APLEnabled = plan.APLEnabled.ValueBool()
	}
	if !plan.SubnetID.IsNull() && !plan.SubnetID.IsUnknown() {
		createOpts.SubnetID = linodego.Pointer(int(plan.SubnetID.ValueInt64()))
	}
	if !plan.VpcID.IsNull() && !plan.VpcID.IsUnknown() {
		createOpts.VpcID = linodego.Pointer(int(plan.VpcID.ValueInt64()))
	}
	if !plan.StackType.IsNull() && !plan.StackType.IsUnknown() {
		createOpts.StackType = linodego.Pointer(linodego.LKEClusterStackType(plan.StackType.ValueString()))
	}

	// Expand control_plane.
	if !plan.ControlPlane.IsNull() && !plan.ControlPlane.IsUnknown() {
		var cpModels []controlPlaneModel
		resp.Diagnostics.Append(plan.ControlPlane.ElementsAs(ctx, &cpModels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(cpModels) > 0 {
			expandedCP, d := expandControlPlaneOptionsFramework(ctx, cpModels[0])
			resp.Diagnostics.Append(d...)
			if resp.Diagnostics.HasError() {
				return
			}
			createOpts.ControlPlane = &expandedCP
		}
	}

	// Expand pool.
	if !plan.Pool.IsNull() && !plan.Pool.IsUnknown() {
		var pools []poolModel
		resp.Diagnostics.Append(plan.Pool.ElementsAs(ctx, &pools, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for _, p := range pools {
			poolOpts, d := expandPoolCreateOptions(ctx, p)
			resp.Diagnostics.Append(d...)
			if resp.Diagnostics.HasError() {
				return
			}
			createOpts.NodePools = append(createOpts.NodePools, poolOpts)
		}
	}

	// Expand tags.
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.Tags = tags
	}

	tflog.Debug(ctx, "client.CreateLKECluster(...)", map[string]any{"options": createOpts})
	cluster, err := client.CreateLKECluster(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create LKE cluster", err.Error())
		return
	}

	plan.ID = types.Int64Value(int64(cluster.ID))
	ctx = tflog.SetField(ctx, "cluster_id", cluster.ID)

	// Enterprise clusters need longer for kubeconfig to be generated.
	retryTimeout := time.Second * 25
	if cluster.Tier == TierEnterprise {
		retryTimeout = time.Second * 120
		pollMS := r.Meta.Config.EventPollMilliseconds.ValueInt64()
		if err := waitForLKEKubeConfig(ctx, *client, int(pollMS), cluster.ID); err != nil {
			resp.Diagnostics.AddError("Failed to get LKE cluster kubeconfig", err.Error())
			return
		}
	}

	tflog.Debug(ctx, "Waiting for a single LKE cluster node to be ready")
	_ = retryUntil(ctx, retryTimeout, func() error {
		tflog.Debug(ctx, "client.WaitForLKEClusterCondition(...)", map[string]any{"condition": "ClusterHasReadyNode"})
		return client.WaitForLKEClusterConditions(ctx, cluster.ID, linodego.LKEClusterPollOptions{
			TimeoutSeconds: 15 * 60,
		}, k8scondition.ClusterHasReadyNode)
	})

	resp.Diagnostics.Append(r.readIntoState(ctx, cluster.ID, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read implements resource.Resource.
func (r *lkeClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lkeClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{"cluster_id": state.ID.ValueInt64()})
	tflog.Debug(ctx, "Read linode_lke_cluster")

	id := int(state.ID.ValueInt64())
	resp.Diagnostics.Append(r.readIntoState(ctx, id, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.ID.ValueInt64() == 0 {
		// Resource was removed remotely.
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update implements resource.Resource.
func (r *lkeClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state lkeClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, d := plan.Timeouts.Update(ctx, updateLKETimeout)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{"cluster_id": state.ID.ValueInt64()})
	tflog.Debug(ctx, "Update linode_lke_cluster")

	client := r.Meta.Client
	id := int(state.ID.ValueInt64())

	updateOpts := linodego.LKEClusterUpdateOptions{}
	hasClusterUpdate := false

	if !plan.Label.Equal(state.Label) {
		updateOpts.Label = plan.Label.ValueString()
		hasClusterUpdate = true
	}
	if !plan.K8sVersion.Equal(state.K8sVersion) {
		updateOpts.K8sVersion = plan.K8sVersion.ValueString()
		hasClusterUpdate = true
	}
	if !plan.Tags.Equal(state.Tags) {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.Tags = &tags
		hasClusterUpdate = true
	}
	if !plan.ControlPlane.Equal(state.ControlPlane) {
		var cpModels []controlPlaneModel
		resp.Diagnostics.Append(plan.ControlPlane.ElementsAs(ctx, &cpModels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(cpModels) > 0 {
			expandedCP, d := expandControlPlaneOptionsFramework(ctx, cpModels[0])
			resp.Diagnostics.Append(d...)
			if resp.Diagnostics.HasError() {
				return
			}
			updateOpts.ControlPlane = &expandedCP
		}
		hasClusterUpdate = true
	}

	if hasClusterUpdate {
		tflog.Debug(ctx, "client.UpdateLKECluster(...)", map[string]any{"options": updateOpts})
		if _, err := client.UpdateLKECluster(ctx, id, updateOpts); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to update LKE Cluster %d", id), err.Error())
			return
		}
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	apiPools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to get Pools for LKE Cluster %d", id), err.Error())
		return
	}

	if !plan.K8sVersion.Equal(state.K8sVersion) {
		tflog.Debug(ctx, "Implicitly recycling LKE cluster to apply Kubernetes version upgrade")
		pollMS := int(r.Meta.Config.EventPollMilliseconds.ValueInt64())
		if err := recycleLKEClusterFramework(ctx, client, pollMS, id, apiPools); err != nil {
			resp.Diagnostics.AddError("Failed to recycle LKE cluster", err.Error())
			return
		}
	}

	// Reconcile node pools.
	var oldPoolModels, newPoolModels []poolModel
	resp.Diagnostics.Append(state.Pool.ElementsAs(ctx, &oldPoolModels, false)...)
	resp.Diagnostics.Append(plan.Pool.ElementsAs(ctx, &newPoolModels, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cluster, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to get LKE cluster %d", id), err.Error())
		return
	}

	oldSpecs := expandPoolSpecsFramework(oldPoolModels, false)
	newSpecs := expandPoolSpecsFramework(newPoolModels, true)

	updates, err := ReconcileLKENodePoolSpecs(ctx, oldSpecs, newSpecs, cluster.Tier == TierEnterprise)
	if err != nil {
		resp.Diagnostics.AddError("Failed to reconcile LKE cluster node pools", err.Error())
		return
	}

	updatedIDs := []int{}

	for poolID, poolUpdateOpts := range updates.ToUpdate {
		tflog.Debug(ctx, "client.UpdateLKENodePool(...)", map[string]any{"node_pool_id": poolID, "options": poolUpdateOpts})
		if _, err := client.UpdateLKENodePool(ctx, id, poolID, poolUpdateOpts); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to update LKE Cluster %d Pool %d", id, poolID), err.Error())
			return
		}
		updatedIDs = append(updatedIDs, poolID)
	}

	for _, createPoolOpts := range updates.ToCreate {
		tflog.Debug(ctx, "client.CreateLKENodePool(...)", map[string]any{"options": createPoolOpts})
		pool, err := client.CreateLKENodePool(ctx, id, createPoolOpts)
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to create LKE Cluster %d Pool", id), err.Error())
			return
		}
		updatedIDs = append(updatedIDs, pool.ID)
	}

	for _, poolID := range updates.ToDelete {
		tflog.Debug(ctx, "client.DeleteLKENodePool(...)", map[string]any{"node_pool_id": poolID})
		if err := client.DeleteLKENodePool(ctx, id, poolID); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to delete LKE Cluster %d Pool %d", id, poolID), err.Error())
			return
		}
	}

	tflog.Debug(ctx, "Waiting for all updated node pools to be ready")
	pollMS := int(r.Meta.Config.LKENodeReadyPollMilliseconds.ValueInt64())
	for _, poolID := range updatedIDs {
		tflog.Trace(ctx, "Waiting for node pool to be ready", map[string]any{"node_pool_id": poolID})
		if _, err := lkenodepool.WaitForNodePoolReady(ctx, *client, pollMS, id, poolID); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to wait for LKE Cluster %d pool %d ready", id, poolID), err.Error())
			return
		}
	}

	resp.Diagnostics.Append(r.readIntoState(ctx, id, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete implements resource.Resource.
func (r *lkeClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lkeClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, d := state.Timeouts.Delete(ctx, deleteLKETimeout)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{"cluster_id": state.ID.ValueInt64()})
	tflog.Debug(ctx, "Delete linode_lke_cluster")

	client := r.Meta.Client
	id := int(state.ID.ValueInt64())
	skipDeletePoll := r.Meta.Config.SkipLKEClusterDeletePoll.ValueBool()

	var oldNodes []linodego.LKENodePoolLinode
	if !skipDeletePoll {
		tflog.Trace(ctx, "client.ListLKENodePools(...)")
		apiPools, err := client.ListLKENodePools(ctx, id, nil)
		if err != nil {
			if linodego.IsNotFound(err) {
				tflog.Warn(ctx, "LKE cluster not found when listing node pools, assuming already deleted")
				return
			}
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to list node pools for LKE cluster %d", id), err.Error())
			return
		}
		for _, pool := range apiPools {
			oldNodes = append(oldNodes, pool.Linodes...)
		}
		tflog.Debug(ctx, "Collected Linode instances from LKE cluster node pools", map[string]any{"nodes": oldNodes})
	}

	tflog.Debug(ctx, "client.DeleteLKECluster(...)")
	if err := client.DeleteLKECluster(ctx, id); err != nil {
		if !linodego.IsNotFound(err) {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to delete Linode LKE cluster %d", id), err.Error())
			return
		}
	}

	timeoutSeconds, err := helper.SafeFloat64ToInt(deleteTimeout.Seconds())
	if err != nil {
		resp.Diagnostics.AddError("Failed to convert deletion timeout", err.Error())
		return
	}

	tflog.Debug(ctx, "Deleted LKE cluster, waiting for all nodes deleted...")
	tflog.Trace(ctx, "client.WaitForLKEClusterStatus(...)", map[string]any{"status": "not_ready", "timeout": timeoutSeconds})
	if _, err := client.WaitForLKEClusterStatus(ctx, id, "not_ready", timeoutSeconds); err != nil {
		if !linodego.IsNotFound(err) {
			resp.Diagnostics.AddError("Failed waiting for LKE cluster deletion", err.Error())
			return
		}
	}

	if !skipDeletePoll {
		pollMS := int(r.Meta.Config.EventPollMilliseconds.ValueInt64())
		if err := waitForNodesDeleted(ctx, *client, pollMS, oldNodes); err != nil {
			resp.Diagnostics.AddError("Failed waiting for Linode instances to be deleted", err.Error())
			return
		}
	}
}

// ImportState implements resource.ResourceWithImportState.
func (r *lkeClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("failed to parse LKE cluster ID: %s", err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// readIntoState fetches fresh API data and populates the given model in place.
func (r *lkeClusterResource) readIntoState(ctx context.Context, id int, state *lkeClusterResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	client := r.Meta.Client

	cluster, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			state.ID = types.Int64Value(0)
			return nil
		}
		diags.AddError(fmt.Sprintf("Failed to get LKE cluster %d", id), err.Error())
		return diags
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	apiPools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to get pools for LKE cluster %d", id), err.Error())
		return diags
	}

	// Filter external pools.
	if !state.ExternalPoolTags.IsNull() && !state.ExternalPoolTags.IsUnknown() {
		var externalTags []string
		diags.Append(state.ExternalPoolTags.ElementsAs(ctx, &externalTags, false)...)
		if diags.HasError() {
			return diags
		}
		if len(externalTags) > 0 {
			apiPools = filterExternalPools(ctx, externalTags, apiPools)
		}
	}

	kubeconfig, err := client.GetLKEClusterKubeconfig(ctx, id)
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to get kubeconfig for LKE cluster %d", id), err.Error())
		return diags
	}

	tflog.Trace(ctx, "client.ListLKEClusterAPIEndpoints(...)")
	endpoints, err := client.ListLKEClusterAPIEndpoints(ctx, id, nil)
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to get API endpoints for LKE cluster %d", id), err.Error())
		return diags
	}

	acl, err := client.GetLKEClusterControlPlaneACL(ctx, id)
	if err != nil {
		if lerr, ok := err.(*linodego.Error); ok &&
			(lerr.Code == 404 || (lerr.Code == 400 && strings.Contains(lerr.Message, "Cluster does not support Control Plane ACL"))) {
			acl = nil
		} else {
			diags.AddError(fmt.Sprintf("Failed to get control plane ACL for LKE cluster %d", id), err.Error())
			return diags
		}
	}

	// Scalar fields.
	state.ID = types.Int64Value(int64(cluster.ID))
	state.Label = types.StringValue(cluster.Label)
	state.K8sVersion = types.StringValue(cluster.K8sVersion)
	state.Region = types.StringValue(cluster.Region)
	state.Status = types.StringValue(string(cluster.Status))
	state.Tier = types.StringValue(cluster.Tier)
	state.APLEnabled = types.BoolValue(cluster.APLEnabled)
	state.SubnetID = types.Int64Value(int64(cluster.SubnetID))
	state.VpcID = types.Int64Value(int64(cluster.VpcID))
	state.StackType = types.StringValue(string(cluster.StackType))
	state.Kubeconfig = types.StringValue(kubeconfig.KubeConfig)

	if cluster.Tier == TierStandard {
		dashboard, err := client.GetLKEClusterDashboard(ctx, id)
		if err != nil {
			diags.AddError(fmt.Sprintf("Failed to get dashboard URL for LKE cluster %d", id), err.Error())
			return diags
		}
		state.DashboardURL = types.StringValue(dashboard.URL)
	} else {
		state.DashboardURL = types.StringNull()
	}

	tags, d := types.SetValueFrom(ctx, types.StringType, cluster.Tags)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.Tags = tags

	apiEndpointURLs := make([]string, len(endpoints))
	for i, ep := range endpoints {
		apiEndpointURLs[i] = ep.Endpoint
	}
	apiEndpoints, d := types.ListValueFrom(ctx, types.StringType, apiEndpointURLs)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.APIEndpoints = apiEndpoints

	cpVal, d := flattenControlPlaneFramework(ctx, cluster.ControlPlane, acl)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.ControlPlane = cpVal

	// Match pools to declared order.
	var declaredPoolModels []poolModel
	if !state.Pool.IsNull() && !state.Pool.IsUnknown() {
		diags.Append(state.Pool.ElementsAs(ctx, &declaredPoolModels, false)...)
		if diags.HasError() {
			return diags
		}
	}
	matchedPools, err := matchPoolsWithSchemaFramework(ctx, apiPools, declaredPoolModels)
	if err != nil {
		diags.AddError("Failed to match API pools with schema", err.Error())
		return diags
	}

	poolListVal, d := flattenPoolsFramework(ctx, matchedPools)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.Pool = poolListVal

	return diags
}

// matchPoolsWithSchemaFramework matches API pools to declared pools preserving order.
func matchPoolsWithSchemaFramework(_ context.Context, apiPools []linodego.LKENodePool, declaredPools []poolModel) ([]linodego.LKENodePool, error) {
	result := make([]linodego.LKENodePool, len(declaredPools))
	apiPoolMap := make(map[int]linodego.LKENodePool, len(apiPools))
	for _, pool := range apiPools {
		apiPoolMap[pool.ID] = pool
	}
	pairedDeclared := make(map[int]bool)

	// First pass: match by ID.
	for i, dp := range declaredPools {
		if dp.ID.IsNull() || dp.ID.IsUnknown() || dp.ID.ValueInt64() == 0 {
			continue
		}
		poolID := int(dp.ID.ValueInt64())
		apiPool, ok := apiPoolMap[poolID]
		if !ok {
			continue
		}
		result[i] = apiPool
		delete(apiPoolMap, poolID)
		pairedDeclared[i] = true
	}

	// Second pass: match by type + count.
	for i, dp := range declaredPools {
		if _, ok := pairedDeclared[i]; ok {
			continue
		}
		for _, apiPool := range apiPoolMap {
			if dp.Type.ValueString() != apiPool.Type {
				continue
			}
			if int(dp.Count.ValueInt64()) != apiPool.Count {
				continue
			}
			result[i] = apiPool
			delete(apiPoolMap, apiPool.ID)
			break
		}
	}

	// Append unresolved API pools to end.
	for _, pool := range apiPoolMap {
		result = append(result, pool)
	}

	return result, nil
}

// flattenPoolsFramework converts linodego pools to a framework types.List.
func flattenPoolsFramework(ctx context.Context, pools []linodego.LKENodePool) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	poolObjType := types.ObjectType{AttrTypes: poolAttrTypes}

	if len(pools) == 0 {
		return types.ListValueMust(poolObjType, []attr.Value{}), nil
	}

	poolVals := make([]attr.Value, len(pools))
	for i, pool := range pools {
		// Nodes.
		nodeVals := make([]attr.Value, len(pool.Linodes))
		for j, node := range pool.Linodes {
			nObj, d := types.ObjectValue(nodeAttrTypes, map[string]attr.Value{
				"id":          types.StringValue(node.ID),
				"instance_id": types.Int64Value(int64(node.InstanceID)),
				"status":      types.StringValue(string(node.Status)),
			})
			diags.Append(d...)
			nodeVals[j] = nObj
		}
		nodesListVal, d := types.ListValue(types.ObjectType{AttrTypes: nodeAttrTypes}, nodeVals)
		diags.Append(d...)

		// Autoscaler.
		autoscalerVals := []attr.Value{}
		if pool.Autoscaler.Enabled {
			aObj, d := types.ObjectValue(autoscalerAttrTypes, map[string]attr.Value{
				"min": types.Int64Value(int64(pool.Autoscaler.Min)),
				"max": types.Int64Value(int64(pool.Autoscaler.Max)),
			})
			diags.Append(d...)
			autoscalerVals = []attr.Value{aObj}
		}
		autoscalerListVal, d := types.ListValue(types.ObjectType{AttrTypes: autoscalerAttrTypes}, autoscalerVals)
		diags.Append(d...)

		// Taints.
		taintVals := make([]attr.Value, len(pool.Taints))
		for j, t := range pool.Taints {
			tObj, d := types.ObjectValue(taintAttrTypes, map[string]attr.Value{
				"effect": types.StringValue(string(t.Effect)),
				"key":    types.StringValue(t.Key),
				"value":  types.StringValue(t.Value),
			})
			diags.Append(d...)
			taintVals[j] = tObj
		}
		taintSetVal, d := types.SetValue(types.ObjectType{AttrTypes: taintAttrTypes}, taintVals)
		diags.Append(d...)

		tagsSetVal, d := types.SetValueFrom(ctx, types.StringType, pool.Tags)
		diags.Append(d...)

		labelsMapVal, d := types.MapValueFrom(ctx, types.StringType, map[string]string(pool.Labels))
		diags.Append(d...)

		firewallID := types.Int64Value(0)
		if pool.FirewallID != nil {
			firewallID = types.Int64Value(int64(*pool.FirewallID))
		}

		label := types.StringValue("")
		if pool.Label != nil {
			label = types.StringValue(*pool.Label)
		}

		k8sVersion := types.StringValue("")
		if pool.K8sVersion != nil {
			k8sVersion = types.StringValue(*pool.K8sVersion)
		}

		updateStrategy := types.StringValue("")
		if pool.UpdateStrategy != nil {
			updateStrategy = types.StringValue(string(*pool.UpdateStrategy))
		}

		pObj, d := types.ObjectValue(poolAttrTypes, map[string]attr.Value{
			"id":              types.Int64Value(int64(pool.ID)),
			"label":           label,
			"count":           types.Int64Value(int64(pool.Count)),
			"type":            types.StringValue(pool.Type),
			"firewall_id":     firewallID,
			"labels":          labelsMapVal,
			"taint":           taintSetVal,
			"tags":            tagsSetVal,
			"disk_encryption": types.StringValue(string(pool.DiskEncryption)),
			"nodes":           nodesListVal,
			"autoscaler":      autoscalerListVal,
			"k8s_version":     k8sVersion,
			"update_strategy": updateStrategy,
		})
		diags.Append(d...)
		poolVals[i] = pObj
	}

	if diags.HasError() {
		return types.ListNull(poolObjType), diags
	}
	listVal, d := types.ListValue(poolObjType, poolVals)
	diags.Append(d...)
	return listVal, diags
}

// flattenControlPlaneFramework converts control plane API data to a framework types.List.
func flattenControlPlaneFramework(
	ctx context.Context,
	controlPlane linodego.LKEClusterControlPlane,
	aclResp *linodego.LKEClusterControlPlaneACLResponse,
) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	cpObjType := types.ObjectType{AttrTypes: controlPlaneAttrTypes}

	// Build ACL.
	aclVals := []attr.Value{}
	if aclResp != nil {
		acl := aclResp.ACL

		addrVals := []attr.Value{}
		if acl.Addresses != nil {
			ipv4SetVal, d := types.SetValueFrom(ctx, types.StringType, acl.Addresses.IPv4)
			diags.Append(d...)
			ipv6SetVal, d := types.SetValueFrom(ctx, types.StringType, acl.Addresses.IPv6)
			diags.Append(d...)
			addrObj, d := types.ObjectValue(aclAddressesAttrTypes, map[string]attr.Value{
				"ipv4": ipv4SetVal,
				"ipv6": ipv6SetVal,
			})
			diags.Append(d...)
			addrVals = []attr.Value{addrObj}
		}
		addrListVal, d := types.ListValue(types.ObjectType{AttrTypes: aclAddressesAttrTypes}, addrVals)
		diags.Append(d...)

		aclObj, d := types.ObjectValue(aclAttrTypes, map[string]attr.Value{
			"enabled":   types.BoolValue(acl.Enabled),
			"addresses": addrListVal,
		})
		diags.Append(d...)
		aclVals = []attr.Value{aclObj}
	}
	aclListVal, d := types.ListValue(types.ObjectType{AttrTypes: aclAttrTypes}, aclVals)
	diags.Append(d...)

	cpObj, d := types.ObjectValue(controlPlaneAttrTypes, map[string]attr.Value{
		"high_availability":  types.BoolValue(controlPlane.HighAvailability),
		"audit_logs_enabled": types.BoolValue(controlPlane.AuditLogsEnabled),
		"acl":                aclListVal,
	})
	diags.Append(d...)

	if diags.HasError() {
		return types.ListNull(cpObjType), diags
	}
	listVal, d := types.ListValue(cpObjType, []attr.Value{cpObj})
	diags.Append(d...)
	return listVal, diags
}

// expandControlPlaneOptionsFramework expands a controlPlaneModel to linodego options.
func expandControlPlaneOptionsFramework(ctx context.Context, cp controlPlaneModel) (linodego.LKEClusterControlPlaneOptions, diag.Diagnostics) {
	var result linodego.LKEClusterControlPlaneOptions
	var diags diag.Diagnostics

	if !cp.HighAvailability.IsNull() && !cp.HighAvailability.IsUnknown() {
		v := cp.HighAvailability.ValueBool()
		result.HighAvailability = &v
	}
	if !cp.AuditLogsEnabled.IsNull() && !cp.AuditLogsEnabled.IsUnknown() {
		v := cp.AuditLogsEnabled.ValueBool()
		result.AuditLogsEnabled = &v
	}

	// Default to disabled ACL.
	enabled := false
	result.ACL = &linodego.LKEClusterControlPlaneACLOptions{Enabled: &enabled}

	if !cp.ACL.IsNull() && !cp.ACL.IsUnknown() {
		var aclModels []aclModel
		diags.Append(cp.ACL.ElementsAs(ctx, &aclModels, false)...)
		if diags.HasError() {
			return result, diags
		}
		if len(aclModels) > 0 {
			aclOpts, d := expandACLOptionsFramework(ctx, aclModels[0])
			diags.Append(d...)
			if diags.HasError() {
				return result, diags
			}
			result.ACL = aclOpts
		}
	}

	return result, diags
}

// expandACLOptionsFramework expands an aclModel to linodego ACL options.
func expandACLOptionsFramework(ctx context.Context, acl aclModel) (*linodego.LKEClusterControlPlaneACLOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result linodego.LKEClusterControlPlaneACLOptions

	if !acl.Enabled.IsNull() && !acl.Enabled.IsUnknown() {
		v := acl.Enabled.ValueBool()
		result.Enabled = &v
	}

	if !acl.Addresses.IsNull() && !acl.Addresses.IsUnknown() {
		var addrModels []aclAddressesModel
		diags.Append(acl.Addresses.ElementsAs(ctx, &addrModels, false)...)
		if diags.HasError() {
			return nil, diags
		}
		if len(addrModels) > 0 {
			addr := addrModels[0]
			var ipv4, ipv6 []string
			diags.Append(addr.IPv4.ElementsAs(ctx, &ipv4, false)...)
			diags.Append(addr.IPv6.ElementsAs(ctx, &ipv6, false)...)
			if diags.HasError() {
				return nil, diags
			}
			result.Addresses = &linodego.LKEClusterControlPlaneACLAddressesOptions{
				IPv4: &ipv4,
				IPv6: &ipv6,
			}
		}
	}

	if result.Enabled != nil && !*result.Enabled &&
		result.Addresses != nil &&
		((result.Addresses.IPv4 != nil && len(*result.Addresses.IPv4) > 0) ||
			(result.Addresses.IPv6 != nil && len(*result.Addresses.IPv6) > 0)) {
		diags.AddError("Invalid ACL configuration", "addresses are not acceptable when ACL is disabled")
		return nil, diags
	}

	return &result, diags
}

// expandPoolCreateOptions expands a poolModel to linodego pool create options.
func expandPoolCreateOptions(ctx context.Context, p poolModel) (linodego.LKENodePoolCreateOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	opts := linodego.LKENodePoolCreateOptions{
		Type: p.Type.ValueString(),
	}

	count := int(p.Count.ValueInt64())

	if !p.Autoscaler.IsNull() && !p.Autoscaler.IsUnknown() {
		var scalers []autoscalerModel
		diags.Append(p.Autoscaler.ElementsAs(ctx, &scalers, false)...)
		if diags.HasError() {
			return opts, diags
		}
		if len(scalers) > 0 {
			a := scalers[0]
			opts.Autoscaler = &linodego.LKENodePoolAutoscaler{
				Enabled: true,
				Min:     int(a.Min.ValueInt64()),
				Max:     int(a.Max.ValueInt64()),
			}
			if count == 0 {
				count = opts.Autoscaler.Min
			}
		}
	}
	opts.Count = count

	if !p.Label.IsNull() && !p.Label.IsUnknown() && p.Label.ValueString() != "" {
		opts.Label = linodego.Pointer(p.Label.ValueString())
	}
	if !p.FirewallID.IsNull() && !p.FirewallID.IsUnknown() && p.FirewallID.ValueInt64() != 0 {
		opts.FirewallID = linodego.Pointer(int(p.FirewallID.ValueInt64()))
	}
	if !p.Tags.IsNull() && !p.Tags.IsUnknown() {
		var tags []string
		diags.Append(p.Tags.ElementsAs(ctx, &tags, false)...)
		opts.Tags = tags
	}
	if !p.Taint.IsNull() && !p.Taint.IsUnknown() {
		var taintModels []taintModel
		diags.Append(p.Taint.ElementsAs(ctx, &taintModels, false)...)
		if diags.HasError() {
			return opts, diags
		}
		taints := make([]linodego.LKENodePoolTaint, len(taintModels))
		for i, t := range taintModels {
			taints[i] = linodego.LKENodePoolTaint{
				Key:    t.Key.ValueString(),
				Value:  t.Value.ValueString(),
				Effect: linodego.LKENodePoolTaintEffect(t.Effect.ValueString()),
			}
		}
		opts.Taints = taints
	}
	if !p.Labels.IsNull() && !p.Labels.IsUnknown() {
		var labels map[string]string
		diags.Append(p.Labels.ElementsAs(ctx, &labels, false)...)
		opts.Labels = linodego.LKENodePoolLabels(labels)
	}
	if !p.K8sVersion.IsNull() && !p.K8sVersion.IsUnknown() && p.K8sVersion.ValueString() != "" {
		opts.K8sVersion = linodego.Pointer(p.K8sVersion.ValueString())
	}
	if !p.UpdateStrategy.IsNull() && !p.UpdateStrategy.IsUnknown() && p.UpdateStrategy.ValueString() != "" {
		v := linodego.LKENodePoolUpdateStrategy(p.UpdateStrategy.ValueString())
		opts.UpdateStrategy = &v
	}

	return opts, diags
}

// expandPoolSpecsFramework converts []poolModel to []NodePoolSpec for reconciliation.
func expandPoolSpecsFramework(pools []poolModel, preserveNoTarget bool) []NodePoolSpec {
	var specs []NodePoolSpec
	for _, p := range pools {
		id := int(p.ID.ValueInt64())
		if !preserveNoTarget && id == 0 {
			continue
		}

		spec := NodePoolSpec{
			ID:    id,
			Type:  p.Type.ValueString(),
			Count: int(p.Count.ValueInt64()),
		}

		if !p.Label.IsNull() && !p.Label.IsUnknown() && p.Label.ValueString() != "" {
			spec.Label = linodego.Pointer(p.Label.ValueString())
		}
		if !p.FirewallID.IsNull() && !p.FirewallID.IsUnknown() && p.FirewallID.ValueInt64() != 0 {
			spec.FirewallID = linodego.Pointer(int(p.FirewallID.ValueInt64()))
		}
		if !p.K8sVersion.IsNull() && !p.K8sVersion.IsUnknown() && p.K8sVersion.ValueString() != "" {
			spec.K8sVersion = linodego.Pointer(p.K8sVersion.ValueString())
		}
		if !p.UpdateStrategy.IsNull() && !p.UpdateStrategy.IsUnknown() && p.UpdateStrategy.ValueString() != "" {
			spec.UpdateStrategy = linodego.Pointer(p.UpdateStrategy.ValueString())
		}

		specs = append(specs, spec)
	}
	return specs
}

// recycleLKEClusterFramework recycles an LKE cluster's nodes using framework types.
// This is the framework equivalent of the SDKv2 recycleLKECluster helper in cluster.go,
// taking individual client/pollMS args rather than a *helper.ProviderMeta.
func recycleLKEClusterFramework(
	ctx context.Context,
	client *linodego.Client,
	pollMS int,
	id int,
	pools []linodego.LKENodePool,
) error {
	ctx = helper.SetLogFieldBulk(ctx, map[string]any{
		"cluster_id": id,
		"pools":      pools,
	})

	tflog.Info(ctx, "Recycling LKE cluster")
	tflog.Trace(ctx, "client.RecycleLKEClusterNodes(...)")

	if err := client.RecycleLKEClusterNodes(ctx, id); err != nil {
		return fmt.Errorf("failed to recycle LKE Cluster (%d): %s", id, err)
	}

	oldNodes := make([]linodego.LKENodePoolLinode, 0)
	for _, pool := range pools {
		oldNodes = append(oldNodes, pool.Linodes...)
	}

	tflog.Debug(ctx, "Waiting for all nodes to be deleted", map[string]any{"nodes": oldNodes})

	if err := waitForNodesDeleted(ctx, *client, pollMS, oldNodes); err != nil {
		return fmt.Errorf("failed to wait for old nodes to be recycled: %w", err)
	}

	tflog.Debug(ctx, "All old nodes detected as deleted, waiting for all node pools to enter ready status")

	for _, pool := range pools {
		if _, err := lkenodepool.WaitForNodePoolReady(ctx, *client, pollMS, id, pool.ID); err != nil {
			return fmt.Errorf("failed to wait for pool %d ready: %w", pool.ID, err)
		}
	}

	tflog.Debug(ctx, "All node pools have entered ready status; recycle operation completed")
	return nil
}

// retryUntil retries fn until it succeeds or ctx/timeout expires.
func retryUntil(ctx context.Context, timeout time.Duration, fn func() error) error {
	ctx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		err := fn()
		if err == nil {
			return nil
		}
		select {
		case <-ctx2.Done():
			return err
		default:
		}
	}
}
