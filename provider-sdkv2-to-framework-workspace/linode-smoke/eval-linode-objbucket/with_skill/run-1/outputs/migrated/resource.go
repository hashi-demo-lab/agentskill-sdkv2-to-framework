package objbucket

import (
	"context"
	"errors"
	"fmt"
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

// Ensure interface compliance at compile time.
var (
	_ resource.Resource                = &BucketResource{}
	_ resource.ResourceWithConfigure   = &BucketResource{}
	_ resource.ResourceWithImportState = &BucketResource{}
)

// ---------------------------------------------------------------------------
// Local key struct (avoids import cycle with obj package)
// ---------------------------------------------------------------------------

type bucketObjectKeys struct {
	AccessKey string
	SecretKey string
}

func (k bucketObjectKeys) ok() bool {
	return k.AccessKey != "" && k.SecretKey != ""
}

// ---------------------------------------------------------------------------
// Model types
// ---------------------------------------------------------------------------

// CertModel maps to the cert { ... } block.
type CertModel struct {
	Certificate types.String `tfsdk:"certificate"`
	PrivateKey  types.String `tfsdk:"private_key"`
}

// ExpirationModel maps to the expiration { ... } block inside lifecycle_rule.
type ExpirationModel struct {
	Date                      types.String `tfsdk:"date"`
	Days                      types.Int64  `tfsdk:"days"`
	ExpiredObjectDeleteMarker types.Bool   `tfsdk:"expired_object_delete_marker"`
}

// NoncurrentVersionExpirationModel maps to the noncurrent_version_expiration { ... } block.
type NoncurrentVersionExpirationModel struct {
	Days types.Int64 `tfsdk:"days"`
}

// LifecycleRuleModel maps to one lifecycle_rule { ... } block.
type LifecycleRuleModel struct {
	ID                                 types.String                       `tfsdk:"id"`
	Prefix                             types.String                       `tfsdk:"prefix"`
	Enabled                            types.Bool                         `tfsdk:"enabled"`
	AbortIncompleteMultipartUploadDays types.Int64                        `tfsdk:"abort_incomplete_multipart_upload_days"`
	Expiration                         []ExpirationModel                  `tfsdk:"expiration"`
	NoncurrentVersionExpiration        []NoncurrentVersionExpirationModel `tfsdk:"noncurrent_version_expiration"`
}

// ResourceModel is the top-level model for the resource state.
type ResourceModel struct {
	ID             types.String         `tfsdk:"id"`
	Label          types.String         `tfsdk:"label"`
	Cluster        types.String         `tfsdk:"cluster"`
	Region         types.String         `tfsdk:"region"`
	Endpoint       types.String         `tfsdk:"endpoint"`
	S3Endpoint     types.String         `tfsdk:"s3_endpoint"`
	EndpointType   types.String         `tfsdk:"endpoint_type"`
	Hostname       types.String         `tfsdk:"hostname"`
	ACL            types.String         `tfsdk:"acl"`
	CORSEnabled    types.Bool           `tfsdk:"cors_enabled"`
	Versioning     types.Bool           `tfsdk:"versioning"`
	AccessKey      types.String         `tfsdk:"access_key"`
	SecretKey      types.String         `tfsdk:"secret_key"`
	Cert           []CertModel          `tfsdk:"cert"`
	LifecycleRules []LifecycleRuleModel `tfsdk:"lifecycle_rule"`
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func NewFrameworkResource() resource.Resource {
	return &BucketResource{}
}

// BucketResource is the framework implementation of linode_object_storage_bucket.
type BucketResource struct {
	Meta *helper.FrameworkProviderMeta
}

// ---------------------------------------------------------------------------
// resource.Resource interface
// ---------------------------------------------------------------------------

func (r *BucketResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = "linode_object_storage_bucket"
}

func (r *BucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
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
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The region of the Linode Object Storage Bucket.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
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
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"endpoint_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The type of the S3 endpoint available in this region.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hostname": schema.StringAttribute{
				Computed:    true,
				Description: "The hostname where this bucket can be accessed. This hostname can be accessed through a browser if the bucket is made public.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// acl has a Default of "private" — must be Computed:true per framework rules.
			"acl": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The Access Control Level of the bucket using a canned ACL string.",
				Default:     stringdefault.StaticString("private"),
			},
			"cors_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "If true, the bucket will be created with CORS enabled for all origins.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
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
				Optional:    true,
				Description: "The S3 access key to use for this resource. (Required for lifecycle_rule and versioning). If not specified with the resource, the value will be read from provider-level obj_access_key, or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			},
			"secret_key": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The S3 secret key to use for this resource. (Required for lifecycle_rule and versioning). If not specified with the resource, the value will be read from provider-level obj_secret_key, or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			},
		},
		Blocks: map[string]schema.Block{
			// cert: MaxItems:1 in SDKv2 — kept as ListNestedBlock+SizeAtMost(1) to preserve
			// practitioner block syntax (cert { ... }) without breaking HCL.
			"cert": schema.ListNestedBlock{
				Description: "The cert used by this Object Storage Bucket.",
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
				NestedObject: schema.NestedBlockObject{
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
			},
			// lifecycle_rule: repeating list (no MaxItems) — stays as ListNestedBlock.
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
							Optional:    true,
							Description: "Specifies the number of days after initiating a multipart upload when the multipart upload must be completed.",
						},
					},
					Blocks: map[string]schema.Block{
						// expiration: MaxItems:1 nested inside lifecycle_rule — ListNestedBlock+SizeAtMost(1).
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
										Description: "Directs Linode Object Storage to remove expired deleted markers.",
									},
								},
							},
						},
						// noncurrent_version_expiration: MaxItems:1 nested inside lifecycle_rule.
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
// resource.ResourceWithConfigure
// ---------------------------------------------------------------------------

