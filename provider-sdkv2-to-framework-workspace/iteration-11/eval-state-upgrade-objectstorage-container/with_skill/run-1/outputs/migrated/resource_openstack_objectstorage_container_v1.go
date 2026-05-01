// Migrated from SDKv2 to terraform-plugin-framework.
//
// State upgraders use single-step (framework) semantics:
//   - Entry keyed at 0 upgrades V0 state directly to the current (V1) schema.
//
// The V0 schema had "versioning" as a TypeSet block; V1 renamed that to
// "versioning_legacy" and introduced "versioning" as a bool.  The upgrader
// moves the old set value into "versioning_legacy" and sets "versioning" to
// false — exactly what the SDKv2 upgrader did, expressed as a single-step
// framework upgrader that produces the current (V1) state directly.

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time assertions.
var _ resource.Resource = &objectStorageContainerV1Resource{}
var _ resource.ResourceWithImportState = &objectStorageContainerV1Resource{}
var _ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}

// objectStorageContainerV1Resource is the framework implementation of
// openstack_objectstorage_container_v1.
type objectStorageContainerV1Resource struct {
	config *Config
}

// NewObjectStorageContainerV1Resource is the constructor registered with the
// provider's Resources list.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

// ---------------------------------------------------------------------------
// Metadata
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_objectstorage_container_v1"
}

// ---------------------------------------------------------------------------
// Schema — current schema version is 1
// ---------------------------------------------------------------------------

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
				DeprecationMessage: `Use newer "versioning" implementation`,
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

// ---------------------------------------------------------------------------
// Typed model — current (V1) schema
// ---------------------------------------------------------------------------

// versioningLegacyEntry represents one element of the versioning_legacy set.
type versioningLegacyEntry struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// objectStorageContainerModel is the full typed model for the current schema.
type objectStorageContainerModel struct {
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

// versioningLegacyElemAttrTypes returns the attr.Type map for a
// versioningLegacyEntry object value.
func versioningLegacyElemAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":     types.StringType,
		"location": types.StringType,
	}
}

// ---------------------------------------------------------------------------
// Configure — receive provider meta
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)
		return
	}
	r.config = config
}

