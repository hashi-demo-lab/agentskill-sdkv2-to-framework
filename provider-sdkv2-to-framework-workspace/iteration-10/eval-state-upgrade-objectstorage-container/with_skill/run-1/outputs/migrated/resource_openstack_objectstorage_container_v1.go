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
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure interface compliance.
var _ resource.Resource = &objectStorageContainerV1Resource{}
var _ resource.ResourceWithImportState = &objectStorageContainerV1Resource{}
var _ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}

func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

type objectStorageContainerV1Resource struct {
	config *Config
}

// containerModel is the Terraform state model for schema version 1 (current).
type containerModel struct {
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

// versioningLegacyModel is the nested object type for versioning_legacy.
type versioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

var versioningLegacyAttrTypes = map[string]attr.Type{
	"type":     types.StringType,
	"location": types.StringType,
}

func (r *objectStorageContainerV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_objectstorage_container_v1"
}

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

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan containerModel
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
		metaMap := make(map[string]types.String)
		resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metaMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range metaMap {
			metadata[k] = v.ValueString()
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
		vItems := make([]versioningLegacyModel, 0, 1)
		resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &vItems, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if len(vItems) > 0 {
			switch strings.ToLower(vItems[0].Type.ValueString()) {
			case "versions":
				createOpts.VersionsLocation = vItems[0].Location.ValueString()
			case "history":
				createOpts.HistoryLocation = vItems[0].Location.ValueString()
			}
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

	// Read back so computed fields are populated.
	r.readIntoModel(ctx, objectStorageClient, cn, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state containerModel
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

	r.readIntoModel(ctx, objectStorageClient, state.ID.ValueString(), region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// readIntoModel reads the container from the API and fills in model fields.
// It handles 404 by removing the resource from state (resp.State.RemoveResource is called via diag).
func (r *objectStorageContainerV1Resource) readIntoModel(ctx context.Context, client *gophercloud.ServiceClient, id, region string, model *containerModel, diags *diag.Diagnostics) {
	result := containers.Get(ctx, client, id, nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			log.Printf("[DEBUG] objectstorage_container_v1 '%s' not found, removing from state", id)
			model.ID = types.StringNull()
			return
		}
		diags.AddError(fmt.Sprintf("Error reading objectstorage_container_v1 '%s'", id), result.Err.Error())
		return
	}

	headers, err := result.Extract()
	if err != nil {
		diags.AddError(fmt.Sprintf("Error extracting headers for objectstorage_container_v1 '%s'", id), err.Error())
		return
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", id, headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(fmt.Sprintf("Error extracting metadata for objectstorage_container_v1 '%s'", id), err.Error())
		return
	}

	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", id, metadata)

	model.Name = types.StringValue(id)
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
	}

	model.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))
	model.Versioning = types.BoolValue(headers.VersionsEnabled)

	// Handle versioning_legacy
	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		diags.AddError(
			fmt.Sprintf("Error reading versioning headers for objectstorage_container_v1 '%s'", id),
			fmt.Sprintf("found location for both exclusive types, versions ('%s') and history ('%s')", headers.VersionsLocation, headers.HistoryLocation),
		)
		return
	}

	if headers.VersionsLocation != "" {
		vItem := versioningLegacyModel{
			Type:     types.StringValue("versions"),
			Location: types.StringValue(headers.VersionsLocation),
		}
		setVal, d := types.SetValueFrom(ctx, types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []versioningLegacyModel{vItem})
		diags.Append(d...)
		model.VersioningLegacy = setVal
	} else if headers.HistoryLocation != "" {
		vItem := versioningLegacyModel{
			Type:     types.StringValue("history"),
			Location: types.StringValue(headers.HistoryLocation),
		}
		setVal, d := types.SetValueFrom(ctx, types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []versioningLegacyModel{vItem})
		diags.Append(d...)
		model.VersioningLegacy = setVal
	} else {
		if model.VersioningLegacy.IsNull() || model.VersioningLegacy.IsUnknown() {
			model.VersioningLegacy = types.SetValueMust(types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []attr.Value{})
		}
	}

	// Metadata
	if len(metadata) > 0 {
		metaAttrs := make(map[string]attr.Value, len(metadata))
		for k, v := range metadata {
			metaAttrs[k] = types.StringValue(v)
		}
		mapVal, d := types.MapValue(types.StringType, metaAttrs)
		diags.Append(d...)
		model.Metadata = mapVal
	}
}

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state containerModel
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

	id := state.ID.ValueString()

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
			vItems := make([]versioningLegacyModel, 0, 1)
			resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &vItems, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			if len(vItems) > 0 {
				if vItems[0].Location.ValueString() == "" || vItems[0].Type.ValueString() == "" {
					updateOpts.RemoveVersionsLocation = "true"
					updateOpts.RemoveHistoryLocation = "true"
				} else {
					switch strings.ToLower(vItems[0].Type.ValueString()) {
					case "versions":
						updateOpts.VersionsLocation = vItems[0].Location.ValueString()
					case "history":
						updateOpts.HistoryLocation = vItems[0].Location.ValueString()
					}
				}
			}
		}
	}

	// Remove legacy versioning first, before enabling the new versioning.
	if updateOpts.VersionsEnabled != nil && *updateOpts.VersionsEnabled &&
		(updateOpts.RemoveVersionsLocation == "true" || updateOpts.RemoveHistoryLocation == "true") {
		opts := containers.UpdateOpts{
			RemoveVersionsLocation: "true",
			RemoveHistoryLocation:  "true",
		}
		_, err = containers.Update(ctx, objectStorageClient, id, opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", id), err.Error())
			return
		}
	}

	// Remove new versioning first, before enabling the legacy versioning.
	if (updateOpts.VersionsLocation != "" || updateOpts.HistoryLocation != "") &&
		updateOpts.VersionsEnabled != nil && !*updateOpts.VersionsEnabled {
		opts := containers.UpdateOpts{
			VersionsEnabled: updateOpts.VersionsEnabled,
		}
		_, err = containers.Update(ctx, objectStorageClient, id, opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", id), err.Error())
			return
		}
	}

	if !plan.Metadata.Equal(state.Metadata) {
		metaMap := make(map[string]types.String)
		resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metaMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		metadata := make(map[string]string, len(metaMap))
		for k, v := range metaMap {
			metadata[k] = v.ValueString()
		}
		updateOpts.Metadata = metadata
	}

	_, err = containers.Update(ctx, objectStorageClient, id, updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", id), err.Error())
		return
	}

	plan.ID = state.ID
	plan.Region = state.Region

	r.readIntoModel(ctx, objectStorageClient, id, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state containerModel
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

			opts := &objects.ListOpts{
				Versions: true,
			}
			pager := objects.List(objectStorageClient, id, opts)
			err = pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
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
			if err != nil {
				resp.Diagnostics.AddError(fmt.Sprintf("Error force-destroying objectstorage_container_v1 '%s'", id), err.Error())
				return
			}

			// Retry the delete after clearing objects.
			r.Delete(ctx, req, resp)
			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(fmt.Sprintf("Error deleting objectstorage_container_v1 '%s'", id), err.Error())
	}
}

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	region := r.config.Region

	var state containerModel
	state.ID = types.StringValue(req.ID)
	state.Region = types.StringValue(region)
	state.ForceDestroy = types.BoolValue(false)
	state.VersioningLegacy = types.SetValueMust(types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []attr.Value{})
	state.Metadata = types.MapValueMust(types.StringType, map[string]attr.Value{})

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	r.readIntoModel(ctx, objectStorageClient, req.ID, region, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// UpgradeState implements single-step framework semantics: each map entry keyed
// at a prior version directly produces the *current* (version 1) state. There
// is no chaining between upgrader entries.
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// V0 → V1 (current): "versioning" was a TypeSet block; it became
		// "versioning_legacy" (still a set block) and "versioning" became a bool.
		0: {
			PriorSchema: &schema.Schema{
				Version: 0,
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
					"versioning": schema.SetNestedAttribute{
						Optional: true,
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"type": schema.StringAttribute{
									Required: true,
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
					},
					"storage_policy": schema.StringAttribute{
						Optional: true,
						Computed: true,
					},
				},
			},
			StateUpgrader: upgradeObjectStorageContainerStateFromV0,
		},
	}
}

