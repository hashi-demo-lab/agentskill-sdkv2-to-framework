// Package lke provides the migrated terraform-plugin-framework resource for linode_lke_cluster.
// Migrated from terraform-plugin-sdk/v2 to terraform-plugin-framework.
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
	linodesetplanmodifiers "github.com/linode/terraform-provider-linode/v3/linode/helper/setplanmodifiers"
	"github.com/linode/terraform-provider-linode/v3/linode/lkenodepool"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                = &LKEResource{}
	_ resource.ResourceWithConfigure   = &LKEResource{}
	_ resource.ResourceWithImportState = &LKEResource{}
	_ resource.ResourceWithModifyPlan  = &LKEResource{}
)

// NewLKEResource returns a new framework resource for linode_lke_cluster.
func NewLKEResource() resource.Resource {
	return &LKEResource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_lke_cluster",
				IDType: types.Int64Type,
				Schema: &lkeResourceSchema,
				TimeoutOpts: &timeouts.Opts{
					Create: true,
					Update: true,
					Delete: true,
				},
			},
		),
	}
}

// LKEResource is the framework resource.
type LKEResource struct {
	helper.BaseResource
}

// ---------------------------------------------------------------------------
// Model structs
// ---------------------------------------------------------------------------

// LKEResourceModel is the top-level state/plan model.
type LKEResourceModel struct {
	ID               types.Int64               `tfsdk:"id"`
	Label            types.String              `tfsdk:"label"`
	K8sVersion       types.String              `tfsdk:"k8s_version"`
	APLEnabled       types.Bool                `tfsdk:"apl_enabled"`
	Tags             types.Set                 `tfsdk:"tags"`
	ExternalPoolTags types.Set                 `tfsdk:"external_pool_tags"`
	Region           types.String              `tfsdk:"region"`
	APIEndpoints     types.List                `tfsdk:"api_endpoints"`
	Kubeconfig       types.String              `tfsdk:"kubeconfig"`
	DashboardURL     types.String              `tfsdk:"dashboard_url"`
	Status           types.String              `tfsdk:"status"`
	Tier             types.String              `tfsdk:"tier"`
	SubnetID         types.Int64               `tfsdk:"subnet_id"`
	VpcID            types.Int64               `tfsdk:"vpc_id"`
	StackType        types.String              `tfsdk:"stack_type"`
	Pool             []LKEResourceNodePool     `tfsdk:"pool"`
	ControlPlane     []LKEResourceControlPlane `tfsdk:"control_plane"`
	Timeouts         timeouts.Value            `tfsdk:"timeouts"`
}

// LKEResourceControlPlane maps the control_plane block.
type LKEResourceControlPlane struct {
	HighAvailability types.Bool                    `tfsdk:"high_availability"`
	AuditLogsEnabled types.Bool                    `tfsdk:"audit_logs_enabled"`
	ACL              []LKEResourceControlPlaneACL  `tfsdk:"acl"`
}

// LKEResourceControlPlaneACL maps the acl block inside control_plane.
type LKEResourceControlPlaneACL struct {
	Enabled   types.Bool                             `tfsdk:"enabled"`
	Addresses []LKEResourceControlPlaneACLAddresses  `tfsdk:"addresses"`
}

// LKEResourceControlPlaneACLAddresses maps the addresses block inside acl.
type LKEResourceControlPlaneACLAddresses struct {
	IPv4 types.Set `tfsdk:"ipv4"`
	IPv6 types.Set `tfsdk:"ipv6"`
}

// LKEResourceNodePool maps each pool block.
type LKEResourceNodePool struct {
	ID             types.Int64                     `tfsdk:"id"`
	Label          types.String                    `tfsdk:"label"`
	Count          types.Int64                     `tfsdk:"count"`
	Type           types.String                    `tfsdk:"type"`
	FirewallID     types.Int64                     `tfsdk:"firewall_id"`
	Labels         types.Map                       `tfsdk:"labels"`
	Taint          []LKEResourceNodePoolTaint      `tfsdk:"taint"`
	Tags           types.Set                       `tfsdk:"tags"`
	DiskEncryption types.String                    `tfsdk:"disk_encryption"`
	Nodes          []LKEResourceNodePoolNode       `tfsdk:"nodes"`
	Autoscaler     []LKEResourceNodePoolAutoscaler `tfsdk:"autoscaler"`
	K8sVersion     types.String                    `tfsdk:"k8s_version"`
	UpdateStrategy types.String                    `tfsdk:"update_strategy"`
}

// LKEResourceNodePoolTaint maps a single taint entry.
type LKEResourceNodePoolTaint struct {
	Effect types.String `tfsdk:"effect"`
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
}

