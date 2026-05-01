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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/linode/linodego"
	"github.com/linode/terraform-provider-linode/v3/linode/helper"
)

// bucketObjectKeys holds S3 access credentials.
type bucketObjectKeys struct {
	AccessKey string
	SecretKey string
}

func (k *bucketObjectKeys) ok() bool {
	return k.AccessKey != "" && k.SecretKey != ""
}

var (
	_ resource.Resource                = &BucketResource{}
	_ resource.ResourceWithImportState = &BucketResource{}
)

func NewResource() resource.Resource {
	return &BucketResource{
		BaseResource: helper.NewBaseResource(
			helper.BaseResourceConfig{
				Name:   "linode_object_storage_bucket",
				IDType: types.StringType,
				Schema: &frameworkResourceSchema,
			},
		),
	}
}

type BucketResource struct {
	helper.BaseResource
}

// frameworkResourceSchema is the framework schema for the object storage bucket resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The ID of the Linode Object Storage Bucket in the form <cluster>:<label>.",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"secret_key": schema.StringAttribute{
			Description: "The S3 secret key to use for this resource. (Required for lifecycle_rule and versioning). " +
				"If not specified with the resource, the value will be read from provider-level obj_secret_key, " +
				"or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			Optional:  true,
			Sensitive: true,
		},
		"access_key": schema.StringAttribute{
			Description: "The S3 access key to use for this resource. (Required for lifecycle_rule and versioning). " +
				"If not specified with the resource, the value will be read from provider-level obj_access_key, " +
				"or, generated implicitly at apply-time if obj_use_temp_keys in provider configuration is set.",
			Optional: true,
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
		"label": schema.StringAttribute{
			Description: "The label of the Linode Object Storage Bucket.",
			Required:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
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
	},
	Blocks: map[string]schema.Block{
		// cert: MaxItems:1 — kept as ListNestedBlock + SizeAtMost(1) to preserve block HCL syntax
		"cert": schema.ListNestedBlock{
			Description: "The cert used by this Object Storage Bucket.",
			Validators: []validator.List{
				listvalidator.SizeAtMost(1),
			},
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
		},
		// lifecycle_rule: repeating block (no MaxItems:1) — kept as ListNestedBlock
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
						Description: "Specifies the number of days after initiating a multipart upload when the multipart upload must be completed.",
						Optional:    true,
					},
				},
				Blocks: map[string]schema.Block{
					// expiration: MaxItems:1 — kept as ListNestedBlock + SizeAtMost(1)
					"expiration": schema.ListNestedBlock{
						Description: "Specifies a period in the object's expire.",
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
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
					},
					// noncurrent_version_expiration: MaxItems:1 — kept as ListNestedBlock + SizeAtMost(1)
					"noncurrent_version_expiration": schema.ListNestedBlock{
						Description: "Specifies when non-current object versions expire.",
						Validators: []validator.List{
							listvalidator.SizeAtMost(1),
						},
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"days": schema.Int64Attribute{
									Description: "Specifies the number of days non-current object versions expire.",
									Required:    true,
								},
							},
						},
					},
				},
			},
		},
	},
}

// ResourceModel is the framework model for the object storage bucket resource.
type ResourceModel struct {
	ID           types.String          `tfsdk:"id"`
	SecretKey    types.String          `tfsdk:"secret_key"`
	AccessKey    types.String          `tfsdk:"access_key"`
	Cluster      types.String          `tfsdk:"cluster"`
	Region       types.String          `tfsdk:"region"`
	Endpoint     types.String          `tfsdk:"endpoint"`
	S3Endpoint   types.String          `tfsdk:"s3_endpoint"`
	EndpointType types.String          `tfsdk:"endpoint_type"`
	Label        types.String          `tfsdk:"label"`
	ACL          types.String          `tfsdk:"acl"`
	CORSEnabled  types.Bool            `tfsdk:"cors_enabled"`
	Hostname     types.String          `tfsdk:"hostname"`
	Versioning   types.Bool            `tfsdk:"versioning"`
	Cert         []CertModel           `tfsdk:"cert"`
	LifecycleRule []LifecycleRuleModel `tfsdk:"lifecycle_rule"`
}

