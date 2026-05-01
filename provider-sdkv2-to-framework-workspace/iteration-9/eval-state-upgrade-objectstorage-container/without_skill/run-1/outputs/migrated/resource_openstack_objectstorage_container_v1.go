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

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigure   = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithImportState = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}
)

// NewObjectStorageContainerV1Resource is a helper function to simplify the
// provider implementation.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

// objectStorageContainerV1Resource is the resource implementation.
type objectStorageContainerV1Resource struct {
	config *Config
}

// objectStorageContainerV1Model maps the resource schema data.
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

// objectStorageContainerVersioningLegacyModel represents the versioning_legacy block.
type objectStorageContainerVersioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// Metadata returns the resource type name.
func (r *objectStorageContainerV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_objectstorage_container_v1"
}

// Schema defines the schema for the resource.
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

// Configure adds the provider configured client to the resource.
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

// Create creates the resource and sets the initial Terraform state.
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
		diags := plan.Metadata.ElementsAs(ctx, &metadata, false)
		resp.Diagnostics.Append(diags...)
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

	// Handle versioning_legacy block.
	var versioningLegacy []objectStorageContainerVersioningLegacyModel
	if !plan.VersioningLegacy.IsNull() && !plan.VersioningLegacy.IsUnknown() {
		diags := plan.VersioningLegacy.ElementsAs(ctx, &versioningLegacy, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if len(versioningLegacy) > 0 {
		v := versioningLegacy[0]
		switch strings.ToLower(v.Type.ValueString()) {
		case "versions":
			createOpts.VersionsLocation = v.Location.ValueString()
		case "history":
			createOpts.HistoryLocation = v.Location.ValueString()
		}
	}

	log.Printf("[DEBUG] Create Options for objectstorage_container_v1: %#v", createOpts)

	_, err = containers.Create(ctx, objectStorageClient, cn, createOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating objectstorage_container_v1",
			fmt.Sprintf("Could not create container '%s': %s", cn, err),
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

	// Refresh from API.
	r.readIntoState(ctx, cn, region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
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

// readIntoState fetches container data from the API and populates the model.
func (r *objectStorageContainerV1Resource) readIntoState(ctx context.Context, id, region string, state *objectStorageContainerV1Model, diags *diag.Diagnostics) {
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		diags.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	result := containers.Get(ctx, objectStorageClient, id, nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			// Resource no longer exists.
			state.ID = types.StringValue("")
			return
		}
		diags.AddError(
			"Error reading objectstorage_container_v1",
			fmt.Sprintf("Could not read container '%s': %s", id, result.Err),
		)
		return
	}

	headers, err := result.Extract()
	if err != nil {
		diags.AddError(
			"Error extracting headers for objectstorage_container_v1",
			fmt.Sprintf("Could not extract headers for container '%s': %s", id, err),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", id, headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(
			"Error extracting metadata for objectstorage_container_v1",
			fmt.Sprintf("Could not extract metadata for container '%s': %s", id, err),
		)
		return
	}

	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", id, metadata)

	state.ID = types.StringValue(id)
	state.Name = types.StringValue(id)
	state.Region = types.StringValue(region)

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
	} else {
		state.StoragePolicy = types.StringNull()
	}

	// Handle storage_class — returned as X-Storage-Class header.
	storageClass := result.Header.Get("X-Storage-Class")
	state.StorageClass = types.StringValue(storageClass)

	// Handle versioning.
	state.Versioning = types.BoolValue(headers.VersionsEnabled)

	// Handle versioning_legacy.
	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		diags.AddError(
			"Error reading versioning headers for objectstorage_container_v1",
			fmt.Sprintf("Found location for both exclusive types, versions ('%s') and history ('%s')", headers.VersionsLocation, headers.HistoryLocation),
		)
		return
	}

	versioningLegacyElemType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type":     types.StringType,
			"location": types.StringType,
		},
	}

	if headers.VersionsLocation != "" {
		vObj, d := types.ObjectValue(
			versioningLegacyElemType.AttrTypes,
			map[string]attr.Value{
				"type":     types.StringValue("versions"),
				"location": types.StringValue(headers.VersionsLocation),
			},
		)
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		vSet, d := types.SetValue(versioningLegacyElemType, []attr.Value{vObj})
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		state.VersioningLegacy = vSet
	} else if headers.HistoryLocation != "" {
		vObj, d := types.ObjectValue(
			versioningLegacyElemType.AttrTypes,
			map[string]attr.Value{
				"type":     types.StringValue("history"),
				"location": types.StringValue(headers.HistoryLocation),
			},
		)
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		vSet, d := types.SetValue(versioningLegacyElemType, []attr.Value{vObj})
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		state.VersioningLegacy = vSet
	} else {
		emptySet, d := types.SetValue(versioningLegacyElemType, []attr.Value{})
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		state.VersioningLegacy = emptySet
	}

	// Populate metadata.
	if len(metadata) > 0 {
		metadataValues := make(map[string]attr.Value, len(metadata))
		for k, v := range metadata {
			metadataValues[k] = types.StringValue(v)
		}
		metaMap, d := types.MapValue(types.StringType, metadataValues)
		diags.Append(d...)
		if diags.HasError() {
			return
		}
		state.Metadata = metaMap
	} else {
		state.Metadata = types.MapValueMust(types.StringType, map[string]attr.Value{})
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan objectStorageContainerV1Model
	var state objectStorageContainerV1Model
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

	// Handle versioning change.
	if !plan.Versioning.Equal(state.Versioning) {
		versioning := plan.Versioning.ValueBool()
		updateOpts.VersionsEnabled = &versioning
	}

	// Handle versioning_legacy changes.
	versioningChanged := !plan.VersioningLegacy.Equal(state.VersioningLegacy)
	if versioningChanged {
		var versioningLegacy []objectStorageContainerVersioningLegacyModel
		if !plan.VersioningLegacy.IsNull() && !plan.VersioningLegacy.IsUnknown() {
			resp.Diagnostics.Append(plan.VersioningLegacy.ElementsAs(ctx, &versioningLegacy, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}

		if len(versioningLegacy) == 0 {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			v := versioningLegacy[0]
			if v.Location.ValueString() == "" || v.Type.ValueString() == "" {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			}
			switch strings.ToLower(v.Type.ValueString()) {
			case "versions":
				updateOpts.VersionsLocation = v.Location.ValueString()
			case "history":
				updateOpts.HistoryLocation = v.Location.ValueString()
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
		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating objectstorage_container_v1",
				fmt.Sprintf("Could not remove legacy versioning for container '%s': %s", state.ID.ValueString(), err),
			)
			return
		}
	}

	// Remove new versioning first, before enabling the legacy versioning.
	if (updateOpts.VersionsLocation != "" || updateOpts.HistoryLocation != "") &&
		updateOpts.VersionsEnabled != nil && !*updateOpts.VersionsEnabled {
		opts := containers.UpdateOpts{
			VersionsEnabled: updateOpts.VersionsEnabled,
		}
		_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), opts).Extract()
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating objectstorage_container_v1",
				fmt.Sprintf("Could not disable new versioning for container '%s': %s", state.ID.ValueString(), err),
			)
			return
		}
	}

	// Handle metadata changes.
	if !plan.Metadata.Equal(state.Metadata) {
		metadata := make(map[string]string)
		if !plan.Metadata.IsNull() && !plan.Metadata.IsUnknown() {
			resp.Diagnostics.Append(plan.Metadata.ElementsAs(ctx, &metadata, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
		}
		updateOpts.Metadata = metadata
	}

	_, err = containers.Update(ctx, objectStorageClient, state.ID.ValueString(), updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating objectstorage_container_v1",
			fmt.Sprintf("Could not update container '%s': %s", state.ID.ValueString(), err),
		)
		return
	}

	// Refresh state.
	plan.ID = state.ID
	plan.Region = types.StringValue(region)
	r.readIntoState(ctx, state.ID.ValueString(), region, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
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

	containerID := state.ID.ValueString()

	_, err = containers.Delete(ctx, objectStorageClient, containerID).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
			// Container may have objects. Delete them.
			log.Printf("[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s': %+v", containerID, err)

			opts := &objects.ListOpts{
				Versions: true,
			}
			pager := objects.List(objectStorageClient, containerID, opts)
			err = pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
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
			if err != nil {
				resp.Diagnostics.AddError(
					"Error force-destroying objectstorage_container_v1",
					fmt.Sprintf("Could not delete objects in container '%s': %s", containerID, err),
				)
				return
			}

			// Retry deletion now that objects are gone.
			_, err = containers.Delete(ctx, objectStorageClient, containerID).Extract()
			if err != nil {
				if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
					return
				}
				resp.Diagnostics.AddError(
					"Error deleting objectstorage_container_v1",
					fmt.Sprintf("Could not delete container '%s' after force-destroy: %s", containerID, err),
				)
				return
			}
			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			"Error deleting objectstorage_container_v1",
			fmt.Sprintf("Could not delete container '%s': %s", containerID, err),
		)
		return
	}
}

