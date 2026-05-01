package openstack

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// protoV6ProviderFactories wires up the framework provider for acceptance tests.
// Replace NewFrameworkProvider() with the actual framework provider constructor
// once the provider has been fully migrated.
var protoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openstack": providerserver.NewProtocol6WithError(NewFrameworkProvider("test")()),
}

func TestAccBlockStorageV3AvailabilityZonesV3_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccBlockStorageV3AvailabilityZonesConfig,
				Check: resource.ComposeTestCheckFunc(
					resource.TestMatchResourceAttr("data.openstack_blockstorage_availability_zones_v3.zones", "names.#", regexp.MustCompile(`[1-9]\d*`)),
					resource.TestCheckResourceAttr("data.openstack_blockstorage_availability_zones_v3.zones", "state", "available"),
				),
			},
		},
	})
}

func TestAccBlockStorageV3AvailabilityZonesV3_unavailable(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: protoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccBlockStorageV3AvailabilityZonesConfigUnavailable,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.openstack_blockstorage_availability_zones_v3.zones", "state", "unavailable"),
				),
			},
		},
	})
}

const testAccBlockStorageV3AvailabilityZonesConfig = `
data "openstack_blockstorage_availability_zones_v3" "zones" {}
`

const testAccBlockStorageV3AvailabilityZonesConfigUnavailable = `
data "openstack_blockstorage_availability_zones_v3" "zones" {
  state = "unavailable"
}
`
