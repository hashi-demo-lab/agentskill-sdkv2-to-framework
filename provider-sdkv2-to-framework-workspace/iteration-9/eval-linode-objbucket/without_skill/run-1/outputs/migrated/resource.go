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
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// Ensure interface compliance.
var _ resource.ResourceWithImportState = &Resource{}

// ---- Model types ----

// CertModel represents the cert block (MaxItems:1 → list with SizeAtMost(1)).
type CertModel struct {
	Certificate types.String `tfsdk:"certificate"`
	PrivateKey  types.String `tfsdk:"private_key"`
}

// ExpirationModel represents a lifecycle_rule expiration sub-block (MaxItems:1).
type ExpirationModel struct {
	Date                    types.String `tfsdk:"date"`
	Days                    types.Int64  `tfsdk:"days"`
	ExpiredObjectDeleteMark types.Bool   `tfsdk:"expired_object_delete_marker"`
}

// NoncurrentVersionExpirationModel represents noncurrent_version_expiration (MaxItems:1).
type NoncurrentVersionExpirationModel struct {
	Days types.Int64 `tfsdk:"days"`
}

// LifecycleRuleModel represents one lifecycle_rule list element.
type LifecycleRuleModel struct {
	ID                                 types.String                       `tfsdk:"id"`
	Prefix                             types.String                       `tfsdk:"prefix"`
	Enabled                            types.Bool                         `tfsdk:"enabled"`
	AbortIncompleteMultipartUploadDays types.Int64                        `tfsdk:"abort_incomplete_multipart_upload_days"`
	Expiration                         []ExpirationModel                  `tfsdk:"expiration"`
	NoncurrentVersionExpiration        []NoncurrentVersionExpirationModel `tfsdk:"noncurrent_version_expiration"`
}

// ResourceModel is the top-level model for linode_object_storage_bucket.
type ResourceModel struct {
	ID            types.String         `tfsdk:"id"`
	Label         types.String         `tfsdk:"label"`
	Cluster       types.String         `tfsdk:"cluster"`
	Region        types.String         `tfsdk:"region"`
	ACL           types.String         `tfsdk:"acl"`
	CORSEnabled   types.Bool           `tfsdk:"cors_enabled"`
	Endpoint      types.String         `tfsdk:"endpoint"`
	S3Endpoint    types.String         `tfsdk:"s3_endpoint"`
	EndpointType  types.String         `tfsdk:"endpoint_type"`
	Hostname      types.String         `tfsdk:"hostname"`
	Versioning    types.Bool           `tfsdk:"versioning"`
	AccessKey     types.String         `tfsdk:"access_key"`
	SecretKey     types.String         `tfsdk:"secret_key"`
	Cert          []CertModel          `tfsdk:"cert"`
	LifecycleRule []LifecycleRuleModel `tfsdk:"lifecycle_rule"`
}

// regionOrCluster returns the region if set, otherwise the cluster.
func (m *ResourceModel) regionOrCluster() string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}
	return m.Cluster.ValueString()
}