func (r *BucketResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	r.Meta = helper.GetResourceMeta(req, resp)
}

// ---------------------------------------------------------------------------
// resource.ResourceWithImportState — composite ID "<cluster_or_region>:<label>"
// ---------------------------------------------------------------------------

func (r *BucketResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	// The import ID format is "<cluster_or_region>:<label>", e.g. "us-mia:my-bucket".
	// We write the full composite ID into the "id" attribute; Read will decode it via
	// decodeBucketIDFromModel and populate the remaining attributes.
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf(
				"Expected import ID in the form '<cluster_or_region>:<label>', got %q. "+
					"Example: terraform import linode_object_storage_bucket.example us-mia:my-bucket",
				req.ID,
			),
		)
		return
	}

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
	client := r.Meta.Client

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, plan)

	// Validate region if specified.
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
		if diags := r.validateRegionIfPresent(ctx, plan.Region.ValueString()); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	createOpts := linodego.ObjectStorageBucketCreateOptions{
		Label: plan.Label.ValueString(),
		ACL:   linodego.ObjectStorageACL(plan.ACL.ValueString()),
	}

	if !plan.CORSEnabled.IsNull() && !plan.CORSEnabled.IsUnknown() {
		v := plan.CORSEnabled.ValueBool()
		createOpts.CorsEnabled = &v
	}

	if !plan.S3Endpoint.IsNull() && !plan.S3Endpoint.IsUnknown() && plan.S3Endpoint.ValueString() != "" {
		createOpts.S3Endpoint = plan.S3Endpoint.ValueString()
	}

	if !plan.EndpointType.IsNull() && !plan.EndpointType.IsUnknown() && plan.EndpointType.ValueString() != "" {
		createOpts.EndpointType = linodego.ObjectStorageEndpointType(plan.EndpointType.ValueString())
	}

	if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
		createOpts.Region = plan.Region.ValueString()
	}

	if !plan.Cluster.IsNull() && !plan.Cluster.IsUnknown() && plan.Cluster.ValueString() != "" {
		createOpts.Cluster = plan.Cluster.ValueString()
	}

	tflog.Debug(ctx, "client.CreateObjectStorageBucket(...)", map[string]any{"options": createOpts})
	bucket, err := client.CreateObjectStorageBucket(ctx, createOpts)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Create Object Storage Bucket", err.Error())
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

	// Persist partial state early to avoid dangling resources.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply access, cert, lifecycle, versioning updates (same path as updateResource).
	r.applyUpdates(ctx, &plan, &resp.Diagnostics)
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
	client := r.Meta.Client

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, state)

	regionOrCluster, label, err := decodeBucketIDFromModel(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Parse Object Storage Bucket ID",
			fmt.Sprintf("failed to parse Linode ObjectStorageBucket id %s: %s", state.ID.ValueString(), err),
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
		resp.Diagnostics.AddError("Failed to Find Object Storage Bucket", err.Error())
		return
	}

	tflog.Debug(ctx, "getting bucket access info")
	access, err := client.GetObjectStorageBucketAccessV2(ctx, regionOrCluster, label)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Find Bucket Access Config", err.Error())
		return
	}

	// Populate state from API response.
	state.Cluster = types.StringValue(bucket.Cluster)
	state.Region = types.StringValue(bucket.Region)
	state.Label = types.StringValue(bucket.Label)
	state.Hostname = types.StringValue(bucket.Hostname)
	state.ACL = types.StringValue(string(access.ACL))
	if access.CorsEnabled != nil {
		state.CORSEnabled = types.BoolValue(*access.CorsEnabled)
	}

	endpoint := getS3Endpoint(ctx, *bucket)
	state.Endpoint = types.StringValue(endpoint)
	state.S3Endpoint = types.StringValue(endpoint)
	state.EndpointType = types.StringValue(string(bucket.EndpointType))

	if bucket.Region != "" {
		state.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
	} else {
		state.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
	}

	// Read versioning/lifecycle only when they are already in state (user configured them).
	hasVersioning := !state.Versioning.IsNull() && !state.Versioning.IsUnknown()
	hasLifecycle := len(state.LifecycleRules) > 0

	if hasVersioning || hasLifecycle {
		tflog.Debug(ctx, "versioning or lifecycle present — fetching S3 config")

		var endpointType *linodego.ObjectStorageEndpointType
		if !state.EndpointType.IsNull() && state.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(state.EndpointType.ValueString())
			endpointType = &et
		}

		objKeys, teardown := r.getObjKeys(ctx, state, regionOrCluster, "read_only", endpointType, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		s3Client, err := helper.S3Connection(ctx, endpoint, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Create S3 Connection", err.Error())
			return
		}

		if hasLifecycle {
			tflog.Trace(ctx, "getting bucket lifecycle")
			resp.Diagnostics.Append(r.readBucketLifecycle(ctx, &state, s3Client)...)
		}

		if hasVersioning {
			tflog.Trace(ctx, "getting bucket versioning")
			resp.Diagnostics.Append(r.readBucketVersioning(ctx, &state, s3Client)...)
		}

		if resp.Diagnostics.HasError() {
			return
		}
	}

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
	client := r.Meta.Client

	var plan, state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, state)

	if !plan.Region.Equal(state.Region) {
		if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
			if diags := r.validateRegionIfPresent(ctx, plan.Region.ValueString()); diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	}

	if !plan.ACL.Equal(state.ACL) || !plan.CORSEnabled.Equal(state.CORSEnabled) {
		tflog.Debug(ctx, "'acl'/'cors_enabled' changes detected, will update bucket access")
		regionOrCluster := getRegionOrClusterFromModel(state)
		if err := r.updateBucketAccess(ctx, plan, regionOrCluster); err != nil {
			resp.Diagnostics.AddError("Failed to Update Bucket Access", err.Error())
			return
		}
	}

	if !certListEqual(plan.Cert, state.Cert) {
		tflog.Debug(ctx, "'cert' changes detected, will update bucket certificate")
		regionOrCluster := getRegionOrClusterFromModel(state)
		if err := r.updateBucketCert(ctx, state.Cert, plan.Cert, regionOrCluster, plan.Label.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to Update Bucket Certificate", err.Error())
			return
		}
		// cert fields are write-only (not readable from API); preserve plan values.
		plan.Cert = plan.Cert
	}

	versioningChanged := !plan.Versioning.Equal(state.Versioning)
	lifecycleChanged := !lifecycleListEqual(plan.LifecycleRules, state.LifecycleRules)

	if versioningChanged || lifecycleChanged {
		tflog.Debug(ctx, "versioning or lifecycle change detected")

		regionOrCluster := getRegionOrClusterFromModel(state)
		label := plan.Label.ValueString()

		var endpointType *linodego.ObjectStorageEndpointType
		if !plan.EndpointType.IsNull() && plan.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(plan.EndpointType.ValueString())
			endpointType = &et
		}

		objKeys, teardown := r.getObjKeys(ctx, plan, regionOrCluster, "read_write", endpointType, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := plan.S3Endpoint.ValueString()
		if endpoint == "" {
			endpoint = plan.Endpoint.ValueString()
		}
		s3client, err := helper.S3Connection(ctx, endpoint, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Create S3 Connection", err.Error())
			return
		}

		if versioningChanged {
			tflog.Debug(ctx, "Updating bucket versioning configuration")
			if err := r.updateBucketVersioning(ctx, label, plan.Versioning.ValueBool(), s3client); err != nil {
				resp.Diagnostics.AddError("Failed to Update Bucket Versioning", err.Error())
				return
			}
		}

		if lifecycleChanged {
			tflog.Debug(ctx, "Updating bucket lifecycle configuration")
			if err := r.updateBucketLifecycle(ctx, label, plan.LifecycleRules, s3client); err != nil {
				resp.Diagnostics.AddError("Failed to Update Bucket Lifecycle", err.Error())
				return
			}
		}
	}

	// Re-read to populate computed fields.
	r.populateModelFromAPI(ctx, &plan, client, &resp.Diagnostics)
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
	client := r.Meta.Client
	config := r.Meta.Config

	// Delete reads from State — req.Plan is null on delete.
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, state)

	regionOrCluster, label, err := decodeBucketIDFromModel(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Parsing Bucket ID",
			fmt.Sprintf("Error parsing Linode ObjectStorageBucket id %s", state.ID.ValueString()),
		)
		return
	}

	if config.ObjBucketForceDelete.ValueBool() {
		var endpointType *linodego.ObjectStorageEndpointType
		if !state.EndpointType.IsNull() && state.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(state.EndpointType.ValueString())
			endpointType = &et
		}

		objKeys, teardown := r.getObjKeys(ctx, state, regionOrCluster, "read_write", endpointType, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := state.S3Endpoint.ValueString()
		if endpoint == "" {
			endpoint = state.Endpoint.ValueString()
		}
		s3client, err := helper.S3Connection(ctx, endpoint, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to Create S3 Connection for Force Delete", err.Error())
			return
		}

		tflog.Debug(ctx, "helper.PurgeAllObjects(...)")
		if err := helper.PurgeAllObjects(ctx, label, s3client, true, true); err != nil {
			resp.Diagnostics.AddError(
				"Error Purging Objects",
				fmt.Sprintf("Error purging all objects from ObjectStorageBucket %s: %s", state.ID.ValueString(), err),
			)
			return
		}
	}

	tflog.Debug(ctx, "client.DeleteObjectStorageBucket(...)")
	if err := client.DeleteObjectStorageBucket(ctx, regionOrCluster, label); err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Object Storage Bucket",
			fmt.Sprintf("Error deleting Linode ObjectStorageBucket %s: %s", state.ID.ValueString(), err),
		)
	}
}

