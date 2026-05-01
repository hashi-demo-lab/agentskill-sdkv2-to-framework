package lke

// resource.go — migrated from SDKv2 to terraform-plugin-framework.
//
// Key migration decisions:
//   - control_plane (MaxItems:1): → SingleNestedBlock (preserves HCL block syntax).
//   - acl (MaxItems:1 inside control_plane): → SingleNestedBlock.
//   - pool (repeating): → ListNestedBlock.
//   - autoscaler (MaxItems:1 inside pool): → ListNestedBlock + SizeAtMost(1).
//   - Timeouts: timeouts.Block (preserves `timeouts { ... }` HCL block syntax).
//   - CustomizeDiff: migrated to ModifyPlan (ResourceWithModifyPlan).
//   - kubeconfig: Sensitive: true preserved.
//   - matchPoolsWithSchema: re-implemented without SDKv2 types.

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
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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

// Compile-time interface assertions.
var (
	_ resource.Resource                = &lkeClusterResource{}
	_ resource.ResourceWithConfigure   = &lkeClusterResource{}
	_ resource.ResourceWithImportState = &lkeClusterResource{}
	_ resource.ResourceWithModifyPlan  = &lkeClusterResource{}
)

// NewResource returns the framework resource for registration.
func NewResource() resource.Resource {
	return &lkeClusterResource{}
}

// lkeClusterResource is the framework resource implementation.
type lkeClusterResource struct {
	client *linodego.Client
	meta   *helper.FrameworkProviderMeta
}

// --------------------------------------------------------------------------
// Model types
// --------------------------------------------------------------------------

type lkeNodeModel struct {
	ID         types.String `tfsdk:"id"`
	InstanceID types.Int64  `tfsdk:"instance_id"`
	Status     types.String `tfsdk:"status"`
}

type lkeTaintModel struct {
	Effect types.String `tfsdk:"effect"`
	Key    types.String `tfsdk:"key"`
	Value  types.String `tfsdk:"value"`
}

type lkeAutoscalerModel struct {
	Min types.Int64 `tfsdk:"min"`
	Max types.Int64 `tfsdk:"max"`
}

type lkePoolModel struct {
	ID             types.Int64          `tfsdk:"id"`
	Label          types.String         `tfsdk:"label"`
	Count          types.Int64          `tfsdk:"count"`
	Type           types.String         `tfsdk:"type"`
	FirewallID     types.Int64          `tfsdk:"firewall_id"`
	Labels         types.Map            `tfsdk:"labels"`
	Taints         []lkeTaintModel      `tfsdk:"taint"`
	Tags           types.Set            `tfsdk:"tags"`
	DiskEncryption types.String         `tfsdk:"disk_encryption"`
	Nodes          []lkeNodeModel       `tfsdk:"nodes"`
	Autoscaler     []lkeAutoscalerModel `tfsdk:"autoscaler"`
	K8sVersion     types.String         `tfsdk:"k8s_version"`
	UpdateStrategy types.String         `tfsdk:"update_strategy"`
}

type lkeACLAddressesModel struct {
	IPv4 types.Set `tfsdk:"ipv4"`
	IPv6 types.Set `tfsdk:"ipv6"`
}

type lkeACLModel struct {
	Enabled   types.Bool             `tfsdk:"enabled"`
	Addresses []lkeACLAddressesModel `tfsdk:"addresses"`
}

type lkeControlPlaneModel struct {
	HighAvailability types.Bool    `tfsdk:"high_availability"`
	AuditLogsEnabled types.Bool    `tfsdk:"audit_logs_enabled"`
	ACL              []lkeACLModel `tfsdk:"acl"`
}

type lkeClusterModel struct {
	ID               types.Int64            `tfsdk:"id"`
	Label            types.String           `tfsdk:"label"`
	K8sVersion       types.String           `tfsdk:"k8s_version"`
	APLEnabled       types.Bool             `tfsdk:"apl_enabled"`
	Tags             types.Set              `tfsdk:"tags"`
	ExternalPoolTags types.Set              `tfsdk:"external_pool_tags"`
	Region           types.String           `tfsdk:"region"`
	APIEndpoints     types.List             `tfsdk:"api_endpoints"`
	Kubeconfig       types.String           `tfsdk:"kubeconfig"`
	DashboardURL     types.String           `tfsdk:"dashboard_url"`
	Status           types.String           `tfsdk:"status"`
	Tier             types.String           `tfsdk:"tier"`
	SubnetID         types.Int64            `tfsdk:"subnet_id"`
	VpcID            types.Int64            `tfsdk:"vpc_id"`
	StackType        types.String           `tfsdk:"stack_type"`
	Pools            []lkePoolModel         `tfsdk:"pool"`
	ControlPlane     *lkeControlPlaneModel  `tfsdk:"control_plane"`
	Timeouts         timeouts.Value         `tfsdk:"timeouts"`
}

// --------------------------------------------------------------------------
// resource.Resource — Metadata, Schema
// --------------------------------------------------------------------------

func (r *lkeClusterResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lke_cluster"
}

