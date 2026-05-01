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

// Compile-time interface assertions.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithModifyPlan  = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func NewResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_lke_cluster",
				IDType: types.StringType,
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
// Model types
// ---------------------------------------------------------------------------

type nodeModel struct {
	ID         types.String `tfsdk:"id"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	Status     types.String `tfsdk:"status"`
}

type autoscalerModel struct {
	Min types.Int64 `tfsdk:"min"`
	Max types.Int64 `tfsdk:"max"`
}

type taintModel struct {
	Effect types.String `tfsdk:"effect"`
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
}

type poolModel struct {
	ID             types.Int64       `tfsdk:"id"`
	Label          types.String      `tfsdk:"label"`
	Count          types.Int64       `tfsdk:"count"`
	Type           types.String      `tfsdk:"type"`
	FirewallID     types.Int64       `tfsdk:"firewall_id"`
	Labels         types.Map         `tfsdk:"labels"`
	Taints         types.Set         `tfsdk:"taint"`
	Tags           types.Set         `tfsdk:"tags"`
	DiskEncryption types.String      `tfsdk:"disk_encryption"`
	Nodes          []nodeModel       `tfsdk:"nodes"`
	Autoscaler     []autoscalerModel `tfsdk:"autoscaler"`
	K8sVersion     types.String      `tfsdk:"k8s_version"`
	UpdateStrategy types.String      `tfsdk:"update_strategy"`
}

type aclAddressesModel struct {
	IPv4 types.Set `tfsdk:"ipv4"`
	IPv6 types.Set `tfsdk:"ipv6"`
}

type aclModel struct {
	Enabled   types.Bool          `tfsdk:"enabled"`
	Addresses []aclAddressesModel `tfsdk:"addresses"`
}

type controlPlaneModel struct {
	HighAvailability types.Bool `tfsdk:"high_availability"`
	AuditLogsEnabled types.Bool `tfsdk:"audit_logs_enabled"`
	ACL              []aclModel `tfsdk:"acl"`
}

// ResourceModel is the top-level Terraform state model.
type ResourceModel struct {
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
	Pools            []poolModel         `tfsdk:"pool"`
	ControlPlane     []controlPlaneModel `tfsdk:"control_plane"`
	Timeouts         timeouts.Value      `tfsdk:"timeouts"`
}

// taintObjectType is the object type descriptor for taint set elements.
var taintObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"effect": types.StringType,
		"key":    types.StringType,
		"value":  types.StringType,
	},
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed: true,
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
			Description: "The desired Kubernetes version for this Kubernetes cluster in the format of " +
				"<major>.<minor>. The latest supported patch version will be deployed.",
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
		},
		"external_pool_tags": schema.SetAttribute{
			ElementType: types.StringType,
			Optional:    true,
			Description: "An array of tags indicating that node pools having those tags are defined with a " +
				"separate nodepool resource, rather than inside the current cluster resource.",
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
			Description: "The ID of the VPC subnet to use for the Kubernetes cluster.",
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
						Computed:    true,
						Description: "Key-value pairs added as labels to nodes in the node pool.",
					},
					"disk_encryption": schema.StringAttribute{
						Computed:    true,
						Description: "The disk encryption policy for the nodes in this pool.",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"tags": schema.SetAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Computed:    true,
						Description: "A set of tags applied to this node pool.",
						PlanModifiers: []planmodifier.Set{
							setplanmodifier.UseStateForUnknown(),
						},
					},
					"k8s_version": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The desired Kubernetes version for this pool. This is only available for Enterprise clusters.",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"update_strategy": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "The strategy for updating the node pool k8s version. For LKE enterprise only.",
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
				},
				Blocks: map[string]schema.Block{
					"taint": schema.SetNestedBlock{
						Description: "Kubernetes taints to add to node pool nodes.",
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
		"control_plane": schema.ListNestedBlock{
			Description: "Defines settings for the Kubernetes Control Plane.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"high_availability": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Defines whether High Availability is enabled for the Control Plane Components of the cluster.",
						PlanModifiers: []planmodifier.Bool{
							boolplanmodifier.UseStateForUnknown(),
						},
					},
					"audit_logs_enabled": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Enables audit logs on the cluster's control plane.",
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
									Optional:    true,
									Computed:    true,
									Description: "Defines default policy. true = DENY, false = ALLOW.",
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

// ---------------------------------------------------------------------------
// ModifyPlan — replaces CustomizeDiff
// ---------------------------------------------------------------------------

// ModifyPlan performs cross-field validation previously handled by CustomizeDiff.
// It runs on create, update, and plan-only. Guards prevent running during destroy.
func (r *Resource) ModifyPlan(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
) {
	// No plan during destroy.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// --- validate tier requires v4beta API version ---
	// Mirrors SDKv2ValidateFieldRequiresAPIVersion(helper.APIVersionV4Beta, "tier").
	if !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && plan.Tier.ValueString() != "" {
		apiVersion := ""
		if r.Meta != nil && r.Meta.Config != nil {
			apiVersion = r.Meta.Config.APIVersion.ValueString()
		}
		if !strings.EqualFold(apiVersion, helper.APIVersionV4Beta) {
			resp.Diagnostics.AddAttributeError(
				path.Root("tier"),
				"Unsupported API Version",
				fmt.Sprintf(
					"tier: The api_version provider argument must be set to '%s' to use this field.",
					helper.APIVersionV4Beta,
				),
			)
		}
	}

	// --- validate count must be set when no autoscaler is defined ---
	for i, pool := range plan.Pools {
		if pool.Count.IsNull() || pool.Count.IsUnknown() {
			if len(pool.Autoscaler) == 0 {
				resp.Diagnostics.AddAttributeError(
					path.Root("pool").AtListIndex(i).AtName("count"),
					"Missing count",
					fmt.Sprintf("pool.%d: `count` must be defined when no autoscaler is defined", i),
				)
			}
		}
	}

	// --- validate standard tier requires at least one pool ---
	tierIsStandard := plan.Tier.IsNull() || plan.Tier.IsUnknown() || plan.Tier.ValueString() == TierStandard
	if tierIsStandard && len(plan.Pools) == 0 {
		resp.Diagnostics.AddError(
			"Missing Node Pool",
			"at least one pool is required for standard tier clusters",
		)
	}

	// --- validate update_strategy only allowed for enterprise tier ---
	tierIsEnterprise := !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && plan.Tier.ValueString() == TierEnterprise
	if !tierIsEnterprise {
		var invalidPools []string
		for i, pool := range plan.Pools {
			if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
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

// ---------------------------------------------------------------------------
// CRUD operations
// ---------------------------------------------------------------------------

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_lke_cluster")

	var plan ResourceModel
	client := r.Meta.Client

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

	if !plan.SubnetID.IsNull() && !plan.SubnetID.IsUnknown() && plan.SubnetID.ValueInt64() != 0 {
		v := int(plan.SubnetID.ValueInt64())
		createOpts.SubnetID = &v
	}

	if !plan.VpcID.IsNull() && !plan.VpcID.IsUnknown() && plan.VpcID.ValueInt64() != 0 {
		v := int(plan.VpcID.ValueInt64())
		createOpts.VpcID = &v
	}

	if !plan.StackType.IsNull() && !plan.StackType.IsUnknown() && plan.StackType.ValueString() != "" {
		st := linodego.LKEClusterStackType(plan.StackType.ValueString())
		createOpts.StackType = &st
	}

	if len(plan.ControlPlane) > 0 {
		cp, diags := expandControlPlaneOptionsFramework(ctx, plan.ControlPlane[0])
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.ControlPlane = &cp
	}

	for _, pool := range plan.Pools {
		poolOpts, diags := expandPoolCreateOptionsFramework(ctx, pool)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.NodePools = append(createOpts.NodePools, poolOpts)
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &createOpts.Tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	tflog.Debug(ctx, "client.CreateLKECluster(...)", map[string]any{"options": createOpts})
	cluster, err := client.CreateLKECluster(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create LKE cluster", err.Error())
		return
	}

	// Persist the ID early so partial state is available on error.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(strconv.Itoa(cluster.ID)))...)

	// Enterprise clusters take longer for kubeconfig to be available.
	if cluster.Tier == TierEnterprise {
		if err := waitForLKEKubeConfig(
			ctx,
			*client,
			int(r.Meta.Config.EventPollMilliseconds.ValueInt64()),
			cluster.ID,
		); err != nil {
			resp.Diagnostics.AddError("Failed to get LKE cluster kubeconfig", err.Error())
			return
		}
	}

	tflog.Debug(ctx, "Waiting for a single LKE cluster node to be ready")
	if err := client.WaitForLKEClusterConditions(ctx, cluster.ID, linodego.LKEClusterPollOptions{
		TimeoutSeconds: 15 * 60,
	}, k8scondition.ClusterHasReadyNode); err != nil {
		// Log but don't fail — the node may still come up.
		tflog.Debug(ctx, "WaitForLKEClusterConditions error (non-fatal): "+err.Error())
	}

	r.readIntoModel(ctx, cluster.ID, &plan, &resp.Diagnostics)
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

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if helper.FrameworkAttemptRemoveResourceForEmptyID(ctx, state.ID, resp) {
		return
	}

	id := helper.FrameworkSafeStringToInt(state.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readIntoModel(ctx, id, &state, &resp.Diagnostics)
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

	client := r.Meta.Client

	var plan, state ResourceModel
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

	id := helper.FrameworkSafeStringToInt(state.ID.ValueString(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
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
		cp, diags := expandControlPlaneOptionsFramework(ctx, plan.ControlPlane[0])
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.ControlPlane = &cp
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
		tflog.Debug(ctx, "client.UpdateLKECluster(...)", map[string]any{"options": updateOpts})
		if _, err := client.UpdateLKECluster(ctx, id, updateOpts); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to update LKE Cluster %d", id), err.Error())
			return
		}
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to get Pools for LKE Cluster %d", id), err.Error())
		return
	}

	// Recycle all nodes when the Kubernetes version is upgraded.
	if !plan.K8sVersion.Equal(state.K8sVersion) {
		tflog.Debug(ctx, "Implicitly recycling LKE cluster to apply Kubernetes version upgrade")
		if err := recycleLKEClusterFramework(ctx, r.Meta, id, pools); err != nil {
			resp.Diagnostics.AddError("Failed to recycle LKE cluster", err.Error())
			return
		}
	}

	// Determine enterprise status for reconciliation.
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

	enterprise := cluster.Tier == TierEnterprise

	oldPoolSpecs := expandPoolSpecsFromModel(state.Pools)
	newPoolSpecs := expandPoolSpecsFromModel(plan.Pools)

	updates, err := ReconcileLKENodePoolSpecs(ctx, oldPoolSpecs, newPoolSpecs, enterprise)
	if err != nil {
		resp.Diagnostics.AddError("Failed to reconcile LKE cluster node pools", err.Error())
		return
	}

	tflog.Trace(ctx, "Reconciled LKE cluster node pool updates", map[string]any{"updates": updates})

	updatedIDs := make([]int, 0)

	for poolID, upOpts := range updates.ToUpdate {
		tflog.Debug(ctx, "client.UpdateLKENodePool(...)", map[string]any{"node_pool_id": poolID, "options": upOpts})
		if _, err := client.UpdateLKENodePool(ctx, id, poolID, upOpts); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to update LKE Cluster %d Pool %d", id, poolID), err.Error())
			return
		}
		updatedIDs = append(updatedIDs, poolID)
	}

	for _, createOpts := range updates.ToCreate {
		tflog.Debug(ctx, "client.CreateLKENodePool(...)", map[string]any{"options": createOpts})
		pool, err := client.CreateLKENodePool(ctx, id, createOpts)
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
	for _, poolID := range updatedIDs {
		if _, err := lkenodepool.WaitForNodePoolReady(
			ctx,
			*client,
			int(r.Meta.Config.LKENodeReadyPollMilliseconds.ValueInt64()),
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

	r.readIntoModel(ctx, id, &plan, &resp.Diagnostics)
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

	client := r.Meta.Client

	var state ResourceModel
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

	id := helper.FrameworkSafeStringToInt(state.ID.ValueString(), &resp.Diagnostics)
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
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed to list node pools for LKE cluster %d", id),
				err.Error(),
			)
			return
		}
		for _, pool := range pools {
			oldNodes = append(oldNodes, pool.Linodes...)
		}
		tflog.Debug(ctx, "Collected Linode instances from LKE cluster node pools",
			map[string]any{"nodes": oldNodes})
	}

	tflog.Debug(ctx, "client.DeleteLKECluster(...)")
	if err := client.DeleteLKECluster(ctx, id); err != nil {
		if !linodego.IsNotFound(err) {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to delete Linode LKE cluster %d", id), err.Error())
			return
		}
	}

	timeoutSeconds := helper.FrameworkSafeFloat64ToInt(deleteTimeout.Seconds(), &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Deleted LKE cluster, waiting for all nodes deleted...")
	tflog.Trace(ctx, "client.WaitForLKEClusterStatus(...)",
		map[string]any{"status": "not_ready", "timeout": timeoutSeconds})

	_, err := client.WaitForLKEClusterStatus(ctx, id, "not_ready", timeoutSeconds)
	if err != nil && !linodego.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed waiting for LKE cluster deletion", err.Error())
		return
	}

	if !skipDeletePoll {
		if err := waitForNodesDeleted(
			ctx,
			*client,
			int(r.Meta.Config.EventPollMilliseconds.ValueInt64()),
			oldNodes,
		); err != nil {
			resp.Diagnostics.AddError("Failed waiting for Linode instances to be deleted", err.Error())
			return
		}
	}
}

// ImportState delegates to the base resource's passthrough import.
func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

// readIntoModel fetches all remote data for a cluster and populates model.
func (r *Resource) readIntoModel(
	ctx context.Context,
	id int,
	model *ResourceModel,
	diagnostics *diag.Diagnostics,
) {
	client := r.Meta.Client

	cluster, err := client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			// Caller is responsible for removing from state when reading.
			return
		}
		diagnostics.AddError(fmt.Sprintf("Failed to get LKE cluster %d", id), err.Error())
		return
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		diagnostics.AddError(fmt.Sprintf("Failed to get pools for LKE cluster %d", id), err.Error())
		return
	}

	// Filter out pools managed by external nodepool resources.
	if !model.ExternalPoolTags.IsNull() && !model.ExternalPoolTags.IsUnknown() {
		var externalTags []string
		diagnostics.Append(model.ExternalPoolTags.ElementsAs(ctx, &externalTags, false)...)
		if diagnostics.HasError() {
			return
		}
		if len(externalTags) > 0 {
			pools = filterExternalPools(ctx, externalTags, pools)
		}
	}

	kubeconfig, err := client.GetLKEClusterKubeconfig(ctx, id)
	if err != nil {
		diagnostics.AddError(fmt.Sprintf("Failed to get kubeconfig for LKE cluster %d", id), err.Error())
		return
	}

	tflog.Trace(ctx, "client.ListLKEClusterAPIEndpoints(...)")
	endpoints, err := client.ListLKEClusterAPIEndpoints(ctx, id, nil)
	if err != nil {
		diagnostics.AddError(fmt.Sprintf("Failed to get API endpoints for LKE cluster %d", id), err.Error())
		return
	}

	acl, err := client.GetLKEClusterControlPlaneACL(ctx, id)
	if err != nil {
		if lerr, ok := err.(*linodego.Error); ok &&
			(lerr.Code == 404 ||
				(lerr.Code == 400 && strings.Contains(lerr.Message, "Cluster does not support Control Plane ACL"))) {
			acl = nil
		} else {
			diagnostics.AddError(
				fmt.Sprintf("Failed to get control plane ACL for LKE cluster %d", id),
				err.Error(),
			)
			return
		}
	}

	// Dashboard URL is only available for standard clusters.
	if cluster.Tier == TierStandard {
		dashboard, err := client.GetLKEClusterDashboard(ctx, id)
		if err != nil {
			diagnostics.AddError(fmt.Sprintf("Failed to get dashboard URL for LKE cluster %d", id), err.Error())
			return
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
	model.SubnetID = types.Int64Value(int64(cluster.SubnetID))
	model.VpcID = types.Int64Value(int64(cluster.VpcID))
	model.StackType = types.StringValue(string(cluster.StackType))

	tagSet, d := types.SetValueFrom(ctx, types.StringType, cluster.Tags)
	diagnostics.Append(d...)
	if diagnostics.HasError() {
		return
	}
	model.Tags = tagSet

	endpointStrings := flattenLKEClusterAPIEndpoints(endpoints)
	endpointList, d := types.ListValueFrom(ctx, types.StringType, endpointStrings)
	diagnostics.Append(d...)
	if diagnostics.HasError() {
		return
	}
	model.APIEndpoints = endpointList

	model.ControlPlane = flattenControlPlaneFramework(ctx, cluster.ControlPlane, acl, diagnostics)
	if diagnostics.HasError() {
		return
	}

	model.Pools = flattenPoolsFramework(ctx, pools, diagnostics)
}

// recycleLKEClusterFramework is the Framework equivalent of recycleLKECluster.
// It recycles all nodes in the cluster and waits for new ones to become ready.
func recycleLKEClusterFramework(
	ctx context.Context,
	meta *helper.FrameworkProviderMeta,
	id int,
	pools []linodego.LKENodePool,
) error {
	client := meta.Client

	tflog.Info(ctx, "Recycling LKE cluster")
	tflog.Trace(ctx, "client.RecycleLKEClusterNodes(...)")

	if err := client.RecycleLKEClusterNodes(ctx, id); err != nil {
		return fmt.Errorf("failed to recycle LKE Cluster (%d): %w", id, err)
	}

	oldNodes := make([]linodego.LKENodePoolLinode, 0)
	for _, pool := range pools {
		oldNodes = append(oldNodes, pool.Linodes...)
	}

	tflog.Debug(ctx, "Waiting for all nodes to be deleted", map[string]any{"nodes": oldNodes})

	if err := waitForNodesDeleted(
		ctx,
		*client,
		int(meta.Config.EventPollMilliseconds.ValueInt64()),
		oldNodes,
	); err != nil {
		return fmt.Errorf("failed to wait for old nodes to be recycled: %w", err)
	}

	tflog.Debug(ctx, "All old nodes detected as deleted, waiting for all node pools to enter ready status")

	for _, pool := range pools {
		if _, err := lkenodepool.WaitForNodePoolReady(
			ctx,
			*client,
			int(meta.Config.EventPollMilliseconds.ValueInt64()),
			id,
			pool.ID,
		); err != nil {
			return fmt.Errorf("failed to wait for pool %d ready: %w", pool.ID, err)
		}
	}

	tflog.Debug(ctx, "All node pools have entered ready status; recycle operation completed")
	return nil
}

// expandPoolCreateOptionsFramework converts a pool model to linodego create options.
func expandPoolCreateOptionsFramework(ctx context.Context, pool poolModel) (linodego.LKENodePoolCreateOptions, diag.Diagnostics) {
	var d diag.Diagnostics

	opts := linodego.LKENodePoolCreateOptions{
		Type: pool.Type.ValueString(),
	}

	if !pool.Count.IsNull() && !pool.Count.IsUnknown() {
		opts.Count = int(pool.Count.ValueInt64())
	}

	if !pool.Label.IsNull() && !pool.Label.IsUnknown() && pool.Label.ValueString() != "" {
		v := pool.Label.ValueString()
		opts.Label = &v
	}

	if !pool.FirewallID.IsNull() && !pool.FirewallID.IsUnknown() && pool.FirewallID.ValueInt64() != 0 {
		v := int(pool.FirewallID.ValueInt64())
		opts.FirewallID = &v
	}

	if !pool.Tags.IsNull() && !pool.Tags.IsUnknown() {
		d.Append(pool.Tags.ElementsAs(ctx, &opts.Tags, false)...)
		if d.HasError() {
			return opts, d
		}
	}

	if !pool.Labels.IsNull() && !pool.Labels.IsUnknown() {
		labels := make(map[string]string)
		d.Append(pool.Labels.ElementsAs(ctx, &labels, false)...)
		if d.HasError() {
			return opts, d
		}
		opts.Labels = linodego.LKENodePoolLabels(labels)
	}

	if !pool.Taints.IsNull() && !pool.Taints.IsUnknown() {
		var taintModels []taintModel
		d.Append(pool.Taints.ElementsAs(ctx, &taintModels, false)...)
		if d.HasError() {
			return opts, d
		}
		opts.Taints = expandTaintsFromModels(taintModels)
	}

	if len(pool.Autoscaler) > 0 {
		as := pool.Autoscaler[0]
		opts.Autoscaler = &linodego.LKENodePoolAutoscaler{
			Enabled: true,
			Min:     int(as.Min.ValueInt64()),
			Max:     int(as.Max.ValueInt64()),
		}
		// When count is not explicit, default to autoscaler minimum.
		if opts.Count == 0 {
			opts.Count = opts.Autoscaler.Min
		}
	}

	if !pool.K8sVersion.IsNull() && !pool.K8sVersion.IsUnknown() && pool.K8sVersion.ValueString() != "" {
		v := pool.K8sVersion.ValueString()
		opts.K8sVersion = &v
	}

	if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
		us := linodego.LKENodePoolUpdateStrategy(pool.UpdateStrategy.ValueString())
		opts.UpdateStrategy = &us
	}

	return opts, d
}

// expandControlPlaneOptionsFramework converts a controlPlaneModel to linodego options.
func expandControlPlaneOptionsFramework(ctx context.Context, cp controlPlaneModel) (linodego.LKEClusterControlPlaneOptions, diag.Diagnostics) {
	var d diag.Diagnostics
	var result linodego.LKEClusterControlPlaneOptions

	if !cp.HighAvailability.IsNull() && !cp.HighAvailability.IsUnknown() {
		v := cp.HighAvailability.ValueBool()
		result.HighAvailability = &v
	}

	if !cp.AuditLogsEnabled.IsNull() && !cp.AuditLogsEnabled.IsUnknown() {
		v := cp.AuditLogsEnabled.ValueBool()
		result.AuditLogsEnabled = &v
	}

	// Default ACL to disabled so we always send it.
	disabled := false
	result.ACL = &linodego.LKEClusterControlPlaneACLOptions{Enabled: &disabled}

	if len(cp.ACL) > 0 {
		aclResult, diags := expandACLOptionsFramework(ctx, cp.ACL[0])
		d.Append(diags...)
		if d.HasError() {
			return result, d
		}
		result.ACL = aclResult
	}

	return result, d
}

func expandACLOptionsFramework(ctx context.Context, acl aclModel) (*linodego.LKEClusterControlPlaneACLOptions, diag.Diagnostics) {
	var d diag.Diagnostics
	var result linodego.LKEClusterControlPlaneACLOptions

	if !acl.Enabled.IsNull() && !acl.Enabled.IsUnknown() {
		v := acl.Enabled.ValueBool()
		result.Enabled = &v
	}

	if len(acl.Addresses) > 0 {
		addr := acl.Addresses[0]
		addrsResult := &linodego.LKEClusterControlPlaneACLAddressesOptions{}

		if !addr.IPv4.IsNull() && !addr.IPv4.IsUnknown() {
			var ipv4 []string
			d.Append(addr.IPv4.ElementsAs(ctx, &ipv4, false)...)
			if d.HasError() {
				return nil, d
			}
			addrsResult.IPv4 = &ipv4
		}

		if !addr.IPv6.IsNull() && !addr.IPv6.IsUnknown() {
			var ipv6 []string
			d.Append(addr.IPv6.ElementsAs(ctx, &ipv6, false)...)
			if d.HasError() {
				return nil, d
			}
			addrsResult.IPv6 = &ipv6
		}

		result.Addresses = addrsResult
	}

	// Validate: addresses must be empty when ACL is disabled.
	if result.Enabled != nil && !*result.Enabled &&
		result.Addresses != nil &&
		((result.Addresses.IPv4 != nil && len(*result.Addresses.IPv4) > 0) ||
			(result.Addresses.IPv6 != nil && len(*result.Addresses.IPv6) > 0)) {
		d.AddError("Invalid ACL configuration", "addresses are not acceptable when ACL is disabled")
	}

	return &result, d
}

// flattenControlPlaneFramework converts API control plane data to model slice.
func flattenControlPlaneFramework(
	ctx context.Context,
	cp linodego.LKEClusterControlPlane,
	aclResp *linodego.LKEClusterControlPlaneACLResponse,
	diagnostics *diag.Diagnostics,
) []controlPlaneModel {
	cpModel := controlPlaneModel{
		HighAvailability: types.BoolValue(cp.HighAvailability),
		AuditLogsEnabled: types.BoolValue(cp.AuditLogsEnabled),
	}

	if aclResp != nil {
		acl := aclResp.ACL
		aclM := aclModel{
			Enabled: types.BoolValue(acl.Enabled),
		}

		if acl.Addresses != nil {
			addr := aclAddressesModel{}
			var ipv4Strs, ipv6Strs []string

			if acl.Addresses.IPv4 != nil {
				ipv4Strs = *acl.Addresses.IPv4
			}
			if acl.Addresses.IPv6 != nil {
				ipv6Strs = *acl.Addresses.IPv6
			}

			ipv4Set, d := types.SetValueFrom(ctx, types.StringType, ipv4Strs)
			diagnostics.Append(d...)
			ipv6Set, d := types.SetValueFrom(ctx, types.StringType, ipv6Strs)
			diagnostics.Append(d...)

			addr.IPv4 = ipv4Set
			addr.IPv6 = ipv6Set
			aclM.Addresses = []aclAddressesModel{addr}
		}

		cpModel.ACL = []aclModel{aclM}
	}

	return []controlPlaneModel{cpModel}
}

// flattenPoolsFramework converts API node pools to model list.
func flattenPoolsFramework(
	ctx context.Context,
	pools []linodego.LKENodePool,
	diagnostics *diag.Diagnostics,
) []poolModel {
	result := make([]poolModel, len(pools))
	for i, pool := range pools {
		p := poolModel{
			ID:             types.Int64Value(int64(pool.ID)),
			Count:          types.Int64Value(int64(pool.Count)),
			Type:           types.StringValue(pool.Type),
			DiskEncryption: types.StringValue(string(pool.DiskEncryption)),
		}

		if pool.Label != nil {
			p.Label = types.StringValue(*pool.Label)
		} else {
			p.Label = types.StringValue("")
		}

		if pool.FirewallID != nil {
			p.FirewallID = types.Int64Value(int64(*pool.FirewallID))
		} else {
			p.FirewallID = types.Int64Value(0)
		}

		if pool.K8sVersion != nil {
			p.K8sVersion = types.StringValue(*pool.K8sVersion)
		} else {
			p.K8sVersion = types.StringNull()
		}

		if pool.UpdateStrategy != nil {
			p.UpdateStrategy = types.StringValue(string(*pool.UpdateStrategy))
		} else {
			p.UpdateStrategy = types.StringNull()
		}

		tagSet, d := types.SetValueFrom(ctx, types.StringType, pool.Tags)
		diagnostics.Append(d...)

		labelsMap, d := types.MapValueFrom(ctx, types.StringType, map[string]string(pool.Labels))
		diagnostics.Append(d...)

		p.Tags = tagSet
		p.Labels = labelsMap

		// taints
		taintModels := flattenTaints(pool.Taints)
		taintSet, d := types.SetValueFrom(ctx, taintObjectType, taintModels)
		diagnostics.Append(d...)
		p.Taints = taintSet

		// nodes
		nodes := make([]nodeModel, len(pool.Linodes))
		for j, node := range pool.Linodes {
			nodes[j] = nodeModel{
				ID:         types.StringValue(node.ID),
				InstanceID: types.Int64Value(int64(node.InstanceID)),
				Status:     types.StringValue(string(node.Status)),
			}
		}
		p.Nodes = nodes

		// autoscaler
		if pool.Autoscaler.Enabled {
			p.Autoscaler = []autoscalerModel{
				{
					Min: types.Int64Value(int64(pool.Autoscaler.Min)),
					Max: types.Int64Value(int64(pool.Autoscaler.Max)),
				},
			}
		} else {
			p.Autoscaler = []autoscalerModel{}
		}

		result[i] = p
	}

	return result
}

// flattenTaints converts API taints to taintModel slice.
func flattenTaints(taints []linodego.LKENodePoolTaint) []taintModel {
	result := make([]taintModel, len(taints))
	for i, t := range taints {
		result[i] = taintModel{
			Effect: types.StringValue(string(t.Effect)),
			Key:    types.StringValue(t.Key),
			Value:  types.StringValue(t.Value),
		}
	}
	return result
}

// expandTaintsFromModels converts taintModel slice to linodego taints.
func expandTaintsFromModels(taints []taintModel) []linodego.LKENodePoolTaint {
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

// expandPoolSpecsFromModel converts pool models to NodePoolSpec slice for reconciliation.
func expandPoolSpecsFromModel(pools []poolModel) []NodePoolSpec {
	specs := make([]NodePoolSpec, 0, len(pools))
	bg := context.Background()

	for _, pool := range pools {
		spec := NodePoolSpec{
			ID:    int(pool.ID.ValueInt64()),
			Type:  pool.Type.ValueString(),
			Count: int(pool.Count.ValueInt64()),
		}

		if !pool.Label.IsNull() && !pool.Label.IsUnknown() && pool.Label.ValueString() != "" {
			v := pool.Label.ValueString()
			spec.Label = &v
		}

		if !pool.FirewallID.IsNull() && !pool.FirewallID.IsUnknown() && pool.FirewallID.ValueInt64() != 0 {
			v := int(pool.FirewallID.ValueInt64())
			spec.FirewallID = &v
		}

		if !pool.Tags.IsNull() && !pool.Tags.IsUnknown() {
			var tags []string
			pool.Tags.ElementsAs(bg, &tags, false) //nolint:errcheck
			spec.Tags = tags
		}

		if !pool.Labels.IsNull() && !pool.Labels.IsUnknown() {
			labels := make(map[string]string)
			pool.Labels.ElementsAs(bg, &labels, false) //nolint:errcheck
			spec.Labels = labels
		}

		if !pool.Taints.IsNull() && !pool.Taints.IsUnknown() {
			var taintModels []taintModel
			pool.Taints.ElementsAs(bg, &taintModels, false) //nolint:errcheck
			taints := make([]map[string]any, len(taintModels))
			for i, t := range taintModels {
				taints[i] = map[string]any{
					"effect": t.Effect.ValueString(),
					"key":    t.Key.ValueString(),
					"value":  t.Value.ValueString(),
				}
			}
			spec.Taints = taints
		}

		if len(pool.Autoscaler) > 0 {
			as := pool.Autoscaler[0]
			spec.AutoScalerEnabled = true
			spec.AutoScalerMin = int(as.Min.ValueInt64())
			spec.AutoScalerMax = int(as.Max.ValueInt64())
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

// flattenLKEClusterAPIEndpoints converts endpoint objects to a string slice.
func flattenLKEClusterAPIEndpoints(apiEndpoints []linodego.LKEClusterAPIEndpoint) []string {
	flattened := make([]string, len(apiEndpoints))
	for i, endpoint := range apiEndpoints {
		flattened[i] = endpoint.Endpoint
	}
	return flattened
}