// ---------------------------------------------------------------------------
// Internal — shared logic reused by Create and Update
// ---------------------------------------------------------------------------

// applyUpdates handles the post-create pass (cert, lifecycle, versioning, access refresh).
func (r *BucketResource) applyUpdates(ctx context.Context, plan *ResourceModel, diags *diag.Diagnostics) {
	client := r.Meta.Client

	regionOrCluster := getRegionOrClusterFromModel(*plan)
	label := plan.Label.ValueString()

	// Access (ACL / CORS).
	if err := r.updateBucketAccess(ctx, *plan, regionOrCluster); err != nil {
		diags.AddError("Failed to Update Bucket Access", err.Error())
		return
	}

	// Cert.
	if len(plan.Cert) > 0 {
		if err := r.updateBucketCert(ctx, nil, plan.Cert, regionOrCluster, label); err != nil {
			diags.AddError("Failed to Upload Bucket Certificate", err.Error())
			return
		}
	}

	// Versioning / lifecycle.
	hasVersioning := !plan.Versioning.IsNull() && !plan.Versioning.IsUnknown()
	hasLifecycle := len(plan.LifecycleRules) > 0

	if hasVersioning || hasLifecycle {
		var endpointType *linodego.ObjectStorageEndpointType
		if !plan.EndpointType.IsNull() && plan.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(plan.EndpointType.ValueString())
			endpointType = &et
		}

		objKeys, teardown := r.getObjKeys(ctx, *plan, regionOrCluster, "read_write", endpointType, diags)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := plan.S3Endpoint.ValueString()
		s3client, err := helper.S3Connection(ctx, endpoint, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			diags.AddError("Failed to Create S3 Connection", err.Error())
			return
		}

		if hasVersioning {
			if err := r.updateBucketVersioning(ctx, label, plan.Versioning.ValueBool(), s3client); err != nil {
				diags.AddError("Failed to Update Bucket Versioning", err.Error())
				return
			}
		}

		if hasLifecycle {
			if err := r.updateBucketLifecycle(ctx, label, plan.LifecycleRules, s3client); err != nil {
				diags.AddError("Failed to Update Bucket Lifecycle", err.Error())
				return
			}
		}
	}

	// Re-read to populate computed fields.
	r.populateModelFromAPI(ctx, plan, client, diags)
}