func (r *lkeClusterResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
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
				Description: "The desired Kubernetes version for this Kubernetes cluster in the format of " +
					"<major>.<minor>. The latest supported patch version will be deployed.",
			},
			"apl_enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Description: "Enables the App Platform Layer for this cluster. " +
					"Note: v4beta only and may not currently be available to all users.",
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
				Description: "An array of tags indicating that node pools having those tags are defined with " +
					"a separate nodepool resource, rather than inside the current cluster resource.",
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
				Validators: []validator.String{
					stringvalidator.OneOf(
						string(linodego.LKEClusterStackIPv4),
						string(linodego.LKEClusterDualStack),
					),
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
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"count": schema.Int64Attribute{
							Optional:    true,
							Computed:    true,
							Description: "The number of nodes in the Node Pool.",
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
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
						},
						"k8s_version": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "The desired Kubernetes version for this pool. Only available for Enterprise clusters.",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"update_strategy": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "The strategy for updating the node pool k8s version. For LKE enterprise only.",
							Validators: []validator.String{
								stringvalidator.OneOf(
									string(linodego.LKENodePoolOnRecycle),
									string(linodego.LKENodePoolRollingUpdate),
								),
							},
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
						// autoscaler: MaxItems:1 — ListNestedBlock + SizeAtMost(1).
						// Using ListNestedBlock (not SingleNestedBlock) to preserve the
						// list-shaped state path (autoscaler.0.min) existing configs reference.
						"autoscaler": schema.ListNestedBlock{
							Description: "When specified, the number of nodes autoscales within the defined minimum and maximum values.",
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
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
					},
				},
			},
			// control_plane: MaxItems:1 — SingleNestedBlock preserves `control_plane { ... }` HCL syntax.
			"control_plane": schema.SingleNestedBlock{
				Description: "Defines settings for the Kubernetes Control Plane.",
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
					// acl: MaxItems:1 — SingleNestedBlock.
					"acl": schema.SingleNestedBlock{
						Description: "Defines the ACL configuration for an LKE cluster's control plane.",
						Attributes: map[string]schema.Attribute{
							"enabled": schema.BoolAttribute{
								Optional:    true,
								Computed:    true,
								Description: "Defines default policy. A value of true results in a default policy of DENY.",
								PlanModifiers: []planmodifier.Bool{
									boolplanmodifier.UseStateForUnknown(),
								},
							},
						},
						Blocks: map[string]schema.Block{
							"addresses": schema.SingleNestedBlock{
								Description: "A list of ip addresses to allow.",
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
			// timeouts: Block (not Attributes) to preserve `timeouts { create = "35m" }` HCL syntax.
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

// --------------------------------------------------------------------------
// resource.ResourceWithConfigure
// --------------------------------------------------------------------------

func (r *lkeClusterResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	meta, ok := req.ProviderData.(*helper.FrameworkProviderMeta)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *helper.FrameworkProviderMeta, got: %T", req.ProviderData),
		)
		return
	}
	r.meta = meta
	r.client = meta.Client
}

// --------------------------------------------------------------------------
// resource.ResourceWithImportState
// --------------------------------------------------------------------------

func (r *lkeClusterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected a numeric cluster ID, got %q: %s", req.ID, err),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// --------------------------------------------------------------------------
// resource.ResourceWithModifyPlan — replaces CustomizeDiff
// --------------------------------------------------------------------------

func (r *lkeClusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Destroy path — no plan modifications needed.
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan lkeClusterModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// 1. ComputedWithDefault("tags", []string{}): default tags to empty set when unset.
	if plan.Tags.IsNull() || plan.Tags.IsUnknown() {
		emptySet, d := types.SetValueFrom(ctx, types.StringType, []string{})
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Tags = emptySet
	}

	// 2. CaseInsensitiveSet("tags"): carry prior-state case forward for case-insensitive matches.
	if !req.State.Raw.IsNull() {
		var state lkeClusterModel
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
		if resp.Diagnostics.HasError() {
			return
		}
		plan.Tags = applyCaseInsensitiveSet(ctx, state.Tags, plan.Tags, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// 3. customDiffValidateOptionalCount: pools without autoscaler must have count.
	{
		invalid := make([]string, 0)
		for i, pool := range plan.Pools {
			if (pool.Count.IsNull() || pool.Count.IsUnknown()) && len(pool.Autoscaler) == 0 {
				invalid = append(invalid, fmt.Sprintf("pool.%d", i))
			}
		}
		if len(invalid) > 0 {
			resp.Diagnostics.AddError(
				"Missing node count",
				fmt.Sprintf("%s: `count` must be defined when no autoscaler is defined", strings.Join(invalid, ", ")),
			)
			return
		}
	}

	// 4. customDiffValidatePoolForStandardTier: standard tier needs at least one pool.
	{
		tierIsStandard := plan.Tier.IsNull() || plan.Tier.IsUnknown() || plan.Tier.ValueString() == TierStandard
		if tierIsStandard && len(plan.Pools) == 0 {
			resp.Diagnostics.AddError(
				"Missing pool",
				"at least one pool is required for standard tier clusters",
			)
			return
		}
	}

	// 5. customDiffValidateUpdateStrategyWithTier: update_strategy requires enterprise tier.
	{
		tierIsEnterprise := !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && plan.Tier.ValueString() == TierEnterprise
		if !tierIsEnterprise {
			invalid := make([]string, 0)
			for i, pool := range plan.Pools {
				if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
					invalid = append(invalid, fmt.Sprintf("pool.%d", i))
				}
			}
			if len(invalid) > 0 {
				resp.Diagnostics.AddError(
					"Invalid update_strategy",
					fmt.Sprintf(
						"%s: `update_strategy` can only be configured when tier is set to \"enterprise\"",
						strings.Join(invalid, ", "),
					),
				)
				return
			}
		}
	}

	// 6. SDKv2ValidateFieldRequiresAPIVersion: tier requires v4beta API version.
	if r.meta != nil {
		if !plan.Tier.IsNull() && !plan.Tier.IsUnknown() && plan.Tier.ValueString() != "" {
			apiVersion := r.meta.Config.APIVersion.ValueString()
			if !strings.EqualFold(apiVersion, helper.APIVersionV4Beta) {
				resp.Diagnostics.AddError(
					"API version mismatch",
					fmt.Sprintf(
						"tier: The api_version provider argument must be set to '%s' to use this field.",
						helper.APIVersionV4Beta,
					),
				)
				return
			}
		}
	}

	// Write plan modifications back.
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

// applyCaseInsensitiveSet returns newTags with the original casing from oldTags for
// tags that are equal case-insensitively, matching the SDKv2 CaseInsensitiveSet behaviour.
func applyCaseInsensitiveSet(ctx context.Context, oldTags, newTags types.Set, diags *diag.Diagnostics) types.Set {
	var oldSlice, newSlice []string
	diags.Append(oldTags.ElementsAs(ctx, &oldSlice, false)...)
	diags.Append(newTags.ElementsAs(ctx, &newSlice, false)...)
	if diags.HasError() {
		return newTags
	}

	oldMap := make(map[string]string, len(oldSlice))
	for _, t := range oldSlice {
		oldMap[strings.ToLower(t)] = t
	}

	result := make([]string, 0, len(newSlice))
	for _, t := range newSlice {
		if old, ok := oldMap[strings.ToLower(t)]; ok {
			result = append(result, old)
		} else {
			result = append(result, t)
		}
	}

	out, d := types.SetValueFrom(ctx, types.StringType, result)
	diags.Append(d...)
	return out
}

// --------------------------------------------------------------------------
// CRUD methods
// --------------------------------------------------------------------------

func (r *lkeClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lkeClusterModel
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

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{"cluster_id": plan.ID.ValueInt64()})
	tflog.Debug(ctx, "Create linode_lke_cluster")

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
		createOpts.StackType = linodego.Pointer(linodego.LKEClusterStackType(plan.StackType.ValueString()))
	}

	if plan.ControlPlane != nil {
		expanded, dd := expandControlPlaneFramework(ctx, plan.ControlPlane)
		resp.Diagnostics.Append(dd...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.ControlPlane = &expanded
	}

	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.Tags = tags
	}

	for _, pool := range plan.Pools {
		poolOpts, dd := expandPoolCreateOptionsFramework(ctx, pool)
		resp.Diagnostics.Append(dd...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOpts.NodePools = append(createOpts.NodePools, poolOpts)
	}

	tflog.Debug(ctx, "client.CreateLKECluster(...)", map[string]any{"options": createOpts})
	cluster, err := r.client.CreateLKECluster(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create LKE cluster", err.Error())
		return
	}
	plan.ID = types.Int64Value(int64(cluster.ID))

	// Enterprise clusters need extra time for kubeconfig to be ready.
	var retryTimeout time.Duration
	if cluster.Tier == TierEnterprise {
		retryTimeout = 120 * time.Second
		pollMS := r.meta.Config.EventPollMilliseconds.ValueInt64()
		if err := waitForLKEKubeconfigFramework(ctx, *r.client, pollMS, cluster.ID); err != nil {
			resp.Diagnostics.AddError("Failed waiting for LKE cluster kubeconfig", err.Error())
			return
		}
	} else {
		retryTimeout = 25 * time.Second
	}

	ctx = tflog.SetField(ctx, "cluster_id", cluster.ID)
	tflog.Debug(ctx, "Waiting for a single LKE cluster node to be ready")

	retryCtx, retryCancel := context.WithTimeout(ctx, retryTimeout)
	defer retryCancel()
	for {
		err := r.client.WaitForLKEClusterConditions(retryCtx, cluster.ID, linodego.LKEClusterPollOptions{
			TimeoutSeconds: 15 * 60,
		}, k8scondition.ClusterHasReadyNode)
		if err == nil {
			break
		}
		tflog.Debug(ctx, err.Error())
		if retryCtx.Err() != nil {
			break // Retry window exceeded; proceed to read.
		}
	}

	_, dd := r.refreshState(ctx, &plan)
	resp.Diagnostics.Append(dd...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *lkeClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lkeClusterModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = helper.SetLogFieldBulk(ctx, map[string]any{"cluster_id": state.ID.ValueInt64()})
	tflog.Debug(ctx, "Read linode_lke_cluster")

	gone, dd := r.refreshState(ctx, &state)
	resp.Diagnostics.Append(dd...)
	if resp.Diagnostics.HasError() {
		return
	}
	if gone {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *lkeClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state lkeClusterModel
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

	id := int(state.ID.ValueInt64())
	updateOpts := linodego.LKEClusterUpdateOptions{}
	changed := false

	if !plan.Label.Equal(state.Label) {
		updateOpts.Label = plan.Label.ValueString()
		changed = true
	}
	if !plan.K8sVersion.Equal(state.K8sVersion) {
		updateOpts.K8sVersion = plan.K8sVersion.ValueString()
		changed = true
	}

	if plan.ControlPlane != nil {
		expanded, dd := expandControlPlaneFramework(ctx, plan.ControlPlane)
		resp.Diagnostics.Append(dd...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.ControlPlane = &expanded
		changed = true
	}

	if !plan.Tags.Equal(state.Tags) {
		var tags []string
		resp.Diagnostics.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.Tags = &tags
		changed = true
	}

	if changed {
		tflog.Debug(ctx, "client.UpdateLKECluster(...)", map[string]any{"options": updateOpts})
		if _, err := r.client.UpdateLKECluster(ctx, id, updateOpts); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to update LKE Cluster %d", id), err.Error())
			return
		}
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	pools, err := r.client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to get Pools for LKE Cluster %d", id), err.Error())
		return
	}

	if !plan.K8sVersion.Equal(state.K8sVersion) {
		tflog.Debug(ctx, "Implicitly recycling LKE cluster to apply Kubernetes version upgrade")
		if err := recycleLKECluster(ctx, &helper.ProviderMeta{
			Client: *r.client,
			Config: frameworkConfigToProviderConfig(r.meta),
		}, id, pools); err != nil {
			resp.Diagnostics.AddError("Failed to recycle LKE cluster", err.Error())
			return
		}
	}

	cluster, err := r.client.GetLKECluster(ctx, id)
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

	oldSpecs := poolModelsToSpecs(state.Pools)
	newSpecs := poolModelsToSpecsPreserveNoTarget(plan.Pools)

	updates, err := ReconcileLKENodePoolSpecs(ctx, oldSpecs, newSpecs, enterprise)
	if err != nil {
		resp.Diagnostics.AddError("Failed to reconcile LKE cluster node pools", err.Error())
		return
	}

	tflog.Trace(ctx, "Reconciled LKE cluster node pool updates", map[string]any{"updates": updates})

	var updatedIDs []int

	for poolID, poolUpdateOpts := range updates.ToUpdate {
		tflog.Debug(ctx, "client.UpdateLKENodePool(...)", map[string]any{"node_pool_id": poolID, "options": poolUpdateOpts})
		if _, err := r.client.UpdateLKENodePool(ctx, id, poolID, poolUpdateOpts); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to update LKE Cluster %d Pool %d", id, poolID), err.Error())
			return
		}
		updatedIDs = append(updatedIDs, poolID)
	}

	for _, createOpts := range updates.ToCreate {
		tflog.Debug(ctx, "client.CreateLKENodePool(...)", map[string]any{"options": createOpts})
		pool, err := r.client.CreateLKENodePool(ctx, id, createOpts)
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to create LKE Cluster %d Pool", id), err.Error())
			return
		}
		updatedIDs = append(updatedIDs, pool.ID)
	}

	for _, poolID := range updates.ToDelete {
		tflog.Debug(ctx, "client.DeleteLKENodePool(...)", map[string]any{"node_pool_id": poolID})
		if err := r.client.DeleteLKENodePool(ctx, id, poolID); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to delete LKE Cluster %d Pool %d", id, poolID), err.Error())
			return
		}
	}

	tflog.Debug(ctx, "Waiting for all updated node pools to be ready")
	pollMS := int(r.meta.Config.LKENodeReadyPollMilliseconds.ValueInt64())
	for _, poolID := range updatedIDs {
		tflog.Trace(ctx, "Waiting for node pool to be ready", map[string]any{"node_pool_id": poolID})
		if _, err := lkenodepool.WaitForNodePoolReady(ctx, *r.client, pollMS, id, poolID); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Failed to wait for LKE Cluster %d pool %d ready", id, poolID), err.Error())
			return
		}
	}

	_, dd := r.refreshState(ctx, &plan)
	resp.Diagnostics.Append(dd...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *lkeClusterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Delete reads from req.State — req.Plan is null on Delete.
	var state lkeClusterModel
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

	id := int(state.ID.ValueInt64())
	skipDeletePoll := r.meta.Config.SkipLKEClusterDeletePoll.ValueBool()

	var oldNodes []linodego.LKENodePoolLinode
	if !skipDeletePoll {
		tflog.Trace(ctx, "client.ListLKENodePools(...)")
		apiPools, err := r.client.ListLKENodePools(ctx, id, nil)
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
	err := r.client.DeleteLKECluster(ctx, id)
	if err != nil && !linodego.IsNotFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed to delete Linode LKE cluster %d", id), err.Error())
		return
	}

	timeoutSeconds := int(deleteTimeout.Seconds())
	tflog.Debug(ctx, "Deleted LKE cluster, waiting for all nodes deleted...")
	tflog.Trace(ctx, "client.WaitForLKEClusterStatus(...)", map[string]any{
		"status":  "not_ready",
		"timeout": timeoutSeconds,
	})

	_, err = r.client.WaitForLKEClusterStatus(ctx, id, "not_ready", timeoutSeconds)
	if err != nil && !linodego.IsNotFound(err) {
		resp.Diagnostics.AddError("Failed waiting for LKE cluster deletion", err.Error())
		return
	}

	if !skipDeletePoll {
		pollMS := int(r.meta.Config.EventPollMilliseconds.ValueInt64())
		if err := waitForNodesDeleted(ctx, *r.client, pollMS, oldNodes); err != nil {
			resp.Diagnostics.AddError("Failed waiting for Linode instances to be deleted", err.Error())
			return
		}
	}
}

// --------------------------------------------------------------------------
// refreshState — reads current API state into the model.
// Returns (gone bool, diags).
// --------------------------------------------------------------------------

func (r *lkeClusterResource) refreshState(ctx context.Context, model *lkeClusterModel) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	id := int(model.ID.ValueInt64())

	cluster, err := r.client.GetLKECluster(ctx, id)
	if err != nil {
		if linodego.IsNotFound(err) {
			log.Printf("[WARN] removing LKE Cluster ID %d from state because it no longer exists", id)
			return true, diags
		}
		diags.AddError(fmt.Sprintf("Failed to get LKE cluster %d", id), err.Error())
		return false, diags
	}

	tflog.Trace(ctx, "client.ListLKENodePools(...)")
	apiPools, err := r.client.ListLKENodePools(ctx, id, nil)
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to get pools for LKE cluster %d", id), err.Error())
		return false, diags
	}

	var externalPoolTags []string
	if !model.ExternalPoolTags.IsNull() && !model.ExternalPoolTags.IsUnknown() {
		diags.Append(model.ExternalPoolTags.ElementsAs(ctx, &externalPoolTags, false)...)
	}
	if len(externalPoolTags) > 0 && len(apiPools) > 0 {
		apiPools = filterExternalPools(ctx, externalPoolTags, apiPools)
	}

	kubeconfig, err := r.client.GetLKEClusterKubeconfig(ctx, id)
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to get kubeconfig for LKE cluster %d", id), err.Error())
		return false, diags
	}

	tflog.Trace(ctx, "client.ListLKEClusterAPIEndpoints(...)")
	endpoints, err := r.client.ListLKEClusterAPIEndpoints(ctx, id, nil)
	if err != nil {
		diags.AddError(fmt.Sprintf("Failed to get API endpoints for LKE cluster %d", id), err.Error())
		return false, diags
	}

	acl, err := r.client.GetLKEClusterControlPlaneACL(ctx, id)
	if err != nil {
		if lerr, ok := err.(*linodego.Error); !ok ||
			(lerr.Code != 404 && !(lerr.Code == 400 && strings.Contains(lerr.Message, "Cluster does not support Control Plane ACL"))) {
			diags.AddError(fmt.Sprintf("Failed to get control plane ACL for LKE cluster %d", id), err.Error())
			return false, diags
		}
		acl = nil
	}

	// Dashboard URL (standard tier only).
	if cluster.Tier == TierStandard {
		dashboard, err := r.client.GetLKEClusterDashboard(ctx, id)
		if err != nil {
			diags.AddError(fmt.Sprintf("Failed to get dashboard URL for LKE cluster %d", id), err.Error())
			return false, diags
		}
		model.DashboardURL = types.StringValue(dashboard.URL)
	}

	// Scalar fields.
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

	// Tags.
	tagSet, dd := types.SetValueFrom(ctx, types.StringType, cluster.Tags)
	diags.Append(dd...)
	model.Tags = tagSet

	// API endpoints.
	flatEndpoints := flattenLKEClusterAPIEndpoints(endpoints)
	endpointList, dd := types.ListValueFrom(ctx, types.StringType, flatEndpoints)
	diags.Append(dd...)
	model.APIEndpoints = endpointList

	// Match API pools to declared pools (preserves ordering).
	matchedPools := matchPoolsFramework(ctx, apiPools, model.Pools, &diags)
	if diags.HasError() {
		return false, diags
	}
	model.Pools = flattenAPIPoolsFramework(ctx, matchedPools, &diags)

	// Control plane.
	model.ControlPlane = flattenControlPlaneFramework(ctx, cluster.ControlPlane, acl, &diags)

	return false, diags
}