type CertModel struct {
	Certificate types.String `tfsdk:"certificate"`
	PrivateKey  types.String `tfsdk:"private_key"`
}

type LifecycleRuleModel struct {
	ID                               types.String                     `tfsdk:"id"`
	Prefix                           types.String                     `tfsdk:"prefix"`
	Enabled                          types.Bool                       `tfsdk:"enabled"`
	AbortIncompleteMultipartUploadDays types.Int64                   `tfsdk:"abort_incomplete_multipart_upload_days"`
	Expiration                       []LifecycleExpirationModel       `tfsdk:"expiration"`
	NoncurrentVersionExpiration      []NoncurrentVersionExpirationModel `tfsdk:"noncurrent_version_expiration"`
}

type LifecycleExpirationModel struct {
	Date                    types.String `tfsdk:"date"`
	Days                    types.Int64  `tfsdk:"days"`
	ExpiredObjectDeleteMarker types.Bool `tfsdk:"expired_object_delete_marker"`
}

type NoncurrentVersionExpirationModel struct {
	Days types.Int64 `tfsdk:"days"`
}

func (r *BucketResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	// composite ID: cluster:label
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected 'cluster_or_region:label', got %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("label"), parts[1])...)
	// Determine if region or cluster based on format (isCluster check)
	// We set both; Read will populate the correct one from the API response.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("region"), parts[0])...)
}

