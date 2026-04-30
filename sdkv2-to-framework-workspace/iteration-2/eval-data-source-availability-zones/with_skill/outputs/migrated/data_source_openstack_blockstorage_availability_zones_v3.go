package openstack

import (
	"context"
	"sort"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/availabilityzones"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	fwschema "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-openstack/utils/v2/hashcode"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &blockStorageAvailabilityZonesV3DataSource{}
	_ datasource.DataSourceWithConfigure = &blockStorageAvailabilityZonesV3DataSource{}
)

// NewBlockStorageAvailabilityZonesV3DataSource is the constructor used when
// registering this data source in the provider's DataSources() list.
func NewBlockStorageAvailabilityZonesV3DataSource() datasource.DataSource {
	return &blockStorageAvailabilityZonesV3DataSource{}
}

// blockStorageAvailabilityZonesV3DataSource holds provider-level config.
type blockStorageAvailabilityZonesV3DataSource struct {
	config *Config
}

// blockStorageAvailabilityZonesV3Model is the Terraform state model.
type blockStorageAvailabilityZonesV3Model struct {
	ID     types.String `tfsdk:"id"`
	Region types.String `tfsdk:"region"`
	State  types.String `tfsdk:"state"`
	Names  types.List   `tfsdk:"names"`
}

// --------------------------------------------------------------------------
// datasource.DataSource
// --------------------------------------------------------------------------

func (d *blockStorageAvailabilityZonesV3DataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_blockstorage_availability_zones_v3"
}

func (d *blockStorageAvailabilityZonesV3DataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"region": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			// "state" has a default of "available" in SDKv2. The framework's
			// datasource schema has no Default field, so we handle the default
			// in the Read method: if the practitioner omits the attribute (null),
			// we treat it as "available".
			"state": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Validators: []fwschema.String{
					stringvalidator.OneOfCaseInsensitive("available", "unavailable"),
				},
			},
			"names": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// --------------------------------------------------------------------------
// datasource.DataSourceWithConfigure
// --------------------------------------------------------------------------

func (d *blockStorageAvailabilityZonesV3DataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Expected *Config, got a different type.",
		)

		return
	}

	d.config = config
}

// --------------------------------------------------------------------------
// Read
// --------------------------------------------------------------------------

func (d *blockStorageAvailabilityZonesV3DataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	var model blockStorageAvailabilityZonesV3Model

	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve region: use the value from config if set, else fall back to the
	// provider-level region.
	region := d.config.Region
	if !model.Region.IsNull() && !model.Region.IsUnknown() && model.Region.ValueString() != "" {
		region = model.Region.ValueString()
	}

	client, err := d.config.BlockStorageV3Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError("Error creating OpenStack block storage client", err.Error())

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

	// Resolve state filter: default to "available" when not specified by the
	// practitioner, matching the SDKv2 Default: "available" behaviour.
	stateFilter := "available"
	if !model.State.IsNull() && !model.State.IsUnknown() && model.State.ValueString() != "" {
		stateFilter = model.State.ValueString()
	}

	wantAvailable := stateFilter == "available"

	var zones []string

	for _, z := range zoneInfo {
		if z.ZoneState.Available == wantAvailable {
			zones = append(zones, z.ZoneName)
		}
	}

	sort.Strings(zones)

	names, diags := types.ListValueFrom(ctx, types.StringType, zones)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	model.ID = types.StringValue(hashcode.Strings(zones))
	model.Region = types.StringValue(region)
	model.State = types.StringValue(stateFilter)
	model.Names = names

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
