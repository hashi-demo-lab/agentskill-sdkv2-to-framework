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

// Ensure the Resource implements the required interfaces.
var (
	_ resource.Resource                = &BucketResource{}
	_ resource.ResourceWithImportState = &BucketResource{}
)

// bucketObjectKeys holds the S3 access/secret key pair resolved for a request.
type bucketObjectKeys struct {
	AccessKey string
	SecretKey string
}

func (k bucketObjectKeys) ok() bool {
	return k.AccessKey != "" && k.SecretKey != ""
}

// NewFrameworkResource returns a new framework resource for linode_object_storage_bucket.
func NewFrameworkResource() resource.Resource {
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

// BucketResource is the framework resource implementation.
type BucketResource struct {
	helper.BaseResource
}

// frameworkResourceSchema is the framework schema for the resource.
var frameworkResourceSchema = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "The unique ID of this resource.",
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
				"or, generated implicitly at apply-time using obj_use_temp_keys in provider configuration.",
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
		// cert has MaxItems:1 in SDKv2. Keep as SingleNestedBlock to preserve
		// block HCL syntax `cert { ... }` for backward compatibility.
		"cert": schema.SingleNestedBlock{
			Attributes: map[string]schema.Attribute{
				"certificate": schema.StringAttribute{
					Description: "The Base64 encoded and PEM formatted SSL certificate.",
					Required:    true,
					Sensitive:   true,
				},
				"private_key": schema.StringAttribute{
					Description: "The private key associated with the TLS/SSL certificate.",
					Required:    true,
					Sensitive:   true,
				},
			},
		},
		// lifecycle_rule has no MaxItems — keep as ListNestedBlock to preserve block HCL syntax.
		"lifecycle_rule": schema.ListNestedBlock{
			Validators: []validator.List{
				listvalidator.SizeAtMost(100),
			},
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
					// expiration has MaxItems:1 — use SingleNestedBlock for block compat.
					"expiration": schema.SingleNestedBlock{
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
					// noncurrent_version_expiration has MaxItems:1 — use SingleNestedBlock.
					"noncurrent_version_expiration": schema.SingleNestedBlock{
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
}

// --- Model types ---

// BucketResourceModel holds all top-level state for the resource.
type BucketResourceModel struct {
	ID            types.String         `tfsdk:"id"`
	SecretKey     types.String         `tfsdk:"secret_key"`
	AccessKey     types.String         `tfsdk:"access_key"`
	Cluster       types.String         `tfsdk:"cluster"`
	Region        types.String         `tfsdk:"region"`
	Endpoint      types.String         `tfsdk:"endpoint"`
	S3Endpoint    types.String         `tfsdk:"s3_endpoint"`
	EndpointType  types.String         `tfsdk:"endpoint_type"`
	Label         types.String         `tfsdk:"label"`
	ACL           types.String         `tfsdk:"acl"`
	CorsEnabled   types.Bool           `tfsdk:"cors_enabled"`
	Hostname      types.String         `tfsdk:"hostname"`
	Versioning    types.Bool           `tfsdk:"versioning"`
	Cert          *BucketCertModel     `tfsdk:"cert"`
	LifecycleRule []LifecycleRuleModel `tfsdk:"lifecycle_rule"`
}

// BucketCertModel corresponds to the `cert` block.
type BucketCertModel struct {
	Certificate types.String `tfsdk:"certificate"`
	PrivateKey  types.String `tfsdk:"private_key"`
}

// LifecycleRuleModel corresponds to each item in `lifecycle_rule`.
type LifecycleRuleModel struct {
	ID                                 types.String                      `tfsdk:"id"`
	Prefix                             types.String                      `tfsdk:"prefix"`
	Enabled                            types.Bool                        `tfsdk:"enabled"`
	AbortIncompleteMultipartUploadDays types.Int64                       `tfsdk:"abort_incomplete_multipart_upload_days"`
	Expiration                         *LifecycleExpirationModel         `tfsdk:"expiration"`
	NoncurrentVersionExpiration        *NoncurrentVersionExpirationModel `tfsdk:"noncurrent_version_expiration"`
}

// LifecycleExpirationModel corresponds to the `expiration` block inside `lifecycle_rule`.
type LifecycleExpirationModel struct {
	Date                      types.String `tfsdk:"date"`
	Days                      types.Int64  `tfsdk:"days"`
	ExpiredObjectDeleteMarker types.Bool   `tfsdk:"expired_object_delete_marker"`
}

// NoncurrentVersionExpirationModel corresponds to the `noncurrent_version_expiration` block.
type NoncurrentVersionExpirationModel struct {
	Days types.Int64 `tfsdk:"days"`
}

// getRegionOrCluster returns the region or cluster value from the model.
func (m *BucketResourceModel) getRegionOrCluster(ctx context.Context) string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}
	tflog.Warn(ctx, "Cluster is deprecated for Linode Object Storage services, please consider switching to region.")
	return m.Cluster.ValueString()
}

// getObjectStorageKeys resolves the access/secret key pair for S3 operations.
func (m *BucketResourceModel) getObjectStorageKeys(
	ctx context.Context,
	client *linodego.Client,
	config *helper.FrameworkProviderModel,
	permission string,
	endpointType *linodego.ObjectStorageEndpointType,
	diags *diag.Diagnostics,
) (bucketObjectKeys, func()) {
	keys := bucketObjectKeys{
		AccessKey: m.AccessKey.ValueString(),
		SecretKey: m.SecretKey.ValueString(),
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
		regionOrCluster := m.getRegionOrCluster(ctx)
		label := m.Label.ValueString()

		tempKey := fwCreateTempKeys(ctx, client, label, regionOrCluster, permission, endpointType, diags)
		if diags.HasError() {
			return keys, nil
		}

		keys.AccessKey = tempKey.AccessKey
		keys.SecretKey = tempKey.SecretKey
		teardown := func() { fwCleanUpTempKeys(ctx, client, tempKey.ID) }
		return keys, teardown
	}

	diags.AddError(
		"Keys Not Found",
		"`access_key` and `secret_key` are required but not configured.",
	)
	return keys, nil
}

// fwCreateTempKeys creates temporary Object Storage Keys scoped to the bucket.
func fwCreateTempKeys(
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

	// Cluster names have the form "us-mia-1" (3 parts separated by "-");
	// region names have the form "us-mia" (2 parts). This heuristic matches the
	// existing provider behaviour.
	if strings.Count(regionOrCluster, "-") >= 2 {
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
		diags.AddError("Failed to Create Object Storage Key", err.Error())
		return nil
	}

	return keys
}

// fwCleanUpTempKeys deletes temporary Object Storage Keys.
func fwCleanUpTempKeys(ctx context.Context, client *linodego.Client, keyID int) {
	if err := client.DeleteObjectStorageKey(ctx, keyID); err != nil {
		tflog.Warn(ctx, "Failed to clean up temporary object storage keys", map[string]any{
			"details": err,
		})
	}
}

// --- CRUD Handlers ---

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

	client := r.Meta.Client

	ctx = populateLogAttributesFW(ctx, &plan)

	if diags := validateRegionIfPresentFW(ctx, &plan, client); diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	label := plan.Label.ValueString()
	acl := plan.ACL.ValueString()

	createOpts := linodego.ObjectStorageBucketCreateOptions{
		Label: label,
		ACL:   linodego.ObjectStorageACL(acl),
	}

	if !plan.CorsEnabled.IsNull() && !plan.CorsEnabled.IsUnknown() {
		b := plan.CorsEnabled.ValueBool()
		createOpts.CorsEnabled = &b
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
		resp.Diagnostics.AddError("Failed to create Linode ObjectStorageBucket", err.Error())
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

	// Persist partial state early to avoid dangling resources if update/read fails.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.update(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &plan, &resp.Diagnostics, func() { resp.State.RemoveResource(ctx) })
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

	var state BucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFW(ctx, &state)

	r.read(ctx, &state, &resp.Diagnostics, func() { resp.State.RemoveResource(ctx) })
	if resp.Diagnostics.HasError() {
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

	var plan, state BucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Carry the ID forward from state — it's computed and won't be in Plan.
	plan.ID = state.ID

	ctx = populateLogAttributesFW(ctx, &plan)

	if plan.Region.ValueString() != state.Region.ValueString() {
		if diags := validateRegionIfPresentFW(ctx, &plan, r.Meta.Client); diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
	}

	r.update(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	r.read(ctx, &plan, &resp.Diagnostics, nil)
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

	// Delete reads from State, not Plan — Plan is null on Delete.
	var state BucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx = populateLogAttributesFW(ctx, &state)

	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster, label, err := DecodeBucketIDString(ctx, state.ID.ValueString())
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

		keys, teardown := state.getObjectStorageKeys(ctx, client, config, "read_write", endpointType, &resp.Diagnostics)
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
		s3client, err := helper.S3Connection(ctx, endpoint, keys.AccessKey, keys.SecretKey)
		if err != nil {
			resp.Diagnostics.AddError("Failed to create S3 connection", err.Error())
			return
		}

		tflog.Debug(ctx, "helper.PurgeAllObjects(...)")
		if err := helper.PurgeAllObjects(ctx, label, s3client, true, true); err != nil {
			resp.Diagnostics.AddError(
				"Error purging all objects from ObjectStorageBucket",
				fmt.Sprintf("%s: %s", state.ID.ValueString(), err),
			)
			return
		}
	}

	tflog.Debug(ctx, "client.DeleteObjectStorageBucket(...)")
	if err := client.DeleteObjectStorageBucket(ctx, regionOrCluster, label); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Linode ObjectStorageBucket",
			fmt.Sprintf("%s: %s", state.ID.ValueString(), err),
		)
	}
}