// containerModelV0 matches the V0 schema. The "versioning" attribute was a
// set-nested block (same shape as V1's versioning_legacy).
type containerModelV0 struct {
	ID               types.String `tfsdk:"id"`
	Region           types.String `tfsdk:"region"`
	Name             types.String `tfsdk:"name"`
	ContainerRead    types.String `tfsdk:"container_read"`
	ContainerSyncTo  types.String `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String `tfsdk:"container_sync_key"`
	ContainerWrite   types.String `tfsdk:"container_write"`
	ContentType      types.String `tfsdk:"content_type"`
	// In V0 "versioning" was the legacy set-type block, not a bool.
	Versioning    types.Set    `tfsdk:"versioning"`
	Metadata      types.Map    `tfsdk:"metadata"`
	ForceDestroy  types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy types.String `tfsdk:"storage_policy"`
}

// upgradeObjectStorageContainerStateFromV0 converts V0 state directly to the
// current (V1) schema in a single step — no intermediate state is produced.
func upgradeObjectStorageContainerStateFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior containerModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build the current model. versioning_legacy gets the old "versioning" set;
	// the new "versioning" bool defaults to false.
	current := containerModel{
		ID:               prior.ID,
		Region:           prior.Region,
		Name:             prior.Name,
		ContainerRead:    prior.ContainerRead,
		ContainerSyncTo:  prior.ContainerSyncTo,
		ContainerSyncKey: prior.ContainerSyncKey,
		ContainerWrite:   prior.ContainerWrite,
		ContentType:      prior.ContentType,
		Versioning:       types.BoolValue(false),
		VersioningLegacy: prior.Versioning,
		Metadata:         prior.Metadata,
		ForceDestroy:     prior.ForceDestroy,
		StoragePolicy:    prior.StoragePolicy,
		// storage_class was added in V1; default to empty string.
		StorageClass: types.StringValue(""),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
}

