package objbucket

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// Ensure the resource satisfies the required framework interfaces.
var (
	_ resource.Resource                = &BucketResource{}
	_ resource.ResourceWithConfigure   = &BucketResource{}
	_ resource.ResourceWithImportState = &BucketResource{}
)

// NewResource returns a new framework Resource for linode_object_storage_bucket.
func NewResource() resource.Resource {
	return &BucketResource{}
}

// BucketResource implements the framework resource for linode_object_storage_bucket.
type BucketResource struct {
	Meta *helper.FrameworkProviderMeta
}

// ---------------------------------------------------------------------------
// Model structs
// ---------------------------------------------------------------------------

// bucketKeys holds resolved S3 access/secret key pair.
type bucketKeys struct {
	AccessKey string
	SecretKey string
}

func (k bucketKeys) ok() bool {
	return k.AccessKey != "" && k.SecretKey != ""
}

// BucketCertModel maps to the `cert` SingleNestedBlock.
type BucketCertModel struct {
	Certificate types.String `tfsdk:"certificate"`
	PrivateKey  types.String `tfsdk:"private_key"`
}

// ExpirationModel maps to `expiration` sub-block inside lifecycle_rule.
type ExpirationModel struct {
	Date                      types.String `tfsdk:"date"`
	Days                      types.Int64  `tfsdk:"days"`
	ExpiredObjectDeleteMarker types.Bool   `tfsdk:"expired_object_delete_marker"`
}

// NoncurrentVersionExpirationModel maps to `noncurrent_version_expiration` block.
type NoncurrentVersionExpirationModel struct {
	Days types.Int64 `tfsdk:"days"`
}

// LifecycleRuleModel maps to one element of the `lifecycle_rule` ListNestedBlock.
type LifecycleRuleModel struct {
	ID                                 types.String                       `tfsdk:"id"`
	Prefix                             types.String                       `tfsdk:"prefix"`
	Enabled                            types.Bool                         `tfsdk:"enabled"`
	AbortIncompleteMultipartUploadDays types.Int64                        `tfsdk:"abort_incomplete_multipart_upload_days"`
	Expiration                         []ExpirationModel                  `tfsdk:"expiration"`
	NoncurrentVersionExpiration        []NoncurrentVersionExpirationModel `tfsdk:"noncurrent_version_expiration"`
}

// BucketResourceModel is the full state / plan model for the resource.
type BucketResourceModel struct {
	ID            types.String         `tfsdk:"id"`
	Label         types.String         `tfsdk:"label"`
	Cluster       types.String         `tfsdk:"cluster"`
	Region        types.String         `tfsdk:"region"`
	Endpoint      types.String         `tfsdk:"endpoint"`
	S3Endpoint    types.String         `tfsdk:"s3_endpoint"`
	EndpointType  types.String         `tfsdk:"endpoint_type"`
	Hostname      types.String         `tfsdk:"hostname"`
	ACL           types.String         `tfsdk:"acl"`
	CorsEnabled   types.Bool           `tfsdk:"cors_enabled"`
	Versioning    types.Bool           `tfsdk:"versioning"`
	AccessKey     types.String         `tfsdk:"access_key"`
	SecretKey     types.String         `tfsdk:"secret_key"`
	Cert          *BucketCertModel     `tfsdk:"cert"`
	LifecycleRule []LifecycleRuleModel `tfsdk:"lifecycle_rule"`
}

// ---------------------------------------------------------------------------
// ResourceWithConfigure
// ---------------------------------------------------------------------------