// ImportState handles composite ID import: `terraform import linode_object_storage_bucket.foo cluster:label`
//
// The ID format is "clusterOrRegion:label", e.g. "us-mia-1:my-bucket" or "us-mia:my-bucket".
// After ImportState, the framework calls Read with the partial state.
func (r *BucketResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	tflog.Debug(ctx, "ImportState linode_object_storage_bucket", map[string]any{"id": req.ID})

	regionOrCluster, label, err := DecodeBucketIDString(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"invalid import ID",
			fmt.Sprintf("expected 'clusterOrRegion:label', got %q: %s", req.ID, err),
		)
		return
	}

	// Write enough state so that Read can find the resource.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("label"), label)...)

	// Preserve the cluster vs region distinction. Cluster IDs end with a digit
	// segment, e.g. "us-mia-1"; region IDs do not, e.g. "us-mia".
	// We store the value in the appropriate attribute so Read can resolve it.
	if strings.Count(regionOrCluster, "-") >= 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cluster"), regionOrCluster)...)
	} else {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("region"), regionOrCluster)...)
	}
}

// --- Internal helpers ---

// read populates the model from the API.
// removeResource is called when the resource is gone (404) so Read can remove it from state.
func (r *BucketResource) read(
	ctx context.Context,
	data *BucketResourceModel,
	diags *diag.Diagnostics,
	removeResource func(),
) {
	client := r.Meta.Client
	config := r.Meta.Config

	regionOrCluster, label, err := DecodeBucketIDString(ctx, data.ID.ValueString())
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
			if removeResource != nil {
				removeResource()
			}
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

	// Read lifecycle and versioning only when the user declared them.
	hasVersioning := !data.Versioning.IsNull()
	hasLifecycle := len(data.LifecycleRule) > 0

	if hasVersioning || hasLifecycle {
		tflog.Debug(ctx, "versioning or lifecycle present", map[string]any{
			"versioningPresent": hasVersioning,
			"lifecyclePresent":  hasLifecycle,
		})

		var endpointType *linodego.ObjectStorageEndpointType
		if !data.EndpointType.IsNull() && !data.EndpointType.IsUnknown() && data.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(data.EndpointType.ValueString())
			endpointType = &et
		}

		keys, teardown := data.getObjectStorageKeys(ctx, r.Meta.Client, config, "read_only", endpointType, diags)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := getS3Endpoint(ctx, *bucket)
		s3client, err := helper.S3Connection(ctx, endpoint, keys.AccessKey, keys.SecretKey)
		if err != nil {
			diags.AddError("Failed to create S3 connection", err.Error())
			return
		}

		if hasLifecycle {
			tflog.Trace(ctx, "getting bucket lifecycle")
			if err := readBucketLifecycleFW(ctx, data, s3client); err != nil {
				diags.AddError("Failed to get object storage bucket lifecycle", err.Error())
				return
			}
		}

		if hasVersioning {
			tflog.Trace(ctx, "getting bucket versioning")
			if err := readBucketVersioningFW(ctx, data, s3client); err != nil {
				diags.AddError("Failed to get object storage bucket versioning", err.Error())
				return
			}
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
	}

	data.Label = types.StringValue(bucket.Label)
	data.Hostname = types.StringValue(bucket.Hostname)
	data.ACL = types.StringValue(string(access.ACL))
	if access.CorsEnabled != nil {
		data.CorsEnabled = types.BoolValue(*access.CorsEnabled)
	}
	data.Endpoint = types.StringValue(endpoint)
	data.S3Endpoint = types.StringValue(endpoint)
	data.EndpointType = types.StringValue(string(bucket.EndpointType))
}

