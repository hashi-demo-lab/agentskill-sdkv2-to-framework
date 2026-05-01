package openstack

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccProtoV6ProviderFactories uses the framework provider server.
// NewOpenStackProvider() is the framework provider constructor introduced
// as part of the SDKv2 → framework migration (step 4 of the workflow).
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"openstack": providerserver.NewProtocol6WithError(NewOpenStackProvider()()),
}

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
					resource.TestMatchResourceAttr("data.openstack_blockstorage_availability_zones_v3.zones", "names.#", regexp.MustCompile(`[1-9]\d*`)),
				),
			},
		},
	})
}

const testAccBlockStorageV3AvailabilityZonesConfig = `
data "openstack_blockstorage_availability_zones_v3" "zones" {}
`