// ImportState imports an existing resource into Terraform.
func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// UpgradeState implements ResourceWithUpgradeState to handle schema version upgrades.
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Upgrade from state version 0 (SDKv2 schema with versioning as a TypeSet)
		// to version 1 (versioning split into versioning bool + versioning_legacy block).
		0: {
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
				Blocks: map[string]schema.Block{
					// In state version 0, "versioning" was a set block (not a bool).
					"versioning": schema.SetNestedBlock{
						NestedObject: schema.NestedBlockObject{
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
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				// Prior state model — mirrors the v0 schema above.
				type versioningV0Model struct {
					Type     types.String `tfsdk:"type"`
					Location types.String `tfsdk:"location"`
				}

				type containerV0Model struct {
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

				var priorState containerV0Model
				resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
				if resp.Diagnostics.HasError() {
					return
				}

				// Build the current-version state model.
				upgradedState := objectStorageContainerV1Model{
					ID:               priorState.ID,
					Region:           priorState.Region,
					Name:             priorState.Name,
					ContainerRead:    priorState.ContainerRead,
					ContainerSyncTo:  priorState.ContainerSyncTo,
					ContainerSyncKey: priorState.ContainerSyncKey,
					ContainerWrite:   priorState.ContainerWrite,
					ContentType:      priorState.ContentType,
					Metadata:         priorState.Metadata,
					ForceDestroy:     priorState.ForceDestroy,
					StoragePolicy:    priorState.StoragePolicy,
					// New versioning bool defaults to false — the old set becomes versioning_legacy.
					Versioning:   types.BoolValue(false),
					StorageClass: types.StringNull(),
				}

				// Move the old versioning set to versioning_legacy.
				versioningLegacyElemType := types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"type":     types.StringType,
						"location": types.StringType,
					},
				}

				if priorState.Versioning.IsNull() || priorState.Versioning.IsUnknown() || len(priorState.Versioning.Elements()) == 0 {
					emptySet, d := types.SetValue(versioningLegacyElemType, []attr.Value{})
					resp.Diagnostics.Append(d...)
					if resp.Diagnostics.HasError() {
						return
					}
					upgradedState.VersioningLegacy = emptySet
				} else {
					var vItems []versioningV0Model
					resp.Diagnostics.Append(priorState.Versioning.ElementsAs(ctx, &vItems, false)...)
					if resp.Diagnostics.HasError() {
						return
					}

					legacyObjs := make([]attr.Value, 0, len(vItems))
					for _, v := range vItems {
						obj, d := types.ObjectValue(
							versioningLegacyElemType.AttrTypes,
							map[string]attr.Value{
								"type":     v.Type,
								"location": v.Location,
							},
						)
						resp.Diagnostics.Append(d...)
						if resp.Diagnostics.HasError() {
							return
						}
						legacyObjs = append(legacyObjs, obj)
					}

					legacySet, d := types.SetValue(versioningLegacyElemType, legacyObjs)
					resp.Diagnostics.Append(d...)
					if resp.Diagnostics.HasError() {
						return
					}
					upgradedState.VersioningLegacy = legacySet
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, &upgradedState)...)
			},
		},
	}
}
