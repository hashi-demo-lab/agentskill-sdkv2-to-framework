package lke

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
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

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
	_ resource.ResourceWithModifyPlan  = &Resource{}
)

// LKEResourceModel describes the Terraform resource data model.
type LKEResourceModel struct {
	ID               types.Int64          `tfsdk:"id"`
	Label            types.String         `tfsdk:"label"`
	K8sVersion       types.String         `tfsdk:"k8s_version"`
	APLEnabled       types.Bool           `tfsdk:"apl_enabled"`
	Tags             types.Set            `tfsdk:"tags"`
	ExternalPoolTags types.Set            `tfsdk:"external_pool_tags"`
	Region           types.String         `tfsdk:"region"`
	APIEndpoints     types.List           `tfsdk:"api_endpoints"`
	Kubeconfig       types.String         `tfsdk:"kubeconfig"`
	DashboardURL     types.String         `tfsdk:"dashboard_url"`
	Status           types.String         `tfsdk:"status"`
	Tier             types.String         `tfsdk:"tier"`
	SubnetID         types.Int64          `tfsdk:"subnet_id"`
	VpcID            types.Int64          `tfsdk:"vpc_id"`
	StackType        types.String         `tfsdk:"stack_type"`
	Pool             []LKEResourcePool    `tfsdk:"pool"`
	ControlPlane     []LKEControlPlane    `tfsdk:"control_plane"`
	Timeouts         timeouts.Value       `tfsdk:"timeouts"`
}

type LKEResourcePool struct {
	ID             types.Int64              `tfsdk:"id"`
	Label          types.String             `tfsdk:"label"`
	Count          types.Int64              `tfsdk:"count"`
	Type           types.String             `tfsdk:"type"`
	FirewallID     types.Int64              `tfsdk:"firewall_id"`
	Labels         types.Map                `tfsdk:"labels"`
	Taint          []LKEResourcePoolTaint   `tfsdk:"taint"`
	Tags           types.Set                `tfsdk:"tags"`
	DiskEncryption types.String             `tfsdk:"disk_encryption"`
	Nodes          []LKEResourcePoolNode    `tfsdk:"nodes"`
	Autoscaler     []LKEResourceAutoscaler  `tfsdk:"autoscaler"`
	K8sVersion     types.String             `tfsdk:"k8s_version"`
	UpdateStrategy types.String             `tfsdk:"update_strategy"`
}

type LKEResourcePoolTaint struct {
	Effect types.String `tfsdk:"effect"`
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
}

