package openstack

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/objects"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
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
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Compile-time interface assertions; missing methods become compile errors.
var (
	_ resource.Resource                 = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithConfigure    = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithImportState  = &objectStorageContainerV1Resource{}
	_ resource.ResourceWithUpgradeState = &objectStorageContainerV1Resource{}
)

// NewObjectStorageContainerV1Resource is the framework constructor for the
// openstack_objectstorage_container_v1 resource.
func NewObjectStorageContainerV1Resource() resource.Resource {
	return &objectStorageContainerV1Resource{}
}

type objectStorageContainerV1Resource struct {
	config *Config
}

// objectStorageContainerV1Model represents the *current* (V1) schema shape.
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

// versioningLegacyModel is the nested-block element type for both the current
// schema and the V0 prior schema (V0 named the block `versioning`).
type versioningLegacyModel struct {
	Type     types.String `tfsdk:"type"`
	Location types.String `tfsdk:"location"`
}

// versioningLegacyObjectType returns the attr.Type for one nested element of
// the versioning_legacy set. Used by Read (constructing types.Set values) and
// the V0 upgrader (allocating typed nulls).
func versioningLegacyObjectType() attr.Type {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type":     types.StringType,
			"location": types.StringType,
		},
	}
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
		// versioning_legacy stays as a block to preserve practitioner HCL —
		// production configs use `versioning_legacy { type = ..., location = ... }`
		// (see acceptance tests). Block decision per references/blocks.md.
		// `versioning` (bool) and `versioning_legacy` (block) are mutually
		// exclusive — emulated via setvalidator.ConflictsWith.
		Blocks: map[string]schema.Block{
			"versioning_legacy": schema.SetNestedBlock{
				DeprecationMessage: "Use newer \"versioning\" implementation",
				Validators: []validator.Set{
					setvalidator.SizeAtMost(1),
					setvalidator.ConflictsWith(path.MatchRoot("versioning")),
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

func (r *objectStorageContainerV1Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data",
			fmt.Sprintf("Expected *Config, got: %T", req.ProviderData),
		)

		return
	}

	r.config = cfg
}

func (r *objectStorageContainerV1Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The container's `name` is the resource ID (Create sets d.SetId(name)).
	// Set both `id` and `name` from the import string so Read can find it.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
}

// regionFromModel returns the per-resource region, falling back to the provider
// region (mirrors the SDKv2 GetRegion helper).
func (r *objectStorageContainerV1Resource) regionFromModel(m *objectStorageContainerV1Model) string {
	if !m.Region.IsNull() && !m.Region.IsUnknown() && m.Region.ValueString() != "" {
		return m.Region.ValueString()
	}

	return r.config.Region
}

// metadataFromModel converts the typed map into a plain map[string]string for
// the gophercloud client.
func metadataFromModel(ctx context.Context, m *objectStorageContainerV1Model) (map[string]string, diag.Diagnostics) {
	if m.Metadata.IsNull() || m.Metadata.IsUnknown() {
		return map[string]string{}, nil
	}

	out := make(map[string]string, len(m.Metadata.Elements()))
	diags := m.Metadata.ElementsAs(ctx, &out, false)

	return out, diags
}

// extractVersioningLegacy reads a single versioning_legacy block (if any) into
// a typed value.
func extractVersioningLegacy(ctx context.Context, set types.Set) (*versioningLegacyModel, diag.Diagnostics) {
	if set.IsNull() || set.IsUnknown() || len(set.Elements()) == 0 {
		return nil, nil
	}

	var elems []versioningLegacyModel

	diags := set.ElementsAs(ctx, &elems, false)
	if diags.HasError() {
		return nil, diags
	}

	if len(elems) == 0 {
		return nil, nil
	}

	return &elems[0], nil
}

func (r *objectStorageContainerV1Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan objectStorageContainerV1Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromModel(&plan)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("error creating OpenStack object storage client", err.Error())

		return
	}

	cn := plan.Name.ValueString()

	metadata, mdDiags := metadataFromModel(ctx, &plan)

	resp.Diagnostics.Append(mdDiags...)
	if resp.Diagnostics.HasError() {
		return
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

	versioningLegacy, vDiags := extractVersioningLegacy(ctx, plan.VersioningLegacy)

	resp.Diagnostics.Append(vDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if versioningLegacy != nil {
		switch strings.ToLower(versioningLegacy.Type.ValueString()) {
		case "versions":
			createOpts.VersionsLocation = versioningLegacy.Location.ValueString()
		case "history":
			createOpts.HistoryLocation = versioningLegacy.Location.ValueString()
		}
	}

	tflog.Debug(ctx, "Create options for objectstorage_container_v1", map[string]any{
		"opts": fmt.Sprintf("%#v", createOpts),
	})

	if _, err := containers.Create(ctx, objectStorageClient, cn, createOpts).Extract(); err != nil {
		resp.Diagnostics.AddError("error creating objectstorage_container_v1", err.Error())

		return
	}

	tflog.Info(ctx, "objectstorage_container_v1 created", map[string]any{"id": cn})

	plan.ID = types.StringValue(cn)
	plan.Region = types.StringValue(region)

	// Refresh from API so computed fields are populated.
	r.refreshAndSet(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *objectStorageContainerV1Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state objectStorageContainerV1Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.refreshAndSet(ctx, &state, &resp.Diagnostics, &resp.State)
}

// refreshAndSet calls the API to populate computed fields on `m` and writes
// the result via stateOps. Shared between Create, Read, and Update.
func (r *objectStorageContainerV1Resource) refreshAndSet(ctx context.Context, m *objectStorageContainerV1Model, diags *diag.Diagnostics, state *tfsdk.State) {
	region := r.regionFromModel(m)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		diags.AddError("error creating OpenStack object storage client", err.Error())

		return
	}

	id := m.ID.ValueString()

	result := containers.Get(ctx, objectStorageClient, id, nil)
	if result.Err != nil {
		if gophercloud.ResponseCodeIs(result.Err, http.StatusNotFound) {
			state.RemoveResource(ctx)

			return
		}

		diags.AddError(
			fmt.Sprintf("error reading objectstorage_container_v1 '%s'", id),
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

	tflog.Debug(ctx, "Retrieved headers for objectstorage_container_v1", map[string]any{
		"id":      id,
		"headers": fmt.Sprintf("%#v", headers),
	})

	apiMetadata, err := result.ExtractMetadata()
	if err != nil {
		diags.AddError(
			fmt.Sprintf("error extracting metadata for objectstorage_container_v1 '%s'", id),
			err.Error(),
		)

		return
	}

	tflog.Debug(ctx, "Retrieved metadata for objectstorage_container_v1", map[string]any{
		"id":       id,
		"metadata": fmt.Sprintf("%#v", apiMetadata),
	})

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
		setVal, sd := types.SetValueFrom(ctx, versioningLegacyObjectType(), []versioningLegacyModel{{
			Type:     types.StringValue("versions"),
			Location: types.StringValue(headers.VersionsLocation),
		}})

		diags.Append(sd...)

		if diags.HasError() {
			return
		}

		m.VersioningLegacy = setVal
	case headers.HistoryLocation != "":
		setVal, sd := types.SetValueFrom(ctx, versioningLegacyObjectType(), []versioningLegacyModel{{
			Type:     types.StringValue("history"),
			Location: types.StringValue(headers.HistoryLocation),
		}})

		diags.Append(sd...)

		if diags.HasError() {
			return
		}

		m.VersioningLegacy = setVal
	default:
		// Ensure the field has a typed null when the API reports no legacy versioning.
		if m.VersioningLegacy.IsUnknown() {
			m.VersioningLegacy = types.SetNull(versioningLegacyObjectType())
		}
	}

	// Despite the create request "X-Object-Storage-Class" header, the response
	// header is "X-Storage-Class".
	m.StorageClass = types.StringValue(result.Header.Get("X-Storage-Class"))
	m.Versioning = types.BoolValue(headers.VersionsEnabled)
	m.Region = types.StringValue(region)

	// Metadata: surface API-side metadata back into state when present;
	// otherwise leave the user's plan value (or null).
	if len(apiMetadata) > 0 {
		mdVal, mdDiags := types.MapValueFrom(ctx, types.StringType, apiMetadata)

		diags.Append(mdDiags...)

		if diags.HasError() {
			return
		}

		m.Metadata = mdVal
	} else if m.Metadata.IsUnknown() {
		m.Metadata = types.MapNull(types.StringType)
	}

	m.ID = types.StringValue(id)

	diags.Append(state.Set(ctx, m)...)
}

func (r *objectStorageContainerV1Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state objectStorageContainerV1Model

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromModel(&plan)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("error creating OpenStack object storage client", err.Error())

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
		legacy, vDiags := extractVersioningLegacy(ctx, plan.VersioningLegacy)

		resp.Diagnostics.Append(vDiags...)

		if resp.Diagnostics.HasError() {
			return
		}

		if legacy == nil {
			updateOpts.RemoveVersionsLocation = "true"
			updateOpts.RemoveHistoryLocation = "true"
		} else {
			loc := legacy.Location.ValueString()
			typ := legacy.Type.ValueString()

			if len(loc) == 0 || len(typ) == 0 {
				updateOpts.RemoveVersionsLocation = "true"
				updateOpts.RemoveHistoryLocation = "true"
			}

			switch strings.ToLower(typ) {
			case "versions":
				updateOpts.VersionsLocation = loc
			case "history":
				updateOpts.HistoryLocation = loc
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

		if _, err := containers.Update(ctx, objectStorageClient, id, opts).Extract(); err != nil {
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

		if _, err := containers.Update(ctx, objectStorageClient, id, opts).Extract(); err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("error updating objectstorage_container_v1 '%s'", id),
				err.Error(),
			)

			return
		}
	}

	if !plan.Metadata.Equal(state.Metadata) {
		md, mdDiags := metadataFromModel(ctx, &plan)

		resp.Diagnostics.Append(mdDiags...)

		if resp.Diagnostics.HasError() {
			return
		}

		updateOpts.Metadata = md
	}

	if _, err := containers.Update(ctx, objectStorageClient, id, updateOpts).Extract(); err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("error updating objectstorage_container_v1 '%s'", id),
			err.Error(),
		)

		return
	}

	plan.ID = types.StringValue(id)
	plan.Region = types.StringValue(region)

	r.refreshAndSet(ctx, &plan, &resp.Diagnostics, &resp.State)
}

func (r *objectStorageContainerV1Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state objectStorageContainerV1Model

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	region := r.regionFromModel(&state)

	objectStorageClient, err := r.config.ObjectStorageV1Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("error creating OpenStack object storage client", err.Error())

		return
	}

	id := state.ID.ValueString()
	forceDestroy := state.ForceDestroy.ValueBool()

	if err := r.deleteContainer(ctx, objectStorageClient, id, forceDestroy); err != nil {
		// 404 — already gone, no error.
		if gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
			return
		}

		resp.Diagnostics.AddError(
			fmt.Sprintf("error deleting objectstorage_container_v1 '%s'", id),
			err.Error(),
		)

		return
	}
}