// populateModelFromAPI refreshes computed fields from the Linode API.
func (r *BucketResource) populateModelFromAPI(
	ctx context.Context,
	model *ResourceModel,
	client *linodego.Client,
	diags *diag.Diagnostics,
) {
	regionOrCluster, label, err := decodeBucketIDFromModel(ctx, *model)
	if err != nil {
		diags.AddError("Failed to Parse Bucket ID", err.Error())
		return
	}

	bucket, err := client.GetObjectStorageBucket(ctx, regionOrCluster, label)
	if err != nil {
		diags.AddError("Failed to Find Object Storage Bucket", err.Error())
		return
	}

	access, err := client.GetObjectStorageBucketAccessV2(ctx, regionOrCluster, label)
	if err != nil {
		diags.AddError("Failed to Find Bucket Access Config", err.Error())
		return
	}

	model.Cluster = types.StringValue(bucket.Cluster)
	model.Region = types.StringValue(bucket.Region)
	model.Label = types.StringValue(bucket.Label)
	model.Hostname = types.StringValue(bucket.Hostname)
	model.ACL = types.StringValue(string(access.ACL))
	if access.CorsEnabled != nil {
		model.CORSEnabled = types.BoolValue(*access.CorsEnabled)
	}

	endpoint := getS3Endpoint(ctx, *bucket)
	model.Endpoint = types.StringValue(endpoint)
	model.S3Endpoint = types.StringValue(endpoint)
	model.EndpointType = types.StringValue(string(bucket.EndpointType))

	if bucket.Region != "" {
		model.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
	} else {
		model.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
	}
}