func (r *BucketResource) Create(
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
	ctx = r.populateLogAttributes(ctx, plan)

	if !plan.Region.IsNull() && !plan.Region.IsUnknown() {
		r.validateRegionIfPresent(ctx, plan.Region.ValueString(), client, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
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
		resp.Diagnostics.AddError("Failed to create a Linode ObjectStorageBucket", err.Error())
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

	// Write partial state so resource is tracked even if update fails
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.update(ctx, &plan, client, r.Meta.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &plan, client, r.Meta.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	tflog.Debug(ctx, "Read linode_object_storage_bucket")

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, state)

	r.read(ctx, &state, r.Meta.Client, r.Meta.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// If ID was cleared by read (not found), remove from state
	if state.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *BucketResource) Update(
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

	ctx = r.populateLogAttributes(ctx, plan)
	client := r.Meta.Client

	// Carry over computed/stable fields from state
	plan.ID = state.ID
	plan.Hostname = state.Hostname
	plan.Cluster = state.Cluster
	plan.Region = state.Region

	r.update(ctx, &plan, client, r.Meta.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &plan, client, r.Meta.Config, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *BucketResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	tflog.Debug(ctx, "Delete linode_object_storage_bucket")

	var state ResourceModel
	// Read from State (not Plan — Plan is null on Delete)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = r.populateLogAttributes(ctx, state)
	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster, label, err := decodeBucketIDFromModel(state)
	if err != nil {
		resp.Diagnostics.AddError("Error parsing Linode ObjectStorageBucket id", err.Error())
		return
	}

	if config.ObjBucketForceDelete.ValueBool() {
		var endpointType *linodego.ObjectStorageEndpointType
		if !state.EndpointType.IsNull() && !state.EndpointType.IsUnknown() && state.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(state.EndpointType.ValueString())
			endpointType = &et
		}

		objKeys, teardown := r.getObjKeys(
			ctx, client, config, state, label, regionOrCluster, "read_write", endpointType, &resp.Diagnostics,
		)
		if resp.Diagnostics.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := state.S3Endpoint.ValueString()
		s3client, err := helper.S3Connection(ctx, endpoint, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to create S3 connection", err.Error())
			return
		}

		tflog.Debug(ctx, "helper.PurgeAllObjects(...)")
		if err := helper.PurgeAllObjects(ctx, label, s3client, true, true); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error purging all objects from ObjectStorageBucket: %s", state.ID.ValueString()),
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

// read populates the model from the API. Used by Read, Create (after create), and Update (after update).
func (r *BucketResource) read(
	ctx context.Context,
	data *ResourceModel,
	client *linodego.Client,
	config *helper.FrameworkProviderModel,
	diags *diag.Diagnostics,
) {
	regionOrCluster, label, err := decodeBucketIDFromModel(*data)
	if err != nil {
		diags.AddError("Failed to parse Linode ObjectStorageBucket id", err.Error())
		return
	}

	bucket, err := client.GetObjectStorageBucket(ctx, regionOrCluster, label)
	if err != nil {
		if linodego.IsNotFound(err) {
			tflog.Warn(ctx, fmt.Sprintf(
				"[WARN] removing Object Storage Bucket %q from state because it no longer exists",
				data.ID.ValueString(),
			))
			// Signal removal by setting ID to null (caller should remove from state)
			data.ID = types.StringNull()
			return
		}
		diags.AddError("Failed to find the specified Linode ObjectStorageBucket", err.Error())
		return
	}

	tflog.Debug(ctx, "getting bucket access info")
	access, err := client.GetObjectStorageBucketAccessV2(ctx, regionOrCluster, label)
	if err != nil {
		diags.AddError("Failed to find the access config for the specified Linode ObjectStorageBucket", err.Error())
		return
	}

	// Populate versioning and lifecycle via S3 if they are configured
	versioningPresent := !data.Versioning.IsNull() && !data.Versioning.IsUnknown()
	lifecyclePresent := len(data.LifecycleRule) > 0

	if versioningPresent || lifecyclePresent {
		tflog.Debug(ctx, "versioning or lifecycle presents", map[string]any{
			"versioningPresent": versioningPresent,
			"lifecyclePresent":  lifecyclePresent,
		})

		var endpointType *linodego.ObjectStorageEndpointType
		if !data.EndpointType.IsNull() && !data.EndpointType.IsUnknown() && data.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(data.EndpointType.ValueString())
			endpointType = &et
		}

		objKeys, teardown := r.getObjKeys(
			ctx, client, config, *data, label, regionOrCluster, "read_only", endpointType, diags,
		)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := getS3Endpoint(ctx, *bucket)
		s3client, err := helper.S3Connection(ctx, endpoint, objKeys.AccessKey, objKeys.SecretKey)
		if err != nil {
			diags.AddError("Failed to create S3 connection", err.Error())
			return
		}

		tflog.Trace(ctx, "getting bucket lifecycle")
		if err := r.readBucketLifecycle(ctx, data, s3client); err != nil {
			diags.AddError("Failed to get object storage bucket lifecycle", err.Error())
			return
		}

		tflog.Trace(ctx, "getting bucket versioning")
		if err := r.readBucketVersioning(ctx, data, s3client); err != nil {
			diags.AddError("Failed to get object storage bucket versioning", err.Error())
			return
		}
	}

	endpoint := getS3Endpoint(ctx, *bucket)

	if bucket.Region != "" {
		data.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Region, bucket.Label))
		data.Region = types.StringValue(bucket.Region)
		data.Cluster = types.StringValue(bucket.Cluster)
	} else {
		data.ID = types.StringValue(fmt.Sprintf("%s:%s", bucket.Cluster, bucket.Label))
		data.Cluster = types.StringValue(bucket.Cluster)
		data.Region = types.StringValue(bucket.Region)
	}

	data.Label = types.StringValue(bucket.Label)
	data.Hostname = types.StringValue(bucket.Hostname)
	data.ACL = types.StringValue(string(access.ACL))
	if access.CorsEnabled != nil {
		data.CORSEnabled = types.BoolValue(*access.CorsEnabled)
	} else {
		data.CORSEnabled = types.BoolValue(false)
	}
	data.Endpoint = types.StringValue(endpoint)
	data.S3Endpoint = types.StringValue(endpoint)
	data.EndpointType = types.StringValue(string(bucket.EndpointType))
}

// update applies changes to ACL/CORS, cert, versioning, and lifecycle.
func (r *BucketResource) update(
	ctx context.Context,
	data *ResourceModel,
	client *linodego.Client,
	config *helper.FrameworkProviderModel,
	diags *diag.Diagnostics,
) {
	regionOrCluster, label, err := decodeBucketIDFromModel(*data)
	if err != nil {
		diags.AddError("Error parsing Linode ObjectStorageBucket id", err.Error())
		return
	}

	// Update ACL/CORS
	r.updateBucketAccess(ctx, data, regionOrCluster, label, client, diags)
	if diags.HasError() {
		return
	}

	// Update cert
	r.updateBucketCert(ctx, data, regionOrCluster, label, client, diags)
	if diags.HasError() {
		return
	}

	// Update versioning and lifecycle via S3
	versioningOrLifecyclePresent := !data.Versioning.IsNull() || len(data.LifecycleRule) > 0
	if !versioningOrLifecyclePresent {
		return
	}

	var endpointType *linodego.ObjectStorageEndpointType
	if !data.EndpointType.IsNull() && !data.EndpointType.IsUnknown() && data.EndpointType.ValueString() != "" {
		et := linodego.ObjectStorageEndpointType(data.EndpointType.ValueString())
		endpointType = &et
	}

	objKeys, teardown := r.getObjKeys(
		ctx, client, config, *data, label, regionOrCluster, "read_write", endpointType, diags,
	)
	if diags.HasError() {
		return
	}
	if teardown != nil {
		defer teardown()
	}

	s3client, err := helper.S3Connection(ctx, data.S3Endpoint.ValueString(), objKeys.AccessKey, objKeys.SecretKey)
	if err != nil {
		diags.AddError("Failed to create S3 connection", err.Error())
		return
	}

	if !data.Versioning.IsNull() && !data.Versioning.IsUnknown() {
		tflog.Debug(ctx, "Updating bucket versioning configuration")
		if err := r.updateBucketVersioning(ctx, data, s3client); err != nil {
			diags.AddError("Failed to update bucket versioning", err.Error())
			return
		}
	}

	tflog.Debug(ctx, "Updating bucket lifecycle configuration")
	if err := r.updateBucketLifecycle(ctx, data, s3client); err != nil {
		diags.AddError("Failed to update bucket lifecycle", err.Error())
	}
}

func (r *BucketResource) updateBucketAccess(
	ctx context.Context,
	data *ResourceModel,
	regionOrCluster, label string,
	client *linodego.Client,
	diags *diag.Diagnostics,
) {
	tflog.Debug(ctx, "entering updateBucketAccess")

	updateOpts := linodego.ObjectStorageBucketUpdateAccessOptions{}
	if !data.ACL.IsNull() && !data.ACL.IsUnknown() {
		updateOpts.ACL = linodego.ObjectStorageACL(data.ACL.ValueString())
	}
	if !data.CORSEnabled.IsNull() && !data.CORSEnabled.IsUnknown() {
		v := data.CORSEnabled.ValueBool()
		updateOpts.CorsEnabled = &v
	}

	tflog.Debug(ctx, "client.UpdateObjectStorageBucketAccess(...)", map[string]any{"options": updateOpts})
	if err := client.UpdateObjectStorageBucketAccess(ctx, regionOrCluster, label, updateOpts); err != nil {
		diags.AddError("Failed to update bucket access", err.Error())
	}
}

func (r *BucketResource) updateBucketCert(
	ctx context.Context,
	data *ResourceModel,
	regionOrCluster, label string,
	client *linodego.Client,
	diags *diag.Diagnostics,
) {
	// len(data.Cert) == 0 means no cert block; nothing to do unless removing cert
	if len(data.Cert) == 0 {
		// Try deleting existing cert (if any); ignore 404
		if err := client.DeleteObjectStorageBucketCert(ctx, regionOrCluster, label); err != nil {
			// Ignore not-found errors when removing
			if !linodego.IsNotFound(err) {
				diags.AddError("Failed to delete old bucket cert", err.Error())
			}
		}
		return
	}

	certSpec := data.Cert[0]

	// Delete any existing cert before uploading the new one
	if err := client.DeleteObjectStorageBucketCert(ctx, regionOrCluster, label); err != nil {
		if !linodego.IsNotFound(err) {
			diags.AddError("Failed to delete old bucket cert", err.Error())
			return
		}
	}

	// Upload the new certificate
	uploadOptions := linodego.ObjectStorageBucketCertUploadOptions{
		Certificate: certSpec.Certificate.ValueString(),
		PrivateKey:  certSpec.PrivateKey.ValueString(),
	}
	if _, err := client.UploadObjectStorageBucketCertV2(ctx, regionOrCluster, label, uploadOptions); err != nil {
		diags.AddError("Failed to upload new bucket cert", err.Error())
	}
}

func (r *BucketResource) readBucketVersioning(ctx context.Context, data *ResourceModel, client *s3.Client) error {
	tflog.Trace(ctx, "entering readBucketVersioning")
	label := data.Label.ValueString()

	versioningOutput, err := client.GetBucketVersioning(
		ctx,
		&s3.GetBucketVersioningInput{Bucket: &label},
	)
	if err != nil {
		return fmt.Errorf("failed to get versioning for bucket %s: %s", label, err)
	}

	data.Versioning = types.BoolValue(versioningOutput.Status == s3types.BucketVersioningStatusEnabled)
	return nil
}

func (r *BucketResource) readBucketLifecycle(ctx context.Context, data *ResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()

	lifecycleConfigOutput, err := client.GetBucketLifecycleConfiguration(
		ctx,
		&s3.GetBucketLifecycleConfigurationInput{Bucket: &label},
	)
	if err != nil {
		var ae smithy.APIError
		if ok := errors.As(err, &ae); !ok || ae.ErrorCode() != "NoSuchLifecycleConfiguration" {
			return fmt.Errorf("failed to get lifecycle for bucket %s: %w", label, err)
		}
	}

	if lifecycleConfigOutput == nil {
		tflog.Debug(ctx, "'lifecycleConfigOutput' is nil, skipping further processing")
		data.LifecycleRule = nil
		return nil
	}

	rulesMatched := lifecycleConfigOutput.Rules
	// Match the order of existing rules in the model
	rulesMatched = matchRulesWithModelRules(ctx, rulesMatched, data.LifecycleRule)
	data.LifecycleRule = flattenLifecycleRulesFramework(ctx, rulesMatched)
	return nil
}

func (r *BucketResource) updateBucketVersioning(
	ctx context.Context,
	data *ResourceModel,
	client *s3.Client,
) error {
	bucket := data.Label.ValueString()
	n := data.Versioning.ValueBool()

	status := s3types.BucketVersioningStatusSuspended
	if n {
		status = s3types.BucketVersioningStatusEnabled
	}

	inputVersioningConfig := &s3.PutBucketVersioningInput{
		Bucket: &bucket,
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: status,
		},
	}
	tflog.Debug(ctx, "client.PutBucketVersioning(...)", map[string]any{
		"options": inputVersioningConfig,
	})
	if _, err := client.PutBucketVersioning(ctx, inputVersioningConfig); err != nil {
		return err
	}
	return nil
}

func (r *BucketResource) updateBucketLifecycle(
	ctx context.Context,
	data *ResourceModel,
	client *s3.Client,
) error {
	bucket := data.Label.ValueString()

	rules, err := expandLifecycleRulesFramework(ctx, data.LifecycleRule)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, "got expanded lifecycle rules", map[string]any{
		"rules": rules,
	})
	if len(rules) > 0 {
		tflog.Debug(ctx, "there is at least one rule, calling the put endpoint")
		_, err = client.PutBucketLifecycleConfiguration(
			ctx,
			&s3.PutBucketLifecycleConfigurationInput{
				Bucket: &bucket,
				LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
					Rules: rules,
				},
			},
		)
	} else {
		options := &s3.DeleteBucketLifecycleInput{Bucket: &bucket}
		tflog.Debug(ctx, "client.DeleteBucketLifecycle(...)", map[string]any{
			"options": options,
		})
		_, err = client.DeleteBucketLifecycle(ctx, options)
	}
	return err
}

