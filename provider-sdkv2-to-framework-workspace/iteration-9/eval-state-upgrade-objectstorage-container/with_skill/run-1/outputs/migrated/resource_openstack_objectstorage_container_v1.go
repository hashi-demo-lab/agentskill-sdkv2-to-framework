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
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigure   = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithImportState = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}
)

// NewObjectStorageContainerV1Resource is the resource constructor used when
// registering it with the provider.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

type objectStorageContainerV1Resource struct {
	config *Config
}

// objectStorageContainerV1Model is the framework model for schema version 1
// (the current/target version).
type objectStorageContainerV1Model struct {
	ID               types.String `tfsdk:"id"`
	Region           types.String `tfsdk:"region"`
	Name             types.String `tfsdk:"name"`
	ContainerRead    types.String `tfsdk:"container_read"`
	ContainerSyncTo  types.String `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String `tfsdk:"container_sync_key"`
	ContainerWrite   types.String `tfsdk:"container_write"`
	ContentType      types.String `tfsdk:"content_type"`
	Versioning       types.Bool   `tfsdk:"versioning"`
	VersioningLegacy types.Set    `tfsdk:"versioning_legacy"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ForceDestroy     types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy    types.String `tfsdk:"storage_policy"`
	StorageClass     types.String `tfsdk:"storage_class"`
}

// versioningLegacyAttrTypes holds the element attribute types for the
// versioning_legacy set-nested-attribute.
var versioningLegacyAttrTypes = map[string]attr.Type{
	"type":     types.StringType,
	"location": types.StringType,
}