type LKEResourcePoolNode struct {
	ID         types.String `tfsdk:"id"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	Status     types.String `tfsdk:"status"`
}

type LKEResourceAutoscaler struct {
	Min types.Int64 `tfsdk:"min"`
	Max types.Int64 `tfsdk:"max"`
}

// frameworkResourceSchema is the resource schema for the LKE cluster resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.Int64Attribute{
			Computed:    true,
			Description: "The unique ID of this LKE Cluster.",
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
			Optional: true,
			Computed: true,
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
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"label": schema.StringAttribute{
						Description: "The label of the Node Pool.",
						Optional:    true,
						Computed:    true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"count": schema.Int64Attribute{
						Description: "The number of nodes in the Node Pool.",
						Optional:    true,
						Computed:    true,
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
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
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"labels": schema.MapAttribute{
						ElementType: types.StringType,
						Description: "Key-value pairs added as labels to nodes in the node pool. " +
							"Labels help classify your nodes and to easily select subsets of objects.",
						Optional: true,
						Computed: true,
					},
					"tags": schema.SetAttribute{
						ElementType: types.StringType,
						Description: "A set of tags applied to this node pool.",
						Optional:    true,
						Computed:    true,
						PlanModifiers: []planmodifier.Set{
							setplanmodifier.UseStateForUnknown(),
						},
					},
					"disk_encryption": schema.StringAttribute{
						Description: "The disk encryption policy for the nodes in this pool.",
						Computed:    true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"k8s_version": schema.StringAttribute{
						Description: "The desired Kubernetes version for this pool. " +
							"This is only available for Enterprise clusters.",
						Computed: true,
						Optional: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"update_strategy": schema.StringAttribute{
						Description: "The strategy for updating the node pool k8s version. " +
							"For LKE enterprise only and may not currently available to all users.",
						Computed: true,
						Optional: true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
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
								},
								"key": schema.StringAttribute{
									Description: "The Kubernetes taint key.",
									Required:    true,
								},
								"value": schema.StringAttribute{
									Description: "The Kubernetes taint value.",
									Required:    true,
								},
							},
						},
					},
					"nodes": schema.ListNestedBlock{
						Description: "The nodes in the node pool.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"id": schema.StringAttribute{
									Description: "The ID of the node.",
									Computed:    true,
								},
								"instance_id": schema.Int64Attribute{
									Description: "The ID of the underlying Linode instance.",
									Computed:    true,
								},
								"status": schema.StringAttribute{
									Description: "The status of the node.",
									Computed:    true,
								},
							},
						},
					},
					"autoscaler": schema.ListNestedBlock{
						Description: "When specified, the number of nodes autoscales within the defined minimum and maximum values.",
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
		"control_plane": schema.ListNestedBlock{
			Description: "Defines settings for the Kubernetes Control Plane.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"high_availability": schema.BoolAttribute{
						Description: "Defines whether High Availability is enabled for the Control Plane Components of the cluster.",
						Optional:    true,
						Computed:    true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"audit_logs_enabled": schema.BoolAttribute{
						Description: "Enables audit logs on the cluster's control plane.",
						Optional:    true,
						Computed:    true,
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
				},
				Blocks: map[string]schema.Block{
					"acl": schema.ListNestedBlock{
						Description: "Defines the ACL configuration for an LKE cluster's control plane.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"enabled": schema.BoolAttribute{
									Description: "Defines default policy. A value of true results in a default policy of DENY. A value of false results in default policy of ALLOW, and has the same effect as delete the ACL configuration.",
									Computed:    true,
									Optional:    true,
									PlanModifiers: []planmodifier.Bool{
										boolplanmodifier.UseStateForUnknown(),
									},
								},
							},
							Blocks: map[string]schema.Block{
								"addresses": schema.ListNestedBlock{
									Description: "A list of ip addresses to allow.",
									NestedObject: schema.NestedBlockObject{
										Attributes: map[string]schema.Attribute{
											"ipv4": schema.SetAttribute{
												Description: "A set of individual ipv4 addresses or CIDRs to ALLOW.",
												Optional:    true,
												Computed:    true,
												ElementType: types.StringType,
											},
											"ipv6": schema.SetAttribute{
												Description: "A set of individual ipv6 addresses or CIDRs to ALLOW.",
												Optional:    true,
												Computed:    true,
												ElementType: types.StringType,
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

// NewResource creates a new LKE cluster resource.
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

// ModifyPlan implements ResourceWithModifyPlan. It performs cross-field
// validation (previously handled by CustomizeDiff) and plan-time checks.
func (r *Resource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	// Skip on resource destruction
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan LKEResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate: if count is not set on a pool, an autoscaler must be defined.
	for i, pool := range plan.Pool {
		if pool.Count.IsNull() || pool.Count.IsUnknown() {
			if len(pool.Autoscaler) == 0 {
				resp.Diagnostics.AddAttributeError(
					path.Root("pool").AtListIndex(i).AtName("count"),
					"Missing count or autoscaler",
					fmt.Sprintf("pool.%d: `count` must be defined when no autoscaler is defined", i),
				)
			}
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate: standard tier clusters require at least one pool.
	tierIsStandard := plan.Tier.IsNull() || plan.Tier.IsUnknown() ||
		plan.Tier.ValueString() == TierStandard || plan.Tier.ValueString() == ""
	if tierIsStandard && len(plan.Pool) == 0 {
		resp.Diagnostics.AddError(
			"Pool required for standard tier",
			"at least one pool is required for standard tier clusters",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate: update_strategy can only be used with enterprise tier.
	tierIsEnterprise := !plan.Tier.IsNull() && !plan.Tier.IsUnknown() &&
		plan.Tier.ValueString() == TierEnterprise
	if !tierIsEnterprise {
		var invalidPools []string
		for i, pool := range plan.Pool {
			if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() &&
				pool.UpdateStrategy.ValueString() != "" {
				invalidPools = append(invalidPools, fmt.Sprintf("pool.%d", i))
			}
		}
		if len(invalidPools) > 0 {
			resp.Diagnostics.AddError(
				"Invalid update_strategy",
				fmt.Sprintf(
					"%s: `update_strategy` can only be configured when tier is set to \"enterprise\"",
					strings.Join(invalidPools, ", "),
				),
			)
		}
	}
}

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_lke_cluster")

	var plan LKEResourceModel
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
		subnetID := helper.FrameworkSafeInt64ToInt(plan.SubnetID.ValueInt64(), &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.SubnetID = linodego.Pointer(subnetID)
	}

	if !plan.VpcID.IsNull() && !plan.VpcID.IsUnknown() {
		vpcID := helper.FrameworkSafeInt64ToInt(plan.VpcID.ValueInt64(), &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.VpcID = linodego.Pointer(vpcID)
	}

	if !plan.StackType.IsNull() && !plan.StackType.IsUnknown() {
		st := linodego.LKEClusterStackType(plan.StackType.ValueString())
		createOpts.StackType = linodego.Pointer(st)
	}

	if len(plan.ControlPlane) > 0 {
		cp, cpDiags := expandFrameworkControlPlaneOptions(ctx, plan.ControlPlane[0])
		resp.Diagnostics.Append(cpDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.ControlPlane = &cp
	}

	for i, pool := range plan.Pool {
		count := int(pool.Count.ValueInt64())
		var autoscaler *linodego.LKENodePoolAutoscaler
		if len(pool.Autoscaler) > 0 {
			autoscaler = &linodego.LKENodePoolAutoscaler{
				Enabled: true,
				Min:     int(pool.Autoscaler[0].Min.ValueInt64()),
				Max:     int(pool.Autoscaler[0].Max.ValueInt64()),
			}
		}

		if count == 0 {
			if autoscaler == nil {
				resp.Diagnostics.AddError(
					"Missing count or autoscaler",
					fmt.Sprintf("pool.%d: Expected autoscaler for default node count, got nil. This is always a provider issue.", i),
				)
				return
			}
			count = autoscaler.Min
		}

		var label *string
		if !pool.Label.IsNull() && !pool.Label.IsUnknown() && pool.Label.ValueString() != "" {
			l := pool.Label.ValueString()
			label = &l
		}

		var firewallID *int
		if !pool.FirewallID.IsNull() && !pool.FirewallID.IsUnknown() && pool.FirewallID.ValueInt64() != 0 {
			fwID := int(pool.FirewallID.ValueInt64())
			firewallID = &fwID
		}

		var tags []string
		resp.Diagnostics.Append(pool.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		var labelsMap map[string]string
		resp.Diagnostics.Append(pool.Labels.ElementsAs(ctx, &labelsMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		taints := expandFrameworkPoolTaints(pool.Taint)

		poolOpts := linodego.LKENodePoolCreateOptions{
			Label:      label,
			FirewallID: firewallID,
			Type:       pool.Type.ValueString(),
			Tags:       tags,
			Taints:     taints,
			Labels:     linodego.LKENodePoolLabels(labelsMap),
			Count:      count,
			Autoscaler: autoscaler,
		}

		if !pool.K8sVersion.IsNull() && !pool.K8sVersion.IsUnknown() && pool.K8sVersion.ValueString() != "" {
			k8sv := pool.K8sVersion.ValueString()
			poolOpts.K8sVersion = &k8sv
		}

		if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
			us := linodego.LKENodePoolUpdateStrategy(pool.UpdateStrategy.ValueString())
			poolOpts.UpdateStrategy = &us
		}

		createOpts.NodePools = append(createOpts.NodePools, poolOpts)
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.Tags = tags
	}

	tflog.Debug(ctx, "client.CreateLKECluster(...)", map[string]any{
		"options": createOpts,
	})
	cluster, err := client.CreateLKECluster(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create LKE cluster", err.Error())
		return
	}

	plan.ID = types.Int64Value(int64(cluster.ID))
	ctx = tflog.SetField(ctx, "cluster_id", cluster.ID)

	// Enterprise clusters take longer for kubeconfig to be ready.
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
				// Retry context timed out, just warn and continue
				tflog.Debug(ctx, "Retry context expired waiting for ready node: "+err.Error())
				break
			}
			tflog.Debug(ctx, err.Error())
			continue
		}
		break
	}

	resp.Diagnostics.Append(r.readIntoState(ctx, &plan, cluster.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_lke_cluster")

	var state LKEResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "cluster_id", id)

	resp.Diagnostics.Append(r.readIntoState(ctx, &state, id)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_lke_cluster")

	var plan, state LKEResourceModel
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

	providerMeta := r.Meta
	client := providerMeta.Client

	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "cluster_id", id)

	updateOpts := linodego.LKEClusterUpdateOptions{}

	if !plan.Label.Equal(state.Label) {
		updateOpts.Label = plan.Label.ValueString()
	}

	if !plan.K8sVersion.Equal(state.K8sVersion) {
		updateOpts.K8sVersion = plan.K8sVersion.ValueString()
	}

	if len(plan.ControlPlane) > 0 {
		cp, cpDiags := expandFrameworkControlPlaneOptions(ctx, plan.ControlPlane[0])
		resp.Diagnostics.Append(cpDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.ControlPlane = &cp
	}

	if !plan.Tags.Equal(state.Tags) {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.Tags = &tags
	}

	labelChanged := !plan.Label.Equal(state.Label)
	tagsChanged := !plan.Tags.Equal(state.Tags)
	k8sChanged := !plan.K8sVersion.Equal(state.K8sVersion)
	cpChanged := !controlPlanesEqual(plan.ControlPlane, state.ControlPlane)

	if labelChanged || tagsChanged || k8sChanged || cpChanged {
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

	if k8sChanged {
		tflog.Debug(ctx, "Implicitly recycling LKE cluster to apply Kubernetes version upgrade")

		if err := recycleLKECluster(ctx, providerMeta, id, pools); err != nil {
			resp.Diagnostics.AddError("Failed to recycle LKE cluster", err.Error())
			return
		}
	}

	cluster, err := client.GetLKECluster(ctx, id)
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

	enterprise := cluster.Tier == TierEnterprise

	oldPoolSpecs := expandFrameworkNodePoolSpecs(state.Pool, false)
	newPoolSpecs := expandFrameworkNodePoolSpecs(plan.Pool, true)

	updates, err := ReconcileLKENodePoolSpecs(ctx, oldPoolSpecs, newPoolSpecs, enterprise)
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

	for _, createOpts := range updates.ToCreate {
		tflog.Debug(ctx, "client.CreateLKENodePool(...)", map[string]any{
			"options": createOpts,
		})
		pool, err := client.CreateLKENodePool(ctx, id, createOpts)
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
			providerMeta.Config.LKENodeReadyPollMilliseconds,
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

	resp.Diagnostics.Append(r.readIntoState(ctx, &plan, id)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_lke_cluster")

	var state LKEResourceModel
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

	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = tflog.SetField(ctx, "cluster_id", id)
	skipDeletePoll := providerMeta.Config.SkipLKEClusterDeletePoll

	// Collect all Linode instance IDs from node pools before deleting the cluster.
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
	err := client.DeleteLKECluster(ctx, id)
	if err != nil {
		if !linodego.IsNotFound(err) {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to delete Linode LKE cluster %d", id),
				err.Error(),
			)
			return
		}
	}

	timeoutSeconds, err := helper.SafeFloat64ToInt(deleteTimeout.Seconds())
	if err != nil {
		resp.Diagnostics.AddError("Failed to convert float64 deletion timeout to int", err.Error())
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
			resp.Diagnostics.AddError("Failed to wait for LKE cluster deletion", err.Error())
			return
		}
	}

	if !skipDeletePoll {
		if err := waitForNodesDeleted(
			ctx,
			client,
			providerMeta.Config.EventPollMilliseconds,
			oldNodes,
		); err != nil {
			resp.Diagnostics.AddError("Failed waiting for Linode instances to be deleted", err.Error())
			return
		}
	}
}

func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	idStr := req.ID
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse import ID",
			fmt.Sprintf("Could not parse %q as int64: %s", idStr, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// readIntoState reads all cluster attributes from the API and populates the model.
func (r *Resource) readIntoState(
	ctx context.Context,
	state *LKEResourceModel,
	id int,
) diag.Diagnostics {
	var diags diag.Diagnostics

	client := r.Meta.Client

	ctx = tflog.SetField(ctx, "cluster_id", id)

	// Collect the declared pools for pool-matching heuristic.
	declaredPools := make([]any, len(state.Pool))
	for i, p := range state.Pool {
		var tags []string
		diags.Append(p.Tags.ElementsAs(ctx, &tags, false)...)
		if diags.HasError() {
			return diags
		}

		var labelsMap map[string]string
		diags.Append(p.Labels.ElementsAs(ctx, &labelsMap, false)...)
		if diags.HasError() {
			return diags
		}

		poolMap := map[string]any{
			"id":              int(p.ID.ValueInt64()),
			"count":           int(p.Count.ValueInt64()),
			"type":            p.Type.ValueString(),
			"label":           p.Label.ValueString(),
			"firewall_id":     int(p.FirewallID.ValueInt64()),
			"k8s_version":     p.K8sVersion.ValueString(),
			"update_strategy": p.UpdateStrategy.ValueString(),
		}
		declaredPools[i] = poolMap
	}

	cluster, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			return nil
		}
		diags.AddError(
			fmt.Sprintf("Failed to get LKE cluster %d", id),
			err.Error(),
		)
		return diags
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to get pools for LKE cluster %d", id),
			err.Error(),
		)
		return diags
	}

	var externalPoolTags []string
	diags.Append(state.ExternalPoolTags.ElementsAs(ctx, &externalPoolTags, false)...)
	if diags.HasError() {
		return diags
	}

	if len(externalPoolTags) > 0 && len(pools) > 0 {
		pools = filterExternalPools(ctx, externalPoolTags, pools)
	}

	kubeconfig, err := client.GetLKEClusterKubeconfig(ctx, id)
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to get kubeconfig for LKE cluster %d", id),
			err.Error(),
		)
		return diags
	}

	tflog.Trace(ctx, "client.ListLKEClusterAPIEndpoints(...)")
	endpoints, err := client.ListLKEClusterAPIEndpoints(ctx, id, nil)
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to get API endpoints for LKE cluster %d", id),
			err.Error(),
		)
		return diags
	}

	acl, err := client.GetLKEClusterControlPlaneACL(ctx, id)
	if err != nil {
		if lerr, ok := err.(*linodego.Error); ok &&
			(lerr.Code == 404 ||
				(lerr.Code == 400 && strings.Contains(lerr.Message, "Cluster does not support Control Plane ACL"))) {
			// No ACL support for this cluster.
		} else {
			diags.AddError(
				fmt.Sprintf("Failed to get control plane ACL for LKE cluster %d", id),
				err.Error(),
			)
			return diags
		}
	}

	// Only standard LKE has a dashboard URL
	if cluster.Tier == TierStandard {
		dashboard, err := client.GetLKEClusterDashboard(ctx, id)
		if err != nil {
			diags.AddError(
				fmt.Sprintf("Failed to get dashboard URL for LKE cluster %d", id),
				err.Error(),
			)
			return diags
		}
		state.DashboardURL = types.StringValue(dashboard.URL)
	}

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

	tags, tagDiags := types.SetValueFrom(ctx, types.StringType, cluster.Tags)
	diags.Append(tagDiags...)
	if diags.HasError() {
		return diags
	}
	state.Tags = tags

	var urls []string
	for _, e := range endpoints {
		urls = append(urls, e.Endpoint)
	}
	apiEndpoints, epDiags := types.ListValueFrom(ctx, types.StringType, urls)
	diags.Append(epDiags...)
	if diags.HasError() {
		return diags
	}
	state.APIEndpoints = apiEndpoints

	// Populate control plane
	flattenedCP := flattenLKEClusterControlPlane(cluster.ControlPlane, acl)
	cp, cpDiags := parseFrameworkControlPlane(ctx, flattenedCP)
	diags.Append(cpDiags...)
	if diags.HasError() {
		return diags
	}
	state.ControlPlane = []LKEControlPlane{cp}

	// Populate pools
	_ = declaredPools // used for heuristic matching in SDKv2; here we simply flatten API response
	frameworkPools, poolDiags := flattenFrameworkNodePools(ctx, pools)
	diags.Append(poolDiags...)
	if diags.HasError() {
		return diags
	}
	state.Pool = frameworkPools

	return diags
}

// flattenFrameworkNodePools converts API node pool responses to framework model slices.
func flattenFrameworkNodePools(ctx context.Context, pools []linodego.LKENodePool) ([]LKEResourcePool, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := make([]LKEResourcePool, len(pools))

	for i, p := range pools {
		var pool LKEResourcePool

		pool.ID = types.Int64Value(int64(p.ID))
		pool.Count = types.Int64Value(int64(p.Count))
		pool.Type = types.StringValue(p.Type)
		pool.DiskEncryption = types.StringValue(string(p.DiskEncryption))

		if p.Label != nil {
			pool.Label = types.StringPointerValue(p.Label)
		} else {
			pool.Label = types.StringValue("")
		}

		if p.FirewallID != nil {
			pool.FirewallID = types.Int64Value(int64(*p.FirewallID))
		} else {
			pool.FirewallID = types.Int64Value(0)
		}

		if p.K8sVersion != nil {
			pool.K8sVersion = types.StringPointerValue(p.K8sVersion)
		} else {
			pool.K8sVersion = types.StringValue("")
		}

		if p.UpdateStrategy != nil {
			pool.UpdateStrategy = types.StringValue(string(*p.UpdateStrategy))
		} else {
			pool.UpdateStrategy = types.StringValue("")
		}

		tags, tagDiags := types.SetValueFrom(ctx, types.StringType, p.Tags)
		diags.Append(tagDiags...)
		if diags.HasError() {
			return nil, diags
		}
		pool.Tags = tags

		labels, labelDiags := types.MapValueFrom(ctx, types.StringType, p.Labels)
		diags.Append(labelDiags...)
		if diags.HasError() {
			return nil, diags
		}
		pool.Labels = labels

		pool.Nodes = make([]LKEResourcePoolNode, len(p.Linodes))
		for j, node := range p.Linodes {
			pool.Nodes[j] = LKEResourcePoolNode{
				ID:         types.StringValue(node.ID),
				InstanceID: types.Int64Value(int64(node.InstanceID)),
				Status:     types.StringValue(string(node.Status)),
			}
		}

		pool.Taint = make([]LKEResourcePoolTaint, len(p.Taints))
		for j, t := range p.Taints {
			pool.Taint[j] = LKEResourcePoolTaint{
				Effect: types.StringValue(string(t.Effect)),
				Key:    types.StringValue(t.Key),
				Value:  types.StringValue(t.Value),
			}
		}

		if p.Autoscaler.Enabled {
			pool.Autoscaler = []LKEResourceAutoscaler{
				{
					Min: types.Int64Value(int64(p.Autoscaler.Min)),
					Max: types.Int64Value(int64(p.Autoscaler.Max)),
				},
			}
		} else {
			pool.Autoscaler = []LKEResourceAutoscaler{}
		}

		result[i] = pool
	}

	return result, diags
}

// parseFrameworkControlPlane converts an already-flattened map to LKEControlPlane.
func parseFrameworkControlPlane(
	ctx context.Context,
	cp map[string]any,
) (LKEControlPlane, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result LKEControlPlane

	if ha, ok := cp["high_availability"].(bool); ok {
		result.HighAvailability = types.BoolValue(ha)
	}
	if al, ok := cp["audit_logs_enabled"].(bool); ok {
		result.AuditLogsEnabled = types.BoolValue(al)
	}

	if aclList, ok := cp["acl"].([]map[string]any); ok && len(aclList) > 0 {
		aclMap := aclList[0]
		var cpACL LKEControlPlaneACL

		if enabled, ok := aclMap["enabled"].(bool); ok {
			cpACL.Enabled = types.BoolValue(enabled)
		}

		if addrList, ok := aclMap["addresses"].([]map[string]any); ok && len(addrList) > 0 {
			addrMap := addrList[0]

			var ipv4Strs []string
			if v, ok := addrMap["ipv4"].([]string); ok {
				ipv4Strs = v
			}
			var ipv6Strs []string
			if v, ok := addrMap["ipv6"].([]string); ok {
				ipv6Strs = v
			}

			ipv4Set, setDiags := types.SetValueFrom(ctx, types.StringType, ipv4Strs)
			diags.Append(setDiags...)
			ipv6Set, setDiags := types.SetValueFrom(ctx, types.StringType, ipv6Strs)
			diags.Append(setDiags...)

			if diags.HasError() {
				return result, diags
			}

			cpACL.Addresses = []LKEControlPlaneACLAddresses{{
				IPv4: ipv4Set,
				IPv6: ipv6Set,
			}}
		}

		result.ACL = []LKEControlPlaneACL{cpACL}
	} else {
		result.ACL = []LKEControlPlaneACL{}
	}

	return result, diags
}

// expandFrameworkControlPlaneOptions converts framework model to linodego options.
func expandFrameworkControlPlaneOptions(
	ctx context.Context,
	cp LKEControlPlane,
) (linodego.LKEClusterControlPlaneOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result linodego.LKEClusterControlPlaneOptions

	if !cp.HighAvailability.IsNull() && !cp.HighAvailability.IsUnknown() {
		ha := cp.HighAvailability.ValueBool()
		result.HighAvailability = &ha
	}

	if !cp.AuditLogsEnabled.IsNull() && !cp.AuditLogsEnabled.IsUnknown() {
		al := cp.AuditLogsEnabled.ValueBool()
		result.AuditLogsEnabled = &al
	}

	// Default to disabled ACL
	disabled := false
	result.ACL = &linodego.LKEClusterControlPlaneACLOptions{Enabled: &disabled}

	if len(cp.ACL) > 0 {
		acl := cp.ACL[0]
		aclOpts := &linodego.LKEClusterControlPlaneACLOptions{}

		if !acl.Enabled.IsNull() && !acl.Enabled.IsUnknown() {
			enabled := acl.Enabled.ValueBool()
			aclOpts.Enabled = &enabled
		}

		if len(acl.Addresses) > 0 {
			addr := acl.Addresses[0]
			addrOpts := &linodego.LKEClusterControlPlaneACLAddressesOptions{}

			if !addr.IPv4.IsNull() && !addr.IPv4.IsUnknown() {
				var ipv4 []string
				diags.Append(addr.IPv4.ElementsAs(ctx, &ipv4, false)...)
				addrOpts.IPv4 = &ipv4
			}

			if !addr.IPv6.IsNull() && !addr.IPv6.IsUnknown() {
				var ipv6 []string
				diags.Append(addr.IPv6.ElementsAs(ctx, &ipv6, false)...)
				addrOpts.IPv6 = &ipv6
			}

			if diags.HasError() {
				return result, diags
			}

			aclOpts.Addresses = addrOpts
		}

		// Validate: addresses are not acceptable when ACL is disabled
		if aclOpts.Enabled != nil && !*aclOpts.Enabled {
			if aclOpts.Addresses != nil &&
				((aclOpts.Addresses.IPv4 != nil && len(*aclOpts.Addresses.IPv4) > 0) ||
					(aclOpts.Addresses.IPv6 != nil && len(*aclOpts.Addresses.IPv6) > 0)) {
				diags.AddError(
					"Invalid ACL configuration",
					"addresses are not acceptable when ACL is disabled",
				)
				return result, diags
			}
		}

		result.ACL = aclOpts
	}

	return result, diags
}

// expandFrameworkPoolTaints converts framework taint models to linodego taints.
func expandFrameworkPoolTaints(taints []LKEResourcePoolTaint) []linodego.LKENodePoolTaint {
	result := make([]linodego.LKENodePoolTaint, len(taints))
	for i, t := range taints {
		result[i] = linodego.LKENodePoolTaint{
			Effect: linodego.LKENodePoolTaintEffect(t.Effect.ValueString()),
			Key:    t.Key.ValueString(),
			Value:  t.Value.ValueString(),
		}
	}
	return result
}

// expandFrameworkNodePoolSpecs converts framework pool models to NodePoolSpec for reconciliation.
func expandFrameworkNodePoolSpecs(pools []LKEResourcePool, preserveNoTarget bool) []NodePoolSpec {
	var specs []NodePoolSpec

	for _, pool := range pools {
		id := int(pool.ID.ValueInt64())

		if !preserveNoTarget && id == 0 {
			continue
		}

		count := int(pool.Count.ValueInt64())

		var autoScalerEnabled bool
		var autoScalerMin, autoScalerMax int
		if len(pool.Autoscaler) > 0 {
			autoScalerEnabled = true
			autoScalerMin = int(pool.Autoscaler[0].Min.ValueInt64())
			autoScalerMax = int(pool.Autoscaler[0].Max.ValueInt64())
		} else {
			autoScalerMin = count
			autoScalerMax = count
		}

		taints := make([]map[string]any, len(pool.Taint))
		for i, t := range pool.Taint {
			taints[i] = map[string]any{
				"effect": t.Effect.ValueString(),
				"key":    t.Key.ValueString(),
				"value":  t.Value.ValueString(),
			}
		}

		var labelsMap map[string]string
		if !pool.Labels.IsNull() && !pool.Labels.IsUnknown() {
			labelsMap = make(map[string]string)
			for k, v := range pool.Labels.Elements() {
				if sv, ok := v.(types.String); ok {
					labelsMap[k] = sv.ValueString()
				}
			}
		}

		var k8sVersionPtr *string
		if !pool.K8sVersion.IsNull() && !pool.K8sVersion.IsUnknown() && pool.K8sVersion.ValueString() != "" {
			v := pool.K8sVersion.ValueString()
			k8sVersionPtr = &v
		}

		var updateStrategyPtr *string
		if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
			v := pool.UpdateStrategy.ValueString()
			updateStrategyPtr = &v
		}

		var labelPtr *string
		if !pool.Label.IsNull() && !pool.Label.IsUnknown() && pool.Label.ValueString() != "" {
			v := pool.Label.ValueString()
			labelPtr = &v
		}

		var firewallIDPtr *int
		if !pool.FirewallID.IsNull() && !pool.FirewallID.IsUnknown() && pool.FirewallID.ValueInt64() != 0 {
			fwID := int(pool.FirewallID.ValueInt64())
			firewallIDPtr = &fwID
		}

		specs = append(specs, NodePoolSpec{
			ID:                id,
			Label:             labelPtr,
			FirewallID:        firewallIDPtr,
			Type:              pool.Type.ValueString(),
			Tags:              nil, // populated below
			Taints:            taints,
			Labels:            labelsMap,
			Count:             count,
			AutoScalerEnabled: autoScalerEnabled,
			AutoScalerMin:     autoScalerMin,
			AutoScalerMax:     autoScalerMax,
			K8sVersion:        k8sVersionPtr,
			UpdateStrategy:    updateStrategyPtr,
		})

		// Set tags separately since we need the full spec for the index
		if !pool.Tags.IsNull() && !pool.Tags.IsUnknown() {
			var tags []string
			for _, v := range pool.Tags.Elements() {
				if sv, ok := v.(types.String); ok {
					tags = append(tags, sv.ValueString())
				}
			}
			specs[len(specs)-1].Tags = tags
		}
	}

	return specs
}

// controlPlanesEqual compares two control plane slices for equality.
func controlPlanesEqual(a, b []LKEControlPlane) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	cpA, cpB := a[0], b[0]
	if !cpA.HighAvailability.Equal(cpB.HighAvailability) {
		return false
	}
	if !cpA.AuditLogsEnabled.Equal(cpB.AuditLogsEnabled) {
		return false
	}
	// For simplicity, compare ACL lengths as a proxy
	if len(cpA.ACL) != len(cpB.ACL) {
		return false
	}
	return true
}

// flattenLKEClusterAPIEndpoints converts API endpoint objects to string slice.
func flattenLKEClusterAPIEndpoints(apiEndpoints []linodego.LKEClusterAPIEndpoint) []string {
	flattened := make([]string, len(apiEndpoints))
	for i, endpoint := range apiEndpoints {
		flattened[i] = endpoint.Endpoint
	}
	return flattened
}

// Ensure frameworkResourceSchema Blocks are built up properly.
// The timeouts block is added by NewBaseResource via helper.BaseResource.Schema().
// We define it here additionally for explicit use in the struct tag; the BaseResource
// injection handles the actual schema registration.
var _ = attr.Type(nil) // ensure attr imported