// --------------------------------------------------------------------------
// Pool matching (pure-Go replacement for matchPoolsWithSchema)
// --------------------------------------------------------------------------

// matchPoolsFramework matches API pools to declared model pools to preserve
// ordering stable across refreshes. It is the framework equivalent of
// matchPoolsWithSchema (which uses SDKv2 *schema.Set types).
func matchPoolsFramework(ctx context.Context, apiPools []linodego.LKENodePool, declared []lkePoolModel, diags *diag.Diagnostics) []linodego.LKENodePool {
	result := make([]linodego.LKENodePool, len(declared))

	// index all api pools by ID for fast lookup.
	apiByID := make(map[int]linodego.LKENodePool, len(apiPools))
	for _, p := range apiPools {
		apiByID[p.ID] = p
	}

	paired := make(map[int]bool) // api pool IDs already matched

	// Pass 1: match by ID.
	for i, d := range declared {
		did := int(d.ID.ValueInt64())
		if did == 0 {
			continue
		}
		if ap, ok := apiByID[did]; ok {
			result[i] = ap
			paired[ap.ID] = true
		}
	}

	// Pass 2: match by type+count+autoscaler+tags (for new pools without an ID).
	for i, d := range declared {
		if d.ID.ValueInt64() != 0 {
			continue // already handled above
		}

		var declaredTags []string
		if !d.Tags.IsNull() {
			diags.Append(d.Tags.ElementsAs(ctx, &declaredTags, false)...)
		}

		declaredCount := int(d.Count.ValueInt64())
		var declaredAutoMin, declaredAutoMax int
		autoscalerEnabled := len(d.Autoscaler) > 0
		if autoscalerEnabled {
			declaredAutoMin = int(d.Autoscaler[0].Min.ValueInt64())
			declaredAutoMax = int(d.Autoscaler[0].Max.ValueInt64())
			if declaredCount == 0 {
				declaredCount = declaredAutoMin
			}
		}

		for _, ap := range apiPools {
			if paired[ap.ID] {
				continue
			}
			if ap.Type != d.Type.ValueString() {
				continue
			}
			if ap.Count != declaredCount {
				continue
			}
			if ap.Autoscaler.Enabled != autoscalerEnabled {
				continue
			}
			if autoscalerEnabled {
				if ap.Autoscaler.Min != declaredAutoMin || ap.Autoscaler.Max != declaredAutoMax {
					continue
				}
			}
			if !stringSliceSetsEqual(declaredTags, ap.Tags) {
				continue
			}

			result[i] = ap
			paired[ap.ID] = true
			break
		}
	}

	// Append unmatched API pools (typically pools about to be deleted).
	for _, ap := range apiPools {
		if !paired[ap.ID] {
			result = append(result, ap)
		}
	}

	return result
}