// objectStorageContainerV1ModelV0 is the framework model for schema version 0
// (the prior version before the versioning field was split into
// versioning_legacy + versioning bool).
type objectStorageContainerV1ModelV0 struct {
	ID               types.String `tfsdk:"id"`
	Region           types.String `tfsdk:"region"`
	Name             types.String `tfsdk:"name"`
	ContainerRead    types.String `tfsdk:"container_read"`
	ContainerSyncTo  types.String `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String `tfsdk:"container_sync_key"`
	ContainerWrite   types.String `tfsdk:"container_write"`
	ContentType      types.String `tfsdk:"content_type"`
	// In V0 "versioning" was the legacy versioning block (Set), not a bool.
	Versioning    types.Set    `tfsdk:"versioning"`
	Metadata      types.Map    `tfsdk:"metadata"`
	ForceDestroy  types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy types.String `tfsdk:"storage_policy"`
}

// Metadata returns the resource type name.
func (r *objectStorageContainerV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_objectstorage_container_v1"
}

// Schema returns the current (V1) schema.
func (r *objectStorageContainerV1Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
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
			"versioning_legacy": schema.SetNestedAttribute{
				Optional:           true,
				DeprecationMessage: "Use newer \"versioning\" implementation",
				NestedObject: schema.NestedAttributeObject{
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
	}
}

// Configure stores the provider client so CRUD methods can use it.
func (r *objectStorageContainerV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)
		return
	}
	r.config = config
}

// Create creates a new object storage container.
func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := plan.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	cn := plan.Name.ValueString()

	metadata := make(map[string]string)
	if !plan.Metadata.IsNull() && !plan.Metadata.IsUnknown() {
		var m map[string]string
		resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &m, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		metadata = m
	}

	createOpts := &containerCreateOpts{
		CreateOpts: containers.CreateOpts{
			ContainerRead:    plan.ContainerRead.ValueString(),
			ContainerSyncTo:  plan.ContainerSyncTo.ValueString(),
			ContainerSyncKey: plan.ContainerSyncKey.ValueString(),
			ContainerWrite:   plan.ContainerWrite.ValueString(),
			ContentType:      plan.ContentType.ValueString(),
			StoragePolicy:    plan.StoragePolicy.ValueString(),
			VersionsEnabled:  plan.Versioning.ValueBool(),
			Metadata:         metadata,
		},
		StorageClass: plan.StorageClass.ValueString(),
	}

	if !plan.VersioningLegacy.IsNull() && !plan.VersioningLegacy.IsUnknown() && len(plan.VersioningLegacy.Elements()) > 0 {
		vParams, d := versioningLegacyFromSet(ctx, plan.VersioningLegacy)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		switch vParams["type"] {
		case "versions":
			createOpts.VersionsLocation = vParams["location"]
		case "history":
			createOpts.HistoryLocation = vParams["location"]
		}
	}

	log.Printf("[DEBUG] Create Options for objectstorage_container_v1: %#v", createOpts)

	_, err = containers.Create(ctx, objectStorageClient, cn, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating objectstorage_container_v1",
			err.Error(),
		)
		return
	}

	log.Printf("[INFO] objectstorage_container_v1 created with ID: %s", cn)

	plan.ID = types.StringValue(cn)
	plan.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Re-read to populate computed fields.
	r.readIntoState(ctx, cn, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read reads the container state from the API.
func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	r.readIntoState(ctx, state.ID.ValueString(), region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// readIntoState fetches container data and populates the model. It calls
// resp.State.RemoveResource on 404.
func (r *objectStorageContainerV1Resource) readIntoState(
	ctx context.Context,
	id, region string,
	m *objectStorageContainerV1Model,
	diags *diag.Diagnostics,
) {
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		diags.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	result := containers.Get(ctx, objectStorageClient, id, nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			// Signal drift — the caller must call resp.State.RemoveResource.
			diags.AddWarning("resource_removed", fmt.Sprintf("objectstorage_container_v1 '%s' no longer exists", id))
			return
		}
		diags.AddError(
			fmt.Sprintf("Error reading objectstorage_container_v1 '%s'", id),
			result.Err.Error(),
		)
		return
	}

	headers, err := result.Extract()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Error extracting headers for objectstorage_container_v1 '%s'", id),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", id, headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Error extracting metadata for objectstorage_container_v1 '%s'", id),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", id, metadata)

	m.ID = types.StringValue(id)
	m.Name = types.StringValue(id)
	m.Region = types.StringValue(region)

	if len(headers.Read) > 0 && headers.Read[0] != "" {
		m.ContainerRead = types.StringValue(strings.Join(headers.Read, ","))
	}

	if len(headers.Write) > 0 && headers.Write[0] != "" {
		m.ContainerWrite = types.StringValue(strings.Join(headers.Write, ","))
	}

	if len(headers.StoragePolicy) > 0 {
		m.StoragePolicy = types.StringValue(headers.StoragePolicy)
	}

	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		diags.AddError(
			fmt.Sprintf("Error reading versioning headers for objectstorage_container_v1 '%s'", id),
			fmt.Sprintf("found location for both exclusive types, versions ('%s') and history ('%s')",
				headers.VersionsLocation, headers.HistoryLocation),
		)
		return
	}

	if headers.VersionsLocation != "" {
		legacySet, d := types.SetValue(
			types.ObjectType{AttrTypes: versioningLegacyAttrTypes},
			[]attr.Value{
				mustObjectValue(versioningLegacyAttrTypes, map[string]attr.Value{
					"type":     types.StringValue("versions"),
					"location": types.StringValue(headers.VersionsLocation),
				}),
			},
		)
		diags.Append(d...)
		m.VersioningLegacy = legacySet
	} else if headers.HistoryLocation != "" {
		legacySet, d := types.SetValue(
			types.ObjectType{AttrTypes: versioningLegacyAttrTypes},
			[]attr.Value{
				mustObjectValue(versioningLegacyAttrTypes, map[string]attr.Value{
					"type":     types.StringValue("history"),
					"location": types.StringValue(headers.HistoryLocation),
				}),
			},
		)
		diags.Append(d...)
		m.VersioningLegacy = legacySet
	} else if m.VersioningLegacy.IsNull() || m.VersioningLegacy.IsUnknown() {
		emptySet, d := types.SetValue(types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []attr.Value{})
		diags.Append(d...)
		m.VersioningLegacy = emptySet
	}

	// "X-Object-Storage-Class" is returned as "X-Storage-Class" in responses.
	m.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))

	m.Versioning = types.BoolValue(headers.VersionsEnabled)

	// Populate metadata map.
	if len(metadata) > 0 {
		metaValues := make(map[string]attr.Value, len(metadata))
		for k, v := range metadata {
			metaValues[k] = types.StringValue(v)
		}
		metaMap, d := types.MapValue(types.StringType, metaValues)
		diags.Append(d...)
		m.Metadata = metaMap
	} else {
		m.Metadata = types.MapValueMust(types.StringType, map[string]attr.Value{})
	}
}

// Update updates the container.
func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
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

	// Versioning (new-style bool).
	if !plan.Versioning.Equal(state.Versioning) {
		v := plan.Versioning.ValueBool()
		updateOpts.VersionsEnabled = &v
	}

	// Legacy versioning.
	if !plan.VersioningLegacy.Equal(state.VersioningLegacy) {
		if plan.VersioningLegacy.IsNull() || len(plan.VersioningLegacy.Elements()) == 0 {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			vParams, d := versioningLegacyFromSet(ctx, plan.VersioningLegacy)
			resp.Diagnostics.Append(d...)
			if resp.Diagnostics.HasError() {
				return
			}
			if len(vParams["location"]) == 0 || len(vParams["type"]) == 0 {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			} else {
				switch vParams["type"] {
				case "versions":
					updateOpts.VersionsLocation = vParams["location"]
				case "history":
					updateOpts.HistoryLocation = vParams["location"]
				}
			}
		}
	}

	// Remove legacy versioning before enabling new versioning.
	if updateOpts.VersionsEnabled != nil && *updateOpts.VersionsEnabled &&
		(updateOpts.RemoveVersionsLocation == "true" || updateOpts.RemoveHistoryLocation == "true") {
		opts := containers.UpdateOpts{
			RemoveVersionsLocation: "true",
			RemoveHistoryLocation:  "true",
		}
		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	// Remove new versioning before enabling legacy versioning.
	if (updateOpts.VersionsLocation != "" || updateOpts.HistoryLocation != "") &&
		updateOpts.VersionsEnabled != nil && !*updateOpts.VersionsEnabled {
		opts := containers.UpdateOpts{
			VersionsEnabled: updateOpts.VersionsEnabled,
		}
		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
				err.Error(),
			)
			return
		}
	}

	// Metadata.
	if !plan.Metadata.Equal(state.Metadata) {
		var m map[string]string
		resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &m, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		updateOpts.Metadata = m
	}

	_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	// Copy plan into state-shaped model, preserving ID / region.
	plan.ID = state.ID
	plan.Region = types.StringValue(region)

	r.readIntoState(ctx, state.ID.ValueString(), region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the container (force-destroying objects first if requested).
func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.Region
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	id := state.ID.ValueString()

	_, err = containers.Delete(ctx, objectStorageClient, id).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
			log.Printf("[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s': %+v", id, err)

			opts := &objects.ListOpts{Versions: true}
			pager := objects.List(objectStorageClient, id, opts)
			pagerErr := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
				objectList, err := objects.ExtractInfo(page)
				if err != nil {
					return false, fmt.Errorf("error extracting names from objects from page for objectstorage_container_v1 '%s': %+w", id, err)
				}
				for _, object := range objectList {
					delOpts := objects.DeleteOpts{
						ObjectVersionID: object.VersionID,
					}
					_, err = objects.Delete(ctx, objectStorageClient, id, object.Name, delOpts).Extract()
					if err != nil {
						latest := "latest"
						if !object.IsLatest && object.VersionID != "" {
							latest = object.VersionID
						}
						return false, fmt.Errorf("error deleting object '%s@%s' from objectstorage_container_v1 '%s': %+w", object.Name, latest, id, err)
					}
				}
				return true, nil
			})
			if pagerErr != nil {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error force-deleting objects from objectstorage_container_v1 '%s'", id),
					pagerErr.Error(),
				)
				return
			}

			// Retry the container delete after objects are removed.
			_, err = containers.Delete(ctx, objectStorageClient, id).Extract()
			if err != nil {
				if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
					return
				}
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error deleting objectstorage_container_v1 '%s' after force-destroy", id),
					err.Error(),
				)
				return
			}
			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting objectstorage_container_v1 '%s'", id),
			err.Error(),
		)
		return
	}
}

// ImportState implements passthrough import using the container name as ID.
func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// UpgradeState implements ResourceWithUpgradeState. Schema version 0 had a
// "versioning" Set attribute (the legacy block); version 1 renamed that to
// "versioning_legacy" and introduced "versioning" as a Bool.
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Single-step: V0 state → current (V1) state in one call.
		// There is no intermediate V0→V0.5→V1 chain; the transformation is
		// applied directly.
		0: {
			PriorSchema: priorSchemaV0ObjectStorageContainer(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior objectStorageContainerV1ModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				// Translate: V0 "versioning" (Set) → V1 "versioning_legacy" (Set)
				//            V0 has no bool "versioning"; default it to false.
				current := objectStorageContainerV1Model{
					ID:               prior.ID,
					Region:           prior.Region,
					Name:             prior.Name,
					ContainerRead:    prior.ContainerRead,
					ContainerSyncTo:  prior.ContainerSyncTo,
					ContainerSyncKey: prior.ContainerSyncKey,
					ContainerWrite:   prior.ContainerWrite,
					ContentType:      prior.ContentType,
					// The old "versioning" Set becomes "versioning_legacy".
					VersioningLegacy: prior.Versioning,
					// New bool "versioning" defaults to false (V0 never had it).
					Versioning:    types.BoolValue(false),
					Metadata:      prior.Metadata,
					ForceDestroy:  prior.ForceDestroy,
					StoragePolicy: prior.StoragePolicy,
					// V0 had no storage_class; default to empty string.
					StorageClass: types.StringValue(""),
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
			},
		},
	}
}

// priorSchemaV0ObjectStorageContainer returns the framework schema that mirrors
// the SDKv2 V0 schema — required by the framework so it can deserialise old
// state before passing it to the upgrader.
func priorSchemaV0ObjectStorageContainer() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"name": schema.StringAttribute{Required: true},
			"container_read": schema.StringAttribute{Optional: true},
			"container_sync_to": schema.StringAttribute{Optional: true},
			"container_sync_key": schema.StringAttribute{Optional: true},
			"container_write": schema.StringAttribute{Optional: true},
			"content_type": schema.StringAttribute{Optional: true},
			// In V0 this was a TypeSet with sub-attributes; represented as a
			// SetNestedAttribute here so the framework can decode the stored JSON.
			"versioning": schema.SetNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":     schema.StringAttribute{Required: true},
						"location": schema.StringAttribute{Required: true},
					},
				},
			},
			"metadata":       schema.MapAttribute{Optional: true, ElementType: types.StringType},
			"force_destroy":  schema.BoolAttribute{Optional: true, Computed: true},
			"storage_policy": schema.StringAttribute{Optional: true, Computed: true},
		},
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// versioningLegacyFromSet extracts the first element from the versioning_legacy
// set and returns type/location strings.
func versioningLegacyFromSet(ctx context.Context, s types.Set) (map[string]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	type vl struct {
		Type     types.String `tfsdk:"type"`
		Location types.String `tfsdk:"location"`
	}
	var elems []vl
	diags.Append(s.ElementsAs(ctx, &elems, false)...)
	if diags.HasError() || len(elems) == 0 {
		return nil, diags
	}
	return map[string]string{
		"type":     elems[0].Type.ValueString(),
		"location": elems[0].Location.ValueString(),
	}, diags
}

// mustObjectValue builds a types.ObjectValue and panics on error (used only
// with hard-coded attr maps whose shape is guaranteed correct).
func mustObjectValue(attrTypes map[string]attr.Type, attrs map[string]attr.Value) attr.Value {
	v, diags := types.ObjectValue(attrTypes, attrs)
	if diags.HasError() {
		panic(fmt.Sprintf("mustObjectValue: %v", diags))
	}
	return v
}
