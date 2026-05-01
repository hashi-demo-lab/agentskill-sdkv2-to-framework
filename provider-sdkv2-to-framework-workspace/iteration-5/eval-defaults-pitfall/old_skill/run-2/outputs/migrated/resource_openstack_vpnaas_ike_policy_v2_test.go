package openstack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccProtoV6ProviderFactoriesIKE returns the factories used by the
// migrated openstack_vpnaas_ike_policy_v2 acceptance tests. The migrated
// resource is served via the framework provider, while the rest of the
// repository continues to live on the SDKv2 Provider() definition. The
// glue (constructing a framework provider that registers
// NewIKEPolicyV2Resource and serves over protocol v6) is provider-level
// wiring; once that wiring is in place, the variable below points at it.
//
// During the partial-migration phase, the test factory map name follows the
// SKILL.md guidance: ProtoV6ProviderFactories on the TestCase, factory
// returning a tfprotov6.ProviderServer.
var testAccProtoV6ProviderFactoriesIKE = func() map[string]func() (tfprotov6.ProviderServer, error) {
	// The factory is wired up alongside the framework provider scaffolding
	// added as part of step 3 of the workflow (serve via framework). See
	// provider_framework.go (added in the same migration cycle).
	return testAccProtoV6ProviderFactories()
}()

func TestAccIKEPolicyVPNaaSV2_basic(t *testing.T) {
	var policy ikepolicies.Policy

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckVPN(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesIKE,
		CheckDestroy:             testAccCheckIKEPolicyV2Destroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccIKEPolicyV2Basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIKEPolicyV2Exists(t.Context(),
						"openstack_vpnaas_ike_policy_v2.policy_1", &policy),
					resource.TestCheckResourceAttrPtr("openstack_vpnaas_ike_policy_v2.policy_1", "name", &policy.Name),
					resource.TestCheckResourceAttrPtr("openstack_vpnaas_ike_policy_v2.policy_1", "description", &policy.Description),
					resource.TestCheckResourceAttrPtr("openstack_vpnaas_ike_policy_v2.policy_1", "tenant_id", &policy.TenantID),
					// Defaults migrated to framework stringdefault.StaticString
					// values; assert them in state to lock the migration in.
					resource.TestCheckResourceAttr("openstack_vpnaas_ike_policy_v2.policy_1", "auth_algorithm", "sha1"),
					resource.TestCheckResourceAttr("openstack_vpnaas_ike_policy_v2.policy_1", "encryption_algorithm", "aes-128"),
					resource.TestCheckResourceAttr("openstack_vpnaas_ike_policy_v2.policy_1", "pfs", "group5"),
					resource.TestCheckResourceAttr("openstack_vpnaas_ike_policy_v2.policy_1", "phase1_negotiation_mode", "main"),
					resource.TestCheckResourceAttr("openstack_vpnaas_ike_policy_v2.policy_1", "ike_version", "v1"),
				),
			},
		},
	})
}

func TestAccIKEPolicyVPNaaSV2_withLifetime(t *testing.T) {
	var policy ikepolicies.Policy

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckVPN(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesIKE,
		CheckDestroy:             testAccCheckIKEPolicyV2Destroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccIKEPolicyV2WithLifetime,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIKEPolicyV2Exists(t.Context(),
						"openstack_vpnaas_ike_policy_v2.policy_1", &policy),
					// testAccCheckLifetime("openstack_vpnaas_ike_policy_v2.policy_1", &policy.Lifetime.Units, &policy.Lifetime.Value),
				),
			},
		},
	})
}

func TestAccIKEPolicyVPNaaSV2_Update(t *testing.T) {
	var policy ikepolicies.Policy

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckVPN(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesIKE,
		CheckDestroy:             testAccCheckIKEPolicyV2Destroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccIKEPolicyV2Basic,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIKEPolicyV2Exists(t.Context(),
						"openstack_vpnaas_ike_policy_v2.policy_1", &policy),
					resource.TestCheckResourceAttrPtr("openstack_vpnaas_ike_policy_v2.policy_1", "name", &policy.Name),
				),
			},
			{
				Config: testAccIKEPolicyV2Update,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIKEPolicyV2Exists(t.Context(),
						"openstack_vpnaas_ike_policy_v2.policy_1", &policy),
					resource.TestCheckResourceAttrPtr("openstack_vpnaas_ike_policy_v2.policy_1", "name", &policy.Name),
				),
			},
		},
	})
}

