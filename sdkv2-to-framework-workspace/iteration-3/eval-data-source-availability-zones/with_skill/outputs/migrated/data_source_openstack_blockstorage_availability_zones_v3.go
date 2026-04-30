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

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &blockStorageAvailabilityZonesV3DataSource{}
	_ datasource.DataSourceWithConfigure = &blockStorageAvailabilityZonesV3DataSource{}
)

// blockStorageAvailabilityZonesV3DataSource is the data source implementation.
type blockStorageAvailabilityZonesV3DataSource struct {
	config *Config
}

// blockStorageAvailabilityZonesV3DataSourceModel maps the data source schema data.
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
				Computed: true,
				Optional: true,
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
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.config = config
}

// Read refreshes the Terraform state with the latest data.
func (d *blockStorageAvailabilityZonesV3DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state blockStorageAvailabilityZonesV3DataSourceModel

	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve region: use value from config if set, otherwise fall back to provider-level region.
	region := d.config.DetermineRegion(state.Region.ValueString())

	client, err := d.config.BlockStorageV3Client(ctx, region)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating OpenStack block storage client",
			fmt.Sprintf("Error creating OpenStack block storage client: %s", err),
		)
		return
	}

	allPages, err := availabilityzones.List(client).AllPages(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error retrieving openstack_blockstorage_availability_zones_v3",
			fmt.Sprintf("Error retrieving openstack_blockstorage_availability_zones_v3: %s", err),
		)
		return
	}

	zoneInfo, err := availabilityzones.ExtractAvailabilityZones(allPages)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error extracting openstack_blockstorage_availability_zones_v3 from response",
			fmt.Sprintf("Error extracting openstack_blockstorage_availability_zones_v3 from response: %s", err),
		)
		return
	}

	// Default state to "available" when not set (mirrors SDKv2 Default: "available").
	filterState := "available"
	if !state.State.IsNull() && !state.State.IsUnknown() && state.State.ValueString() != "" {
		filterState = state.State.ValueString()
	}
	stateBool := filterState == "available"

	var zones []string
	for _, z := range zoneInfo {
		if z.ZoneState.Available == stateBool {
			zones = append(zones, z.ZoneName)
		}
	}

	sort.Strings(zones)

	namesValue, diags2 := types.ListValueFrom(ctx, types.StringType, zones)
	resp.Diagnostics.Append(diags2...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = types.StringValue(hashcode.Strings(zones))
	state.Region = types.StringValue(region)
	state.State = types.StringValue(filterState)
	state.Names = namesValue

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
