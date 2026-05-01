package openstack

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/keypairs"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccComputeV2Keypair_basic verifies that a keypair can be created with
// an explicit public key and is destroyed cleanly.
//
// NOTE: ProviderFactories here still uses the SDKv2 testAccProviders because
// the overall provider has not yet been migrated to the framework.  Once the
// provider root is migrated, replace ProviderFactories with:
//
//	ProtoV6ProviderFactories: protoV6ProviderFactories
//
// where protoV6ProviderFactories is wired to the framework provider factory.
func TestAccComputeV2Keypair_basic(t *testing.T) {
	var keypair keypairs.KeyPair

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		// TODO(migration): swap to ProtoV6ProviderFactories once the provider
		// itself is migrated to terraform-plugin-framework.
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckComputeV2KeypairDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccComputeV2KeypairBasic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckComputeV2KeypairExists(t.Context(), "openstack_compute_keypair_v2.kp_1", &keypair),
				),
			},
			// Verify import round-trip.
			{
				ResourceName:      "openstack_compute_keypair_v2.kp_1",
				ImportState:       true,
				ImportStateVerify: true,
				// private_key is returned only on Create and cannot be
				// reconstructed by the API on import.
				ImportStateVerifyIgnore: []string{"private_key"},
			},
		},
	})
}

// TestAccComputeV2Keypair_generatePrivate verifies that omitting public_key
// causes the provider to generate an RSA key pair and return the private key.
func TestAccComputeV2Keypair_generatePrivate(t *testing.T) {
	var keypair keypairs.KeyPair

	fingerprintRe := regexp.MustCompile(`[a-f0-9:]+`)
	privateKeyRe := regexp.MustCompile(`.*BEGIN RSA PRIVATE KEY.*`)

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		// TODO(migration): swap to ProtoV6ProviderFactories once the provider
		// itself is migrated to terraform-plugin-framework.
		ProviderFactories: testAccProviders,
		CheckDestroy:      testAccCheckComputeV2KeypairDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccComputeV2KeypairGeneratePrivate,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckComputeV2KeypairExists(t.Context(), "openstack_compute_keypair_v2.kp_1", &keypair),
					resource.TestMatchResourceAttr(
						"openstack_compute_keypair_v2.kp_1", "fingerprint", fingerprintRe),
					resource.TestMatchResourceAttr(
						"openstack_compute_keypair_v2.kp_1", "private_key", privateKeyRe),
				),
			},
		},
	})
}

// testAccCheckComputeV2KeypairDestroy asserts that all test keypairs have
// been removed from the API.
func testAccCheckComputeV2KeypairDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		config := testAccProvider.Meta().(*Config)

		computeClient, err := config.ComputeV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack compute client: %w", err)
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "openstack_compute_keypair_v2" {
				continue
			}

			_, err := keypairs.Get(ctx, computeClient, rs.Primary.ID, keypairs.GetOpts{}).Extract()
			if err == nil {
				return errors.New("Keypair still exists")
			}
		}

		return nil
	}
}

// testAccCheckComputeV2KeypairExists verifies the named resource is present
// in Terraform state and retrievable from the API.
func testAccCheckComputeV2KeypairExists(ctx context.Context, n string, kp *keypairs.KeyPair) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return errors.New("No ID is set")
		}

		config := testAccProvider.Meta().(*Config)

		computeClient, err := config.ComputeV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack compute client: %w", err)
		}

		found, err := keypairs.Get(ctx, computeClient, rs.Primary.ID, keypairs.GetOpts{}).Extract()
		if err != nil {
			return err
		}

		if found.Name != rs.Primary.ID {
			return errors.New("Keypair not found")
		}

		*kp = *found

		return nil
	}
}

const testAccComputeV2KeypairBasic = `
resource "openstack_compute_keypair_v2" "kp_1" {
  name = "kp_1"
  public_key = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDAjpC1hwiOCCmKEWxJ4qzTTsJbKzndLo1BCz5PcwtUnflmU+gHJtWMZKpuEGVi29h0A/+ydKek1O18k10Ff+4tyFjiHDQAT9+OfgWf7+b1yK+qDip3X1C0UPMbwHlTfSGWLGZquwhvEFx9k3h/M+VtMvwR1lJ9LUyTAImnNjWG7TAIPmui30HvM2UiFEmqkr4ijq45MyX2+fLIePLRIFuu1p4whjHAQYufqyno3BS48icQb4p6iVEZPo4AE2o9oIyQvj2mx4dk5Y8CgSETOZTYDOR3rU2fZTRDRgPJDH9FWvQjF5tA0p3d9CoWWd2s6GKKbfoUIi8R/Db1BSPJwkqB jrp-hp-pc"
}
`

const testAccComputeV2KeypairGeneratePrivate = `
resource "openstack_compute_keypair_v2" "kp_1" {
  name = "kp_1"
}
`
