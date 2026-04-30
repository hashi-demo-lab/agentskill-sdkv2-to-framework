package openstack

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccBlockStorageV3AvailabilityZonesV3_basic exercises the migrated
// framework data source. ProviderFactories is updated from the SDKv2
// testAccProviders (map[string]func() (*schema.Provider, error)) to
// testAccProtoV6ProviderFactories, which boots the provider via
// providerserver.NewProtocol6WithError so the framework data source is
// properly served.
func TestAccBlockStorageV3AvailabilityZonesV3_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccBlockStorageV3AvailabilityZonesConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr(
						"data.openstack_blockstorage_availability_zones_v3.zones",
						"names.#",
						regexp.MustCompile(`[1-9]\d*`),
					),
				),
			},
		},
	})
}

const testAccBlockStorageV3AvailabilityZonesConfig = `
data "openstack_blockstorage_availability_zones_v3" "zones" {}
`