func (r *BucketResource) Configure(
	ctx context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	r.Meta = helper.GetResourceMeta(req, resp)
}

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (r *BucketResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = "linode_object_storage_bucket"
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

func (r *BucketResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the Linode Object Storage Bucket (format: <cluster_or_region>:<label>).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"label": schema.StringAttribute{
				Required:    true,
				Description: "The label of the Linode Object Storage Bucket.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cluster": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The cluster of the Linode Object Storage Bucket.",
				DeprecationMessage: "The cluster attribute has been deprecated, please consider switching to the region attribute. " +
					"For example, a cluster value of `us-mia-1` can be translated to a region value of `us-mia`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The region of the Linode Object Storage Bucket.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint": schema.StringAttribute{
				Computed:           true,
				Description:        "The endpoint for the bucket used for s3 connections.",
				DeprecationMessage: "Use `s3_endpoint` instead",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"s3_endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The endpoint for the bucket used for s3 connections.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The type of the S3 endpoint available in this region.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Computed:    true,
				Description: "The hostname where this bucket can be accessed. This hostname can be accessed through a browser if the bucket is made public.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"acl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("private"),
				Description: "The Access Control Level of the bucket using a canned ACL string.",
			},
			"cors_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "If true, the bucket will be created with CORS enabled for all origins.",
			},
			"versioning": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether to enable versioning.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"access_key": schema.StringAttribute{
				Optional: true,
				Description: "The S3 access key to use for this resource. (Required for lifecycle_rule and versioning). " +
					"If not specified with the resource, the value will be read from provider-level obj_access_key, " +
					"or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			},
			"secret_key": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "The S3 secret key to use for this resource. (Required for lifecycle_rule and versioning). " +
					"If not specified with the resource, the value will be read from provider-level obj_secret_key, " +
					"or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			},
		},
		Blocks: map[string]schema.Block{
			// cert: MaxItems:1 — keep as SingleNestedBlock for HCL backward-compat.
			"cert": schema.SingleNestedBlock{
				Description: "The cert used by this Object Storage Bucket.",
				Attributes: map[string]schema.Attribute{
					"certificate": schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						Description: "The Base64 encoded and PEM formatted SSL certificate.",
					},
					"private_key": schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						Description: "The private key associated with the TLS/SSL certificate.",
					},
				},
			},
			// lifecycle_rule: no MaxItems → ListNestedBlock (practitioners use block syntax).
			"lifecycle_rule": schema.ListNestedBlock{
				Description: "Lifecycle rules to be applied to the bucket.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Description: "The unique identifier for the rule.",
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"prefix": schema.StringAttribute{
							Optional:    true,
							Description: "The object key prefix identifying one or more objects to which the rule applies.",
						},
						"enabled": schema.BoolAttribute{
							Required:    true,
							Description: "Specifies whether the lifecycle rule is active.",
						},
						"abort_incomplete_multipart_upload_days": schema.Int64Attribute{
							Optional: true,
							Description: "Specifies the number of days after initiating a multipart upload when the " +
								"multipart upload must be completed.",
						},
					},
					Blocks: map[string]schema.Block{
						// expiration: MaxItems:1 → ListNestedBlock + SizeAtMost(1)
						// (keeps block syntax; preserves `lifecycle_rule.0.expiration.0.*` state path)
						"expiration": schema.ListNestedBlock{
							Description: "Specifies a period in the object's expire.",
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"date": schema.StringAttribute{
										Optional:    true,
										Description: "Specifies the date after which you want the corresponding action to take effect.",
									},
									"days": schema.Int64Attribute{
										Optional:    true,
										Description: "Specifies the number of days after object creation when the specific rule action takes effect.",
									},
									"expired_object_delete_marker": schema.BoolAttribute{
										Optional:    true,
										Computed:    true,
										Default:     booldefault.StaticBool(false),
										Description: "Directs Linode Object Storage to remove expired deleted markers.",
									},
								},
							},
						},
						// noncurrent_version_expiration: MaxItems:1 → ListNestedBlock + SizeAtMost(1)
						"noncurrent_version_expiration": schema.ListNestedBlock{
							Description: "Specifies when non-current object versions expire.",
							Validators: []validator.List{
								listvalidator.SizeAtMost(1),
							},
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"days": schema.Int64Attribute{
										Required:    true,
										Description: "Specifies the number of days non-current object versions expire.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// ImportState — composite ID "clusterOrRegion:label"
// ---------------------------------------------------------------------------

// ImportState handles `terraform import linode_object_storage_bucket.x clusterOrRegion:label`.
// The full composite ID is stored in `id`; Read will parse and populate the rest.
func (r *BucketResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected a non-empty import ID of the form <cluster_or_region>:<label>.",
		)
		return
	}
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected <cluster_or_region>:<label>, got %q.", req.ID),
		)
		return
	}
	// Write the full composite ID so Read can locate the resource.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *BucketResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_object_storage_bucket")

	var plan BucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, plan)
	client := r.Meta.Client

	// Validate region if set.
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
		r.validateRegion(ctx, plan.Region.ValueString(), client, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	createOpts := linodego.ObjectStorageBucketCreateOptions{
		Label: plan.Label.ValueString(),
		ACL:   linodego.ObjectStorageACL(plan.ACL.ValueString()),
	}

	if !plan.CorsEnabled.IsNull() && !plan.CorsEnabled.IsUnknown() {
		v := plan.CorsEnabled.ValueBool()
		createOpts.CorsEnabled = &v
	}
	if !plan.S3Endpoint.IsNull() && !plan.S3Endpoint.IsUnknown() {
		createOpts.S3Endpoint = plan.S3Endpoint.ValueString()
	}
	if !plan.EndpointType.IsNull() && !plan.EndpointType.IsUnknown() {
		createOpts.EndpointType = linodego.ObjectStorageEndpointType(plan.EndpointType.ValueString())
	}
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() {
		createOpts.Region = plan.Region.ValueString()
	}
	if !plan.Cluster.IsNull() && !plan.Cluster.IsUnknown() {
		createOpts.Cluster = plan.Cluster.ValueString()
	}

	tflog.Debug(ctx, "client.CreateObjectStorageBucket(...)", map[string]any{"options": createOpts})
	bucket, err := client.CreateObjectStorageBucket(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create Object Storage Bucket", err.Error())
		return
	}

	endpoint := getS3Endpoint(ctx, *bucket)
	plan.Endpoint = types.StringValue(endpoint)
	plan.S3Endpoint = types.StringValue(endpoint)

	if bucket.Region != "" {
		plan.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
	} else {
		plan.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
	}

	// Write early state so drift is captured even if subsequent steps fail.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply access, cert, versioning, lifecycle, then re-read.
	// For Create there is no prior cert (nil).
	r.applyUpdates(ctx, &plan, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (r *BucketResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_object_storage_bucket")

	var state BucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, state)
	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster, label, err := decodeBucketIDFromModel(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to parse Object Storage Bucket ID",
			fmt.Sprintf("id %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	bucket, err := client.GetObjectStorageBucket(ctx, regionOrCluster, label)
	if err != nil {
		if linodego.IsNotFound(err) {
			tflog.Warn(ctx, fmt.Sprintf(
				"[WARN] removing Object Storage Bucket %q from state because it no longer exists",
				state.ID.ValueString(),
			))
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to find the specified Linode ObjectStorageBucket", err.Error())
		return
	}

	tflog.Debug(ctx, "getting bucket access info")
	access, err := client.GetObjectStorageBucketAccessV2(ctx, regionOrCluster, label)
	if err != nil {
		resp.Diagnostics.AddError("Failed to find the access config for the specified Linode ObjectStorageBucket", err.Error())
		return
	}

	hasVersioning := !state.Versioning.IsNull()
	hasLifecycle := len(state.LifecycleRule) > 0

	if hasVersioning || hasLifecycle {
		tflog.Debug(ctx, "versioning or lifecycle present; fetching S3 data", map[string]any{
			"versioningPresent": hasVersioning,
			"lifecyclePresent":  hasLifecycle,
		})

		objKeys, teardown := r.resolveObjKeys(ctx, state, client, config, label, regionOrCluster, "read_only", &bucket.EndpointType, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		s3endpoint := getS3Endpoint(ctx, *bucket)
		s3Client, err := helper.S3Connection(ctx, s3endpoint, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to create S3 connection", err.Error())
			return
		}

		if hasLifecycle {
			tflog.Trace(ctx, "getting bucket lifecycle")
			r.readBucketLifecycle(ctx, &state, s3Client, &resp.Diagnostics)
			if resp.Diagnostics.HasError() {
				return
			}
		}

		if hasVersioning {
			tflog.Trace(ctx, "getting bucket versioning")
			r.readBucketVersioning(ctx, &state, s3Client, &resp.Diagnostics)
			if resp.Diagnostics.HasError() {
				return
			}
		}
	}

	// Update ID.
	if bucket.Region != "" {
		state.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
	} else {
		state.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
	}

	ep := getS3Endpoint(ctx, *bucket)
	state.Cluster = types.StringValue(bucket.Cluster)
	state.Region = types.StringValue(bucket.Region)
	state.Label = types.StringValue(bucket.Label)
	state.Hostname = types.StringValue(bucket.Hostname)
	state.ACL = types.StringValue(string(access.ACL))
	state.CorsEnabled = types.BoolValue(access.CorsEnabled)
	state.Endpoint = types.StringValue(ep)
	state.S3Endpoint = types.StringValue(ep)
	state.EndpointType = types.StringValue(string(bucket.EndpointType))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *BucketResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_object_storage_bucket")

	var plan, state BucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, plan)

	// Preserve the composite ID from state.
	plan.ID = state.ID

	if !plan.Region.Equal(state.Region) {
		if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
			r.validateRegion(ctx, plan.Region.ValueString(), r.Meta.Client, &resp.Diagnostics)
			if resp.Diagnostics.HasError() {
				return
			}
		}
	}

	// Pass old cert from state so updateBucketCert can delete it when removed.
	r.applyUpdates(ctx, &plan, state.Cert, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func (r *BucketResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_object_storage_bucket")

	// Delete reads from req.State — req.Plan is null on Delete.
	var state BucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, state)
	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster, label, err := decodeBucketIDFromModel(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error parsing Linode ObjectStorageBucket id",
			fmt.Sprintf("id %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	if config.ObjBucketForceDelete.ValueBool() {
		var endpointTypePtr *linodego.ObjectStorageEndpointType
		if !state.EndpointType.IsNull() && !state.EndpointType.IsUnknown() {
			et := linodego.ObjectStorageEndpointType(state.EndpointType.ValueString())
			endpointTypePtr = &et
		}

		objKeys, teardown := r.resolveObjKeys(ctx, state, client, config, label, regionOrCluster, "read_write", endpointTypePtr, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		s3Client, err := r.s3ConnectionFromModel(ctx, state, client, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to create S3 connection for force-delete", err.Error())
			return
		}

		tflog.Debug(ctx, "helper.PurgeAllObjects(...)")
		if err := helper.PurgeAllObjects(ctx, label, s3Client, true, true); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error purging all objects from ObjectStorageBucket %s", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	tflog.Debug(ctx, "client.DeleteObjectStorageBucket(...)")
	if err := client.DeleteObjectStorageBucket(ctx, regionOrCluster, label); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting Linode ObjectStorageBucket %s", state.ID.ValueString()),
			err.Error(),
		)
	}
}

// ---------------------------------------------------------------------------
// applyUpdates — shared between Create (post-create) and Update
// ---------------------------------------------------------------------------

// applyUpdates applies access, cert, versioning, and lifecycle changes, then
// re-reads the resource to populate computed values back into plan.
// oldCert is the cert from prior state (nil on Create or when cert was never set).
func (r *BucketResource) applyUpdates(
	ctx context.Context,
	plan *BucketResourceModel,
	oldCert *BucketCertModel,
	diags *diag.Diagnostics,
) {
	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster, label, err := decodeBucketIDFromModel(ctx, *plan)
	if err != nil {
		diags.AddError("Failed to parse Object Storage Bucket ID during update", err.Error())
		return
	}

	// Update access (ACL, CORS).
	if err := r.updateBucketAccess(ctx, *plan, client, regionOrCluster, label); err != nil {
		diags.AddError("Failed to update bucket access", err.Error())
		return
	}

	// Update cert — pass old cert so we can delete it if removed.
	if err := r.updateBucketCert(ctx, *plan, oldCert, client, regionOrCluster, label); err != nil {
		diags.AddError("Failed to update bucket certificate", err.Error())
		return
	}

	hasVersioning := !plan.Versioning.IsNull() && !plan.Versioning.IsUnknown()
	hasLifecycle := len(plan.LifecycleRule) > 0

	if hasVersioning || hasLifecycle {
		var endpointTypePtr *linodego.ObjectStorageEndpointType
		if !plan.EndpointType.IsNull() && !plan.EndpointType.IsUnknown() {
			et := linodego.ObjectStorageEndpointType(plan.EndpointType.ValueString())
			endpointTypePtr = &et
		}

		objKeys, teardown := r.resolveObjKeys(ctx, *plan, client, config, label, regionOrCluster, "read_write", endpointTypePtr, diags)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		s3Client, err := r.s3ConnectionFromModel(ctx, *plan, client, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			diags.AddError("Failed to create S3 connection", err.Error())
			return
		}

		if hasVersioning {
			tflog.Debug(ctx, "Updating bucket versioning configuration")
			if err := r.updateBucketVersioning(ctx, *plan, s3Client); err != nil {
				diags.AddError("Failed to update bucket versioning", err.Error())
				return
			}
		}

		if hasLifecycle {
			tflog.Debug(ctx, "Updating bucket lifecycle configuration")
			if err := r.updateBucketLifecycle(ctx, *plan, s3Client); err != nil {
				diags.AddError("Failed to update bucket lifecycle", err.Error())
				return
			}
		}
	}

	// Re-read to populate computed values into plan.
	r.readIntoModel(ctx, plan, client, config, regionOrCluster, label, diags)
}

// readIntoModel does a full API read and writes results back into model.
func (r *BucketResource) readIntoModel(
	ctx context.Context,
	plan *BucketResourceModel,
	client linodego.Client,
	config *helper.FrameworkProviderModel,
	regionOrCluster, label string,
	diags *diag.Diagnostics,
) {
	bucket, err := client.GetObjectStorageBucket(ctx, regionOrCluster, label)
	if err != nil {
		diags.AddError("Failed to read Object Storage Bucket after update", err.Error())
		return
	}

	access, err := client.GetObjectStorageBucketAccessV2(ctx, regionOrCluster, label)
	if err != nil {
		diags.AddError("Failed to read bucket access config after update", err.Error())
		return
	}

	hasVersioning := !plan.Versioning.IsNull() && !plan.Versioning.IsUnknown()
	hasLifecycle := len(plan.LifecycleRule) > 0

	if hasVersioning || hasLifecycle {
		objKeys, teardown := r.resolveObjKeys(ctx, *plan, client, config, label, regionOrCluster, "read_only", &bucket.EndpointType, diags)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		s3ep := getS3Endpoint(ctx, *bucket)
		s3Client, err := helper.S3Connection(ctx, s3ep, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			diags.AddError("Failed to create S3 connection during read", err.Error())
			return
		}

		if hasLifecycle {
			r.readBucketLifecycle(ctx, plan, s3Client, diags)
			if diags.HasError() {
				return
			}
		}
		if hasVersioning {
			r.readBucketVersioning(ctx, plan, s3Client, diags)
			if diags.HasError() {
				return
			}
		}
	}

	if bucket.Region != "" {
		plan.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
	} else {
		plan.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
	}

	ep := getS3Endpoint(ctx, *bucket)
	plan.Cluster = types.StringValue(bucket.Cluster)
	plan.Region = types.StringValue(bucket.Region)
	plan.Label = types.StringValue(bucket.Label)
	plan.Hostname = types.StringValue(bucket.Hostname)
	plan.ACL = types.StringValue(string(access.ACL))
	plan.CorsEnabled = types.BoolValue(access.CorsEnabled)
	plan.Endpoint = types.StringValue(ep)
	plan.S3Endpoint = types.StringValue(ep)
	plan.EndpointType = types.StringValue(string(bucket.EndpointType))
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (r *BucketResource) populateLogAttributes(ctx context.Context, m BucketResourceModel) context.Context {
	return helper.SetLogFieldBulk(ctx, map[string]any{
		"bucket":  m.Label.ValueString(),
		"cluster": m.Cluster.ValueString(),
	})
}

// DecodeBucketID is exported so tests can call it without schema.ResourceData.
func DecodeBucketID(ctx context.Context, id string) (regionOrCluster, label string, err error) {
	tflog.Debug(ctx, "decoding bucket ID")
	parts := strings.SplitN(id, ":", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf(
		"Linode Object Storage Bucket ID must be of the form <ClusterOrRegion>:<Label>, got %q", id,
	)
}

// decodeBucketIDFromModel parses "regionOrCluster:label" from the model's ID,
// falling back to Cluster/Region + Label attributes for recovery.
func decodeBucketIDFromModel(ctx context.Context, m BucketResourceModel) (regionOrCluster, label string, err error) {
	id := m.ID.ValueString()
	tflog.Debug(ctx, "decoding bucket ID")
	parts := strings.SplitN(id, ":", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}

	tflog.Warn(ctx, "Corrupted bucket ID detected, trying to recover it from cluster/region and label attributes.")

	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		regionOrCluster = m.Region.ValueString()
	} else if !m.Cluster.IsNull() && !m.Cluster.IsUnknown() && m.Cluster.ValueString() != "" {
		regionOrCluster = m.Cluster.ValueString()
	}

	if !m.Label.IsNull() && !m.Label.IsUnknown() {
		label = m.Label.ValueString()
	}

	if regionOrCluster != "" && label != "" {
		return regionOrCluster, label, nil
	}

	return "", "", fmt.Errorf(
		"Linode Object Storage Bucket ID must be of the form <ClusterOrRegion>:<Label>, "+
			"but a corrupted ID %q was in the state and recovery from attributes failed", id,
	)
}

// resolveObjKeys resolves object storage keys:
// 1. resource-level access_key/secret_key
// 2. provider-level obj_access_key/obj_secret_key
// 3. temporary keys (if obj_use_temp_keys is true)
func (r *BucketResource) resolveObjKeys(
	ctx context.Context,
	m BucketResourceModel,
	client linodego.Client,
	config *helper.FrameworkProviderModel,
	bucketLabel, regionOrCluster, permission string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) (bucketKeys, func()) {
	// 1. Resource-level keys.
	keys := bucketKeys{
		AccessKey: m.AccessKey.ValueString(),
		SecretKey: m.SecretKey.ValueString(),
	}
	if keys.ok() {
		return keys, nil
	}

	// 2. Provider-level keys.
	keys.AccessKey = config.ObjAccessKey.ValueString()
	keys.SecretKey = config.ObjSecretKey.ValueString()
	if keys.ok() {
		return keys, nil
	}

	// 3. Temporary keys.
	if config.ObjUseTempKeys.ValueBool() {
		objKey := r.createTempKeys(ctx, &client, bucketLabel, regionOrCluster, permission, endpointType, diags)
		if diags.HasError() {
			return bucketKeys{}, nil
		}

		keys.AccessKey = objKey.AccessKey
		keys.SecretKey = objKey.SecretKey
		teardown := func() {
			if err := client.DeleteObjectStorageKey(ctx, objKey.ID); err != nil {
				tflog.Warn(ctx, "Failed to clean up temporary object storage keys", map[string]any{
					"details": err,
				})
			}
		}
		return keys, teardown
	}

	diags.AddError(
		"Keys Not Found",
		"`access_key` and `secret_key` are required but not configured.",
	)
	return bucketKeys{}, nil
}

// createTempKeys creates short-lived Object Storage keys scoped to a single bucket.
func (r *BucketResource) createTempKeys(
	ctx context.Context,
	client *linodego.Client,
	bucketLabel, regionOrCluster, permissions string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) *linodego.ObjectStorageKey {
	tflog.Debug(ctx, "Create temporary object storage access keys implicitly.")

	tempBucketAccess := linodego.ObjectStorageKeyBucketAccess{
		BucketName:  bucketLabel,
		Permissions: permissions,
	}

	if isCluster(regionOrCluster) {
		tempBucketAccess.Cluster = regionOrCluster
	} else {
		tempBucketAccess.Region = regionOrCluster
	}

	createOpts := linodego.ObjectStorageKeyCreateOptions{
		Label:        fmt.Sprintf("temp_%s_%v", bucketLabel, time.Now().Unix()),
		BucketAccess: &[]linodego.ObjectStorageKeyBucketAccess{tempBucketAccess},
	}

	tflog.Debug(ctx, "client.CreateObjectStorageKey(...)", map[string]any{"options": createOpts})
	keys, err := client.CreateObjectStorageKey(ctx, createOpts)
	if err != nil {
		diags.AddError("Failed to Create Object Storage Key", err.Error())
		return nil
	}

	if endpointType == nil {
		et, err := getBucketEndpointType(ctx, client, regionOrCluster, bucketLabel)
		if err != nil {
			tflog.Warn(ctx, fmt.Sprintf("Can't determine the type of the object storage endpoint: %s", err.Error()))
		} else {
			endpointType = &et
		}
	}

	// OBJ gen2 limited keys take up to 30s to propagate.
	if endpointType != nil && *endpointType != linodego.ObjectStorageEndpointE0 && *endpointType != linodego.ObjectStorageEndpointE1 {
		time.Sleep(30 * time.Second)
	}

	return keys
}

// isCluster returns true for cluster-style identifiers (e.g. "us-mia-1").
func isCluster(regionOrCluster string) bool {
	re := regexp.MustCompile(`^[a-z]{2}-[a-z]+-[0-9]+$`)
	return re.MatchString(regionOrCluster)
}

// getBucketEndpointType fetches the endpoint type for a given cluster/bucket.
func getBucketEndpointType(
	ctx context.Context, client *linodego.Client, cluster, label string,
) (linodego.ObjectStorageEndpointType, error) {
	bucket, err := client.GetObjectStorageBucket(ctx, cluster, label)
	if err != nil {
		return "", err
	}
	return bucket.EndpointType, nil
}

// s3ConnectionFromModel returns an S3 client using the bucket's s3_endpoint (or a computed fallback).
func (r *BucketResource) s3ConnectionFromModel(
	ctx context.Context,
	m BucketResourceModel,
	client linodego.Client,
	accessKey, secretKey string,
) (*s3.Client, error) {
	endpoint := m.S3Endpoint.ValueString()
	if endpoint == "" {
		regionOrCluster, label, err := decodeBucketIDFromModel(ctx, m)
		if err != nil {
			return nil, err
		}
		b, err := client.GetObjectStorageBucket(ctx, regionOrCluster, label)
		if err != nil {
			return nil, fmt.Errorf("failed to find the specified Linode ObjectStorageBucket: %s", err)
		}
		endpoint = helper.ComputeS3EndpointFromBucket(ctx, *b)
	}
	return helper.S3Connection(ctx, endpoint, accessKey, secretKey)
}

func (r *BucketResource) validateRegion(
	ctx context.Context,
	region string,
	client linodego.Client,
	diags *diag.Diagnostics,
) {
	valid, suggestedRegions, err := validateRegion(ctx, region, &client)
	if err != nil {
		diags.AddError("Failed to validate region", err.Error())
		return
	}
	if !valid {
		msg := fmt.Sprintf("Region '%s' is not valid for Object Storage.", region)
		if len(suggestedRegions) > 0 {
			msg += fmt.Sprintf(" Suggested regions: %s", strings.Join(suggestedRegions, ", "))
		}
		diags.AddError("Invalid region", msg)
	}
}

// ---------------------------------------------------------------------------
// S3 read helpers
// ---------------------------------------------------------------------------

func (r *BucketResource) readBucketVersioning(
	ctx context.Context,
	m *BucketResourceModel,
	client *s3.Client,
	diags *diag.Diagnostics,
) {
	tflog.Trace(ctx, "entering readBucketVersioning")
	label := m.Label.ValueString()

	output, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &label})
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Failed to get versioning for bucket %q", label),
			err.Error(),
		)
		return
	}

	m.Versioning = types.BoolValue(output.Status == s3types.BucketVersioningStatusEnabled)
}

func (r *BucketResource) readBucketLifecycle(
	ctx context.Context,
	m *BucketResourceModel,
	client *s3.Client,
	diags *diag.Diagnostics,
) {
	label := m.Label.ValueString()

	output, err := client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: &label})
	if err != nil {
		var ae smithy.APIError
		if ok := errors.As(err, &ae); !ok || ae.ErrorCode() != "NoSuchLifecycleConfiguration" {
			diags.AddError(
				fmt.Sprintf("Failed to get lifecycle for bucket %q", label),
				err.Error(),
			)
			return
		}
	}

	if output == nil {
		tflog.Debug(ctx, "'lifecycleConfigOutput' is nil, skipping further processing")
		return
	}

	rulesMatched := output.Rules

	// Match existing declared rules to preserve ordering.
	if len(m.LifecycleRule) > 0 {
		rulesMatched = matchRulesWithSchemaModel(ctx, rulesMatched, m.LifecycleRule)
	}

	m.LifecycleRule = flattenLifecycleRulesToModel(ctx, rulesMatched)
}

