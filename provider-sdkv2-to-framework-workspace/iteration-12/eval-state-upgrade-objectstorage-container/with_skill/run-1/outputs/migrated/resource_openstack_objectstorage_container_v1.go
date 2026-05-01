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
	_ resource.ResourceWithImportState = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}
)

func resourceObjectStorageContainerV1() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

type objectStorageContainerV1Resource struct {
	config *Config
}

// objectStorageContainerV1Model is the current (V1) model.
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

// versioningLegacyModel is the nested model for versioning_legacy set elements.
type versioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

var versioningLegacyAttrTypes = map[string]attr.Type{
	"type":     types.StringType,
	"location": types.StringType,
}

// objectStorageContainerV0Model is the prior (V0) model for state upgrade.
type objectStorageContainerV0Model struct {
	ID               types.String `tfsdk:"id"`
	Region           types.String `tfsdk:"region"`
	Name             types.String `tfsdk:"name"`
	ContainerRead    types.String `tfsdk:"container_read"`
	ContainerSyncTo  types.String `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String `tfsdk:"container_sync_key"`
	ContainerWrite   types.String `tfsdk:"container_write"`
	ContentType      types.String `tfsdk:"content_type"`
	Versioning       types.Set    `tfsdk:"versioning"`
	Metadata         types.Map    `tfsdk:"metadata"`
	ForceDestroy     types.Bool   `tfsdk:"force_destroy"`
	StoragePolicy    types.String `tfsdk:"storage_policy"`
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
				Validators: []validator.Bool{
					// ConflictsWith versioning_legacy handled via plan modifier below.
				},
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
				Validators: []validator.Set{
					// ConflictsWith versioning — verified in Create/Update logic.
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
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
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
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.config = config
}

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, GetRegionFromFramework(ctx, plan.Region, r.config))
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
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
			Metadata:         frameworkContainerMetadata(ctx, plan.Metadata, &resp.Diagnostics),
		},
		StorageClass: plan.StorageClass.ValueString(),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	var versioningLegacyElems []versioningLegacyModel
	resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &versioningLegacyElems, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(versioningLegacyElems) > 0 {
		vRaw := versioningLegacyElems[0]
		switch strings.ToLower(vRaw.Type.ValueString()) {
		case "versions":
			createOpts.VersionsLocation = vRaw.Location.ValueString()
		case "history":
			createOpts.HistoryLocation = vRaw.Location.ValueString()
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

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Refresh from API.
	readReq := resource.ReadRequest{State: resp.State}
	readResp := &resource.ReadResponse{State: resp.State}
	r.Read(ctx, readReq, readResp)
	resp.Diagnostics.Append(readResp.Diagnostics...)
	resp.State = readResp.State
}

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, GetRegionFromFramework(ctx, state.Region, r.config))
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	result := containers.Get(ctx, objectStorageClient, state.ID.ValueString(), nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading objectstorage_container_v1 '%s'", state.ID.ValueString()),
			result.Err.Error(),
		)
		return
	}

	headers, err := result.Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error extracting headers for objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", state.ID.ValueString(), headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error extracting metadata for objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", state.ID.ValueString(), metadata)

	state.Name = state.ID

	if len(headers.Read) > 0 && headers.Read[0] != "" {
		state.ContainerRead = types.StringValue(strings.Join(headers.Read, ","))
	}

	if len(headers.Write) > 0 && headers.Write[0] != "" {
		state.ContainerWrite = types.StringValue(strings.Join(headers.Write, ","))
	}

	if len(headers.StoragePolicy) > 0 {
		state.StoragePolicy = types.StringValue(headers.StoragePolicy)
	}

	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error reading versioning headers for objectstorage_container_v1 '%s'", state.ID.ValueString()),
			fmt.Sprintf("found location for both exclusive types, versions ('%s') and history ('%s')", headers.VersionsLocation, headers.HistoryLocation),
		)
		return
	}

	if headers.VersionsLocation != "" {
		vObj, d := types.ObjectValue(versioningLegacyAttrTypes, map[string]attr.Value{
			"type":     types.StringValue("versions"),
			"location": types.StringValue(headers.VersionsLocation),
		})
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		legacySet, d := types.SetValue(types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []attr.Value{vObj})
		resp.Diagnostics.Append(d...)
		state.VersioningLegacy = legacySet
	} else if headers.HistoryLocation != "" {
		vObj, d := types.ObjectValue(versioningLegacyAttrTypes, map[string]attr.Value{
			"type":     types.StringValue("history"),
			"location": types.StringValue(headers.HistoryLocation),
		})
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		legacySet, d := types.SetValue(types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []attr.Value{vObj})
		resp.Diagnostics.Append(d...)
		state.VersioningLegacy = legacySet
	} else if state.VersioningLegacy.IsNull() || state.VersioningLegacy.IsUnknown() {
		state.VersioningLegacy = types.SetValueMust(types.ObjectType{AttrTypes: versioningLegacyAttrTypes}, []attr.Value{})
	}

	// Despite the create request "X-Object-Storage-Class" header, the
	// response header is "X-Storage-Class".
	state.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))

	state.Versioning = types.BoolValue(headers.VersionsEnabled)
	state.Region = types.StringValue(GetRegionFromFramework(ctx, state.Region, r.config))

	// Metadata — only set if server returned it (preserve existing state if empty).
	if len(metadata) > 0 {
		metaVals := make(map[string]attr.Value, len(metadata))
		for k, v := range metadata {
			metaVals[k] = types.StringValue(v)
		}
		metaMap, d := types.MapValue(types.StringType, metaVals)
		resp.Diagnostics.Append(d...)
		state.Metadata = metaMap
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, GetRegionFromFramework(ctx, state.Region, r.config))
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
		var versioningLegacyElems []versioningLegacyModel
		resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &versioningLegacyElems, false)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if len(versioningLegacyElems) == 0 {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			vRaw := versioningLegacyElems[0]
			if vRaw.Location.ValueString() == "" || vRaw.Type.ValueString() == "" {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			}
			switch strings.ToLower(vRaw.Type.ValueString()) {
			case "versions":
				updateOpts.VersionsLocation = vRaw.Location.ValueString()
			case "history":
				updateOpts.HistoryLocation = vRaw.Location.ValueString()
			}
		}
	}

	// remove legacy versioning first, before enabling the new versioning
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

	// remove new versioning first, before enabling the legacy versioning
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

	if !plan.Metadata.Equal(state.Metadata) {
		updateOpts.Metadata = frameworkContainerMetadata(ctx, plan.Metadata, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
		return
	}

	// Carry the ID forward.
	plan.ID = state.ID

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	readReq := resource.ReadRequest{State: resp.State}
	readResp := &resource.ReadResponse{State: resp.State}
	r.Read(ctx, readReq, readResp)
	resp.Diagnostics.Append(readResp.Diagnostics...)
	resp.State = readResp.State
}

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Delete reads from State, never Plan (Plan is null on delete).
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, GetRegionFromFramework(ctx, state.Region, r.config))
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	_, err = containers.Delete(ctx, objectStorageClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
			// Container may have things. Delete them.
			log.Printf("[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s': %+v", state.ID.ValueString(), err)

			container := state.ID.ValueString()
			opts := &objects.ListOpts{
				Versions: true,
			}
			pager := objects.List(objectStorageClient, container, opts)
			err := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
				objectList, err := objects.ExtractInfo(page)
				if err != nil {
					return false, fmt.Errorf("error extracting names from objects from page for objectstorage_container_v1 '%s': %+w", container, err)
				}

				for _, object := range objectList {
					opts := objects.DeleteOpts{
						ObjectVersionID: object.VersionID,
					}
					_, err = objects.Delete(ctx, objectStorageClient, container, object.Name, opts).Extract()
					if err != nil {
						latest := "latest"
						if !object.IsLatest && object.VersionID != "" {
							latest = object.VersionID
						}
						return false, fmt.Errorf("error deleting object '%s@%s' from objectstorage_container_v1 '%s': %+w", object.Name, latest, container, err)
					}
				}

				return true, nil
			})
			if err != nil {
				resp.Diagnostics.AddError("Error force-deleting objects in container", err.Error())
				return
			}

			// Retry the container deletion.
			_, err = containers.Delete(ctx, objectStorageClient, container).Extract()
			if err != nil {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error deleting objectstorage_container_v1 '%s' after force-destroy", container),
					err.Error(),
				)
			}
			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
		return
	}
}

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// Set name equal to ID on import; Read will populate all other fields.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// UpgradeState implements resource.ResourceWithUpgradeState.
// The SDKv2 resource had SchemaVersion: 1 with a single upgrader for V0→V1.
// In the framework, each upgrader is single-step: keyed at the prior version, it
// must produce the *current* (target) schema's state directly.
func (r *objectStorageContainerV1Resource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// V0 → V1 (current): the SDKv2 V0 schema had "versioning" as a TypeSet block
		// (type + location). V1 renamed that block to "versioning_legacy" and added a
		// new boolean "versioning" attribute. We compose the transformation inline.
		0: {
			PriorSchema: priorSchemaObjectStorageContainerV0(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior objectStorageContainerV0Model
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				// Build the current model: versioning_legacy ← prior.Versioning (the old set),
				// versioning ← false (new boolean field that didn't exist in V0).
				current := objectStorageContainerV1Model{
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
					// storage_class did not exist in V0; default to null/unknown.
					StorageClass: types.StringNull(),
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, current)...)
			},
		},
	}
}

// priorSchemaObjectStorageContainerV0 returns the framework schema that mirrors
// the SDKv2 V0 schema, so the framework can deserialise prior state.
func priorSchemaObjectStorageContainerV0() *schema.Schema {
	return &schema.Schema{
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
			// In V0, "versioning" was a TypeSet block with type+location (the
			// field now known as "versioning_legacy" in V1).
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
	}
}

// frameworkContainerMetadata converts a types.Map into map[string]string for gophercloud.
func frameworkContainerMetadata(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	result := make(map[string]string, len(m.Elements()))
	var vals map[string]types.String
	diags.Append(m.ElementsAs(ctx, &vals, false)...)
	for k, v := range vals {
		result[k] = v.ValueString()
	}
	return result
}

// GetRegionFromFramework resolves the region value, falling back to provider config.
func GetRegionFromFramework(_ context.Context, region types.String, config *Config) string {
	if !region.IsNull() && !region.IsUnknown() && region.ValueString() != "" {
		return region.ValueString()
	}
	return config.GetRegion(nil)
}
