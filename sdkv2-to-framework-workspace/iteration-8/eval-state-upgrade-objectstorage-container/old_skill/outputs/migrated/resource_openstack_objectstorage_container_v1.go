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
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                 = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigure    = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithImportState  = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}
)

// NewObjectStorageContainerV1Resource returns the resource constructor for
// registration with the provider.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

// objectStorageContainerV1Resource implements the openstack_objectstorage_container_v1
// resource using terraform-plugin-framework.
type objectStorageContainerV1Resource struct {
	config *Config
}

// versioningLegacyAttrTypes is the attribute-type map for one versioning_legacy element.
var versioningLegacyAttrTypes = map[string]attr.Type{
	"type":     types.StringType,
	"location": types.StringType,
}

// versioningLegacyElemType is the ObjectType for a single versioning_legacy element.
var versioningLegacyElemType = types.ObjectType{AttrTypes: versioningLegacyAttrTypes}

// ---------------------------------------------------------------------------
// Model structs
// ---------------------------------------------------------------------------

// objectStorageContainerV1Model is the current (schema version 1) state model.
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
	VersioningLegacy types.List   `tfsdk:"versioning_legacy"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ForceDestroy     types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy    types.String `tfsdk:"storage_policy"`
	StorageClass     types.String `tfsdk:"storage_class"`
}

// versioningLegacyModel is the typed nested struct for versioning_legacy elements.
type versioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// objectStorageContainerV0Model is the schema-version-0 state model used only
// during state upgrade.  In V0:
//   - "versioning" was a TypeSet of {type, location} objects (renamed to "versioning_legacy" in V1)
//   - there was no boolean "versioning" field
//   - there was no "storage_class" field
type objectStorageContainerV0Model struct {
	ID               types.String `tfsdk:"id"`
	Region           types.String `tfsdk:"region"`
	Name             types.String `tfsdk:"name"`
	ContainerRead    types.String `tfsdk:"container_read"`
	ContainerSyncTo  types.String `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String `tfsdk:"container_sync_key"`
	ContainerWrite   types.String `tfsdk:"container_write"`
	ContentType      types.String `tfsdk:"content_type"`
	Versioning       types.List   `tfsdk:"versioning"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ForceDestroy     types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy    types.String `tfsdk:"storage_policy"`
}

// ---------------------------------------------------------------------------
// resource.Resource
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_objectstorage_container_v1"
}

func (r *objectStorageContainerV1Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Version matches the SDKv2 SchemaVersion: 1.
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

			// versioning: boolean new-style versioning (ConflictsWith versioning_legacy
			// in SDKv2; cross-attribute validators not reproduced here to keep the
			// migration minimal, but can be added via ResourceWithValidateConfig).
			"versioning": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},

			// versioning_legacy: old-style versioning (deprecated).
			// The SDKv2 had TypeSet + MaxItems:1.  We keep this as a list attribute
			// to preserve backward-compat with existing practitioner state files;
			// changing to a block would change the HCL syntax for existing users.
			"versioning_legacy": schema.ListAttribute{
				Optional:           true,
				Computed:           true,
				ElementType:        versioningLegacyElemType,
				DeprecationMessage: `Use newer "versioning" implementation`,
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
// resource.ResourceWithConfigure
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

// ---------------------------------------------------------------------------
// resource.ResourceWithImportState
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// CRUD methods
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, plan.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())

		return
	}

	cn := plan.Name.ValueString()

	metadata := containerMetadataFromModel(plan.Metadata)

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
		var vl []versioningLegacyModel
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

	// Re-read from API to populate all computed fields.
	found, readErr := r.populateStateFromAPI(ctx, &plan, &resp.Diagnostics)
	if readErr != nil || resp.Diagnostics.HasError() {
		return
	}

	if !found {
		resp.Diagnostics.AddError(
			"objectstorage_container_v1 vanished after create",
			fmt.Sprintf("Container '%s' returned 404 immediately after successful creation", cn),
		)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	found, err := r.populateStateFromAPI(ctx, &state, &resp.Diagnostics)
	if err != nil || resp.Diagnostics.HasError() {
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, state.Region.ValueString())
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

	if !plan.Versioning.Equal(state.Versioning) {
		v := plan.Versioning.ValueBool()
		updateOpts.VersionsEnabled = &v
	}

	if !plan.VersioningLegacy.Equal(state.VersioningLegacy) {
		var vl []versioningLegacyModel

		if !plan.VersioningLegacy.IsNull() && !plan.VersioningLegacy.IsUnknown() {
			resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &vl, false)...)

			if resp.Diagnostics.HasError() {
				return
			}
		}

		if len(vl) == 0 {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			if vl[0].Location.ValueString() == "" || vl[0].Type.ValueString() == "" {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			}

			switch strings.ToLower(vl[0].Type.ValueString()) {
			case "versions":
				updateOpts.VersionsLocation = vl[0].Location.ValueString()
			case "history":
				updateOpts.HistoryLocation = vl[0].Location.ValueString()
			}
		}
	}

	// Remove legacy versioning before enabling new-style versioning.
	if updateOpts.VersionsEnabled != nil && *updateOpts.VersionsEnabled &&
		(updateOpts.RemoveVersionsLocation == "true" || updateOpts.RemoveHistoryLocation == "true") {
		clearOpts := containers.UpdateOpts{
			RemoveVersionsLocation: "true",
			RemoveHistoryLocation:  "true",
		}

		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), clearOpts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error clearing legacy versioning on objectstorage_container_v1 '%s'", state.ID.ValueString()),
				err.Error(),
			)

			return
		}
	}

	// Remove new-style versioning before enabling legacy versioning.
	if (updateOpts.VersionsLocation != "" || updateOpts.HistoryLocation != "") &&
		updateOpts.VersionsEnabled != nil && !*updateOpts.VersionsEnabled {
		clearOpts := containers.UpdateOpts{VersionsEnabled: updateOpts.VersionsEnabled}

		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), clearOpts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Error clearing new versioning on objectstorage_container_v1 '%s'", state.ID.ValueString()),
				err.Error(),
			)

			return
		}
	}

	if !plan.Metadata.Equal(state.Metadata) {
		updateOpts.Metadata = containerMetadataFromModel(plan.Metadata)
	}

	_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)

		return
	}

	// Carry immutable fields forward, then re-read computed fields from API.
	plan.ID = state.ID
	plan.Region = state.Region

	found, readErr := r.populateStateFromAPI(ctx, &plan, &resp.Diagnostics)
	if readErr != nil || resp.Diagnostics.HasError() {
		return
	}

	if !found {
		resp.State.RemoveResource(ctx)

		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, state.Region.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())

		return
	}

	_, err = containers.Delete(ctx, objectStorageClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
			log.Printf("[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s': %+v", state.ID.ValueString(), err)

			container := state.ID.ValueString()

			pager := objects.List(objectStorageClient, container, &objects.ListOpts{Versions: true})

			pageErr := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
				objectList, extractErr := objects.ExtractInfo(page)
				if extractErr != nil {
					return false, fmt.Errorf(
						"error extracting objects from objectstorage_container_v1 '%s': %+w",
						container, extractErr,
					)
				}

				for _, obj := range objectList {
					_, delErr := objects.Delete(ctx, objectStorageClient, container, obj.Name,
						objects.DeleteOpts{ObjectVersionID: obj.VersionID}).Extract()
					if delErr != nil {
						latest := "latest"
						if !obj.IsLatest && obj.VersionID != "" {
							latest = obj.VersionID
						}

						return false, fmt.Errorf(
							"error deleting object '%s@%s' from objectstorage_container_v1 '%s': %+w",
							obj.Name, latest, container, delErr,
						)
					}
				}

				return true, nil
			})
			if pageErr != nil {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error force-destroying objects in objectstorage_container_v1 '%s'", state.ID.ValueString()),
					pageErr.Error(),
				)

				return
			}

			// Retry the container deletion now that all objects are removed.
			r.Delete(ctx, req, resp)

			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
	}
}

// ---------------------------------------------------------------------------
// resource.ResourceWithUpgradeState  (single-step semantics)
//
// The SDKv2 resource had SchemaVersion = 1 and one StateUpgraders entry for
// prior version 0.  In the framework, UpgradeState returns a map keyed by the
// *prior* schema version.  Each entry must produce the *current* (target)
// schema's state directly in a single call — there is no chaining.
//
// Entry 0 is the only upgrader because the SDKv2 chain had exactly one step
// (V0 → V1).  The transformation it performs:
//
//	Prior "versioning"  (list of {type,location} objects) → current "versioning_legacy"
//	Current "versioning" (bool)                            → false  (absent in V0)
//	Current "storage_class"                                → null   (absent in V0)
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Upgrade from prior version 0 to current version 1 in a single step.
		0: {
			// PriorSchema describes the V0 shape so the framework can deserialise
			// persisted V0 state bytes into objectStorageContainerV0Model.
			// Every attribute present in V0 must appear here; attributes added in
			// V1 ("versioning_legacy", "storage_class") must NOT appear here.
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed: true,
					},
					"region": schema.StringAttribute{
						Optional: true,
						Computed: true,
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
					// In V0 "versioning" was the TypeSet of {type, location} blocks.
					"versioning": schema.ListAttribute{
						Optional:    true,
						Computed:    true,
						ElementType: versioningLegacyElemType,
					},
					"metadata": schema.MapAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
					"force_destroy": schema.BoolAttribute{
						Optional: true,
						Computed: true,
					},
					"storage_policy": schema.StringAttribute{
						Optional: true,
						Computed: true,
					},
				},
			},

			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				// Deserialise the prior (V0) state.
				var prior objectStorageContainerV0Model
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)

				if resp.Diagnostics.HasError() {
					return
				}

				// Build the current (V1) state in one step, mirroring the SDKv2
				// upgrader logic exactly:
				//   rawState["versioning_legacy"] = rawState["versioning"]
				//   rawState["versioning"]        = false
				//
				// "storage_class" is a V1-only field; it was not persisted in V0
				// state, so we set it to null here and let the next plan/apply
				// refresh it from the API.
				current := objectStorageContainerV1Model{
					ID:               prior.ID,
					Region:           prior.Region,
					Name:             prior.Name,
					ContainerRead:    prior.ContainerRead,
					ContainerSyncTo:  prior.ContainerSyncTo,
					ContainerSyncKey: prior.ContainerSyncKey,
					ContainerWrite:   prior.ContainerWrite,
					ContentType:      prior.ContentType,
					VersioningLegacy: prior.Versioning,      // field rename
					Versioning:       types.BoolValue(false), // new field, default false
					Metadata:         prior.Metadata,
					ForceDestroy:     prior.ForceDestroy,
					StoragePolicy:    prior.StoragePolicy,
					StorageClass:     types.StringNull(), // new field, absent in V0
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

// populateStateFromAPI fetches container data from the OpenStack API and
// writes it into state.  Returns (true, nil) on success, (false, nil) if the
// container is gone (caller should call RemoveResource), or (false, err) on
// other errors.  Any diagnostic errors are also appended to diags.
func (r *objectStorageContainerV1Resource) populateStateFromAPI(
	ctx context.Context,
	state *objectStorageContainerV1Model,
	diags *diag.Diagnostics,
) (bool, error) {
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, state.Region.ValueString())
	if err != nil {
		diags.AddError("Error creating OpenStack object storage client", err.Error())

		return false, err
	}

	result := containers.Get(ctx, objectStorageClient, state.ID.ValueString(), nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			return false, nil
		}

		diags.AddError(
			fmt.Sprintf("Error reading objectstorage_container_v1 '%s'", state.ID.ValueString()),
			result.Err.Error(),
		)

		return false, result.Err
	}

	headers, err := result.Extract()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Error extracting headers for objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)

		return false, err
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", state.ID.ValueString(), headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Error extracting metadata for objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)

		return false, err
	}

	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", state.ID.ValueString(), metadata)

	// name == ID for this resource type.
	state.Name = state.ID

	if len(headers.Read) > 0 && headers.Read[0] != "" {
		state.ContainerRead = types.StringValue(strings.Join(headers.Read, ","))
	} else {
		state.ContainerRead = types.StringNull()
	}

	if len(headers.Write) > 0 && headers.Write[0] != "" {
		state.ContainerWrite = types.StringValue(strings.Join(headers.Write, ","))
	} else {
		state.ContainerWrite = types.StringNull()
	}

	if len(headers.StoragePolicy) > 0 {
		state.StoragePolicy = types.StringValue(headers.StoragePolicy)
	}

	// X-Storage-Class (response) differs from the create header X-Object-Storage-Class.
	state.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))

	state.Versioning = types.BoolValue(headers.VersionsEnabled)

	// Build metadata map.
	if len(metadata) > 0 {
		metaVals := make(map[string]attr.Value, len(metadata))
		for k, v := range metadata {
			metaVals[k] = types.StringValue(v)
		}

		metaMap, mapDiags := types.MapValue(types.StringType, metaVals)
		diags.Append(mapDiags...)

		if diags.HasError() {
			return false, fmt.Errorf("error building metadata map")
		}

		state.Metadata = metaMap
	} else {
		state.Metadata = types.MapValueMust(types.StringType, map[string]attr.Value{})
	}

	// versioning_legacy: VersionsLocation and HistoryLocation are mutually exclusive.
	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		diags.AddError(
			fmt.Sprintf("Invalid versioning headers for objectstorage_container_v1 '%s'", state.ID.ValueString()),
			fmt.Sprintf("API returned location for both exclusive types — versions('%s') and history('%s')",
				headers.VersionsLocation, headers.HistoryLocation),
		)

		return false, fmt.Errorf("ambiguous versioning headers")
	}

	switch {
	case headers.VersionsLocation != "":
		elem, elemDiags := types.ObjectValue(versioningLegacyAttrTypes, map[string]attr.Value{
			"type":     types.StringValue("versions"),
			"location": types.StringValue(headers.VersionsLocation),
		})
		diags.Append(elemDiags...)
		list, listDiags := types.ListValue(versioningLegacyElemType, []attr.Value{elem})
		diags.Append(listDiags...)
		state.VersioningLegacy = list

	case headers.HistoryLocation != "":
		elem, elemDiags := types.ObjectValue(versioningLegacyAttrTypes, map[string]attr.Value{
			"type":     types.StringValue("history"),
			"location": types.StringValue(headers.HistoryLocation),
		})
		diags.Append(elemDiags...)
		list, listDiags := types.ListValue(versioningLegacyElemType, []attr.Value{elem})
		diags.Append(listDiags...)
		state.VersioningLegacy = list

	default:
		state.VersioningLegacy = types.ListValueMust(versioningLegacyElemType, []attr.Value{})
	}

	return !diags.HasError(), nil
}

// containerMetadataFromModel converts a framework types.Map to the
// map[string]string used by the gophercloud containers API.
func containerMetadataFromModel(m types.Map) map[string]string {
	if m.IsNull() || m.IsUnknown() || len(m.Elements()) == 0 {
		return nil
	}

	result := make(map[string]string, len(m.Elements()))

	for k, v := range m.Elements() {
		if sv, ok := v.(types.String); ok {
			result[k] = sv.ValueString()
		}
	}

	return result
}