// getRegion resolves the effective region: use the plan/state value if set,
// otherwise fall back to the provider-level region.
func (r *objectStorageContainerV1Resource) getRegion(regionAttr types.String) string {
	if !regionAttr.IsNull() && !regionAttr.IsUnknown() && regionAttr.ValueString() != "" {
		return regionAttr.ValueString()
	}
	return r.config.Region
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageContainerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.getRegion(plan.Region)
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	cn := plan.Name.ValueString()

	metadata := map[string]string{}
	if !plan.Metadata.IsNull() && !plan.Metadata.IsUnknown() {
		resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
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
		var vl []versioningLegacyEntry
		resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &vl, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		switch strings.ToLower(vl[0].Type.ValueString()) {
		case "versions":
			createOpts.VersionsLocation = vl[0].Location.ValueString()
		case "history":
			createOpts.HistoryLocation = vl[0].Location.ValueString()
		}
	}

	log.Printf("[DEBUG] Create Options for objectstorage_container_v1: %#v", createOpts)

	_, err = containers.Create(ctx, objectStorageClient, cn, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError("Error creating objectstorage_container_v1", err.Error())
		return
	}

	log.Printf("[INFO] objectstorage_container_v1 created with ID: %s", cn)

	plan.ID = types.StringValue(cn)
	plan.Region = types.StringValue(region)

	readIntoContainerModel(ctx, objectStorageClient, cn, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Read
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageContainerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.getRegion(state.Region)
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	readIntoContainerModel(ctx, objectStorageClient, state.ID.ValueString(), region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the resource was removed remotely, clear state.
	if state.ID.IsNull() {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// readIntoContainerModel performs the API Get and populates model fields.
// On 404, model.ID is set to types.StringNull() so the caller can detect it.
func readIntoContainerModel(
	ctx context.Context,
	objectStorageClient *gophercloud.ServiceClient,
	containerName string,
	region string,
	model *objectStorageContainerModel,
	diags *diag.Diagnostics,
) {
	result := containers.Get(ctx, objectStorageClient, containerName, nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			model.ID = types.StringNull()
			return
		}
		diags.AddError(
			fmt.Sprintf("Error reading objectstorage_container_v1 '%s'", containerName),
			result.Err.Error(),
		)
		return
	}

	headers, err := result.Extract()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Error extracting headers for objectstorage_container_v1 '%s'", containerName),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", containerName, headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Error extracting metadata for objectstorage_container_v1 '%s'", containerName),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", containerName, metadata)

	model.ID = types.StringValue(containerName)
	model.Name = types.StringValue(containerName)
	model.Region = types.StringValue(region)

	if len(headers.Read) > 0 && headers.Read[0] != "" {
		model.ContainerRead = types.StringValue(strings.Join(headers.Read, ","))
	} else {
		model.ContainerRead = types.StringValue("")
	}

	if len(headers.Write) > 0 && headers.Write[0] != "" {
		model.ContainerWrite = types.StringValue(strings.Join(headers.Write, ","))
	} else {
		model.ContainerWrite = types.StringValue("")
	}

	if len(headers.StoragePolicy) > 0 {
		model.StoragePolicy = types.StringValue(headers.StoragePolicy)
	} else {
		model.StoragePolicy = types.StringValue("")
	}

	// versioning_legacy — mutual exclusion is enforced by OpenStack, so at most
	// one of VersionsLocation / HistoryLocation will be set.
	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		diags.AddError(
			fmt.Sprintf("Error reading versioning headers for objectstorage_container_v1 '%s'", containerName),
			fmt.Sprintf("found location for both exclusive types, versions ('%s') and history ('%s')",
				headers.VersionsLocation, headers.HistoryLocation),
		)
		return
	}

	elemType := types.ObjectType{AttrTypes: versioningLegacyElemAttrTypes()}

	if headers.VersionsLocation != "" {
		elem, d := types.ObjectValue(versioningLegacyElemAttrTypes(), map[string]attr.Value{
			"type":     types.StringValue("versions"),
			"location": types.StringValue(headers.VersionsLocation),
		})
		diags.Append(d...)
		setVal, d2 := types.SetValue(elemType, []attr.Value{elem})
		diags.Append(d2...)
		model.VersioningLegacy = setVal
	} else if headers.HistoryLocation != "" {
		elem, d := types.ObjectValue(versioningLegacyElemAttrTypes(), map[string]attr.Value{
			"type":     types.StringValue("history"),
			"location": types.StringValue(headers.HistoryLocation),
		})
		diags.Append(d...)
		setVal, d2 := types.SetValue(elemType, []attr.Value{elem})
		diags.Append(d2...)
		model.VersioningLegacy = setVal
	} else {
		emptySet, d := types.SetValue(elemType, []attr.Value{})
		diags.Append(d...)
		model.VersioningLegacy = emptySet
	}

	// storage_class — created via "X-Object-Storage-Class", returned as "X-Storage-Class".
	model.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))

	model.Versioning = types.BoolValue(headers.VersionsEnabled)

	// metadata
	if len(metadata) > 0 {
		metaVals := make(map[string]attr.Value, len(metadata))
		for k, v := range metadata {
			metaVals[k] = types.StringValue(v)
		}
		metaMap, d := types.MapValue(types.StringType, metaVals)
		diags.Append(d...)
		model.Metadata = metaMap
	} else {
		emptyMeta, d := types.MapValue(types.StringType, map[string]attr.Value{})
		diags.Append(d...)
		model.Metadata = emptyMeta
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageContainerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.getRegion(state.Region)
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	containerID := state.ID.ValueString()

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

	if !plan.VersioningLegacy.Equal(state.VersioningLegacy) {
		if plan.VersioningLegacy.IsNull() || len(plan.VersioningLegacy.Elements()) == 0 {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			var vl []versioningLegacyEntry
			resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &vl, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			if len(vl) > 0 {
				loc := vl[0].Location.ValueString()
				typ := vl[0].Type.ValueString()
				if loc == "" || typ == "" {
					updateOpts.RemoveVersionsLocation = "true"
					updateOpts.RemoveHistoryLocation = "true"
				} else {
					switch strings.ToLower(typ) {
					case "versions":
						updateOpts.VersionsLocation = loc
					case "history":
						updateOpts.HistoryLocation = loc
					}
				}
			}
		}
	}

	// Remove legacy versioning before enabling the new versioning flag.
	if updateOpts.VersionsEnabled != nil && *updateOpts.VersionsEnabled &&
		(updateOpts.RemoveVersionsLocation == "true" || updateOpts.RemoveHistoryLocation == "true") {
		opts := containers.UpdateOpts{
			RemoveVersionsLocation: "true",
			RemoveHistoryLocation:  "true",
		}
		_, err = containers.Update(ctx, objectStorageClient, containerID, opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", containerID),
				err.Error(),
			)
			return
		}
	}

	// Remove new versioning before enabling the legacy versioning flag.
	if (updateOpts.VersionsLocation != "" || updateOpts.HistoryLocation != "") &&
		updateOpts.VersionsEnabled != nil && !*updateOpts.VersionsEnabled {
		opts := containers.UpdateOpts{
			VersionsEnabled: updateOpts.VersionsEnabled,
		}
		_, err = containers.Update(ctx, objectStorageClient, containerID, opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", containerID),
				err.Error(),
			)
			return
		}
	}

	if !plan.Metadata.Equal(state.Metadata) {
		metadata := map[string]string{}
		if !plan.Metadata.IsNull() && !plan.Metadata.IsUnknown() {
			resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}
		updateOpts.Metadata = metadata
	}

	_, err = containers.Update(ctx, objectStorageClient, containerID, updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", containerID),
			err.Error(),
		)
		return
	}

	// Read back updated state.
	plan.ID = state.ID
	plan.Region = types.StringValue(region)
	readIntoContainerModel(ctx, objectStorageClient, containerID, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Delete reads from State, not Plan — req.Plan is null on Delete.
	var state objectStorageContainerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.getRegion(state.Region)
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	containerID := state.ID.ValueString()

	_, err = containers.Delete(ctx, objectStorageClient, containerID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
			log.Printf("[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s': %+v", containerID, err)

			opts := &objects.ListOpts{Versions: true}
			pager := objects.List(objectStorageClient, containerID, opts)
			pageErr := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
				objectList, err := objects.ExtractInfo(page)
				if err != nil {
					return false, fmt.Errorf("error extracting names from objects from page for objectstorage_container_v1 '%s': %+w", containerID, err)
				}
				for _, object := range objectList {
					deleteOpts := objects.DeleteOpts{
						ObjectVersionID: object.VersionID,
					}
					_, err = objects.Delete(ctx, objectStorageClient, containerID, object.Name, deleteOpts).Extract()
					if err != nil {
						latest := "latest"
						if !object.IsLatest && object.VersionID != "" {
							latest = object.VersionID
						}
						return false, fmt.Errorf("error deleting object '%s@%s' from objectstorage_container_v1 '%s': %+w", object.Name, latest, containerID, err)
					}
				}
				return true, nil
			})
			if pageErr != nil {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error force-deleting contents of objectstorage_container_v1 '%s'", containerID),
					pageErr.Error(),
				)
				return
			}

			// Retry the delete now that the container is empty.
			r.Delete(ctx, req, resp)
			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting objectstorage_container_v1 '%s'", containerID),
			err.Error(),
		)
		return
	}
}