// getObjKeys retrieves S3 keys from the model, provider config, or generates temp keys.
func (r *BucketResource) getObjKeys(
	ctx context.Context,
	client *linodego.Client,
	config *helper.FrameworkProviderModel,
	data ResourceModel,
	bucket, regionOrCluster, permission string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) (*bucketObjectKeys, func()) {
	result := &bucketObjectKeys{
		AccessKey: data.AccessKey.ValueString(),
		SecretKey: data.SecretKey.ValueString(),
	}

	if result.ok() {
		return result, nil
	}

	result.AccessKey = config.ObjAccessKey.ValueString()
	result.SecretKey = config.ObjSecretKey.ValueString()
	if result.ok() {
		return result, nil
	}

	if config.ObjUseTempKeys.ValueBool() {
		objKey := fwCreateTempObjKey(ctx, client, bucket, regionOrCluster, permission, endpointType, diags)
		if diags.HasError() {
			return nil, nil
		}

		result.AccessKey = objKey.AccessKey
		result.SecretKey = objKey.SecretKey

		teardown := func() {
			fwCleanUpTempObjKey(ctx, client, objKey.ID)
		}
		return result, teardown
	}

	diags.AddError("Missing S3 credentials",
		"access_key and secret_key are required when obj_access_key/obj_secret_key are not set in the provider config "+
			"and obj_use_temp_keys is not enabled.")
	return nil, nil
}