// update applies ACL/cors, cert, versioning, and lifecycle changes.
func (r *BucketResource) update(
	ctx context.Context,
	plan *BucketResourceModel,
	diags *diag.Diagnostics,
) {
	client := r.Meta.Client
	regionOrCluster := plan.getRegionOrCluster(ctx)
	label := plan.Label.ValueString()

	// ACL / CORS update
	updateOpts := linodego.ObjectStorageBucketUpdateAccessOptions{
		ACL: linodego.ObjectStorageACL(plan.ACL.ValueString()),
	}
	if !plan.CorsEnabled.IsNull() && !plan.CorsEnabled.IsUnknown() {
		b := plan.CorsEnabled.ValueBool()
		updateOpts.CorsEnabled = &b
	}

	tflog.Debug(ctx, "client.UpdateObjectStorageBucketAccess(...)", map[string]any{"options": updateOpts})
	if err := client.UpdateObjectStorageBucketAccess(ctx, regionOrCluster, label, updateOpts); err != nil {
		diags.AddError("Failed to update bucket access", err.Error())
		return
	}

	// Cert update
	if err := updateBucketCertFW(ctx, plan, client); err != nil {
		diags.AddError("Failed to update bucket certificate", err.Error())
		return
	}

	// Versioning / lifecycle require S3 credentials.
	hasVersioning := !plan.Versioning.IsNull() && !plan.Versioning.IsUnknown()
	hasLifecycle := len(plan.LifecycleRule) > 0

	if hasVersioning || hasLifecycle {
		config := r.Meta.Config
		var endpointType *linodego.ObjectStorageEndpointType
		if !plan.EndpointType.IsNull() && !plan.EndpointType.IsUnknown() && plan.EndpointType.ValueString() != "" {
			et := linodego.ObjectStorageEndpointType(plan.EndpointType.ValueString())
			endpointType = &et
		}

		keys, teardown := plan.getObjectStorageKeys(ctx, client, config, "read_write", endpointType, diags)
		if diags.HasError() {
			return
		}
		if teardown != nil {
			defer teardown()
		}

		endpoint := plan.S3Endpoint.ValueString()
		if endpoint == "" {
			endpoint = plan.Endpoint.ValueString()
		}
		s3client, err := helper.S3Connection(ctx, endpoint, keys.AccessKey, keys.SecretKey)
		if err != nil {
			diags.AddError("Failed to create S3 connection", err.Error())
			return
		}

		if hasVersioning {
			tflog.Debug(ctx, "Updating bucket versioning configuration")
			if err := updateBucketVersioningFW(ctx, plan, s3client); err != nil {
				diags.AddError("Failed to update bucket versioning", err.Error())
				return
			}
		}

		if hasLifecycle {
			tflog.Debug(ctx, "Updating bucket lifecycle configuration")
			if err := updateBucketLifecycleFW(ctx, plan, s3client); err != nil {
				diags.AddError("Failed to update bucket lifecycle", err.Error())
				return
			}
		}
	}
}

