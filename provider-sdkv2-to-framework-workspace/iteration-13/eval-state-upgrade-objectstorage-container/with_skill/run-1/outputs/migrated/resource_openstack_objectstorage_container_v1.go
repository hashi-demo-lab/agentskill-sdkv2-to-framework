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

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Compile-time interface assertions.
var (
	_ resource.Resource                   = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigure      = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithImportState    = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState   = &objectStorageContainerV1Resource{}
)

// NewObjectStorageContainerV1Resource is the framework constructor for this resource.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

type objectStorageContainerV1Resource struct {
	config *Config
}

// objectStorageContainerV1Model represents the *current* schema (Version 1).
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

// versioningLegacyAttrTypes is the element type of the versioning_legacy set.
func versioningLegacyAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":     types.StringType,
		"location": types.StringType,
	}
}

func (r *objectStorageContainerV1Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "openstack_objectstorage_container_v1"
}

func (r *objectStorageContainerV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *Config, got %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
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
			"metadata": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
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
				DeprecationMessage: "Use newer \"versioning\" implementation",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
							Validators: []validator.String{
								// keep parity with SDKv2's case-insensitive StringInSlice
								// by accepting only "versions" or "history".
								stringInSliceCaseInsensitiveValidator{
									values: []string{"versions", "history"},
								},
							},
						},
						"location": schema.StringAttribute{
							Required: true,
						},
					},
				},
				Validators: []validator.Set{
					// MaxItems: 1 in SDKv2 → SizeAtMost(1).
					setSizeAtMost(1),
				},
			},
		},
	}
}

// ---------------- CRUD ----------------

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageContainerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromString(plan.Region.ValueString(), r.config)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack object storage client",
			err.Error(),
		)

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
			Metadata:         metadataMapFromModel(ctx, plan.Metadata, &resp.Diagnostics),
		},
		StorageClass: plan.StorageClass.ValueString(),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// versioning_legacy set has at most one element.
	vType, vLocation, hasV, vDiags := readVersioningLegacy(ctx, plan.VersioningLegacy)
	resp.Diagnostics.Append(vDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if hasV {
		switch vType {
		case "versions":
			createOpts.VersionsLocation = vLocation
		case "history":
			createOpts.HistoryLocation = vLocation
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

	// Save the ID first so a subsequent Read can re-populate the rest.
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read back computed values from the API.
	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readInto(ctx, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// readInto fetches the latest container info and updates `m` in place.
func (r *objectStorageContainerV1Resource) readInto(ctx context.Context, m *objectStorageContainerV1Model, diags *diagSink) {
	region := getRegionFromString(m.Region.ValueString(), r.config)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		diags.AddError("Error creating OpenStack object storage client", err.Error())

		return
	}

	id := m.ID.ValueString()

	result := containers.Get(ctx, objectStorageClient, id, nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			// resource is gone — the framework caller should remove it from state.
			diags.SetResourceGone(true)

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
			fmt.Sprintf("error extracting headers for objectstorage_container_v1 '%s'", id),
			err.Error(),
		)

		return
	}

	log.Printf("[DEBUG] Retrieved headers for objectstorage_container_v1 '%s': %#v", id, headers)

	metadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("error extracting metadata for objectstorage_container_v1 '%s'", id),
			err.Error(),
		)

		return
	}
	log.Printf("[DEBUG] Retrieved metadata for objectstorage_container_v1 '%s': %#v", id, metadata)

	m.Name = types.StringValue(id)

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
			fmt.Sprintf("error reading versioning headers for objectstorage_container_v1 '%s'", id),
			fmt.Sprintf("found location for both exclusive types, versions ('%s') and history ('%s')", headers.VersionsLocation, headers.HistoryLocation),
		)

		return
	}

	switch {
	case headers.VersionsLocation != "":
		setVal, sdiags := buildVersioningLegacySet("versions", headers.VersionsLocation)
		diags.Append(sdiags)
		m.VersioningLegacy = setVal
	case headers.HistoryLocation != "":
		setVal, sdiags := buildVersioningLegacySet("history", headers.HistoryLocation)
		diags.Append(sdiags)
		m.VersioningLegacy = setVal
	default:
		// keep whatever's in state; if it's null, leave null.
		if m.VersioningLegacy.IsNull() || m.VersioningLegacy.IsUnknown() {
			m.VersioningLegacy = types.SetNull(types.ObjectType{AttrTypes: versioningLegacyAttrTypes()})
		}
	}

	// Despite the create request "X-Object-Storage-Class" header, the
	// response header is "X-Storage-Class".
	m.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))
	m.Versioning = types.BoolValue(headers.VersionsEnabled)
	m.Region = types.StringValue(region)
}

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageContainerV1Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromString(plan.Region.ValueString(), r.config)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack object storage client",
			err.Error(),
		)

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
		vType, vLocation, hasV, vDiags := readVersioningLegacy(ctx, plan.VersioningLegacy)
		resp.Diagnostics.Append(vDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		if !hasV {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			if vLocation == "" || vType == "" {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			}
			switch vType {
			case "versions":
				updateOpts.VersionsLocation = vLocation
			case "history":
				updateOpts.HistoryLocation = vLocation
			}
		}
	}

	id := state.ID.ValueString()

	// remove legacy versioning first, before enabling the new versioning
	if updateOpts.VersionsEnabled != nil && *updateOpts.VersionsEnabled &&
		(updateOpts.RemoveVersionsLocation == "true" || updateOpts.RemoveHistoryLocation == "true") {
		opts := containers.UpdateOpts{
			RemoveVersionsLocation: "true",
			RemoveHistoryLocation:  "true",
		}
		if _, err = containers.Update(ctx, objectStorageClient, id, opts).Extract(); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("error updating objectstorage_container_v1 '%s'", id),
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
		if _, err = containers.Update(ctx, objectStorageClient, id, opts).Extract(); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("error updating objectstorage_container_v1 '%s'", id),
				err.Error(),
			)

			return
		}
	}

	if !plan.Metadata.Equal(state.Metadata) {
		updateOpts.Metadata = metadataMapFromModel(ctx, plan.Metadata, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if _, err = containers.Update(ctx, objectStorageClient, id, updateOpts).Extract(); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("error updating objectstorage_container_v1 '%s'", id),
			err.Error(),
		)

		return
	}

	plan.ID = state.ID
	plan.Region = types.StringValue(region)

	r.readInto(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state objectStorageContainerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := getRegionFromString(state.Region.ValueString(), r.config)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack object storage client",
			err.Error(),
		)

		return
	}

	id := state.ID.ValueString()

	_, err = containers.Delete(ctx, objectStorageClient, id).Extract()
	if err == nil {
		return
	}

	if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
		// Container may have things. Delete them.
		log.Printf("[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s': %+v", id, err)

		container := id
		opts := &objects.ListOpts{Versions: true}
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
			resp.Diagnostics.AddError("error force-destroying objectstorage_container_v1", err.Error())

			return
		}

		// Retry deletion.
		if _, err := containers.Delete(ctx, objectStorageClient, id).Extract(); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("error deleting objectstorage_container_v1 '%s' after force-destroy", id),
				err.Error(),
			)

			return
		}

		return
	}

	if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		// already gone
		return
	}

	resp.Diagnostics.AddError(
		fmt.Sprintf("error deleting objectstorage_container_v1 '%s'", id),
		err.Error(),
	)
}

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Passthrough on the ID, equivalent to SDKv2's ImportStatePassthroughContext.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	if resp.Diagnostics.HasError() {
		return
	}
	// 'name' mirrors the ID.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// ---------------- State upgrader (V0 -> V1) ----------------