// ---------------------------------------------------------------------------
// Bucket access
// ---------------------------------------------------------------------------

func (r *BucketResource) updateBucketAccess(ctx context.Context, plan ResourceModel, regionOrCluster string) error {
	tflog.Debug(ctx, "entering updateBucketAccess")
	client := r.Meta.Client
	label := plan.Label.ValueString()

	updateOpts := linodego.ObjectStorageBucketUpdateAccessOptions{}
	updateOpts.ACL = linodego.ObjectStorageACL(plan.ACL.ValueString())
	if !plan.CORSEnabled.IsNull() && !plan.CORSEnabled.IsUnknown() {
		v := plan.CORSEnabled.ValueBool()
		updateOpts.CorsEnabled = &v
	}

	tflog.Debug(ctx, "client.UpdateObjectStorageBucketAccess(...)", map[string]any{"options": updateOpts})
	return client.UpdateObjectStorageBucketAccess(ctx, regionOrCluster, label, updateOpts)
}

// ---------------------------------------------------------------------------
// Cert
// ---------------------------------------------------------------------------

func (r *BucketResource) updateBucketCert(
	ctx context.Context,
	oldCert, newCert []CertModel,
	regionOrCluster, label string,
) error {
	tflog.Debug(ctx, "entering updateBucketCert")
	client := r.Meta.Client

	if len(oldCert) > 0 {
		tflog.Debug(ctx, "client.DeleteObjectStorageBucketCert(...)")
		if err := client.DeleteObjectStorageBucketCert(ctx, regionOrCluster, label); err != nil {
			return fmt.Errorf("failed to delete old bucket cert: %s", err)
		}
	}

	if len(newCert) == 0 {
		return nil
	}

	uploadOptions := linodego.ObjectStorageBucketCertUploadOptions{
		Certificate: newCert[0].Certificate.ValueString(),
		PrivateKey:  newCert[0].PrivateKey.ValueString(),
	}
	if _, err := client.UploadObjectStorageBucketCertV2(ctx, regionOrCluster, label, uploadOptions); err != nil {
		return fmt.Errorf("failed to upload new bucket cert: %s", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Versioning
// ---------------------------------------------------------------------------

func (r *BucketResource) updateBucketVersioning(ctx context.Context, bucket string, enable bool, s3client *s3.Client) error {
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
	_, err := s3client.PutBucketVersioning(ctx, input)
	return err
}

func (r *BucketResource) readBucketVersioning(ctx context.Context, model *ResourceModel, s3client *s3.Client) diag.Diagnostics {
	tflog.Trace(ctx, "entering readBucketVersioning")
	var diags diag.Diagnostics
	label := model.Label.ValueString()

	out, err := s3client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &label})
	if err != nil {
		diags.AddError(
			"Failed to Get Bucket Versioning",
			fmt.Sprintf("failed to get versioning for bucket id %s: %s", model.ID.ValueString(), err),
		)
		return diags
	}

	model.Versioning = types.BoolValue(out.Status == s3types.BucketVersioningStatusEnabled)
	return diags
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (r *BucketResource) updateBucketLifecycle(ctx context.Context, bucket string, rules []LifecycleRuleModel, s3client *s3.Client) error {
	expanded, err := expandLifecycleRules(ctx, rules)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, "got expanded lifecycle rules", map[string]any{"rules": expanded})

	if len(expanded) > 0 {
		tflog.Debug(ctx, "calling PutBucketLifecycleConfiguration")
		_, err = s3client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
			Bucket: &bucket,
			LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
				Rules: expanded,
			},
		})
	} else {
		opts := &s3.DeleteBucketLifecycleInput{Bucket: &bucket}
		tflog.Debug(ctx, "client.DeleteBucketLifecycle(...)", map[string]any{"options": opts})
		_, err = s3client.DeleteBucketLifecycle(ctx, opts)
	}
	return err
}