// --- S3 helpers ---

func readBucketVersioningFW(ctx context.Context, data *BucketResourceModel, client *s3.Client) error {
	tflog.Trace(ctx, "entering readBucketVersioningFW")
	label := data.Label.ValueString()

	out, err := client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: &label})
	if err != nil {
		return fmt.Errorf("failed to get versioning for bucket %s: %w", label, err)
	}

	data.Versioning = types.BoolValue(out.Status == s3types.BucketVersioningStatusEnabled)
	return nil
}

func readBucketLifecycleFW(ctx context.Context, data *BucketResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()

	out, err := client.GetBucketLifecycleConfiguration(ctx, &s3.GetBucketLifecycleConfigurationInput{Bucket: &label})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "NoSuchLifecycleConfiguration" {
			data.LifecycleRule = []LifecycleRuleModel{}
			return nil
		}
		return fmt.Errorf("failed to get lifecycle for bucket %s: %w", label, err)
	}

	if out == nil {
		data.LifecycleRule = []LifecycleRuleModel{}
		return nil
	}

	rules := matchRulesWithSchemaNative(ctx, out.Rules, data.LifecycleRule)
	data.LifecycleRule = flattenLifecycleRulesFW(ctx, rules)
	return nil
}

func updateBucketVersioningFW(ctx context.Context, data *BucketResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()

	status := s3types.BucketVersioningStatusSuspended
	if data.Versioning.ValueBool() {
		status = s3types.BucketVersioningStatusEnabled
	}

	_, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: &label,
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: status,
		},
	})
	return err
}