// fwCreateTempObjKey creates a scoped temporary key for a bucket.
func fwCreateTempObjKey(
	ctx context.Context,
	client *linodego.Client,
	bucketLabel, regionOrCluster, permissions string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) *linodego.ObjectStorageKey {
	tempBucketAccess := linodego.ObjectStorageKeyBucketAccess{
		BucketName:  bucketLabel,
		Permissions: permissions,
	}

	// Cluster vs region determination
	clusterPattern := func(s string) bool {
		// Clusters match: two-letter code + region-name + number (e.g. us-east-1)
		return len(strings.Split(s, "-")) >= 3
	}
	if clusterPattern(regionOrCluster) {
		tempBucketAccess.Cluster = regionOrCluster
	} else {
		tempBucketAccess.Region = regionOrCluster
	}

	createOpts := linodego.ObjectStorageKeyCreateOptions{
		Label:        fmt.Sprintf("temp_%s_%v", bucketLabel, time.Now().Unix()),
		BucketAccess: &[]linodego.ObjectStorageKeyBucketAccess{tempBucketAccess},
	}

	keys, err := client.CreateObjectStorageKey(ctx, createOpts)
	if err != nil {
		diags.AddError("Failed to Create Temporary Object Storage Key", err.Error())
		return nil
	}
	return keys
}