func (r *BucketResource) readBucketLifecycle(ctx context.Context, model *ResourceModel, s3client *s3.Client) diag.Diagnostics {
	var diags diag.Diagnostics
	label := model.Label.ValueString()

	out, err := s3client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: &label})
	if err != nil {
		var ae smithy.APIError
		if ok := errors.As(err, &ae); !ok || ae.ErrorCode() != "NoSuchLifecycleConfiguration" {
			diags.AddError(
				"Failed to Get Bucket Lifecycle",
				fmt.Sprintf("failed to get lifecycle for bucket id %s: %s", model.ID.ValueString(), err),
			)
			return diags
		}
	}

	if out == nil {
		tflog.Debug(ctx, "'lifecycleConfigOutput' is nil, skipping further processing")
		model.LifecycleRules = []LifecycleRuleModel{}
		return diags
	}

	rules := out.Rules
	if len(model.LifecycleRules) > 0 {
		rules = matchRulesWithSchema(ctx, rules, model.LifecycleRules)
	}

	model.LifecycleRules = flattenLifecycleRules(ctx, rules)
	return diags
}

// ---------------------------------------------------------------------------
// Object key resolution
// ---------------------------------------------------------------------------

// getObjKeys returns access/secret keys following the same precedence as the SDKv2 version:
// 1) resource-level access_key/secret_key
// 2) provider-level obj_access_key/obj_secret_key
// 3) implicit temp keys if obj_use_temp_keys is set
func (r *BucketResource) getObjKeys(
	ctx context.Context,
	model ResourceModel,
	regionOrCluster, permission string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) (bucketObjectKeys, func()) {
	client := r.Meta.Client
	config := r.Meta.Config

	keys := bucketObjectKeys{
		AccessKey: model.AccessKey.ValueString(),
		SecretKey: model.SecretKey.ValueString(),
	}

	if keys.ok() {
		return keys, nil
	}

	keys.AccessKey = config.ObjAccessKey.ValueString()
	keys.SecretKey = config.ObjSecretKey.ValueString()

	if keys.ok() {
		return keys, nil
	}

	if config.ObjUseTempKeys.ValueBool() {
		label := model.Label.ValueString()
		tempKey := r.createTempKeys(ctx, client, label, regionOrCluster, permission, endpointType, diags)
		if diags.HasError() {
			return bucketObjectKeys{}, nil
		}
		keys.AccessKey = tempKey.AccessKey
		keys.SecretKey = tempKey.SecretKey
		teardown := func() {
			if err := client.DeleteObjectStorageKey(ctx, tempKey.ID); err != nil {
				tflog.Warn(ctx, "Failed to clean up temporary object storage keys", map[string]any{"details": err})
			}
		}
		return keys, teardown
	}

	diags.AddError("Keys Not Found", "`access_key` and `secret_key` are required.")
	return bucketObjectKeys{}, nil
}

// createTempKeys creates temporary scoped object storage keys for bucket operations.
func (r *BucketResource) createTempKeys(
	ctx context.Context,
	client *linodego.Client,
	bucketLabel, regionOrCluster, permissions string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) *linodego.ObjectStorageKey {
	access := linodego.ObjectStorageKeyBucketAccess{
		BucketName:  bucketLabel,
		Permissions: permissions,
	}

	if isClusterID(regionOrCluster) {
		access.Cluster = regionOrCluster
	} else {
		access.Region = regionOrCluster
	}

	truncLabel := bucketLabel
	if len(truncLabel) > 34 {
		truncLabel = truncLabel[:34]
	}

	createOpts := linodego.ObjectStorageKeyCreateOptions{
		Label:        fmt.Sprintf("temp_%s_%v", truncLabel, time.Now().Unix()),
		BucketAccess: &[]linodego.ObjectStorageKeyBucketAccess{access},
	}

	tflog.Debug(ctx, "client.CreateObjectStorageKey(...)", map[string]any{"options": createOpts})
	keys, err := client.CreateObjectStorageKey(ctx, createOpts)
	if err != nil {
		diags.AddError("Failed to Create Temporary Object Storage Keys", err.Error())
		return nil
	}

	// OBJ gen2 keys take up to 30s to become effective.
	if endpointType != nil &&
		*endpointType != linodego.ObjectStorageEndpointE0 &&
		*endpointType != linodego.ObjectStorageEndpointE1 {
		time.Sleep(30 * time.Second)
	}

	return keys
}