// getObjKeys resolves access/secret keys from (in order):
//  1. Keys explicitly set on the resource
//  2. Provider-level obj_access_key / obj_secret_key
//  3. Temporary keys (if obj_use_temp_keys is set)
func (m *ResourceModel) getObjKeys(
	ctx context.Context,
	client *linodego.Client,
	config *helper.FrameworkProviderModel,
	permission string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) (accessKey, secretKey string, teardown func()) {
	accessKey = m.AccessKey.ValueString()
	secretKey = m.SecretKey.ValueString()

	if accessKey != "" && secretKey != "" {
		return accessKey, secretKey, nil
	}

	// Try provider-level keys.
	if pkAccess := config.ObjAccessKey.ValueString(); pkAccess != "" {
		if pkSecret := config.ObjSecretKey.ValueString(); pkSecret != "" {
			return pkAccess, pkSecret, nil
		}
	}

	// Try temp keys.
	if config.ObjUseTempKeys.ValueBool() {
		regionOrCluster := m.regionOrCluster()
		label := m.Label.ValueString()

		createOpts := linodego.ObjectStorageKeyCreateOptions{
			Label: fmt.Sprintf("temp_%s_%v", label, time.Now().Unix()),
			BucketAccess: &[]linodego.ObjectStorageKeyBucketAccess{
				{
					BucketName:  label,
					Region:      regionOrCluster,
					Permissions: permission,
				},
			},
		}

		keys, err := client.CreateObjectStorageKey(ctx, createOpts)
		if err != nil {
			diags.AddError("Failed to create temporary object storage keys", err.Error())
			return "", "", nil
		}

		teardown = func() {
			if delErr := client.DeleteObjectStorageKey(ctx, keys.ID); delErr != nil {
				tflog.Warn(ctx, "Failed to clean up temporary object storage keys", map[string]any{
					"details": delErr,
				})
			}
		}
		return keys.AccessKey, keys.SecretKey, teardown
	}

	diags.AddError(
		"Keys Not Found",
		"`access_key` and `secret_key` are required but not configured. "+
			"Set them on the resource, at provider level (obj_access_key / obj_secret_key), "+
			"or enable obj_use_temp_keys.",
	)
	return "", "", nil
}

// ---- Schema ----

var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The id of the bucket (<region_or_cluster>:<label>).",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"label": schema.StringAttribute{
			Description: "The label of the Linode Object Storage Bucket.",
			Required:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"cluster": schema.StringAttribute{
			Description: "The cluster of the Linode Object Storage Bucket.",
			DeprecationMessage: "The cluster attribute has been deprecated, please consider switching to the region attribute. " +
				"For example, a cluster value of `us-mia-1` can be translated to a region value of `us-mia`.",
			Optional: true,
			Computed: true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
			Validators: []validator.String{
				stringvalidator.ExactlyOneOf(path.MatchRoot("region")),
			},
		},
		"region": schema.StringAttribute{
			Description: "The region of the Linode Object Storage Bucket.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
			Validators: []validator.String{
				stringvalidator.ExactlyOneOf(path.MatchRoot("cluster")),
			},
		},
		"acl": schema.StringAttribute{
			Description: "The Access Control Level of the bucket using a canned ACL string.",
			Optional:    true,
			Computed:    true,
			Default:     stringdefault.StaticString("private"),
		},
		"cors_enabled": schema.BoolAttribute{
			Description: "If true, the bucket will be created with CORS enabled for all origins.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		},
		"endpoint": schema.StringAttribute{
			Description:        "The endpoint for the bucket used for s3 connections.",
			DeprecationMessage: "Use `s3_endpoint` instead",
			Computed:           true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"s3_endpoint": schema.StringAttribute{
			Description: "The endpoint for the bucket used for s3 connections.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"endpoint_type": schema.StringAttribute{
			Description: "The type of the S3 endpoint available in this region.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"hostname": schema.StringAttribute{
			Description: "The hostname where this bucket can be accessed. " +
				"This hostname can be accessed through a browser if the bucket is made public.",
			Computed: true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"versioning": schema.BoolAttribute{
			Description: "Whether to enable versioning.",
			Optional:    true,
			Computed:    true,
			PlanModifiers: []planmodifier.Bool{
				boolplanmodifier.UseStateForUnknown(),
			},
		},
		"access_key": schema.StringAttribute{
			Description: "The S3 access key to use for this resource. (Required for lifecycle_rule and versioning). " +
				"If not specified with the resource, the value will be read from provider-level obj_access_key, " +
				"or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			Optional: true,
		},
		"secret_key": schema.StringAttribute{
			Description: "The S3 secret key to use for this resource. (Required for lifecycle_rule and versioning). " +
				"If not specified with the resource, the value will be read from provider-level obj_secret_key, " +
				"or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			Optional:  true,
			Sensitive: true,
		},
	},
	Blocks: map[string]schema.Block{
		// MaxItems:1 is expressed as a ListNestedBlock with SizeAtMost(1) validator.
		"cert": schema.ListNestedBlock{
			Description: "The cert used by this Object Storage Bucket.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"certificate": schema.StringAttribute{
						Description: "The Base64 encoded and PEM formatted SSL certificate.",
						Sensitive:   true,
						Required:    true,
					},
					"private_key": schema.StringAttribute{
						Description: "The private key associated with the TLS/SSL certificate.",
						Sensitive:   true,
						Required:    true,
					},
				},
			},
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
			PlanModifiers: []planmodifier.List{
				listplanmodifier.UseStateForUnknown(),
			},
		},
		"lifecycle_rule": schema.ListNestedBlock{
			Description: "Lifecycle rules to be applied to the bucket.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Description: "The unique identifier for the rule.",
						Optional:    true,
						Computed:    true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseStateForUnknown(),
						},
					},
					"prefix": schema.StringAttribute{
						Description: "The object key prefix identifying one or more objects to which the rule applies.",
						Optional:    true,
					},
					"enabled": schema.BoolAttribute{
						Description: "Specifies whether the lifecycle rule is active.",
						Required:    true,
					},
					"abort_incomplete_multipart_upload_days": schema.Int64Attribute{
						Description: "Specifies the number of days after initiating a multipart upload when the multipart " +
							"upload must be completed.",
						Optional: true,
					},
				},
				Blocks: map[string]schema.Block{
					// MaxItems:1 sub-block — expiration
					"expiration": schema.ListNestedBlock{
						Description: "Specifies a period in the object's expire.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"date": schema.StringAttribute{
									Description: "Specifies the date after which you want the corresponding action to take effect.",
									Optional:    true,
								},
								"days": schema.Int64Attribute{
									Description: "Specifies the number of days after object creation when the specific rule action takes effect.",
									Optional:    true,
								},
								"expired_object_delete_marker": schema.BoolAttribute{
									Description: "Directs Linode Object Storage to remove expired deleted markers.",
									Optional:    true,
								},
							},
						},
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
					},
					// MaxItems:1 sub-block — noncurrent_version_expiration
					"noncurrent_version_expiration": schema.ListNestedBlock{
						Description: "Specifies when non-current object versions expire.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"days": schema.Int64Attribute{
									Description: "Specifies the number of days non-current object versions expire.",
									Required:    true,
								},
							},
						},
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
					},
				},
			},
		},
	},
}