// fwCleanUpTempObjKey deletes a temporary key by ID.
func fwCleanUpTempObjKey(ctx context.Context, client *linodego.Client, keyID int) {
	if err := client.DeleteObjectStorageKey(ctx, keyID); err != nil {
		tflog.Warn(ctx, fmt.Sprintf("Failed to clean up temporary Object Storage Key %d: %s", keyID, err))
	}
}

// decodeBucketIDFromModel extracts cluster/region and label from the model.
func decodeBucketIDFromModel(data ResourceModel) (regionOrCluster, label string, err error) {
	id := data.ID.ValueString()
	if id != "" {
		parts := strings.SplitN(id, ":", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], nil
		}
	}

	// Fall back to individual attributes
	if !data.Region.IsNull() && data.Region.ValueString() != "" {
		regionOrCluster = data.Region.ValueString()
	} else if !data.Cluster.IsNull() && data.Cluster.ValueString() != "" {
		regionOrCluster = data.Cluster.ValueString()
	}
	label = data.Label.ValueString()

	if regionOrCluster == "" || label == "" {
		return "", "", fmt.Errorf(
			"Linode Object Storage Bucket ID must be of the form <ClusterOrRegion>:<Label>, "+
				"but got %q; could not recover region/cluster=%q and label=%q",
			id, regionOrCluster, label,
		)
	}
	return regionOrCluster, label, nil
}