// ---------------------------------------------------------------------------
// Package-level utilities
// ---------------------------------------------------------------------------

// decodeBucketIDFromModel parses "regionOrCluster:label" from the model ID.
// Falls back to Cluster/Region + Label attributes if the ID is malformed.
func decodeBucketIDFromModel(ctx context.Context, model ResourceModel) (regionOrCluster, label string, err error) {
	id := model.ID.ValueString()
	parts := strings.Split(id, ":")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}

	tflog.Warn(ctx, "Corrupted bucket ID detected, trying to recover from cluster and label attributes.")

	if !model.Cluster.IsNull() && model.Cluster.ValueString() != "" {
		regionOrCluster = model.Cluster.ValueString()
	} else if !model.Region.IsNull() && model.Region.ValueString() != "" {
		regionOrCluster = model.Region.ValueString()
	}

	if !model.Label.IsNull() {
		label = model.Label.ValueString()
	}

	if regionOrCluster == "" || label == "" {
		err = fmt.Errorf(
			"Linode Object Storage Bucket ID must be of the form <ClusterOrRegion>:<Label>, "+
				"but a corrupted ID %q was found in state", id,
		)
	}
	return
}

// getRegionOrClusterFromModel returns region (preferred) or cluster.
func getRegionOrClusterFromModel(model ResourceModel) string {
	if !model.Region.IsNull() && model.Region.ValueString() != "" {
		return model.Region.ValueString()
	}
	return model.Cluster.ValueString()
}

// isClusterID returns true for old-style cluster IDs (e.g. "us-mia-1").
// Cluster names match <cc>-<city>-<N>; region names don't have that third segment.
func isClusterID(s string) bool {
	parts := strings.Split(s, "-")
	return len(parts) == 3
}

// flattenLifecycleRules converts S3 LifecycleRule slice to model slice.
func flattenLifecycleRules(ctx context.Context, rules []s3types.LifecycleRule) []LifecycleRuleModel {
	tflog.Debug(ctx, "entering flattenLifecycleRules")
	result := make([]LifecycleRuleModel, len(rules))

	for i, rule := range rules {
		m := LifecycleRuleModel{
			Enabled: types.BoolValue(rule.Status == s3types.ExpirationStatusEnabled),
		}

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
			nve := NoncurrentVersionExpirationModel{}
			if rule.NoncurrentVersionExpiration.NoncurrentDays != nil && *rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
				nve.Days = types.Int64Value(int64(*rule.NoncurrentVersionExpiration.NoncurrentDays))
			} else {
				nve.Days = types.Int64Value(0)
			}
			m.NoncurrentVersionExpiration = []NoncurrentVersionExpirationModel{nve}
		}

		tflog.Debug(ctx, "a rule has been flattened", map[string]any{"rule_id": m.ID.ValueString()})
		result[i] = m
	}

	return result
}