// ---- Constructor ----

func NewFrameworkResource() resource.Resource {
	return &Resource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_object_storage_bucket",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

// Resource is the framework resource implementation.
type Resource struct {
	helper.BaseResource
}

// ---- ImportState ----

// ImportState parses a composite "<cluster_or_region>:<label>" ID.
func (r *Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf(
				"Expected format <cluster_or_region>:<label>, got %q",
				req.ID,
			),
		)
		return
	}
	// Set id so that Read can parse it back.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// ---- CRUD ----

func (r *Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	tflog.Debug(ctx, "Create linode_object_storage_bucket")

	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	client := r.Meta.Client

	// Validate region if provided.
	if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
		if diags := fwValidateRegion(ctx, plan.Region.ValueString(), client); diags.HasError() {
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
		resp.Diagnostics.AddError(
			"Failed to create Linode ObjectStorageBucket",
			err.Error(),
		)
		return
	}

	endpoint := computeS3Endpoint(ctx, *bucket)
	plan.Endpoint = types.StringValue(endpoint)
	plan.S3Endpoint = types.StringValue(endpoint)
	plan.Cluster = types.StringValue(bucket.Cluster)
	plan.Region = types.StringValue(bucket.Region)
	plan.Label = types.StringValue(bucket.Label)

	if bucket.Region != "" {
		plan.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
	} else {
		plan.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
	}

	// Persist minimal state before attempting updates (prevents dangling resources).
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply mutable configuration.
	r.applyMutableConfig(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Final authoritative read.
	r.readInto(ctx, &plan, &resp.Diagnostics)
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
	tflog.Debug(ctx, "Read linode_object_storage_bucket")

	var data ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if helper.FrameworkAttemptRemoveResourceForEmptyID(ctx, data.ID, resp) {
		return
	}

	r.readInto(ctx, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the resource was removed from state (not found).
	if data.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	tflog.Debug(ctx, "Update linode_object_storage_bucket")

	var plan, state ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the stable ID.
	plan.ID = state.ID

	// Validate region if it changed.
	if !plan.Region.Equal(state.Region) {
		if !plan.Region.IsNull() && !plan.Region.IsUnknown() && plan.Region.ValueString() != "" {
			client := r.Meta.Client
			if diags := fwValidateRegion(ctx, plan.Region.ValueString(), client); diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}
		}
	}

	r.applyMutableConfig(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readInto(ctx, &plan, &resp.Diagnostics)
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
	tflog.Debug(ctx, "Delete linode_object_storage_bucket")

	var data ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	config := r.Meta.Config
	client := r.Meta.Client

	regionOrCluster, label, err := DecodeBucketIDString(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error parsing Linode ObjectStorageBucket id", err.Error())
		return
	}

	if config.ObjBucketForceDelete.ValueBool() {
		var endpointType *linodego.ObjectStorageEndpointType
		if !data.EndpointType.IsNull() && !data.EndpointType.IsUnknown() && data.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(data.EndpointType.ValueString())
			endpointType = &et
		}

		accessKey, secretKey, teardown := data.getObjKeys(
			ctx, client, config, "read_write", endpointType, &resp.Diagnostics,
		)
		if resp.Diagnostics.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		s3endpoint := data.S3Endpoint.ValueString()
		s3client, connErr := helper.S3Connection(ctx, s3endpoint, accessKey, secretKey)
		if connErr != nil {
			resp.Diagnostics.AddError("Failed to create S3 connection", connErr.Error())
			return
		}

		tflog.Debug(ctx, "helper.PurgeAllObjects(...)")
		if purgeErr := helper.PurgeAllObjects(ctx, label, s3client, true, true); purgeErr != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error purging all objects from ObjectStorageBucket: %s", data.ID.ValueString()),
				purgeErr.Error(),
			)
			return
		}
	}

	tflog.Debug(ctx, "client.DeleteObjectStorageBucket(...)")
	if delErr := client.DeleteObjectStorageBucket(ctx, regionOrCluster, label); delErr != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting Linode ObjectStorageBucket %s", data.ID.ValueString()),
			delErr.Error(),
		)
	}
}