// stringSliceSetsEqual returns true if a and b contain the same elements (order-insensitive).
func stringSliceSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]int, len(a))
	for _, v := range a {
		m[v]++
	}
	for _, v := range b {
		m[v]--
		if m[v] < 0 {
			return false
		}
	}
	return true
}

// --------------------------------------------------------------------------
// Flatten helpers
// --------------------------------------------------------------------------

func flattenAPIPoolsFramework(ctx context.Context, apiPools []linodego.LKENodePool, diags *diag.Diagnostics) []lkePoolModel {
	result := make([]lkePoolModel, len(apiPools))
	for i, ap := range apiPools {
		nodes := make([]lkeNodeModel, len(ap.Linodes))
		for j, n := range ap.Linodes {
			nodes[j] = lkeNodeModel{
				ID:         types.StringValue(n.ID),
				InstanceID: types.Int64Value(int64(n.InstanceID)),
				Status:     types.StringValue(string(n.Status)),
			}
		}

		var autoscaler []lkeAutoscalerModel
		if ap.Autoscaler.Enabled {
			autoscaler = []lkeAutoscalerModel{{
				Min: types.Int64Value(int64(ap.Autoscaler.Min)),
				Max: types.Int64Value(int64(ap.Autoscaler.Max)),
			}}
		}

		taints := make([]lkeTaintModel, len(ap.Taints))
		for j, t := range ap.Taints {
			taints[j] = lkeTaintModel{
				Effect: types.StringValue(string(t.Effect)),
				Key:    types.StringValue(t.Key),
				Value:  types.StringValue(t.Value),
			}
		}

		labelsMap := make(map[string]string, len(ap.Labels))
		for k, v := range ap.Labels {
			labelsMap[k] = v
		}
		labelsAttr, dd := types.MapValueFrom(ctx, types.StringType, labelsMap)
		diags.Append(dd...)

		tagsAttr, dd := types.SetValueFrom(ctx, types.StringType, ap.Tags)
		diags.Append(dd...)

		var label types.String
		if ap.Label != nil {
			label = types.StringValue(*ap.Label)
		} else {
			label = types.StringValue("")
		}

		var firewallID types.Int64
		if ap.FirewallID != nil {
			firewallID = types.Int64Value(int64(*ap.FirewallID))
		} else {
			firewallID = types.Int64Value(0)
		}

		var k8sVersion types.String
		if ap.K8sVersion != nil {
			k8sVersion = types.StringValue(*ap.K8sVersion)
		} else {
			k8sVersion = types.StringNull()
		}

		var updateStrategy types.String
		if ap.UpdateStrategy != nil {
			updateStrategy = types.StringValue(string(*ap.UpdateStrategy))
		} else {
			updateStrategy = types.StringNull()
		}

		result[i] = lkePoolModel{
			ID:             types.Int64Value(int64(ap.ID)),
			Label:          label,
			Count:          types.Int64Value(int64(ap.Count)),
			Type:           types.StringValue(ap.Type),
			Tags:           tagsAttr,
			DiskEncryption: types.StringValue(string(ap.DiskEncryption)),
			Taints:         taints,
			Labels:         labelsAttr,
			Nodes:          nodes,
			Autoscaler:     autoscaler,
			K8sVersion:     k8sVersion,
			UpdateStrategy: updateStrategy,
			FirewallID:     firewallID,
		}
	}
	return result
}