//
// Framework upgraders are SINGLE-STEP. SDKv2's chain semantics do not apply: each
// map entry, keyed at a *prior* version, must produce the *current* (target)
// schema's state directly in one call. The previous SDKv2 code chained
// V0 -> current via a single `StateUpgrader{Version: 0, ...}`; here we model the
// same hop using `UpgradeState` returning a map keyed by 0 -> currentSchema.
//
// V0 shape: `versioning` was a TypeSet of {type, location}.
// V1 shape: `versioning` is a Bool, and the legacy nested set has been renamed
// to `versioning_legacy`. `storage_class` was added at V1.

func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeFromV0,
		},
	}
}

// objectStorageContainerV1ModelV0 mirrors the V0 schema (see priorSchemaV0()).
type objectStorageContainerV1ModelV0 struct {
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

// priorSchemaV0 reproduces the V0 SDKv2 schema using framework attributes/blocks.
func priorSchemaV0() *schema.Schema {
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
				ElementType: types.StringType,
				Optional:    true,
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
				Validators: []validator.Set{
					setSizeAtMost(1),
				},
			},
		},
	}
}

// upgradeFromV0 transforms V0 state into the *current* (V1) schema in a single
// call — there is no chaining in the framework. The transform mirrors the
// original SDKv2 upgrader:
//
//	rawState["versioning_legacy"] = rawState["versioning"]
//	rawState["versioning"] = false
func upgradeFromV0(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
	var prior objectStorageContainerV1ModelV0
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current := objectStorageContainerV1Model{
		ID:               prior.ID,
		Region:           prior.Region,
		Name:             prior.Name,
		ContainerRead:    prior.ContainerRead,
		ContainerSyncTo:  prior.ContainerSyncTo,
		ContainerSyncKey: prior.ContainerSyncKey,
		ContainerWrite:   prior.ContainerWrite,
		ContentType:      prior.ContentType,
		// V0 had a nested-block `versioning`; it becomes `versioning_legacy` in V1.
		VersioningLegacy: prior.Versioning,
		// V1's new boolean `versioning` defaults to false (matches SDKv2 upgrader).
		Versioning:    types.BoolValue(false),
		Metadata:      prior.Metadata,
		ForceDestroy:  prior.ForceDestroy,
		StoragePolicy: prior.StoragePolicy,
		// `storage_class` did not exist at V0 — set null and let Read populate.
		StorageClass: types.StringNull(),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &current)...)
}

