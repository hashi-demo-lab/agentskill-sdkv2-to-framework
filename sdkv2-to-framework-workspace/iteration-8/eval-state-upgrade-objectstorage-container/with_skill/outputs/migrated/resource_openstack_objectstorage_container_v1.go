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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                 = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigure    = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithImportState  = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}
)

// NewObjectStorageContainerV1Resource returns a new framework resource.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

type objectStorageContainerV1Resource struct {
	config *Config
}

// versioningLegacyModel represents one entry in the versioning_legacy block.
type versioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// objectStorageContainerV1Model is the Terraform state model for schema version 1 (current).
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
	VersioningLegacy []versioningLegacyModel `tfsdk:"versioning_legacy"`
	Metadata         types.Map               `tfsdk:"metadata"`
	ForceDestroy     types.Bool              `tfsdk:"force_destroy"`
	StoragePolicy    types.String            `tfsdk:"storage_policy"`
	StorageClass     types.String            `tfsdk:"storage_class"`
}

// objectStorageContainerV0Model is the state model for schema version 0.
// Used only by the UpgradeState PriorSchema deserialiser.
//
// Key difference from V1: "versioning" was a Set block (type+location),
// not a bool. There was no "versioning_legacy" or "storage_class" attribute.
type objectStorageContainerV0Model struct {
	ID               types.String            `tfsdk:"id"`
	Region           types.String            `tfsdk:"region"`
	Name             types.String            `tfsdk:"name"`
	ContainerRead    types.String            `tfsdk:"container_read"`
	ContainerSyncTo  types.String            `tfsdk:"container_sync_to"`
	ContainerSyncKey types.String            `tfsdk:"container_sync_key"`
	ContainerWrite   types.String            `tfsdk:"container_write"`
	ContentType      types.String            `tfsdk:"content_type"`
	Versioning       []versioningLegacyModel `tfsdk:"versioning"`
	Metadata         types.Map               `tfsdk:"metadata"`
	ForceDestroy     types.Bool              `tfsdk:"force_destroy"`
	StoragePolicy    types.String            `tfsdk:"storage_policy"`
}

// ---------------------------------------------------------------------------
// resource.Resource methods
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_objectstorage_container_v1"
}

func (r *objectStorageContainerV1Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// Version must be set to the current schema version so that Terraform
		// knows to invoke UpgradeState when it finds an older state file.
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
			// versioning_legacy is kept as a SetNestedBlock to preserve the
			// practitioner HCL syntax (`versioning_legacy { type = "versions" ... }`).
			// Changing it to a nested attribute would be a breaking HCL change.
			// SDKv2 used TypeSet MaxItems:1, so SetNestedBlock is the direct analogue.
			// Deprecated in favour of the `versioning` bool attribute.
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

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.resolveRegion(plan.Region)
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
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
			Metadata:         r.metadataFromModel(ctx, plan.Metadata, &resp.Diagnostics),
		},
		StorageClass: plan.StorageClass.ValueString(),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	for _, v := range plan.VersioningLegacy {
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
		resp.Diagnostics.AddError("Error creating objectstorage_container_v1", err.Error())
		return
	}

	log.Printf("[INFO] objectstorage_container_v1 created with ID: %s", cn)

	plan.ID = types.StringValue(cn)
	plan.Region = types.StringValue(region)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Refresh from API to populate computed fields.
	r.refreshIntoState(ctx, cn, region, &resp.Diagnostics, &resp.State)
}

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.resolveRegion(state.Region)
	containerID := state.ID.ValueString()

	removed := r.refreshIntoState(ctx, containerID, region, &resp.Diagnostics, &resp.State)
	if removed {
		resp.State.RemoveResource(ctx)
	}
}

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.resolveRegion(state.Region)
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

	versioningChanged := !plan.Versioning.Equal(state.Versioning)
	if versioningChanged {
		v := plan.Versioning.ValueBool()
		updateOpts.VersionsEnabled = &v
	}

	versioningLegacyChanged := r.versioningLegacyChanged(plan.VersioningLegacy, state.VersioningLegacy)
	if versioningLegacyChanged {
		if len(plan.VersioningLegacy) == 0 {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			v := plan.VersioningLegacy[0]
			vType := v.Type.ValueString()
			vLoc := v.Location.ValueString()
			if len(vLoc) == 0 || len(vType) == 0 {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			} else {
				switch strings.ToLower(vType) {
				case "versions":
					updateOpts.VersionsLocation = vLoc
				case "history":
					updateOpts.HistoryLocation = vLoc
				}
			}
		}
	}

	// Remove legacy versioning before enabling the new versioning.
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

	// Remove new versioning before enabling legacy versioning.
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
		updateOpts.Metadata = r.metadataFromModel(ctx, plan.Metadata, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	_, err = containers.Update(ctx, objectStorageClient, containerID, updateOpts).Extract()
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error updating objectstorage_container_v1 '%s'", containerID),
			err.Error(),
		)
		return
	}

	// Commit plan to state then refresh from API.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.refreshIntoState(ctx, containerID, region, &resp.Diagnostics, &resp.State)
}

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.resolveRegion(state.Region)
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

			listOpts := &objects.ListOpts{Versions: true}
			pager := objects.List(objectStorageClient, containerID, listOpts)
			pagerErr := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
				objectList, err := objects.ExtractInfo(page)
				if err != nil {
					return false, fmt.Errorf("error extracting objects for objectstorage_container_v1 '%s': %+w", containerID, err)
				}

				for _, obj := range objectList {
					deleteOpts := objects.DeleteOpts{ObjectVersionID: obj.VersionID}
					_, err = objects.Delete(ctx, objectStorageClient, containerID, obj.Name, deleteOpts).Extract()
					if err != nil {
						latest := "latest"
						if !obj.IsLatest && obj.VersionID != "" {
							latest = obj.VersionID
						}
						return false, fmt.Errorf("error deleting object '%s@%s' from objectstorage_container_v1 '%s': %+w", obj.Name, latest, containerID, err)
					}
				}

				return true, nil
			})
			if pagerErr != nil {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error force-deleting contents of objectstorage_container_v1 '%s'", containerID),
					pagerErr.Error(),
				)
				return
			}

			// Retry deletion after emptying the container.
			_, err = containers.Delete(ctx, objectStorageClient, containerID).Extract()
			if err != nil {
				resp.Diagnostics.AddError(
					fmt.Sprintf("Error deleting objectstorage_container_v1 '%s' after force-destroy", containerID),
					err.Error(),
				)
			}
			return
		}

		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			// Already gone — not an error.
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting objectstorage_container_v1 '%s'", containerID),
			err.Error(),
		)
	}
}

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// State upgrade — single-step, not chained
// ---------------------------------------------------------------------------

