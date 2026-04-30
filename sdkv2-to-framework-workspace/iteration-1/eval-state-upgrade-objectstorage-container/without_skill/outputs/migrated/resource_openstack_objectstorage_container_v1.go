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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure interface compliance at compile time.
var (
	_ resource.Resource                = (*objectStorageContainerV1Resource)(nil)
	_ resource.ResourceWithImportState = (*objectStorageContainerV1Resource)(nil)
	_ resource.ResourceWithUpgradeState = (*objectStorageContainerV1Resource)(nil)
)

// objectStorageContainerV1Resource is the framework resource implementation.
type objectStorageContainerV1Resource struct {
	config *Config
}

// NewObjectStorageContainerV1Resource is the factory used by the provider.
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
// Configure (receives the provider meta)
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)
		return
	}
	r.config = config
}

// ---------------------------------------------------------------------------
// State model
// ---------------------------------------------------------------------------

// versioningLegacyModel represents one entry of the versioning_legacy block.
type versioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// containerV1Model is the Go struct that mirrors the Terraform state.
type containerV1Model struct {
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

// versioningLegacyAttrTypes returns the attribute type map for versioning_legacy elements.
func versioningLegacyAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"type":     types.StringType,
		"location": types.StringType,
	}
}

// ---------------------------------------------------------------------------
// Schema (current version = 1)
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// SchemaVersion tracks the current state schema version. The framework
		// uses this to decide which UpgradeState handler to invoke when it finds
		// older persisted state.
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

			// versioning enables Swift's built-in object versioning (X-Versions-Enabled).
			// ConflictsWith versioning_legacy is enforced at apply time in Create/Update
			// rather than by a plan-time validator, because versioning_legacy is a block.
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

		// versioning_legacy is a MaxItems:1 list block, preserved as
		// ListNestedBlock + SizeAtMost(1) so that existing HCL block syntax
		// (without index brackets) continues to work unchanged.
		// Deprecated in favour of the "versioning" bool attribute.
		Blocks: map[string]schema.Block{
			"versioning_legacy": schema.ListNestedBlock{
				DeprecationMessage: `Use newer "versioning" implementation`,
				Validators: []validator.List{
					listvalidator.SizeAtMost(1),
				},
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
// UpgradeState — V0 → V1
// ---------------------------------------------------------------------------
//
// In SDK v2 the state upgrader was registered via StateUpgraders. In the
// framework it is declared through ResourceWithUpgradeState. There is a
// single handler (version 0 → current version 1).
//
// V0 state shape:
//
//	{
//	  "versioning": [{"type": "versions", "location": "..."}]   // TypeSet
//	  ...
//	}
//
// V1 state shape:
//
//	{
//	  "versioning":        false                                 // TypeBool
//	  "versioning_legacy": [{"type": "versions", "location": "..."}]
//	  ...
//	}

func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// Upgrade from schema version 0 to current version 1.
		0: {
			// PriorSchema describes the old (V0) schema so the framework can
			// deserialise the raw state bytes into a typed value.
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id":                schema.StringAttribute{Computed: true},
					"region":            schema.StringAttribute{Optional: true, Computed: true},
					"name":              schema.StringAttribute{Required: true},
					"container_read":    schema.StringAttribute{Optional: true},
					"container_sync_to": schema.StringAttribute{Optional: true},
					"container_sync_key": schema.StringAttribute{Optional: true},
					"container_write":   schema.StringAttribute{Optional: true},
					"content_type":      schema.StringAttribute{Optional: true},
					"metadata":          schema.MapAttribute{ElementType: types.StringType, Optional: true},
					"force_destroy":     schema.BoolAttribute{Optional: true, Computed: true},
					"storage_policy":    schema.StringAttribute{Optional: true, Computed: true},
				},
				Blocks: map[string]schema.Block{
					// In V0 "versioning" was a TypeSet block (renamed in V1 to
					// "versioning_legacy"). There was no separate bool "versioning"
					// attribute.
					"versioning": schema.ListNestedBlock{
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"type":     schema.StringAttribute{Required: true},
								"location": schema.StringAttribute{Required: true},
							},
						},
					},
				},
			},

			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				// v0StateModel mirrors the PriorSchema above.
				type versioningV0Model struct {
					Type     types.String `tfsdk:"type"`
					Location types.String `tfsdk:"location"`
				}
				type containerV0Model struct {
					ID               types.String        `tfsdk:"id"`
					Region           types.String        `tfsdk:"region"`
					Name             types.String        `tfsdk:"name"`
					ContainerRead    types.String        `tfsdk:"container_read"`
					ContainerSyncTo  types.String        `tfsdk:"container_sync_to"`
					ContainerSyncKey types.String        `tfsdk:"container_sync_key"`
					ContainerWrite   types.String        `tfsdk:"container_write"`
					ContentType      types.String        `tfsdk:"content_type"`
					Versioning       []versioningV0Model `tfsdk:"versioning"`
					Metadata         types.Map           `tfsdk:"metadata"`
					ForceDestroy     types.Bool          `tfsdk:"force_destroy"`
					StoragePolicy    types.String        `tfsdk:"storage_policy"`
				}

				var priorState containerV0Model
				resp.Diagnostics.Append(req.State.Get(ctx, &priorState)...)
				if resp.Diagnostics.HasError() {
					return
				}

				// Translate versioning (old block) → versioning_legacy (new block).
				var versioningLegacyList types.List
				if len(priorState.Versioning) == 0 {
					versioningLegacyList = types.ListValueMust(
						types.ObjectType{AttrTypes: versioningLegacyAttrTypes()},
						[]attr.Value{},
					)
				} else {
					elems := make([]attr.Value, 0, len(priorState.Versioning))
					for _, v := range priorState.Versioning {
						obj, d := types.ObjectValue(versioningLegacyAttrTypes(), map[string]attr.Value{
							"type":     v.Type,
							"location": v.Location,
						})
						resp.Diagnostics.Append(d...)
						elems = append(elems, obj)
					}
					if resp.Diagnostics.HasError() {
						return
					}
					versioningLegacyList = types.ListValueMust(
						types.ObjectType{AttrTypes: versioningLegacyAttrTypes()},
						elems,
					)
				}

				upgradedState := containerV1Model{
					ID:               priorState.ID,
					Region:           priorState.Region,
					Name:             priorState.Name,
					ContainerRead:    priorState.ContainerRead,
					ContainerSyncTo:  priorState.ContainerSyncTo,
					ContainerSyncKey: priorState.ContainerSyncKey,
					ContainerWrite:   priorState.ContainerWrite,
					ContentType:      priorState.ContentType,
					// "versioning" bool defaults to false (was a block in V0)
					Versioning:       types.BoolValue(false),
					VersioningLegacy: versioningLegacyList,
					Metadata:         priorState.Metadata,
					ForceDestroy:     priorState.ForceDestroy,
					StoragePolicy:    priorState.StoragePolicy,
					// storage_class was added in V1 — default to unknown so a
					// subsequent refresh populates it.
					StorageClass: types.StringUnknown(),
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, upgradedState)...)
			},
		},
	}
}