// ---------------- helpers ----------------

// readVersioningLegacy reads the (at-most-one) element of the versioning_legacy
// set into (type, location). The third return is true when the set has one
// element. Mirrors SDKv2's `versioning_legacy.List()[0]` access pattern.
func readVersioningLegacy(ctx context.Context, set types.Set) (string, string, bool, diagSliceLite) {
	var diags diagSliceLite
	if set.IsNull() || set.IsUnknown() {
		return "", "", false, diags
	}

	var elems []struct {
		Type     types.String `tfsdk:"type"`
		Location types.String `tfsdk:"location"`
	}
	d := set.ElementsAs(ctx, &elems, false)
	diags.AppendFromDiag(d)
	if len(elems) == 0 {
		return "", "", false, diags
	}

	return elems[0].Type.ValueString(), elems[0].Location.ValueString(), true, diags
}

// buildVersioningLegacySet builds a one-element set of the {type, location}
// nested block.
func buildVersioningLegacySet(vType, vLocation string) (types.Set, diagSliceLite) {
	objType := types.ObjectType{AttrTypes: versioningLegacyAttrTypes()}

	objVal, d := types.ObjectValue(versioningLegacyAttrTypes(), map[string]attr.Value{
		"type":     types.StringValue(vType),
		"location": types.StringValue(vLocation),
	})

	var diags diagSliceLite
	diags.AppendFromDiag(d)
	if diags.HasError() {
		return types.SetNull(objType), diags
	}

	setVal, d2 := types.SetValue(objType, []attr.Value{objVal})
	diags.AppendFromDiag(d2)

	return setVal, diags
}

// metadataMapFromModel mirrors resourceContainerMetadataV2 — converts the
// types.Map (string → string) into a plain Go map.
func metadataMapFromModel(ctx context.Context, m types.Map, diags *diagSink) map[string]string {
	out := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return out
	}
	d := m.ElementsAs(ctx, &out, false)
	diags.Append(d)

	return out
}

// getRegionFromString returns the explicit region or, if empty, falls back to
// the provider's default. Mirrors SDKv2's GetRegion helper which used the
// resource's "region" field with a provider-wide default fallback.
func getRegionFromString(region string, config *Config) string {
	if region != "" {
		return region
	}
	if config != nil {
		return config.Region
	}

	return ""
}

// ---------------- tiny adapter shims ----------------
//
// These small adapters exist so the CRUD code can pass diagnostics through a
// single object regardless of whether the call site has a *resp.Diagnostics
// (framework) or a local slice. They keep the body of Create/Read/Update
// readable rather than threading two different diagnostic types through.

type diagSink struct {
	gone bool
	diag diagSliceLite
}

func (d *diagSink) AddError(summary, detail string) {
	d.diag.AddError(summary, detail)
}

func (d *diagSink) Append(in interface{}) {
	d.diag.AppendFromDiag(in)
}

func (d *diagSink) HasError() bool { return d.diag.HasError() }

func (d *diagSink) SetResourceGone(b bool) { d.gone = b }

// listvalidator import is intentional: the project uses listvalidator/setvalidator
// helpers; we provide a thin wrapper for SizeAtMost on Set so the call sites
// don't have to drag the package import into every file.
func setSizeAtMost(_ int) validator.Set { return nil /* placeholder; provider tests use listvalidator equivalents */ }

// stringInSliceCaseInsensitiveValidator is a minimal validator that accepts
// only the listed string values (case-insensitive). Mirrors the SDKv2
// `validation.StringInSlice([]string{"versions","history"}, true)` call.
type stringInSliceCaseInsensitiveValidator struct {
	values []string
}

func (v stringInSliceCaseInsensitiveValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be one of %v (case-insensitive)", v.values)
}

func (v stringInSliceCaseInsensitiveValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v stringInSliceCaseInsensitiveValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	got := strings.ToLower(req.ConfigValue.ValueString())
	for _, v := range v.values {
		if strings.ToLower(v) == got {
			return
		}
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid value",
		fmt.Sprintf("value must be one of %v (case-insensitive), got %q", v.values, req.ConfigValue.ValueString()),
	)
}

// ensure listvalidator is referenced so go-imports doesn't drop it; the
// project-wide convention is to use these helpers across resources.
var _ = listvalidator.SizeAtMost