// LKEResourceNodePoolNode maps a single node entry (computed).
type LKEResourceNodePoolNode struct {
	ID         types.String `tfsdk:"id"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	Status     types.String `tfsdk:"status"`
}

// LKEResourceNodePoolAutoscaler maps the autoscaler block inside a pool.
type LKEResourceNodePoolAutoscaler struct {
	Min types.Int64 `tfsdk:"min"`
	Max types.Int64 `tfsdk:"max"`
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

// lkeResourceSchema is the framework schema for linode_lke_cluster.
// MaxItems:1 blocks (control_plane, acl, autoscaler) are kept as
// ListNestedBlock to preserve block-syntax backward compatibility with
// practitioner configs.
var lkeResourceSchema = schema.Schema{
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
				linodesetplanmodifiers.CaseInsensitiveSet(),
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
		// kubeconfig is Sensitive:true — value is stored in state but redacted from output/logs.
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
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
	},
	Blocks: map[string]schema.Block{
		// timeouts uses timeouts.Block to preserve the block-syntax that SDKv2
		// practitioners used: timeouts { create = "35m" update = "40m" delete = "20m" }
		"timeouts": timeouts.Block(context.Background(), timeouts.Opts{
			Create: true,
			Update: true,
			Delete: true,
		}),

		// pool — repeating block (no MaxItems in SDKv2); kept as ListNestedBlock.
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
						PlanModifiers: []planmodifier.Int64{
							int64planmodifier.UseStateForUnknown(),
						},
					},
					"labels": schema.MapAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Description: "Key-value pairs added as labels to nodes in the node pool. Labels help classify your nodes and to easily select subsets of objects.",
					},
					"disk_encryption": schema.StringAttribute{
						Computed:    true,
						Description: "The disk encryption policy for the nodes in this pool.",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"k8s_version": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The desired Kubernetes version for this pool. This is only available for Enterprise clusters.",
					},
					"update_strategy": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The strategy for updating the node pool k8s version. For LKE enterprise only and may not currently be available to all users.",
					},
				},
				Blocks: map[string]schema.Block{
					// taint — set-shaped; no MaxItems so stays as SetNestedBlock.
					"taint": schema.SetNestedBlock{
						Description: "Kubernetes taints to add to node pool nodes. Taints help control how pods are scheduled onto nodes, specifically allowing them to repel certain pods.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"effect": schema.StringAttribute{
									Required:    true,
									Description: "The Kubernetes taint effect.",
								},
								"key": schema.StringAttribute{
									Required:    true,
									Description: "The Kubernetes taint key.",
								},
								"value": schema.StringAttribute{
									Required:    true,
									Description: "The Kubernetes taint value.",
								},
							},
						},
					},
					// nodes — computed list of the actual Linode instances in the pool.
					"nodes": schema.ListNestedBlock{
						Description: "The nodes in the node pool.",
						NestedObject: schema.NestedBlockObject{
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
					// autoscaler — MaxItems:1 in SDKv2; kept as ListNestedBlock for HCL compat.
					"autoscaler": schema.ListNestedBlock{
						Description: "When specified, the number of nodes autoscales within the defined minimum and maximum values.",
						NestedObject: schema.NestedBlockObject{
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
				},
			},
		},

		// control_plane — MaxItems:1 in SDKv2; kept as ListNestedBlock for HCL compat.
		// Decision (Pre-flight C): Q1 → practitioners use block syntax in prod configs.
		// Keeping as block; note for follow-up: convert to SingleNestedBlock on next major.
		"control_plane": schema.ListNestedBlock{
			Description: "Defines settings for the Kubernetes Control Plane.",
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
					// acl — MaxItems:1; kept as ListNestedBlock for HCL compat.
					"acl": schema.ListNestedBlock{
						Description: "Defines the ACL configuration for an LKE cluster's control plane.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"enabled": schema.BoolAttribute{
									Optional:    true,
									Computed:    true,
									Description: "Defines default policy. A value of true results in a default policy of DENY. A value of false results in default policy of ALLOW.",
								},
							},
							Blocks: map[string]schema.Block{
								// addresses — MaxItems:1; kept as ListNestedBlock for HCL compat.
								"addresses": schema.ListNestedBlock{
									Description: "A list of ip addresses to allow.",
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
	},
}

// taintObjectAttrTypes is used by tests.
var taintObjectAttrTypes = map[string]attr.Type{
	"effect": types.StringType,
	"key":    types.StringType,
	"value":  types.StringType,
}

// ---------------------------------------------------------------------------
// ModifyPlan — replaces SDKv2 CustomizeDiff
// ---------------------------------------------------------------------------

// ModifyPlan implements resource.ResourceWithModifyPlan. It replaces the
// following SDKv2 CustomizeDiff functions:
//
//   - customDiffValidateOptionalCount
//   - customDiffValidatePoolForStandardTier
//   - customDiffValidateUpdateStrategyWithTier
//   - linodediffs.ComputedWithDefault("tags", []string{}) — handled by plan modifiers
//   - linodediffs.CaseInsensitiveSet("tags") — handled by CaseInsensitiveSet plan modifier
//   - helper.SDKv2ValidateFieldRequiresAPIVersion(v4beta, "tier")
func (r *LKEResource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	// Skip destroy plans (Plan is null on delete).
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan LKEResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 1. Validate that "tier" requires api_version = v4beta.
	//    Replaces: helper.SDKv2ValidateFieldRequiresAPIVersion(helper.APIVersionV4Beta, "tier")
	if r.Meta != nil && !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && plan.Tier.ValueString() != "" {
		apiVersion := r.Meta.Config.APIVersion.ValueString()
		if !strings.EqualFold(apiVersion, helper.APIVersionV4Beta) {
			resp.Diagnostics.AddAttributeError(
				path.Root("tier"),
				"Invalid API Version for Field",
				fmt.Sprintf(
					"The api_version provider argument must be set to '%s' to use the 'tier' field.",
					helper.APIVersionV4Beta,
				),
			)
		}
	}

	// 2. Validate that standard tier clusters have at least one pool.
	//    Replaces: customDiffValidatePoolForStandardTier
	tierIsStandard := plan.Tier.IsNull() || plan.Tier.IsUnknown() || plan.Tier.ValueString() == TierStandard
	if tierIsStandard && len(plan.Pool) == 0 {
		resp.Diagnostics.AddError(
			"Missing Node Pool",
			"at least one pool is required for standard tier clusters",
		)
	}

	// 3. Validate that count is set when no autoscaler is defined.
	//    Replaces: customDiffValidateOptionalCount
	invalidCountPools := make([]string, 0)
	for i, pool := range plan.Pool {
		countIsAbsent := pool.Count.IsNull() || pool.Count.IsUnknown() || pool.Count.ValueInt64() == 0
		if countIsAbsent && len(pool.Autoscaler) == 0 {
			invalidCountPools = append(invalidCountPools, fmt.Sprintf("pool[%d]", i))
		}
	}
	if len(invalidCountPools) > 0 {
		resp.Diagnostics.AddError(
			"Missing Pool Count",
			fmt.Sprintf(
				"%s: `count` must be defined when no autoscaler is defined",
				strings.Join(invalidCountPools, ", "),
			),
		)
	}

	// 4. Validate that update_strategy is only set for enterprise tier.
	//    Replaces: customDiffValidateUpdateStrategyWithTier
	tierIsEnterprise := !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && plan.Tier.ValueString() == TierEnterprise
	if !tierIsEnterprise {
		invalidStrategyPools := make([]string, 0)
		for i, pool := range plan.Pool {
			hasStrategy := !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != ""
			if hasStrategy {
				invalidStrategyPools = append(invalidStrategyPools, fmt.Sprintf("pool[%d]", i))
			}
		}
		if len(invalidStrategyPools) > 0 {
			resp.Diagnostics.AddError(
				"Invalid Update Strategy",
				fmt.Sprintf(
					"%s: `update_strategy` can only be configured when tier is set to \"enterprise\"",
					strings.Join(invalidStrategyPools, ", "),
				),
			)
		}
	}
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// Create implements resource.Resource.
func (r *LKEResource) Create(
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
		v := helper.FrameworkSafeInt64ToInt(plan.SubnetID.ValueInt64(), &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.SubnetID = linodego.Pointer(v)
	}

	if !plan.VpcID.IsNull() && !plan.VpcID.IsUnknown() {
		v := helper.FrameworkSafeInt64ToInt(plan.VpcID.ValueInt64(), &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.VpcID = linodego.Pointer(v)
	}

	if !plan.StackType.IsNull() && !plan.StackType.IsUnknown() {
		st := linodego.LKEClusterStackType(plan.StackType.ValueString())
		createOpts.StackType = linodego.Pointer(st)
	}

	// Control plane options.
	if len(plan.ControlPlane) > 0 {
		cp, cpDiags := expandFWControlPlaneOptions(ctx, plan.ControlPlane[0])
		resp.Diagnostics.Append(cpDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.ControlPlane = &cp
	}

	// Node pools.
	for _, poolModel := range plan.Pool {
		poolOpts, poolDiags := expandFWNodePoolCreateOptions(ctx, poolModel)
		resp.Diagnostics.Append(poolDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.NodePools = append(createOpts.NodePools, poolOpts)
	}

	// Tags.
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
		resp.Diagnostics.AddError("Failed to Create LKE Cluster", err.Error())
		return
	}

	plan.ID = types.Int64Value(int64(cluster.ID))
	// Persist ID early so state is not lost if later steps fail.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Enterprise clusters need kubeconfig to be ready before polling nodes.
	var retryContextTimeout time.Duration
	if cluster.Tier == TierEnterprise {
		retryContextTimeout = time.Second * 120
		if err := waitForLKEKubeConfig(ctx, *client, int(r.Meta.Config.EventPollMilliseconds.ValueInt64()), cluster.ID); err != nil {
			resp.Diagnostics.AddError("Failed to Get LKE Cluster Kubeconfig", err.Error())
			return
		}
	} else {
		retryContextTimeout = time.Second * 25
	}

	tflog.Debug(ctx, "Waiting for a single LKE cluster node to be ready")

	// Retry loop replacing retry.RetryContext — polls until a ready node exists
	// or the short retry deadline expires.
	retryDeadline := time.Now().Add(retryContextTimeout)
	for {
		if time.Now().After(retryDeadline) {
			break
		}
		tflog.Debug(ctx, "client.WaitForLKEClusterConditions(...)", map[string]any{
			"condition": "ClusterHasReadyNode",
		})
		waitErr := client.WaitForLKEClusterConditions(ctx, cluster.ID, linodego.LKEClusterPollOptions{
			TimeoutSeconds: 15 * 60,
		}, k8scondition.ClusterHasReadyNode)
		if waitErr == nil {
			break
		}
		tflog.Debug(ctx, waitErr.Error())
		select {
		case <-ctx.Done():
			break
		case <-time.After(2 * time.Second):
		}
	}

	r.readIntoModel(ctx, cluster.ID, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read implements resource.Resource.
func (r *LKEResource) Read(
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

	clusterID := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readIntoModel(ctx, clusterID, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Signal removal if the cluster was deleted externally.
	if state.ID.ValueInt64() == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update implements resource.Resource.
func (r *LKEResource) Update(
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

	client := r.Meta.Client
	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updateOpts := linodego.LKEClusterUpdateOptions{}
	clusterChanged := false

	if !plan.Label.Equal(state.Label) {
		updateOpts.Label = plan.Label.ValueString()
		clusterChanged = true
	}

	if !plan.K8sVersion.Equal(state.K8sVersion) {
		updateOpts.K8sVersion = plan.K8sVersion.ValueString()
		clusterChanged = true
	}

	if !plan.Tags.Equal(state.Tags) {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.Tags = &tags
		clusterChanged = true
	}

	if len(plan.ControlPlane) > 0 {
		cp, cpDiags := expandFWControlPlaneOptions(ctx, plan.ControlPlane[0])
		resp.Diagnostics.Append(cpDiags...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.ControlPlane = &cp
		clusterChanged = true
	}

	if clusterChanged {
		tflog.Debug(ctx, "client.UpdateLKECluster(...)", map[string]any{"options": updateOpts})
		if _, err := client.UpdateLKECluster(ctx, id, updateOpts); err != nil {
			resp.Diagnostics.AddError("Failed to Update LKE Cluster", fmt.Sprintf("failed to update LKE Cluster %d: %s", id, err))
			return
		}
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		resp.Diagnostics.AddError("Failed to List LKE Node Pools", fmt.Sprintf("failed to get Pools for LKE Cluster %d: %s", id, err))
		return
	}

	// Recycle cluster nodes to apply k8s version upgrade.
	if !plan.K8sVersion.Equal(state.K8sVersion) {
		tflog.Debug(ctx, "Implicitly recycling LKE cluster to apply Kubernetes version upgrade")
		providerMeta := &helper.ProviderMeta{
			Client: *client,
			Config: &helper.Config{
				EventPollMilliseconds: int(r.Meta.Config.EventPollMilliseconds.ValueInt64()),
			},
		}
		if err := recycleLKECluster(ctx, providerMeta, id, pools); err != nil {
			resp.Diagnostics.AddError("Failed to Recycle LKE Cluster", err.Error())
			return
		}
	}

	// Reconcile node pools.
	oldSpecs := expandFWNodePoolSpecs(state.Pool, false)
	newSpecs := expandFWNodePoolSpecs(plan.Pool, true)

	cluster, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to Get LKE Cluster", fmt.Sprintf("failed to get LKE cluster %d: %s", id, err))
		return
	}

	enterprise := cluster.Tier == TierEnterprise

	updates, reconcileErr := ReconcileLKENodePoolSpecs(ctx, oldSpecs, newSpecs, enterprise)
	if reconcileErr != nil {
		resp.Diagnostics.AddError("Failed to Reconcile LKE Node Pools", fmt.Sprintf("Failed to reconcile LKE cluster node pools: %s", reconcileErr))
		return
	}

	tflog.Trace(ctx, "Reconciled LKE cluster node pool updates", map[string]any{"updates": updates})

	updatedIDs := make([]int, 0)

	for poolID, poolUpdateOpts := range updates.ToUpdate {
		tflog.Debug(ctx, "client.UpdateLKENodePool(...)", map[string]any{"node_pool_id": poolID, "options": poolUpdateOpts})
		if _, err := client.UpdateLKENodePool(ctx, id, poolID, poolUpdateOpts); err != nil {
			resp.Diagnostics.AddError("Failed to Update LKE Node Pool", fmt.Sprintf("failed to update LKE Cluster %d Pool %d: %s", id, poolID, err))
			return
		}
		updatedIDs = append(updatedIDs, poolID)
	}

	for _, poolCreateOpts := range updates.ToCreate {
		tflog.Debug(ctx, "client.CreateLKENodePool(...)", map[string]any{"options": poolCreateOpts})
		pool, err := client.CreateLKENodePool(ctx, id, poolCreateOpts)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Create LKE Node Pool", fmt.Sprintf("failed to create LKE Cluster %d Pool: %s", id, err))
			return
		}
		updatedIDs = append(updatedIDs, pool.ID)
	}

	for _, poolID := range updates.ToDelete {
		tflog.Debug(ctx, "client.DeleteLKENodePool(...)", map[string]any{"node_pool_id": poolID})
		if err := client.DeleteLKENodePool(ctx, id, poolID); err != nil {
			resp.Diagnostics.AddError("Failed to Delete LKE Node Pool", fmt.Sprintf("failed to delete LKE Cluster %d Pool %d: %s", id, poolID, err))
			return
		}
	}

	tflog.Debug(ctx, "Waiting for all updated node pools to be ready")
	for _, poolID := range updatedIDs {
		tflog.Trace(ctx, "Waiting for node pool to be ready", map[string]any{"node_pool_id": poolID})
		if _, err := lkenodepool.WaitForNodePoolReady(
			ctx,
			*client,
			int(r.Meta.Config.LKENodeReadyPollMilliseconds.ValueInt64()),
			id,
			poolID,
		); err != nil {
			resp.Diagnostics.AddError("Failed Waiting for LKE Node Pool", fmt.Sprintf("failed to wait for LKE Cluster %d pool %d ready: %s", id, poolID, err))
			return
		}
	}

	r.readIntoModel(ctx, id, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete implements resource.Resource.
func (r *LKEResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_lke_cluster")

	// Delete reads from req.State, not req.Plan (Plan is null on delete).
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

	client := r.Meta.Client
	id := helper.FrameworkSafeInt64ToInt(state.ID.ValueInt64(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	skipDeletePoll := r.Meta.Config.SkipLKEClusterDeletePoll.ValueBool()

	var oldNodes []linodego.LKENodePoolLinode
	if !skipDeletePoll {
		tflog.Trace(ctx, "client.ListLKENodePools(...)")
		pools, err := client.ListLKENodePools(ctx, id, nil)
		if err != nil {
			if linodego.IsNotFound(err) {
				tflog.Warn(ctx, "LKE cluster not found when listing node pools, assuming already deleted")
				return
			}
			resp.Diagnostics.AddError("Failed to List LKE Node Pools", fmt.Sprintf("failed to list node pools for LKE cluster %d: %s", id, err))
			return
		}
		for _, pool := range pools {
			oldNodes = append(oldNodes, pool.Linodes...)
		}
		tflog.Debug(ctx, "Collected Linode instances from LKE cluster node pools", map[string]any{"nodes": oldNodes})
	}

	tflog.Debug(ctx, "client.DeleteLKECluster(...)")
	if err := client.DeleteLKECluster(ctx, id); err != nil {
		if !linodego.IsNotFound(err) {
			resp.Diagnostics.AddError("Failed to Delete LKE Cluster", fmt.Sprintf("failed to delete Linode LKE cluster %d: %s", id, err))
			return
		}
	}

	timeoutSeconds, convErr := helper.SafeFloat64ToInt(deleteTimeout.Seconds())
	if convErr != nil {
		resp.Diagnostics.AddError("Failed to Convert Timeout", fmt.Sprintf("failed to convert float64 deletion timeout to int: %s", convErr))
		return
	}

	tflog.Debug(ctx, "Deleted LKE cluster, waiting for all nodes deleted...")
	tflog.Trace(ctx, "client.WaitForLKEClusterStatus(...)", map[string]any{
		"status":  "not_ready",
		"timeout": timeoutSeconds,
	})

	_, statusErr := client.WaitForLKEClusterStatus(ctx, id, "not_ready", timeoutSeconds)
	if statusErr != nil {
		if !linodego.IsNotFound(statusErr) {
			resp.Diagnostics.AddError("Failed Waiting for LKE Cluster Status", statusErr.Error())
			return
		}
	}

	if !skipDeletePoll {
		if err := waitForNodesDeleted(
			ctx,
			*client,
			int(r.Meta.Config.EventPollMilliseconds.ValueInt64()),
			oldNodes,
		); err != nil {
			resp.Diagnostics.AddError("Failed Waiting for Linode Instances to Delete", fmt.Sprintf("failed waiting for Linode instances to be deleted: %s", err))
			return
		}
	}
}

// ImportState implements resource.ResourceWithImportState.
func (r *LKEResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// readIntoModel — shared read logic for Create/Read/Update
// ---------------------------------------------------------------------------

func (r *LKEResource) readIntoModel(
	ctx context.Context,
	id int,
	model *LKEResourceModel,
	diags *diag.Diagnostics,
) {
	client := r.Meta.Client
	ctx = tflog.SetField(ctx, "cluster_id", strconv.Itoa(id))

	cluster, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			model.ID = types.Int64Value(0) // caller checks for 0 to call RemoveResource
			return
		}
		diags.AddError("Failed to Get LKE Cluster", fmt.Sprintf("failed to get LKE cluster %d: %s", id, err))
		return
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		diags.AddError("Failed to List LKE Node Pools", fmt.Sprintf("failed to get pools for LKE cluster %d: %s", id, err))
		return
	}

	// Filter externally-managed pools.
	if !model.ExternalPoolTags.IsNull() && !model.ExternalPoolTags.IsUnknown() {
		var externalTags []string
		if tagDiags := model.ExternalPoolTags.ElementsAs(ctx, &externalTags, false); tagDiags.HasError() {
			diags.Append(tagDiags...)
			return
		}
		if len(externalTags) > 0 {
			pools = filterExternalPools(ctx, externalTags, pools)
		}
	}

	kubeconfig, err := client.GetLKEClusterKubeconfig(ctx, id)
	if err != nil {
		diags.AddError("Failed to Get LKE Cluster Kubeconfig", fmt.Sprintf("failed to get kubeconfig for LKE cluster %d: %s", id, err))
		return
	}

	tflog.Trace(ctx, "client.ListLKEClusterAPIEndpoints(...)")
	endpoints, err := client.ListLKEClusterAPIEndpoints(ctx, id, nil)
	if err != nil {
		diags.AddError("Failed to List LKE Cluster API Endpoints", fmt.Sprintf("failed to get API endpoints for LKE cluster %d: %s", id, err))
		return
	}

	var aclResp *linodego.LKEClusterControlPlaneACLResponse
	aclResult, aclErr := client.GetLKEClusterControlPlaneACL(ctx, id)
	if aclErr != nil {
		if lerr, ok := aclErr.(*linodego.Error); ok &&
			(lerr.Code == 404 ||
				(lerr.Code == 400 && strings.Contains(lerr.Message, "Cluster does not support Control Plane ACL"))) {
			// No ACL support — aclResp stays nil.
		} else {
			diags.AddError("Failed to Get Control Plane ACL", fmt.Sprintf("failed to get control plane ACL for LKE cluster %d: %s", id, aclErr))
			return
		}
	} else {
		aclResp = aclResult
	}

	// Dashboard URL — standard tier only.
	if cluster.Tier == TierStandard {
		dashboard, dashErr := client.GetLKEClusterDashboard(ctx, id)
		if dashErr != nil {
			diags.AddError("Failed to Get LKE Cluster Dashboard", fmt.Sprintf("failed to get dashboard URL for LKE cluster %d: %s", id, dashErr))
			return
		}
		model.DashboardURL = types.StringValue(dashboard.URL)
	}

	model.Label = types.StringValue(cluster.Label)
	model.K8sVersion = types.StringValue(cluster.K8sVersion)
	model.Region = types.StringValue(cluster.Region)
	model.Status = types.StringValue(string(cluster.Status))
	model.Tier = types.StringValue(cluster.Tier)
	model.APLEnabled = types.BoolValue(cluster.APLEnabled)
	model.SubnetID = types.Int64Value(int64(cluster.SubnetID))
	model.VpcID = types.Int64Value(int64(cluster.VpcID))
	model.StackType = types.StringValue(string(cluster.StackType))
	model.Kubeconfig = types.StringValue(kubeconfig.KubeConfig)

	tags, tagDiags := types.SetValueFrom(ctx, types.StringType, cluster.Tags)
	diags.Append(tagDiags...)
	if diags.HasError() {
		return
	}
	model.Tags = tags

	var endpointURLs []string
	for _, e := range endpoints {
		endpointURLs = append(endpointURLs, e.Endpoint)
	}
	apiEndpoints, endDiags := types.ListValueFrom(ctx, types.StringType, endpointURLs)
	diags.Append(endDiags...)
	if diags.HasError() {
		return
	}
	model.APIEndpoints = apiEndpoints

	// Control plane.
	cpModel, cpDiags := flattenFWControlPlane(ctx, cluster.ControlPlane, aclResp)
	diags.Append(cpDiags...)
	if diags.HasError() {
		return
	}
	model.ControlPlane = []LKEResourceControlPlane{cpModel}

	// Pools — match API pools to declared ordering using IDs.
	declaredPoolIDs := make([]int, len(model.Pool))
	for i, p := range model.Pool {
		if !p.ID.IsNull() && !p.ID.IsUnknown() {
			declaredPoolIDs[i] = int(p.ID.ValueInt64())
		}
	}
	orderedPools := matchFWPoolsWithDeclared(pools, declaredPoolIDs)

	flatPools, flatDiags := flattenFWNodePools(ctx, orderedPools)
	diags.Append(flatDiags...)
	if diags.HasError() {
		return
	}
	model.Pool = flatPools
}

// ---------------------------------------------------------------------------
// Framework-specific expand/flatten helpers
// (distinct from SDKv2 helpers in cluster.go which use *schema.Set)
// ---------------------------------------------------------------------------

func expandFWControlPlaneOptions(
	ctx context.Context,
	cp LKEResourceControlPlane,
) (linodego.LKEClusterControlPlaneOptions, diag.Diagnostics) {
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
	disabled := false
	result.ACL = &linodego.LKEClusterControlPlaneACLOptions{Enabled: &disabled}

	if len(cp.ACL) > 0 {
		aclOpts, aclDiags := expandFWACLOptions(ctx, cp.ACL[0])
		diags.Append(aclDiags...)
		if diags.HasError() {
			return result, diags
		}
		result.ACL = aclOpts
	}

	return result, diags
}

func expandFWACLOptions(
	ctx context.Context,
	acl LKEResourceControlPlaneACL,
) (*linodego.LKEClusterControlPlaneACLOptions, diag.Diagnostics) {
	var result linodego.LKEClusterControlPlaneACLOptions
	var diags diag.Diagnostics

	if !acl.Enabled.IsNull() && !acl.Enabled.IsUnknown() {
		v := acl.Enabled.ValueBool()
		result.Enabled = &v
	}

	if len(acl.Addresses) > 0 {
		addrs, addrDiags := expandFWACLAddresses(ctx, acl.Addresses[0])
		diags.Append(addrDiags...)
		if diags.HasError() {
			return nil, diags
		}
		result.Addresses = addrs
	}

	if result.Enabled != nil && !*result.Enabled &&
		result.Addresses != nil &&
		((result.Addresses.IPv4 != nil && len(*result.Addresses.IPv4) > 0) ||
			(result.Addresses.IPv6 != nil && len(*result.Addresses.IPv6) > 0)) {
		diags.AddError("Invalid ACL Configuration", "addresses are not acceptable when ACL is disabled")
		return nil, diags
	}

	return &result, diags
}

func expandFWACLAddresses(
	ctx context.Context,
	addrs LKEResourceControlPlaneACLAddresses,
) (*linodego.LKEClusterControlPlaneACLAddressesOptions, diag.Diagnostics) {
	var result linodego.LKEClusterControlPlaneACLAddressesOptions
	var diags diag.Diagnostics

	if !addrs.IPv4.IsNull() && !addrs.IPv4.IsUnknown() {
		var ipv4 []string
		diags.Append(addrs.IPv4.ElementsAs(ctx, &ipv4, false)...)
		if diags.HasError() {
			return nil, diags
		}
		result.IPv4 = &ipv4
	}

	if !addrs.IPv6.IsNull() && !addrs.IPv6.IsUnknown() {
		var ipv6 []string
		diags.Append(addrs.IPv6.ElementsAs(ctx, &ipv6, false)...)
		if diags.HasError() {
			return nil, diags
		}
		result.IPv6 = &ipv6
	}

	return &result, diags
}

func expandFWNodePoolCreateOptions(
	ctx context.Context,
	pool LKEResourceNodePool,
) (linodego.LKENodePoolCreateOptions, diag.Diagnostics) {
	var result linodego.LKENodePoolCreateOptions
	var diags diag.Diagnostics

	result.Type = pool.Type.ValueString()

	if !pool.Count.IsNull() && !pool.Count.IsUnknown() {
		result.Count = int(pool.Count.ValueInt64())
	}

	if !pool.Label.IsNull() && !pool.Label.IsUnknown() && pool.Label.ValueString() != "" {
		result.Label = linodego.Pointer(pool.Label.ValueString())
	}

	if !pool.FirewallID.IsNull() && !pool.FirewallID.IsUnknown() && pool.FirewallID.ValueInt64() != 0 {
		v := int(pool.FirewallID.ValueInt64())
		result.FirewallID = &v
	}

	if !pool.Tags.IsNull() && !pool.Tags.IsUnknown() {
		var tags []string
		diags.Append(pool.Tags.ElementsAs(ctx, &tags, false)...)
		if diags.HasError() {
			return result, diags
		}
		result.Tags = tags
	}

	if !pool.Labels.IsNull() && !pool.Labels.IsUnknown() {
		var labels map[string]string
		diags.Append(pool.Labels.ElementsAs(ctx, &labels, false)...)
		if diags.HasError() {
			return result, diags
		}
		result.Labels = linodego.LKENodePoolLabels(labels)
	}

	taints := make([]linodego.LKENodePoolTaint, len(pool.Taint))
	for i, t := range pool.Taint {
		taints[i] = linodego.LKENodePoolTaint{
			Effect: linodego.LKENodePoolTaintEffect(t.Effect.ValueString()),
			Key:    t.Key.ValueString(),
			Value:  t.Value.ValueString(),
		}
	}
	result.Taints = taints

	if len(pool.Autoscaler) > 0 {
		as := pool.Autoscaler[0]
		result.Autoscaler = &linodego.LKENodePoolAutoscaler{
			Enabled: true,
			Min:     int(as.Min.ValueInt64()),
			Max:     int(as.Max.ValueInt64()),
		}
		if result.Count == 0 {
			result.Count = result.Autoscaler.Min
		}
	}

	if !pool.K8sVersion.IsNull() && !pool.K8sVersion.IsUnknown() && pool.K8sVersion.ValueString() != "" {
		result.K8sVersion = linodego.Pointer(pool.K8sVersion.ValueString())
	}

	if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
		us := linodego.LKENodePoolUpdateStrategy(pool.UpdateStrategy.ValueString())
		result.UpdateStrategy = linodego.Pointer(us)
	}

	return result, diags
}

// expandFWNodePoolSpecs converts framework model pools to NodePoolSpec slice
// for use with ReconcileLKENodePoolSpecs (defined in cluster.go).
func expandFWNodePoolSpecs(pools []LKEResourceNodePool, preserveNoTarget bool) []NodePoolSpec {
	specs := make([]NodePoolSpec, 0, len(pools))

	for _, p := range pools {
		var id int
		if !p.ID.IsNull() && !p.ID.IsUnknown() {
			id = int(p.ID.ValueInt64())
		}

		if !preserveNoTarget && id == 0 {
			continue
		}

		spec := NodePoolSpec{
			ID:   id,
			Type: p.Type.ValueString(),
		}

		if !p.Count.IsNull() && !p.Count.IsUnknown() {
			spec.Count = int(p.Count.ValueInt64())
		}

		if !p.Label.IsNull() && !p.Label.IsUnknown() && p.Label.ValueString() != "" {
			spec.Label = linodego.Pointer(p.Label.ValueString())
		}

		if !p.FirewallID.IsNull() && !p.FirewallID.IsUnknown() && p.FirewallID.ValueInt64() != 0 {
			v := int(p.FirewallID.ValueInt64())
			spec.FirewallID = &v
		}

		if !p.K8sVersion.IsNull() && !p.K8sVersion.IsUnknown() && p.K8sVersion.ValueString() != "" {
			spec.K8sVersion = linodego.Pointer(p.K8sVersion.ValueString())
		}

		if !p.UpdateStrategy.IsNull() && !p.UpdateStrategy.IsUnknown() && p.UpdateStrategy.ValueString() != "" {
			spec.UpdateStrategy = linodego.Pointer(p.UpdateStrategy.ValueString())
		}

		if len(p.Autoscaler) > 0 {
			as := p.Autoscaler[0]
			spec.AutoScalerEnabled = true
			spec.AutoScalerMin = int(as.Min.ValueInt64())
			spec.AutoScalerMax = int(as.Max.ValueInt64())
		} else {
			spec.AutoScalerEnabled = false
			spec.AutoScalerMin = spec.Count
			spec.AutoScalerMax = spec.Count
		}

		spec.Taints = make([]map[string]any, len(p.Taint))
		for i, t := range p.Taint {
			spec.Taints[i] = map[string]any{
				"effect": t.Effect.ValueString(),
				"key":    t.Key.ValueString(),
				"value":  t.Value.ValueString(),
			}
		}

		specs = append(specs, spec)
	}

	return specs
}

// matchFWPoolsWithDeclared orders API pools to match the declared ordering
// using pool IDs, appending any unmatched API pools at the end.
// This is the framework counterpart to matchPoolsWithSchema (which uses *schema.Set).
func matchFWPoolsWithDeclared(apiPools []linodego.LKENodePool, declaredIDs []int) []linodego.LKENodePool {
	apiMap := make(map[int]linodego.LKENodePool, len(apiPools))
	for _, p := range apiPools {
		apiMap[p.ID] = p
	}

	result := make([]linodego.LKENodePool, len(declaredIDs))
	used := make(map[int]bool)

	for i, id := range declaredIDs {
		if p, ok := apiMap[id]; ok {
			result[i] = p
			used[id] = true
		}
	}

	// Append pools that weren't matched (e.g. newly created, or declared without IDs yet).
	for _, p := range apiPools {
		if !used[p.ID] {
			result = append(result, p)
		}
	}

	return result
}

func flattenFWNodePools(
	ctx context.Context,
	pools []linodego.LKENodePool,
) ([]LKEResourceNodePool, diag.Diagnostics) {
	var diags diag.Diagnostics
	result := make([]LKEResourceNodePool, len(pools))

	for i, pool := range pools {
		var m LKEResourceNodePool
		m.ID = types.Int64Value(int64(pool.ID))
		m.Count = types.Int64Value(int64(pool.Count))
		m.Type = types.StringValue(pool.Type)
		m.DiskEncryption = types.StringValue(string(pool.DiskEncryption))
		m.K8sVersion = types.StringPointerValue(pool.K8sVersion)

		if pool.Label != nil {
			m.Label = types.StringPointerValue(pool.Label)
		} else {
			m.Label = types.StringValue("")
		}

		if pool.FirewallID != nil {
			m.FirewallID = types.Int64Value(int64(*pool.FirewallID))
		} else {
			m.FirewallID = types.Int64Value(0)
		}

		if pool.UpdateStrategy != nil {
			m.UpdateStrategy = types.StringValue(string(*pool.UpdateStrategy))
		} else {
			m.UpdateStrategy = types.StringValue("")
		}

		tags, tagDiags := types.SetValueFrom(ctx, types.StringType, pool.Tags)
		diags.Append(tagDiags...)
		if diags.HasError() {
			return nil, diags
		}
		m.Tags = tags

		labels, labelDiags := types.MapValueFrom(ctx, types.StringType, pool.Labels)
		diags.Append(labelDiags...)
		if diags.HasError() {
			return nil, diags
		}
		m.Labels = labels

		m.Nodes = make([]LKEResourceNodePoolNode, len(pool.Linodes))
		for j, node := range pool.Linodes {
			m.Nodes[j] = LKEResourceNodePoolNode{
				ID:         types.StringValue(node.ID),
				InstanceID: types.Int64Value(int64(node.InstanceID)),
				Status:     types.StringValue(string(node.Status)),
			}
		}

		if pool.Autoscaler.Enabled {
			m.Autoscaler = []LKEResourceNodePoolAutoscaler{{
				Min: types.Int64Value(int64(pool.Autoscaler.Min)),
				Max: types.Int64Value(int64(pool.Autoscaler.Max)),
			}}
		} else {
			m.Autoscaler = []LKEResourceNodePoolAutoscaler{}
		}

		m.Taint = make([]LKEResourceNodePoolTaint, len(pool.Taints))
		for j, t := range pool.Taints {
			m.Taint[j] = LKEResourceNodePoolTaint{
				Effect: types.StringValue(string(t.Effect)),
				Key:    types.StringValue(t.Key),
				Value:  types.StringValue(t.Value),
			}
		}

		result[i] = m
	}

	return result, diags
}

func flattenFWControlPlane(
	ctx context.Context,
	cp linodego.LKEClusterControlPlane,
	aclResp *linodego.LKEClusterControlPlaneACLResponse,
) (LKEResourceControlPlane, diag.Diagnostics) {
	var result LKEResourceControlPlane
	var diags diag.Diagnostics

	result.HighAvailability = types.BoolValue(cp.HighAvailability)
	result.AuditLogsEnabled = types.BoolValue(cp.AuditLogsEnabled)

	if aclResp != nil {
		acl := aclResp.ACL
		var cpACL LKEResourceControlPlaneACL
		cpACL.Enabled = types.BoolValue(acl.Enabled)

		if acl.Addresses != nil {
			ipv4, ipv4Diags := types.SetValueFrom(ctx, types.StringType, acl.Addresses.IPv4)
			diags.Append(ipv4Diags...)
			if diags.HasError() {
				return result, diags
			}
			ipv6, ipv6Diags := types.SetValueFrom(ctx, types.StringType, acl.Addresses.IPv6)
			diags.Append(ipv6Diags...)
			if diags.HasError() {
				return result, diags
			}
			cpACL.Addresses = []LKEResourceControlPlaneACLAddresses{{IPv4: ipv4, IPv6: ipv6}}
		}

		result.ACL = []LKEResourceControlPlaneACL{cpACL}
	} else {
		result.ACL = []LKEResourceControlPlaneACL{}
	}

	return result, diags
}
