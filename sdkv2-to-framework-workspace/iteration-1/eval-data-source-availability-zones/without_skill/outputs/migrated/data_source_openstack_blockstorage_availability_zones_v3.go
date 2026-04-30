package openstack

import (
	"context"
	"sort"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/availabilityzones"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/terraform-provider-openstack/utils/v2/hashcode"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &blockStorageAvailabilityZonesV3DataSource{}
	_ datasource.DataSourceWithConfigure = &blockStorageAvailabilityZonesV3DataSource{}
)

// blockStorageAvailabilityZonesV3DataSource is the data source implementation.
type blockStorageAvailabilityZonesV3DataSource struct {
	config *Config
}

// blockStorageAvailabilityZonesV3DataSourceModel describes the data source data model.
type blockStorageAvailabilityZonesV3DataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	Region types.String `tfsdk:"region"`
	State  types.String `tfsdk:"state"`
	Names  types.List   `tfsdk:"names"`
}

// NewBlockStorageAvailabilityZonesV3DataSource is a helper function to simplify
// the provider implementation.
func NewBlockStorageAvailabilityZonesV3DataSource() datasource.DataSource {
	return &blockStorageAvailabilityZonesV3DataSource{}
}

// Metadata returns the data source type name.
func (d *blockStorageAvailabilityZonesV3DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_blockstorage_availability_zones_v3"
}

// Schema defines the schema for the data source.
func (d *blockStorageAvailabilityZonesV3DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
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

// Configure adds the provider-configured client to the data source.
func (d *blockStorageAvailabilityZonesV3DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Expected *Config, got an unexpected type. Please report this issue to the provider developers.",
		)

		return
	}

	d.config = config
}

// Read refreshes the Terraform state with the latest data.
func (d *blockStorageAvailabilityZonesV3DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data blockStorageAvailabilityZonesV3DataSourceModel

	// Read Terraform configuration data into the model.
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Determine the region.
	region := d.config.Region
	if !data.Region.IsNull() && !data.Region.IsUnknown() && data.Region.ValueString() != "" {
		region = data.Region.ValueString()
	}

	region = d.config.DetermineRegion(region)

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

	// Default state to "available" if not set.
	stateVal := "available"
	if !data.State.IsNull() && !data.State.IsUnknown() && data.State.ValueString() != "" {
		stateVal = data.State.ValueString()
	}

	stateBool := stateVal == "available"

	var zones []string

	for _, z := range zoneInfo {
		if z.ZoneState.Available == stateBool {
			zones = append(zones, z.ZoneName)
		}
	}

	// sort.Strings sorts in place, returns nothing.
	sort.Strings(zones)

	namesValue, diags := types.ListValueFrom(ctx, types.StringType, zones)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	data.ID = types.StringValue(hashcode.Strings(zones))
	data.Region = types.StringValue(region)
	data.State = types.StringValue(stateVal)
	data.Names = namesValue

	// Save data into Terraform state.
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