// expandLifecycleRules converts model slice to S3 LifecycleRule slice.
func expandLifecycleRules(ctx context.Context, rules []LifecycleRuleModel) ([]s3types.LifecycleRule, error) {
	tflog.Debug(ctx, "entering expandLifecycleRules")
	result := make([]s3types.LifecycleRule, len(rules))

	for i, m := range rules {
		rule := s3types.LifecycleRule{}

		status := s3types.ExpirationStatusDisabled
		if m.Enabled.ValueBool() {
			status = s3types.ExpirationStatusEnabled
		}
		rule.Status = status

		if !m.ID.IsNull() && m.ID.ValueString() != "" {
			v := m.ID.ValueString()
			rule.ID = &v
		}

		if !m.Prefix.IsNull() {
			v := m.Prefix.ValueString()
			rule.Prefix = &v
		}

		if !m.AbortIncompleteMultipartUploadDays.IsNull() {
			days := m.AbortIncompleteMultipartUploadDays.ValueInt64()
			if days > 0 {
				int32Days, err := helper.SafeIntToInt32(int(days))
				if err != nil {
					return nil, err
				}
				rule.AbortIncompleteMultipartUpload = &s3types.AbortIncompleteMultipartUpload{
					DaysAfterInitiation: &int32Days,
				}
			}
		}

		if len(m.Expiration) > 0 {
			tflog.Debug(ctx, "expanding expiration list")
			rule.Expiration = &s3types.LifecycleExpiration{}
			exp := m.Expiration[0]

			if !exp.Date.IsNull() && exp.Date.ValueString() != "" {
				date, err := time.Parse(time.RFC3339, fmt.Sprintf("%sT00:00:00Z", exp.Date.ValueString()))
				if err != nil {
					return nil, err
				}
				rule.Expiration.Date = &date
			}

			if !exp.Days.IsNull() {
				days := int(exp.Days.ValueInt64())
				if days > 0 {
					int32Days, err := helper.SafeIntToInt32(days)
					if err != nil {
						return nil, err
					}
					rule.Expiration.Days = &int32Days
				}
			}

			if !exp.ExpiredObjectDeleteMarker.IsNull() && exp.ExpiredObjectDeleteMarker.ValueBool() {
				v := true
				rule.Expiration.ExpiredObjectDeleteMarker = &v
			}
		}

		if len(m.NoncurrentVersionExpiration) > 0 {
			tflog.Debug(ctx, "expanding noncurrent_version_expiration list")
			rule.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{}
			nve := m.NoncurrentVersionExpiration[0]

			if !nve.Days.IsNull() {
				days := int(nve.Days.ValueInt64())
				if days > 0 {
					int32Days, err := helper.SafeIntToInt32(days)
					if err != nil {
						return nil, err
					}
					rule.NoncurrentVersionExpiration.NoncurrentDays = &int32Days
				}
			}
		}

		tflog.Debug(ctx, "a rule has been expanded", map[string]any{"rule": rule})
		result[i] = rule
	}

	return result, nil
}

// matchRulesWithSchema preserves declared ordering and appends any additional API rules.
func matchRulesWithSchema(
	ctx context.Context,
	rules []s3types.LifecycleRule,
	declared []LifecycleRuleModel,
) []s3types.LifecycleRule {
	tflog.Debug(ctx, "entering matchRulesWithSchema")

	result := make([]s3types.LifecycleRule, 0)
	ruleMap := make(map[string]s3types.LifecycleRule)
	for _, rule := range rules {
		if rule.ID != nil {
			ruleMap[*rule.ID] = rule
		}
	}

	for _, d := range declared {
		id := d.ID.ValueString()
		if id == "" {
			continue
		}
		if rule, ok := ruleMap[id]; ok {
			result = append(result, rule)
			delete(ruleMap, id)
		}
	}

	for _, rule := range ruleMap {
		tflog.Debug(ctx, "adding new rule", map[string]any{"rule": rule})
		result = append(result, rule)
	}

	return result
}

// certListEqual returns true when two CertModel slices are structurally equal.
func certListEqual(a, b []CertModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Certificate.Equal(b[i].Certificate) || !a[i].PrivateKey.Equal(b[i].PrivateKey) {
			return false
		}
	}
	return true
}

// lifecycleListEqual performs a shallow equality check on lifecycle rule lists.
func lifecycleListEqual(a, b []LifecycleRuleModel) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].ID.Equal(b[i].ID) ||
			!a[i].Enabled.Equal(b[i].Enabled) ||
			!a[i].Prefix.Equal(b[i].Prefix) ||
			!a[i].AbortIncompleteMultipartUploadDays.Equal(b[i].AbortIncompleteMultipartUploadDays) {
			return false
		}
	}
	return true
}

// validateRegion delegates to the existing validateRegion helper in helpers.go.
func (r *BucketResource) validateRegionIfPresent(ctx context.Context, region string) diag.Diagnostics {
	var diags diag.Diagnostics
	client := r.Meta.Client

	valid, suggestedRegions, err := validateRegion(ctx, region, client)
	if err != nil {
		diags.AddError("Failed to Validate Region", err.Error())
		return diags
	}

	if !valid {
		msg := fmt.Sprintf("Region '%s' is not valid for Object Storage.", region)
		if len(suggestedRegions) > 0 {
			msg += fmt.Sprintf(" Suggested regions: %s", strings.Join(suggestedRegions, ", "))
		}
		diags.AddError("Invalid Region", msg)
	}

	return diags
}

func (r *BucketResource) populateLogAttributes(ctx context.Context, model ResourceModel) context.Context {
	return helper.SetLogFieldBulk(ctx, map[string]any{
		"bucket":  model.Label.ValueString(),
		"cluster": model.Cluster.ValueString(),
	})
}
