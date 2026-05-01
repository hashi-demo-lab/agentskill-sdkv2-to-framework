package lke

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	k8scondition "github.com/linode/linodego/k8s/pkg/condition"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
	"github.com/linode/terraform-provider-linode/v3/linode/lkenodepool"
)

const (
	createLKETimeout = 35 * time.Minute
	updateLKETimeout = 40 * time.Minute
	deleteLKETimeout = 20 * time.Minute
	TierEnterprise   = "enterprise"
	TierStandard     = "standard"
)

// Ensure the resource satisfies expected interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
	_ resource.ResourceWithModifyPlan  = &Resource{}
)

func NewResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_lke_cluster",
				IDType: types.Int64Type,
				Schema: &frameworkResourceSchema,
				TimeoutOpts: &timeouts.Opts{
					Create: true,
					Update: true,
					Delete: true,
				},
			},
		),
	}
}

type Resource struct {
	helper.BaseResource
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:    true,
			Description: "The unique ID of this LKE Cluster.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
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
				boolplanmodifier.UseStateForUnknown(),
			},
		},
		"tags": schema.SetAttribute{
			ElementType: types.StringType,
			Optional:    true,
			Computed:    true,
			Description: "An array of tags applied to this object. Tags are for organizational purposes only.",
			PlanModifiers: []planmodifier.Set{
				setplanmodifier.UseStateForUnknown(),
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
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
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
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"status": schema.StringAttribute{
			Computed:    true,
			Description: "The status of the cluster.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
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
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"vpc_id": schema.Int64Attribute{
			Optional:    true,
			Computed:    true,
			Description: "The ID of the VPC to use for the Kubernetes cluster.",
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
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
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
	},
	Blocks: map[string]schema.Block{
		"pool": schema.ListNestedBlock{
			Description: "A node pool in the cluster. At least one pool is required for standard tier clusters.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.Int64Attribute{
						Computed:    true,
						Description: "The ID of the Node Pool.",
					},
					"label": schema.StringAttribute{
						Description: "The label of the Node Pool.",
						Optional:    true,
						Computed:    true,
					},
					"count": schema.Int64Attribute{
						Description: "The number of nodes in the Node Pool.",
						Optional:    true,
						Computed:    true,
						Validators: []validator.Int64{
							int64validator.AtLeast(1),
						},
					},
					"type": schema.StringAttribute{
						Description: "A Linode Type for all of the nodes in the Node Pool.",
						Required:    true,
					},
					"firewall_id": schema.Int64Attribute{
						Description: "The ID of the Firewall to attach to nodes in this node pool.",
						Optional:    true,
						Computed:    true,
					},
					"labels": schema.MapAttribute{
						ElementType: types.StringType,
						Description: "Key-value pairs added as labels to nodes in the node pool. " +
							"Labels help classify your nodes and to easily select subsets of objects.",
						Optional: true,
						Computed: true,
					},
					"disk_encryption": schema.StringAttribute{
						Description: "The disk encryption policy for the nodes in this pool.",
						Computed:    true,
					},
					"tags": schema.SetAttribute{
						ElementType: types.StringType,
						Description: "A set of tags applied to this node pool.",
						Optional:    true,
						Computed:    true,
					},
					"k8s_version": schema.StringAttribute{
						Description: "The desired Kubernetes version for this pool. " +
							"This is only available for Enterprise clusters.",
						Computed: true,
						Optional: true,
					},
					"update_strategy": schema.StringAttribute{
						Description: "The strategy for updating the node pool k8s version. " +
							"For LKE enterprise only and may not currently available to all users.",
						Computed: true,
						Optional: true,
						Validators: []validator.String{
							stringvalidator.OneOf(
								string(linodego.LKENodePoolOnRecycle),
								string(linodego.LKENodePoolRollingUpdate),
							),
						},
					},
					"nodes": schema.ListAttribute{
						Computed:    true,
						Description: "The nodes in the node pool.",
						ElementType: types.ObjectType{
							AttrTypes: map[string]attr.Type{
								"id":          types.StringType,
								"instance_id": types.Int64Type,
								"status":      types.StringType,
							},
						},
					},
				},
				Blocks: map[string]schema.Block{
					"taint": schema.SetNestedBlock{
						Description: "Kubernetes taints to add to node pool nodes. Taints help control how " +
							"pods are scheduled onto nodes, specifically allowing them to repel certain pods.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"effect": schema.StringAttribute{
									Description: "The Kubernetes taint effect.",
									Required:    true,
									Validators: []validator.String{
										stringvalidator.OneOf(
											string(linodego.LKENodePoolTaintEffectNoExecute),
											string(linodego.LKENodePoolTaintEffectNoSchedule),
											string(linodego.LKENodePoolTaintEffectPreferNoSchedule),
										),
									},
								},
								"key": schema.StringAttribute{
									Description: "The Kubernetes taint key.",
									Required:    true,
									Validators: []validator.String{
										stringvalidator.LengthAtLeast(1),
									},
								},
								"value": schema.StringAttribute{
									Description: "The Kubernetes taint value.",
									Required:    true,
									Validators: []validator.String{
										stringvalidator.LengthAtLeast(1),
									},
								},
							},
						},
					},
					"autoscaler": schema.ListNestedBlock{
						Description: "When specified, the number of nodes autoscales within " +
							"the defined minimum and maximum values.",
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"min": schema.Int64Attribute{
									Description: "The minimum number of nodes to autoscale to.",
									Required:    true,
								},
								"max": schema.Int64Attribute{
									Description: "The maximum number of nodes to autoscale to.",
									Required:    true,
								},
							},
						},
					},
				},
			},
		},
		// control_plane is MaxItems:1 — kept as ListNestedBlock with SizeAtMost(1) to
		// preserve block syntax (control_plane { ... }) in practitioner configs.
		"control_plane": schema.ListNestedBlock{
			Description: "Defines settings for the Kubernetes Control Plane.",
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"high_availability": schema.BoolAttribute{
						Description: "Defines whether High Availability is enabled for the Control Plane Components of the cluster.",
						Optional:    true,
						Computed:    true,
					},
					"audit_logs_enabled": schema.BoolAttribute{
						Description: "Enables audit logs on the cluster's control plane.",
						Optional:    true,
						Computed:    true,
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
									Description: "Defines default policy. A value of true results in a default policy of DENY. A value of false results in default policy of ALLOW, and has the same effect as delete the ACL configuration.",
									Computed:    true,
									Optional:    true,
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
												Description: "A set of individual ipv4 addresses or CIDRs to ALLOW.",
												Optional:    true,
												Computed:    true,
											},
											"ipv6": schema.SetAttribute{
												ElementType: types.StringType,
												Description: "A set of individual ipv6 addresses or CIDRs to ALLOW.",
												Optional:    true,
												Computed:    true,
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
		"timeouts": timeouts.Block(context.Background(), timeouts.Opts{
			Create: true,
			Update: true,
			Delete: true,
		}),
	},
}