func TestAccIKEPolicyVPNaaSV2_withLifetimeUpdate(t *testing.T) {
	var policy ikepolicies.Policy

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
			testAccPreCheckVPN(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesIKE,
		CheckDestroy:             testAccCheckIKEPolicyV2Destroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccIKEPolicyV2WithLifetime,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIKEPolicyV2Exists(t.Context(),
						"openstack_vpnaas_ike_policy_v2.policy_1", &policy),
					// testAccCheckLifetime("openstack_vpnaas_ike_policy_v2.policy_1", &policy.Lifetime.Units, &policy.Lifetime.Value),
					resource.TestCheckResourceAttrPtr("openstack_vpnaas_ike_policy_v2.policy_1", "auth_algorithm", &policy.AuthAlgorithm),
					resource.TestCheckResourceAttrPtr("openstack_vpnaas_ike_policy_v2.policy_1", "pfs", &policy.PFS),
				),
			},
			{
				Config: testAccIKEPolicyV2WithLifetimeUpdate,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckIKEPolicyV2Exists(t.Context(),
						"openstack_vpnaas_ike_policy_v2.policy_1", &policy),
					// testAccCheckLifetime("openstack_vpnaas_ike_policy_v2.policy_1", &policy.Lifetime.Units, &policy.Lifetime.Value),
				),
			},
		},
	})
}

func testAccCheckIKEPolicyV2Destroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// testAccProvider remains the SDKv2 Provider during the partial
		// migration; the framework provider is only used to serve the
		// migrated resource. The Config object reachable via testAccProvider
		// still has the credentials we need to talk to the API directly.
		config := testAccProvider.Meta().(*Config)

		networkingClient, err := config.NetworkingV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack networking client: %w", err)
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "openstack_vpnaas_ike_policy_v2" {
				continue
			}

			_, err = ikepolicies.Get(ctx, networkingClient, rs.Primary.ID).Extract()
			if err == nil {
				return fmt.Errorf("IKE policy (%s) still exists", rs.Primary.ID)
			}

			if !gophercloud.ResponseCodeIs(err, http.StatusNotFound) {
				return err
			}
		}

		return nil
	}
}

func testAccCheckIKEPolicyV2Exists(ctx context.Context, n string, policy *ikepolicies.Policy) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[n]
		if !ok {
			return fmt.Errorf("Not found: %s", n)
		}

		if rs.Primary.ID == "" {
			return errors.New("No ID is set")
		}

		config := testAccProvider.Meta().(*Config)

		networkingClient, err := config.NetworkingV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack networking client: %w", err)
		}

		found, err := ikepolicies.Get(ctx, networkingClient, rs.Primary.ID).Extract()
		if err != nil {
			return err
		}

		*policy = *found

		return nil
	}
}

const testAccIKEPolicyV2Basic = `
resource "openstack_vpnaas_ike_policy_v2" "policy_1" {
}
`

const testAccIKEPolicyV2Update = `
resource "openstack_vpnaas_ike_policy_v2" "policy_1" {
	name = "updatedname"
}
`

// testAccIKEPolicyV2WithLifetime keeps the original block-form HCL syntax —
// the migrated schema retains `lifetime` as a SetNestedBlock so practitioner
// configurations don't break.
const testAccIKEPolicyV2WithLifetime = `
resource "openstack_vpnaas_ike_policy_v2" "policy_1" {
	auth_algorithm = "sha256"
	pfs = "group14"
	lifetime {
		units = "seconds"
		value = 1200
	}
}
`

const testAccIKEPolicyV2WithLifetimeUpdate = `
resource "openstack_vpnaas_ike_policy_v2" "policy_1" {
	auth_algorithm = "sha256"
	pfs = "group14"
	lifetime {
		units = "seconds"
		value = 1400
	}
}
`
