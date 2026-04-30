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

var (
	_ datasource.DataSource              = &blockStorageAvailabilityZonesV3DataSource{}
	_ datasource.DataSourceWithConfigure = &blockStorageAvailabilityZonesV3DataSource{}
)

// NewBlockStorageAvailabilityZonesV3DataSource returns a new instance of the data source.
func NewBlockStorageAvailabilityZonesV3DataSource() datasource.DataSource {
	return &blockStorageAvailabilityZonesV3DataSource{}
}

type blockStorageAvailabilityZonesV3DataSource struct {
	config *Config
}

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
			"id": schema.StringAttribute{
				Computed: true,
			},

			"region": schema.StringAttribute{
				Computed: true,
				Optional: true,
			},

			// Default "available" is applied in Read when the value is null.
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

func (d *blockStorageAvailabilityZonesV3DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Config, got: %T. Please report this issue.", req.ProviderData),
		)
		return
	}

	d.config = config
}

func (d *blockStorageAvailabilityZonesV3DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg blockStorageAvailabilityZonesV3DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Apply default for state when not provided.
	stateFilter := "available"
	if !cfg.State.IsNull() && !cfg.State.IsUnknown() && cfg.State.ValueString() != "" {
		stateFilter = cfg.State.ValueString()
	}

	region := d.config.Region
	if !cfg.Region.IsNull() && !cfg.Region.IsUnknown() && cfg.Region.ValueString() != "" {
		region = cfg.Region.ValueString()
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

	stateBool := stateFilter == "available"

	var zones []string
	for _, z := range zoneInfo {
		if z.ZoneState.Available == stateBool {
			zones = append(zones, z.ZoneName)
		}
	}

	sort.Strings(zones)

	names, diags := types.ListValueFrom(ctx, types.StringType, zones)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg.ID = types.StringValue(hashcode.Strings(zones))
	cfg.Region = types.StringValue(region)
	cfg.State = types.StringValue(stateFilter)
	cfg.Names = names

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