// ---- Private orchestration ----

// readInto populates data from the API. Sets data.ID to null if not found.
func (r *Resource) readInto(ctx context.Context, data *ResourceModel, diags *diag.Diagnostics) {
	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster, label, err := DecodeBucketIDString(data.ID.ValueString())
	if err != nil {
		diags.AddError("Failed to parse Linode ObjectStorageBucket id", err.Error())
		return
	}

	bucket, err := client.GetObjectStorageBucket(ctx, regionOrCluster, label)
	if err != nil {
		if linodego.IsNotFound(err) {
			tflog.Warn(ctx, fmt.Sprintf(
				"removing Object Storage Bucket %q from state because it no longer exists",
				data.ID.ValueString(),
			))
			data.ID = types.StringNull()
			return
		}
		diags.AddError(
			"Failed to find the specified Linode ObjectStorageBucket",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "getting bucket access info")
	access, err := client.GetObjectStorageBucketAccessV2(ctx, regionOrCluster, label)
	if err != nil {
		diags.AddError(
			"Failed to find the access config for the specified Linode ObjectStorageBucket",
			err.Error(),
		)
		return
	}

	// Only read versioning/lifecycle when they are configured (non-null).
	needVersioning := !data.Versioning.IsNull() && !data.Versioning.IsUnknown()
	needLifecycle := len(data.LifecycleRule) > 0

	if needVersioning || needLifecycle {
		var endpointType *linodego.ObjectStorageEndpointType
		if !data.EndpointType.IsNull() && !data.EndpointType.IsUnknown() && data.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(data.EndpointType.ValueString())
			endpointType = &et
		}

		accessKey, secretKey, teardown := data.getObjKeys(
			ctx, client, config, "read_only", endpointType, diags,
		)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := computeS3Endpoint(ctx, *bucket)
		s3Client, s3err := helper.S3Connection(ctx, endpoint, accessKey, secretKey)
		if s3err != nil {
			diags.AddError("Failed to create S3 connection", s3err.Error())
			return
		}

		if needLifecycle {
			if lErr := readBucketLifecycleFW(ctx, data, s3Client); lErr != nil {
				diags.AddError("Failed to read object storage bucket lifecycle", lErr.Error())
				return
			}
		}

		if needVersioning {
			if vErr := readBucketVersioningFW(ctx, data, s3Client); vErr != nil {
				diags.AddError("Failed to read object storage bucket versioning", vErr.Error())
				return
			}
		}
	}

	// Update ID to the canonical form returned by the API.
	if bucket.Region != "" {
		data.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
	} else {
		data.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
	}

	endpoint := computeS3Endpoint(ctx, *bucket)

	data.Cluster = types.StringValue(bucket.Cluster)
	data.Region = types.StringValue(bucket.Region)
	data.Label = types.StringValue(bucket.Label)
	data.Hostname = types.StringValue(bucket.Hostname)
	data.ACL = types.StringValue(string(access.ACL))
	data.CORSEnabled = types.BoolPointerValue(access.CorsEnabled)
	data.Endpoint = types.StringValue(endpoint)
	data.S3Endpoint = types.StringValue(endpoint)
	data.EndpointType = types.StringValue(string(bucket.EndpointType))
}