// ---------------------------------------------------------------------------
// S3 update helpers
// ---------------------------------------------------------------------------

func (r *BucketResource) updateBucketVersioning(
	ctx context.Context,
	m BucketResourceModel,
	client *s3.Client,
) error {
	bucket := m.Label.ValueString()
	enable := m.Versioning.ValueBool()

	status := s3types.BucketVersioningStatusSuspended
	if enable {
		status = s3types.BucketVersioningStatusEnabled
	}

	input := &s3.PutBucketVersioningInput{
		Bucket: &bucket,
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: status,
		},
	}
	tflog.Debug(ctx, "client.PutBucketVersioning(...)", map[string]any{"options": input})
	_, err := client.PutBucketVersioning(ctx, input)
	return err
}

func (r *BucketResource) updateBucketLifecycle(
	ctx context.Context,
	m BucketResourceModel,
	client *s3.Client,
) error {
	bucket := m.Label.ValueString()

	rules, err := expandLifecycleRulesFromModel(ctx, m.LifecycleRule)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, "got expanded lifecycle rules", map[string]any{"rules": rules})

	if len(rules) > 0 {
		tflog.Debug(ctx, "there is at least one rule, calling the put endpoint")
		_, err = client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
			Bucket: &bucket,
			LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
				Rules: rules,
			},
		})
	} else {
		opts := &s3.DeleteBucketLifecycleInput{Bucket: &bucket}
		tflog.Debug(ctx, "client.DeleteBucketLifecycle(...)", map[string]any{"options": opts})
		_, err = client.DeleteBucketLifecycle(ctx, opts)
	}

	return err
}

