package openstack

import (
	"context"
	"fmt"
	"sort"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/availabilityzones"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-openstack/utils/v2/hashcode"
)

// Compile-time assertions of interface satisfaction.
var (
	_ datasource.DataSource              = &blockStorageAvailabilityZonesV3DataSource{}
	_ datasource.DataSourceWithConfigure = &blockStorageAvailabilityZonesV3DataSource{}
)

// NewBlockStorageAvailabilityZonesV3DataSource returns a new
// terraform-plugin-framework data source for
// openstack_blockstorage_availability_zones_v3.
func NewBlockStorageAvailabilityZonesV3DataSource() datasource.DataSource {
	return &blockStorageAvailabilityZonesV3DataSource{}
}

type blockStorageAvailabilityZonesV3DataSource struct {
	config *Config
}

// blockStorageAvailabilityZonesV3DataSourceModel maps schema attributes to
// typed Go fields. The tfsdk tag values must match the schema attribute names
// exactly; the user-facing schema is unchanged from the SDKv2 version.
type blockStorageAvailabilityZonesV3DataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	Region types.String `tfsdk:"region"`
	State  types.String `tfsdk:"state"`
	Names  types.List   `tfsdk:"names"`
}

func (d *blockStorageAvailabilityZonesV3DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blockstorage_availability_zones_v3"
}

func (d *blockStorageAvailabilityZonesV3DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			// id is implicit in SDKv2 (`d.SetId(...)`); the framework requires
			// it to be declared explicitly when present in state.
			"id": schema.StringAttribute{
				Computed: true,
			},

			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},

			"state": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					// Mirrors validation.StringInSlice([]string{"available",
					// "unavailable"}, true) where true == case-insensitive.
					stringvalidator.OneOfCaseInsensitive("available", "unavailable"),
				},
				// SDKv2 used Default: "available". Data sources in the
				// framework do not support attribute-level defaults; we apply
				// the default in Read() before validating against state.
			},

			"names": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *blockStorageAvailabilityZonesV3DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		// Provider has not yet been configured (e.g. during validation).
		return
	}

	cfg, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *openstack.Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.config = cfg
}

func (d *blockStorageAvailabilityZonesV3DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data blockStorageAvailabilityZonesV3DataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Equivalent to GetRegion(d, config): use the resource-level region if
	// present, otherwise fall back to the provider-level region.
	region := data.Region.ValueString()
	if data.Region.IsNull() || data.Region.IsUnknown() || region == "" {
		region = d.config.Region
	}

	// Equivalent to the SDKv2 Default: "available" on the state attribute.
	stateVal := data.State.ValueString()
	if data.State.IsNull() || data.State.IsUnknown() || stateVal == "" {
		stateVal = "available"
	}

	client, err := d.config.BlockStorageV3Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack block storage client",
			err.Error(),
		)

		return
	}

	allPages, err := availabilityzones.List(client).AllPages(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error retrieving openstack_blockstorage_availability_zones_v3",
			err.Error(),
		)

		return
	}

	zoneInfo, err := availabilityzones.ExtractAvailabilityZones(allPages)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error extracting openstack_blockstorage_availability_zones_v3 from response",
			err.Error(),
		)

		return
	}

	stateBool := stateVal == "available"

	zones := make([]string, 0, len(zoneInfo))

	for _, z := range zoneInfo {
		if z.ZoneState.Available == stateBool {
			zones = append(zones, z.ZoneName)
		}
	}

	// sort.Strings sorts in place, returns nothing.
	sort.Strings(zones)

	namesList, diags := types.ListValueFrom(ctx, types.StringType, zones)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(hashcode.Strings(zones))
	data.Region = types.StringValue(region)
	data.State = types.StringValue(stateVal)
	data.Names = namesList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