func updateBucketLifecycleFW(ctx context.Context, data *BucketResourceModel, client *s3.Client) error {
	label := data.Label.ValueString()

	rules, err := expandLifecycleRulesFW(ctx, data.LifecycleRule)
	if err != nil {
		return err
	}

	if len(rules) > 0 {
		_, err = client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
			Bucket: &label,
			LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{
				Rules: rules,
			},
		})
	} else {
		_, err = client.DeleteBucketLifecycle(ctx, &s3.DeleteBucketLifecycleInput{Bucket: &label})
	}
	return err
}

func updateBucketCertFW(ctx context.Context, data *BucketResourceModel, client *linodego.Client) error {
	tflog.Debug(ctx, "entering updateBucketCertFW")

	// If cert block is absent (nil), nothing to do.
	if data.Cert == nil {
		return nil
	}

	regionOrCluster := data.getRegionOrCluster(ctx)
	label := data.Label.ValueString()

	// Delete existing cert (ignore error — it may not exist).
	_ = client.DeleteObjectStorageBucketCert(ctx, regionOrCluster, label)

	uploadOpts := linodego.ObjectStorageBucketCertUploadOptions{
		Certificate: data.Cert.Certificate.ValueString(),
		PrivateKey:  data.Cert.PrivateKey.ValueString(),
	}
	if _, err := client.UploadObjectStorageBucketCertV2(ctx, regionOrCluster, label, uploadOpts); err != nil {
		return fmt.Errorf("failed to upload new bucket cert: %s", err)
	}
	return nil
}

// --- Model flatten/expand ---

func flattenLifecycleRulesFW(ctx context.Context, rules []s3types.LifecycleRule) []LifecycleRuleModel {
	tflog.Debug(ctx, "entering flattenLifecycleRulesFW")
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
			exp := &LifecycleExpirationModel{}
			if rule.Expiration.Date != nil {
				exp.Date = types.StringValue(rule.Expiration.Date.Format("2006-01-02"))
			}
			if rule.Expiration.Days != nil {
				exp.Days = types.Int64Value(int64(*rule.Expiration.Days))
			}
			if rule.Expiration.ExpiredObjectDeleteMarker != nil && *rule.Expiration.ExpiredObjectDeleteMarker {
				exp.ExpiredObjectDeleteMarker = types.BoolValue(true)
			}
			m.Expiration = exp
		}

		if rule.NoncurrentVersionExpiration != nil {
			nce := &NoncurrentVersionExpirationModel{}
			if rule.NoncurrentVersionExpiration.NoncurrentDays != nil && *rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
				nce.Days = types.Int64Value(int64(*rule.NoncurrentVersionExpiration.NoncurrentDays))
			}
			m.NoncurrentVersionExpiration = nce
		}

		result[i] = m
	}

	return result
}