// ---------------------------------------------------------------------------
// ImportState
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The container name is used as the ID.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan containerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := plan.Region.ValueString()
	if region == "" {
		region = r.config.GetRegion(ctx, nil)
	}

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
			Metadata:         containerMetadataFromPlan(ctx, plan, &resp.Diagnostics),
		},
		StorageClass: plan.StorageClass.ValueString(),
	}
	if resp.Diagnostics.HasError() {
		return
	}

	vl := containerVersioningLegacyFromPlan(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if vl != nil {
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
		resp.Diagnostics.AddError("Error creating objectstorage_container_v1", err.Error())
		return
	}

	log.Printf("[INFO] objectstorage_container_v1 created with ID: %s", cn)

	plan.ID = types.StringValue(cn)
	plan.Region = types.StringValue(region)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.readIntoState(ctx, cn, region, &resp.State, &resp.Diagnostics)
}

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state containerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.GetRegion(ctx, nil)
	}

	r.readIntoState(ctx, state.ID.ValueString(), region, &resp.State, &resp.Diagnostics)
}

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state containerV1Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.GetRegion(ctx, nil)
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

	// versioning (bool)
	if !plan.Versioning.Equal(state.Versioning) {
		v := plan.Versioning.ValueBool()
		updateOpts.VersionsEnabled = &v
	}

	// versioning_legacy (block)
	if !plan.VersioningLegacy.Equal(state.VersioningLegacy) {
		vl := containerVersioningLegacyFromPlan(ctx, plan, &resp.Diagnostics)
		if resp.Diagnostics.HasError() {
			return
		}
		if vl == nil {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			if vl.Location.ValueString() == "" || vl.Type.ValueString() == "" {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			}
			switch vl.Type.ValueString() {
			case "versions":
				updateOpts.VersionsLocation = vl.Location.ValueString()
			case "history":
				updateOpts.HistoryLocation = vl.Location.ValueString()
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

	if !plan.Metadata.Equal(state.Metadata) {
		updateOpts.Metadata = containerMetadataFromPlan(ctx, plan, &resp.Diagnostics)
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

	r.readIntoState(ctx, state.ID.ValueString(), region, &resp.State, &resp.Diagnostics)
}

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state containerV1Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := state.Region.ValueString()
	if region == "" {
		region = r.config.GetRegion(ctx, nil)
	}

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	_, err = containers.Delete(ctx, objectStorageClient, state.ID.ValueString()).Extract()
	if err != nil {
		if gophercloud.ResponseCodeIs(err, http.StatusConflict) && state.ForceDestroy.ValueBool() {
			log.Printf("[DEBUG] Attempting to forceDestroy objectstorage_container_v1 '%s'", state.ID.ValueString())

			opts := &objects.ListOpts{Versions: true}
			pager := objects.List(objectStorageClient, state.ID.ValueString(), opts)
			err = pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
				objectList, err := objects.ExtractInfo(page)
				if err != nil {
					return false, fmt.Errorf("error extracting names from objects for objectstorage_container_v1 '%s': %w", state.ID.ValueString(), err)
				}
				for _, object := range objectList {
					delOpts := objects.DeleteOpts{ObjectVersionID: object.VersionID}
					_, err = objects.Delete(ctx, objectStorageClient, state.ID.ValueString(), object.Name, delOpts).Extract()
					if err != nil {
						latest := "latest"
						if !object.IsLatest && object.VersionID != "" {
							latest = object.VersionID
						}
						return false, fmt.Errorf("error deleting object '%s@%s' from objectstorage_container_v1 '%s': %w", object.Name, latest, state.ID.ValueString(), err)
					}
				}
				return true, nil
			})
			if err != nil {
				resp.Diagnostics.AddError("Error force-destroying objectstorage_container_v1", err.Error())
				return
			}

			// Retry delete now that objects have been removed.
			r.Delete(ctx, req, resp)
			return
		}

		if checkDeletedDiag(state.ID.ValueString(), err, "container") {
			// Already gone — remove from state.
			return
		}
		resp.Diagnostics.AddError(
			fmt.Sprintf("Error deleting objectstorage_container_v1 '%s'", state.ID.ValueString()),
			err.Error(),
		)
	}
}

