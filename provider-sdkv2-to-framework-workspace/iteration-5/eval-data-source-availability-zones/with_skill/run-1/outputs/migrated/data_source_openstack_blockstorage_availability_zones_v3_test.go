package openstack

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBlockStorageV3AvailabilityZonesV3_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		// Switched from ProviderFactories (SDKv2) to ProtoV6ProviderFactories
		// (framework). testAccProtoV6ProviderFactories is expected to be
		// declared once the openstack provider is itself migrated to the
		// framework or muxed; see notes.md in this run's outputs for the
		// caveat that, on a still-SDKv2 provider tree, this symbol is not
		// yet defined and the test will fail to compile until that wiring
		// lands. That compile failure IS the TDD red gate (step 7).
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