// ---------------------------------------------------------------------------
// ImportState
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// UpgradeState — single-step framework semantics
// ---------------------------------------------------------------------------

// UpgradeState returns a map of single-step upgraders keyed by prior schema
// version.  The framework calls each map entry independently — there is no
// chain.  Each entry must produce the *current* (V1) state directly.
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// V0 → current (V1):
		// The V0 schema stored "versioning" as a TypeSet of {type, location}.
		// V1 renamed that to "versioning_legacy" and changed "versioning" to bool.
		// This upgrader applies both transformations in one step and writes the
		// current schema state directly — it never produces an intermediate state.
		0: {
			PriorSchema:   priorSchemaV0ObjectStorageContainer(),
			StateUpgrader: upgradeObjectStorageContainerFromV0,
		},
	}
}

// ---------------------------------------------------------------------------
// Prior schema — V0
// ---------------------------------------------------------------------------

// priorSchemaV0ObjectStorageContainer describes the on-disk state shape stored
// by the SDKv2 V0 provider.  Attribute names and types must exactly match what
// SDKv2 stored; the framework deserialises via this schema before calling the
// upgrader function.
func priorSchemaV0ObjectStorageContainer() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id":                 schema.StringAttribute{Computed: true},
			"region":             schema.StringAttribute{Optional: true, Computed: true},
			"name":               schema.StringAttribute{Required: true},
			"container_read":     schema.StringAttribute{Optional: true},
			"container_sync_to":  schema.StringAttribute{Optional: true},
			"container_sync_key": schema.StringAttribute{Optional: true},
			"container_write":    schema.StringAttribute{Optional: true},
			"content_type":       schema.StringAttribute{Optional: true},
			// In V0 "versioning" was a TypeSet block with type+location elements.
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
// Typed model — V0
// ---------------------------------------------------------------------------