// ---------------------------------------------------------------------------
// Models
// ---------------------------------------------------------------------------

type TaintModel struct {
	Effect types.String `tfsdk:"effect"`
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
}

type AutoscalerModel struct {
	Min types.Int64 `tfsdk:"min"`
	Max types.Int64 `tfsdk:"max"`
}

type NodeModel struct {
	ID         types.String `tfsdk:"id"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	Status     types.String `tfsdk:"status"`
}

type PoolModel struct {
	ID             types.Int64  `tfsdk:"id"`
	Label          types.String `tfsdk:"label"`
	Count          types.Int64  `tfsdk:"count"`
	Type           types.String `tfsdk:"type"`
	FirewallID     types.Int64  `tfsdk:"firewall_id"`
	Labels         types.Map    `tfsdk:"labels"`
	DiskEncryption types.String `tfsdk:"disk_encryption"`
	Tags           types.Set    `tfsdk:"tags"`
	K8sVersion     types.String `tfsdk:"k8s_version"`
	UpdateStrategy types.String `tfsdk:"update_strategy"`
	Nodes          types.List   `tfsdk:"nodes"`
	Taint          []TaintModel `tfsdk:"taint"`
	Autoscaler     []AutoscalerModel `tfsdk:"autoscaler"`
}

type ACLAddressesModel struct {
	IPv4 types.Set `tfsdk:"ipv4"`
	IPv6 types.Set `tfsdk:"ipv6"`
}

type ACLModel struct {
	Enabled   types.Bool          `tfsdk:"enabled"`
	Addresses []ACLAddressesModel `tfsdk:"addresses"`
}

type ControlPlaneModel struct {
	HighAvailability types.Bool `tfsdk:"high_availability"`
	AuditLogsEnabled types.Bool `tfsdk:"audit_logs_enabled"`
	ACL              []ACLModel `tfsdk:"acl"`
}

type LKEClusterResourceModel struct {
	ID               types.String        `tfsdk:"id"`
	Label            types.String        `tfsdk:"label"`
	K8sVersion       types.String        `tfsdk:"k8s_version"`
	APLEnabled       types.Bool          `tfsdk:"apl_enabled"`
	Tags             types.Set           `tfsdk:"tags"`
	ExternalPoolTags types.Set           `tfsdk:"external_pool_tags"`
	Region           types.String        `tfsdk:"region"`
	APIEndpoints     types.List          `tfsdk:"api_endpoints"`
	Kubeconfig       types.String        `tfsdk:"kubeconfig"`
	DashboardURL     types.String        `tfsdk:"dashboard_url"`
	Status           types.String        `tfsdk:"status"`
	Tier             types.String        `tfsdk:"tier"`
	SubnetID         types.Int64         `tfsdk:"subnet_id"`
	VpcID            types.Int64         `tfsdk:"vpc_id"`
	StackType        types.String        `tfsdk:"stack_type"`
	Pool             []PoolModel         `tfsdk:"pool"`
	ControlPlane     []ControlPlaneModel `tfsdk:"control_plane"`
	Timeouts         timeouts.Value      `tfsdk:"timeouts"`
}

// ---------------------------------------------------------------------------
// ModifyPlan — replaces CustomizeDiff
// ---------------------------------------------------------------------------

func (r *Resource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	// Destroy case — nothing to validate.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan LKEClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// --- customDiffValidateOptionalCount ---
	// Ensure count is defined when no autoscaler is present.
	invalidPools := make([]string, 0)
	for i, pool := range plan.Pool {
		hasAutoscaler := len(pool.Autoscaler) > 0
		countNull := pool.Count.IsNull() || pool.Count.IsUnknown()
		if countNull && !hasAutoscaler {
			invalidPools = append(invalidPools, fmt.Sprintf("pool.%d", i))
		}
	}
	if len(invalidPools) > 0 {
		resp.Diagnostics.AddError(
			"Missing pool count",
			fmt.Sprintf(
				"%s: `count` must be defined when no autoscaler is defined",
				strings.Join(invalidPools, ", "),
			),
		)
		return
	}

	// --- customDiffValidatePoolForStandardTier ---
	// At least one pool is required for standard tier clusters.
	tier := plan.Tier.ValueString()
	tierIsStandard := plan.Tier.IsNull() || tier == TierStandard
	if tierIsStandard && len(plan.Pool) == 0 {
		resp.Diagnostics.AddError(
			"Missing node pool",
			"at least one pool is required for standard tier clusters",
		)
		return
	}

	// --- customDiffValidateUpdateStrategyWithTier ---
	// update_strategy may only be set for enterprise tier.
	tierIsEnterprise := !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && tier == TierEnterprise
	if !tierIsEnterprise {
		invalidStrategyPools := make([]string, 0)
		for i, pool := range plan.Pool {
			if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() &&
				pool.UpdateStrategy.ValueString() != "" {
				invalidStrategyPools = append(invalidStrategyPools, fmt.Sprintf("pool.%d", i))
			}
		}
		if len(invalidStrategyPools) > 0 {
			resp.Diagnostics.AddError(
				"Invalid update_strategy",
				fmt.Sprintf(
					"%s: `update_strategy` can only be configured when tier is set to \"enterprise\"",
					strings.Join(invalidStrategyPools, ", "),
				),
			)
			return
		}
	}

	// --- helper.SDKv2ValidateFieldRequiresAPIVersion ---
	// tier field requires v4beta API version.
	// The framework provider configures this via ProviderMeta; we surface it
	// as a plan-time error only when tier is explicitly configured.
	if !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && r.Meta != nil {
		if r.Meta.Config.APIVersion != helper.APIVersionV4Beta {
			resp.Diagnostics.AddAttributeError(
				path.Root("tier"),
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

// ---------------------------------------------------------------------------
// ImportState
// ---------------------------------------------------------------------------

func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_lke_cluster")

	var plan LKEClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createTimeout, diags := plan.Timeouts.Create(ctx, createLKETimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

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
		v := int(plan.SubnetID.ValueInt64())
		createOpts.SubnetID = linodego.Pointer(v)
	}

	if !plan.VpcID.IsNull() && !plan.VpcID.IsUnknown() {
		v := int(plan.VpcID.ValueInt64())
		createOpts.VpcID = linodego.Pointer(v)
	}

	if !plan.StackType.IsNull() && !plan.StackType.IsUnknown() {
		st := linodego.LKEClusterStackType(plan.StackType.ValueString())
		createOpts.StackType = linodego.Pointer(st)
	}

	if len(plan.ControlPlane) > 0 {
		expandedCP, expandDiags := expandFrameworkControlPlaneOptions(plan.ControlPlane[0])
		resp.Diagnostics.Append(expandDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.ControlPlane = &expandedCP
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.Tags = tags
	}

	for i, poolModel := range plan.Pool {
		autoscaler := expandFrameworkAutoscaler(poolModel)
		count := int(poolModel.Count.ValueInt64())
		if count == 0 {
			if autoscaler == nil {
				resp.Diagnostics.AddError(
					"Missing count",
					"Expected autoscaler for default node count, got nil. This is always a provider issue.",
				)
				return
			}
			count = autoscaler.Min
		}

		poolCreateOpts := linodego.LKENodePoolCreateOptions{
			Type:  poolModel.Type.ValueString(),
			Count: count,
		}

		if !poolModel.Label.IsNull() && !poolModel.Label.IsUnknown() && poolModel.Label.ValueString() != "" {
			poolCreateOpts.Label = linodego.Pointer(poolModel.Label.ValueString())
		}

		if !poolModel.FirewallID.IsNull() && !poolModel.FirewallID.IsUnknown() && poolModel.FirewallID.ValueInt64() != 0 {
			v := int(poolModel.FirewallID.ValueInt64())
			poolCreateOpts.FirewallID = linodego.Pointer(v)
		}

		if !poolModel.Tags.IsNull() && !poolModel.Tags.IsUnknown() {
			var tags []string
			resp.Diagnostics.Append(poolModel.Tags.ElementsAs(ctx, &tags, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			poolCreateOpts.Tags = tags
		}

		if len(poolModel.Taint) > 0 {
			poolCreateOpts.Taints = expandFrameworkTaints(poolModel.Taint)
		}

		if !poolModel.Labels.IsNull() && !poolModel.Labels.IsUnknown() {
			var labelsMap map[string]string
			resp.Diagnostics.Append(poolModel.Labels.ElementsAs(ctx, &labelsMap, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			poolCreateOpts.Labels = linodego.LKENodePoolLabels(labelsMap)
		}

		if !poolModel.K8sVersion.IsNull() && !poolModel.K8sVersion.IsUnknown() && poolModel.K8sVersion.ValueString() != "" {
			v := poolModel.K8sVersion.ValueString()
			poolCreateOpts.K8sVersion = &v
		}

		if !poolModel.UpdateStrategy.IsNull() && !poolModel.UpdateStrategy.IsUnknown() && poolModel.UpdateStrategy.ValueString() != "" {
			us := linodego.LKENodePoolUpdateStrategy(poolModel.UpdateStrategy.ValueString())
			poolCreateOpts.UpdateStrategy = &us
		}

		if autoscaler != nil {
			poolCreateOpts.Autoscaler = autoscaler
		}

		_ = i
		createOpts.NodePools = append(createOpts.NodePools, poolCreateOpts)
	}

	tflog.Debug(ctx, "client.CreateLKECluster(...)", map[string]any{
		"options": createOpts,
	})
	cluster, err := client.CreateLKECluster(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create LKE Cluster", err.Error())
		return
	}

	plan.ID = types.StringValue(strconv.Itoa(cluster.ID))

	// For enterprise clusters, wait for kubeconfig to be available.
	var retryContextTimeout time.Duration
	if cluster.Tier == TierEnterprise {
		retryContextTimeout = time.Second * 120
		if err := waitForLKEKubeConfig(ctx, client, r.Meta.Config.EventPollMilliseconds, cluster.ID); err != nil {
			resp.Diagnostics.AddError("Failed to get LKE cluster kubeconfig", err.Error())
			return
		}
	} else {
		retryContextTimeout = time.Second * 25
	}

	tflog.Debug(ctx, "Waiting for a single LKE cluster node to be ready")

	retryCtx, retryCancel := context.WithTimeout(ctx, retryContextTimeout)
	defer retryCancel()

	for {
		err := client.WaitForLKEClusterConditions(retryCtx, cluster.ID, linodego.LKEClusterPollOptions{
			TimeoutSeconds: 15 * 60,
		}, k8scondition.ClusterHasReadyNode)
		if err != nil {
			if retryCtx.Err() != nil {
				break
			}
			tflog.Debug(ctx, err.Error())
			continue
		}
		break
	}

	// Read and set state.
	resp.Diagnostics.Append(r.readAndSetState(ctx, cluster.ID, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (r *Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_lke_cluster")

	var state LKEClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error parsing Linode LKE Cluster ID", err.Error())
		return
	}

	resp.Diagnostics.Append(r.readAndSetState(ctx, id, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_lke_cluster")

	var plan, state LKEClusterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateTimeout, diags := plan.Timeouts.Update(ctx, updateLKETimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	client := r.Meta.Client

	id, err := strconv.Atoi(plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed parsing LKE Cluster ID", err.Error())
		return
	}

	updateOpts := linodego.LKEClusterUpdateOptions{}
	needsClusterUpdate := false

	if !plan.Label.Equal(state.Label) {
		updateOpts.Label = plan.Label.ValueString()
		needsClusterUpdate = true
	}

	if !plan.K8sVersion.Equal(state.K8sVersion) {
		updateOpts.K8sVersion = plan.K8sVersion.ValueString()
		needsClusterUpdate = true
	}

	if len(plan.ControlPlane) > 0 {
		expandedCP, expandDiags := expandFrameworkControlPlaneOptions(plan.ControlPlane[0])
		resp.Diagnostics.Append(expandDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.ControlPlane = &expandedCP
		needsClusterUpdate = true
	}

	if !plan.Tags.Equal(state.Tags) {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.Tags = &tags
		needsClusterUpdate = true
	}

	if needsClusterUpdate {
		tflog.Debug(ctx, "client.UpdateLKECluster(...)", map[string]any{
			"options": updateOpts,
		})
		if _, err := client.UpdateLKECluster(ctx, id, updateOpts); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to update LKE Cluster %d", id),
				err.Error(),
			)
			return
		}
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to get Pools for LKE Cluster %d", id),
			err.Error(),
		)
		return
	}

	if !plan.K8sVersion.Equal(state.K8sVersion) {
		tflog.Debug(ctx, "Implicitly recycling LKE cluster to apply Kubernetes version upgrade")
		if err := recycleLKECluster(ctx, r.Meta, id, pools); err != nil {
			resp.Diagnostics.AddError("Failed to recycle LKE Cluster", err.Error())
			return
		}
	}

	// Reconcile node pools.
	clusterObj, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed to get LKE cluster %d", id),
			err.Error(),
		)
		return
	}

	enterprise := clusterObj.Tier == TierEnterprise

	oldSpecs := expandFrameworkNodePoolSpecs(state.Pool, ctx, resp.Diagnostics, false)
	if resp.Diagnostics.HasError() {
		return
	}

	newSpecs := expandFrameworkNodePoolSpecs(plan.Pool, ctx, resp.Diagnostics, true)
	if resp.Diagnostics.HasError() {
		return
	}

	updates, err := ReconcileLKENodePoolSpecs(ctx, oldSpecs, newSpecs, enterprise)
	if err != nil {
		resp.Diagnostics.AddError("Failed to reconcile LKE cluster node pools", err.Error())
		return
	}

	tflog.Trace(ctx, "Reconciled LKE cluster node pool updates", map[string]any{
		"updates": updates,
	})

	updatedIds := []int{}

	for poolID, poolUpdateOpts := range updates.ToUpdate {
		tflog.Debug(ctx, "client.UpdateLKENodePool(...)", map[string]any{
			"node_pool_id": poolID,
			"options":      poolUpdateOpts,
		})
		if _, err := client.UpdateLKENodePool(ctx, id, poolID, poolUpdateOpts); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to update LKE Cluster %d Pool %d", id, poolID),
				err.Error(),
			)
			return
		}
		updatedIds = append(updatedIds, poolID)
	}

	for _, createPoolOpts := range updates.ToCreate {
		tflog.Debug(ctx, "client.CreateLKENodePool(...)", map[string]any{
			"options": createPoolOpts,
		})
		pool, err := client.CreateLKENodePool(ctx, id, createPoolOpts)
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to create LKE Cluster %d Pool", id),
				err.Error(),
			)
			return
		}
		updatedIds = append(updatedIds, pool.ID)
	}

	for _, poolID := range updates.ToDelete {
		tflog.Debug(ctx, "client.DeleteLKENodePool(...)", map[string]any{
			"node_pool_id": poolID,
		})
		if err := client.DeleteLKENodePool(ctx, id, poolID); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to delete LKE Cluster %d Pool %d", id, poolID),
				err.Error(),
			)
			return
		}
	}

	tflog.Debug(ctx, "Waiting for all updated node pools to be ready")
	for _, poolID := range updatedIds {
		tflog.Trace(ctx, "Waiting for node pool to be ready", map[string]any{
			"node_pool_id": poolID,
		})
		if _, err := lkenodepool.WaitForNodePoolReady(
			ctx,
			client,
			r.Meta.Config.LKENodeReadyPollMilliseconds,
			id,
			poolID,
		); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to wait for LKE Cluster %d pool %d ready", id, poolID),
				err.Error(),
			)
			return
		}
	}

	resp.Diagnostics.Append(r.readAndSetState(ctx, id, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func (r *Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_lke_cluster")

	var state LKEClusterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deleteTimeout, diags := state.Timeouts.Delete(ctx, deleteLKETimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()

	providerMeta := r.Meta
	client := providerMeta.Client

	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed parsing LKE Cluster ID", err.Error())
		return
	}

	skipDeletePoll := providerMeta.Config.SkipLKEClusterDeletePoll

	var oldNodes []linodego.LKENodePoolLinode
	if !skipDeletePoll {
		tflog.Trace(ctx, "client.ListLKENodePools(...)")
		pools, err := client.ListLKENodePools(ctx, id, nil)
		if err != nil {
			if linodego.IsNotFound(err) {
				tflog.Warn(ctx, "LKE cluster not found when listing node pools, assuming already deleted")
				return
			}
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to list node pools for LKE cluster %d", id),
				err.Error(),
			)
			return
		}
		for _, pool := range pools {
			oldNodes = append(oldNodes, pool.Linodes...)
		}
		tflog.Debug(ctx, "Collected Linode instances from LKE cluster node pools", map[string]any{
			"nodes": oldNodes,
		})
	}

	tflog.Debug(ctx, "client.DeleteLKECluster(...)")
	err = client.DeleteLKECluster(ctx, id)
	if err != nil {
		if !linodego.IsNotFound(err) {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to delete Linode LKE cluster %d", id),
				err.Error(),
			)
			return
		}
	}

	timeoutSeconds, convErr := helper.SafeFloat64ToInt(deleteTimeout.Seconds())
	if convErr != nil {
		resp.Diagnostics.AddError("Failed to convert deletion timeout", convErr.Error())
		return
	}

	tflog.Debug(ctx, "Deleted LKE cluster, waiting for all nodes deleted...")
	tflog.Trace(ctx, "client.WaitForLKEClusterStatus(...)", map[string]any{
		"status":  "not_ready",
		"timeout": timeoutSeconds,
	})

	_, err = client.WaitForLKEClusterStatus(ctx, id, "not_ready", timeoutSeconds)
	if err != nil {
		if !linodego.IsNotFound(err) {
			resp.Diagnostics.AddError("Error waiting for LKE cluster deletion", err.Error())
			return
		}
	}

	if !skipDeletePoll {
		if err := waitForNodesDeleted(ctx, client, providerMeta.Config.EventPollMilliseconds, oldNodes); err != nil {
			resp.Diagnostics.AddError(
				"Failed waiting for Linode instances to be deleted",
				err.Error(),
			)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// readAndSetState — shared read logic
// ---------------------------------------------------------------------------

func (r *Resource) readAndSetState(
	ctx context.Context,
	id int,
	model *LKEClusterResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	client := r.Meta.Client

	cluster, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			return diags
		}
		diags.AddError(fmt.Sprintf("Failed to get LKE cluster %d", id), err.Error())
		return diags
	}

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{
		"cluster_id": id,
	})

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to get pools for LKE cluster %d", id), err.Error())
		return diags
	}

	var externalPoolTags []string
	diags.Append(model.ExternalPoolTags.ElementsAs(ctx, &externalPoolTags, false)...)
	if diags.HasError() {
		return diags
	}

	if len(externalPoolTags) > 0 && len(pools) > 0 {
		pools = filterExternalPools(ctx, externalPoolTags, pools)
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
			(lerr.Code == 404 ||
				(lerr.Code == 400 && strings.Contains(lerr.Message, "Cluster does not support Control Plane ACL"))) {
			// The customer doesn't have access to LKE ACL or the cluster does not have a Gateway. Nothing to do here.
		} else {
			diags.AddError(fmt.Sprintf("Failed to get control plane ACL for LKE cluster %d", id), err.Error())
			return diags
		}
	}

	// Only standard LKE has a dashboard URL
	if cluster.Tier == TierStandard {
		dashboard, err := client.GetLKEClusterDashboard(ctx, id)
		if err != nil {
			diags.AddError(fmt.Sprintf("Failed to get dashboard URL for LKE cluster %d", id), err.Error())
			return diags
		}
		model.DashboardURL = types.StringValue(dashboard.URL)
	}

	model.ID = types.StringValue(strconv.Itoa(cluster.ID))
	model.Label = types.StringValue(cluster.Label)
	model.K8sVersion = types.StringValue(cluster.K8sVersion)
	model.Region = types.StringValue(cluster.Region)
	model.Status = types.StringValue(string(cluster.Status))
	model.Tier = types.StringValue(cluster.Tier)
	model.Kubeconfig = types.StringValue(kubeconfig.KubeConfig)
	model.APLEnabled = types.BoolValue(cluster.APLEnabled)

	if cluster.SubnetID != nil {
		model.SubnetID = types.Int64Value(int64(*cluster.SubnetID))
	} else {
		model.SubnetID = types.Int64Null()
	}

	if cluster.VpcID != nil {
		model.VpcID = types.Int64Value(int64(*cluster.VpcID))
	} else {
		model.VpcID = types.Int64Null()
	}

	if cluster.StackType != nil {
		model.StackType = types.StringValue(string(*cluster.StackType))
	} else {
		model.StackType = types.StringNull()
	}

	// Tags
	tagValues := make([]attr.Value, len(cluster.Tags))
	for i, t := range cluster.Tags {
		tagValues[i] = types.StringValue(t)
	}
	tagsSet, tagDiags := types.SetValue(types.StringType, tagValues)
	diags.Append(tagDiags...)
	model.Tags = tagsSet

	// API endpoints
	epValues := make([]attr.Value, len(endpoints))
	for i, ep := range endpoints {
		epValues[i] = types.StringValue(ep.Endpoint)
	}
	epList, epDiags := types.ListValue(types.StringType, epValues)
	diags.Append(epDiags...)
	model.APIEndpoints = epList

	// Node pools — match with declared pools in state for stable ordering.
	var declaredPools []any
	for _, pool := range model.Pool {
		poolID := int(pool.ID.ValueInt64())
		declaredPools = append(declaredPools, map[string]any{"id": poolID})
	}

	matchedPools, matchErr := matchPoolsWithSchemaFramework(pools, model.Pool)
	if matchErr != nil {
		diags.AddError("Failed to match api pools with schema", matchErr.Error())
		return diags
	}

	flatPools, flatDiags := flattenFrameworkNodePools(ctx, matchedPools)
	diags.Append(flatDiags...)
	if diags.HasError() {
		return diags
	}
	model.Pool = flatPools

	// Control plane
	cpModel := flattenFrameworkControlPlane(cluster.ControlPlane, acl)
	model.ControlPlane = []ControlPlaneModel{cpModel}

	return diags
}

// ---------------------------------------------------------------------------
// Framework expand/flatten helpers
// ---------------------------------------------------------------------------

func expandFrameworkAutoscaler(pool PoolModel) *linodego.LKENodePoolAutoscaler {
	if len(pool.Autoscaler) == 0 {
		return nil
	}
	return &linodego.LKENodePoolAutoscaler{
		Enabled: true,
		Min:     int(pool.Autoscaler[0].Min.ValueInt64()),
		Max:     int(pool.Autoscaler[0].Max.ValueInt64()),
	}
}

func expandFrameworkTaints(taints []TaintModel) []linodego.LKENodePoolTaint {
	result := make([]linodego.LKENodePoolTaint, len(taints))
	for i, t := range taints {
		result[i] = linodego.LKENodePoolTaint{
			Key:    t.Key.ValueString(),
			Value:  t.Value.ValueString(),
			Effect: linodego.LKENodePoolTaintEffect(t.Effect.ValueString()),
		}
	}
	return result
}

func expandFrameworkControlPlaneOptions(cp ControlPlaneModel) (linodego.LKEClusterControlPlaneOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result linodego.LKEClusterControlPlaneOptions

	if !cp.HighAvailability.IsNull() && !cp.HighAvailability.IsUnknown() {
		v := cp.HighAvailability.ValueBool()
		result.HighAvailability = &v
	}

	if !cp.AuditLogsEnabled.IsNull() && !cp.AuditLogsEnabled.IsUnknown() {
		v := cp.AuditLogsEnabled.ValueBool()
		result.AuditLogsEnabled = &v
	}

	// Default to disabled
	enabled := false
	result.ACL = &linodego.LKEClusterControlPlaneACLOptions{Enabled: &enabled}

	if len(cp.ACL) > 0 {
		aclOpts, aclDiags := expandFrameworkACLOptions(cp.ACL[0])
		diags.Append(aclDiags...)
		if !diags.HasError() {
			result.ACL = aclOpts
		}
	}

	return result, diags
}

func expandFrameworkACLOptions(acl ACLModel) (*linodego.LKEClusterControlPlaneACLOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result linodego.LKEClusterControlPlaneACLOptions

	if !acl.Enabled.IsNull() && !acl.Enabled.IsUnknown() {
		v := acl.Enabled.ValueBool()
		result.Enabled = &v
	}

	if len(acl.Addresses) > 0 {
		addr := acl.Addresses[0]
		var addrResult linodego.LKEClusterControlPlaneACLAddressesOptions

		if !addr.IPv4.IsNull() && !addr.IPv4.IsUnknown() {
			var ipv4 []string
			diags.Append(addr.IPv4.ElementsAs(context.Background(), &ipv4, false)...)
			addrResult.IPv4 = &ipv4
		}

		if !addr.IPv6.IsNull() && !addr.IPv6.IsUnknown() {
			var ipv6 []string
			diags.Append(addr.IPv6.ElementsAs(context.Background(), &ipv6, false)...)
			addrResult.IPv6 = &ipv6
		}

		result.Addresses = &addrResult
	}

	if result.Enabled != nil && !*result.Enabled {
		if result.Addresses != nil &&
			((result.Addresses.IPv4 != nil && len(*result.Addresses.IPv4) > 0) ||
				(result.Addresses.IPv6 != nil && len(*result.Addresses.IPv6) > 0)) {
			diags.AddError("Invalid ACL configuration", "addresses are not acceptable when ACL is disabled")
			return nil, diags
		}
	}

	return &result, diags
}

func flattenFrameworkControlPlane(
	controlPlane linodego.LKEClusterControlPlane,
	aclResp *linodego.LKEClusterControlPlaneACLResponse,
) ControlPlaneModel {
	model := ControlPlaneModel{
		HighAvailability: types.BoolValue(controlPlane.HighAvailability),
		AuditLogsEnabled: types.BoolValue(controlPlane.AuditLogsEnabled),
	}

	if aclResp != nil {
		acl := aclResp.ACL
		aclModel := ACLModel{
			Enabled: types.BoolValue(acl.Enabled),
		}

		if acl.Addresses != nil {
			addrModel := ACLAddressesModel{}

			if acl.Addresses.IPv4 != nil {
				ipv4Values := make([]attr.Value, len(acl.Addresses.IPv4))
				for i, ip := range acl.Addresses.IPv4 {
					ipv4Values[i] = types.StringValue(ip)
				}
				ipv4Set, _ := types.SetValue(types.StringType, ipv4Values)
				addrModel.IPv4 = ipv4Set
			} else {
				addrModel.IPv4 = types.SetNull(types.StringType)
			}

			if acl.Addresses.IPv6 != nil {
				ipv6Values := make([]attr.Value, len(acl.Addresses.IPv6))
				for i, ip := range acl.Addresses.IPv6 {
					ipv6Values[i] = types.StringValue(ip)
				}
				ipv6Set, _ := types.SetValue(types.StringType, ipv6Values)
				addrModel.IPv6 = ipv6Set
			} else {
				addrModel.IPv6 = types.SetNull(types.StringType)
			}

			aclModel.Addresses = []ACLAddressesModel{addrModel}
		}

		model.ACL = []ACLModel{aclModel}
	}

	return model
}

func flattenFrameworkNodePools(ctx context.Context, pools []linodego.LKENodePool) ([]PoolModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := make([]PoolModel, len(pools))

	nodeAttrTypes := map[string]attr.Type{
		"id":          types.StringType,
		"instance_id": types.Int64Type,
		"status":      types.StringType,
	}

	for i, pool := range pools {
		// Nodes
		nodeValues := make([]attr.Value, len(pool.Linodes))
		for j, node := range pool.Linodes {
			nodeObj, objDiags := types.ObjectValue(nodeAttrTypes, map[string]attr.Value{
				"id":          types.StringValue(node.ID),
				"instance_id": types.Int64Value(int64(node.InstanceID)),
				"status":      types.StringValue(string(node.Status)),
			})
			diags.Append(objDiags...)
			nodeValues[j] = nodeObj
		}
		nodesList, listDiags := types.ListValue(types.ObjectType{AttrTypes: nodeAttrTypes}, nodeValues)
		diags.Append(listDiags...)

		// Taints
		var taints []TaintModel
		for _, t := range pool.Taints {
			taints = append(taints, TaintModel{
				Effect: types.StringValue(string(t.Effect)),
				Key:    types.StringValue(t.Key),
				Value:  types.StringValue(t.Value),
			})
		}

		// Autoscaler
		var autoscaler []AutoscalerModel
		if pool.Autoscaler.Enabled {
			autoscaler = []AutoscalerModel{
				{
					Min: types.Int64Value(int64(pool.Autoscaler.Min)),
					Max: types.Int64Value(int64(pool.Autoscaler.Max)),
				},
			}
		}

		// Tags
		tagValues := make([]attr.Value, len(pool.Tags))
		for j, t := range pool.Tags {
			tagValues[j] = types.StringValue(t)
		}
		tagsSet, tagDiags := types.SetValue(types.StringType, tagValues)
		diags.Append(tagDiags...)

		// Labels
		labelValues := make(map[string]attr.Value, len(pool.Labels))
		for k, v := range pool.Labels {
			labelValues[k] = types.StringValue(v)
		}
		labelsMap, labelDiags := types.MapValue(types.StringType, labelValues)
		diags.Append(labelDiags...)

		pm := PoolModel{
			ID:             types.Int64Value(int64(pool.ID)),
			Count:          types.Int64Value(int64(pool.Count)),
			Type:           types.StringValue(pool.Type),
			DiskEncryption: types.StringValue(string(pool.DiskEncryption)),
			Tags:           tagsSet,
			Labels:         labelsMap,
			Nodes:          nodesList,
			Taint:          taints,
			Autoscaler:     autoscaler,
		}

		if pool.Label != nil {
			pm.Label = types.StringValue(*pool.Label)
		} else {
			pm.Label = types.StringValue("")
		}

		if pool.FirewallID != nil {
			pm.FirewallID = types.Int64Value(int64(*pool.FirewallID))
		} else {
			pm.FirewallID = types.Int64Value(0)
		}

		if pool.K8sVersion != nil {
			pm.K8sVersion = types.StringValue(*pool.K8sVersion)
		} else {
			pm.K8sVersion = types.StringNull()
		}

		if pool.UpdateStrategy != nil {
			pm.UpdateStrategy = types.StringValue(string(*pool.UpdateStrategy))
		} else {
			pm.UpdateStrategy = types.StringNull()
		}

		result[i] = pm
	}

	return result, diags
}

func expandFrameworkNodePoolSpecs(pools []PoolModel, ctx context.Context, diags diag.Diagnostics, preserveNoTarget bool) []NodePoolSpec {
	var specs []NodePoolSpec

	for _, pool := range pools {
		autoscaler := expandFrameworkAutoscaler(pool)
		if autoscaler == nil {
			count := int(pool.Count.ValueInt64())
			autoscaler = &linodego.LKENodePoolAutoscaler{
				Enabled: false,
				Min:     count,
				Max:     count,
			}
		}

		poolID := int(pool.ID.ValueInt64())
		if !preserveNoTarget && poolID == 0 {
			continue
		}

		var tags []string
		diags.Append(pool.Tags.ElementsAs(ctx, &tags, false)...)

		var labelsMap map[string]string
		diags.Append(pool.Labels.ElementsAs(ctx, &labelsMap, false)...)

		var taintsRaw []map[string]any
		for _, t := range pool.Taint {
			taintsRaw = append(taintsRaw, map[string]any{
				"effect": t.Effect.ValueString(),
				"key":    t.Key.ValueString(),
				"value":  t.Value.ValueString(),
			})
		}

		spec := NodePoolSpec{
			ID:                poolID,
			Type:              pool.Type.ValueString(),
			Tags:              tags,
			Taints:            taintsRaw,
			Labels:            labelsMap,
			Count:             int(pool.Count.ValueInt64()),
			AutoScalerEnabled: autoscaler.Enabled,
			AutoScalerMin:     autoscaler.Min,
			AutoScalerMax:     autoscaler.Max,
		}

		if !pool.Label.IsNull() && !pool.Label.IsUnknown() && pool.Label.ValueString() != "" {
			v := pool.Label.ValueString()
			spec.Label = &v
		}

		if !pool.FirewallID.IsNull() && !pool.FirewallID.IsUnknown() && pool.FirewallID.ValueInt64() != 0 {
			v := int(pool.FirewallID.ValueInt64())
			spec.FirewallID = &v
		}

		if !pool.K8sVersion.IsNull() && !pool.K8sVersion.IsUnknown() && pool.K8sVersion.ValueString() != "" {
			v := pool.K8sVersion.ValueString()
			spec.K8sVersion = &v
		}

		if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
			v := pool.UpdateStrategy.ValueString()
			spec.UpdateStrategy = &v
		}

		specs = append(specs, spec)
	}

	return specs
}

// matchPoolsWithSchemaFramework matches API pools with declared pools for stable ordering.
func matchPoolsWithSchemaFramework(pools []linodego.LKENodePool, declared []PoolModel) ([]linodego.LKENodePool, error) {
	result := make([]linodego.LKENodePool, len(declared))

	apiPools := make(map[int]linodego.LKENodePool, len(pools))
	for _, p := range pools {
		apiPools[p.ID] = p
	}

	pairedDeclared := make(map[int]bool)

	// First pass: match by ID
	for i, dp := range declared {
		poolID := int(dp.ID.ValueInt64())
		if poolID == 0 {
			continue
		}
		if ap, ok := apiPools[poolID]; ok {
			result[i] = ap
			delete(apiPools, poolID)
			pairedDeclared[i] = true
		}
	}

	// Second pass: match by attributes for new pools without IDs
	for i, dp := range declared {
		if pairedDeclared[i] {
			continue
		}
		for _, ap := range apiPools {
			if dp.Type.ValueString() != ap.Type {
				continue
			}
			result[i] = ap
			delete(apiPools, ap.ID)
			break
		}
	}

	// Append any unresolved API pools
	for _, ap := range apiPools {
		//nolint:makezero
		result = append(result, ap)
	}

	return result, nil
}