func (r *BucketResource) populateLogAttributes(ctx context.Context, data ResourceModel) context.Context {
	return helper.SetLogFieldBulk(ctx, map[string]any{
		"bucket":  data.Label.ValueString(),
		"cluster": data.Cluster.ValueString(),
	})
}

func (r *BucketResource) validateRegionIfPresent(
	ctx context.Context,
	region string,
	client *linodego.Client,
	diags *diag.Diagnostics,
) {
	valid, suggestedRegions, err := validateRegion(ctx, region, client)
	if err != nil {
		diags.AddError("Failed to validate region", err.Error())
		return
	}
	if !valid {
		errorMsg := fmt.Sprintf("Region '%s' is not valid for Object Storage.", region)
		if len(suggestedRegions) > 0 {
			errorMsg += fmt.Sprintf(" Suggested regions: %s", strings.Join(suggestedRegions, ", "))
		}
		diags.AddError("Invalid region", errorMsg)
	}
}

// flattenLifecycleRulesFramework converts S3 lifecycle rules to the framework model.
func flattenLifecycleRulesFramework(ctx context.Context, rules []s3types.LifecycleRule) []LifecycleRuleModel {
	tflog.Debug(ctx, "entering flattenLifecycleRulesFramework")
	result := make([]LifecycleRuleModel, len(rules))

	for i, rule := range rules {
		m := LifecycleRuleModel{
			Enabled: types.BoolValue(rule.Status == s3types.ExpirationStatusEnabled),
		}

		if rule.ID != nil {
			m.ID = types.StringValue(*rule.ID)
		} else {
			m.ID = types.StringNull()
		}

		if rule.Prefix != nil {
			m.Prefix = types.StringValue(*rule.Prefix)
		} else {
			m.Prefix = types.StringNull()
		}

		if rule.AbortIncompleteMultipartUpload != nil && rule.AbortIncompleteMultipartUpload.DaysAfterInitiation != nil {
			m.AbortIncompleteMultipartUploadDays = types.Int64Value(int64(*rule.AbortIncompleteMultipartUpload.DaysAfterInitiation))
		} else {
			m.AbortIncompleteMultipartUploadDays = types.Int64Null()
		}

		if rule.Expiration != nil {
			exp := LifecycleExpirationModel{}
			if rule.Expiration.Date != nil {
				exp.Date = types.StringValue(rule.Expiration.Date.Format("2006-01-02"))
			} else {
				exp.Date = types.StringNull()
			}
			if rule.Expiration.Days != nil {
				exp.Days = types.Int64Value(int64(*rule.Expiration.Days))
			} else {
				exp.Days = types.Int64Null()
			}
			if rule.Expiration.ExpiredObjectDeleteMarker != nil && *rule.Expiration.ExpiredObjectDeleteMarker {
				exp.ExpiredObjectDeleteMarker = types.BoolValue(*rule.Expiration.ExpiredObjectDeleteMarker)
			} else {
				exp.ExpiredObjectDeleteMarker = types.BoolNull()
			}
			m.Expiration = []LifecycleExpirationModel{exp}
		}

		if rule.NoncurrentVersionExpiration != nil {
			nce := NoncurrentVersionExpirationModel{}
			if rule.NoncurrentVersionExpiration.NoncurrentDays != nil && *rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
				nce.Days = types.Int64Value(int64(*rule.NoncurrentVersionExpiration.NoncurrentDays))
			} else {
				nce.Days = types.Int64Null()
			}
			m.NoncurrentVersionExpiration = []NoncurrentVersionExpirationModel{nce}
		}

		result[i] = m
	}
	return result
}