// applyMutableConfig applies access/cert/versioning/lifecycle from plan.
func (r *Resource) applyMutableConfig(ctx context.Context, plan *ResourceModel, diags *diag.Diagnostics) {
	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster := plan.regionOrCluster()
	label := plan.Label.ValueString()

	// ---- Update access (ACL / CORS) ----
	updateOpts := linodego.ObjectStorageBucketUpdateAccessOptions{}
	updateOpts.ACL = linodego.ObjectStorageACL(plan.ACL.ValueString())
	if !plan.CORSEnabled.IsNull() && !plan.CORSEnabled.IsUnknown() {
		v := plan.CORSEnabled.ValueBool()
		updateOpts.CorsEnabled = &v
	}
	tflog.Debug(ctx, "client.UpdateObjectStorageBucketAccess(...)", map[string]any{"options": updateOpts})
	if err := client.UpdateObjectStorageBucketAccess(ctx, regionOrCluster, label, updateOpts); err != nil {
		diags.AddError("Failed to update bucket access", err.Error())
		return
	}

	// ---- Update cert ----
	if err := updateBucketCertFW(ctx, plan, client); err != nil {
		diags.AddError("Failed to update bucket certificate", err.Error())
		return
	}

	// ---- Update versioning / lifecycle (requires S3 connection) ----
	needVersioning := !plan.Versioning.IsNull() && !plan.Versioning.IsUnknown()
	// Lifecycle is updated whenever the resource has a lifecycle_rule block configured
	// (including an empty list, which deletes any existing lifecycle config).
	// We check if it was explicitly set (not unknown).
	needLifecycle := plan.LifecycleRule != nil

	if needVersioning || needLifecycle {
		var endpointType *linodego.ObjectStorageEndpointType
		if !plan.EndpointType.IsNull() && !plan.EndpointType.IsUnknown() && plan.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(plan.EndpointType.ValueString())
			endpointType = &et
		}

		accessKey, secretKey, teardown := plan.getObjKeys(
			ctx, client, config, "read_write", endpointType, diags,
		)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		s3endpoint := plan.S3Endpoint.ValueString()
		if s3endpoint == "" {
			b, bErr := client.GetObjectStorageBucket(ctx, regionOrCluster, label)
			if bErr != nil {
				diags.AddError("Failed to get bucket for S3 connection", bErr.Error())
				return
			}
			s3endpoint = computeS3Endpoint(ctx, *b)
			plan.S3Endpoint = types.StringValue(s3endpoint)
			plan.Endpoint = types.StringValue(s3endpoint)
		}

		s3client, connErr := helper.S3Connection(ctx, s3endpoint, accessKey, secretKey)
		if connErr != nil {
			diags.AddError("Failed to create S3 connection", connErr.Error())
			return
		}

		if needVersioning {
			if vErr := updateBucketVersioningFW(ctx, plan, s3client); vErr != nil {
				diags.AddError("Failed to update bucket versioning", vErr.Error())
				return
			}
		}

		if lErr := updateBucketLifecycleFW(ctx, plan, s3client); lErr != nil {
			diags.AddError("Failed to update bucket lifecycle", lErr.Error())
			return
		}
	}
}