func (r *BucketResource) updateBucketAccess(
	ctx context.Context,
	m BucketResourceModel,
	client linodego.Client,
	regionOrCluster, label string,
) error {
	tflog.Debug(ctx, "entering updateBucketAccess")

	updateOpts := linodego.ObjectStorageBucketUpdateAccessOptions{
		ACL: linodego.ObjectStorageACL(m.ACL.ValueString()),
	}
	if !m.CorsEnabled.IsNull() && !m.CorsEnabled.IsUnknown() {
		v := m.CorsEnabled.ValueBool()
		updateOpts.CorsEnabled = &v
	}

	tflog.Debug(ctx, "client.UpdateObjectStorageBucketAccess(...)", map[string]any{"options": updateOpts})
	if err := client.UpdateObjectStorageBucketAccess(ctx, regionOrCluster, label, updateOpts); err != nil {
		return fmt.Errorf("failed to update bucket access: %s", err)
	}
	return nil
}

// updateBucketCert handles cert create/update/delete.
// oldCert is the prior-state cert (nil if there was none). newCert comes from m.Cert (nil if removed).
func (r *BucketResource) updateBucketCert(
	ctx context.Context,
	m BucketResourceModel,
	oldCert *BucketCertModel,
	client linodego.Client,
	regionOrCluster, label string,
) error {
	tflog.Debug(ctx, "entering updateBucketCert")

	hasOldCert := oldCert != nil
	hasNewCert := m.Cert != nil

	if !hasOldCert && !hasNewCert {
		// Nothing to do.
		return nil
	}

	if hasOldCert {
		// Delete the existing cert (needed before upload, or when cert is removed).
		tflog.Debug(ctx, "client.DeleteObjectStorageBucketCert(...)")
		if err := client.DeleteObjectStorageBucketCert(ctx, regionOrCluster, label); err != nil {
			return fmt.Errorf("failed to delete old bucket cert: %s", err)
		}
	}

	if !hasNewCert {
		// Cert was removed; deletion above is sufficient.
		return nil
	}

	uploadOptions := linodego.ObjectStorageBucketCertUploadOptions{
		Certificate: m.Cert.Certificate.ValueString(),
		PrivateKey:  m.Cert.PrivateKey.ValueString(),
	}
	if _, err := client.UploadObjectStorageBucketCertV2(ctx, regionOrCluster, label, uploadOptions); err != nil {
		return fmt.Errorf("failed to upload new bucket cert: %s", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Lifecycle rule flatten / expand helpers
// ---------------------------------------------------------------------------

func flattenLifecycleRulesToModel(ctx context.Context, rules []s3types.LifecycleRule) []LifecycleRuleModel {
	tflog.Debug(ctx, "entering flattenLifecycleRulesToModel")
	result := make([]LifecycleRuleModel, len(rules))

	for i, rule := range rules {
		m := LifecycleRuleModel{}

		if rule.ID != nil {
			m.ID = types.StringValue(*rule.ID)
		} else {
			m.ID = types.StringValue("")
		}
		if rule.Prefix != nil {
			m.Prefix = types.StringValue(*rule.Prefix)
		} else {
			m.Prefix = types.StringValue("")
		}
		m.Enabled = types.BoolValue(rule.Status == s3types.ExpirationStatusEnabled)

		if rule.AbortIncompleteMultipartUpload != nil && rule.AbortIncompleteMultipartUpload.DaysAfterInitiation != nil {
			m.AbortIncompleteMultipartUploadDays = types.Int64Value(int64(*rule.AbortIncompleteMultipartUpload.DaysAfterInitiation))
		}

		if rule.Expiration != nil {
			exp := ExpirationModel{}
			if rule.Expiration.Date != nil {
				exp.Date = types.StringValue(rule.Expiration.Date.Format("2006-01-02"))
			} else {
				exp.Date = types.StringValue("")
			}
			if rule.Expiration.Days != nil {
				exp.Days = types.Int64Value(int64(*rule.Expiration.Days))
			} else {
				exp.Days = types.Int64Value(0)
			}
			if rule.Expiration.ExpiredObjectDeleteMarker != nil {
				exp.ExpiredObjectDeleteMarker = types.BoolValue(*rule.Expiration.ExpiredObjectDeleteMarker)
			} else {
				exp.ExpiredObjectDeleteMarker = types.BoolValue(false)
			}
			m.Expiration = []ExpirationModel{exp}
		}

		if rule.NoncurrentVersionExpiration != nil {
			ncve := NoncurrentVersionExpirationModel{}
			if rule.NoncurrentVersionExpiration.NoncurrentDays != nil && *rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
				ncve.Days = types.Int64Value(int64(*rule.NoncurrentVersionExpiration.NoncurrentDays))
			} else {
				ncve.Days = types.Int64Value(0)
			}
			m.NoncurrentVersionExpiration = []NoncurrentVersionExpirationModel{ncve}
		}

		tflog.Debug(ctx, "a rule has been flattened")
		result[i] = m
	}

	return result
}

func expandLifecycleRulesFromModel(ctx context.Context, rules []LifecycleRuleModel) ([]s3types.LifecycleRule, error) {
	tflog.Debug(ctx, "entering expandLifecycleRulesFromModel")

	result := make([]s3types.LifecycleRule, len(rules))
	for i, r := range rules {
		rule := s3types.LifecycleRule{}

		status := s3types.ExpirationStatusDisabled
		if r.Enabled.ValueBool() {
			status = s3types.ExpirationStatusEnabled
		}
		rule.Status = status

		if !r.ID.IsNull() && !r.ID.IsUnknown() && r.ID.ValueString() != "" {
			v := r.ID.ValueString()
			rule.ID = &v
		}
		if !r.Prefix.IsNull() && !r.Prefix.IsUnknown() && r.Prefix.ValueString() != "" {
			v := r.Prefix.ValueString()
			rule.Prefix = &v
		}

		if !r.AbortIncompleteMultipartUploadDays.IsNull() && !r.AbortIncompleteMultipartUploadDays.IsUnknown() {
			days := r.AbortIncompleteMultipartUploadDays.ValueInt64()
			if days > 0 {
				d, err := helper.SafeIntToInt32(int(days))
				if err != nil {
					return nil, err
				}
				rule.AbortIncompleteMultipartUpload = &s3types.AbortIncompleteMultipartUpload{
					DaysAfterInitiation: &d,
				}
			}
		}

		if len(r.Expiration) > 0 {
			tflog.Debug(ctx, "expanding expiration list")
			rule.Expiration = &s3types.LifecycleExpiration{}
			exp := r.Expiration[0]

			if !exp.Date.IsNull() && !exp.Date.IsUnknown() && exp.Date.ValueString() != "" {
				date, err := time.Parse(time.RFC3339, fmt.Sprintf("%sT00:00:00Z", exp.Date.ValueString()))
				if err != nil {
					return nil, err
				}
				rule.Expiration.Date = &date
			}

			if !exp.Days.IsNull() && !exp.Days.IsUnknown() && exp.Days.ValueInt64() > 0 {
				d, err := helper.SafeIntToInt32(int(exp.Days.ValueInt64()))
				if err != nil {
					return nil, err
				}
				rule.Expiration.Days = &d
			}

			if !exp.ExpiredObjectDeleteMarker.IsNull() && !exp.ExpiredObjectDeleteMarker.IsUnknown() && exp.ExpiredObjectDeleteMarker.ValueBool() {
				v := true
				rule.Expiration.ExpiredObjectDeleteMarker = &v
			}
		}

		if len(r.NoncurrentVersionExpiration) > 0 {
			tflog.Debug(ctx, "expanding noncurrent_version_expiration list")
			rule.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{}
			ncve := r.NoncurrentVersionExpiration[0]

			if !ncve.Days.IsNull() && !ncve.Days.IsUnknown() && ncve.Days.ValueInt64() > 0 {
				d, err := helper.SafeIntToInt32(int(ncve.Days.ValueInt64()))
				if err != nil {
					return nil, err
				}
				rule.NoncurrentVersionExpiration.NoncurrentDays = &d
			}
		}

		tflog.Debug(ctx, "a rule has been expanded", map[string]any{"rule": rule})
		result[i] = rule
	}

	return result, nil
}

// matchRulesWithSchemaModel preserves the declared order of lifecycle rules.
func matchRulesWithSchemaModel(
	ctx context.Context,
	rules []s3types.LifecycleRule,
	declared []LifecycleRuleModel,
) []s3types.LifecycleRule {
	tflog.Debug(ctx, "entering matchRulesWithSchemaModel")

	result := make([]s3types.LifecycleRule, 0)
	ruleMap := make(map[string]s3types.LifecycleRule)
	for _, rule := range rules {
		if rule.ID != nil {
			ruleMap[*rule.ID] = rule
		}
	}

	for _, d := range declared {
		if d.ID.IsNull() || d.ID.IsUnknown() || d.ID.ValueString() == "" {
			continue
		}
		id := d.ID.ValueString()
		if rule, ok := ruleMap[id]; ok {
			result = append(result, rule)
			delete(ruleMap, id)
		}
	}

	// Append any remaining (server-added) rules.
	for _, rule := range ruleMap {
		result = append(result, rule)
	}

	return result
}