// expandLifecycleRulesFramework converts the framework model to S3 lifecycle rules.
func expandLifecycleRulesFramework(ctx context.Context, rules []LifecycleRuleModel) ([]s3types.LifecycleRule, error) {
	tflog.Debug(ctx, "entering expandLifecycleRulesFramework")
	result := make([]s3types.LifecycleRule, len(rules))

	for i, ruleModel := range rules {
		rule := s3types.LifecycleRule{}

		status := s3types.ExpirationStatusDisabled
		if ruleModel.Enabled.ValueBool() {
			status = s3types.ExpirationStatusEnabled
		}
		rule.Status = status

		if !ruleModel.ID.IsNull() && !ruleModel.ID.IsUnknown() {
			id := ruleModel.ID.ValueString()
			rule.ID = &id
		}

		if !ruleModel.Prefix.IsNull() && !ruleModel.Prefix.IsUnknown() {
			prefix := ruleModel.Prefix.ValueString()
			rule.Prefix = &prefix
		}

		if !ruleModel.AbortIncompleteMultipartUploadDays.IsNull() && !ruleModel.AbortIncompleteMultipartUploadDays.IsUnknown() {
			days := ruleModel.AbortIncompleteMultipartUploadDays.ValueInt64()
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

		if len(ruleModel.Expiration) > 0 {
			tflog.Debug(ctx, "expanding expiration")
			exp := ruleModel.Expiration[0]
			rule.Expiration = &s3types.LifecycleExpiration{}

			if !exp.Date.IsNull() && !exp.Date.IsUnknown() && exp.Date.ValueString() != "" {
				date, err := time.Parse(time.RFC3339, fmt.Sprintf("%sT00:00:00Z", exp.Date.ValueString()))
				if err != nil {
					return nil, err
				}
				rule.Expiration.Date = &date
			}

			if !exp.Days.IsNull() && !exp.Days.IsUnknown() {
				days := int32(exp.Days.ValueInt64())
				if days > 0 {
					rule.Expiration.Days = &days
				}
			}

			if !exp.ExpiredObjectDeleteMarker.IsNull() && !exp.ExpiredObjectDeleteMarker.IsUnknown() {
				marker := exp.ExpiredObjectDeleteMarker.ValueBool()
				if marker {
					rule.Expiration.ExpiredObjectDeleteMarker = &marker
				}
			}
		}

		if len(ruleModel.NoncurrentVersionExpiration) > 0 {
			tflog.Debug(ctx, "expanding noncurrent_version_expiration")
			nce := ruleModel.NoncurrentVersionExpiration[0]
			rule.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{}

			if !nce.Days.IsNull() && !nce.Days.IsUnknown() {
				days := int32(nce.Days.ValueInt64())
				if days > 0 {
					rule.NoncurrentVersionExpiration.NoncurrentDays = &days
				}
			}
		}

		result[i] = rule
	}

	return result, nil
}

// matchRulesWithModelRules maintains the order of declared lifecycle rules.
func matchRulesWithModelRules(
	ctx context.Context,
	rules []s3types.LifecycleRule,
	declaredRules []LifecycleRuleModel,
) []s3types.LifecycleRule {
	tflog.Debug(ctx, "entering matchRulesWithModelRules")

	result := make([]s3types.LifecycleRule, 0)
	ruleMap := make(map[string]s3types.LifecycleRule)
	for _, rule := range rules {
		if rule.ID != nil {
			ruleMap[*rule.ID] = rule
		}
	}

	for _, declared := range declaredRules {
		if declared.ID.IsNull() || declared.ID.IsUnknown() || declared.ID.ValueString() == "" {
			continue
		}
		id := declared.ID.ValueString()
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