// UpgradeState implements resource.ResourceWithUpgradeState.
//
// The framework calls the upgrader keyed by the prior state's schema version.
// Each entry produces the *current* (target) state directly — there is no
// chain. With only one historical version (V0 → V1) there is one entry. If a
// future V2 schema is introduced, a new "1:" entry would upgrade V1 → V2
// directly, and the existing "0:" entry would need to be updated to produce
// V2 state (V0 → V2 in one pass), not V1 state.
func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			// PriorSchema tells the framework how to deserialise the V0 state
			// blob. Without it, req.State.Get would fail because the framework
			// wouldn't know the shape.
			PriorSchema:   priorSchemaObjectStorageContainerV0(),
			StateUpgrader: upgradeObjectStorageContainerStateFromV0,
		},
	}
}

// priorSchemaObjectStorageContainerV0 returns the schema that described
// objectstorage_container_v1 at schema version 0. It mirrors the SDKv2
// resourceObjectStorageContainerV1V0() definition.
//
// V0 differences from V1:
//   - "versioning" was a Set block (type + location), not a bool.
//   - "versioning_legacy" did not exist.
//   - "storage_class" did not exist.
func priorSchemaObjectStorageContainerV0() *schema.Schema {
	return &schema.Schema{
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
			// In V0, "versioning" was a Set block (type+location).
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
	}
}

// upgradeObjectStorageContainerStateFromV0 upgrades a V0 state directly to
// the current (V1) schema. Single-step — it does not call any intermediate
// upgrader.
//
// SDKv2 equivalent:
//
//	rawState["versioning_legacy"] = rawState["versioning"]
//	rawState["versioning"] = false
func upgradeObjectStorageContainerStateFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	// Deserialise V0 state through the PriorSchema.
	var priorState objectStorageContainerV0Model
	resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Produce current (V1) state in one step.
	upgradedState := objectStorageContainerV1Model{
		ID:               priorState.ID,
		Region:           priorState.Region,
		Name:             priorState.Name,
		ContainerRead:    priorState.ContainerRead,
		ContainerSyncTo:  priorState.ContainerSyncTo,
		ContainerSyncKey: priorState.ContainerSyncKey,
		ContainerWrite:   priorState.ContainerWrite,
		ContentType:      priorState.ContentType,
		// V0 "versioning" block → V1 "versioning_legacy" block (same shape).
		VersioningLegacy: priorState.Versioning,
		// V1 "versioning" is now a bool; default to false on upgrade.
		Versioning:    types.BoolValue(false),
		Metadata:      priorState.Metadata,
		ForceDestroy:  priorState.ForceDestroy,
		StoragePolicy: priorState.StoragePolicy,
		// "storage_class" was not present in V0 — default to null.
		StorageClass: types.StringNull(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, upgradedState)...)
}

