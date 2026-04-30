package openstack

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/objects"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface checks.
var (
	_ resource.Resource                     = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigure        = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithImportState      = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState     = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigValidators = &objectStorageContainerV1Resource{}
)

// NewObjectStorageContainerV1Resource is the factory function registered with
// the provider.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

// objectStorageContainerV1Resource holds the provider-level client reference
// injected by Configure.
type objectStorageContainerV1Resource struct {
	config *Config
}

// ---------------------------------------------------------------------------
// Model structs
// ---------------------------------------------------------------------------

// objectStorageContainerV1Model maps the current (V1) schema attributes to Go
// types used by Plan.Get / State.Set.
type objectStorageContainerV1Model struct {
	ID               types.String            `tfsdk:"id"`
	Region           types.String            `tfsdk:"region"`
	Name             types.String            `tfsdk:"name"`
	ContainerRead    types.String            `tfsdk:"container_read"`
	ContainerSyncTo  types.String            `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String            `tfsdk:"container_sync_key"`
	ContainerWrite   types.String            `tfsdk:"container_write"`
	ContentType      types.String            `tfsdk:"content_type"`
	Versioning       types.Bool              `tfsdk:"versioning"`
	// versioning_legacy is a SetNestedBlock; the model field is a slice.
	// A nil/empty slice means the block is absent.
	VersioningLegacy []versioningLegacyModel `tfsdk:"versioning_legacy"`
	Metadata         types.Map               `tfsdk:"metadata"`
	ForceDestroy     types.Bool              `tfsdk:"force_destroy"`
	StoragePolicy    types.String            `tfsdk:"storage_policy"`
	StorageClass     types.String            `tfsdk:"storage_class"`
}

// versioningLegacyModel holds the attributes of a single versioning_legacy
// block.
type versioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_objectstorage_container_v1"
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		// Version 1 — matches the last SDKv2 SchemaVersion. The UpgradeState
		// method (upgrade_objectstorage_container_v1.go) handles V0 state
		// written by older SDKv2 provider releases.
		Version: 1,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
			"container_read": schema.StringAttribute{
				Optional: true,
			},
			"container_sync_to": schema.StringAttribute{
				Optional: true,
			},
			"container_sync_key": schema.StringAttribute{
				Optional: true,
			},
			"container_write": schema.StringAttribute{
				Optional: true,
			},
			"content_type": schema.StringAttribute{
				Optional: true,
			},
			"versioning": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
			},
			"force_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
			},
			"storage_policy": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"storage_class": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			// versioning_legacy is kept as a SetNestedBlock (block syntax:
			//
			//   versioning_legacy { type = "…" location = "…" }
			//
			// ) to preserve backward-compatible HCL syntax for practitioners.
			// The SDKv2 schema used TypeSet + MaxItems:1; we mirror that with
			// a SetNestedBlock and a ConfigValidator enforcing at most one entry.
			"versioning_legacy": schema.SetNestedBlock{
				DeprecationMessage: `Use newer "versioning" implementation`,
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								stringvalidator.OneOfCaseInsensitive("versions", "history"),
							},
						},
						"location": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
		},
	}
}

// ---------------------------------------------------------------------------
// ConfigValidators — enforces ConflictsWith between versioning and
// versioning_legacy, and the MaxItems:1 constraint on versioning_legacy.
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) ConfigValidators(
	_ context.Context,
) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		versioningConflictConfigValidator{},
	}
}

// versioningConflictConfigValidator checks that versioning=true and
// versioning_legacy block are not both specified, and that at most one
// versioning_legacy block is provided.
type versioningConflictConfigValidator struct{}

func (v versioningConflictConfigValidator) Description(_ context.Context) string {
	return "versioning and versioning_legacy are mutually exclusive"
}

func (v versioningConflictConfigValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v versioningConflictConfigValidator) ValidateResource(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var config objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(config.VersioningLegacy) > 1 {
		resp.Diagnostics.AddAttributeError(
			path.Root("versioning_legacy"),
			"too many versioning_legacy blocks",
			"at most one versioning_legacy block is allowed",
		)
	}

	if config.Versioning.ValueBool() && len(config.VersioningLegacy) > 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("versioning"),
			"conflicting attributes",
			`"versioning" and "versioning_legacy" cannot both be set`,
		)
	}
}

// ---------------------------------------------------------------------------
// Configure — injects provider client
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"unexpected provider data type",
			fmt.Sprintf("expected *Config, got: %T", req.ProviderData),
		)
		return
	}
	r.config = config
}

// ---------------------------------------------------------------------------
// ImportState
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, plan.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("error creating OpenStack object storage client", err.Error())
		return
	}

	cn := plan.Name.ValueString()

	createOpts := &containerCreateOpts{
		CreateOpts: containers.CreateOpts{
			ContainerRead:    plan.ContainerRead.ValueString(),
			ContainerSyncTo:  plan.ContainerSyncTo.ValueString(),
			ContainerSyncKey: plan.ContainerSyncKey.ValueString(),
			ContainerWrite:   plan.ContainerWrite.ValueString(),
			ContentType:      plan.ContentType.ValueString(),
			StoragePolicy:    plan.StoragePolicy.ValueString(),
			VersionsEnabled:  plan.Versioning.ValueBool(),
			Metadata:         containerMetadataFromModel(plan),
		},
		StorageClass: plan.StorageClass.ValueString(),
	}

	if len(plan.VersioningLegacy) > 0 {
		vl := plan.VersioningLegacy[0]
		switch vl.Type.ValueString() {
		case "versions":
			createOpts.VersionsLocation = vl.Location.ValueString()
		case "history":
			createOpts.HistoryLocation = vl.Location.ValueString()
		}
	}

	log.Printf("[DEBUG] Create Options for objectstorage_container_v1: %#v", createOpts)

	_, err = containers.Create(ctx, objectStorageClient, cn, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError("error creating objectstorage_container_v1", err.Error())
		return
	}

	log.Printf("[INFO] objectstorage_container_v1 created with ID: %s", cn)

	// ID equals the container name in Swift.
	plan.ID = types.StringValue(cn)

	// Refresh from API to populate computed fields.
	readIntoModel(ctx, objectStorageClient, r.config, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, state.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("error creating OpenStack object storage client", err.Error())
		return
	}

	result := containers.Get(ctx, objectStorageClient, state.ID.ValueString(), nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("error reading objectstorage_container_v1 '%s'", state.ID.ValueString()),
			result.Err.Error(),
		)
		return
	}

	readIntoModel(ctx, objectStorageClient, r.config, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan, state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, plan.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("error creating OpenStack object storage client", err.Error())
		return
	}

	containerRead := plan.ContainerRead.ValueString()
	containerSyncTo := plan.ContainerSyncTo.ValueString()
	containerSyncKey := plan.ContainerSyncKey.ValueString()
	containerWrite := plan.ContainerWrite.ValueString()
	contentType := plan.ContentType.ValueString()

	updateOpts := containers.UpdateOpts{
		ContainerRead:    &containerRead,
		ContainerSyncTo:  &containerSyncTo,
		ContainerSyncKey: &containerSyncKey,
		ContainerWrite:   &containerWrite,
		ContentType:      &contentType,
	}

	if !plan.Versioning.Equal(state.Versioning) {
		v := plan.Versioning.ValueBool()
		updateOpts.VersionsEnabled = &v
	}

	versioningLegacyChanged := versioningLegacyDiffers(plan.VersioningLegacy, state.VersioningLegacy)
	if versioningLegacyChanged {
		if len(plan.VersioningLegacy) == 0 {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			vl := plan.VersioningLegacy[0]
			if vl.Location.ValueString() == "" || vl.Type.ValueString() == "" {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			} else {
				switch vl.Type.ValueString() {
				case "versions":
					updateOpts.VersionsLocation = vl.Location.ValueString()
				case "history":
					updateOpts.HistoryLocation = vl.Location.ValueString()
				}
			}
		}
	}

	// Remove legacy versioning first, before enabling new versioning.
	if updateOpts.VersionsEnabled != nil && *updateOpts.VersionsEnabled &&
		(updateOpts.RemoveVersionsLocation == "true" || updateOpts.RemoveHistoryLocation == "true") {
		opts := containers.UpdateOpts{
			RemoveVersionsLocation: "true",
			RemoveHistoryLocation:  "true",
		}
		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	// Remove new versioning first, before enabling legacy versioning.
	if (updateOpts.VersionsLocation != "" || updateOpts.HistoryLocation != "") &&
		updateOpts.VersionsEnabled != nil && !*updateOpts.VersionsEnabled {
		opts := containers.UpdateOpts{VersionsEnabled: updateOpts.VersionsEnabled}
		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	if !plan.Metadata.Equal(state.Metadata) {
		updateOpts.Metadata = containerMetadataFromModel(plan)
	}

	_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	plan.ID = state.ID
	readIntoModel(ctx, objectStorageClient, r.config, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, state.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("error creating OpenStack object storage client", err.Error())
		return
	}

	_, err = containers.Delete(ctx, objectStorageClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
			log.Printf(
				"[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s': %+v",
				state.ID.ValueString(), err,
			)

			container := state.ID.ValueString()
			pager := objects.List(objectStorageClient, container, &objects.ListOpts{Versions: true})
			pageErr := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
				objectList, err := objects.ExtractInfo(page)
				if err != nil {
					return false, fmt.Errorf(
						"error extracting objects from objectstorage_container_v1 '%s': %+w",
						container, err,
					)
				}
				for _, object := range objectList {
					dopts := objects.DeleteOpts{ObjectVersionID: object.VersionID}
					_, err = objects.Delete(ctx, objectStorageClient, container, object.Name, dopts).Extract()
					if err != nil {
						latest := "latest"
						if !object.IsLatest && object.VersionID != "" {
							latest = object.VersionID
						}
						return false, fmt.Errorf(
							"error deleting object '%s@%s' from objectstorage_container_v1 '%s': %+w",
							object.Name, latest, container, err,
						)
					}
				}
				return true, nil
			})
			if pageErr != nil {
				resp.Diagnostics.AddError("error force-deleting container objects", pageErr.Error())
				return
			}

			// Retry deletion now that the container is empty.
			r.Delete(ctx, req, resp)
			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return // already gone — success
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("error deleting objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
	}
}

// ---------------------------------------------------------------------------
// Package-level helpers
// ---------------------------------------------------------------------------

// readIntoModel populates a model from the Swift API. Shared by Create and Read.
func readIntoModel(
	ctx context.Context,
	client *gophercloud.ServiceClient,
	config *Config,
	model *objectStorageContainerV1Model,
	diags *diag.Diagnostics,
) {
	result := containers.Get(ctx, client, model.ID.ValueString(), nil)
	if result.Err != nil {
		diags.AddError(
			fmt.Sprintf("error reading objectstorage_container_v1 '%s'", model.ID.ValueString()),
			result.Err.Error(),
		)
		return
	}

	headers, err := result.Extract()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("error extracting headers for objectstorage_container_v1 '%s'", model.ID.ValueString()),
			err.Error(),
		)
		return
	}

	log.Printf(
		"[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v",
		model.ID.ValueString(), headers,
	)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("error extracting metadata for objectstorage_container_v1 '%s'", model.ID.ValueString()),
			err.Error(),
		)
		return
	}

	// name == id in Swift.
	model.Name = model.ID

	if len(headers.Read) > 0 && headers.Read[0] != "" {
		model.ContainerRead = types.StringValue(strings.Join(headers.Read, ","))
	}

	if len(headers.Write) > 0 && headers.Write[0] != "" {
		model.ContainerWrite = types.StringValue(strings.Join(headers.Write, ","))
	}

	if len(headers.StoragePolicy) > 0 {
		model.StoragePolicy = types.StringValue(headers.StoragePolicy)
	}

	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		diags.AddError(
			fmt.Sprintf(
				"error reading versioning headers for objectstorage_container_v1 '%s'",
				model.ID.ValueString(),
			),
			fmt.Sprintf(
				"found location for both exclusive types, versions ('%s') and history ('%s')",
				headers.VersionsLocation, headers.HistoryLocation,
			),
		)
		return
	}

	model.VersioningLegacy = nil
	if headers.VersionsLocation != "" {
		model.VersioningLegacy = []versioningLegacyModel{
			{
				Type:     types.StringValue("versions"),
				Location: types.StringValue(headers.VersionsLocation),
			},
		}
	} else if headers.HistoryLocation != "" {
		model.VersioningLegacy = []versioningLegacyModel{
			{
				Type:     types.StringValue("history"),
				Location: types.StringValue(headers.HistoryLocation),
			},
		}
	}

	// Despite the create request using "X-Object-Storage-Class", the response
	// header is "X-Storage-Class".
	model.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))
	model.Versioning = types.BoolValue(headers.VersionsEnabled)
	model.Region = types.StringValue(config.Region)

	if len(metadata) > 0 {
		metaMap := make(map[string]string, len(metadata))
		for k, v := range metadata {
			metaMap[k] = v
		}
		mv, d := types.MapValueFrom(ctx, types.StringType, metaMap)
		diags.Append(d...)
		if !diags.HasError() {
			model.Metadata = mv
		}
	}
}

// containerMetadataFromModel converts the metadata map attribute to the
// map[string]string format gophercloud expects.
func containerMetadataFromModel(model objectStorageContainerV1Model) map[string]string {
	if model.Metadata.IsNull() || model.Metadata.IsUnknown() {
		return nil
	}
	result := make(map[string]string, len(model.Metadata.Elements()))
	for k, v := range model.Metadata.Elements() {
		if sv, ok := v.(types.String); ok {
			result[k] = sv.ValueString()
		}
	}
	return result
}

// versioningLegacyDiffers returns true when the two slices differ.
func versioningLegacyDiffers(a, b []versioningLegacyModel) bool {
	if len(a) != len(b) {
		return true
	}
	if len(a) == 0 {
		return false
	}
	return !a[0].Type.Equal(b[0].Type) || !a[0].Location.Equal(b[0].Location)
}