// ---------------------------------------------------------------------------
// Shared read helper
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) readIntoState(
	ctx context.Context,
	id, region string,
	state *tfsdk.State,
	diags *diag.Diagnostics,
) {
	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		diags.AddError("Error creating OpenStack object storage client", err.Error())
		return
	}

	result := containers.Get(ctx, objectStorageClient, id, nil)
	if result.Err != nil {
		if checkDeletedDiag(id, result.Err, "container") {
			state.RemoveResource(ctx)
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

	// Build the new state model.
	var m containerV1Model

	m.ID = types.StringValue(id)
	m.Name = types.StringValue(id)
	m.Region = types.StringValue(region)

	if len(headers.Read) > 0 && headers.Read[0] != "" {
		m.ContainerRead = types.StringValue(strings.Join(headers.Read, ","))
	} else {
		m.ContainerRead = types.StringValue("")
	}

	if len(headers.Write) > 0 && headers.Write[0] != "" {
		m.ContainerWrite = types.StringValue(strings.Join(headers.Write, ","))
	} else {
		m.ContainerWrite = types.StringValue("")
	}

	if len(headers.StoragePolicy) > 0 {
		m.StoragePolicy = types.StringValue(headers.StoragePolicy)
	} else {
		m.StoragePolicy = types.StringValue("")
	}

	// storage_class is returned under a different header name on read.
	m.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))

	m.Versioning = types.BoolValue(headers.VersionsEnabled)

	// versioning_legacy
	if headers.VersionsLocation != "" && headers.HistoryLocation != "" {
		diags.AddError(
			fmt.Sprintf("Error reading versioning headers for objectstorage_container_v1 '%s'", id),
			fmt.Sprintf("found location for both exclusive types, versions ('%s') and history ('%s')", headers.VersionsLocation, headers.HistoryLocation),
		)
		return
	}

	objType := types.ObjectType{AttrTypes: versioningLegacyAttrTypes()}
	switch {
	case headers.VersionsLocation != "":
		obj, d := types.ObjectValue(versioningLegacyAttrTypes(), map[string]attr.Value{
			"type":     types.StringValue("versions"),
			"location": types.StringValue(headers.VersionsLocation),
		})
		diags.Append(d...)
		m.VersioningLegacy = types.ListValueMust(objType, []attr.Value{obj})
	case headers.HistoryLocation != "":
		obj, d := types.ObjectValue(versioningLegacyAttrTypes(), map[string]attr.Value{
			"type":     types.StringValue("history"),
			"location": types.StringValue(headers.HistoryLocation),
		})
		diags.Append(d...)
		m.VersioningLegacy = types.ListValueMust(objType, []attr.Value{obj})
	default:
		m.VersioningLegacy = types.ListValueMust(objType, []attr.Value{})
	}
	if diags.HasError() {
		return
	}

	// Metadata — convert map[string]string to types.Map.
	metaElems := make(map[string]attr.Value, len(metadata))
	for k, v := range metadata {
		metaElems[k] = types.StringValue(v)
	}
	metaMap, d := types.MapValue(types.StringType, metaElems)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	m.Metadata = metaMap

	// force_destroy is write-only (not returned by the API); keep state value.
	// We read the current state to preserve it.
	var currentState containerV1Model
	diags.Append(state.Get(ctx, &currentState)...)
	if !diags.HasError() {
		m.ForceDestroy = currentState.ForceDestroy
		m.ContainerSyncTo = currentState.ContainerSyncTo
		m.ContainerSyncKey = currentState.ContainerSyncKey
		m.ContentType = currentState.ContentType
	}
	// Reset diagnostics from the state.Get above if the state was empty (import).
	// We still continue; fields will be zero values which is acceptable.

	diags.Append(state.Set(ctx, &m)...)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// containerMetadataFromPlan converts the plan's metadata map to map[string]string.