// deleteContainer is the recursive delete helper, mirroring the SDKv2
// force-destroy retry loop.
func (r *objectStorageContainerV1Resource) deleteContainer(ctx context.Context, client *gophercloud.ServiceClient, container string, forceDestroy bool) error {
	_, err := containers.Delete(ctx, client, container).Extract()
	if err == nil {
		return nil
	}

	if !gophercloud.ResponseCodeIs(err, http.StatusConflict) || !forceDestroy {
		return err
	}

	tflog.Debug(ctx, "Attempting to forceDestroy objectstorage_container_v1", map[string]any{
		"id":  container,
		"err": err.Error(),
	})

	opts := &objects.ListOpts{Versions: true}
	pager := objects.List(client, container, opts)

	listErr := pager.EachPage(ctx, func(_ context.Context, page pagination.Page) (bool, error) {
		objectList, err := objects.ExtractInfo(page)
		if err != nil {
			return false, fmt.Errorf("error extracting names from objects from page for objectstorage_container_v1 '%s': %+w", container, err)
		}

		for _, object := range objectList {
			delOpts := objects.DeleteOpts{ObjectVersionID: object.VersionID}
			if _, err := objects.Delete(ctx, client, container, object.Name, delOpts).Extract(); err != nil {
				latest := "latest"
				if !object.IsLatest && object.VersionID != "" {
					latest = object.VersionID
				}

				return false, fmt.Errorf("error deleting object '%s@%s' from objectstorage_container_v1 '%s': %+w", object.Name, latest, container, err)
			}
		}

		return true, nil
	})
	if listErr != nil {
		return listErr
	}

	return r.deleteContainer(ctx, client, container, forceDestroy)
}

// ---------------------------------------------------------------------------
// State upgrade — single-step semantics (V0 → current).
//
// Per references/state-upgrade.md, the framework calls each entry of
// UpgradeState() *independently* with the matching PriorSchema; there is no
// chain. Because this resource only has one prior version (V0), there is
// exactly one entry: 0 → current. The V0 prior schema, model struct, and
// upgrader function are defined in
// migrate_resource_openstack_objectstorage_container_v1.go.
// ---------------------------------------------------------------------------

func (r *objectStorageContainerV1Resource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema:   priorSchemaV0(),
			StateUpgrader: upgradeStateFromV0,
		},
	}
}
