package vpc_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/acceptance"
	"github.com/digitalocean/terraform-provider-digitalocean/digitalocean/config"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// NOTE: acceptance.TestAccProviderFactories is the SDKv2 factory map
// (ProviderFactories shape). Once the provider definition itself is
// migrated to terraform-plugin-framework, the acceptance package should
// expose a ProtoV6ProviderFactories map of type
// map[string]func() (tfprotov6.ProviderServer, error). All test cases
// below reference acceptance.TestAccProtoV6ProviderFactories and assume
// that field exists alongside (or replacing) TestAccProviderFactories.

func TestAccDigitalOceanVPC_Basic(t *testing.T) {
	vpcName := acceptance.RandomTestName()
	vpcDesc := "A description for the VPC"
	vpcCreateConfig := fmt.Sprintf(testAccCheckDigitalOceanVPCConfig_Basic, vpcName, vpcDesc)

	updatedVPCName := acceptance.RandomTestName()
	updatedVPVDesc := "A brand new updated description for the VPC"
	vpcUpdateConfig := fmt.Sprintf(testAccCheckDigitalOceanVPCConfig_Basic, updatedVPCName, updatedVPVDesc)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanVPCDestroy,
		Steps: []resource.TestStep{
			{
				Config: vpcCreateConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckDigitalOceanVPCExists("digitalocean_vpc.foobar"),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "name", vpcName),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "default", "false"),
					resource.TestCheckResourceAttrSet(
						"digitalocean_vpc.foobar", "created_at"),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "description", vpcDesc),
					resource.TestCheckResourceAttrSet(
						"digitalocean_vpc.foobar", "ip_range"),
					resource.TestCheckResourceAttrSet(
						"digitalocean_vpc.foobar", "urn"),
				),
			},
			{
				Config: vpcUpdateConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckDigitalOceanVPCExists("digitalocean_vpc.foobar"),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "name", updatedVPCName),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "description", updatedVPVDesc),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "default", "false"),
				),
			},
		},
	})
}

func TestAccDigitalOceanVPC_IPRange(t *testing.T) {
	vpcName := acceptance.RandomTestName()
	vpcCreateConfig := fmt.Sprintf(testAccCheckDigitalOceanVPCConfig_IPRange, vpcName)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanVPCDestroy,
		Steps: []resource.TestStep{
			{
				Config: vpcCreateConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckDigitalOceanVPCExists("digitalocean_vpc.foobar"),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "name", vpcName),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "ip_range", "10.10.10.0/24"),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foobar", "default", "false"),
				),
			},
		},
	})
}

// https://github.com/digitalocean/terraform-provider-digitalocean/issues/551
func TestAccDigitalOceanVPC_IPRangeRace(t *testing.T) {
	vpcNameOne := acceptance.RandomTestName()
	vpcNameTwo := acceptance.RandomTestName()
	vpcCreateConfig := fmt.Sprintf(testAccCheckDigitalOceanVPCConfig_IPRangeRace, vpcNameOne, vpcNameTwo)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanVPCDestroy,
		Steps: []resource.TestStep{
			{
				Config: vpcCreateConfig,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckDigitalOceanVPCExists("digitalocean_vpc.foo"),
					testAccCheckDigitalOceanVPCExists("digitalocean_vpc.bar"),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.foo", "name", vpcNameOne),
					resource.TestCheckResourceAttrSet(
						"digitalocean_vpc.foo", "ip_range"),
					resource.TestCheckResourceAttr(
						"digitalocean_vpc.bar", "name", vpcNameTwo),
					resource.TestCheckResourceAttrSet(
						"digitalocean_vpc.bar", "ip_range"),
				),
			},
		},
	})
}

// TestAccDigitalOceanVPC_importBasic was previously in import_vpc_test.go
// against the SDKv2 resource. The import path is preserved (passthrough
// ID), so the test shape is unchanged aside from the framework provider
// factory. We keep it co-located here for the migration smoke; if the
// surrounding repo prefers it back in import_vpc_test.go, lift it across.
func TestAccDigitalOceanVPC_importBasic(t *testing.T) {
	resourceName := "digitalocean_vpc.foobar"
	vpcName := acceptance.RandomTestName()
	vpcCreateConfig := fmt.Sprintf(testAccCheckDigitalOceanVPCConfig_Basic, vpcName, "A description for the VPC")

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { acceptance.TestAccPreCheck(t) },
		ProtoV6ProviderFactories: acceptance.TestAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDigitalOceanVPCDestroy,
		Steps: []resource.TestStep{
			{
				Config: vpcCreateConfig,
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Test that importing a non-existent resource produces the
			// expected error message.
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: false,
				ImportStateId:     "123abc",
				ExpectError:       regexp.MustCompile(`(Please verify the ID is correct|Cannot import non-existent remote object)`),
			},
		},
	})
}

func testAccCheckDigitalOceanVPCDestroy(s *terraform.State) error {
	client := acceptance.TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "digitalocean_vpc" {
			continue
		}

		_, _, err := client.VPCs.Get(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("VPC resource still exists")
		}
	}

	return nil
}

func testAccCheckDigitalOceanVPCExists(resourceAddr string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		client := acceptance.TestAccProvider.Meta().(*config.CombinedConfig).GodoClient()

		rs, ok := s.RootModule().Resources[resourceAddr]
		if !ok {
			return fmt.Errorf("Not found: %s", resourceAddr)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("No ID set for resource: %s", resourceAddr)
		}

		foundVPC, _, err := client.VPCs.Get(context.Background(), rs.Primary.ID)
		if err != nil {
			return err
		}
		if foundVPC.ID != rs.Primary.ID {
			return fmt.Errorf("Resource not found: %s : %s", resourceAddr, rs.Primary.ID)
		}

		return nil
	}
}

const testAccCheckDigitalOceanVPCConfig_Basic = `
resource "digitalocean_vpc" "foobar" {
  name        = "%s"
  description = "%s"
  region      = "nyc3"
}
`

const testAccCheckDigitalOceanVPCConfig_IPRange = `
resource "digitalocean_vpc" "foobar" {
  name     = "%s"
  region   = "nyc3"
  ip_range = "10.10.10.0/24"
}
`

const testAccCheckDigitalOceanVPCConfig_IPRangeRace = `
resource "digitalocean_vpc" "foo" {
  name   = "%s"
  region = "nyc3"
}

resource "digitalocean_vpc" "bar" {
  name   = "%s"
  region = "nyc3"
}
`