func containerMetadataFromPlan(ctx context.Context, plan containerV1Model, diags *diag.Diagnostics) map[string]string {
	if plan.Metadata.IsNull() || plan.Metadata.IsUnknown() {
		return nil
	}
	m := make(map[string]string, len(plan.Metadata.Elements()))
	for k, v := range plan.Metadata.Elements() {
		sv, ok := v.(types.String)
		if !ok {
			diags.AddError("Unexpected metadata value type", fmt.Sprintf("key %q: %T", k, v))
			return nil
		}
		m[k] = sv.ValueString()
	}
	return m
}

// containerVersioningLegacyFromPlan returns the first (and only) versioning_legacy
// entry from the plan, or nil if the list is empty.
func containerVersioningLegacyFromPlan(ctx context.Context, plan containerV1Model, diags *diag.Diagnostics) *versioningLegacyModel {
	if plan.VersioningLegacy.IsNull() || plan.VersioningLegacy.IsUnknown() {
		return nil
	}
	elems := plan.VersioningLegacy.Elements()
	if len(elems) == 0 {
		return nil
	}
	var items []versioningLegacyModel
	diags.Append(plan.VersioningLegacy.ElementsAs(ctx, &items, false)...)
	if diags.HasError() || len(items) == 0 {
		return nil
	}
	return &items[0]
}

// checkDeletedDiag returns true if the error indicates the resource is gone,
// allowing the caller to remove it from state silently.
func checkDeletedDiag(id string, err error, resourceType string) bool {
	if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
		log.Printf("[DEBUG] %s '%s' not found, removing from state", resourceType, id)
		return true
	}
	return false
}