func flattenControlPlaneFramework(
	ctx context.Context,
	cp linodego.LKEClusterControlPlane,
	aclResp *linodego.LKEClusterControlPlaneACLResponse,
	diags *diag.Diagnostics,
) *lkeControlPlaneModel {
	model := &lkeControlPlaneModel{
		HighAvailability: types.BoolValue(cp.HighAvailability),
		AuditLogsEnabled: types.BoolValue(cp.AuditLogsEnabled),
	}

	if aclResp != nil {
		acl := aclResp.ACL
		aclModel := lkeACLModel{
			Enabled: types.BoolValue(acl.Enabled),
		}

		if acl.Addresses != nil {
			ipv4, dd := types.SetValueFrom(ctx, types.StringType, acl.Addresses.IPv4)
			diags.Append(dd...)
			ipv6, dd := types.SetValueFrom(ctx, types.StringType, acl.Addresses.IPv6)
			diags.Append(dd...)
			aclModel.Addresses = []lkeACLAddressesModel{{IPv4: ipv4, IPv6: ipv6}}
		}
		model.ACL = []lkeACLModel{aclModel}
	}

	return model
}

// --------------------------------------------------------------------------
// Expand helpers
// --------------------------------------------------------------------------

func expandControlPlaneFramework(ctx context.Context, cp *lkeControlPlaneModel) (linodego.LKEClusterControlPlaneOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	var result linodego.LKEClusterControlPlaneOptions

	if cp == nil {
		return result, diags
	}

	if !cp.HighAvailability.IsNull() && !cp.HighAvailability.IsUnknown() {
		v := cp.HighAvailability.ValueBool()
		result.HighAvailability = &v
	}
	if !cp.AuditLogsEnabled.IsNull() && !cp.AuditLogsEnabled.IsUnknown() {
		v := cp.AuditLogsEnabled.ValueBool()
		result.AuditLogsEnabled = &v
	}

	// Default to ACL disabled.
	disabled := false
	result.ACL = &linodego.LKEClusterControlPlaneACLOptions{Enabled: &disabled}

	if len(cp.ACL) > 0 {
		aclM := cp.ACL[0]
		aclOpts := &linodego.LKEClusterControlPlaneACLOptions{}

		if !aclM.Enabled.IsNull() && !aclM.Enabled.IsUnknown() {
			v := aclM.Enabled.ValueBool()
			aclOpts.Enabled = &v
		}

		if len(aclM.Addresses) > 0 {
			addrM := aclM.Addresses[0]
			addrOpts := &linodego.LKEClusterControlPlaneACLAddressesOptions{}
			if !addrM.IPv4.IsNull() && !addrM.IPv4.IsUnknown() {
				var ipv4 []string
				diags.Append(addrM.IPv4.ElementsAs(ctx, &ipv4, false)...)
				addrOpts.IPv4 = &ipv4
			}
			if !addrM.IPv6.IsNull() && !addrM.IPv6.IsUnknown() {
				var ipv6 []string
				diags.Append(addrM.IPv6.ElementsAs(ctx, &ipv6, false)...)
				addrOpts.IPv6 = &ipv6
			}
			aclOpts.Addresses = addrOpts
		}

		// Validation: addresses not acceptable when ACL disabled.
		if aclOpts.Enabled != nil && !*aclOpts.Enabled &&
			aclOpts.Addresses != nil &&
			((aclOpts.Addresses.IPv4 != nil && len(*aclOpts.Addresses.IPv4) > 0) ||
				(aclOpts.Addresses.IPv6 != nil && len(*aclOpts.Addresses.IPv6) > 0)) {
			diags.AddError("Invalid ACL configuration", "addresses are not acceptable when ACL is disabled")
			return result, diags
		}

		result.ACL = aclOpts
	}

	return result, diags
}