func expandLifecycleRulesFW(ctx context.Context, ruleModels []LifecycleRuleModel) ([]s3types.LifecycleRule, error) {
	tflog.Debug(ctx, "entering expandLifecycleRulesFW")

	rules := make([]s3types.LifecycleRule, len(ruleModels))

	for i, m := range ruleModels {
		rule := s3types.LifecycleRule{}

		status := s3types.ExpirationStatusDisabled
		if m.Enabled.ValueBool() {
			status = s3types.ExpirationStatusEnabled
		}
		rule.Status = status

		if !m.ID.IsNull() && !m.ID.IsUnknown() {
			id := m.ID.ValueString()
			rule.ID = &id
		}

		if !m.Prefix.IsNull() && !m.Prefix.IsUnknown() {
			prefix := m.Prefix.ValueString()
			rule.Prefix = &prefix
		}

		if !m.AbortIncompleteMultipartUploadDays.IsNull() && !m.AbortIncompleteMultipartUploadDays.IsUnknown() {
			days := m.AbortIncompleteMultipartUploadDays.ValueInt64()
			if days > 0 {
				days32, err := helper.SafeIntToInt32(int(days))
				if err != nil {
					return nil, err
				}
				rule.AbortIncompleteMultipartUpload = &s3types.AbortIncompleteMultipartUpload{
					DaysAfterInitiation: &days32,
				}
			}
		}

		if m.Expiration != nil {
			rule.Expiration = &s3types.LifecycleExpiration{}

			if !m.Expiration.Date.IsNull() && !m.Expiration.Date.IsUnknown() && m.Expiration.Date.ValueString() != "" {
				dateStr := m.Expiration.Date.ValueString()
				date, err := time.Parse(time.RFC3339, fmt.Sprintf("%sT00:00:00Z", dateStr))
				if err != nil {
					return nil, err
				}
				rule.Expiration.Date = &date
			}

			if !m.Expiration.Days.IsNull() && !m.Expiration.Days.IsUnknown() {
				days := m.Expiration.Days.ValueInt64()
				if days > 0 {
					days32, err := helper.SafeIntToInt32(int(days))
					if err != nil {
						return nil, err
					}
					rule.Expiration.Days = &days32
				}
			}

			if !m.Expiration.ExpiredObjectDeleteMarker.IsNull() && !m.Expiration.ExpiredObjectDeleteMarker.IsUnknown() {
				marker := m.Expiration.ExpiredObjectDeleteMarker.ValueBool()
				if marker {
					rule.Expiration.ExpiredObjectDeleteMarker = &marker
				}
			}
		}

		if m.NoncurrentVersionExpiration != nil {
			rule.NoncurrentVersionExpiration = &s3types.NoncurrentVersionExpiration{}

			if !m.NoncurrentVersionExpiration.Days.IsNull() && !m.NoncurrentVersionExpiration.Days.IsUnknown() {
				days := m.NoncurrentVersionExpiration.Days.ValueInt64()
				if days > 0 {
					days32, err := helper.SafeIntToInt32(int(days))
					if err != nil {
						return nil, err
					}
					rule.NoncurrentVersionExpiration.NoncurrentDays = &days32
				}
			}
		}

		rules[i] = rule
	}

	return rules, nil
}

// matchRulesWithSchemaNative preserves order of declared lifecycle rules and appends new ones.
func matchRulesWithSchemaNative(
	ctx context.Context,
	rules []s3types.LifecycleRule,
	declaredRules []LifecycleRuleModel,
) []s3types.LifecycleRule {
	tflog.Debug(ctx, "entering matchRulesWithSchemaNative")

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
		declaredID := declared.ID.ValueString()
		if rule, ok := ruleMap[declaredID]; ok {
			result = append(result, rule)
			delete(ruleMap, declaredID)
		}
	}

	for _, rule := range ruleMap {
		tflog.Debug(ctx, "adding new rule", map[string]any{"rule": rule})
		result = append(result, rule)
	}

	return result
}

// --- Utility ---

func populateLogAttributesFW(ctx context.Context, data *BucketResourceModel) context.Context {
	return helper.SetLogFieldBulk(ctx, map[string]any{
		"bucket":  data.Label.ValueString(),
		"cluster": data.Cluster.ValueString(),
	})
}

// DecodeBucketIDString parses a composite bucket ID "clusterOrRegion:label".
// It is exported so tests can use it.
func DecodeBucketIDString(ctx context.Context, id string) (regionOrCluster, label string, err error) {
	tflog.Debug(ctx, "decoding bucket ID")
	parts := strings.Split(id, ":")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf(
		"Linode Object Storage Bucket ID must be of the form <ClusterOrRegion>:<Label>, got %q", id,
	)
}

// validateRegionIfPresentFW validates the region specified in the model.
func validateRegionIfPresentFW(
	ctx context.Context,
	data *BucketResourceModel,
	client *linodego.Client,
) diag.Diagnostics {
	var diags diag.Diagnostics

	if data.Region.IsNull() || data.Region.IsUnknown() || data.Region.ValueString() == "" {
		return nil
	}

	region := data.Region.ValueString()
	valid, suggestedRegions, err := validateRegion(ctx, region, client)
	if err != nil {
		diags.AddError("Failed to validate region", err.Error())
		return diags
	}

	if !valid {
		errorMsg := fmt.Sprintf("Region '%s' is not valid for Object Storage.", region)
		if len(suggestedRegions) > 0 {
			errorMsg += fmt.Sprintf(" Suggested regions: %s", strings.Join(suggestedRegions, ", "))
		}
		diags.AddError("Invalid Region", errorMsg)
	}

	return diags
}
