package openstack

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/attachinterfaces"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// regexpConflictsWith builds a regex that matches the framework's
// stringvalidator.ConflictsWith error message. The framework formats the
// diagnostic as "Attribute <a> cannot be specified when <b> is specified",
// but the order of <a>/<b> depends on which attribute the validator is
// attached to. We accept either ordering.
func regexpConflictsWith(a, b string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(
		`(?s)(Attribute "?%[1]s"?.*cannot be specified when "?%[2]s"?.*is specified|Attribute "?%[2]s"?.*cannot be specified when "?%[1]s"?.*is specified)`,
		regexp.QuoteMeta(a), regexp.QuoteMeta(b),
	))
}

func TestAccComputeV2InterfaceAttach_basic(t *testing.T) {
	var ai attachinterfaces.Interface

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckComputeV2InterfaceAttachDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccComputeV2InterfaceAttachBasic(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckComputeV2InterfaceAttachExists(t.Context(), "openstack_compute_interface_attach_v2.ai_1", &ai),
				),
			},
		},
	})
}

func TestAccComputeV2InterfaceAttach_IP(t *testing.T) {
	var ai attachinterfaces.Interface

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckComputeV2InterfaceAttachDestroy(t.Context()),
		Steps: []resource.TestStep{
			{
				Config: testAccComputeV2InterfaceAttachIP(),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckComputeV2InterfaceAttachExists(t.Context(), "openstack_compute_interface_attach_v2.ai_1", &ai),
					testAccCheckComputeV2InterfaceAttachIP(&ai, "192.168.1.100"),
				),
			},
		},
	})
}

// TestAccComputeV2InterfaceAttach_conflictPortNetwork asserts that the
// per-attribute ConflictsWith validator (translated from the SDKv2
// ConflictsWith schema field) rejects configs that set both port_id and
// network_id at plan time, before any API calls are made.
func TestAccComputeV2InterfaceAttach_conflictPortNetwork(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccComputeV2InterfaceAttachConflictPortNetwork(),
				ExpectError: regexpConflictsWith("port_id", "network_id"),
				PlanOnly:    true,
			},
		},
	})
}

// TestAccComputeV2InterfaceAttach_conflictFixedIPPort asserts that fixed_ip
// and port_id cannot be set together (translated from the SDKv2
// ConflictsWith: []string{"port_id"} on fixed_ip).
func TestAccComputeV2InterfaceAttach_conflictFixedIPPort(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccComputeV2InterfaceAttachConflictFixedIPPort(),
				ExpectError: regexpConflictsWith("fixed_ip", "port_id"),
				PlanOnly:    true,
			},
		},
	})
}

func testAccCheckComputeV2InterfaceAttachDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		config := testAccProvider.Meta().(*Config)

		computeClient, err := config.ComputeV2Client(ctx, osRegionName)
		if err != nil {
			return fmt.Errorf("Error creating OpenStack compute client: %w", err)
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "openstack_compute_interface_attach_v2" {
				continue
			}

			instanceID, portID, err := parsePairedIDs(rs.Primary.ID, "openstack_compute_interface_attach_v2")
			if err != nil {
				return err
			}

			_, err = attachinterfaces.Get(ctx, computeClient, instanceID, portID).Extract()
			if err == nil {
				return errors.New("Volume attachment still exists")
			}
		}

		return nil
	}
}

func testAccCheckComputeV2InterfaceAttachExists(ctx context.Context, n string, ai *attachinterfaces.Interface) resource.TestCheckFunc {
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

		instanceID, portID, err := parsePairedIDs(rs.Primary.ID, "openstack_compute_interface_attach_v2")
		if err != nil {
			return err
		}

		found, err := attachinterfaces.Get(ctx, computeClient, instanceID, portID).Extract()
		if err != nil {
			return err
		}

		// if found.instanceID != instanceID || found.PortID != portID {
		if found.PortID != portID {
			return errors.New("InterfaceAttach not found")
		}

		*ai = *found

		return nil
	}
}

func testAccCheckComputeV2InterfaceAttachIP(
	ai *attachinterfaces.Interface, ip string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		for _, i := range ai.FixedIPs {
			if i.IPAddress == ip {
				return nil
			}
		}

		return fmt.Errorf("Requested ip (%s) does not exist on port", ip)
	}
}

func testAccComputeV2InterfaceAttachBasic() string {
	return fmt.Sprintf(`
resource "openstack_networking_port_v2" "port_1" {
  name = "port_1"
  network_id = "%s"
  admin_state_up = "true"
}

resource "openstack_compute_instance_v2" "instance_1" {
  name = "instance_1"
  security_groups = ["default"]
  network {
    uuid = "%s"
  }
}

resource "openstack_compute_interface_attach_v2" "ai_1" {
  instance_id = openstack_compute_instance_v2.instance_1.id
  port_id = openstack_networking_port_v2.port_1.id
}
`, osNetworkID, osNetworkID)
}

func testAccComputeV2InterfaceAttachIP() string {
	return fmt.Sprintf(`
resource "openstack_networking_network_v2" "network_1" {
  name = "network_1"
}

resource "openstack_networking_subnet_v2" "subnet_1" {
  name = "subnet_1"
  network_id = openstack_networking_network_v2.network_1.id
  cidr = "192.168.1.0/24"
  ip_version = 4
  enable_dhcp = true
  no_gateway = true
}

resource "openstack_compute_instance_v2" "instance_1" {
  name = "instance_1"
  security_groups = ["default"]
  network {
    uuid = "%s"
  }
}

resource "openstack_compute_interface_attach_v2" "ai_1" {
  instance_id = openstack_compute_instance_v2.instance_1.id
  network_id = openstack_networking_network_v2.network_1.id
  fixed_ip = "192.168.1.100"
}
`, osNetworkID)
}

// testAccComputeV2InterfaceAttachConflictPortNetwork sets both port_id and
// network_id, which should be rejected by the migrated ConflictsWith
// validators before any API call is made.
func testAccComputeV2InterfaceAttachConflictPortNetwork() string {
	return `
resource "openstack_compute_interface_attach_v2" "ai_1" {
  instance_id = "00000000-0000-0000-0000-000000000000"
  port_id     = "11111111-1111-1111-1111-111111111111"
  network_id  = "22222222-2222-2222-2222-222222222222"
}
`
}

// testAccComputeV2InterfaceAttachConflictFixedIPPort sets both fixed_ip and
// port_id, which should be rejected by the migrated ConflictsWith validator
// on fixed_ip.
func testAccComputeV2InterfaceAttachConflictFixedIPPort() string {
	return `
resource "openstack_compute_interface_attach_v2" "ai_1" {
  instance_id = "00000000-0000-0000-0000-000000000000"
  port_id     = "11111111-1111-1111-1111-111111111111"
  fixed_ip    = "192.168.1.100"
}
`
}