func expandPoolCreateOptionsFramework(ctx context.Context, pool lkePoolModel) (linodego.LKENodePoolCreateOptions, diag.Diagnostics) {
	var diags diag.Diagnostics
	opts := linodego.LKENodePoolCreateOptions{
		Type: pool.Type.ValueString(),
	}

	if !pool.Count.IsNull() && !pool.Count.IsUnknown() {
		opts.Count = int(pool.Count.ValueInt64())
	}

	if !pool.Tags.IsNull() && !pool.Tags.IsUnknown() {
		var tags []string
		diags.Append(pool.Tags.ElementsAs(ctx, &tags, false)...)
		opts.Tags = tags
	}

	if !pool.Labels.IsNull() && !pool.Labels.IsUnknown() {
		var labelsMap map[string]string
		diags.Append(pool.Labels.ElementsAs(ctx, &labelsMap, false)...)
		opts.Labels = linodego.LKENodePoolLabels(labelsMap)
	}

	taints := make([]linodego.LKENodePoolTaint, len(pool.Taints))
	for i, t := range pool.Taints {
		taints[i] = linodego.LKENodePoolTaint{
			Key:    t.Key.ValueString(),
			Value:  t.Value.ValueString(),
			Effect: linodego.LKENodePoolTaintEffect(t.Effect.ValueString()),
		}
	}
	opts.Taints = taints

	if !pool.Label.IsNull() && !pool.Label.IsUnknown() && pool.Label.ValueString() != "" {
		v := pool.Label.ValueString()
		opts.Label = &v
	}
	if !pool.FirewallID.IsNull() && !pool.FirewallID.IsUnknown() && pool.FirewallID.ValueInt64() != 0 {
		v := int(pool.FirewallID.ValueInt64())
		opts.FirewallID = &v
	}

	if len(pool.Autoscaler) > 0 {
		as := pool.Autoscaler[0]
		opts.Autoscaler = &linodego.LKENodePoolAutoscaler{
			Enabled: true,
			Min:     int(as.Min.ValueInt64()),
			Max:     int(as.Max.ValueInt64()),
		}
		if opts.Count == 0 {
			opts.Count = int(as.Min.ValueInt64())
		}
	}

	if !pool.K8sVersion.IsNull() && !pool.K8sVersion.IsUnknown() && pool.K8sVersion.ValueString() != "" {
		v := pool.K8sVersion.ValueString()
		opts.K8sVersion = &v
	}
	if !pool.UpdateStrategy.IsNull() && !pool.UpdateStrategy.IsUnknown() && pool.UpdateStrategy.ValueString() != "" {
		v := linodego.LKENodePoolUpdateStrategy(pool.UpdateStrategy.ValueString())
		opts.UpdateStrategy = &v
	}

	return opts, diags
}