// ---- S3 / lifecycle / versioning helpers ----

func readBucketVersioningFW(ctx context.Context, data *ResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()
	out, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &label})
	if err != nil {
		return fmt.Errorf("failed to get versioning for bucket %s: %s", data.ID.ValueString(), err)
	}
	data.Versioning = types.BoolValue(out.Status == s3types.BucketVersioningStatusEnabled)
	return nil
}

func readBucketLifecycleFW(ctx context.Context, data *ResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()
	out, err := client.GetBucketLifecycleConfiguration(
		ctx,
		&s3.GetBucketLifecycleConfigurationInput{Bucket: &label},
	)
	if err != nil {
		var ae smithy.APIError
		if ok := errors.As(err, &ae); !ok || ae.ErrorCode() != "NoSuchLifecycleConfiguration" {
			return fmt.Errorf("failed to get lifecycle for bucket %s: %w", data.ID.ValueString(), err)
		}
	}
	if out == nil {
		data.LifecycleRule = nil
		return nil
	}

	rules := out.Rules
	// Preserve ordering from declared state.
	if len(data.LifecycleRule) > 0 {
		rules = matchRulesWithSchemaFW(ctx, rules, data.LifecycleRule)
	}

	data.LifecycleRule = flattenLifecycleRulesFW(ctx, rules)
	return nil
}

func updateBucketVersioningFW(ctx context.Context, data *ResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()
	status := s3types.BucketVersioningStatusSuspended
	if data.Versioning.ValueBool() {
		status = s3types.BucketVersioningStatusEnabled
	}
	tflog.Debug(ctx, "client.PutBucketVersioning(...)")
	_, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: &label,
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: status,
		},
	})
	return err
}

func updateBucketLifecycleFW(ctx context.Context, data *ResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()
	rules, err := expandLifecycleRulesFW(ctx, data.LifecycleRule)
	if err != nil {
		return err
	}

	if len(rules) > 0 {
		tflog.Debug(ctx, "client.PutBucketLifecycleConfiguration(...)")
		_, err = client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
			Bucket: &label,
			LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
				Rules: rules,
			},
		})
	} else {
		tflog.Debug(ctx, "client.DeleteBucketLifecycle(...)")
		_, err = client.DeleteBucketLifecycle(ctx, &s3.DeleteBucketLifecycleInput{Bucket: &label})
	}
	return err
}