// ---------------------------------------------------------------------------
// Private helpers
// ---------------------------------------------------------------------------

// resolveRegion returns the configured region string, falling back to the
// provider-level region when the model value is empty or null.
func (r *objectStorageContainerV1Resource) resolveRegion(regionAttr types.String) string {
	if !regionAttr.IsNull() && !regionAttr.IsUnknown() && regionAttr.ValueString() != "" {
		return regionAttr.ValueString()
	}
	return r.config.Region
}

// metadataFromModel converts a types.Map of strings into a map[string]string
// suitable for gophercloud CreateOpts / UpdateOpts.
func (r *objectStorageContainerV1Resource) metadataFromModel(ctx context.Context, m types.Map, d *diag.Diagnostics) map[string]string {
	result := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return result
	}
	var elems map[string]string
	d.Append(m.ElementsAs(ctx, &elems, false)...)
	if d.HasError() {
		return result
	}
	return elems
}

// versioningLegacyChanged returns true when the plan and state differ.
func (r *objectStorageContainerV1Resource) versioningLegacyChanged(plan, state []versioningLegacyModel) bool {
	if len(plan) != len(state) {
		return true
	}
	for i := range plan {
		if !plan[i].Type.Equal(state[i].Type) || !plan[i].Location.Equal(state[i].Location) {
			return true
		}
	}
	return false
}

// refreshIntoState reads the container from the API and writes the result
// back into tfState. Returns true if the resource was not found (caller
// should call RemoveResource).
func (r *objectStorageContainerV1Resource) refreshIntoState(
	ctx context.Context,
	containerID string,
	region string,
	d *diag.Diagnostics,
	tfState *tfsdk.State,
) (removed bool) {
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		d.AddError("Error creating OpenStack object storage client", err.Error())
		return false
	}

	// Read current state so we can preserve user-controlled fields that the
	// API does not echo back (force_destroy, container_sync_to, etc.).
	var state objectStorageContainerV1Model
	d.Append(tfState.Get(ctx, &state)...)
	if d.HasError() {
		return false
	}

	result := containers.Get(ctx, objectStorageClient, containerID, nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			return true
		}
		d.AddError(
			"Error reading objectstorage_container_v1",
			fmt.Sprintf("Could not get container '%s': %s", containerID, result.Err),
		)
		return false
	}

	headers, err := result.Extract()
	if err != nil {
		d.AddError(
			"Error extracting headers for objectstorage_container_v1",
			fmt.Sprintf("'%s': %s", containerID, err),
		)
		return false
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", containerID, headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		d.AddError(
			"Error extracting metadata for objectstorage_container_v1",
			fmt.Sprintf("'%s': %s", containerID, err),
		)
		return false
	}

	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", containerID, metadata)

	state.ID = types.StringValue(containerID)
	state.Name = types.StringValue(containerID)
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
	}

	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		d.AddError(
			"Error reading versioning headers for objectstorage_container_v1",
			fmt.Sprintf("'%s': found location for both exclusive types, versions ('%s') and history ('%s')",
				containerID, headers.VersionsLocation, headers.HistoryLocation),
		)
		return false
	}

	if headers.VersionsLocation != "" {
		state.VersioningLegacy = []versioningLegacyModel{
			{Type: types.StringValue("versions"), Location: types.StringValue(headers.VersionsLocation)},
		}
	} else if headers.HistoryLocation != "" {
		state.VersioningLegacy = []versioningLegacyModel{
			{Type: types.StringValue("history"), Location: types.StringValue(headers.HistoryLocation)},
		}
	} else {
		state.VersioningLegacy = []versioningLegacyModel{}
	}

	// X-Storage-Class is the response header name for storage class.
	state.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))
	state.Versioning = types.BoolValue(headers.VersionsEnabled)

	// Metadata
	if len(metadata) > 0 {
		metaMap, metaDiags := types.MapValueFrom(ctx, types.StringType, metadata)
		d.Append(metaDiags...)
		if d.HasError() {
			return false
		}
		state.Metadata = metaMap
	} else {
		state.Metadata = types.MapValueMust(types.StringType, map[string]types.Value{})
	}

	d.Append(tfState.Set(ctx, state)...)
	return false
}