// --------------------------------------------------------------------------
// NodePoolSpec conversion (for ReconcileLKENodePoolSpecs in cluster.go)
// --------------------------------------------------------------------------

// poolModelsToSpecs converts model pools to NodePoolSpec for reconciliation.
// Only pools with a non-zero ID (existing) are included.
func poolModelsToSpecs(pools []lkePoolModel) []NodePoolSpec {
	specs := make([]NodePoolSpec, 0, len(pools))
	for _, p := range pools {
		id := int(p.ID.ValueInt64())
		if id == 0 {
			continue
		}
		specs = append(specs, poolModelToSpec(p, id))
	}
	return specs
}

// poolModelsToSpecsPreserveNoTarget converts model pools for the new-spec side;
// pools without an ID (newly declared) are still included.
func poolModelsToSpecsPreserveNoTarget(pools []lkePoolModel) []NodePoolSpec {
	specs := make([]NodePoolSpec, 0, len(pools))
	for _, p := range pools {
		specs = append(specs, poolModelToSpec(p, int(p.ID.ValueInt64())))
	}
	return specs
}

func poolModelToSpec(p lkePoolModel, id int) NodePoolSpec {
	spec := NodePoolSpec{
		ID:    id,
		Type:  p.Type.ValueString(),
		Count: int(p.Count.ValueInt64()),
	}

	if !p.Label.IsNull() && !p.Label.IsUnknown() && p.Label.ValueString() != "" {
		v := p.Label.ValueString()
		spec.Label = &v
	}
	if !p.FirewallID.IsNull() && !p.FirewallID.IsUnknown() && p.FirewallID.ValueInt64() != 0 {
		v := int(p.FirewallID.ValueInt64())
		spec.FirewallID = &v
	}

	if !p.Tags.IsNull() {
		var tags []string
		_ = p.Tags.ElementsAs(context.Background(), &tags, false)
		spec.Tags = tags
	}

	taints := make([]map[string]any, len(p.Taints))
	for i, t := range p.Taints {
		taints[i] = map[string]any{
			"key":    t.Key.ValueString(),
			"value":  t.Value.ValueString(),
			"effect": t.Effect.ValueString(),
		}
	}
	spec.Taints = taints

	if !p.Labels.IsNull() {
		var labelsMap map[string]string
		_ = p.Labels.ElementsAs(context.Background(), &labelsMap, false)
		spec.Labels = labelsMap
	}

	if len(p.Autoscaler) > 0 {
		spec.AutoScalerEnabled = true
		spec.AutoScalerMin = int(p.Autoscaler[0].Min.ValueInt64())
		spec.AutoScalerMax = int(p.Autoscaler[0].Max.ValueInt64())
	}

	if !p.K8sVersion.IsNull() && !p.K8sVersion.IsUnknown() {
		v := p.K8sVersion.ValueString()
		spec.K8sVersion = &v
	}
	if !p.UpdateStrategy.IsNull() && !p.UpdateStrategy.IsUnknown() {
		v := p.UpdateStrategy.ValueString()
		spec.UpdateStrategy = &v
	}

	return spec
}