// versioningLegacyEntryV0 is the element type for the V0 "versioning" set.
// tfsdk tags must exactly match the V0 "versioning" nested attributes.
type versioningLegacyEntryV0 struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// objectStorageContainerModelV0 is the typed model for V0 prior state.
// tfsdk tags must exactly match priorSchemaV0ObjectStorageContainer attribute names.
type objectStorageContainerModelV0 struct {
	ID               types.String `tfsdk:"id"`
	Region           types.String `tfsdk:"region"`
	Name             types.String `tfsdk:"name"`
	ContainerRead    types.String `tfsdk:"container_read"`
	ContainerSyncTo  types.String `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String `tfsdk:"container_sync_key"`
	ContainerWrite   types.String `tfsdk:"container_write"`
	ContentType      types.String `tfsdk:"content_type"`
	// In V0 "versioning" was a set of {type, location} blocks.
	Versioning    types.Set    `tfsdk:"versioning"`
	Metadata      types.Map    `tfsdk:"metadata"`
	ForceDestroy  types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy types.String `tfsdk:"storage_policy"`
}

// ---------------------------------------------------------------------------
// Upgrader function — V0 → current (V1)
// ---------------------------------------------------------------------------

// upgradeObjectStorageContainerFromV0 upgrades V0 state to the current (V1)
// schema in a single step.
//
// The transformation mirrors the SDKv2 upgrader exactly:
//   - "versioning" (set of {type,location}) → "versioning_legacy"
//   - "versioning" (bool) set to false
//   - All other fields carried forward unchanged.
//   - "storage_class", added in V1, was not stored by V0; defaults to "".
func upgradeObjectStorageContainerFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior objectStorageContainerModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	elemType := types.ObjectType{AttrTypes: versioningLegacyElemAttrTypes()}

	// Move V0 "versioning" set → "versioning_legacy".
	var versioningLegacy types.Set
	if prior.Versioning.IsNull() || prior.Versioning.IsUnknown() || len(prior.Versioning.Elements()) == 0 {
		empty, d := types.SetValue(elemType, []attr.Value{})
		resp.Diagnostics.Append(d...)
		versioningLegacy = empty
	} else {
		var v0Entries []versioningLegacyEntryV0
		resp.Diagnostics.Append(prior.Versioning.ElementsAs(ctx, &v0Entries, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		newElems := make([]attr.Value, 0, len(v0Entries))
		for _, e := range v0Entries {
			objVal, d := types.ObjectValue(versioningLegacyElemAttrTypes(), map[string]attr.Value{
				"type":     e.Type,
				"location": e.Location,
			})
			resp.Diagnostics.Append(d...)
			newElems = append(newElems, objVal)
		}
		setVal, d := types.SetValue(elemType, newElems)
		resp.Diagnostics.Append(d...)
		versioningLegacy = setVal
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Carry metadata forward; default to empty map if absent.
	metadata := prior.Metadata
	if metadata.IsNull() || metadata.IsUnknown() {
		empty, d := types.MapValue(types.StringType, map[string]attr.Value{})
		resp.Diagnostics.Append(d...)
		metadata = empty
	}

	current := objectStorageContainerModel{
		ID:               prior.ID,
		Region:           prior.Region,
		Name:             prior.Name,
		ContainerRead:    prior.ContainerRead,
		ContainerSyncTo:  prior.ContainerSyncTo,
		ContainerSyncKey: prior.ContainerSyncKey,
		ContainerWrite:   prior.ContainerWrite,
		ContentType:      prior.ContentType,
		// "versioning" becomes a bool; the SDKv2 upgrader set it to false.
		Versioning:       types.BoolValue(false),
		VersioningLegacy: versioningLegacy,
		Metadata:         metadata,
		ForceDestroy:     prior.ForceDestroy,
		StoragePolicy:    prior.StoragePolicy,
		// "storage_class" was not stored in V0; use empty string.
		StorageClass: types.StringValue(""),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}
