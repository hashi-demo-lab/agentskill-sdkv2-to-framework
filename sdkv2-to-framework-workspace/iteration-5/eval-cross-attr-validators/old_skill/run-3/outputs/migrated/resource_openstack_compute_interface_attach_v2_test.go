package openstack

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/attachinterfaces"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

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

// TestAccComputeV2InterfaceAttach_conflictingPortAndNetwork asserts that the
// framework cross-attribute validator rejects a config that sets both
// port_id and network_id (replacing the SDKv2 ConflictsWith schema field).
func TestAccComputeV2InterfaceAttach_conflictingPortAndNetwork(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccComputeV2InterfaceAttachConflictingPortAndNetwork(),
				ExpectError: regexpConflictsWith("port_id", "network_id"),
				PlanOnly:    true,
			},
		},
	})
}

// TestAccComputeV2InterfaceAttach_conflictingFixedIPAndPort asserts that the
// framework cross-attribute validator rejects a config that sets both
// fixed_ip and port_id.
func TestAccComputeV2InterfaceAttach_conflictingFixedIPAndPort(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccPreCheckNonAdminOnly(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccComputeV2InterfaceAttachConflictingFixedIPAndPort(),
				ExpectError: regexpConflictsWith("fixed_ip", "port_id"),
				PlanOnly:    true,
			},
		},
	})
}

func testAccCheckComputeV2InterfaceAttachDestroy(ctx context.Context) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		config, err := testAccFrameworkProviderConfig()
		if err != nil {
			return err
		}

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

		config, err := testAccFrameworkProviderConfig()
		if err != nil {
			return err
		}

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

// testAccComputeV2InterfaceAttachConflictingPortAndNetwork sets both port_id
// and network_id, which the framework cross-attribute validator must reject.
// instance_id is set to a fake value because the test should fail at config
// validation before any API calls are made.
func testAccComputeV2InterfaceAttachConflictingPortAndNetwork() string {
	return `
resource "openstack_compute_interface_attach_v2" "ai_1" {
  instance_id = "00000000-0000-0000-0000-000000000000"
  port_id     = "00000000-0000-0000-0000-000000000001"
  network_id  = "00000000-0000-0000-0000-000000000002"
}
`
}

// testAccComputeV2InterfaceAttachConflictingFixedIPAndPort sets both fixed_ip
// and port_id, which the framework cross-attribute validator must reject.
func testAccComputeV2InterfaceAttachConflictingFixedIPAndPort() string {
	return `
resource "openstack_compute_interface_attach_v2" "ai_1" {
  instance_id = "00000000-0000-0000-0000-000000000000"
  port_id     = "00000000-0000-0000-0000-000000000001"
  fixed_ip    = "192.168.1.100"
}
`
}