// --------------------------------------------------------------------------
// Misc helpers
// --------------------------------------------------------------------------

// frameworkConfigToProviderConfig bridges FrameworkProviderMeta → ProviderMeta.Config.
// Needed because cluster.go's recycleLKECluster still accepts *helper.ProviderMeta.
func frameworkConfigToProviderConfig(meta *helper.FrameworkProviderMeta) *helper.Config {
	return &helper.Config{
		EventPollMilliseconds:        int(meta.Config.EventPollMilliseconds.ValueInt64()),
		LKEEventPollMilliseconds:     int(meta.Config.LKEEventPollMilliseconds.ValueInt64()),
		LKENodeReadyPollMilliseconds: int(meta.Config.LKENodeReadyPollMilliseconds.ValueInt64()),
		SkipLKEClusterDeletePoll:     meta.Config.SkipLKEClusterDeletePoll.ValueBool(),
	}
}

// waitForLKEKubeconfigFramework polls until the LKE cluster kubeconfig is available.
// Duplicates the logic from cluster.go but without importing SDKv2.
func waitForLKEKubeconfigFramework(ctx context.Context, client linodego.Client, intervalMS int64, clusterID int) error {
	if intervalMS == 0 {
		intervalMS = 500
	}
	ticker := time.NewTicker(time.Duration(intervalMS) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := client.GetLKEClusterKubeconfig(ctx, clusterID)
			if err != nil {
				if strings.Contains(err.Error(), "Cluster kubeconfig is not yet available") {
					continue
				}
				return fmt.Errorf("failed to get Kubeconfig for LKE cluster %d: %w", clusterID, err)
			}
			return nil
		case <-ctx.Done():
			return fmt.Errorf("error waiting for Cluster %d kubeconfig: %w", clusterID, ctx.Err())
		}
	}
}