func updateBucketCertFW(ctx context.Context, data *ResourceModel, client *linodego.Client) error {
	regionOrCluster := data.regionOrCluster()
	label := data.Label.ValueString()

	if len(data.Cert) == 0 {
		// No cert desired — delete if one exists (ignore 404).
		_ = client.DeleteObjectStorageBucketCert(ctx, regionOrCluster, label)
		return nil
	}

	// Delete existing, then upload new.
	_ = client.DeleteObjectStorageBucketCert(ctx, regionOrCluster, label)

	uploadOptions := linodego.ObjectStorageBucketCertUploadOptions{
		Certificate: data.Cert[0].Certificate.ValueString(),
		PrivateKey:  data.Cert[0].PrivateKey.ValueString(),
	}
	if _, err := client.UploadObjectStorageBucketCertV2(ctx, regionOrCluster, label, uploadOptions); err != nil {
		return fmt.Errorf("failed to upload new bucket cert: %s", err)
	}
	return nil
}

// ---- Flatten / Expand lifecycle rules ----

func flattenLifecycleRulesFW(ctx context.Context, rules []s3types.LifecycleRule) []LifecycleRuleModel {
	result := make([]LifecycleRuleModel, len(rules))
	for i, rule := range rules {
		m := LifecycleRuleModel{}
		if rule.ID != nil {
			m.ID = types.StringValue(*rule.ID)
		}
		if rule.Prefix != nil {
			m.Prefix = types.StringValue(*rule.Prefix)
		}
		m.Enabled = types.BoolValue(rule.Status == s3types.ExpirationStatusEnabled)

		if rule.AbortIncompleteMultipartUpload != nil && rule.AbortIncompleteMultipartUpload.DaysAfterInitiation != nil {
			m.AbortIncompleteMultipartUploadDays = types.Int64Value(int64(*rule.AbortIncompleteMultipartUpload.DaysAfterInitiation))
		}

		if rule.Expiration != nil {
			exp := ExpirationModel{}
			if rule.Expiration.Date != nil {
				exp.Date = types.StringValue(rule.Expiration.Date.Format("2006-01-02"))
			}
			if rule.Expiration.Days != nil {
				exp.Days = types.Int64Value(int64(*rule.Expiration.Days))
			}
			if rule.Expiration.ExpiredObjectDeleteMarker != nil {
				exp.ExpiredObjectDeleteMark = types.BoolValue(*rule.Expiration.ExpiredObjectDeleteMarker)
			}
			m.Expiration = []ExpirationModel{exp}
		}

		if rule.NoncurrentVersionExpiration != nil {
			nce := NoncurrentVersionExpirationModel{}
			if rule.NoncurrentVersionExpiration.NoncurrentDays != nil && *rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
				nce.Days = types.Int64Value(int64(*rule.NoncurrentVersionExpiration.NoncurrentDays))
			}
			m.NoncurrentVersionExpiration = []NoncurrentVersionExpirationModel{nce}
		}

		tflog.Debug(ctx, "a rule has been flattened")
		result[i] = m
	}
	return result
}

func expandLifecycleRulesFW(ctx context.Context, ruleModels []LifecycleRuleModel) ([]s3types.LifecycleRule, error) {
	rules := make([]s3types.LifecycleRule, len(ruleModels))
	for i, m := range ruleModels {
		rule := s3types.LifecycleRule{}

		status := s3types.ExpirationStatusDisabled
		if m.Enabled.ValueBool() {
			status = s3types.ExpirationStatusEnabled
		}
		rule.Status = status

		if !m.ID.IsNull() && m.ID.ValueString() != "" {
			id := m.ID.ValueString()
			rule.ID = &id
		}

		if !m.Prefix.IsNull() {
			prefix := m.Prefix.ValueString()
			rule.Prefix = &prefix
		}

		if !m.AbortIncompleteMultipartUploadDays.IsNull() && !m.AbortIncompleteMultipartUploadDays.IsUnknown() {
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
			exp := m.Expiration[0]
			rule.Expiration = &s3types.LifecycleExpiration{}

			if !exp.Date.IsNull() && !exp.Date.IsUnknown() && exp.Date.ValueString() != "" {
				date, err := time.Parse(time.RFC3339, fmt.Sprintf("%sT00:00:00Z", exp.Date.ValueString()))
				if err != nil {
					return nil, err
				}
				rule.Expiration.Date = &date
			}

			if !exp.Days.IsNull() && !exp.Days.IsUnknown() {
				d := exp.Days.ValueInt64()
				if d > 0 {
					int32Days, err := helper.SafeIntToInt32(int(d))
					if err != nil {
						return nil, err
					}
					rule.Expiration.Days = &int32Days
				}
			}

			if !exp.ExpiredObjectDeleteMark.IsNull() && !exp.ExpiredObjectDeleteMark.IsUnknown() && exp.ExpiredObjectDeleteMark.ValueBool() {
				marker := true
				rule.Expiration.ExpiredObjectDeleteMarker = &marker
			}
		}

		if len(m.NoncurrentVersionExpiration) > 0 {
			nce := m.NoncurrentVersionExpiration[0]
			rule.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{}
			if !nce.Days.IsNull() && !nce.Days.IsUnknown() {
				d := nce.Days.ValueInt64()
				if d > 0 {
					int32Days, err := helper.SafeIntToInt32(int(d))
					if err != nil {
						return nil, err
					}
					rule.NoncurrentVersionExpiration.NoncurrentDays = &int32Days
				}
			}
		}

		tflog.Debug(ctx, "a rule has been expanded", map[string]any{"rule": rule})
		rules[i] = rule
	}
	return rules, nil
}

// matchRulesWithSchemaFW preserves the order of declared rules and appends remaining.
func matchRulesWithSchemaFW(
	ctx context.Context,
	rules []s3types.LifecycleRule,
	declared []LifecycleRuleModel,
) []s3types.LifecycleRule {
	result := make([]s3types.LifecycleRule, 0)
	ruleMap := make(map[string]s3types.LifecycleRule)
	for _, rule := range rules {
		if rule.ID != nil {
			ruleMap[*rule.ID] = rule
		}
	}

	for _, dec := range declared {
		if dec.ID.IsNull() || dec.ID.ValueString() == "" {
			continue
		}
		id := dec.ID.ValueString()
		if rule, ok := ruleMap[id]; ok {
			result = append(result, rule)
			delete(ruleMap, id)
		}
	}

	for _, rule := range ruleMap {
		tflog.Debug(ctx, "adding new rules", map[string]any{"rule": rule})
		result = append(result, rule)
	}
	return result
}

// ---- Standalone utilities ----

// DecodeBucketIDString parses "<regionOrCluster>:<label>" from a string ID.
func DecodeBucketIDString(id string) (regionOrCluster, label string, err error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf(
		"Linode Object Storage Bucket ID must be of the form <ClusterOrRegion>:<Label>, got %q", id,
	)
}

// computeS3Endpoint returns the S3 endpoint for a bucket.
func computeS3Endpoint(ctx context.Context, bucket linodego.ObjectStorageBucket) string {
	if bucket.S3Endpoint != "" {
		return bucket.S3Endpoint
	}
	return helper.ComputeS3EndpointFromBucket(ctx, bucket)
}

// fwValidateRegion validates that the given region is valid for Object Storage.
func fwValidateRegion(ctx context.Context, region string, client *linodego.Client) diag.Diagnostics {
	var diags diag.Diagnostics
	valid, suggestedRegions, err := validateRegion(ctx, region, client)
	if err != nil {
		diags.AddError("Failed to validate region", err.Error())
		return diags
	}
	if !valid {
		errMsg := fmt.Sprintf("Region '%s' is not valid for Object Storage.", region)
		if len(suggestedRegions) > 0 {
			errMsg += fmt.Sprintf(" Suggested regions: %s", strings.Join(suggestedRegions, ", "))
		}
		diags.AddError("Invalid region", errMsg)
	}
	return diags
}

